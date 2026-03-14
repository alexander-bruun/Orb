// Package smartplaylist handles smart playlist CRUD and track evaluation.
package smartplaylist

import (
	"encoding/json"
	"net/http"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Service handles smart playlist HTTP routes.
type Service struct {
	db *store.Store
}

// New returns a new smart playlist Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// Routes registers smart playlist endpoints.
func (s *Service) Routes(r chi.Router) {
	r.Get("/", s.list)
	r.Post("/", s.create)
	r.Get("/{id}", s.detail)
	r.Patch("/{id}", s.update)
	r.Delete("/{id}", s.delete)
	r.Get("/{id}/tracks", s.tracks)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	pls, err := s.db.ListSmartPlaylistsByUser(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pls)
}

type upsertReq struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Rules       []store.SmartPlaylistRule `json:"rules"`
	RuleMatch   string                    `json:"rule_match"`
	SortBy      string                    `json:"sort_by"`
	SortDir     string                    `json:"sort_dir"`
	LimitCount  *int                      `json:"limit_count"`
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var req upsertReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	req = applyDefaults(req)
	pl, err := s.db.CreateSmartPlaylist(r.Context(), store.CreateSmartPlaylistParams{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Rules:       req.Rules,
		RuleMatch:   req.RuleMatch,
		SortBy:      req.SortBy,
		SortDir:     req.SortDir,
		LimitCount:  req.LimitCount,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pl)
}

func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, err := s.db.GetSmartPlaylistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "smart playlist not found")
		return
	}
	writeJSON(w, http.StatusOK, pl)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.db.GetSmartPlaylistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "smart playlist not found")
		return
	}
	var req upsertReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Merge: keep existing values for fields not set in request.
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Description == "" {
		req.Description = existing.Description
	}
	if req.Rules == nil {
		req.Rules = existing.Rules
	}
	if req.RuleMatch == "" {
		req.RuleMatch = existing.RuleMatch
	}
	if req.SortBy == "" {
		req.SortBy = existing.SortBy
	}
	if req.SortDir == "" {
		req.SortDir = existing.SortDir
	}
	if req.LimitCount == nil {
		req.LimitCount = existing.LimitCount
	}
	pl, err := s.db.UpdateSmartPlaylist(r.Context(), store.UpdateSmartPlaylistParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Rules:       req.Rules,
		RuleMatch:   req.RuleMatch,
		SortBy:      req.SortBy,
		SortDir:     req.SortDir,
		LimitCount:  req.LimitCount,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pl)
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteSmartPlaylist(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) tracks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, err := s.db.GetSmartPlaylistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "smart playlist not found")
		return
	}
	tracks, err := s.db.EvaluateSmartPlaylist(r.Context(), pl)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func applyDefaults(req upsertReq) upsertReq {
	if req.RuleMatch == "" {
		req.RuleMatch = "all"
	}
	if req.SortBy == "" {
		req.SortBy = "title"
	}
	if req.SortDir == "" {
		req.SortDir = "asc"
	}
	if req.Rules == nil {
		req.Rules = []store.SmartPlaylistRule{}
	}
	return req
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
