package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

type pairMailbox struct {
	createPermit                           [32]byte
	permitUsed                             bool
	permitIssued, approved                 bool
	locator                                string
	expires                                time.Time
	claimExpires                           time.Time
	claimSource                            string
	attempts                               int
	receiver                               [32]byte
	phase                                  int
	preparation                            []byte
	initial, reply, delivery, confirmation []byte
}

const (
	maxPairRequestBytes = 16 << 10
	pairClaimTTL        = 15 * time.Second
	pairExchangeTTL     = 30 * time.Second
	maxPairAttempts     = 3
)

type codeRequest struct {
	SessionID string `json:"session_id"`
	Data      struct {
		Locator string `json:"locator"`
		Message []byte `json:"message"`
	} `json:"data"`
}

func (s *Server) servePairCode(w http.ResponseWriter, r *http.Request, auth, source string) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	var req codeRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPairRequestBytes))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || len(req.Data.Message) > 8192 {
		http.Error(w, "invalid request", 400)
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	status, out := s.pairResponse(r.URL.Path, auth, source, req)
	if status != http.StatusOK {
		http.Error(w, "pairing code unavailable", status)
		return
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) pairResponse(path, auth, source string, req codeRequest) (int, any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if path == "/v1/device/create" {
		if !s.cfg.PublicDevicePairing || !s.publicCapacity(s.deviceCodes, source) || len(req.Data.Message) == 0 {
			return http.StatusForbidden, nil
		}
		id, e := token()
		if e != nil {
			return http.StatusServiceUnavailable, nil
		}
		owner, e := token()
		if e != nil {
			return http.StatusServiceUnavailable, nil
		}
		locator := ""
		for i := 0; i < 20; i++ {
			n, e := rand.Int(rand.Reader, big.NewInt(1000000))
			if e != nil {
				return http.StatusServiceUnavailable, nil
			}
			candidate := fmt.Sprintf("%06d", n)
			found := false
			for _, room := range s.codeRooms() {
				if room.pairCode != nil && room.pairCode.locator == candidate {
					found = true
					break
				}
			}
			if !found {
				locator = candidate
				break
			}
		}
		if locator == "" {
			return http.StatusServiceUnavailable, nil
		}
		until := time.Now().Add(5 * time.Minute)
		room := &room{credential: RoomCredentials{SessionID: id, ExpiresAt: until}, creatorHash: sha256.Sum256([]byte(owner)), pairCode: &pairMailbox{locator: locator, expires: until, initial: req.Data.Message}, publicSource: source}
		s.deviceCodes[id] = room
		return http.StatusOK, struct {
			ID      string `json:"session_id"`
			Token   string `json:"token"`
			Locator string `json:"locator"`
		}{id, owner, locator}
	}
	if path == "/v1/code/create" {
		room, role := s.authenticated(req.SessionID, auth)
		if room == nil || role != "merchant" || room.claimed || room.pairCode != nil || len(req.Data.Message) == 0 {
			return http.StatusForbidden, nil
		}
		var locator string
		for attempts := 0; attempts < 20; attempts++ {
			n, e := rand.Int(rand.Reader, big.NewInt(1000000))
			if e != nil {
				return http.StatusServiceUnavailable, nil
			}
			locator = fmt.Sprintf("%06d", n)
			exists := false
			for _, other := range s.codeRooms() {
				if other.pairCode != nil && other.pairCode.locator == locator {
					exists = true
					break
				}
			}
			if !exists {
				break
			}
			locator = ""
		}
		if locator == "" {
			return http.StatusServiceUnavailable, nil
		}
		until := time.Now().Add(5 * time.Minute)
		if room.credential.ExpiresAt.Before(until) {
			until = room.credential.ExpiresAt
		}
		room.pairCode = &pairMailbox{locator: locator, expires: until, initial: req.Data.Message}
		return http.StatusOK, struct {
			Locator string    `json:"locator"`
			Expires time.Time `json:"expires_at"`
		}{locator, until}
	}
	var selected *room
	var box *pairMailbox
	if req.SessionID != "" {
		selected = s.rooms[req.SessionID]
		if selected == nil {
			selected = s.deviceCodes[req.SessionID]
		}
		if selected != nil {
			box = selected.pairCode
		}
	} else {
		if len(req.Data.Locator) != 6 {
			return http.StatusForbidden, nil
		}
		for _, room := range s.codeRooms() {
			if room.pairCode != nil && hmac.Equal([]byte(room.pairCode.locator), []byte(req.Data.Locator)) {
				selected = room
				box = room.pairCode
				break
			}
		}
	}
	now := time.Now()
	if selected == nil || box == nil || !now.Before(box.expires) {
		return http.StatusForbidden, nil
	}
	device := s.deviceCodes[selected.credential.SessionID] == selected
	if box.phase == 1 && len(box.preparation) == 0 && !now.Before(box.claimExpires) {
		box.reset(box.initial)
	}
	if path == "/v1/code/claim" {
		if box.phase != 0 || box.attempts >= maxPairAttempts {
			return http.StatusForbidden, nil
		}
		receiver, e := token()
		if e != nil {
			return http.StatusServiceUnavailable, nil
		}
		box.receiver = sha256.Sum256([]byte(receiver))
		box.phase = 1
		box.attempts++
		box.claimSource = source
		box.claimExpires = minTime(box.expires, now.Add(pairClaimTTL))
		return http.StatusOK, struct {
			SessionID string `json:"session_id"`
			Token     string `json:"token"`
			Message   []byte `json:"message"`
		}{selected.credential.SessionID, receiver, box.initial}
	}
	supplied := sha256.Sum256([]byte(auth))
	creator := hmac.Equal(supplied[:], selected.creatorHash[:])
	receiver := box.phase > 0 && hmac.Equal(supplied[:], box.receiver[:])
	switch path {
	case "/v1/code/sender":
		if !creator {
			return http.StatusForbidden, nil
		}
		expired := device && box.phase == 1 && len(box.preparation) != 0 && !now.Before(box.claimExpires)
		if expired && box.approved {
			return http.StatusForbidden, nil
		}
		return http.StatusOK, struct {
			Phase        int    `json:"phase"`
			Message      []byte `json:"message"`
			Preparation  []byte `json:"preparation,omitempty"`
			Confirmation []byte `json:"confirmation"`
			Expired      bool   `json:"expired,omitempty"`
		}{box.phase, box.reply, box.preparation, box.confirmation, expired}
	case "/v1/code/cancel":
		if !creator {
			return http.StatusForbidden, nil
		}
		selected.pairCode = nil
		if device {
			delete(s.deviceCodes, selected.credential.SessionID)
		}
	case "/v1/code/prepare":
		if !device || !receiver || box.phase != 1 || len(box.preparation) != 0 || len(req.Data.Message) == 0 {
			return http.StatusForbidden, nil
		}
		box.preparation = req.Data.Message
		box.claimExpires = minTime(box.expires, now.Add(pairExchangeTTL))
	case "/v1/code/authorize":
		if !device || !creator || box.phase != 1 || box.approved || len(box.preparation) == 0 || !now.Before(box.claimExpires) {
			return http.StatusForbidden, nil
		}
		box.approved = true
	case "/v1/code/permit":
		if !device || !receiver || box.phase != 1 || box.claimSource != source || !box.approved || box.permitIssued || !now.Before(box.claimExpires) {
			return http.StatusForbidden, nil
		}
		permit, e := token()
		if e != nil {
			return http.StatusServiceUnavailable, nil
		}
		box.permitIssued = true
		box.createPermit = sha256.Sum256([]byte(permit))
		return http.StatusOK, struct {
			Permit string `json:"create_permit"`
		}{permit}
	case "/v1/code/reset":
		if !creator || box.approved || box.phase == 0 || len(req.Data.Message) == 0 {
			return http.StatusForbidden, nil
		}
		box.reset(req.Data.Message)
	case "/v1/code/reply":
		if !receiver || box.phase != 1 || len(req.Data.Message) == 0 || !now.Before(box.claimExpires) || device && !box.approved {
			return http.StatusForbidden, nil
		}
		box.reply = req.Data.Message
		box.phase = 2
	case "/v1/code/deliver":
		if !creator || box.phase != 2 || len(req.Data.Message) == 0 {
			return http.StatusForbidden, nil
		}
		box.delivery = req.Data.Message
		box.phase = 3
	case "/v1/code/receiver":
		if !receiver || box.phase == 1 && !now.Before(box.claimExpires) {
			return http.StatusForbidden, nil
		}
		return http.StatusOK, struct {
			Phase    int    `json:"phase"`
			Message  []byte `json:"message"`
			Approved bool   `json:"approved,omitempty"`
		}{box.phase, box.delivery, box.approved}
	case "/v1/code/finish":
		if !receiver || box.phase != 3 || len(req.Data.Message) != 32 {
			return http.StatusForbidden, nil
		}
		box.confirmation = req.Data.Message
		box.phase = 4
		box.initial = nil
		box.preparation = nil
		box.reply = nil
		box.delivery = nil
		if device {
			delete(s.deviceCodes, selected.credential.SessionID)
		}
	default:
		return http.StatusNotFound, nil
	}
	return http.StatusOK, struct{}{}
}

func (b *pairMailbox) reset(initial []byte) {
	*b = pairMailbox{locator: b.locator, expires: b.expires, attempts: b.attempts, initial: initial}
}
func (s *Server) codeRooms() []*room {
	rooms := make([]*room, 0, len(s.rooms)+len(s.deviceCodes))
	for _, r := range s.rooms {
		rooms = append(rooms, r)
	}
	for _, r := range s.deviceCodes {
		rooms = append(rooms, r)
	}
	return rooms
}
