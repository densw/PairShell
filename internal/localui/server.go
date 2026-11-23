package localui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"pairshell/internal/executor"
	"pairshell/internal/pairing"
	"pairshell/internal/session"
	"pairshell/internal/uiassets"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Sequence uint64 `json:"sequence"`
	Type     string `json:"type"`
	Data     string `json:"data"`
}
type Server struct {
	ReturnURL        string
	VerificationCode string
	ReadEvents       func(uint64) ([]Event, error)
	SelectFileRoot   func(string) (string, error)
	FileRoot         string
	Host             string
	Bootstrap        string
	controlToken     string
	mu               sync.Mutex
	consentMu        sync.Mutex
	used             bool
	last             time.Time
	gate             *session.Gate
	tasks            *executor.Manager
	record           func(string, session.Snapshot, string) error
}

func random() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func New(g *session.Gate, t *executor.Manager, record func(string, session.Snapshot, string) error) *Server {
	return &Server{Bootstrap: random(), controlToken: random(), gate: g, tasks: t, record: record, last: time.Now()}
}
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Host != s.Host {
		http.Error(w, "invalid host", 403)
		return
	}
	if r.URL.Path == "/" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, page)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/ui.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		io.WriteString(w, uiassets.CSS)
		return
	}
	if r.URL.Path == "/app.js" && r.Method == "GET" {
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, script)
		return
	}
	if r.Method == "GET" && (r.URL.Path == "/terminal-assets/xterm.js" || r.URL.Path == "/terminal-assets/xterm.css") {
		data, err := uiassets.TerminalAssets.ReadFile(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		contentType := "application/javascript"
		if strings.HasSuffix(r.URL.Path, ".css") {
			contentType = "text/css; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
		return
	}
	if r.URL.Path == "/terminal-view.js" && r.Method == "GET" {
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, uiassets.TerminalViewJS)
		return
	}
	if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.Host {
		http.Error(w, "invalid origin", 403)
		return
	}
	if r.URL.Path == "/api/bootstrap" && r.Method == "POST" {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("Origin") != "http://"+s.Host || s.used || !equal(r.Header.Get("X-Bootstrap"), s.Bootstrap) {
			http.Error(w, "unauthorized", 403)
			return
		}
		s.used = true
		s.last = time.Now()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Token string `json:"token"`
		}{s.controlToken})
		return
	}
	if !equal(r.Header.Get("X-PairShell-Control"), s.controlToken) {
		http.Error(w, "unauthorized", 403)
		return
	}
	if r.URL.Path == "/api/terminal" {
		if r.Method != "GET" {
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		after := uint64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			var err error
			after, err = strconv.ParseUint(raw, 10, 64)
			if err != nil {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
		}
		view := executor.TerminalView{Events: []executor.TerminalEvent{}}
		if s.tasks != nil {
			view = s.tasks.TerminalView(after)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(view)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/events" {
		after := uint64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			var err error
			after, err = strconv.ParseUint(raw, 10, 64)
			if err != nil {
				http.Error(w, "invalid cursor", 400)
				return
			}
		}
		events := []Event{}
		if s.ReadEvents != nil {
			var err error
			events, err = s.ReadEvents(after)
			if err != nil {
				http.Error(w, "events unavailable", 503)
				return
			}
		}
		if len(events) > 64 {
			events = events[:64]
		}
		json.NewEncoder(w).Encode(events)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/tasks" {
		if s.tasks == nil {
			json.NewEncoder(w).Encode([]executor.VisibleTask{})
			return
		}
		json.NewEncoder(w).Encode(s.tasks.List())
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/session" {
		s.mu.Lock()
		fileRoot := s.FileRoot
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Session        session.Snapshot `json:"session"`
			Code           string           `json:"verification_code"`
			ReturnURL      string           `json:"return_url,omitempty"`
			FileRoot       string           `json:"file_root,omitempty"`
			SelectFileRoot bool             `json:"select_file_root"`
		}{s.gate.Snapshot(), s.VerificationCode, s.ReturnURL, fileRoot, s.SelectFileRoot != nil})
		return
	}
	if r.Method != "POST" || r.Header.Get("Origin") != "http://"+s.Host {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.URL.Path == "/api/heartbeat" {
		s.mu.Lock()
		s.last = time.Now()
		s.mu.Unlock()
		io.WriteString(w, "{}")
		return
	}
	var e error
	var fileRoot string
	switch r.URL.Path {
	case "/api/consent":
		s.consentMu.Lock()
		defer s.consentMu.Unlock()
		var v struct {
			Capabilities     session.Capabilities `json:"capabilities"`
			AI               bool                 `json:"ai"`
			FileRoot         string               `json:"file_root"`
			VerificationCode string               `json:"verification_code"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		var tail any
		if d.Decode(&v) != nil || d.Decode(&tail) != io.EOF {
			http.Error(w, "invalid request", 400)
			return
		}
		if s.gate.Snapshot().State != "AWAITING_CONSENT" {
			http.Error(w, "CONSENT_REQUIRED", 409)
			return
		}
		if !pairing.MatchVerificationCode(s.VerificationCode, v.VerificationCode) {
			http.Error(w, "校验码不匹配。请通过已有的聊天或电话联系协助方，无法确认时结束本次协助", http.StatusConflict)
			return
		}
		if v.Capabilities.FileRead || v.Capabilities.FileWrite {
			fileRoot = s.FileRoot
			if s.SelectFileRoot != nil {
				fileRoot, e = s.SelectFileRoot(v.FileRoot)
				if e != nil {
					http.Error(w, e.Error(), 400)
					return
				}
			}
		}
		requested := s.gate.Snapshot()
		requested.Capabilities, requested.AI = v.Capabilities, v.AI
		if !s.auditControl(w, "consent_requested", requested, fileRoot) {
			return
		}
		e = s.gate.Consent(v.Capabilities, v.AI)
		if e == nil && fileRoot != "" {
			s.mu.Lock()
			s.FileRoot = fileRoot
			s.mu.Unlock()
		}
	case "/api/pause":
		e = s.gate.Pause()
	case "/api/resume":
		e = s.gate.Resume()
	case "/api/end":
		s.end()
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	result := s.gate.Snapshot()
	if !s.auditControl(w, strings.TrimPrefix(r.URL.Path, "/api/"), result, fileRoot) {
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (s *Server) auditControl(w http.ResponseWriter, action string, snapshot session.Snapshot, fileRoot string) bool {
	if s.record != nil && s.record(action, snapshot, fileRoot) != nil {
		s.gate.AuditFailure()
		http.Error(w, "AUDIT_IO_FAILED", 503)
		return false
	}
	return true
}

func (s *Server) end() {
	s.gate.End()
	if s.tasks != nil {
		go s.tasks.Close()
	}
}

func (s *Server) Watchdog(stop <-chan struct{}) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			if s.gate.Snapshot().State == "ENDED" {
				return
			}
			s.mu.Lock()
			elapsed := time.Since(s.last)
			s.mu.Unlock()
			if elapsed > 15*time.Second {
				s.end()
				return
			} else if elapsed > 5*time.Second {
				s.gate.Pause()
			}
		}
	}
}

var page = uiassets.Control

var script = uiassets.ControlJS
