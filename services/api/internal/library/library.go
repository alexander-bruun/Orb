// Package library handles browsing tracks, albums, artists, search, and recently played.
package library

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/alexander-bruun/orb/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
)

// Service handles library HTTP routes.
type Service struct {
	db *store.Store
}

// New returns a new library Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// Routes registers library endpoints.
func (s *Service) Routes(r chi.Router) {
	r.Get("/tracks", s.listTracks)
	r.Get("/albums", s.listAlbums)
	r.Get("/artists", s.listArtists)
	r.Get("/albums/{id}", s.albumDetail)
	r.Get("/artists/{id}", s.artistDetail)
	r.Get("/tracks/{id}", s.trackDetail)
	r.Post("/tracks/{id}", s.addTrack)
	r.Delete("/tracks/{id}", s.removeTrack)
	r.Get("/search", s.search)
	r.Get("/recently-played", s.recentlyPlayed)
}

func (s *Service) listTracks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit, offset := pagination(r)
	tracks, err := s.db.ListTracksByUser(r.Context(), store.ListTracksByUserParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Service) listAlbums(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	sortBy := r.URL.Query().Get("sort_by")
	switch sortBy {
	case "artist", "year":
		// valid
	default:
		sortBy = "title"
	}
	albums, err := s.db.ListAlbums(r.Context(), store.ListAlbumsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
		SortBy: sortBy,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := s.db.CountAlbums(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": albums, "total": total})
}

func (s *Service) listArtists(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	artists, err := s.db.ListArtists(r.Context(), store.ListArtistsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, artists)
}

func (s *Service) albumDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	album, err := s.db.GetAlbumByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "album not found")
		return
	}
	tracks, err := s.db.ListTracksByAlbum(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"album": album, "tracks": tracks}
	if album.ArtistID != nil {
		if artist, err := s.db.GetArtistByID(r.Context(), *album.ArtistID); err == nil {
			resp["artist"] = artist
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) artistDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	artist, err := s.db.GetArtistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "artist not found")
		return
	}
	albums, err := s.db.ListAlbumsByArtist(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artist": artist, "albums": albums})
}

func (s *Service) trackDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := s.db.GetTrackByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *Service) addTrack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	trackID := chi.URLParam(r, "id")
	if err := s.db.AddTrackToLibrary(r.Context(), store.AddTrackToLibraryParams{
		UserID:  userID,
		TrackID: trackID,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) removeTrack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	trackID := chi.URLParam(r, "id")
	if err := s.db.RemoveTrackFromLibrary(r.Context(), userID, trackID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "q is required")
		return
	}
	tracks, _ := s.db.SearchTracks(r.Context(), store.SearchTracksParams{
		ToTsquery: q,
		Limit:     20,
	})
	albums, _ := s.db.SearchAlbums(r.Context(), store.SearchAlbumsParams{
		ToTsquery: q,
		Limit:     20,
	})
	artists, _ := s.db.SearchArtists(r.Context(), store.SearchArtistsParams{
		ToTsquery: q,
		Limit:     20,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"tracks":  tracks,
		"albums":  albums,
		"artists": artists,
	})
}

func (s *Service) recentlyPlayed(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	rows, err := s.db.ListRecentlyPlayed(r.Context(), store.ListRecentlyPlayedParams{
		UserID: userID,
		Limit:  20,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// --- helpers ---

func pagination(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
