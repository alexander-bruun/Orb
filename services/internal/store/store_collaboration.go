package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
)

// ── Playlist collaboration ────────────────────────────────────────────────────

// ListCollaborators returns all collaborators for a playlist (including pending).
func (s *Store) ListCollaborators(ctx context.Context, playlistID string) ([]CollaboratorRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pc.user_id, u.username, u.display_name, u.avatar_key,
		       pc.role, pc.invited_by, pc.invited_at, pc.accepted_at
		FROM playlist_collaborators pc
		JOIN users u ON u.id = pc.user_id
		WHERE pc.playlist_id = $1
		ORDER BY pc.invited_at ASC`,
		playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CollaboratorRow
	for rows.Next() {
		var c CollaboratorRow
		var displayName, avatarKey sql.NullString
		var acceptedAt sql.NullTime
		if err := rows.Scan(
			&c.UserID, &c.Username, &displayName, &avatarKey,
			&c.Role, &c.InvitedBy, &c.InvitedAt, &acceptedAt,
		); err != nil {
			return nil, err
		}
		if displayName.Valid {
			c.DisplayName = &displayName.String
		}
		if avatarKey.Valid {
			c.AvatarKey = &avatarKey.String
		}
		if acceptedAt.Valid {
			t := acceptedAt.Time
			c.AcceptedAt = &t
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddCollaborator inserts a pending collaborator row.
func (s *Store) AddCollaborator(ctx context.Context, playlistID, userID, invitedBy, role string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO playlist_collaborators (playlist_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (playlist_id, user_id) DO NOTHING`,
		playlistID, userID, role, invitedBy)
	return err
}

// AcceptCollaborator sets accepted_at for a pending invite.
func (s *Store) AcceptCollaborator(ctx context.Context, playlistID, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE playlist_collaborators SET accepted_at = now()
		WHERE playlist_id = $1 AND user_id = $2 AND accepted_at IS NULL`,
		playlistID, userID)
	return err
}

// RemoveCollaborator deletes a collaborator from a playlist.
func (s *Store) RemoveCollaborator(ctx context.Context, playlistID, userID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM playlist_collaborators WHERE playlist_id = $1 AND user_id = $2`,
		playlistID, userID)
	return err
}

// UpdateCollaboratorRole changes a collaborator's role.
func (s *Store) UpdateCollaboratorRole(ctx context.Context, playlistID, userID, role string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE playlist_collaborators SET role = $3 WHERE playlist_id = $1 AND user_id = $2`,
		playlistID, userID, role)
	return err
}

// IsCollaborator returns whether userID is an accepted collaborator on playlistID,
// and their role. Returns (false, "", nil) when no row is found.
func (s *Store) IsCollaborator(ctx context.Context, playlistID, userID string) (bool, string, error) {
	var role string
	var acceptedAt sql.NullTime
	err := s.pool.QueryRow(ctx,
		`SELECT role, accepted_at FROM playlist_collaborators WHERE playlist_id = $1 AND user_id = $2`,
		playlistID, userID).Scan(&role, &acceptedAt)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return acceptedAt.Valid, role, nil
}

// CreatePlaylistInviteToken creates a new invite token for a playlist.
func (s *Store) CreatePlaylistInviteToken(ctx context.Context, playlistID, invitedBy, role string) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO playlist_invite_tokens (token, playlist_id, invited_by, role)
		VALUES ($1, $2, $3, $4)`,
		token, playlistID, invitedBy, role)
	if err != nil {
		return "", err
	}
	return token, nil
}

// GetPlaylistInviteToken returns a playlist invite token row.
func (s *Store) GetPlaylistInviteToken(ctx context.Context, token string) (*PlaylistInvite, error) {
	var inv PlaylistInvite
	var usedAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT token, playlist_id, invited_by, role, created_at, expires_at, used_at
		FROM playlist_invite_tokens WHERE token = $1`,
		token).Scan(
		&inv.Token, &inv.PlaylistID, &inv.InvitedBy, &inv.Role,
		&inv.CreatedAt, &inv.ExpiresAt, &usedAt,
	)
	if err != nil {
		return nil, err
	}
	if usedAt.Valid {
		t := usedAt.Time
		inv.UsedAt = &t
	}
	return &inv, nil
}

// RedeemPlaylistInviteToken accepts the invite: marks the token used and adds the collaborator.
func (s *Store) RedeemPlaylistInviteToken(ctx context.Context, token, userID string) (*PlaylistInvite, error) {
	inv, err := s.GetPlaylistInviteToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if inv.UsedAt != nil {
		return nil, ErrInviteAlreadyUsed
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`UPDATE playlist_invite_tokens SET used_at = now() WHERE token = $1`, token)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO playlist_collaborators (playlist_id, user_id, role, invited_by, accepted_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (playlist_id, user_id) DO UPDATE SET role = EXCLUDED.role, accepted_at = now()`,
		inv.PlaylistID, userID, inv.Role, inv.InvitedBy)
	if err != nil {
		return nil, err
	}
	return inv, tx.Commit(ctx)
}

// GetPlaylistsForUser returns playlists owned by or accepted-collaborator of userID.
func (s *Store) GetPlaylistsForUser(ctx context.Context, userID string) ([]Playlist, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, description, cover_art_key, created_at
		FROM playlists
		WHERE user_id = $1
		   OR id IN (
		     SELECT playlist_id FROM playlist_collaborators
		     WHERE user_id = $1 AND accepted_at IS NOT NULL
		   )
		ORDER BY updated_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlaylists(rows)
}

// sentinel errors for invite token validation
var (
	ErrInviteAlreadyUsed = storeErr("invite token already used")
	ErrInviteExpired     = storeErr("invite token expired")
	ErrProfileNotPublic  = storeErr("profile not public")
)

type storeErr string

func (e storeErr) Error() string { return string(e) }
