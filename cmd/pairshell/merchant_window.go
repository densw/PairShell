package main

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"net/http"
	"os"
	"pairshell/internal/audit"
	"pairshell/internal/config"
	"pairshell/internal/paircode"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
	"pairshell/internal/transport"
	"pairshell/internal/uiassets"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type merchantAttempt struct {
	id         uint64
	ctx        context.Context
	cancel     context.CancelFunc
	confirm    chan struct{}
	client     *ssh.Client
	audit      *audit.Writer
	auditError error
}

// Each window can reconnect; each attempt remains bound to exactly one peer.
type merchantWindow struct {
	httpClient                                     *http.Client
	mu                                             sync.Mutex
	ctx                                            context.Context
	cancel                                         context.CancelFunc
	state, invite, code, output, reference, system string
	phase                                          string
	attemptID                                      uint64
	active                                         *merchantAttempt
	client                                         *ssh.Client
	remote                                         *remote.Merchant
	running                                        bool
	logRoot                                        string
}

func (m *merchantWindow) currentLocked(a *merchantAttempt) bool {
	return a != nil && m.active == a && m.attemptID == a.id
}
func (m *merchantWindow) canConnectLocked() bool {
	return m.phase == "" || m.phase == "idle" || m.phase == "error" || m.phase == "ended"
}
func (m *merchantWindow) beginAttemptLocked() *merchantAttempt {
	root := m.ctx
	if root == nil {
		root = context.Background()
	}
	ctx, cancel := context.WithTimeout(root, 2*time.Hour)
	m.attemptID++
	a := &merchantAttempt{id: m.attemptID, ctx: ctx, cancel: cancel, confirm: make(chan struct{})}
	if m.logRoot != "" {
		id := "merchant-" + token()
		a.audit, a.auditError = audit.NewWriter(filepath.Join(m.logRoot, "session-"+id), id)
		if a.audit != nil {
			a.auditError = a.audit.Append(audit.Event{Actor: "merchant", EventType: "connection.started", AuthorizationEpoch: 1, Payload: json.RawMessage(`{"role":"merchant"}`)})
		}
	}
	m.active = a
	m.cancel = cancel
	m.client = nil
	m.remote = nil
	m.running = false
	m.invite = ""
	m.code = ""
	m.output = ""
	m.reference = ""
	m.system = ""
	m.phase = "pairing"
	return a
}
func retireMerchantAttempt(a *merchantAttempt) {
	if a == nil {
		return
	}
	if a.client != nil {
		a.client.Close()
	}
	a.cancel()
	if a.audit != nil {
		a.audit.Close("connection closed")
	}
}
func (m *merchantWindow) finishAttempt(a *merchantAttempt, message, phase string) {
	m.mu.Lock()
	if !m.currentLocked(a) {
		m.mu.Unlock()
		return
	}
	m.active = nil
	m.client = nil
	m.remote = nil
	m.running = false
	m.reference = ""
	m.invite = ""
	m.code = ""
	m.system = ""
	m.state = message
	m.phase = phase
	m.mu.Unlock()
	retireMerchantAttempt(a)
}
func (m *merchantWindow) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/merchant/settings" {
		cfg, e := config.LoadDefault()
		if e != nil {
			http.Error(w, "无法读取服务器配置", 500)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Server            string
			ForceCustom       bool   `json:"force_custom"`
			OfficialAvailable bool   `json:"official_available"`
			OfficialServer    string `json:"official_server"`
		}{cfg.Server, cfg.ForceCustom, config.OfficialServer != "", config.OfficialServer})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/merchant/state" {
		m.mu.Lock()
		defer m.mu.Unlock()
		route := transport.RouteStatus{Selected: "connecting"}
		if m.remote != nil {
			route = m.remote.RouteStatus()
		}
		phase := m.phase
		if phase == "" {
			phase = "idle"
		}
		json.NewEncoder(w).Encode(struct {
			State, Invite, Code, Output, Reference, System string
			Running                                        bool
			Route                                          transport.RouteStatus
			Phase                                          string
			CanConnect, CanEnd                             bool
			AttemptID                                      uint64
		}{m.state, m.invite, m.code, m.output, m.reference, m.system, m.running, route, phase, m.canConnectLocked(), m.active != nil, m.attemptID})
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	var v struct {
		Server, Token, Command, Code string
		ForceCustom                  bool   `json:"force_custom"`
		AttemptID                    uint64 `json:"attempt_id"`
		VerificationCode             string `json:"verification_code"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	var tail any
	if d.Decode(&v) != nil || d.Decode(&tail) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	m.mu.Lock()
	switch r.URL.Path {
	case "/merchant/settings":
		if !m.canConnectLocked() {
			m.mu.Unlock()
			http.Error(w, "请先结束当前连接再修改服务器", 409)
			return
		}
		m.mu.Unlock()
		if e := config.UpdatePreferences(v.Server, v.ForceCustom); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
	case "/merchant/join", "/merchant/create":
		if !m.canConnectLocked() {
			m.mu.Unlock()
			http.Error(w, "当前连接尚未结束，请先取消或结束", 409)
			return
		}
		if r.URL.Path == "/merchant/join" && strings.TrimSpace(v.Code) == "" {
			m.mu.Unlock()
			http.Error(w, "请输入配对码", 400)
			return
		}
		old := m.active
		a := m.beginAttemptLocked()
		byCode := r.URL.Path == "/merchant/join"
		if byCode {
			m.state = "正在验证配对码"
		} else {
			m.state = "正在创建邀请"
		}
		m.mu.Unlock()
		go func() {
			retireMerchantAttempt(old)
			if byCode {
				m.connectCodeAttempt(a, v.Code)
			} else {
				m.connectAttempt(a, v.Server, v.Token)
			}
		}()
	case "/merchant/confirm":
		if m.active == nil || m.active.id != v.AttemptID || m.state != "等待商家核对身份" {
			m.mu.Unlock()
			http.Error(w, "本次身份核对已失效，请查看当前连接状态", http.StatusConflict)
			return
		}
		if !pairing.MatchVerificationCode(m.code, v.VerificationCode) {
			m.mu.Unlock()
			http.Error(w, "校验码不匹配。请通过已有的聊天或电话联系客户，无法确认时结束本次连接", http.StatusConflict)
			return
		}
		close(m.active.confirm)
		m.state = "等待客户授权并建立连接"
		m.phase = "awaiting_consent"
		m.mu.Unlock()
	case "/merchant/exec":
		if m.client == nil || m.state != "已连接" || m.running || strings.TrimSpace(v.Command) == "" {
			m.mu.Unlock()
			http.Error(w, "连接未就绪或终端正在使用", 409)
			return
		}
		a := m.active
		m.running = true
		m.output = ""
		m.mu.Unlock()
		go m.execAttempt(a, v.Command)
	case "/merchant/end":
		a := m.active
		m.active = nil
		m.client = nil
		m.remote = nil
		m.running = false
		m.invite = ""
		m.code = ""
		m.reference = ""
		m.system = ""
		m.phase = "ended"
		m.state = "连接已结束，可以连接另一台设备"
		m.mu.Unlock()
		retireMerchantAttempt(a)
	default:
		m.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	io.WriteString(w, "{}")
}
func (m *merchantWindow) connectAttempt(a *merchantAttempt, server, credential string) {
	if a.auditError != nil {
		m.finishAttempt(a, "无法创建本机操作记录，请检查磁盘和权限后重试", "error")
		return
	}
	bases, e := config.ServerCandidates(server)
	if e != nil {
		m.finishAttempt(a, "服务器配置无效，请修改后重试", "error")
		return
	}
	var c *remote.Merchant
	for i, base := range bases {
		deploy := credential
		if i > 0 {
			deploy = ""
		}
		c, e = remote.Create(a.ctx, base, deploy, m.httpClient)
		if e == nil || a.ctx.Err() != nil {
			break
		}
	}
	if e != nil {
		m.finishAttempt(a, "创建失败，请检查服务器与凭据后重试", "error")
		return
	}
	m.mu.Lock()
	if !m.currentLocked(a) {
		m.mu.Unlock()
		retireMerchantAttempt(a)
		return
	}
	m.invite = c.Invite.URL()
	m.state = "邀请已创建，等待客户"
	m.mu.Unlock()
	m.connectAttemptSession(a, c)
}
func (m *merchantWindow) connectCodeAttempt(a *merchantAttempt, code string) {
	if a.auditError != nil {
		m.finishAttempt(a, "无法创建本机操作记录，请检查磁盘和权限后重试", "error")
		return
	}
	bases, e := config.ServerCandidates("")
	if e != nil {
		m.finishAttempt(a, "服务器配置无效，请修改后重试", "error")
		return
	}
	var c *remote.Merchant
	for i, base := range bases {
		deploy := os.Getenv("PAIRSHELL_CREATE_TOKEN")
		if i > 0 {
			deploy = ""
		}
		c, e = remote.ConnectCode(a.ctx, base, strings.TrimSpace(code), deploy, m.httpClient)
		if e == nil || a.ctx.Err() != nil {
			break
		}
	}
	if e != nil {
		m.finishAttempt(a, paircode.Explain(e), "error")
		return
	}
	m.connectAttemptSession(a, c)
}
func (m *merchantWindow) connectAttemptSession(a *merchantAttempt, c *remote.Merchant) {
	m.mu.Lock()
	if !m.currentLocked(a) {
		m.mu.Unlock()
		retireMerchantAttempt(a)
		return
	}
	m.remote = c
	m.mu.Unlock()
	var peerKey []byte
	client, e := c.Connect(a.ctx, func(tr pairing.Transcript, code string) error {
		peerKey = append([]byte(nil), tr.CustomerKey...)
		m.mu.Lock()
		if !m.currentLocked(a) {
			m.mu.Unlock()
			return context.Canceled
		}
		m.code = code
		m.phase = "awaiting_consent"
		m.state = "等待商家核对身份"
		m.mu.Unlock()
		select {
		case <-a.confirm:
			return nil
		case <-a.ctx.Done():
			return a.ctx.Err()
		}
	})
	if e != nil {
		m.finishAttempt(a, "连接未完成，请确认对方允许本次协助后使用新码重试", "error")
		return
	}
	m.mu.Lock()
	if !m.currentLocked(a) || a.ctx.Err() != nil {
		m.mu.Unlock()
		client.Close()
		return
	}
	a.client = client
	m.client = client
	m.state = "已连接"
	m.phase = "active"
	m.invite = ""
	m.mu.Unlock()
	go func() {
		r, _ := http.NewRequest("POST", "http://customer/bridge/session_info", strings.NewReader("{}"))
		status, body, e := merchantRPC(client, r, 4096, 10*time.Second)
		if e != nil || status != http.StatusOK {
			return
		}
		var info struct{ OS, Arch string }
		if json.Unmarshal(body, &info) == nil {
			m.mu.Lock()
			if m.currentLocked(a) {
				m.system = info.OS + " / " + info.Arch
			}
			m.mu.Unlock()
		}
	}()
	go func() {
		e := serveMerchantAdapters(a.ctx, client, c, peerKey, "", func(ref string) {
			m.mu.Lock()
			if m.currentLocked(a) {
				m.reference = ref
			}
			m.mu.Unlock()
		})
		if e != nil {
			m.mu.Lock()
			if m.currentLocked(a) {
				m.output = "CLI/MCP 适配器未启动，请检查本机权限。"
			}
			m.mu.Unlock()
		}
	}()
	go func() { <-a.ctx.Done(); client.Close() }()
	client.Wait()
	m.finishAttempt(a, "连接已结束，可以输入新配对码再次连接", "ended")
}

type merchantOutput struct {
	m *merchantWindow
	a *merchantAttempt
}

func (o merchantOutput) Write(p []byte) (int, error) {
	o.m.mu.Lock()
	defer o.m.mu.Unlock()
	if !o.m.currentLocked(o.a) {
		return 0, context.Canceled
	}
	if len(o.m.output)+len(p) > 4<<20 {
		return 0, errors.New("输出超过 4MiB 上限")
	}
	o.m.output += string(p)
	return len(p), nil
}
func (m *merchantWindow) execAttempt(a *merchantAttempt, command string) {
	if a == nil || a.client == nil {
		return
	}
	s, e := a.client.NewSession()
	if e == nil {
		s.Stdout = merchantOutput{m, a}
		s.Stderr = merchantOutput{m, a}
		e = s.Run(command)
		s.Close()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.currentLocked(a) {
		return
	}
	m.running = false
	if e != nil {
		m.output += "\n[未成功完成：" + e.Error() + "]"
	} else {
		m.output += "\n[任务完成]"
	}
}
func merchantPage() string { return uiassets.Merchant }
