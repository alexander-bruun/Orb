package store

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// ── UpdateAudiobookMeta ───────────────────────────────────────────────────────

// UpdateAudiobookMetaParams holds fields the admin can update on an audiobook.
type UpdateAudiobookMetaParams struct {
	ID            string
	Title         string
	// UpdateAuthor controls whether author_id is modified at all.
	// When true, AuthorID is written (nil clears the author, non-nil sets it).
	// When false, the existing author_id is left untouched.
	UpdateAuthor  bool
	AuthorID      *string  // new author ID; nil clears the author when UpdateAuthor=true
	Description   *string  // nil = clear
	Series        *string  // nil = clear
	SeriesIndex   *float64 // nil = clear
	Edition       *string  // nil = clear
	PublishedYear *int     // nil = clear
}

// UpdateAudiobookMeta performs a targeted UPDATE on an audiobook row.
func (s *Store) UpdateAudiobookMeta(ctx context.Context, p UpdateAudiobookMetaParams) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE audiobooks SET
			title          = $2,
			author_id      = CASE WHEN $3::boolean THEN $4 ELSE author_id END,
			description    = $5,
			series         = $6,
			series_index   = $7,
			series_source  = CASE WHEN $6 IS NULL THEN NULL ELSE 'manual' END,
			series_confidence = CASE WHEN $6 IS NULL THEN NULL ELSE 1.0 END,
			edition        = $8,
			published_year = $9
		WHERE id = $1`,
		p.ID,
		p.Title,
		p.UpdateAuthor, p.AuthorID,
		p.Description,
		p.Series,
		p.SeriesIndex,
		p.Edition,
		p.PublishedYear,
	)
	return err
}

// ── UpdateAlbumMeta ───────────────────────────────────────────────────────────

// UpdateAlbumMetaParams holds fields the admin can update on an album.
type UpdateAlbumMetaParams struct {
	ID          string
	Title       string
	ReleaseYear *int    // nil = clear
	Label       *string // nil = clear
}

// UpdateAlbumMeta performs a targeted UPDATE on an album row.
func (s *Store) UpdateAlbumMeta(ctx context.Context, p UpdateAlbumMetaParams) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE albums SET
			title        = $2,
			release_year = $3,
			label        = $4
		WHERE id = $1`,
		p.ID, p.Title, p.ReleaseYear, p.Label,
	)
	return err
}

// ── UpdateTrackMeta ───────────────────────────────────────────────────────────

// UpdateTrackMetaParams holds fields the admin can update on a track.
type UpdateTrackMetaParams struct {
	ID          string
	Title       string
	TrackNumber *int // nil = clear
	DiscNumber  int
}

// UpdateTrackMeta performs a targeted UPDATE on a track row.
func (s *Store) UpdateTrackMeta(ctx context.Context, p UpdateTrackMetaParams) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tracks SET
			title        = $2,
			track_number = $3,
			disc_number  = $4
		WHERE id = $1`,
		p.ID, p.Title, p.TrackNumber, p.DiscNumber,
	)
	return err
}

// ── FindOrCreateArtistByName ──────────────────────────────────────────────────

// FindOrCreateArtistByName looks up an artist by name (case-insensitive) or
// creates a minimal artist record and returns its ID.
func (s *Store) FindOrCreateArtistByName(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	// Try to find existing.
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM artists WHERE lower(name) = lower($1) LIMIT 1`, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}

	// Create a new minimal artist record.
	newID := uuid.New().String()
	_, err = s.pool.Exec(ctx,
		`INSERT INTO artists (id, name, sort_name) VALUES ($1, $2, $2)
		 ON CONFLICT DO NOTHING`,
		newID, name,
	)
	if err != nil {
		return "", err
	}
	// Re-query in case another insert won the race.
	err = s.pool.QueryRow(ctx,
		`SELECT id FROM artists WHERE lower(name) = lower($1) LIMIT 1`, name,
	).Scan(&id)
	return id, err
}

// ── ClearAlbumEnrichment ──────────────────────────────────────────────────────

// ClearAlbumEnrichment sets enriched_at = NULL on the album and its tracks
// so the next ingest pass will re-enrich them from MusicBrainz.
func (s *Store) ClearAlbumEnrichment(ctx context.Context, albumID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE albums SET enriched_at = NULL WHERE id = $1`, albumID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tracks SET enriched_at = NULL WHERE album_id = $1`, albumID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
