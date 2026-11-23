package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	leaseTTL             = 30 * time.Second
	anonymousRoomTTL     = 45 * time.Second
	maxAnonymousBySource = 4
	maxSignalQueueBytes  = 224 << 10
	maxSourceEntries     = 4096
	maxConcurrent        = 64
	maxSourceConcurrent  = 8
)

type Config struct {
	CreateToken  string
	PublicCreate bool
	// PublicDevicePairing explicitly opts into anonymous one-room admission.
	PublicDevicePairing bool
	MaxRooms            int
	RoomTTL             time.Duration
	MaxSessionDuration  time.Duration
	MaxMessageBytes     int64
	TrustedProxies      []netip.Prefix
	TURNSecret          string
	TURNURLs            []string
	TURNTTL             time.Duration
}
type RoomCredentials struct {
	SessionID    string    `json:"session_id"`
	CreatorToken string    `json:"creator_token"`
	JoinToken    string    `json:"join_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type Message struct {
	Role string          `json:"role"`
	Data json.RawMessage `json:"data"`
}
type room struct {
	pairCode                         *pairMailbox
	credential                       RoomCredentials
	creatorHash                      [32]byte
	joinHash                         [32]byte
	customerHash                     [32]byte
	claimed                          bool
	merchantKey                      string
	customerKey                      string
	toMerchant                       []Message
	toCustomer                       []Message
	merchantBytes, customerBytes     int
	merchantLease, customerLease     time.Time
	merchantReady, customerReady     bool
	merchantSignals, customerSignals budget
	publicSource                     string
	turnExpiry                       time.Time
}
type budget struct {
	updated time.Time
	credit  float64
}

func (b *budget) take(now time.Time, cost, perSecond, burst int) bool {
	if b.updated.IsZero() {
		b.credit = float64(burst)
	} else {
		b.credit = min(float64(burst), b.credit+now.Sub(b.updated).Seconds()*float64(perSecond))
	}
	b.updated = now
	if b.credit < float64(cost) {
		return false
	}
	b.credit -= float64(cost)
	return true
}

type sourceLimit struct {
	requests budget
	bytes    budget
	inFlight int
}
type Server struct {
	mu          sync.Mutex
	cfg         Config
	rooms       map[string]*room
	limits      map[string]*sourceLimit
	requests    budget
	bytes       budget
	inFlight    int
	deviceCodes map[string]*room
}

func New(c Config) *Server {
	if c.MaxRooms <= 0 {
		c.MaxRooms = 100
	}
	if c.RoomTTL <= 0 {
		c.RoomTTL = 10 * time.Minute
	}
	if c.MaxSessionDuration <= 0 {
		c.MaxSessionDuration = 2 * time.Hour
	}
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = 262144
	}
	if c.TURNTTL <= 0 {
		c.TURNTTL = 10 * time.Minute
	}
	return &Server{cfg: c, rooms: map[string]*room{}, limits: map[string]*sourceLimit{}, deviceCodes: map[string]*room{}}
}
func token() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func (s *Server) prune() {
	now := time.Now()
	for id, room := range s.deviceCodes {
		if room.pairCode == nil || !now.Before(room.pairCode.expires) {
			delete(s.deviceCodes, id)
		}
	}
	for id, r := range s.rooms {
		if !now.Before(r.credential.ExpiresAt) || r.claimed && (!now.Before(r.merchantLease) || !now.Before(r.customerLease)) {
			delete(s.rooms, id)
		}
	}
	for ip, l := range s.limits {
		if l.inFlight == 0 && now.Sub(l.requests.updated) > time.Minute {
			delete(s.limits, ip)
		}
	}
}

// RunPruner releases expired state even when no requests arrive.
func (s *Server) RunPruner(done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.prune()
			s.mu.Unlock()
		}
	}
}

func (s *Server) sourceKey(r *http.Request) string {
	address, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	ip := address.Addr().Unmap().WithZone("")
	if s.trustedProxy(ip) {
		peer := ip
		forwards := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		for i := len(forwards) - 1; i >= 0 && s.trustedProxy(ip); i-- {
			next, e := netip.ParseAddr(strings.TrimSpace(forwards[i]))
			if e != nil || next.Zone() != "" {
				ip = peer
				break
			}
			ip = next.Unmap()
		}
	}
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

func (s *Server) trustedProxy(ip netip.Addr) bool {
	for _, p := range s.cfg.TrustedProxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Reserve capacity for administrator-created rooms when MaxRooms permits it.
func (s *Server) publicCapacity(rooms map[string]*room, source string) bool {
	public, local := 0, 0
	for _, r := range rooms {
		if r.publicSource != "" {
			public++
		}
		if r.publicSource == source {
			local++
		}
	}
	return public < max(1, s.cfg.MaxRooms*3/4) && local < maxAnonymousBySource
}

func (s *Server) Create(deploy, id, key string) (RoomCredentials, error) {
	return s.create(deploy, id, key, "unknown")
}

func (s *Server) create(deploy, id, key, source string) (RoomCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	var permit *pairMailbox
	publicSource := ""
	if s.cfg.CreateToken == "" || !hmac.Equal([]byte(deploy), []byte(s.cfg.CreateToken)) {
		publicSource = source
		hash := sha256.Sum256([]byte(deploy))
		for _, device := range s.deviceCodes {
			box := device.pairCode
			if box != nil && box.approved && box.permitIssued && !box.permitUsed && box.claimSource == source && time.Now().Before(box.claimExpires) && hmac.Equal(hash[:], box.createPermit[:]) {
				permit = box
				break
			}
		}
		if permit == nil && !s.cfg.PublicCreate {
			return RoomCredentials{}, errors.New("unauthorized")
		}
		if !s.publicCapacity(s.rooms, source) {
			return RoomCredentials{}, errors.New("room unavailable")
		}
	}
	if len(id) < 16 || len(id) > 128 || len(key) > 128 || key == "" || len(s.rooms) >= s.cfg.MaxRooms || s.rooms[id] != nil {
		return RoomCredentials{}, errors.New("room unavailable")
	}
	creator, e := token()
	if e != nil {
		return RoomCredentials{}, e
	}
	join, e := token()
	if e != nil {
		return RoomCredentials{}, e
	}
	ttl := s.cfg.RoomTTL
	if publicSource != "" {
		ttl = min(ttl, anonymousRoomTTL)
	}
	c := RoomCredentials{id, creator, join, time.Now().Add(ttl)}
	s.rooms[id] = &room{credential: RoomCredentials{SessionID: id, ExpiresAt: c.ExpiresAt}, creatorHash: sha256.Sum256([]byte(creator)), joinHash: sha256.Sum256([]byte(join)), merchantKey: key, publicSource: publicSource}
	if permit != nil {
		permit.permitUsed = true
	}
	return c, nil
}
func (s *Server) Claim(id, join, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	r := s.rooms[id]
	h := sha256.Sum256([]byte(join))
	if r == nil || r.claimed || !hmac.Equal(h[:], r.joinHash[:]) || key == "" || len(key) > 128 {
		return errors.New("room unavailable")
	}
	r.claimed = true
	now := time.Now()
	r.credential.ExpiresAt = now.Add(s.cfg.MaxSessionDuration)
	r.merchantLease, r.customerLease = now.Add(leaseTTL), now.Add(leaseTTL)
	r.customerHash = h
	r.customerKey = key
	r.joinHash = [32]byte{}
	return nil
}
func (s *Server) authenticated(id, t string) (*room, string) {
	r := s.rooms[id]
	if r == nil {
		return nil, ""
	}
	h := sha256.Sum256([]byte(t))
	if hmac.Equal(h[:], r.creatorHash[:]) {
		return r, "merchant"
	}
	if r.claimed && hmac.Equal(h[:], r.customerHash[:]) {
		return r, "customer"
	}
	return nil, ""
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		io.WriteString(w, "{\"status\":\"ok\"}\n")
		return
	}
	source := s.sourceKey(r)
	pairRoute := strings.HasPrefix(r.URL.Path, "/v1/code/") || r.URL.Path == "/v1/device/create"
	bodyLimit := s.cfg.MaxMessageBytes
	if pairRoute {
		bodyLimit = maxPairRequestBytes
	}
	bodyBytes := r.ContentLength
	if bodyBytes < 0 {
		// For streaming request bodies reserve the entire route allowance.
		bodyBytes = bodyLimit
	}
	if bodyBytes > bodyLimit {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !s.admit(source, int(bodyBytes)) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	defer s.release(source)
	auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		auth = ""
	}
	if pairRoute {
		s.servePairCode(w, r, auth, source)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	body := http.MaxBytesReader(w, r.Body, s.cfg.MaxMessageBytes)
	d := json.NewDecoder(body)
	d.DisallowUnknownFields()
	var req roomRequest
	var extra any
	if d.Decode(&req) != nil || d.Decode(&extra) != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	switch r.URL.Path {
	case "/v1/rooms":
		c, e := s.create(auth, req.SessionID, req.Key, source)
		if e != nil {
			http.Error(w, "room unavailable", http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode(c)
		return
	case "/v1/claim":
		if s.Claim(req.SessionID, auth, req.Key) != nil {
			http.Error(w, "room unavailable", http.StatusForbidden)
			return
		}
		io.WriteString(w, "{\"status\":\"claimed\"}\n")
		return
	}
	status, out := s.roomResponse(r.URL.Path, auth, req)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) admit(source string, requestBytes int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight >= maxConcurrent {
		return false
	}
	now := time.Now()
	l := s.limits[source]
	if l == nil {
		if len(s.limits) >= maxSourceEntries {
			s.prune()
			if len(s.limits) >= maxSourceEntries {
				return false
			}
		}
		l = &sourceLimit{}
	}
	if l.inFlight >= maxSourceConcurrent ||
		!l.requests.take(now, 1, 12, 32) ||
		!l.bytes.take(now, requestBytes, 256<<10, 512<<10) ||
		!s.requests.take(now, 1, 256, 512) ||
		!s.bytes.take(now, requestBytes, 4<<20, 8<<20) {
		return false
	}
	s.limits[source] = l
	l.inFlight++
	s.inFlight++
	return true
}

func (s *Server) release(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits[source].inFlight--
	s.inFlight--
}

type roomRequest struct {
	SessionID string          `json:"session_id"`
	Key       string          `json:"key"`
	Data      json.RawMessage `json:"data"`
}

func (s *Server) roomResponse(path, auth string, req roomRequest) (int, any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	room, role := s.authenticated(req.SessionID, auth)
	if room == nil {
		return http.StatusForbidden, nil
	}
	switch path {
	case "/v1/lease":
		now := time.Now()
		expires := now.Add(leaseTTL)
		if role == "customer" {
			room.customerLease = expires
		} else {
			room.merchantLease = expires
		}
		return http.StatusOK, struct {
			Expires time.Time `json:"expires_at"`
		}{expires}
	case "/v1/peer":
		peer := room.customerKey
		if role == "customer" {
			peer = room.merchantKey
		}
		return http.StatusOK, struct {
			Key string `json:"key"`
		}{peer}
	case "/v1/ready":
		if !room.claimed {
			return http.StatusForbidden, nil
		}
		if role == "customer" {
			room.customerReady = true
		} else {
			room.merchantReady = true
		}
	case "/v1/signal":
		if len(req.Data) == 0 {
			return http.StatusBadRequest, nil
		}
		queue := &room.toCustomer
		size := &room.customerBytes
		signals := &room.merchantSignals
		if role == "customer" {
			queue = &room.toMerchant
			size = &room.merchantBytes
			signals = &room.customerSignals
		}
		// Include overhead so a poll response fits the client's JSON read limit.
		cost := len(req.Data) + 128
		if cost > maxSignalQueueBytes {
			return http.StatusRequestEntityTooLarge, nil
		}
		if len(*queue) >= 64 || cost+*size > maxSignalQueueBytes || !signals.take(time.Now(), cost, 128<<10, maxSignalQueueBytes) {
			return http.StatusTooManyRequests, nil
		}
		*queue = append(*queue, Message{role, req.Data})
		*size += cost
	case "/v1/poll":
		queue := &room.toMerchant
		size := &room.merchantBytes
		if role == "customer" {
			queue = &room.toCustomer
			size = &room.customerBytes
		}
		out := *queue
		*queue, *size = nil, 0
		return http.StatusOK, out
	case "/v1/end":
		delete(s.rooms, req.SessionID)
	case "/v1/turn":
		if !room.claimed || !room.merchantReady || !room.customerReady {
			return http.StatusForbidden, nil
		}
		if s.cfg.TURNSecret == "" {
			return http.StatusServiceUnavailable, nil
		}
		now := time.Now()
		if !now.Before(room.turnExpiry) {
			room.turnExpiry = minTime(now.Add(s.cfg.TURNTTL), room.credential.ExpiresAt)
		}
		username := fmt.Sprintf("%d:%s", room.turnExpiry.Unix(), req.SessionID)
		mac := hmac.New(sha1.New, []byte(s.cfg.TURNSecret))
		mac.Write([]byte(username))
		return http.StatusOK, struct {
			URLs       []string `json:"urls"`
			Username   string   `json:"username"`
			Credential string   `json:"credential"`
		}{s.cfg.TURNURLs, username, base64.StdEncoding.EncodeToString(mac.Sum(nil))}
	default:
		return http.StatusNotFound, nil
	}
	return http.StatusOK, struct{}{}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
