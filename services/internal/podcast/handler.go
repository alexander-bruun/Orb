package podcast

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	pgx "github.com/jackc/pgx/v5"
)

// Handler handles podcast HTTP endpoints.
type Handler struct {
	svc *Service
	db  *store.Store
}

// NewHandler creates a new podcast Handler.
func NewHandler(svc *Service, db *store.Store) *Handler {
	return &Handler{svc: svc, db: db}
}

// Routes registers all podcast routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/subscribe", h.subscribe)
	r.Get("/subscriptions", h.listSubscriptions)
	r.Get("/recently-added", h.listRecentlyAdded)
	r.Get("/with-new-episodes", h.listWithNewEpisodes)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	r.Get("/{id}/episodes", h.listEpisodes)
	r.Delete("/{id}/unsubscribe", h.unsubscribe)

	r.Get("/episodes/in-progress", h.listInProgress)
	r.Get("/episodes/{id}", h.getEpisode)
	r.Get("/episodes/{id}/progress", h.getProgress)
	r.Put("/episodes/{id}/progress", h.upsertProgress)
	r.Post("/episodes/{id}/download", h.downloadEpisode)
}

func userIDFromCtx(r *http.Request) string {
	return auth.UserIDFromCtx(r.Context())
}

func (h *Handler) listRecentlyAdded(w http.ResponseWriter, r *http.Request) {
	limit := httputil.QueryInt(r, "limit", 20)
	podcasts, err := h.db.ListRecentlyAddedPodcasts(r.Context(), int32(limit))
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list recently added: "+err.Error())
		return
	}
	if podcasts == nil {
		podcasts = []store.Podcast{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"podcasts": podcasts})
}

func (h *Handler) listWithNewEpisodes(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	limit := httputil.QueryInt(r, "limit", 20)
	podcasts, err := h.db.ListPodcastsWithNewEpisodes(r.Context(), userID, int32(limit))
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list with new episodes: "+err.Error())
		return
	}
	if podcasts == nil {
		podcasts = []store.Podcast{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"podcasts": podcasts})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := httputil.Pagination(r, 50, 200)
	podcasts, err := h.db.ListPodcasts(r.Context(), int32(limit), int32(offset))
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list podcasts: "+err.Error())
		return
	}
	if podcasts == nil {
		podcasts = []store.Podcast{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"podcasts": podcasts})
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		Url string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	p, err := h.svc.AddPodcastByRSS(r.Context(), body.Url)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "subscribe: "+err.Error())
		return
	}

	if err := h.db.SubscribeUserToPodcast(r.Context(), userID, p.ID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "subscribe user: "+err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]interface{}{"podcast": p})
}

func (h *Handler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	limit, offset := httputil.Pagination(r, 50, 200)
	podcasts, err := h.db.ListUserSubscriptions(r.Context(), userID, int32(limit), int32(offset))
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list subscriptions: "+err.Error())
		return
	}
	if podcasts == nil {
		podcasts = []store.Podcast{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"podcasts": podcasts})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.db.GetPodcast(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "get podcast: "+err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"podcast": p})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Title       string  `json:"title"`
		Description *string `json:"description"`
		Author      *string `json:"author"`
		RssUrl      string  `json:"rss_url"`
		Link        *string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	p, err := h.db.GetPodcast(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "get podcast: "+err.Error())
		return
	}

	if err := h.db.UpdatePodcast(r.Context(), store.UpdatePodcastParams{
		ID:          id,
		Title:       body.Title,
		Description: body.Description,
		Author:      body.Author,
		RssUrl:      body.RssUrl,
		Link:        body.Link,
		CoverArtKey: p.CoverArtKey,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "update podcast: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeletePodcast(r.Context(), id); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "delete podcast: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listEpisodes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit, offset := httputil.Pagination(r, 50, 200)
	search := r.URL.Query().Get("search")
	sortBy := r.URL.Query().Get("sort_by")
	sortDir := r.URL.Query().Get("sort_dir")

	total, err := h.db.CountPodcastEpisodes(r.Context(), id, search)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "count episodes: "+err.Error())
		return
	}

	episodes, err := h.db.ListPodcastEpisodes(r.Context(), store.ListPodcastEpisodesParams{
		PodcastID: id,
		Search:    search,
		SortBy:    sortBy,
		SortDir:   sortDir,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list episodes: "+err.Error())
		return
	}
	if episodes == nil {
		episodes = []store.PodcastEpisode{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"episodes": episodes, "total": total})
}

func (h *Handler) unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id := chi.URLParam(r, "id")
	if err := h.db.UnsubscribeUserFromPodcast(r.Context(), userID, id); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "unsubscribe: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listInProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	limit := httputil.QueryInt(r, "limit", 20)

	episodes, err := h.db.ListInProgressEpisodes(r.Context(), userID, limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "list in-progress: "+err.Error())
		return
	}
	if episodes == nil {
		episodes = []store.PodcastEpisode{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"episodes": episodes})
}

func (h *Handler) getEpisode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := h.db.GetPodcastEpisode(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "get episode: "+err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"episode": ep})
}

func (h *Handler) getProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id := chi.URLParam(r, "id")
	progress, err := h.db.GetPodcastEpisodeProgress(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"progress": store.PodcastEpisodeProgress{UserID: userID, EpisodeID: id},
			})
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "get progress: "+err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"progress": progress})
}

func (h *Handler) upsertProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id := chi.URLParam(r, "id")

	var body struct {
		PositionMs int64 `json:"position_ms"`
		Completed  bool  `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.db.UpsertPodcastEpisodeProgress(r.Context(), store.UpsertPodcastEpisodeProgressParams{
		UserID:     userID,
		EpisodeID:  id,
		PositionMs: body.PositionMs,
		Completed:  body.Completed,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "upsert progress: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) downloadEpisode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// This could be slow, so we might want to do it in background and return 202.
	// But for now, let's just do it.
	go func() {
		if err := h.svc.DownloadEpisode(context.Background(), id); err != nil {
			slog.Error("async download failed", "id", id, "err", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}
