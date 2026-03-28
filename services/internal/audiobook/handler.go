// Package audiobook provides HTTP handlers for audiobook browsing, streaming,
// progress tracking, and bookmark management.
package audiobook

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
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
	r.Get("/recently-added", s.listRecentlyAdded)
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
	r.Post("/admin/rescan/{id}", s.triggerRescanAudiobook)

	// Admin: missing metadata
	r.Get("/admin/no-cover", s.listNoCover)
	r.Get("/admin/no-series", s.listNoSeries)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func userIDFromCtx(r *http.Request) string {
	return auth.UserIDFromCtx(r.Context())
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := httputil.Pagination(r, 50, 200)
	sortBy := r.URL.Query().Get("sort_by")

	books, err := s.db.ListAudiobooks(r.Context(), store.ListAudiobooksParams{
		Limit:  int32(limit),
		Offset: int32(offset),
		SortBy: sortBy,
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list audiobooks: "+err.Error())
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books})
}

func (s *Service) listRecentlyAdded(w http.ResponseWriter, r *http.Request) {
	limit := httputil.QueryInt(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	books, err := s.db.ListRecentAudiobooks(r.Context(), limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list recent audiobooks: "+err.Error())
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books})
}

func (s *Service) listInProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit := httputil.QueryInt(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	books, err := s.db.ListInProgressAudiobooks(r.Context(), userID, limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list in-progress audiobooks: "+err.Error())
		return
	}
	if books == nil {
		books = []store.AudiobookWithProgress{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	book, err := s.db.GetAudiobook(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "get audiobook: "+err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobook": book})
}

func (s *Service) getProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	progress, err := s.db.GetAudiobookProgress(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No progress yet — return zeros.
			httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"progress": store.AudiobookProgress{UserID: userID, AudiobookID: id},
			})
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "get progress: "+err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"progress": progress})
}

func (s *Service) upsertProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")

	var body struct {
		PositionMs int64 `json:"position_ms"`
		Completed  bool  `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.UpsertAudiobookProgress(r.Context(), store.UpsertAudiobookProgressParams{
		UserID:      userID,
		AudiobookID: id,
		PositionMs:  body.PositionMs,
		Completed:   body.Completed,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "upsert progress: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) listBookmarks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	bookmarks, err := s.db.ListAudiobookBookmarks(r.Context(), userID, id)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list bookmarks: "+err.Error())
		return
	}
	if bookmarks == nil {
		bookmarks = []store.AudiobookBookmark{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"bookmarks": bookmarks})
}

func (s *Service) createBookmark(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")

	var body struct {
		PositionMs int64   `json:"position_ms"`
		Note       *string `json:"note,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
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
		httputil.WriteErr(w, http.StatusInternalServerError, "create bookmark: "+err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]interface{}{"bookmark": bm})
}

func (s *Service) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	bookmarkID := chi.URLParam(r, "bookmark_id")
	if err := s.db.DeleteAudiobookBookmark(r.Context(), bookmarkID, userID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "delete bookmark: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) listBySeries(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	books, err := s.db.ListAudiobooksBySeries(r.Context(), name)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list series: "+err.Error())
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books, "series": name})
}

func (s *Service) triggerRescanAudiobook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil || !u.IsAdmin || !u.IsActive {
		httputil.WriteErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.ingestService == nil {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "audiobook ingest not configured (set AUDIOBOOK_DIRS)")
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.ingestService.TriggerReingestAudiobook(id); err != nil {
		httputil.WriteErr(w, http.StatusConflict, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "reingest started"})
}

func (s *Service) triggerScan(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil || !u.IsAdmin || !u.IsActive {
		httputil.WriteErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.ingestService == nil {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "audiobook ingest not configured (set AUDIOBOOK_DIRS)")
		return
	}
	if r.URL.Query().Get("force") == "true" {
		if err := s.ingestService.TriggerForceScan(s.ingestService.RootCtx()); err != nil {
			httputil.WriteErr(w, http.StatusConflict, err.Error())
			return
		}
		httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "force scan started"})
		return
	}
	if err := s.ingestService.TriggerScan(s.ingestService.RootCtx()); err != nil {
		httputil.WriteErr(w, http.StatusConflict, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}

func (s *Service) listNoCover(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil || !u.IsAdmin || !u.IsActive {
		httputil.WriteErr(w, http.StatusForbidden, "forbidden")
		return
	}

	l, o := httputil.Pagination(r, 50, 200)
	limit, offset := int32(l), int32(o)

	books, total, err := s.db.ListAudiobooksNoCover(r.Context(), limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list audiobooks no cover: "+err.Error())
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books, "total": total})
}

func (s *Service) listNoSeries(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil || !u.IsAdmin || !u.IsActive {
		httputil.WriteErr(w, http.StatusForbidden, "forbidden")
		return
	}

	l, o := httputil.Pagination(r, 50, 200)
	limit, offset := int32(l), int32(o)

	books, total, err := s.db.ListAudiobooksNoSeries(r.Context(), limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list audiobooks no series: "+err.Error())
		return
	}
	if books == nil {
		books = []store.Audiobook{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"audiobooks": books, "total": total})
}
