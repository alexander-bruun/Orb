// Package audiobook provides HTTP handlers for audiobook browsing, streaming,
// progress tracking, and bookmark management.
package audiobook

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/ingest"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
)

// Service handles audiobook HTTP endpoints.
type Service struct {
	db            *store.Store
	obj           objstore.ObjectStore
	ingestService *ingest.AudiobookIngestService // may be nil
}

// New creates an audiobook Service.
func New(db *store.Store, obj objstore.ObjectStore, ingestSvc *ingest.AudiobookIngestService) *Service {
	return &Service{db: db, obj: obj, ingestService: ingestSvc}
}

// Routes registers all audiobook routes. Must be mounted under JWT middleware.
func (s *Service) Routes(r chi.Router) {
	// Browse
	r.Get("/", s.list)
	r.Get("/in-progress", s.listInProgress)
	r.Get("/series/{name}", s.listBySeries)
	r.Get("/{id}", s.get)

	// Streaming (reuses the /stream infrastructure via file_key)
	// The frontend builds the URL itself using the file_key.

	// Cover art (public — served by stream.Cover handler via /covers/audiobook/{id})

	// Progress
	r.Get("/{id}/progress", s.getProgress)
	r.Put("/{id}/progress", s.upsertProgress)

	// Bookmarks
	r.Get("/{id}/bookmarks", s.listBookmarks)
	r.Post("/{id}/bookmarks", s.createBookmark)
	r.Delete("/{id}/bookmarks/{bookmark_id}", s.deleteBookmark)

	// Admin: trigger audiobook scan
	r.Post("/admin/scan", s.triggerScan)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func userIDFromCtx(r *http.Request) string {
	return auth.UserIDFromCtx(r.Context())
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	sortBy := r.URL.Query().Get("sort_by")

	books, err := s.db.ListAudiobooks(r.Context(), store.ListAudiobooksParams{
		Limit:  int32(limit),
		Offset: int32(offset),
		SortBy: sortBy,
	})
	if err != nil {
		http.Error(w, "list audiobooks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books})
}

func (s *Service) listInProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	books, err := s.db.ListInProgressAudiobooks(r.Context(), userID, limit)
	if err != nil {
		http.Error(w, "list in-progress audiobooks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if books == nil {
		books = []store.AudiobookWithProgress{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	book, err := s.db.GetAudiobook(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "get audiobook: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"audiobook": book})
}

func (s *Service) getProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	progress, err := s.db.GetAudiobookProgress(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No progress yet — return zeros.
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"progress": store.AudiobookProgress{UserID: userID, AudiobookID: id},
			})
			return
		}
		http.Error(w, "get progress: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"progress": progress})
}

func (s *Service) upsertProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var body struct {
		PositionMs int64 `json:"position_ms"`
		Completed  bool  `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.db.UpsertAudiobookProgress(r.Context(), store.UpsertAudiobookProgressParams{
		UserID:      userID,
		AudiobookID: id,
		PositionMs:  body.PositionMs,
		Completed:   body.Completed,
	}); err != nil {
		http.Error(w, "upsert progress: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) listBookmarks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	bookmarks, err := s.db.ListAudiobookBookmarks(r.Context(), userID, id)
	if err != nil {
		http.Error(w, "list bookmarks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if bookmarks == nil {
		bookmarks = []store.AudiobookBookmark{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"bookmarks": bookmarks})
}

func (s *Service) createBookmark(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var body struct {
		PositionMs int64   `json:"position_ms"`
		Note       *string `json:"note,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	bm, err := s.db.CreateAudiobookBookmark(r.Context(), store.CreateAudiobookBookmarkParams{
		ID:          uuid.New().String(),
		UserID:      userID,
		AudiobookID: id,
		PositionMs:  body.PositionMs,
		Note:        body.Note,
	})
	if err != nil {
		http.Error(w, "create bookmark: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"bookmark": bm})
}

func (s *Service) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	bookmarkID := chi.URLParam(r, "bookmark_id")
	if err := s.db.DeleteAudiobookBookmark(r.Context(), bookmarkID, userID); err != nil {
		http.Error(w, "delete bookmark: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) listBySeries(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	books, err := s.db.ListAudiobooksBySeries(r.Context(), name)
	if err != nil {
		http.Error(w, "list series: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books, "series": name})
}

func (s *Service) triggerScan(w http.ResponseWriter, r *http.Request) {
	if s.ingestService == nil {
		http.Error(w, "audiobook ingest not configured (set AUDIOBOOK_DIRS)", http.StatusServiceUnavailable)
		return
	}
	if err := s.ingestService.TriggerScan(s.ingestService.RootCtx()); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}
