package listenparty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/lyricfetch"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/alexander-bruun/orb/services/internal/stream"
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
	ItemType     string  `json:"item_type"` // "track" or "audiobook"
	TrackID      string  `json:"track_id"`  // kept for compatibility
	ItemID       string  `json:"item_id"`
	ChapterID    string  `json:"chapter_id,omitempty"`
	PositionMs   float64 `json:"position_ms"`
	Playing      bool    `json:"playing"`
	ServerTimeMs int64   `json:"server_time_ms"` // unix ms when state was set
}

// Session is the data persisted in Redis for a listen-along session.
type Session struct {
	ID          string        `json:"id"`
	HostID      string        `json:"host_id"`
	HostName    string        `json:"host_name"`
	CreatedAt   time.Time     `json:"created_at"`
	State       PlaybackState `json:"state"`
	CodeEnabled bool          `json:"code_enabled"`
	AccessCode  string        `json:"access_code,omitempty"`
}

// Participant represents a guest connected to a session.
type Participant struct {
	ID       string    `json:"id"`
	Nickname string    `json:"nickname"`
	JoinedAt time.Time `json:"joined_at"`
}

// TrackInfo carries just enough track metadata for the guest player to display
// and stream a track without needing JWT-authenticated library API calls.
type TrackInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	ArtistName string `json:"artist_name"`
	AlbumID    string `json:"album_id"`
	BitDepth   int32  `json:"bit_depth"`
	SampleRate int32  `json:"sample_rate"`
	DurationMs int32  `json:"duration_ms"`
}

type AudiobookInfo struct {
	ID         string                   `json:"id"`
	Title      string                   `json:"title"`
	AuthorName string                   `json:"author_name"`
	DurationMs int64                    `json:"duration_ms"`
	Chapters   []store.AudiobookChapter `json:"chapters,omitempty"`
}

// --- WebSocket message types ---

type inMsg struct {
	Type          string         `json:"type"`
	Nickname      string         `json:"nickname,omitempty"`
	Code          string         `json:"code,omitempty"`
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
	AudiobookInfo *AudiobookInfo `json:"audiobook_info,omitempty"`
	Participants  []Participant  `json:"participants,omitempty"`
	Participant   *Participant   `json:"participant,omitempty"`
	Message       string         `json:"message,omitempty"`
	CodeEnabled   bool           `json:"code_enabled,omitempty"`
	AccessCode    string         `json:"access_code,omitempty"`
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
	time.AfterFunc(500*time.Millisecond, func() { closeWebsocket(g.conn) })
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
	db        *store.Store
	kv        *redis.Client
	streamSvc *stream.Service
	jwtKey    []byte
}

// New creates a new listen party Service.
func New(db *store.Store, kv *redis.Client, streamSvc *stream.Service, jwtSecret string) *Service {
	return &Service{db: db, kv: kv, streamSvc: streamSvc, jwtKey: []byte(jwtSecret)}
}

// Routes registers all listen-along HTTP routes on r.
// Auth is validated inside individual handlers rather than via middleware so
// that public and protected endpoints can share a single route group.
func (s *Service) Routes(r chi.Router) {
	r.Post("/", s.createSession)                    // requires JWT (validated internally)
	r.Get("/{id}", s.getSession)                    // public
	r.Delete("/{id}", s.endSession)                 // requires JWT (validated internally)
	r.Post("/{id}/code", s.enableCode)              // requires JWT (host); enables/regenerates access code
	r.Delete("/{id}/code", s.disableCode)           // requires JWT (host); disables access code
	r.Get("/{id}/ws", s.ws)                         // host=JWT guest=open
	r.Get("/{id}/stream/{track_id}", s.guestStream) // guest token
	r.Get("/{id}/stream/audiobook/{audiobook_id}", s.guestAudiobookStream)
	r.Get("/{id}/stream/audiobook/chapter/{chapter_id}", s.guestAudiobookChapterStream)
	r.Get("/{id}/cover/{album_id}", s.guestCover)                        // guest token
	r.Get("/{id}/cover/audiobook/{audiobook_id}", s.guestAudiobookCover) // guest token
	r.Get("/{id}/lyrics/{track_id}", s.guestLyrics)                      // guest token
}

// --- REST handlers ---

func (s *Service) createSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "user not found")
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
		httputil.WriteErr(w, http.StatusInternalServerError, "could not create session")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"session_id": sessionID})
}

func (s *Service) getSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}

	participantCount := 0
	if h := getHub(sessionID); h != nil {
		participantCount = len(h.participants())
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"session_id":        sess.ID,
		"host_name":         sess.HostName,
		"participant_count": participantCount,
		"created_at":        sess.CreatedAt,
		"code_enabled":      sess.CodeEnabled,
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
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.HostID != userID {
		httputil.WriteErr(w, http.StatusForbidden, "not the session host")
		return
	}
	s.kv.Del(r.Context(), kvkeys.ListenSession(sessionID))
	removeHub(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) enableCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.HostID != userID {
		httputil.WriteErr(w, http.StatusForbidden, "not the session host")
		return
	}
	code := fmt.Sprintf("%04d", rand.Intn(10000))
	sess.CodeEnabled = true
	sess.AccessCode = code
	b, _ := json.Marshal(sess)
	if err := s.kv.Set(r.Context(), kvkeys.ListenSession(sessionID), b, sessionTTL).Err(); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "could not update session")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (s *Service) disableCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.HostID != userID {
		httputil.WriteErr(w, http.StatusForbidden, "not the session host")
		return
	}
	sess.CodeEnabled = false
	sess.AccessCode = ""
	b, _ := json.Marshal(sess)
	if err := s.kv.Set(r.Context(), kvkeys.ListenSession(sessionID), b, sessionTTL).Err(); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "could not update session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- WebSocket handler ---

func (s *Service) ws(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	sess, err := s.loadSession(r.Context(), sessionID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
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
		Type:        "joined",
		Role:        "host",
		SessionID:   sess.ID,
		CodeEnabled: sess.CodeEnabled,
		AccessCode:  sess.AccessCode,
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
		closeWebsocket(conn)
		return
	}
	var msg inMsg
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != "join" {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "expected join"))
		closeWebsocket(conn)
		return
	}
	nickname := strings.TrimSpace(msg.Nickname)
	if nickname == "" || len(nickname) > 32 {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "invalid nickname"))
		closeWebsocket(conn)
		return
	}

	// Reload the session from Redis so that code_enabled changes made after the
	// WebSocket was upgraded (but before the join message arrived) are honoured.
	if fresh, err := s.loadSession(context.Background(), sess.ID); err == nil {
		sess = fresh
	}

	// If the session requires an access code, validate it before proceeding.
	// Note: we check CodeEnabled alone (not also AccessCode != "") so that a
	// session where CodeEnabled=true but AccessCode is somehow unset still blocks
	// guests rather than silently admitting them.
	if sess.CodeEnabled {
		if sess.AccessCode == "" || strings.TrimSpace(msg.Code) != sess.AccessCode {
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid access code"))
			closeWebsocket(conn)
			return
		}
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

	// Send joined confirmation with current playback state and metadata.
	currentState := sess.State
	ti, ai := s.fetchMetadata(currentState)
	joined := mustMarshal(outMsg{
		Type:          "joined",
		Role:          "guest",
		SessionID:     sess.ID,
		ParticipantID: participantID,
		GuestToken:    guestToken,
		CurrentState:  &currentState,
		TrackInfo:     ti,
		AudiobookInfo: ai,
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

	// Cache metadata info so we only hit the DB when the track/book changes.
	var cachedItemID string
	var cachedTrackInfo *TrackInfo
	var cachedAudiobookInfo *AudiobookInfo

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

			// Fill ItemID if missing but TrackID is present (compat)
			if st.ItemID == "" && st.TrackID != "" {
				st.ItemID = st.TrackID
				st.ItemType = "track"
			}

			sess.State = st

			// Update Redis with a timeout so a slow Redis never blocks the loop.
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if b, err := json.Marshal(sess); err == nil {
					s.kv.Set(ctx, kvkeys.ListenSession(sess.ID), b, sessionTTL)
				}
			}()

			// Only fetch metadata info from the DB when the item actually changes.
			var ti *TrackInfo
			var ai *AudiobookInfo
			if st.ItemID != cachedItemID {
				ti, ai = s.fetchMetadataWithTimeout(st)
				cachedItemID = st.ItemID
				cachedTrackInfo = ti
				cachedAudiobookInfo = ai
			} else {
				ti = cachedTrackInfo
				ai = cachedAudiobookInfo
			}

			// Broadcast to guests.
			out := mustMarshal(outMsg{Type: "sync", State: &st, TrackInfo: ti, AudiobookInfo: ai})
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
				Type:        "participant_left",
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

func closeWebsocket(conn *websocket.Conn) {
	if err := conn.Close(); err != nil {
		slog.Warn("listenparty: websocket close failed", "err", err)
	}
}

// --- Client write pump ---

func (c *client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		closeWebsocket(c.conn)
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
		httputil.WriteErr(w, http.StatusUnauthorized, "missing guest_token")
		return
	}

	// Validate token → session.
	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid or expired guest token")
		return
	}

	// Verify session still exists.
	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}

	s.streamSvc.ServeByTrackID(w, r, trackID)
}

func (s *Service) guestAudiobookStream(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	audiobookID := chi.URLParam(r, "audiobook_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "missing guest_token")
		return
	}
	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid or expired guest token")
		return
	}
	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}
	s.streamSvc.ServeByAudiobookID(w, r, audiobookID)
}

func (s *Service) guestAudiobookChapterStream(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	chapterID := chi.URLParam(r, "chapter_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "missing guest_token")
		return
	}
	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid or expired guest token")
		return
	}
	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}
	s.streamSvc.ServeByAudiobookChapterID(w, r, chapterID)
}

// --- Guest cover handler ---

func (s *Service) guestCover(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	albumID := chi.URLParam(r, "album_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "missing guest_token")
		return
	}

	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid or expired guest token")
		return
	}

	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}

	s.streamSvc.ServeCover(w, r, albumID)
}

func (s *Service) guestAudiobookCover(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	audiobookID := chi.URLParam(r, "audiobook_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "missing guest_token")
		return
	}
	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid or expired guest token")
		return
	}
	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}
	s.streamSvc.ServeAudiobookCover(w, r, audiobookID)
}

// --- Guest lyrics handler ---

type lrcLine struct {
	TimeMs int    `json:"time_ms"`
	Text   string `json:"text"`
}

var lrcLineRe = regexp.MustCompile(`\[(\d{2}):(\d{2})\.(\d{2,3})\](.*)`)

func parseLRC(raw string) []lrcLine {
	var lines []lrcLine
	for _, line := range strings.Split(raw, "\n") {
		m := lrcLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		ms, _ := strconv.Atoi(m[3])
		if len(m[3]) == 2 {
			ms *= 10
		}
		text := strings.TrimSpace(m[4])
		if text == "" {
			continue
		}
		lines = append(lines, lrcLine{TimeMs: (min*60+sec)*1000 + ms, Text: text})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].TimeMs < lines[j].TimeMs })
	if lines == nil {
		lines = []lrcLine{}
	}
	return lines
}

func (s *Service) guestLyrics(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	trackID := chi.URLParam(r, "track_id")
	guestToken := r.URL.Query().Get("guest_token")
	if guestToken == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "missing guest_token")
		return
	}

	storedSessionID, err := s.kv.Get(r.Context(), kvkeys.ListenGuestToken(guestToken)).Result()
	if err != nil || storedSessionID != sessionID {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid or expired guest token")
		return
	}

	if _, err := s.loadSession(r.Context(), sessionID); err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "session not found")
		return
	}

	// Check DB cache first.
	raw, err := s.db.GetTrackLyrics(r.Context(), trackID)
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, []lrcLine{})
		return
	}

	if raw == "" {
		// Auto-fetch from external providers.
		track, err := s.db.GetTrackByID(r.Context(), trackID)
		if err != nil {
			httputil.WriteJSON(w, http.StatusOK, []lrcLine{})
			return
		}
		artistName := ""
		if track.ArtistID != nil {
			if a, aErr := s.db.GetArtistByID(r.Context(), *track.ArtistID); aErr == nil {
				artistName = a.Name
			}
		}
		albumTitle := ""
		if track.AlbumID != nil {
			if al, alErr := s.db.GetAlbumByID(r.Context(), *track.AlbumID); alErr == nil {
				albumTitle = al.Title
			}
		}

		res, fetchErr := lyricfetch.Search(r.Context(), artistName, albumTitle, track.Title, track.DurationMs)
		if fetchErr != nil || res == nil {
			httputil.WriteJSON(w, http.StatusOK, []lrcLine{})
			return
		}
		raw = res.LRC
		if raw == "" {
			raw = res.Plain
		}
		if raw != "" {
			if err := s.db.SetTrackLyrics(r.Context(), trackID, raw); err != nil {
				slog.Warn("set track lyrics failed", "action", "set track lyrics", "err", err)
			}
		}
	}

	httputil.WriteJSON(w, http.StatusOK, parseLRC(raw))
}

// --- Helpers ---

func (s *Service) fetchMetadata(st PlaybackState) (*TrackInfo, *AudiobookInfo) {
	return s.fetchMetadataWithContext(context.Background(), st)
}

func (s *Service) fetchMetadataWithTimeout(st PlaybackState) (*TrackInfo, *AudiobookInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.fetchMetadataWithContext(ctx, st)
}

func (s *Service) fetchMetadataWithContext(ctx context.Context, st PlaybackState) (*TrackInfo, *AudiobookInfo) {
	if st.ItemID == "" && st.TrackID == "" {
		return nil, nil
	}

	if st.ItemType == "audiobook" {
		book, err := s.db.GetAudiobook(ctx, st.ItemID)
		if err != nil {
			return nil, nil
		}
		ai := &AudiobookInfo{
			ID:         book.ID,
			Title:      book.Title,
			DurationMs: book.DurationMs,
			Chapters:   book.Chapters,
		}
		if book.AuthorName != nil {
			ai.AuthorName = *book.AuthorName
		}
		return nil, ai
	}

	// Default to track
	trackID := st.ItemID
	if trackID == "" {
		trackID = st.TrackID
	}
	track, err := s.db.GetTrackByID(ctx, trackID)
	if err != nil {
		return nil, nil
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
	return ti, nil
}

// requireAuth extracts and validates the JWT from the request. On failure it
// writes the appropriate error response and returns ("", false).
func (s *Service) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	tokenStr := tokenFromRequest(r)
	if tokenStr == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "missing token")
		return "", false
	}
	userID, ok := s.validateJWT(tokenStr)
	if !ok {
		httputil.WriteErr(w, http.StatusUnauthorized, "invalid token")
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
