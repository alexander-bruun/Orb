// Package admin provides analytics and administration endpoints (admin-only).
package admin

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/mailer"
	"github.com/alexander-bruun/orb/services/internal/musicbrainz"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/openlibrary"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/alexander-bruun/orb/services/internal/webhook"
	"github.com/dhowden/tag"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

// Service handles admin HTTP routes.
type Service struct {
	db         *store.Store
	obj        objstore.ObjectStore
	mb         *musicbrainz.Client
	kv         *redis.Client // optional; used to invalidate sessions on deactivation
	dispatcher *webhook.Dispatcher
	dbURL      string
	storeRoot  string
	logPath    string
}

// New returns a new admin Service.
func New(db *store.Store, obj objstore.ObjectStore, mb *musicbrainz.Client, kv *redis.Client, dbURL, storeRoot, logPath string) *Service {
	return &Service{
		db:        db,
		obj:       obj,
		mb:        mb,
		kv:        kv,
		dbURL:     dbURL,
		storeRoot: storeRoot,
		logPath:   logPath,
	}
}

// SetDispatcher attaches a webhook dispatcher. Must be called before serving requests.
func (s *Service) SetDispatcher(d *webhook.Dispatcher) { s.dispatcher = d }

// AdminMiddleware rejects requests from non-admin users and inactive users.
func (s *Service) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromCtx(r.Context())
		if userID == "" {
			httputil.WriteErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		u, err := s.db.GetUserByID(r.Context(), userID)
		if err != nil || !u.IsAdmin || !u.IsActive {
			httputil.WriteErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Routes registers admin endpoints — must be mounted inside jwtMW + AdminMiddleware.
func (s *Service) Routes(r chi.Router) {
	// Analytics
	r.Get("/summary", s.summary)
	r.Get("/users", s.listUsers)
	r.Get("/top-tracks", s.topTracks)
	r.Get("/top-artists", s.topArtists)
	r.Get("/plays-by-day", s.playsByDay)
	r.Get("/storage", s.storageStats)

	// User management
	r.Put("/users/{id}/admin", s.setUserAdmin)
	r.Put("/users/{id}/active", s.setUserActive)
	r.Put("/users/{id}/quota", s.setUserQuota)
	r.Delete("/users/{id}", s.deleteUser)

	// Invite tokens
	r.Post("/invites", s.createInvite)
	r.Get("/invites", s.listInvites)
	r.Delete("/invites/{token}", s.revokeInvite)

	// Audit log
	r.Get("/audit-logs", s.auditLogs)
	r.Get("/logs", s.logs)

	// Library / job control
	r.Post("/albums/{id}/refetch-cover", s.refetchAlbumCover)
	r.Put("/albums/{id}/cover", s.uploadAlbumCover)
	r.Get("/albums/no-cover", s.albumsNoCover)
	r.Post("/audiobooks/{id}/refetch-cover", s.refetchAudiobookCover)
	r.Put("/audiobooks/{id}/cover", s.uploadAudiobookCover)

	// System backup / restore
	r.Get("/backup", s.backup)
	r.Post("/backup", s.backup)
	r.Post("/restore", s.restore)

	// Metadata editing (admin inline-edit)
	r.Patch("/albums/{id}", s.updateAlbumMeta)
	r.Patch("/tracks/{id}", s.updateTrackMeta)
	r.Patch("/audiobooks/{id}", s.updateAudiobookMeta)
	r.Post("/audiobooks/{id}/refresh", s.refreshAudiobookMeta)

	// Site settings
	r.Get("/settings", s.getSettings)
	r.Put("/settings/smtp", s.updateSmtpSettings)
	r.Post("/settings/smtp/test", s.testSmtp)

	// Webhooks
	r.Get("/webhooks", s.listWebhooks)
	r.Post("/webhooks", s.createWebhook)
	r.Get("/webhooks/{id}", s.getWebhook)
	r.Put("/webhooks/{id}", s.updateWebhook)
	r.Delete("/webhooks/{id}", s.deleteWebhook)
	r.Get("/webhooks/{id}/deliveries", s.listWebhookDeliveries)
	r.Post("/webhooks/{id}/test", s.testWebhook)
	r.Get("/webhooks/events", s.listWebhookEvents)
}

// ── Analytics ────────────────────────────────────────────────────────────────

// GET /admin/summary
func (s *Service) summary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.db.GetAdminSummary(r.Context())
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, sum)
}

// GET /admin/users
func (s *Service) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsersWithStats(r.Context())
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, users)
}

// GET /admin/top-tracks?limit=10
func (s *Service) topTracks(w http.ResponseWriter, r *http.Request) {
	limit := httputil.QueryInt(r, "limit", 10)
	tracks, err := s.db.GetTopTracks(r.Context(), limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, tracks)
}

// GET /admin/top-artists?limit=10
func (s *Service) topArtists(w http.ResponseWriter, r *http.Request) {
	limit := httputil.QueryInt(r, "limit", 10)
	artists, err := s.db.GetTopArtists(r.Context(), limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, artists)
}

// GET /admin/plays-by-day?days=30
func (s *Service) playsByDay(w http.ResponseWriter, r *http.Request) {
	days := httputil.QueryInt(r, "days", 30)
	data, err := s.db.GetPlaysByDay(r.Context(), days)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, data)
}

// GET /admin/storage
func (s *Service) storageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStorageStats(r.Context())
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, stats)
}

// ── User management ──────────────────────────────────────────────────────────

// PUT /admin/users/{id}/admin — body: {"is_admin": true|false}
func (s *Service) setUserAdmin(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.SetUserAdmin(r.Context(), targetID, body.IsAdmin); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "set_admin", "user", targetID,
		map[string]any{"is_admin": body.IsAdmin}); err != nil {
		slog.Warn("audit log failed", "action", "set_admin", "err", err)
	}
	event := webhook.EventUserAdminGranted
	if !body.IsAdmin {
		event = webhook.EventUserAdminRevoked
	}
	s.dispatch(r.Context(), event, map[string]any{"user_id": targetID, "is_admin": body.IsAdmin})
	w.WriteHeader(http.StatusNoContent)
}

// PUT /admin/users/{id}/active — body: {"active": true|false}
func (s *Service) setUserActive(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.SetUserActive(r.Context(), targetID, body.Active); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Immediately invalidate any active session so the user is logged out.
	if !body.Active && s.kv != nil {
		if err := s.kv.Del(r.Context(), kvkeys.Session(targetID)).Err(); err != nil {
			slog.Warn("session invalidation failed", "action", "set_active", "err", err)
		}
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "set_active", "user", targetID,
		map[string]any{"active": body.Active}); err != nil {
		slog.Warn("audit log failed", "action", "set_active", "err", err)
	}
	event := webhook.EventUserActivated
	if !body.Active {
		event = webhook.EventUserDeactivated
	}
	s.dispatch(r.Context(), event, map[string]any{"user_id": targetID, "is_active": body.Active})
	w.WriteHeader(http.StatusNoContent)
}

// PUT /admin/users/{id}/quota — body: {"quota_bytes": 10737418240} or {"quota_bytes": null}
func (s *Service) setUserQuota(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		QuotaBytes *int64 `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.SetUserQuota(r.Context(), targetID, body.QuotaBytes); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "set_quota", "user", targetID,
		map[string]any{"quota_bytes": body.QuotaBytes}); err != nil {
		slog.Warn("audit log failed", "action", "set_quota", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /admin/users/{id}
func (s *Service) deleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	if targetID == actorID {
		httputil.WriteErr(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if err := s.db.DeleteUser(r.Context(), targetID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "delete_user", "user", targetID, nil); err != nil {
		slog.Warn("audit log failed", "action", "delete_user", "err", err)
	}
	s.dispatch(r.Context(), webhook.EventUserDeleted, map[string]any{"user_id": targetID})
	w.WriteHeader(http.StatusNoContent)
}

// ── Invite tokens ─────────────────────────────────────────────────────────────

// POST /admin/invites — body: {"email": "user@example.com"}
func (s *Service) createInvite(w http.ResponseWriter, r *http.Request) {
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "email required")
		return
	}

	// Generate a secure 32-byte token.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	if err := s.db.CreateInviteToken(r.Context(), token, body.Email, actorID, expiresAt); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "create_invite", "invite", token,
		map[string]any{"email": body.Email, "expires_at": expiresAt}); err != nil {
		slog.Warn("audit log failed", "action", "create_invite", "err", err)
	}

	// Try to send invite email if SMTP is configured (best-effort).
	smtpKeys := []string{"site_base_url", "smtp_host", "smtp_port", "smtp_username", "smtp_password",
		"smtp_from_address", "smtp_from_name", "smtp_tls"}
	cfg, _ := s.db.GetSiteSettings(r.Context(), smtpKeys)
	siteURL := cfg["site_base_url"]
	if siteURL == "" {
		siteURL = requestBaseURL(r)
	}
	inviteURL := fmt.Sprintf("%s/register?invite=%s", siteURL, token)

	if cfg["smtp_host"] != "" && cfg["smtp_from_address"] != "" {
		m := mailer.New(mailer.Config{
			Host:        cfg["smtp_host"],
			Port:        cfg["smtp_port"],
			Username:    cfg["smtp_username"],
			Password:    cfg["smtp_password"],
			FromAddress: cfg["smtp_from_address"],
			FromName:    cfg["smtp_from_name"],
			TLS:         cfg["smtp_tls"] == "true",
		})
		if err := m.SendInvite(r.Context(), body.Email, inviteURL); err != nil {
			slog.Warn("admin: invite email failed", "to", body.Email, "err", err)
		}
	}

	httputil.WriteOK(w, map[string]string{
		"token":      token,
		"invite_url": inviteURL,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// GET /admin/invites
func (s *Service) listInvites(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.db.ListInviteTokens(r.Context())
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, tokens)
}

// DELETE /admin/invites/{token}
func (s *Service) revokeInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	actorID := auth.UserIDFromCtx(r.Context())
	if err := s.db.RevokeInviteToken(r.Context(), token); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "revoke_invite", "invite", token, nil); err != nil {
		slog.Warn("audit log failed", "action", "revoke_invite", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Audit log ────────────────────────────────────────────────────────────────

// GET /admin/audit-logs?limit=50&offset=0
func (s *Service) auditLogs(w http.ResponseWriter, r *http.Request) {
	limit := httputil.QueryInt(r, "limit", 50)
	offset := httputil.QueryInt(r, "offset", 0)
	logs, total, err := s.db.ListAuditLogs(r.Context(), limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, map[string]any{"logs": logs, "total": total})
}

// GET /admin/logs?lines=300
func (s *Service) logs(w http.ResponseWriter, r *http.Request) {
	lines := httputil.QueryInt(r, "lines", 300)
	if lines < 1 {
		lines = 1
	}
	if lines > 2000 {
		lines = 2000
	}
	if s.logPath == "" {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "log file path not configured")
		return
	}
	out, err := tailFile(s.logPath, lines)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			httputil.WriteOK(w, map[string]any{
				"path":   s.logPath,
				"exists": false,
				"lines":  []string{},
			})
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, map[string]any{
		"path":   s.logPath,
		"exists": true,
		"lines":  out,
	})
}

// POST /admin/backup
// Creates a tar.gz with:
// - metadata.json
// - database.dump (pg_dump custom format)
// - static/* (object-store files)
func (s *Service) backup(w http.ResponseWriter, r *http.Request) {
	if s.dbURL == "" {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "database url not configured")
		return
	}
	if s.storeRoot == "" {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "store root not configured")
		return
	}

	actorID := auth.UserIDFromCtx(r.Context())
	slog.Info("admin backup started", "actor_id", actorID)
	dbDump, err := os.CreateTemp("", "orb-backup-db-*.dump")
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "create temp db dump: "+err.Error())
		return
	}
	dbDumpPath := dbDump.Name()
	_ = dbDump.Close()
	defer func() { _ = os.Remove(dbDumpPath) }()

	if err := runPgDump(r.Context(), s.dbURL, dbDumpPath); err != nil {
		slog.Warn("admin backup pg_dump failed", "actor_id", actorID, "err", err)
		httputil.WriteErr(w, http.StatusInternalServerError, "database backup failed: "+err.Error())
		return
	}

	now := time.Now().UTC()
	fileName := fmt.Sprintf("orb-backup-%s.tar.gz", now.Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))

	gzw := gzip.NewWriter(w)
	defer func() { _ = gzw.Close() }()
	tw := tar.NewWriter(gzw)
	defer func() { _ = tw.Close() }()

	meta := map[string]any{
		"created_at": now.Format(time.RFC3339),
		"format":     "orb-backup-v1",
		"includes":   []string{"database.dump", "static/(non-media objects)"},
		"excludes":   []string{"static/audio/**", "static/audiobooks/**", "static/audiobook-chapters/**"},
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := writeTarBytes(tw, "metadata.json", metaBytes, now); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "write metadata: "+err.Error())
		return
	}

	if err := writeTarFile(tw, "database.dump", dbDumpPath); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "write database dump: "+err.Error())
		return
	}

	if err := writeTarDir(tw, s.storeRoot, "static", now); err != nil {
		slog.Warn("admin backup archive failed", "actor_id", actorID, "err", err)
		httputil.WriteErr(w, http.StatusInternalServerError, "write static files: "+err.Error())
		return
	}
	slog.Info("admin backup completed", "actor_id", actorID, "filename", fileName)

	if err := s.db.InsertAuditLog(r.Context(), actorID, "backup_created", "backup", "", map[string]any{
		"filename": fileName,
	}); err != nil {
		slog.Warn("audit log failed", "action", "backup_created", "err", err)
	}
}

// POST /admin/restore
// Accepts tar.gz backup upload in multipart field "backup".
func (s *Service) restore(w http.ResponseWriter, r *http.Request) {
	if s.dbURL == "" {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "database url not configured")
		return
	}
	if s.storeRoot == "" {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "store root not configured")
		return
	}

	actorID := auth.UserIDFromCtx(r.Context())
	backupFile, err := saveIncomingBackupToTemp(r)
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid backup upload: "+err.Error())
		return
	}
	defer func() { _ = os.Remove(backupFile) }()

	dbDumpPath, staticDir, err := extractBackup(backupFile)
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid backup archive: "+err.Error())
		return
	}
	defer func() { _ = os.Remove(dbDumpPath) }()
	defer func() { _ = os.RemoveAll(staticDir) }()

	if err := runPgRestore(r.Context(), s.dbURL, dbDumpPath); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "database restore failed: "+err.Error())
		return
	}

	if err := restoreNonMediaContents(s.storeRoot, staticDir); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "static files restore failed: "+err.Error())
		return
	}

	if err := s.db.InsertAuditLog(r.Context(), actorID, "backup_restored", "backup", "", nil); err != nil {
		slog.Warn("audit log failed", "action", "backup_restored", "err", err)
	}
	httputil.WriteOK(w, map[string]string{"status": "restored"})
}

// ── Artwork ───────────────────────────────────────────────────────────────────

// GET /admin/albums/no-cover?limit=50&offset=0
func (s *Service) albumsNoCover(w http.ResponseWriter, r *http.Request) {
	limit := httputil.QueryInt(r, "limit", 50)
	offset := httputil.QueryInt(r, "offset", 0)
	albums, total, err := s.db.ListAlbumsWithoutCover(r.Context(), limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, map[string]any{"albums": albums, "total": total})
}

// POST /admin/albums/{id}/refetch-cover
func (s *Service) refetchAlbumCover(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())

	if s.mb == nil {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "MusicBrainz client not configured")
		return
	}

	album, err := s.db.GetAlbumByID(r.Context(), albumID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "album not found")
		return
	}

	// Attempt via release group MBID first.
	var imgData []byte
	if album.ReleaseGroupMbid != nil && *album.ReleaseGroupMbid != "" {
		imgData, err = s.mb.FetchAlbumCoverArt(r.Context(), *album.ReleaseGroupMbid)
		if err != nil {
			slog.Warn("admin: refetch cover via MBID failed", "album", albumID, "err", err)
		}
	}

	// Fall back to MusicBrainz search if no MBID or fetch failed.
	if imgData == nil {
		artistName := derefOr(album.ArtistName, "")
		enrichment, searchErr := s.mb.EnrichAlbum(r.Context(), album.Title, artistName)
		if searchErr == nil && enrichment != nil && enrichment.ReleaseGroupMbid != "" {
			imgData, err = s.mb.FetchAlbumCoverArt(r.Context(), enrichment.ReleaseGroupMbid)
			if err != nil {
				slog.Warn("admin: refetch cover via search failed", "album", albumID, "err", err)
			}
		}
	}

	// Final fallback: try embedded/folder art from original local ingest paths.
	if imgData == nil {
		paths, pathErr := s.db.GetIngestPathsForAlbum(r.Context(), albumID)
		if pathErr != nil {
			slog.Warn("admin: refetch cover ingest-path lookup failed", "album", albumID, "err", pathErr)
		} else {
			imgData = findLocalAlbumCover(paths)
		}
	}

	if imgData == nil {
		httputil.WriteErr(w, http.StatusNotFound, "no cover art found")
		return
	}

	coverKey := fmt.Sprintf("covers/%s.jpg", albumID)
	if err := storeCoverArt(r.Context(), s.obj, coverKey, imgData); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "failed to store cover art: "+err.Error())
		return
	}
	if err := s.db.UpdateAlbumCoverArt(r.Context(), albumID, coverKey); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "refetch_cover", "album", albumID, nil); err != nil {
		slog.Warn("audit log failed", "action", "refetch_cover", "err", err)
	}
	httputil.WriteOK(w, map[string]string{"cover_art_key": coverKey})
}

// PUT /admin/albums/{id}/cover — upload a cover image (multipart field "cover").
func (s *Service) uploadAlbumCover(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("cover")
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "missing cover field")
		return
	}
	defer func() { _ = file.Close() }()

	imgData, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "failed to read cover")
		return
	}

	coverKey := fmt.Sprintf("covers/%s.jpg", albumID)
	if err := storeCoverArt(r.Context(), s.obj, coverKey, imgData); err != nil {
		httputil.WriteErr(w, http.StatusUnprocessableEntity, "invalid image: "+err.Error())
		return
	}
	if err := s.db.UpdateAlbumCoverArt(r.Context(), albumID, coverKey); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "upload_cover", "album", albumID, nil); err != nil {
		slog.Warn("audit log failed", "action", "upload_cover", "err", err)
	}
	httputil.WriteOK(w, map[string]string{"cover_art_key": coverKey})
}

// POST /admin/audiobooks/{id}/refetch-cover — re-fetch cover art from OpenLibrary.
func (s *Service) refetchAudiobookCover(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())

	book, err := s.db.GetAudiobook(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "audiobook not found")
		return
	}

	ol := openlibrary.New()
	authorName := derefOr(book.AuthorName, "")
	meta, olErr := ol.Search(r.Context(), book.Title, authorName)
	if olErr != nil || meta == nil || meta.CoverID == 0 {
		httputil.WriteErr(w, http.StatusNotFound, "no cover art found")
		return
	}

	coverData, err := ol.FetchCoverArt(r.Context(), meta.CoverID)
	if err != nil || len(coverData) == 0 {
		httputil.WriteErr(w, http.StatusNotFound, "no cover art found")
		return
	}

	coverKey := fmt.Sprintf("covers/audiobook/%s.jpg", id)
	if err := storeCoverArt(r.Context(), s.obj, coverKey, coverData); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "failed to store cover art: "+err.Error())
		return
	}
	if err := s.db.UpdateAudiobookCoverArt(r.Context(), id, coverKey); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "refetch_cover", "audiobook", id, nil); err != nil {
		slog.Warn("audit log failed", "action", "refetch_cover", "err", err)
	}
	httputil.WriteOK(w, map[string]string{"cover_art_key": coverKey})
}

// PUT /admin/audiobooks/{id}/cover — upload a cover image (multipart field "cover").
func (s *Service) uploadAudiobookCover(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("cover")
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "missing cover field")
		return
	}
	defer func() { _ = file.Close() }()

	imgData, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "failed to read cover")
		return
	}

	coverKey := fmt.Sprintf("covers/audiobook/%s.jpg", id)
	if err := storeCoverArt(r.Context(), s.obj, coverKey, imgData); err != nil {
		httputil.WriteErr(w, http.StatusUnprocessableEntity, "invalid image: "+err.Error())
		return
	}
	if err := s.db.UpdateAudiobookCoverArt(r.Context(), id, coverKey); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "upload_cover", "audiobook", id, nil); err != nil {
		slog.Warn("audit log failed", "action", "upload_cover", "err", err)
	}
	httputil.WriteOK(w, map[string]string{"cover_art_key": coverKey})
}

// ── Metadata editing ─────────────────────────────────────────────────────────

// PATCH /admin/albums/{id} — body: {title, release_year?, label?}
func (s *Service) updateAlbumMeta(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Title       string  `json:"title"`
		ReleaseYear *int    `json:"release_year"`
		Label       *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Title == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "title required")
		return
	}
	if err := s.db.UpdateAlbumMeta(r.Context(), store.UpdateAlbumMetaParams{
		ID:          id,
		Title:       body.Title,
		ReleaseYear: body.ReleaseYear,
		Label:       body.Label,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "update_album_meta", "album", id,
		map[string]any{"title": body.Title}); err != nil {
		slog.Warn("audit log failed", "action", "update_album_meta", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /admin/tracks/{id} — body: {title, track_number?, disc_number?}
func (s *Service) updateTrackMeta(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Title       string `json:"title"`
		TrackNumber *int   `json:"track_number"`
		DiscNumber  *int   `json:"disc_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Title == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "title required")
		return
	}
	discNumber := derefOr(body.DiscNumber, 1)
	if err := s.db.UpdateTrackMeta(r.Context(), store.UpdateTrackMetaParams{
		ID:          id,
		Title:       body.Title,
		TrackNumber: body.TrackNumber,
		DiscNumber:  discNumber,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "update_track_meta", "track", id,
		map[string]any{"title": body.Title}); err != nil {
		slog.Warn("audit log failed", "action", "update_track_meta", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /admin/audiobooks/{id} — body: {title, author_name?, description?, series?, series_index?, published_year?}
func (s *Service) updateAudiobookMeta(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Title         string   `json:"title"`
		AuthorName    *string  `json:"author_name"`
		Description   *string  `json:"description"`
		Series        *string  `json:"series"`
		SeriesIndex   *float64 `json:"series_index"`
		PublishedYear *int     `json:"published_year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Title == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "title required")
		return
	}

	// Resolve author name → author_id if provided.
	var authorID *string
	var updateAuthor bool
	if body.AuthorName != nil {
		updateAuthor = true
		if *body.AuthorName != "" {
			aid, err := s.db.FindOrCreateArtistByName(r.Context(), *body.AuthorName)
			if err != nil {
				httputil.WriteErr(w, http.StatusInternalServerError, "resolve author: "+err.Error())
				return
			}
			authorID = &aid
		}
		// empty string → updateAuthor=true, authorID=nil → clears author_id to NULL
	}

	if err := s.db.UpdateAudiobookMeta(r.Context(), store.UpdateAudiobookMetaParams{
		ID:            id,
		Title:         body.Title,
		UpdateAuthor:  updateAuthor,
		AuthorID:      authorID,
		Description:   body.Description,
		Series:        body.Series,
		SeriesIndex:   body.SeriesIndex,
		PublishedYear: body.PublishedYear,
	}); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "update_audiobook_meta", "audiobook", id,
		map[string]any{"title": body.Title}); err != nil {
		slog.Warn("audit log failed", "action", "update_audiobook_meta", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/audiobooks/{id}/refresh — re-fetches metadata + cover from OpenLibrary.
func (s *Service) refreshAudiobookMeta(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())

	book, err := s.db.GetAudiobook(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "audiobook not found")
		return
	}

	// Fetch cover from OpenLibrary using ISBN or title+author.
	var coverData []byte
	ol := openlibrary.New()
	authorName := derefOr(book.AuthorName, "")
	meta, olErr := ol.Search(r.Context(), book.Title, authorName)
	if olErr == nil && meta != nil {
		if meta.CoverID > 0 {
			coverData, _ = ol.FetchCoverArt(r.Context(), meta.CoverID)
		}
		// Update metadata fields returned by OpenLibrary.
		params := store.UpdateAudiobookMetaParams{
			ID:    id,
			Title: book.Title, // keep existing title unless OL has one
		}
		if meta.Description != "" {
			params.Description = &meta.Description
		}
		if meta.PublishedYear > 0 {
			params.PublishedYear = &meta.PublishedYear
		}
		if err := s.db.UpdateAudiobookMeta(r.Context(), params); err != nil {
			slog.Warn("update audiobook meta failed", "action", "refresh_audiobook_meta", "err", err)
		}
	}

	// Store cover art if fetched.
	if len(coverData) > 0 {
		coverKey := fmt.Sprintf("covers/audiobook/%s.jpg", id)
		if err := storeCoverArt(r.Context(), s.obj, coverKey, coverData); err == nil {
			if err := s.db.UpdateAudiobookCoverArt(r.Context(), id, coverKey); err != nil {
				slog.Warn("update audiobook cover art failed", "action", "refresh_audiobook_meta", "err", err)
			}
		}
	}

	if err := s.db.InsertAuditLog(r.Context(), actorID, "refresh_audiobook_meta", "audiobook", id, nil); err != nil {
		slog.Warn("audit log failed", "action", "refresh_audiobook_meta", "err", err)
	}
	httputil.WriteOK(w, map[string]string{"status": "refreshed"})
}

// requestBaseURL derives the site base URL from the incoming HTTP request.
// It prefers X-Forwarded-Proto + X-Forwarded-Host (set by reverse proxies), then
// falls back to the Origin header (set by browsers on cross-origin requests),
// and finally uses the scheme inferred from TLS + r.Host.
func requestBaseURL(r *http.Request) string {
	// Origin header (browsers set this on most cross-origin requests).
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

// ── Site settings ─────────────────────────────────────────────────────────────

var smtpSettingKeys = []string{
	"smtp_host", "smtp_port", "smtp_username", "smtp_password",
	"smtp_from_address", "smtp_from_name", "smtp_tls", "site_base_url",
}

// GET /admin/settings
func (s *Service) getSettings(w http.ResponseWriter, r *http.Request) {
	vals, err := s.db.GetSiteSettings(r.Context(), smtpSettingKeys)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Never return the password to the client.
	if _, ok := vals["smtp_password"]; ok {
		vals["smtp_password"] = "••••••••"
	}
	httputil.WriteOK(w, vals)
}

// PUT /admin/settings/smtp
func (s *Service) updateSmtpSettings(w http.ResponseWriter, r *http.Request) {
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		Host        string `json:"smtp_host"`
		Port        string `json:"smtp_port"`
		Username    string `json:"smtp_username"`
		Password    string `json:"smtp_password"`
		FromAddress string `json:"smtp_from_address"`
		FromName    string `json:"smtp_from_name"`
		TLS         bool   `json:"smtp_tls"`
		SiteBaseURL string `json:"site_base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	kvs := map[string]string{
		"smtp_host":         body.Host,
		"smtp_port":         body.Port,
		"smtp_username":     body.Username,
		"smtp_from_address": body.FromAddress,
		"smtp_from_name":    body.FromName,
		"smtp_tls":          strconv.FormatBool(body.TLS),
		"site_base_url":     body.SiteBaseURL,
	}
	// Only overwrite password if a non-placeholder value was sent.
	if body.Password != "" && body.Password != "••••••••" {
		kvs["smtp_password"] = body.Password
	}
	for k, v := range kvs {
		if err := s.db.SetSiteSetting(r.Context(), k, v); err != nil {
			httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "update_smtp_settings", "settings", "", nil); err != nil {
		slog.Warn("audit log failed", "action", "update_smtp_settings", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/settings/smtp/test — body: {"to": "admin@example.com"}
func (s *Service) testSmtp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.To == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "to address required")
		return
	}
	smtpKeys := []string{"smtp_host", "smtp_port", "smtp_username", "smtp_password",
		"smtp_from_address", "smtp_from_name", "smtp_tls"}
	cfg, err := s.db.GetSiteSettings(r.Context(), smtpKeys)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	m := mailer.New(mailer.Config{
		Host:        cfg["smtp_host"],
		Port:        cfg["smtp_port"],
		Username:    cfg["smtp_username"],
		Password:    cfg["smtp_password"],
		FromAddress: cfg["smtp_from_address"],
		FromName:    cfg["smtp_from_name"],
		TLS:         cfg["smtp_tls"] == "true",
	})
	if err := m.Validate(); err != nil {
		httputil.WriteErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := m.SendTest(r.Context(), body.To); err != nil {
		httputil.WriteErr(w, http.StatusBadGateway, "smtp error: "+err.Error())
		return
	}
	httputil.WriteOK(w, map[string]string{"status": "sent"})
}

// ── Webhooks ─────────────────────────────────────────────────────────────────

// GET /admin/webhooks/events — list all supported event types.
func (s *Service) listWebhookEvents(w http.ResponseWriter, r *http.Request) {
	httputil.WriteOK(w, webhook.AllEvents)
}

// GET /admin/webhooks
func (s *Service) listWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.db.ListWebhooks(r.Context())
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, hooks)
}

// POST /admin/webhooks — body: {url, secret, events, description}
func (s *Service) createWebhook(w http.ResponseWriter, r *http.Request) {
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		URL         string   `json:"url"`
		Secret      string   `json:"secret"`
		Events      []string `json:"events"`
		Description string   `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.URL == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "url required")
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "failed to generate id")
		return
	}
	id := hex.EncodeToString(raw)
	hook, err := s.db.CreateWebhook(r.Context(), store.CreateWebhookParams{
		ID:          id,
		URL:         body.URL,
		Secret:      body.Secret,
		Events:      body.Events,
		Description: body.Description,
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "create_webhook", "webhook", id,
		map[string]any{"url": body.URL, "events": body.Events}); err != nil {
		slog.Warn("audit log failed", "action", "create_webhook", "err", err)
	}
	w.WriteHeader(http.StatusCreated)
	httputil.WriteOK(w, hook)
}

// GET /admin/webhooks/{id}
func (s *Service) getWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	hook, err := s.db.GetWebhook(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, err.Error())
		return
	}
	httputil.WriteOK(w, hook)
}

// PUT /admin/webhooks/{id} — body: {url, secret, events, enabled, description}
func (s *Service) updateWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	var body struct {
		URL         string   `json:"url"`
		Secret      string   `json:"secret"`
		Events      []string `json:"events"`
		Enabled     bool     `json:"enabled"`
		Description string   `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	hook, err := s.db.UpdateWebhook(r.Context(), store.UpdateWebhookParams{
		ID:          id,
		URL:         body.URL,
		Secret:      body.Secret,
		Events:      body.Events,
		Enabled:     body.Enabled,
		Description: body.Description,
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "update_webhook", "webhook", id,
		map[string]any{"url": body.URL, "enabled": body.Enabled}); err != nil {
		slog.Warn("audit log failed", "action", "update_webhook", "err", err)
	}
	httputil.WriteOK(w, hook)
}

// DELETE /admin/webhooks/{id}
func (s *Service) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	if err := s.db.DeleteWebhook(r.Context(), id); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.db.InsertAuditLog(r.Context(), actorID, "delete_webhook", "webhook", id, nil); err != nil {
		slog.Warn("audit log failed", "action", "delete_webhook", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/webhooks/{id}/deliveries?limit=50
func (s *Service) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit := httputil.QueryInt(r, "limit", 50)
	deliveries, err := s.db.ListWebhookDeliveries(r.Context(), id, limit)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, deliveries)
}

// POST /admin/webhooks/{id}/test — sends a test event immediately.
func (s *Service) testWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	hook, err := s.db.GetWebhook(r.Context(), id)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "webhook not found")
		return
	}
	if s.dispatcher != nil {
		// Deliver directly to this specific webhook (bypass enabled/event filter).
		s.dispatcher.DispatchTo(r.Context(), hook, webhook.EventTest, map[string]any{
			"message": "Test event from Orb",
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

func tailFile(path string, maxLines int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	ring := make([]string, maxLines)
	count := 0
	pos := 0
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		ring[pos] = sc.Text()
		pos = (pos + 1) % maxLines
		count++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if count <= maxLines {
		return ring[:count], nil
	}
	out := make([]string, 0, maxLines)
	out = append(out, ring[pos:]...)
	out = append(out, ring[:pos]...)
	return out, nil
}

func runPgDump(ctx context.Context, dbURL, outPath string) error {
	bin := strings.TrimSpace(os.Getenv("PG_DUMP_BIN"))
	if bin == "" {
		bin = "pg_dump"
	}
	cmd := exec.CommandContext(ctx, bin,
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--file", outPath,
		dbURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runPgRestore(ctx context.Context, dbURL, dumpPath string) error {
	bin := strings.TrimSpace(os.Getenv("PG_RESTORE_BIN"))
	if bin == "" {
		bin = "pg_restore"
	}
	cmd := exec.CommandContext(ctx, bin,
		"--clean",
		"--if-exists",
		"--no-owner",
		"--no-privileges",
		"--exit-on-error",
		"--dbname", dbURL,
		dumpPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeTarBytes(tw *tar.Writer, name string, data []byte, modTime time.Time) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: modTime,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func writeTarFile(tw *tar.Writer, arcName, srcPath string) error {
	fi, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory", srcPath)
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	hdr := &tar.Header{
		Name:    arcName,
		Mode:    0o644,
		Size:    fi.Size(),
		ModTime: fi.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func writeTarDir(tw *tar.Writer, root, arcPrefix string, modTime time.Time) error {
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			hdr := &tar.Header{Name: arcPrefix + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: modTime}
			return tw.WriteHeader(hdr)
		}
		return err
	}

	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			hdr := &tar.Header{Name: arcPrefix + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: modTime}
			return tw.WriteHeader(hdr)
		}
		if isMediaObjectPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Do not include symlinks in backups.
			return nil
		}
		arcName := filepath.ToSlash(filepath.Join(arcPrefix, rel))
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			hdr := &tar.Header{Name: arcName + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: info.ModTime()}
			return tw.WriteHeader(hdr)
		}
		hdr := &tar.Header{
			Name:    arcName,
			Mode:    0o644,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, cpErr := io.Copy(tw, f)
		closeErr := f.Close()
		if cpErr != nil {
			return cpErr
		}
		return closeErr
	})
}

func saveIncomingBackupToTemp(r *http.Request) (string, error) {
	tmp, err := os.CreateTemp("", "orb-restore-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer func() { _ = tmp.Close() }()

	ct := strings.ToLower(r.Header.Get("Content-Type"))
	switch {
	case strings.HasPrefix(ct, "multipart/form-data"):
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return "", err
		}
		f, _, err := r.FormFile("backup")
		if err != nil {
			return "", err
		}
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(tmp, f); err != nil {
			return "", err
		}
	default:
		if _, err := io.Copy(tmp, r.Body); err != nil {
			return "", err
		}
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

func extractBackup(backupPath string) (dbDumpPath string, staticDir string, retErr error) {
	f, err := os.Open(backupPath)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	dbTmp, err := os.CreateTemp("", "orb-restore-db-*.dump")
	if err != nil {
		return "", "", err
	}
	dbTmpPath := dbTmp.Name()
	_ = dbTmp.Close()

	staticTmp, err := os.MkdirTemp("", "orb-restore-static-*")
	if err != nil {
		_ = os.Remove(dbTmpPath)
		return "", "", err
	}

	var foundDB bool
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = os.Remove(dbTmpPath)
			_ = os.RemoveAll(staticTmp)
			return "", "", err
		}
		name := filepath.ToSlash(filepath.Clean(hdr.Name))
		if name == "." || name == "/" {
			continue
		}

		switch {
		case name == "database.dump":
			out, err := os.OpenFile(dbTmpPath, os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", err
			}
			_, cpErr := io.Copy(out, tr)
			closeErr := out.Close()
			if cpErr != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", cpErr
			}
			if closeErr != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", closeErr
			}
			foundDB = true
		case strings.HasPrefix(name, "static/"):
			rel := strings.TrimPrefix(name, "static/")
			if rel == "" {
				continue
			}
			dest := filepath.Join(staticTmp, filepath.FromSlash(rel))
			cleaned := filepath.Clean(dest)
			staticRoot := filepath.Clean(staticTmp)
			if !strings.HasPrefix(cleaned, staticRoot+string(os.PathSeparator)) && cleaned != staticRoot {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", fmt.Errorf("invalid static path in archive: %s", name)
			}
			if hdr.FileInfo().IsDir() {
				if err := os.MkdirAll(cleaned, 0o755); err != nil {
					_ = os.Remove(dbTmpPath)
					_ = os.RemoveAll(staticTmp)
					return "", "", err
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(cleaned), 0o755); err != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", err
			}
			out, err := os.OpenFile(cleaned, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", err
			}
			_, cpErr := io.Copy(out, tr)
			closeErr := out.Close()
			if cpErr != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", cpErr
			}
			if closeErr != nil {
				_ = os.Remove(dbTmpPath)
				_ = os.RemoveAll(staticTmp)
				return "", "", closeErr
			}
		}
	}

	if !foundDB {
		_ = os.Remove(dbTmpPath)
		_ = os.RemoveAll(staticTmp)
		return "", "", fmt.Errorf("database.dump not found")
	}
	return dbTmpPath, staticTmp, nil
}

func restoreNonMediaContents(dstRoot, srcRoot string) error {
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return err
	}

	return filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if isMediaObjectPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		srcF, err := os.Open(path)
		if err != nil {
			return err
		}
		dstF, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			_ = srcF.Close()
			return err
		}
		_, cpErr := io.Copy(dstF, srcF)
		srcCloseErr := srcF.Close()
		closeErr := dstF.Close()
		if cpErr != nil {
			return cpErr
		}
		if srcCloseErr != nil {
			return srcCloseErr
		}
		return closeErr
	})
}

func isMediaObjectPath(rel string) bool {
	p := filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(rel), "./"))
	return strings.HasPrefix(p, "audio/") ||
		strings.HasPrefix(p, "audiobooks/") ||
		strings.HasPrefix(p, "audiobook-chapters/")
}

// dispatch fires an event if a dispatcher is configured.
func (s *Service) dispatch(ctx context.Context, event string, data any) {
	if s.dispatcher != nil {
		s.dispatcher.Dispatch(ctx, event, data)
	}
}

// derefOr safely dereferences a pointer, returning fallback if the pointer is nil.
func derefOr[T any](p *T, fallback T) T {
	if p != nil {
		return *p
	}
	return fallback
}

// storeCoverArt stores a cover art image, re-encoding as JPEG for consistency.
func storeCoverArt(ctx context.Context, obj objstore.ObjectStore, key string, data []byte) error {
	encoded, err := encodeImageToJPEG(ctx, data)
	if err != nil {
		return fmt.Errorf("decode cover art: %w", err)
	}
	if len(encoded) == 0 {
		return errors.New("encode cover art: empty output")
	}
	sum := sha256.Sum256(encoded)
	if err := obj.Put(ctx, key, bytes.NewReader(encoded), int64(len(encoded))); err != nil {
		return err
	}
	if sz, err := obj.Size(ctx, key); err == nil && sz != int64(len(encoded)) {
		return fmt.Errorf("cover art size mismatch after write: wrote=%d stored=%d sha256=%s", len(encoded), sz, hex.EncodeToString(sum[:]))
	}
	return nil
}

// encodeImageToJPEG decodes any supported image format and re-encodes as JPEG.
// For formats unsupported by Go's image package (e.g. AVIF), it falls back to
// ffmpeg if available.
func encodeImageToJPEG(ctx context.Context, data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		var buf bytes.Buffer
		if encErr := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); encErr != nil {
			return nil, encErr
		}
		return buf.Bytes(), nil
	}
	// Fallback: convert via ffmpeg (handles AVIF, WebP, TIFF, etc.)
	if _, lookErr := exec.LookPath("ffmpeg"); lookErr != nil {
		return nil, err // return original decode error
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-vframes", "1",
		"-f", "image2", "-vcodec", "mjpeg", "-q:v", "2",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(data)
	out, ffErr := cmd.Output()
	if ffErr != nil || len(out) == 0 {
		return nil, err // return original decode error
	}
	return out, nil
}

func findLocalAlbumCover(paths []string) []byte {
	seenDirs := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if b := embeddedCoverFromAudio(p); len(b) > 0 {
			return b
		}
		dir := filepath.Dir(p)
		if _, seen := seenDirs[dir]; seen {
			continue
		}
		seenDirs[dir] = struct{}{}
		if b := bestFolderImage(dir); len(b) > 0 {
			return b
		}
	}
	return nil
}

func embeddedCoverFromAudio(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil
	}
	if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
		return pic.Data
	}
	return nil
}

func bestFolderImage(dir string) []byte {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var bestData []byte
	bestDelta := int(^uint(0) >> 1)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, ".jpeg") && !strings.HasSuffix(name, ".png") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil || len(b) == 0 {
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			continue
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		if delta := abs(w - h); delta < bestDelta {
			bestDelta = delta
			bestData = b
		}
	}
	return bestData
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
