// Package admin provides analytics and administration endpoints (admin-only).
package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
)

// Service handles admin HTTP routes.
type Service struct {
	db *store.Store
}

// New returns a new admin Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// AdminMiddleware rejects requests from non-admin users.
func (s *Service) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromCtx(r.Context())
		if userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := s.db.GetUserByID(r.Context(), userID)
		if err != nil || !u.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Routes registers admin endpoints — must be mounted inside jwtMW + AdminMiddleware.
func (s *Service) Routes(r chi.Router) {
	r.Get("/summary", s.summary)
	r.Get("/users", s.listUsers)
	r.Get("/top-tracks", s.topTracks)
	r.Get("/top-artists", s.topArtists)
	r.Get("/plays-by-day", s.playsByDay)
	r.Put("/users/{id}/admin", s.setUserAdmin)
}

// GET /admin/summary
func (s *Service) summary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.db.GetAdminSummary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sum)
}

// GET /admin/users
func (s *Service) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsersWithStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, users)
}

// GET /admin/top-tracks?limit=10
func (s *Service) topTracks(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 10)
	tracks, err := s.db.GetTopTracks(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tracks)
}

// GET /admin/top-artists?limit=10
func (s *Service) topArtists(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 10)
	artists, err := s.db.GetTopArtists(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, artists)
}

// GET /admin/plays-by-day?days=30
func (s *Service) playsByDay(w http.ResponseWriter, r *http.Request) {
	days := intQuery(r, "days", 30)
	data, err := s.db.GetPlaysByDay(r.Context(), days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

// PUT /admin/users/{id}/admin — body: {"is_admin": true|false}
func (s *Service) setUserAdmin(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	var body struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.db.SetUserAdmin(r.Context(), targetID, body.IsAdmin); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers ----

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
