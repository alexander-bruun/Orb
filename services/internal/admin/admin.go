// Package admin provides analytics and administration endpoints (admin-only).
package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/mailer"
	"github.com/alexander-bruun/orb/services/internal/musicbrainz"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

// Service handles admin HTTP routes.
type Service struct {
	db  *store.Store
	obj objstore.ObjectStore
	mb  *musicbrainz.Client
	kv  *redis.Client // optional; used to invalidate sessions on deactivation
}

// New returns a new admin Service.
func New(db *store.Store, obj objstore.ObjectStore, mb *musicbrainz.Client, kv *redis.Client) *Service {
	return &Service{db: db, obj: obj, mb: mb, kv: kv}
}

// AdminMiddleware rejects requests from non-admin users and inactive users.
func (s *Service) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromCtx(r.Context())
		if userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := s.db.GetUserByID(r.Context(), userID)
		if err != nil || !u.IsAdmin || !u.IsActive {
			http.Error(w, "forbidden", http.StatusForbidden)
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

	// Library / job control
	r.Post("/albums/{id}/refetch-cover", s.refetchAlbumCover)
	r.Get("/albums/no-cover", s.albumsNoCover)

	// Site settings
	r.Get("/settings", s.getSettings)
	r.Put("/settings/smtp", s.updateSmtpSettings)
	r.Post("/settings/smtp/test", s.testSmtp)
}

// ── Analytics ────────────────────────────────────────────────────────────────

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

// GET /admin/storage
func (s *Service) storageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStorageStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
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
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.db.SetUserAdmin(r.Context(), targetID, body.IsAdmin); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "set_admin", "user", targetID,
		map[string]any{"is_admin": body.IsAdmin})
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
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.db.SetUserActive(r.Context(), targetID, body.Active); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Immediately invalidate any active session so the user is logged out.
	if !body.Active && s.kv != nil {
		_ = s.kv.Del(r.Context(), kvkeys.Session(targetID))
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "set_active", "user", targetID,
		map[string]any{"active": body.Active})
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
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.db.SetUserQuota(r.Context(), targetID, body.QuotaBytes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "set_quota", "user", targetID,
		map[string]any{"quota_bytes": body.QuotaBytes})
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /admin/users/{id}
func (s *Service) deleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())
	if targetID == actorID {
		http.Error(w, "cannot delete your own account", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteUser(r.Context(), targetID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "delete_user", "user", targetID, nil)
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
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}

	// Generate a secure 32-byte token.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	if err := s.db.CreateInviteToken(r.Context(), token, body.Email, actorID, expiresAt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "create_invite", "invite", token,
		map[string]any{"email": body.Email, "expires_at": expiresAt})

	// Try to send invite email if SMTP is configured (best-effort).
	smtpKeys := []string{"site_base_url", "smtp_host", "smtp_port", "smtp_username", "smtp_password",
		"smtp_from_address", "smtp_from_name", "smtp_tls"}
	cfg, _ := s.db.GetSiteSettings(r.Context(), smtpKeys)
	siteURL := cfg["site_base_url"]
	if siteURL == "" {
		http.Error(w, "site_base_url is not configured", http.StatusInternalServerError)
		return
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

	writeJSON(w, map[string]string{
		"token":      token,
		"invite_url": inviteURL,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// GET /admin/invites
func (s *Service) listInvites(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.db.ListInviteTokens(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tokens)
}

// DELETE /admin/invites/{token}
func (s *Service) revokeInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	actorID := auth.UserIDFromCtx(r.Context())
	if err := s.db.RevokeInviteToken(r.Context(), token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "revoke_invite", "invite", token, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ── Audit log ────────────────────────────────────────────────────────────────

// GET /admin/audit-logs?limit=50&offset=0
func (s *Service) auditLogs(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	logs, total, err := s.db.ListAuditLogs(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"logs": logs, "total": total})
}

// ── Artwork ───────────────────────────────────────────────────────────────────

// GET /admin/albums/no-cover?limit=50&offset=0
func (s *Service) albumsNoCover(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	albums, total, err := s.db.ListAlbumsWithoutCover(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"albums": albums, "total": total})
}

// POST /admin/albums/{id}/refetch-cover
func (s *Service) refetchAlbumCover(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	actorID := auth.UserIDFromCtx(r.Context())

	if s.mb == nil {
		http.Error(w, "MusicBrainz client not configured", http.StatusServiceUnavailable)
		return
	}

	album, err := s.db.GetAlbumByID(r.Context(), albumID)
	if err != nil {
		http.Error(w, "album not found", http.StatusNotFound)
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
		artistName := ""
		if album.ArtistName != nil {
			artistName = *album.ArtistName
		}
		enrichment, searchErr := s.mb.EnrichAlbum(r.Context(), album.Title, artistName)
		if searchErr == nil && enrichment != nil && enrichment.ReleaseGroupMbid != "" {
			imgData, err = s.mb.FetchAlbumCoverArt(r.Context(), enrichment.ReleaseGroupMbid)
			if err != nil {
				slog.Warn("admin: refetch cover via search failed", "album", albumID, "err", err)
			}
		}
	}

	if imgData == nil {
		http.Error(w, "no cover art found", http.StatusNotFound)
		return
	}

	coverKey := fmt.Sprintf("covers/%s.jpg", albumID)
	if err := storeCoverArt(r.Context(), s.obj, coverKey, imgData); err != nil {
		http.Error(w, "failed to store cover art: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.db.UpdateAlbumCoverArt(r.Context(), albumID, coverKey); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "refetch_cover", "album", albumID, nil)
	writeJSON(w, map[string]string{"cover_art_key": coverKey})
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Never return the password to the client.
	if _, ok := vals["smtp_password"]; ok {
		vals["smtp_password"] = "••••••••"
	}
	writeJSON(w, vals)
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
		http.Error(w, "invalid body", http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	_ = s.db.InsertAuditLog(r.Context(), actorID, "update_smtp_settings", "settings", "", nil)
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/settings/smtp/test — body: {"to": "admin@example.com"}
func (s *Service) testSmtp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.To == "" {
		http.Error(w, "to address required", http.StatusBadRequest)
		return
	}
	smtpKeys := []string{"smtp_host", "smtp_port", "smtp_username", "smtp_password",
		"smtp_from_address", "smtp_from_name", "smtp_tls"}
	cfg, err := s.db.GetSiteSettings(r.Context(), smtpKeys)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := m.SendTest(r.Context(), body.To); err != nil {
		http.Error(w, "smtp error: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func intQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
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

// storeCoverArt stores a cover art image, re-encoding as JPEG for consistency.
func storeCoverArt(ctx context.Context, obj objstore.ObjectStore, key string, data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// Not a decodable image format — store raw bytes.
		return obj.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(jpeg.Encode(pw, img, &jpeg.Options{Quality: 90}))
	}()
	defer pr.Close()
	return obj.Put(ctx, key, pr, -1)
}

