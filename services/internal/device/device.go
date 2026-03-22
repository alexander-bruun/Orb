// Package device implements multi-device playback session management.
//
// When a user enables "Exclusive Device Mode" (one device plays at a time),
// each browser tab / native app registers itself as a named device. Devices
// heartbeat every 30 s; entries expire after 90 s of silence. The active
// device gets exclusive control: when it starts playing it publishes a
// "pause_others" event that all peer SSE connections relay to their clients.
//
// All state is kept in Valkey/Redis – no database writes are required.
package device

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const queueCacheTTL = 24 * time.Hour

const (
	deviceTTL        = 90 * time.Second
	exclusiveModeTTL = 0 // no expiry – persist until user changes it
)

// DeviceState is the playback snapshot sent with every heartbeat.
type DeviceState struct {
	TrackID    string  `json:"track_id,omitempty"`
	TrackTitle string  `json:"track_title,omitempty"`
	AlbumID    string  `json:"album_id,omitempty"`
	PositionMs float64 `json:"position_ms"`
	Playing    bool    `json:"playing"`
	Volume     float64 `json:"volume"` // 0.0–1.0; owned by the active device
	// PlaybackEpochMs is the Unix millisecond timestamp at which the track
	// position would have been 0. Computed server-side as:
	//   now_ms - position_ms
	// Any client can derive the current position as: Date.now() - PlaybackEpochMs.
	PlaybackEpochMs int64 `json:"playback_epoch_ms,omitempty"`
	// Audiobook fields — set when IsAudiobook=true; music fields are omitted.
	IsAudiobook    bool   `json:"is_audiobook,omitempty"`
	AudiobookID    string `json:"audiobook_id,omitempty"`
	AudiobookTitle string `json:"audiobook_title,omitempty"`
}

// Device represents a single registered client session.
type Device struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	State    DeviceState `json:"state"`
	LastSeen time.Time   `json:"last_seen"`
	IsActive bool        `json:"is_active"`
}

// eventMsg is the payload sent over a pub/sub channel and relayed via SSE.
type eventMsg struct {
	Type     string       `json:"type"` // "state" | "pause_others" | "registered" | "unregistered" | "play_command" | "control_command" | "exclusive_mode"
	DeviceID string       `json:"device_id,omitempty"`
	State    *DeviceState `json:"state,omitempty"`
	// For "exclusive_mode" events
	Enabled bool `json:"enabled,omitempty"`
	// For "play_command" events
	TrackID    string          `json:"track_id,omitempty"`
	PositionMs float64         `json:"position_ms,omitempty"`
	Queue      json.RawMessage `json:"queue,omitempty"` // Track[] embedded in play_command
	// For "control_command" events
	Action string  `json:"action,omitempty"` // "toggle" | "next" | "previous" | "seek" | "volume"
	Volume float64 `json:"volume,omitempty"` // 0.0–1.0; for "volume" action
}

// Service handles device HTTP routes.
type Service struct {
	kv *redis.Client
}

// New returns a new device Service.
func New(kv *redis.Client) *Service {
	return &Service{kv: kv}
}

// Routes registers device endpoints; all require JWT middleware upstream.
func (s *Service) Routes(r chi.Router) {
	r.Get("/devices", s.list)
	r.Post("/devices", s.register)
	r.Post("/devices/{id}/heartbeat", s.heartbeat)
	r.Delete("/devices/{id}", s.unregister)
	r.Post("/devices/{id}/activate", s.activate)
	r.Post("/devices/{id}/play", s.playCommand)
	r.Post("/devices/{id}/control", s.controlCommand)
	r.Get("/devices/events", s.events)

	// Exclusive-mode preference.
	r.Get("/playback-settings", s.getPlaybackSettings)
	r.Patch("/playback-settings", s.patchPlaybackSettings)
}

// ── list ─────────────────────────────────────────────────────────────────────

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	devices, err := s.scanDevices(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "redis error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, devices)
}

// ── register ─────────────────────────────────────────────────────────────────

type registerReq struct {
	DeviceID string `json:"device_id"` // client-generated UUID
	Name     string `json:"name"`
}

func (s *Service) register(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" || req.Name == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "device_id and name are required")
		return
	}

	d := Device{
		ID:       req.DeviceID,
		Name:     req.Name,
		LastSeen: time.Now().UTC(),
	}
	if err := s.saveDevice(r.Context(), userID, d); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "redis error")
		return
	}

	if err := s.publish(r.Context(), userID, eventMsg{Type: "registered", DeviceID: d.ID}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}
	httputil.WriteJSON(w, http.StatusOK, d)
}

// ── heartbeat ────────────────────────────────────────────────────────────────

func (s *Service) heartbeat(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	deviceID := chi.URLParam(r, "id")

	var state DeviceState
	_ = json.NewDecoder(r.Body).Decode(&state)

	// Compute a server-side epoch so any client can derive the current position
	// as Date.now() - playback_epoch_ms, eliminating drift from timer intervals.
	if state.Playing {
		nowMs := time.Now().UnixMilli()
		state.PlaybackEpochMs = nowMs - int64(state.PositionMs)
	}

	d, err := s.loadDevice(r.Context(), userID, deviceID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "device not found")
		return
	}
	d.State = state
	d.LastSeen = time.Now().UTC()
	if err := s.saveDevice(r.Context(), userID, d); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "redis error")
		return
	}

	// Notify peers of the updated state.
	if err := s.publish(r.Context(), userID, eventMsg{Type: "state", DeviceID: deviceID, State: &state}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── unregister ───────────────────────────────────────────────────────────────

func (s *Service) unregister(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	deviceID := chi.URLParam(r, "id")

	s.kv.Del(r.Context(), kvkeys.UserDevice(userID, deviceID))

	// Clear active device pointer if it was this device.
	active, _ := s.kv.Get(r.Context(), kvkeys.UserActiveDevice(userID)).Result()
	if active == deviceID {
		s.kv.Del(r.Context(), kvkeys.UserActiveDevice(userID))
	}

	if err := s.publish(r.Context(), userID, eventMsg{Type: "unregistered", DeviceID: deviceID}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── activate (exclusive-mode takeover) ───────────────────────────────────────

func (s *Service) activate(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	deviceID := chi.URLParam(r, "id")

	// Make this device the active one.
	s.kv.Set(r.Context(), kvkeys.UserActiveDevice(userID), deviceID, 0)

	// Tell all other devices to pause via pub/sub.
	if err := s.publish(r.Context(), userID, eventMsg{Type: "pause_others", DeviceID: deviceID}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── play command (remote control) ────────────────────────────────────────────

type playCommandReq struct {
	TrackID    string          `json:"track_id"`
	PositionMs float64         `json:"position_ms"`
	Queue      json.RawMessage `json:"queue,omitempty"` // Track[] from client
}

func (s *Service) playCommand(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	deviceID := chi.URLParam(r, "id")

	var req playCommandReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Atomic queue write: store the queue in the Redis cache so it is
	// immediately available via GET /queue, then embed it in the SSE event
	// so the target device loads it directly from the payload.
	if len(req.Queue) > 0 && string(req.Queue) != "null" {
		s.kv.Set(r.Context(), kvkeys.UserQueue(userID), req.Queue, queueCacheTTL)
	}

	// Promote target to active device.
	s.kv.Set(r.Context(), kvkeys.UserActiveDevice(userID), deviceID, 0)

	// Tell all other devices to pause first, then send the play command.
	// Without pause_others, peers that still hold a stale active-device
	// pointer may mis-delegate playTrack() back to the originator.
	if err := s.publish(r.Context(), userID, eventMsg{Type: "pause_others", DeviceID: deviceID}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}
	if err := s.publish(r.Context(), userID, eventMsg{
		Type:       "play_command",
		DeviceID:   deviceID,
		TrackID:    req.TrackID,
		PositionMs: req.PositionMs,
		Queue:      req.Queue,
	}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── control command (remote-control: toggle/next/previous) ──────────────────

type controlCommandReq struct {
	Action     string  `json:"action"`                // "toggle" | "next" | "previous" | "seek" | "volume"
	PositionMs float64 `json:"position_ms,omitempty"` // for "seek"
	Volume     float64 `json:"volume,omitempty"`      // for "volume"; 0.0–1.0
}

func (s *Service) controlCommand(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	deviceID := chi.URLParam(r, "id")

	var req controlCommandReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Action == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "action is required")
		return
	}

	if err := s.publish(r.Context(), userID, eventMsg{
		Type:       "control_command",
		DeviceID:   deviceID,
		Action:     req.Action,
		PositionMs: req.PositionMs,
		Volume:     req.Volume,
	}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── SSE events ───────────────────────────────────────────────────────────────

func (s *Service) events(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteErr(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send an initial heartbeat/comment to confirm the connection.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Subscribe to the user's device event channel.
	sub := s.kv.Subscribe(r.Context(), kvkeys.UserDeviceEvents(userID))
	defer sub.Close()

	ch := sub.Channel()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Keep-alive comment.
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flusher.Flush()
		}
	}
}

// ── playback settings (exclusive mode) ───────────────────────────────────────

type playbackSettings struct {
	ExclusiveMode bool `json:"exclusive_mode"`
}

func (s *Service) getPlaybackSettings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	val, _ := s.kv.Get(r.Context(), kvkeys.UserExclusiveMode(userID)).Result()
	httputil.WriteJSON(w, http.StatusOK, playbackSettings{ExclusiveMode: val == "1"})
}

func (s *Service) patchPlaybackSettings(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	var req playbackSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	val := "0"
	if req.ExclusiveMode {
		val = "1"
	}
	s.kv.Set(r.Context(), kvkeys.UserExclusiveMode(userID), val, 0)

	// Notify all SSE clients of the mode change.
	if err := s.publish(r.Context(), userID, eventMsg{Type: "exclusive_mode", Enabled: req.ExclusiveMode}); err != nil {
		slog.Warn("publish device event failed", "action", "publish device event", "err", err)
	}

	httputil.WriteJSON(w, http.StatusOK, req)
}

// ── internal helpers ─────────────────────────────────────────────────────────

func (s *Service) saveDevice(ctx context.Context, userID string, d Device) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return s.kv.Set(ctx, kvkeys.UserDevice(userID, d.ID), b, deviceTTL).Err()
}

func (s *Service) loadDevice(ctx context.Context, userID, deviceID string) (Device, error) {
	raw, err := s.kv.Get(ctx, kvkeys.UserDevice(userID, deviceID)).Result()
	if err != nil {
		return Device{}, err
	}
	var d Device
	return d, json.Unmarshal([]byte(raw), &d)
}

func (s *Service) scanDevices(ctx context.Context, userID string) ([]Device, error) {
	keys, err := s.kv.Keys(ctx, kvkeys.UserDeviceGlob(userID)).Result()
	if err != nil {
		return nil, err
	}

	activeID, _ := s.kv.Get(ctx, kvkeys.UserActiveDevice(userID)).Result()

	devices := make([]Device, 0, len(keys))
	for _, k := range keys {
		raw, err := s.kv.Get(ctx, k).Result()
		if err != nil {
			continue
		}
		var d Device
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			continue
		}
		d.IsActive = (d.ID == activeID)
		devices = append(devices, d)
	}
	return devices, nil
}

func (s *Service) publish(ctx context.Context, userID string, msg eventMsg) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return s.kv.Publish(ctx, kvkeys.UserDeviceEvents(userID), b).Err()
}

