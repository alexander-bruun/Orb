// Package playlist handles playlist CRUD and track management.
package playlist

import (
	"encoding/json"
	"net/http"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/store"
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
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pls)
}

type createReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "name is required")
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
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, pl)
}

func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, err := s.db.GetPlaylistByID(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	tracks, err := s.db.ListPlaylistTracks(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"playlist": pl, "tracks": tracks})
}

type updateReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    *bool  `json:"is_public"`
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, err := s.db.GetPlaylistByID(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
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
	isPublic := pl.IsPublic
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	err = s.db.UpdatePlaylist(r.Context(), store.UpdatePlaylistParams{
		ID:          id,
		Name:        name,
		Description: desc,
		CoverArtKey: "",
		IsPublic:    isPublic,
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pl.IsPublic = isPublic
	httputil.WriteJSON(w, http.StatusOK, pl)
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeletePlaylist(r.Context(), store.DeletePlaylistParams{
		ID: id,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
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
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	pos, _ := s.db.GetMaxPlaylistPosition(r.Context(), playlistID)
	if err := s.db.AddTrackToPlaylist(r.Context(), store.AddTrackToPlaylistParams{
		PlaylistID: playlistID,
		TrackID:    req.TrackID,
		Position:   pos + 1,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) removeTrack(w http.ResponseWriter, r *http.Request) {
	if err := s.db.RemoveTrackFromPlaylist(r.Context(), store.RemoveTrackFromPlaylistParams{
		PlaylistID: chi.URLParam(r, "id"),
		TrackID:    chi.URLParam(r, "track_id"),
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
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
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	for _, item := range req.Order {
		if err := s.db.UpdatePlaylistTrackOrder(r.Context(), store.UpdatePlaylistTrackOrderParams{
			PlaylistID: playlistID,
			TrackID:    item.TrackID,
			Position:   int32(item.Position),
		}); err != nil {
			httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
