// Package recommend provides track similarity and recommendation endpoints.
package recommend

import (
	"net/http"
	"strings"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
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
	limit := httputil.QueryInt(r, "limit", 20)
	excludeAlbum := r.URL.Query().Get("exclude_album")

	tracks, err := s.db.ListSimilarTracks(r.Context(), trackID, limit, excludeAlbum)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, tracks)
}

// radio returns a personalized mix based on recent listening history.
// GET /recommend/radio?limit=50&seed_artist_id=<artist_id>
func (s *Service) radio(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit := httputil.QueryInt(r, "limit", 50)
	seedArtistID := r.URL.Query().Get("seed_artist_id")

	var (
		tracks []store.TrackWithScore
		err    error
	)
	if seedArtistID != "" {
		tracks, err = s.db.RecommendForArtist(r.Context(), seedArtistID, userID, limit)
	} else {
		tracks, err = s.db.RecommendForUser(r.Context(), userID, limit)
	}
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, tracks)
}

// autoplay returns tracks to auto-play after the given track ends.
// GET /recommend/autoplay?after={track_id}&exclude=id1,id2&limit=5
func (s *Service) autoplay(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	afterID := r.URL.Query().Get("after")
	if afterID == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "missing 'after' parameter")
		return
	}
	limit := httputil.QueryInt(r, "limit", 5)

	var exclude []string
	if ex := r.URL.Query().Get("exclude"); ex != "" {
		exclude = strings.Split(ex, ",")
	}

	tracks, err := s.db.AutoplayAfter(r.Context(), userID, afterID, exclude, limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, tracks)
}

