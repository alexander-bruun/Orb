// Package queue handles per-user playback queue management with write-through caching.
package queue

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/alexander-bruun/orb/pkg/kvkeys"
	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/alexander-bruun/orb/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const queueCacheTTL = 24 * time.Hour

// Service handles queue HTTP routes.
type Service struct {
	db *store.Store
	kv *redis.Client
}

// New returns a new queue Service.
func New(db *store.Store, kv *redis.Client) *Service {
	return &Service{db: db, kv: kv}
}

// Routes registers queue endpoints.
func (s *Service) Routes(r chi.Router) {
	r.Get("/", s.getQueue)
	r.Put("/", s.replaceQueue)
	r.Delete("/", s.clearQueue)
	r.Post("/next", s.addNext)
	r.Post("/last", s.addLast)
}

func (s *Service) getQueue(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	// Try KeyVal first.
	raw, err := s.kv.Get(r.Context(), kvkeys.UserQueue(userID)).Result()
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(raw))
		return
	}

	// Fall back to Postgres.
	tracks, err := s.db.GetQueue(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tracks == nil {
		tracks = []store.Track{}
	}
	s.cacheQueue(r, userID, tracks)
	writeJSON(w, http.StatusOK, tracks)
}

type replaceReq struct {
	TrackIDs []string `json:"track_ids"`
	Source   string   `json:"source"`
}

func (s *Service) replaceQueue(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req replaceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Write to Postgres.
	if err := s.db.ClearQueue(r.Context(), userID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i, trackID := range req.TrackIDs {
		_ = s.db.InsertQueueEntry(r.Context(), store.InsertQueueEntryParams{
			UserID:   userID,
			TrackID:  trackID,
			Position: i + 1,
			Source:   req.Source,
		})
	}
	// Invalidate KeyVal cache.
	s.kv.Del(r.Context(), kvkeys.UserQueue(userID))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) clearQueue(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	_ = s.db.ClearQueue(r.Context(), userID)
	s.kv.Del(r.Context(), kvkeys.UserQueue(userID))
	w.WriteHeader(http.StatusNoContent)
}

type addTrackReq struct {
	TrackID string `json:"track_id"`
	Source  string `json:"source"`
}

func (s *Service) addNext(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req addTrackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	minPos, _ := s.db.GetMinQueuePosition(r.Context(), userID)
	_ = s.db.InsertQueueEntry(r.Context(), store.InsertQueueEntryParams{
		UserID:   userID,
		TrackID:  req.TrackID,
		Position: minPos - 1,
		Source:   req.Source,
	})
	s.kv.Del(r.Context(), kvkeys.UserQueue(userID))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) addLast(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req addTrackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	maxPos, _ := s.db.GetMaxQueuePosition(r.Context(), userID)
	_ = s.db.InsertQueueEntry(r.Context(), store.InsertQueueEntryParams{
		UserID:   userID,
		TrackID:  req.TrackID,
		Position: maxPos + 1,
		Source:   req.Source,
	})
	s.kv.Del(r.Context(), kvkeys.UserQueue(userID))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) cacheQueue(r *http.Request, userID string, tracks []store.Track) {
	b, err := json.Marshal(tracks)
	if err != nil {
		return
	}
	s.kv.Set(r.Context(), kvkeys.UserQueue(userID), b, queueCacheTTL)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
