// Package playlist handles playlist CRUD and track management.
package playlist

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/alexander-bruun/orb/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Service handles playlist HTTP routes.
type Service struct {
	db *store.Store
}

// New returns a new playlist Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// Routes registers playlist endpoints.
func (s *Service) Routes(r chi.Router) {
	r.Get("/", s.list)
	r.Post("/", s.create)
	r.Get("/{id}", s.detail)
	r.Patch("/{id}", s.update)
	r.Delete("/{id}", s.delete)
	r.Post("/{id}/tracks", s.addTrack)
	r.Delete("/{id}/tracks/{track_id}", s.removeTrack)
	r.Put("/{id}/tracks/order", s.reorderTracks)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	pls, err := s.db.ListPlaylistsByUser(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pls)
}

type createReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	pl, err := s.db.CreatePlaylist(r.Context(), store.CreatePlaylistParams{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		CoverArtKey: "",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pl)
}

func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, err := s.db.GetPlaylistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	tracks, err := s.db.ListPlaylistTracks(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"playlist": pl, "tracks": tracks})
}

type updateReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, err := s.db.GetPlaylistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := pl.Name
	if req.Name != "" {
		name = req.Name
	}
	desc := pl.Description
	if req.Description != "" {
		desc = req.Description
	}
	err = s.db.UpdatePlaylist(r.Context(), store.UpdatePlaylistParams{
		ID:          id,
		Name:        name,
		Description: desc,
		CoverArtKey: "",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pl)
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeletePlaylist(r.Context(), store.DeletePlaylistParams{
		ID: id,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addTrackReq struct {
	TrackID string `json:"track_id"`
}

func (s *Service) addTrack(w http.ResponseWriter, r *http.Request) {
	playlistID := chi.URLParam(r, "id")
	var req addTrackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	pos, _ := s.db.GetMaxPlaylistPosition(r.Context(), playlistID)
	if err := s.db.AddTrackToPlaylist(r.Context(), store.AddTrackToPlaylistParams{
		PlaylistID: playlistID,
		TrackID:    req.TrackID,
		Position:   pos + 1,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) removeTrack(w http.ResponseWriter, r *http.Request) {
	if err := s.db.RemoveTrackFromPlaylist(r.Context(), store.RemoveTrackFromPlaylistParams{
		PlaylistID: chi.URLParam(r, "id"),
		TrackID:    chi.URLParam(r, "track_id"),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type orderReq struct {
	Order []struct {
		TrackID  string `json:"track_id"`
		Position int    `json:"position"`
	} `json:"order"`
}

func (s *Service) reorderTracks(w http.ResponseWriter, r *http.Request) {
	playlistID := chi.URLParam(r, "id")
	var req orderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	for _, item := range req.Order {
		if err := s.db.UpdatePlaylistTrackOrder(r.Context(), store.UpdatePlaylistTrackOrderParams{
			PlaylistID: playlistID,
			TrackID:    item.TrackID,
			Position:   int32(item.Position),
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

var _ = strconv.Itoa // silence unused import
