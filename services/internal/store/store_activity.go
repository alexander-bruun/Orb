package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// ── Activity feed ─────────────────────────────────────────────────────────────

// InsertActivity records a single activity event.
func (s *Store) InsertActivity(ctx context.Context, p InsertActivityParams) error {
	meta, err := json.Marshal(p.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO user_activity (id, user_id, type, entity_type, entity_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.UserID, p.Type, p.EntityType, p.EntityID, meta)
	return err
}

// GetFeedForUser returns the activity of users that userID follows, newest first.
func (s *Store) GetFeedForUser(ctx context.Context, userID string, limit, offset int) ([]ActivityRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.user_id, u.username, u.display_name, u.avatar_key,
		       a.type, a.entity_type, a.entity_id, a.metadata, a.created_at
		FROM user_activity a
		JOIN users u ON u.id = a.user_id
		WHERE a.user_id IN (
		  SELECT followee_id FROM user_follows WHERE follower_id = $1
		)
		AND (
		  a.type != 'play'
		  OR u.profile_public = TRUE
		)
		ORDER BY a.created_at DESC
		LIMIT $2 OFFSET $3`,
		userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityRows(rows)
}

// GetActivityForUser returns a single user's own activity, newest first.
func (s *Store) GetActivityForUser(ctx context.Context, targetUserID string, limit, offset int) ([]ActivityRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.user_id, u.username, u.display_name, u.avatar_key,
		       a.type, a.entity_type, a.entity_id, a.metadata, a.created_at
		FROM user_activity a
		JOIN users u ON u.id = a.user_id
		WHERE a.user_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2 OFFSET $3`,
		targetUserID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityRows(rows)
}

// HasAnyActivity returns true if there is at least one activity row (used to
// show/hide the Feed nav link when only one user exists).
func (s *Store) HasAnyActivity(ctx context.Context) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_activity LIMIT 1`).Scan(&count)
	return count > 0, err
}

func scanActivityRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]ActivityRow, error) {
	var out []ActivityRow
	for rows.Next() {
		var a ActivityRow
		var displayName, avatarKey sql.NullString
		var metaRaw []byte
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Username, &displayName, &avatarKey,
			&a.Type, &a.EntityType, &a.EntityID, &metaRaw, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		if displayName.Valid {
			a.DisplayName = &displayName.String
		}
		if avatarKey.Valid {
			a.AvatarKey = &avatarKey.String
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &a.Metadata)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ── User follows ──────────────────────────────────────────────────────────────

// FollowUser inserts a follow relationship.
func (s *Store) FollowUser(ctx context.Context, followerID, followeeID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_follows (follower_id, followee_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		followerID, followeeID)
	return err
}

// UnfollowUser removes a follow relationship.
func (s *Store) UnfollowUser(ctx context.Context, followerID, followeeID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_follows WHERE follower_id = $1 AND followee_id = $2`,
		followerID, followeeID)
	return err
}

// IsFollowing returns whether followerID follows followeeID.
func (s *Store) IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_follows WHERE follower_id = $1 AND followee_id = $2`,
		followerID, followeeID).Scan(&count)
	return count > 0, err
}

// ListFollowers returns users that follow userID.
func (s *Store) ListFollowers(ctx context.Context, userID string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.email, u.password_hash, u.created_at,
		       u.last_login_at, u.totp_secret, u.totp_enabled, u.totp_backup_codes
		FROM users u
		JOIN user_follows uf ON uf.follower_id = u.id
		WHERE uf.followee_id = $1
		ORDER BY uf.followed_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSocialUsers(rows)
}

// ListFollowing returns users that userID follows.
func (s *Store) ListFollowing(ctx context.Context, userID string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.email, u.password_hash, u.created_at,
		       u.last_login_at, u.totp_secret, u.totp_enabled, u.totp_backup_codes
		FROM users u
		JOIN user_follows uf ON uf.followee_id = u.id
		WHERE uf.follower_id = $1
		ORDER BY uf.followed_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSocialUsers(rows)
}

func scanSocialUsers(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]User, error) {
	var out []User
	for rows.Next() {
		var u User
		var lastLogin sql.NullTime
		var totpSecret, totpBackup sql.NullString
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt,
			&lastLogin, &totpSecret, &u.TOTPEnabled, &totpBackup,
		); err != nil {
			return nil, err
		}
		if lastLogin.Valid {
			u.LastLoginAt = &lastLogin.Time
		}
		if totpSecret.Valid {
			u.TOTPSecret = &totpSecret.String
		}
		if totpBackup.Valid {
			u.TOTPBackupCodes = &totpBackup.String
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ── Public profiles ───────────────────────────────────────────────────────────

// GetPublicProfile returns a public profile by username.
// requirePublic=true enforces profile_public=TRUE (for strangers).
// requirePublic=false is for the profile owner viewing their own page.
func (s *Store) GetPublicProfile(ctx context.Context, username string, requirePublic bool) (*PublicProfile, error) {
	q := `
		SELECT u.id, u.username,
		       COALESCE(u.display_name, ''), COALESCE(u.bio, ''), u.avatar_key,
		       u.created_at, u.profile_public,
		       (SELECT COUNT(*) FROM user_follows WHERE followee_id = u.id),
		       (SELECT COUNT(*) FROM user_follows WHERE follower_id = u.id),
		       (SELECT COUNT(*) FROM playlists WHERE user_id = u.id AND is_public = TRUE)
		FROM users u
		WHERE u.username = $1`
	if requirePublic {
		q += ` AND u.profile_public = TRUE`
	}
	var p PublicProfile
	var displayName, bio, avatarKey sql.NullString
	var profilePublic bool
	err := s.pool.QueryRow(ctx, q, username).Scan(
		&p.ID, &p.Username, &displayName, &bio, &avatarKey,
		&p.JoinedAt, &profilePublic,
		&p.FollowerCount, &p.FollowingCount, &p.PlaylistCount,
	)
	if err != nil {
		return nil, err
	}
	p.DisplayName = displayName.String
	p.Bio = bio.String
	p.ProfilePublic = profilePublic
	if avatarKey.Valid {
		p.AvatarKey = &avatarKey.String
	}
	return &p, nil
}

// GetUserPublicPlaylists returns public playlists for a user.
func (s *Store) GetUserPublicPlaylists(ctx context.Context, userID string) ([]Playlist, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, description, cover_art_key, created_at
		FROM playlists
		WHERE user_id = $1 AND is_public = TRUE
		ORDER BY updated_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlaylists(rows)
}

// GetUserPublicStats returns aggregate listening stats for a user.
func (s *Store) GetUserPublicStats(ctx context.Context, userID string) (*UserStats, error) {
	var stats UserStats

	// total plays and duration
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(duration_played_ms), 0)
		FROM play_history
		WHERE user_id = $1`,
		userID).Scan(&stats.TotalPlays, &stats.TotalPlayedMs)
	if err != nil {
		return nil, err
	}

	// top artists
	rows, err := s.pool.Query(ctx, `
		SELECT ar.id, ar.name, COUNT(*) AS plays
		FROM play_history ph
		JOIN tracks t ON t.id = ph.track_id
		JOIN artists ar ON ar.id = t.artist_id
		WHERE ph.user_id = $1
		GROUP BY ar.id, ar.name
		ORDER BY plays DESC
		LIMIT 5`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a TopArtistStat
		if err := rows.Scan(&a.ArtistID, &a.ArtistName, &a.Plays); err != nil {
			return nil, err
		}
		stats.TopArtists = append(stats.TopArtists, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &stats, nil
}

// UpdateUserProfile updates profile fields for a user.
func (s *Store) UpdateUserProfile(ctx context.Context, userID, displayName, bio string, profilePublic bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET display_name = $2, bio = $3, profile_public = $4
		WHERE id = $1`,
		userID, displayName, bio, profilePublic)
	return err
}

// SetUserAvatar stores the avatar object key for a user.
func (s *Store) SetUserAvatar(ctx context.Context, userID, avatarKey string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET avatar_key = $2 WHERE id = $1`,
		userID, avatarKey)
	return err
}

// GetUserProfileFields returns the mutable profile fields for the settings page.
func (s *Store) GetUserProfileFields(ctx context.Context, userID string) (displayName, bio string, profilePublic bool, avatarKey *string, err error) {
	var dn, b sql.NullString
	var ak sql.NullString
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name,''), COALESCE(bio,''), profile_public, avatar_key FROM users WHERE id = $1`,
		userID).Scan(&dn, &b, &profilePublic, &ak)
	displayName = dn.String
	bio = b.String
	if ak.Valid {
		avatarKey = &ak.String
	}
	return
}
