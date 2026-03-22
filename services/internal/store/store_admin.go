package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── Admin analytics ───────────────────────────────────────────────────────

// GetAdminSummary returns aggregate server statistics.
func (s *Store) GetAdminSummary(ctx context.Context) (AdminSummary, error) {
	var sum AdminSummary
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users)                                        AS total_users,
			(SELECT COUNT(*) FROM users WHERE is_active = TRUE)                 AS active_users,
			(SELECT COUNT(*) FROM tracks)                                       AS total_tracks,
			(SELECT COUNT(*) FROM albums)                                       AS total_albums,
			(SELECT COUNT(*) FROM artists)                                      AS total_artists,
			(SELECT COUNT(*) FROM play_history)                                 AS total_plays,
			(SELECT COALESCE(SUM(duration_played_ms),0) FROM play_history)      AS total_played_ms,
			(SELECT COALESCE(SUM(file_size),0) FROM tracks)                     AS total_size_bytes,
			(SELECT COUNT(*) FROM albums WHERE cover_art_key IS NULL)           AS albums_no_cover_art
	`).Scan(&sum.TotalUsers, &sum.ActiveUsers, &sum.TotalTracks, &sum.TotalAlbums,
		&sum.TotalArtists, &sum.TotalPlays, &sum.TotalPlayedMs,
		&sum.TotalSizeBytes, &sum.AlbumsNoCoverArt)
	return sum, err
}

// ListUsersWithStats returns all users ordered by play count descending.
func (s *Store) ListUsersWithStats(ctx context.Context) ([]UserPlayStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			u.id, u.username, u.email, u.is_admin, u.is_active,
			u.storage_quota_bytes, u.email_verified,
			COUNT(ph.id) AS play_count,
			u.last_login_at,
			u.created_at
		FROM users u
		LEFT JOIN play_history ph ON ph.user_id = u.id
		GROUP BY u.id
		ORDER BY play_count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]UserPlayStat, 0)
	for rows.Next() {
		var s UserPlayStat
		if err := rows.Scan(&s.UserID, &s.Username, &s.Email, &s.IsAdmin, &s.IsActive,
			&s.StorageQuotaBytes, &s.EmailVerified, &s.PlayCount, &s.LastLoginAt, &s.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetTopTracks returns the most-played tracks.
func (s *Store) GetTopTracks(ctx context.Context, limit int) ([]TrackPlayCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id, t.title,
			a.name  AS artist_name,
			al.title AS album_title,
			COUNT(ph.id) AS plays
		FROM tracks t
		LEFT JOIN play_history ph ON ph.track_id = t.id
		LEFT JOIN artists a  ON a.id  = t.artist_id
		LEFT JOIN albums  al ON al.id = t.album_id
		GROUP BY t.id, a.name, al.title
		ORDER BY plays DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]TrackPlayCount, 0)
	for rows.Next() {
		var r TrackPlayCount
		if err := rows.Scan(&r.TrackID, &r.Title, &r.ArtistName, &r.AlbumTitle, &r.Plays); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetTopArtists returns artists ordered by total play count.
func (s *Store) GetTopArtists(ctx context.Context, limit int) ([]ArtistPlayCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			ar.id, ar.name,
			COUNT(ph.id) AS plays
		FROM artists ar
		JOIN tracks t   ON t.artist_id = ar.id
		LEFT JOIN play_history ph ON ph.track_id = t.id
		GROUP BY ar.id, ar.name
		ORDER BY plays DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]ArtistPlayCount, 0)
	for rows.Next() {
		var r ArtistPlayCount
		if err := rows.Scan(&r.ArtistID, &r.Name, &r.Plays); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetPlaysByDay returns daily play counts for the last n days.
func (s *Store) GetPlaysByDay(ctx context.Context, days int) ([]DailyPlayCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			to_char(d::date, 'YYYY-MM-DD') AS date,
			COUNT(ph.id)                   AS plays
		FROM generate_series(
			(CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'),
			CURRENT_DATE,
			INTERVAL '1 day'
		) AS d
		LEFT JOIN play_history ph
			ON ph.played_at::date = d::date
		GROUP BY d
		ORDER BY d
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]DailyPlayCount, 0)
	for rows.Next() {
		var r DailyPlayCount
		if err := rows.Scan(&r.Date, &r.Plays); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ── Admin user management ─────────────────────────────────────────────────

// SetUserAdmin sets the is_admin flag for a user.
func (s *Store) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET is_admin = $1 WHERE id = $2`, isAdmin, userID)
	return err
}

// CountUsers returns the total number of registered users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// SetUserActive enables or disables a user account.
func (s *Store) SetUserActive(ctx context.Context, userID string, active bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET is_active = $1 WHERE id = $2`, active, userID)
	return err
}

// DeleteUser removes a user and all their data (cascading FK deletes handle owned rows).
func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

// SetUserQuota sets or clears (nil = unlimited) the storage quota for a user.
func (s *Store) SetUserQuota(ctx context.Context, userID string, quotaBytes *int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET storage_quota_bytes = $1 WHERE id = $2`, quotaBytes, userID)
	return err
}

// ── Invite tokens ─────────────────────────────────────────────────────────

// CreateInviteToken inserts a new invite token.
func (s *Store) CreateInviteToken(ctx context.Context, token, email, createdBy string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO invite_tokens (token, email, created_by, expires_at)
		VALUES ($1, $2, $3, $4)
	`, token, email, createdBy, expiresAt)
	return err
}

// GetInviteToken retrieves a single invite token row.
func (s *Store) GetInviteToken(ctx context.Context, token string) (*InviteToken, error) {
	var t InviteToken
	err := s.pool.QueryRow(ctx, `
		SELECT token, email, created_by, created_at, expires_at, used_at, used_by
		FROM invite_tokens WHERE token = $1
	`, token).Scan(&t.Token, &t.Email, &t.CreatedBy, &t.CreatedAt, &t.ExpiresAt, &t.UsedAt, &t.UsedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

// ListInviteTokens returns all invite tokens ordered by creation time descending.
func (s *Store) ListInviteTokens(ctx context.Context) ([]InviteToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT token, email, created_by, created_at, expires_at, used_at, used_by
		FROM invite_tokens ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]InviteToken, 0)
	for rows.Next() {
		var t InviteToken
		if err := rows.Scan(&t.Token, &t.Email, &t.CreatedBy, &t.CreatedAt, &t.ExpiresAt, &t.UsedAt, &t.UsedBy); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// RevokeInviteToken deletes an unused invite token.
func (s *Store) RevokeInviteToken(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM invite_tokens WHERE token = $1 AND used_at IS NULL`, token)
	return err
}

// ConsumeInviteToken marks an invite as used.
func (s *Store) ConsumeInviteToken(ctx context.Context, token, usedBy string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE invite_tokens SET used_at = now(), used_by = $1
		WHERE token = $2 AND used_at IS NULL
	`, usedBy, token)
	return err
}

// ── Audit log ─────────────────────────────────────────────────────────────

// InsertAuditLog records an admin or system action.
func (s *Store) InsertAuditLog(ctx context.Context, actorID, action, targetType, targetID string, detail any) error {
	var detailJSON []byte
	if detail != nil {
		var err error
		detailJSON, err = json.Marshal(detail)
		if err != nil {
			return err
		}
	}
	var actor *string
	if actorID != "" {
		actor = &actorID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, action, target_type, target_id, detail)
		VALUES ($1, $2, $3, $4, $5)
	`, actor, action, targetType, targetID, detailJSON)
	return err
}

// ListAuditLogs returns paginated audit log entries newest-first, optionally joined with actor username.
func (s *Store) ListAuditLogs(ctx context.Context, limit, offset int) ([]AuditLog, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT al.id, al.actor_id, u.username, al.action, al.target_type, al.target_id, al.detail, al.created_at
		FROM audit_logs al
		LEFT JOIN users u ON u.id = al.actor_id
		ORDER BY al.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	results := make([]AuditLog, 0)
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.ActorID, &l.ActorName, &l.Action,
			&l.TargetType, &l.TargetID, &l.Detail, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		results = append(results, l)
	}
	return results, total, rows.Err()
}

// ── Site settings ─────────────────────────────────────────────────────────

// GetSiteSetting retrieves a single setting value by key. Returns "" if not found.
func (s *Store) GetSiteSetting(ctx context.Context, key string) (string, error) {
	var val string
	err := s.pool.QueryRow(ctx, `SELECT value FROM site_settings WHERE key = $1`, key).Scan(&val)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return val, err
}

// SetSiteSetting upserts a site setting.
func (s *Store) SetSiteSetting(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO site_settings (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, key, value)
	return err
}

// GetSiteSettings retrieves multiple settings in one query. Returns a map (missing keys absent).
func (s *Store) GetSiteSettings(ctx context.Context, keys []string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value FROM site_settings WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string, len(keys))
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// ── Storage / artwork ─────────────────────────────────────────────────────

// GetStorageStats returns disk usage broken down by audio format.
func (s *Store) GetStorageStats(ctx context.Context) (StorageStats, error) {
	ss := StorageStats{ByFormat: make([]FormatStat, 0)}
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(file_size),0), COUNT(*) FROM tracks
	`).Scan(&ss.TotalSizeBytes, &ss.TrackCount); err != nil {
		return ss, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT format, COUNT(*) AS cnt, COALESCE(SUM(file_size),0) AS sz
		FROM tracks GROUP BY format ORDER BY sz DESC
	`)
	if err != nil {
		return ss, err
	}
	defer rows.Close()
	for rows.Next() {
		var f FormatStat
		if err := rows.Scan(&f.Format, &f.Count, &f.SizeBytes); err != nil {
			return ss, err
		}
		ss.ByFormat = append(ss.ByFormat, f)
	}
	return ss, rows.Err()
}

// ListAlbumsWithoutCover returns albums that have no cover_art_key, paginated.
func (s *Store) ListAlbumsWithoutCover(ctx context.Context, limit, offset int) ([]Album, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM albums WHERE cover_art_key IS NULL`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT al.id, al.artist_id, ar.name AS artist_name, al.title, al.release_year,
		       al.mbid, al.album_type, al.created_at
		FROM albums al
		LEFT JOIN artists ar ON ar.id = al.artist_id
		WHERE al.cover_art_key IS NULL
		ORDER BY al.title
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	results := make([]Album, 0)
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.ArtistName, &a.Title, &a.ReleaseYear,
			&a.Mbid, &a.AlbumType, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		results = append(results, a)
	}
	return results, total, rows.Err()
}
