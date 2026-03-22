package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── User CRUD ──────────────────────────────────────────────────────────────

// GetUserByID returns a user by ID.
func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	var u User
	row := s.pool.QueryRow(ctx, `SELECT id, username, email, password_hash, created_at, last_login_at, totp_secret, totp_enabled, totp_backup_codes, is_admin, is_active, email_verified FROM users WHERE id = $1`, id)
	var lastLoginAt sql.NullTime
	var totpSecret, totpBackupCodes sql.NullString
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &lastLoginAt, &totpSecret, &u.TOTPEnabled, &totpBackupCodes, &u.IsAdmin, &u.IsActive, &u.EmailVerified)
	if lastLoginAt.Valid {
		u.LastLoginAt = &lastLoginAt.Time
	}
	if totpSecret.Valid {
		u.TOTPSecret = &totpSecret.String
	}
	if totpBackupCodes.Valid {
		u.TOTPBackupCodes = &totpBackupCodes.String
	}
	return u, err
}

// HasAnyUser returns true if at least one user exists in the database.
func (s *Store) HasAnyUser(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users LIMIT 1)`).Scan(&exists)
	return exists, err
}

// CreateUser inserts a new user.
func (s *Store) CreateUser(ctx context.Context, p CreateUserParams) (User, error) {
	var u User
	row := s.pool.QueryRow(ctx, `INSERT INTO users (id, username, email, password_hash, created_at) VALUES ($1, $2, $3, $4, now()) RETURNING id, username, email, password_hash, created_at`, p.ID, p.Username, p.Email, p.PasswordHash)
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

// GetUserByEmail returns a user by email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	row := s.pool.QueryRow(ctx, `SELECT id, username, email, password_hash, created_at, last_login_at, totp_secret, totp_enabled, totp_backup_codes, is_admin, is_active, email_verified FROM users WHERE email = $1`, email)
	var lastLoginAt sql.NullTime
	var totpSecret, totpBackupCodes sql.NullString
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &lastLoginAt, &totpSecret, &u.TOTPEnabled, &totpBackupCodes, &u.IsAdmin, &u.IsActive, &u.EmailVerified)
	if lastLoginAt.Valid {
		u.LastLoginAt = &lastLoginAt.Time
	}
	if totpSecret.Valid {
		u.TOTPSecret = &totpSecret.String
	}
	if totpBackupCodes.Valid {
		u.TOTPBackupCodes = &totpBackupCodes.String
	}
	return u, err
}

// ── Email verification ────────────────────────────────────────────────────

// SetEmailVerificationToken stores a verification token for the user (used when sending verification emails).
func (s *Store) SetEmailVerificationToken(ctx context.Context, userID, token string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET email_verification_token = $2, email_verification_expires_at = $3, email_verified = FALSE WHERE id = $1`,
		userID, token, expiresAt)
	return err
}

// VerifyEmailToken looks up the token, checks expiry, marks the user verified, and clears the token.
// Returns the user ID on success, or an error if the token is invalid or expired.
func (s *Store) VerifyEmailToken(ctx context.Context, token string) (string, error) {
	var userID string
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, email_verification_expires_at FROM users WHERE email_verification_token = $1 AND email_verified = FALSE`,
		token).Scan(&userID, &expiresAt)
	if err != nil {
		return "", err
	}
	if time.Now().After(expiresAt) {
		return "", errors.New("verification token expired")
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE users SET email_verified = TRUE, email_verification_token = NULL, email_verification_expires_at = NULL WHERE id = $1`,
		userID)
	return userID, err
}

// ResetEmailVerification clears the verified flag and any pending token for a user (called on email change).
func (s *Store) ResetEmailVerification(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET email_verified = FALSE, email_verification_token = NULL, email_verification_expires_at = NULL WHERE id = $1`,
		userID)
	return err
}

// ── TOTP ──────────────────────────────────────────────────────────────────

// SetTOTPSecret stores an unconfirmed TOTP secret for a user (not yet enabled).
func (s *Store) SetTOTPSecret(ctx context.Context, userID, secret string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET totp_secret = $2 WHERE id = $1`, userID, secret)
	return err
}

// EnableTOTP marks TOTP as enabled and stores the hashed backup codes.
func (s *Store) EnableTOTP(ctx context.Context, userID, backupCodesJSON string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET totp_enabled = TRUE, totp_backup_codes = $2 WHERE id = $1`, userID, backupCodesJSON)
	return err
}

// DisableTOTP clears all TOTP data for a user.
func (s *Store) DisableTOTP(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET totp_enabled = FALSE, totp_secret = NULL, totp_backup_codes = NULL WHERE id = $1`, userID)
	return err
}

// UpdateTOTPBackupCodes replaces the stored backup codes (after one is consumed).
func (s *Store) UpdateTOTPBackupCodes(ctx context.Context, userID, backupCodesJSON string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET totp_backup_codes = $2 WHERE id = $1`, userID, backupCodesJSON)
	return err
}

// ── Auth helpers ───────────────────────────────────────────────────────────

// UpdateLastLogin updates the last login time for a user.
func (s *Store) UpdateLastLogin(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
	return err
}

// UpdateUserPassword sets a new bcrypt password hash for a user.
func (s *Store) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash)
	return err
}

// UpdateUserEmail sets a new email for a user.
func (s *Store) UpdateUserEmail(ctx context.Context, userID, email string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET email = $2 WHERE id = $1`, userID, email)
	return err
}

// GetUserByIDFull returns a full user row including is_active (for middleware checks).
func (s *Store) GetUserIsActive(ctx context.Context, userID string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx,
		`SELECT is_active FROM users WHERE id = $1`, userID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

// ── Streaming preferences ─────────────────────────────────────────────────

// GetUserStreamingPrefs returns the streaming preferences for a user.
// If no prefs exist, returns a zero-value struct (all limits nil).
func (s *Store) GetUserStreamingPrefs(ctx context.Context, userID string) (UserStreamingPrefs, error) {
	var p UserStreamingPrefs
	p.UserID = userID
	row := s.pool.QueryRow(ctx,
		`SELECT max_bitrate_kbps, max_sample_rate, max_bit_depth,
		        wifi_max_bitrate_kbps, wifi_max_sample_rate, wifi_max_bit_depth,
		        mobile_max_bitrate_kbps, mobile_max_sample_rate, mobile_max_bit_depth,
		        transcode_format, wifi_transcode_format, mobile_transcode_format,
		        updated_at
		   FROM user_streaming_prefs WHERE user_id = $1`, userID)
	var (
		maxBitrate, maxSR, maxBD                   sql.NullInt64
		wifiMaxBitrate, wifiMaxSR, wifiMaxBD       sql.NullInt64
		mobileMaxBitrate, mobileMaxSR, mobileMaxBD sql.NullInt64
		transcodeFmt, wifiTranscodeFmt, mobileTranscodeFmt sql.NullString
		updatedAt                                  time.Time
	)
	err := row.Scan(
		&maxBitrate, &maxSR, &maxBD,
		&wifiMaxBitrate, &wifiMaxSR, &wifiMaxBD,
		&mobileMaxBitrate, &mobileMaxSR, &mobileMaxBD,
		&transcodeFmt, &wifiTranscodeFmt, &mobileTranscodeFmt,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if maxBitrate.Valid {
		v := int(maxBitrate.Int64)
		p.MaxBitrateKbps = &v
	}
	if maxSR.Valid {
		v := int(maxSR.Int64)
		p.MaxSampleRate = &v
	}
	if maxBD.Valid {
		v := int(maxBD.Int64)
		p.MaxBitDepth = &v
	}
	if wifiMaxBitrate.Valid {
		v := int(wifiMaxBitrate.Int64)
		p.WifiMaxBitrateKbps = &v
	}
	if wifiMaxSR.Valid {
		v := int(wifiMaxSR.Int64)
		p.WifiMaxSampleRate = &v
	}
	if wifiMaxBD.Valid {
		v := int(wifiMaxBD.Int64)
		p.WifiMaxBitDepth = &v
	}
	if mobileMaxBitrate.Valid {
		v := int(mobileMaxBitrate.Int64)
		p.MobileMaxBitrateKbps = &v
	}
	if mobileMaxSR.Valid {
		v := int(mobileMaxSR.Int64)
		p.MobileMaxSampleRate = &v
	}
	if mobileMaxBD.Valid {
		v := int(mobileMaxBD.Int64)
		p.MobileMaxBitDepth = &v
	}
	if transcodeFmt.Valid {
		p.TranscodeFormat = &transcodeFmt.String
	}
	if wifiTranscodeFmt.Valid {
		p.WifiTranscodeFormat = &wifiTranscodeFmt.String
	}
	if mobileTranscodeFmt.Valid {
		p.MobileTranscodeFormat = &mobileTranscodeFmt.String
	}
	p.UpdatedAt = updatedAt
	return p, nil
}

// UpsertUserStreamingPrefs inserts or updates streaming preferences for a user.
func (s *Store) UpsertUserStreamingPrefs(ctx context.Context, p UpsertUserStreamingPrefsParams) (UserStreamingPrefs, error) {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_streaming_prefs (
			user_id,
			max_bitrate_kbps, max_sample_rate, max_bit_depth,
			wifi_max_bitrate_kbps, wifi_max_sample_rate, wifi_max_bit_depth,
			mobile_max_bitrate_kbps, mobile_max_sample_rate, mobile_max_bit_depth,
			transcode_format, wifi_transcode_format, mobile_transcode_format
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (user_id) DO UPDATE SET
			max_bitrate_kbps          = EXCLUDED.max_bitrate_kbps,
			max_sample_rate           = EXCLUDED.max_sample_rate,
			max_bit_depth             = EXCLUDED.max_bit_depth,
			wifi_max_bitrate_kbps     = EXCLUDED.wifi_max_bitrate_kbps,
			wifi_max_sample_rate      = EXCLUDED.wifi_max_sample_rate,
			wifi_max_bit_depth        = EXCLUDED.wifi_max_bit_depth,
			mobile_max_bitrate_kbps   = EXCLUDED.mobile_max_bitrate_kbps,
			mobile_max_sample_rate    = EXCLUDED.mobile_max_sample_rate,
			mobile_max_bit_depth      = EXCLUDED.mobile_max_bit_depth,
			transcode_format          = EXCLUDED.transcode_format,
			wifi_transcode_format     = EXCLUDED.wifi_transcode_format,
			mobile_transcode_format   = EXCLUDED.mobile_transcode_format,
			updated_at                = now()`,
		p.UserID,
		p.MaxBitrateKbps, p.MaxSampleRate, p.MaxBitDepth,
		p.WifiMaxBitrateKbps, p.WifiMaxSampleRate, p.WifiMaxBitDepth,
		p.MobileMaxBitrateKbps, p.MobileMaxSampleRate, p.MobileMaxBitDepth,
		p.TranscodeFormat, p.WifiTranscodeFormat, p.MobileTranscodeFormat,
	)
	if err != nil {
		return UserStreamingPrefs{}, err
	}
	return s.GetUserStreamingPrefs(ctx, p.UserID)
}

// ── EQ profiles ───────────────────────────────────────────────────────────

func scanEQProfile(id, userID, name string, bandsJSON []byte, isDefault bool, createdAt, updatedAt time.Time) (EQProfile, error) {
	p := EQProfile{
		ID:        id,
		UserID:    userID,
		Name:      name,
		IsDefault: isDefault,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if bandsJSON != nil {
		if err := json.Unmarshal(bandsJSON, &p.Bands); err != nil {
			return p, err
		}
	}
	return p, nil
}

func (s *Store) ListEQProfiles(ctx context.Context, userID string) ([]EQProfile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, bands,
		       COALESCE((SELECT TRUE FROM user_eq_default d WHERE d.user_id = eq.user_id AND d.profile_id = eq.id), FALSE) AS is_default,
		       created_at, updated_at
		FROM eq_profiles eq
		WHERE user_id = $1
		ORDER BY name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EQProfile, 0)
	for rows.Next() {
		var id, uid, name string
		var bandsJSON []byte
		var isDefault bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &uid, &name, &bandsJSON, &isDefault, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p, err := scanEQProfile(id, uid, name, bandsJSON, isDefault, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetEQProfile(ctx context.Context, id, userID string) (EQProfile, error) {
	var pid, uid, name string
	var bandsJSON []byte
	var isDefault bool
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, bands,
		       COALESCE((SELECT TRUE FROM user_eq_default d WHERE d.user_id = eq.user_id AND d.profile_id = eq.id), FALSE) AS is_default,
		       created_at, updated_at
		FROM eq_profiles eq
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&pid, &uid, &name, &bandsJSON, &isDefault, &createdAt, &updatedAt)
	if err != nil {
		return EQProfile{}, err
	}
	return scanEQProfile(pid, uid, name, bandsJSON, isDefault, createdAt, updatedAt)
}

func (s *Store) CreateEQProfile(ctx context.Context, p CreateEQProfileParams) (EQProfile, error) {
	bandsJSON, err := json.Marshal(p.Bands)
	if err != nil {
		return EQProfile{}, err
	}
	var id, uid, name string
	var stored []byte
	var createdAt, updatedAt time.Time
	err = s.pool.QueryRow(ctx, `
		INSERT INTO eq_profiles (id, user_id, name, bands)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, bands, created_at, updated_at
	`, p.ID, p.UserID, p.Name, bandsJSON).
		Scan(&id, &uid, &name, &stored, &createdAt, &updatedAt)
	if err != nil {
		return EQProfile{}, err
	}
	profile, err := scanEQProfile(id, uid, name, stored, false, createdAt, updatedAt)
	if err != nil {
		return EQProfile{}, err
	}
	if p.IsDefault {
		if err := s.SetDefaultEQProfile(ctx, id, uid); err != nil {
			return profile, err
		}
		profile.IsDefault = true
	}
	return profile, nil
}

func (s *Store) UpdateEQProfile(ctx context.Context, p UpdateEQProfileParams) (EQProfile, error) {
	bandsJSON, err := json.Marshal(p.Bands)
	if err != nil {
		return EQProfile{}, err
	}
	var id, uid, name string
	var stored []byte
	var createdAt, updatedAt time.Time
	err = s.pool.QueryRow(ctx, `
		UPDATE eq_profiles SET name = $3, bands = $4, updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, bands, created_at, updated_at
	`, p.ID, p.UserID, p.Name, bandsJSON).
		Scan(&id, &uid, &name, &stored, &createdAt, &updatedAt)
	if err != nil {
		return EQProfile{}, err
	}
	return scanEQProfile(id, uid, name, stored, false, createdAt, updatedAt)
}

func (s *Store) DeleteEQProfile(ctx context.Context, id, userID string) error {
	// Clear default if this profile is the default.
	_, _ = s.pool.Exec(ctx, `DELETE FROM user_eq_default WHERE user_id = $1 AND profile_id = $2`, userID, id)
	// Clear genre mappings referencing this profile.
	_, _ = s.pool.Exec(ctx, `DELETE FROM user_genre_eq WHERE user_id = $1 AND profile_id = $2`, userID, id)
	_, err := s.pool.Exec(ctx, `DELETE FROM eq_profiles WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (s *Store) SetDefaultEQProfile(ctx context.Context, id, userID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_eq_default (user_id, profile_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET profile_id = EXCLUDED.profile_id
	`, userID, id)
	return err
}

func (s *Store) ClearDefaultEQProfile(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_eq_default WHERE user_id = $1`, userID)
	return err
}

func (s *Store) GetDefaultEQProfile(ctx context.Context, userID string) (*EQProfile, error) {
	var profileID string
	err := s.pool.QueryRow(ctx,
		`SELECT profile_id FROM user_eq_default WHERE user_id = $1`, userID).Scan(&profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p, err := s.GetEQProfile(ctx, profileID, userID)
	if err != nil {
		return nil, err
	}
	p.IsDefault = true
	return &p, nil
}

// ── Genre EQ mappings ─────────────────────────────────────────────────────

func (s *Store) ListGenreEQMappings(ctx context.Context, userID string) ([]GenreEQMapping, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT gq.user_id, gq.genre_id, g.name, gq.profile_id
		FROM user_genre_eq gq
		JOIN genres g ON g.id = gq.genre_id
		WHERE gq.user_id = $1
		ORDER BY g.name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GenreEQMapping
	for rows.Next() {
		var m GenreEQMapping
		if err := rows.Scan(&m.UserID, &m.GenreID, &m.GenreName, &m.ProfileID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []GenreEQMapping{}
	}
	return out, rows.Err()
}

// SetGenreEQMapping upserts a genre→profile mapping for a user.
func (s *Store) SetGenreEQMapping(ctx context.Context, userID, genreID, profileID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_genre_eq (user_id, genre_id, profile_id) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, genre_id) DO UPDATE SET profile_id = EXCLUDED.profile_id`,
		userID, genreID, profileID)
	return err
}

// DeleteGenreEQMapping removes a genre→profile mapping for a user.
func (s *Store) DeleteGenreEQMapping(ctx context.Context, userID, genreID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_genre_eq WHERE user_id = $1 AND genre_id = $2`, userID, genreID)
	return err
}

// GetGenreEQProfile returns the EQ profile mapped to a genre for a user, or nil.
func (s *Store) GetGenreEQProfile(ctx context.Context, userID, genreID string) (*EQProfile, error) {
	var profileID string
	err := s.pool.QueryRow(ctx,
		`SELECT profile_id FROM user_genre_eq WHERE user_id = $1 AND genre_id = $2`,
		userID, genreID).Scan(&profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p, err := s.GetEQProfile(ctx, profileID, userID)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
