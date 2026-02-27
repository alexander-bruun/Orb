// Package listenparty implements the "listen along" feature: a host creates a
// session and shares an invite link; guests join, enter a nickname, and hear
// the same music in sync via WebSocket-driven playback state broadcasts.
package listenparty

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexander-bruun/orb/pkg/kvkeys"
	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/alexander-bruun/orb/services/api/internal/stream"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	sessionTTL    = 12 * time.Hour
	guestTokenTTL = 6 * time.Hour
	writeWait     = 10 * time.Second
	pongWait      = 60 * time.Second
	pingInterval  = (pongWait * 9) / 10
	maxMsgSize    = 4096
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	CheckOrigin:      func(_ *http.Request) bool { return true },
}

// --- Domain types ---

// PlaybackState holds the current playback snapshot stored in Redis and
// broadcast to guests on join and on each sync_state from the host.
type PlaybackState struct {
	TrackID      string  `json:"track_id"`
	PositionMs   float64 `json:"position_ms"`
	Playing      bool    `json:"playing"`
	ServerTimeMs int64   `json:"server_time_ms"` // unix ms when state was set
}

// Session is the data persisted in Redis for a listen-along session.
type Session struct {
	ID        string        `json:"id"`
	HostID    string        `json:"host_id"`
	HostName  string        `json:"host_name"`
	CreatedAt time.Time     `json:"created_at"`
	State     PlaybackState `json:"state"`
}

// Participant represents a guest connected to a session.
type Participant struct {
	ID         string    `json:"id"`
	Nickname   string    `json:"nickname"`
	JoinedAt   time.Time `json:"joined_at"`
}

// TrackInfo carries just enough track metadata for the guest player to display
// and stream a track without needing JWT-authenticated library API calls.
type TrackInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	ArtistName string  `json:"artist_name"`
	AlbumID    string  `json:"album_id"`
	BitDepth   int32   `json:"bit_depth"`
	SampleRate int32   `json:"sample_rate"`
	DurationMs int32   `json:"duration_ms"`
}

// --- WebSocket message types ---

type inMsg struct {
	Type          string         `json:"type"`
	Nickname      string         `json:"nickname,omitempty"`
	State         *PlaybackState `json:"state,omitempty"`
	ParticipantID string         `json:"participant_id,omitempty"`
}

type outMsg struct {
	Type          string         `json:"type"`
	Role          string         `json:"role,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	ParticipantID string         `json:"participant_id,omitempty"`
	GuestToken    string         `json:"guest_token,omitempty"`
	CurrentState  *PlaybackState `json:"current_state,omitempty"`
	State         *PlaybackState `json:"state,omitempty"`
	TrackInfo     *TrackInfo     `json:"track_info,omitempty"`
	Participants  []Participant  `json:"participants,omitempty"`
	Participant   *Participant   `json:"participant,omitempty"`
	Message       string         `json:"message,omitempty"`
}

// --- Hub ---

type client struct {
	hub      *hub
	conn     *websocket.Conn
	send     chan []byte
	id       string
	nickname string
	isHost   bool
}

type hub struct {
	sessionID  string
	host       *client
	guests     map[string]*client
	mu         sync.RWMutex
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	done       chan struct{}
}

func newHub(sessionID string) *hub {
	return &hub{
		sessionID:  sessionID,
		guests:     make(map[string]*client),
		broadcast:  make(chan []byte, 64),
		register:   make(chan *client, 8),
		unregister: make(chan *client, 8),
		done:       make(chan struct{}),
	}
}

func (h *hub) run() {
	for {
		select {
		case <-h.done:
			return
		case c := <-h.register:
			h.mu.Lock()
			if c.isHost {
				h.host = c
			} else {
				h.guests[c.id] = c
			}
			h.mu.Unlock()
		case c := <-h.unregister:
			h.mu.Lock()
			if c.isHost {
				h.host = nil
			} else {
				delete(h.guests, c.id)
			}
			h.mu.Unlock()
			close(c.send)
		case msg := <-h.broadcast:
			h.mu.RLock()
			sent, dropped := 0, 0
			for _, g := range h.guests {
				select {
				case g.send <- msg:
					sent++
				default:
					dropped++
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *hub) sendToHost(msg []byte) {
	h.mu.RLock()
	host := h.host
	h.mu.RUnlock()
	if host == nil {
		return
	}
	select {
	case host.send <- msg:
	default:
	}
}

func (h *hub) sendToGuest(participantID string, msg []byte) bool {
	h.mu.RLock()
	g, ok := h.guests[participantID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case g.send <- msg:
		return true
	default:
		return false
	}
}

func (h *hub) participants() []Participant {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]Participant, 0, len(h.guests))
	for _, g := range h.guests {
		list = append(list, Participant{ID: g.id, Nickname: g.nickname, JoinedAt: time.Now()})
	}
	return list
}

func (h *hub) kickGuest(participantID string) {
	h.mu.Lock()
	g, ok := h.guests[participantID]
	if ok {
		delete(h.guests, participantID)
	}
	h.mu.Unlock()
	if !ok {
		return
	}
	msg := mustMarshal(outMsg{Type: "kicked"})
	select {
	case g.send <- msg:
	default:
	}
	// Give the write pump a moment to flush.
	time.AfterFunc(500*time.Millisecond, func() { g.conn.Close() })
}

func (h *hub) shutdown() {
	select {
	case <-h.done:
	default:
		close(h.done)
	}
	msg := mustMarshal(outMsg{Type: "session_ended"})
	h.mu.RLock()
	for _, g := range h.guests {
		select {
		case g.send <- msg:
		default:
		}
	}
	h.mu.RUnlock()
}

// --- Global hub registry ---

var (
	hubs   = map[string]*hub{}
	hubsMu sync.RWMutex
)

func getHub(sessionID string) *hub {
	hubsMu.RLock()
	defer hubsMu.RUnlock()
	return hubs[sessionID]
}

func getOrCreateHub(sessionID string) *hub {
	hubsMu.Lock()
	defer hubsMu.Unlock()
	h, ok := hubs[sessionID]
	if !ok {
		h = newHub(sessionID)
		hubs[sessionID] = h
		go h.run()
	}
	return h
}

func removeHub(sessionID string) {
	hubsMu.Lock()
	h, ok := hubs[sessionID]
	if ok {
		delete(hubs, sessionID)
	}
	hubsMu.Unlock()
	if ok {
		h.shutdown()
	}
}

// --- Service ---

// Service implements the listen-along HTTP/WebSocket routes.
type Service struct {
	db         *store.Store
	kv         *redis.Client
	streamSvc  *stream.Service
	jwtKey     []byte
}

// New creates a new listen party Service.
func New(db *store.Store, kv *redis.Client, streamSvc *stream.Service, jwtSecret string) *Service {
	return &Service{db: db, kv: kv, streamSvc: streamSvc, jwtKey: []byte(jwtSecret)}
}

// Routes registers all listen-along HTTP routes on r.
// Auth is validated inside individual handlers rather than via middleware so
// that public and protected endpoints can share a single route group.
func (s *Service) Routes(r chi.Router) {
	r.Post("/", s.createSession)       // requires JWT (validated internally)
	r.Get("/{id}", s.getSession)       // public
	r.Delete("/{id}", s.endSession)    // requires JWT (validated internally)
	r.Get("/{id}/ws", s.ws)            // host=JWT guest=open
	r.Get("/{id}/stream/{track_id}", s.guestStream) // guest token
	r.Get("/{id}/cover/{album_id}", s.guestCover)   // guest token
}

// --- REST handlers ---

func (s *Service) createSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "user not found")
		return
	}

	sessionID := uuid.New().String()
	sess := Session{
		ID:        sessionID,
		HostID:    userID,
		HostName:  user.Username,
		CreatedAt: time.Now(),
		State:     PlaybackState{},
	}
	b, _ := json.Marshal(sess)
	if err := s.kv.Set(r.Context(), kvkeys.ListenSession(sessionID), b, sessionTTL).Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create session")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"session_id": sessionID})
}

func (s *Service) getSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "session not found")
		return
	}

	participantCount := 0
	if h := getHub(sessionID); h != nil {
		participantCount = len(h.participants())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":        sess.ID,
		"host_name":         sess.HostName,
		"participant_count": participantCount,
		"created_at":        sess.CreatedAt,
	})
}

func (s *Service) endSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.HostID != userID {
		writeErr(w, http.StatusForbidden, "not the session host")
		return
	}
	s.kv.Del(r.Context(), kvkeys.ListenSession(sessionID))
	removeHub(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// --- WebSocket handler ---

func (s *Service) ws(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Determine if this is the host by validating a JWT.
	isHost := false
	hostUserID := ""
	tokenStr := tokenFromRequest(r)
	if tokenStr != "" {
		if uid, ok := s.validateJWT(tokenStr); ok && uid == sess.HostID {
			isHost = true
			hostUserID = uid
		}
	}
	_ = hostUserID

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	h := getOrCreateHub(sessionID)

	if isHost {
		s.runHost(conn, h, sess)
	} else {
		s.runGuest(conn, h, sess)
	}
}

func (s *Service) runHost(conn *websocket.Conn, h *hub, sess *Session) {
	c := &client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 64),
		id:     sess.HostID,
		isHost: true,
	}
	h.register <- c

	// Confirm to host.
	joined := mustMarshal(outMsg{
		Type:      "joined",
		Role:      "host",
		SessionID: sess.ID,
	})
	c.send <- joined

	// Also send current participant list.
	s.sendParticipantList(c, h)

	go c.writePump()
	c.readPumpHost(s, h, sess)
}

func (s *Service) runGuest(conn *websocket.Conn, h *hub, sess *Session) {
	// Guests must send a join message first.
	conn.SetReadLimit(maxMsgSize)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	_, raw, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}
	var msg inMsg
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != "join" {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "expected join"))
		conn.Close()
		return
	}
	nickname := strings.TrimSpace(msg.Nickname)
	if nickname == "" || len(nickname) > 32 {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "invalid nickname"))
		conn.Close()
		return
	}

	participantID := uuid.New().String()
	guestToken := uuid.New().String()
	s.kv.Set(context.Background(), kvkeys.ListenGuestToken(guestToken), sess.ID, guestTokenTTL)

	c := &client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 64),
		id:       participantID,
		nickname: nickname,
		isHost:   false,
	}
	h.register <- c

	// Send joined confirmation with current playback state and track info.
	currentState := sess.State
	ti := s.fetchTrackInfo(currentState.TrackID)
	joined := mustMarshal(outMsg{
		Type:          "joined",
		Role:          "guest",
		SessionID:     sess.ID,
		ParticipantID: participantID,
		GuestToken:    guestToken,
		CurrentState:  &currentState,
		TrackInfo:     ti,
	})
	c.send <- joined

	// Send participant list to new guest.
	s.sendParticipantList(c, h)

	// Notify host and other guests.
	p := Participant{ID: participantID, Nickname: nickname, JoinedAt: time.Now()}
	joinedNotif := mustMarshal(outMsg{Type: "participant_joined", Participant: &p})
	h.sendToHost(joinedNotif)
	h.broadcast <- joinedNotif

	go c.writePump()

	// Read loop (guests only send ping).
	c.readPumpGuest()

	// On disconnect: notify host + others.
	h.unregister <- c
	s.kv.Del(context.Background(), kvkeys.ListenGuestToken(guestToken))

	leftNotif := mustMarshal(outMsg{Type: "participant_left", Participant: &p})
	h.sendToHost(leftNotif)
	h.broadcast <- leftNotif
}

// sendParticipantList sends the current participant list to c.
func (s *Service) sendParticipantList(c *client, h *hub) {
	list := h.participants()
	msg := mustMarshal(outMsg{Type: "participants", Participants: list})
	select {
	case c.send <- msg:
	default:
	}
}

// --- Client read pumps ---

func (c *client) readPumpHost(s *Service, h *hub, sess *Session) {
	defer func() {
		h.unregister <- c
		removeHub(sess.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.kv.Del(ctx, kvkeys.ListenSession(sess.ID))
	}()

	c.conn.SetReadLimit(maxMsgSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Cache track info so we only hit the DB when the track changes.
	var cachedTrackID string
	var cachedTrackInfo *TrackInfo

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg inMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "sync_state":
			if msg.State == nil {
				continue
			}
			st := *msg.State
			st.ServerTimeMs = time.Now().UnixMilli()
			sess.State = st

			// Update Redis with a timeout so a slow Redis never blocks the loop.
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if b, err := json.Marshal(sess); err == nil {
					s.kv.Set(ctx, kvkeys.ListenSession(sess.ID), b, sessionTTL)
				}
			}()

			// Only fetch track info from the DB when the track actually changes.
			var ti *TrackInfo
			if st.TrackID != cachedTrackID {
				ti = s.fetchTrackInfoWithTimeout(st.TrackID)
				cachedTrackID = st.TrackID
				cachedTrackInfo = ti
			} else {
				ti = cachedTrackInfo
			}

			// Broadcast to guests.
			out := mustMarshal(outMsg{Type: "sync", State: &st, TrackInfo: ti})
			select {
			case h.broadcast <- out:
			default:
			}

		case "kick":
			if msg.ParticipantID == "" {
				continue
			}
			h.kickGuest(msg.ParticipantID)
			// Notify host of removal.
			leftNotif := mustMarshal(outMsg{
				Type:      "participant_left",
				Participant: &Participant{ID: msg.ParticipantID},
			})
			h.sendToHost(leftNotif)

		case "ping":
			_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		}
	}
}

func (c *client) readPumpGuest() {
	c.conn.SetReadLimit(maxMsgSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg inMsg
		if json.Unmarshal(raw, &msg) == nil && msg.Type == "ping" {
			_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		}
	}
}

// --- Client write pump ---

func (c *client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// --- Guest stream handler ---

func (s *Service) guestStream(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	trackID := chi.URLParam(r, "track_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		http.Error(w, "missing guest_token", http.StatusUnauthorized)
		return
	}

	// Validate token → session.
	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		http.Error(w, "invalid or expired guest token", http.StatusUnauthorized)
		return
	}

	// Verify session still exists.
	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	s.streamSvc.ServeByTrackID(w, r, trackID)
}

// --- Guest cover handler ---

func (s *Service) guestCover(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	albumID := chi.URLParam(r, "album_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		http.Error(w, "missing guest_token", http.StatusUnauthorized)
		return
	}

	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		http.Error(w, "invalid or expired guest token", http.StatusUnauthorized)
		return
	}

	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	s.streamSvc.ServeCover(w, r, albumID)
}

// --- Helpers ---

// fetchTrackInfoWithTimeout looks up minimal track metadata with a timeout so
// a slow or exhausted DB pool never blocks the WebSocket read loop.
func (s *Service) fetchTrackInfoWithTimeout(trackID string) *TrackInfo {
	if trackID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	track, err := s.db.GetTrackByID(ctx, trackID)
	if err != nil {
		return nil
	}
	ti := &TrackInfo{
		ID:         track.ID,
		Title:      track.Title,
		SampleRate: int32(track.SampleRate),
		DurationMs: int32(track.DurationMs),
	}
	if track.ArtistID != nil {
		ti.ArtistName = track.Title // fallback; try artist name below
	}
	if track.AlbumID != nil {
		ti.AlbumID = *track.AlbumID
	}
	if track.BitDepth != nil {
		ti.BitDepth = int32(*track.BitDepth)
	}
	// Best-effort artist name resolution.
	if track.ArtistID != nil {
		if artist, err := s.db.GetArtistByID(ctx, *track.ArtistID); err == nil {
			ti.ArtistName = artist.Name
		}
	}
	return ti
}

// fetchTrackInfo is the non-timeout variant used during guest join (where the
// HTTP request context is still alive).
func (s *Service) fetchTrackInfo(trackID string) *TrackInfo {
	return s.fetchTrackInfoWithTimeout(trackID)
}

// requireAuth extracts and validates the JWT from the request. On failure it
// writes the appropriate error response and returns ("", false).
func (s *Service) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	tokenStr := tokenFromRequest(r)
	if tokenStr == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return "", false
	}
	userID, ok := s.validateJWT(tokenStr)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return "", false
	}
	return userID, true
}

func (s *Service) loadSession(ctx context.Context, sessionID string) (*Session, error) {
	raw, err := s.kv.Get(ctx, kvkeys.ListenSession(sessionID)).Result()
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

type jwtClaims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}

func (s *Service) validateJWT(tokenStr string) (string, bool) {
	var c jwtClaims
	tok, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtKey, nil
	})
	if err != nil || !tok.Valid {
		return "", false
	}
	return c.UserID, true
}

func tokenFromRequest(r *http.Request) string {
	if hdr := r.Header.Get("Authorization"); strings.HasPrefix(hdr, "Bearer ") {
		return strings.TrimPrefix(hdr, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
