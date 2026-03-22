package store

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
)

// UpsertArtist inserts or updates an artist.
func (s *Store) UpsertArtist(ctx context.Context, p UpsertArtistParams) (Artist, error) {
	var a Artist
	row := s.pool.QueryRow(ctx, `INSERT INTO artists (id, name, sort_name, mbid)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, sort_name = EXCLUDED.sort_name, mbid = EXCLUDED.mbid RETURNING id, name, sort_name, mbid, created_at`,
		p.ID, p.Name, p.SortName, p.Mbid)
	var mbid sql.NullString
	err := row.Scan(&a.ID, &a.Name, &a.SortName, &mbid, &a.CreatedAt)
	if mbid.Valid {
		a.Mbid = &mbid.String
	}
	return a, err
}

func (s *Store) ListArtists(ctx context.Context, p ListArtistsParams) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, sort_name, mbid, image_key, created_at FROM artists ORDER BY sort_name ASC LIMIT $1 OFFSET $2`,
		p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtists(rows)
}

func (s *Store) GetArtistByID(ctx context.Context, artistID string) (Artist, error) {
	var a Artist
	var mbid, artistType, country, beginDate, endDate, disambiguation, imageKey sql.NullString
	var enrichedAt sql.NullTime
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, sort_name, mbid, artist_type, country, begin_date, end_date, disambiguation, image_key, enriched_at, created_at FROM artists WHERE id = $1`,
		artistID)
	if err := row.Scan(&a.ID, &a.Name, &a.SortName, &mbid, &artistType, &country, &beginDate, &endDate, &disambiguation, &imageKey, &enrichedAt, &a.CreatedAt); err != nil {
		return Artist{}, err
	}
	if mbid.Valid {
		a.Mbid = &mbid.String
	}
	if artistType.Valid {
		a.ArtistType = &artistType.String
	}
	if country.Valid {
		a.Country = &country.String
	}
	if beginDate.Valid {
		a.BeginDate = &beginDate.String
	}
	if endDate.Valid {
		a.EndDate = &endDate.String
	}
	if disambiguation.Valid {
		a.Disambiguation = &disambiguation.String
	}
	if imageKey.Valid {
		a.ImageKey = &imageKey.String
	}
	if enrichedAt.Valid {
		a.EnrichedAt = &enrichedAt.Time
	}
	return a, nil
}

// GetArtistNamesByIDs returns a map of artist ID → name for the given IDs.
// Only id and name are fetched, making it efficient for bulk display enrichment.
func (s *Store) GetArtistNamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, name FROM artists WHERE id = ANY($1)`,
		ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string, len(ids))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		result[id] = name
	}
	return result, rows.Err()
}

func (s *Store) SearchArtists(ctx context.Context, p SearchArtistsParams) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, sort_name, mbid, image_key, created_at FROM artists
WHERE search_vector @@ websearch_to_tsquery('english', $1)
  AND EXISTS (SELECT 1 FROM albums WHERE artist_id = artists.id)
ORDER BY ts_rank(search_vector, websearch_to_tsquery('english', $1)) DESC LIMIT $2`,
		p.ToTsquery, p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtists(rows)
}

// ── Artist enrichment ─────────────────────────────────────────────────────

// UpdateArtistEnrichment updates an artist with MusicBrainz metadata.
func (s *Store) UpdateArtistEnrichment(ctx context.Context, p UpdateArtistEnrichmentParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE artists SET mbid = COALESCE($2, mbid), artist_type = $3, country = $4, begin_date = $5, end_date = $6, disambiguation = $7, image_key = COALESCE($8, image_key), enriched_at = now() WHERE id = $1`,
		p.ID, p.Mbid, p.ArtistType, p.Country, p.BeginDate, p.EndDate, p.Disambiguation, p.ImageKey)
	return err
}

// SetArtistGenres replaces all genre associations for an artist.
func (s *Store) SetArtistGenres(ctx context.Context, artistID string, genreIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM artist_genres WHERE artist_id = $1`, artistID); err != nil {
		return err
	}
	for _, gid := range genreIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO artist_genres (artist_id, genre_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, artistID, gid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListGenresByArtist returns all genres for an artist.
func (s *Store) ListGenresByArtist(ctx context.Context, artistID string) ([]Genre, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT g.id, g.name FROM genres g JOIN artist_genres ag ON ag.genre_id = g.id WHERE ag.artist_id = $1 ORDER BY g.name`,
		artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGenres(rows)
}

// ── Related artists ───────────────────────────────────────────────────────

// UpsertRelatedArtist inserts a related artist relationship.
func (s *Store) UpsertRelatedArtist(ctx context.Context, artistID, relatedID, relType string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO related_artists (artist_id, related_id, rel_type) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		artistID, relatedID, relType)
	return err
}

// ListRelatedArtists returns all related artists for an artist.
func (s *Store) ListRelatedArtists(ctx context.Context, artistID string) ([]RelatedArtist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ra.artist_id, ra.related_id, ra.rel_type, a.name
FROM related_artists ra
JOIN artists a ON a.id = ra.related_id
WHERE ra.artist_id = $1
ORDER BY ra.rel_type, a.name`,
		artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RelatedArtist, 0)
	for rows.Next() {
		var r RelatedArtist
		if err := rows.Scan(&r.ArtistID, &r.RelatedID, &r.RelType, &r.ArtistName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAllRelatedArtists returns all related-artist pairs.
func (s *Store) ListAllRelatedArtists(ctx context.Context) ([]RelatedArtistPair, error) {
	rows, err := s.pool.Query(ctx, `SELECT artist_id, related_id FROM related_artists`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RelatedArtistPair
	for rows.Next() {
		var p RelatedArtistPair
		if err := rows.Scan(&p.ArtistID, &p.RelatedID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListUnenrichedArtists returns artists that haven't been enriched yet.
// If force is true, returns all artists regardless of enrichment status.
func (s *Store) ListUnenrichedArtists(ctx context.Context, limit int, force bool) ([]Artist, error) {
	q := `SELECT id, name, sort_name, mbid, image_key, created_at FROM artists`
	if !force {
		q += ` WHERE enriched_at IS NULL`
	}
	q += ` ORDER BY name LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtists(rows)
}

// ListArtistsByGenre returns artists that have a given genre.
func (s *Store) ListArtistsByGenre(ctx context.Context, genreID string, limit, offset int) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.name, a.sort_name, a.mbid, a.image_key, a.created_at
FROM artists a
JOIN artist_genres ag ON ag.artist_id = a.id
WHERE ag.genre_id = $1
ORDER BY a.sort_name LIMIT $2 OFFSET $3`,
		genreID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtists(rows)
}

// ── Scan helpers ───────────────────────────────────────────────────────────

func scanArtists(rows pgx.Rows) ([]Artist, error) {
	out := make([]Artist, 0)
	for rows.Next() {
		var a Artist
		var mbid, imageKey sql.NullString
		if err := rows.Scan(&a.ID, &a.Name, &a.SortName, &mbid, &imageKey, &a.CreatedAt); err != nil {
			return nil, err
		}
		if mbid.Valid {
			a.Mbid = &mbid.String
		}
		if imageKey.Valid {
			a.ImageKey = &imageKey.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
