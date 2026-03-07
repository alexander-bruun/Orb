// Package recommend provides track similarity and recommendation endpoints.
package recommend

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
)

// Service handles recommendation HTTP routes.
type Service struct {
	db *store.Store
}

// New returns a new recommend Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// Routes registers recommendation endpoints.
func (s *Service) Routes(r chi.Router) {
	r.Get("/similar/{track_id}", s.similarTracks)
	r.Get("/radio", s.radio)
	r.Get("/autoplay", s.autoplay)
}

// similarTracks returns tracks similar to the given track.
// GET /recommend/similar/{track_id}?limit=20&exclude_album=<album_id>
func (s *Service) similarTracks(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "track_id")
	limit := intQuery(r, "limit", 20)
	excludeAlbum := r.URL.Query().Get("exclude_album")

	tracks, err := s.db.ListSimilarTracks(r.Context(), trackID, limit, excludeAlbum)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tracks)
}

// radio returns a personalized mix based on recent listening history.
// GET /recommend/radio?limit=50
func (s *Service) radio(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit := intQuery(r, "limit", 50)

	tracks, err := s.db.RecommendForUser(r.Context(), userID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tracks)
}

// autoplay returns tracks to auto-play after the given track ends.
// GET /recommend/autoplay?after={track_id}&exclude=id1,id2&limit=5
func (s *Service) autoplay(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	afterID := r.URL.Query().Get("after")
	if afterID == "" {
		http.Error(w, "missing 'after' parameter", http.StatusBadRequest)
		return
	}
	limit := intQuery(r, "limit", 5)

	var exclude []string
	if ex := r.URL.Query().Get("exclude"); ex != "" {
		exclude = strings.Split(ex, ",")
	}

	tracks, err := s.db.AutoplayAfter(r.Context(), userID, afterID, exclude, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tracks)
}

func intQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
