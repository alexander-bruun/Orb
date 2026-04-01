package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// UpsertAlbum inserts or updates an album.
func (s *Store) UpsertAlbum(ctx context.Context, p UpsertAlbumParams) (Album, error) {
	var alb Album
	row := s.pool.QueryRow(ctx, `INSERT INTO albums (id, artist_id, title, release_year, label, cover_art_key, mbid, album_group_id, edition)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET artist_id = EXCLUDED.artist_id, title = EXCLUDED.title, release_year = EXCLUDED.release_year, label = EXCLUDED.label, cover_art_key = COALESCE(EXCLUDED.cover_art_key, albums.cover_art_key), mbid = EXCLUDED.mbid, album_group_id = EXCLUDED.album_group_id, edition = EXCLUDED.edition RETURNING id, artist_id, title, release_year, label, cover_art_key, mbid, album_group_id, edition, created_at`,
		p.ID, p.ArtistID, p.Title, p.ReleaseYear, p.Label, p.CoverArtKey, p.Mbid, p.AlbumGroupID, p.Edition)
	var artistID, label, coverArtKey, mbid, albumGroupID, edition sql.NullString
	var releaseYear sql.NullInt64
	err := row.Scan(&alb.ID, &artistID, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &albumGroupID, &edition, &alb.CreatedAt)
	if artistID.Valid {
		alb.ArtistID = &artistID.String
	}
	if releaseYear.Valid {
		y := int(releaseYear.Int64)
		alb.ReleaseYear = &y
	}
	if label.Valid {
		alb.Label = &label.String
	}
	if coverArtKey.Valid {
		alb.CoverArtKey = &coverArtKey.String
	}
	if mbid.Valid {
		alb.Mbid = &mbid.String
	}
	if albumGroupID.Valid {
		alb.AlbumGroupID = &albumGroupID.String
	}
	if edition.Valid {
		alb.Edition = &edition.String
	}
	return alb, err
}

func (s *Store) ListAlbums(ctx context.Context, p ListAlbumsParams) ([]Album, error) {
	// Build ORDER BY from a whitelist — no user input reaches the query string.
	var orderBy string
	switch p.SortBy {
	case "artist":
		orderBy = `regexp_replace(lower(coalesce(ar_name, '')), '^(the |a |an )\s*', '') ASC,` +
			` regexp_replace(lower(title), '^(the |a |an )\s*', '') ASC`
	case "year":
		orderBy = `release_year DESC NULLS LAST,` +
			` regexp_replace(lower(title), '^(the |a |an )\s*', '') ASC`
	default: // "title"
		orderBy = `regexp_replace(lower(title), '^(the |a |an )\s*', '') ASC`
	}
	// Use ROW_NUMBER to pick one representative per album group (prefer albums
	// that have cover art; otherwise pick the earliest-created one).
	rows, err := s.pool.Query(ctx,
		`WITH ranked AS (
			SELECT al.id, al.artist_id, ar.name AS ar_name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) AS track_count, COALESCE(MAX(t.channels), 2) AS max_channels,
			       ROW_NUMBER() OVER (
			           PARTITION BY COALESCE(al.album_group_id, al.id)
			           ORDER BY (al.cover_art_key IS NULL) ASC, al.created_at ASC
			       ) AS rn
			FROM albums al
			LEFT JOIN artists ar ON ar.id = al.artist_id
			LEFT JOIN tracks t ON t.album_id = al.id
			GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
		)
		SELECT id, artist_id, ar_name, title, release_year, label, cover_art_key, mbid, created_at, track_count, max_channels
		FROM ranked
		WHERE rn = 1
		ORDER BY `+orderBy+` LIMIT $1 OFFSET $2`,
		p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// GetAlbumTitlesByIDs returns a map of album ID → title for the given IDs.
// Only id and title are fetched, making it efficient for bulk display enrichment.
func (s *Store) GetAlbumTitlesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, title FROM albums WHERE id = ANY($1)`,
		ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string, len(ids))
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		result[id] = title
	}
	return result, rows.Err()
}

func (s *Store) ListAlbumsByArtist(ctx context.Context, artistID string) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t2.id) as track_count
FROM albums al
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t2 ON t2.album_id = al.id
WHERE al.artist_id = $1
   OR EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = al.id AND t.artist_id = $1)
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY al.release_year ASC, al.title ASC`,
		artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// ListAlbumsWithFeaturedArtist returns albums that contain at least one track
// on which artistID appears as a featured artist, excluding albums where the
// artist is already the primary album or track artist (those are covered by
// ListAlbumsByArtist).
func (s *Store) ListAlbumsWithFeaturedArtist(ctx context.Context, artistID string) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t2.id) AS track_count
FROM albums al
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t2 ON t2.album_id = al.id
WHERE EXISTS (
    SELECT 1 FROM tracks t
    JOIN track_featured_artists tfa ON tfa.track_id = t.id
    WHERE t.album_id = al.id AND tfa.artist_id = $1
)
AND (al.artist_id IS NULL OR al.artist_id != $1)
AND NOT EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = al.id AND t.artist_id = $1)
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY al.release_year ASC, al.title ASC`,
		artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

func (s *Store) SearchAlbums(ctx context.Context, p SearchAlbumsParams) ([]Album, error) {
	args := []any{p.ToTsquery}
	argIdx := 2

	extraJoins := strings.Builder{}
	conds := []string{"al.search_vector @@ websearch_to_tsquery('english', $1)"}

	if p.Genre != "" {
		extraJoins.WriteString(" JOIN album_genres ag ON ag.album_id = al.id JOIN genres g ON g.id = ag.genre_id")
		args = append(args, strings.ToLower(p.Genre))
		conds = append(conds, fmt.Sprintf("LOWER(g.name) = $%d", argIdx))
		argIdx++
	}
	if p.YearFrom != nil {
		args = append(args, *p.YearFrom)
		conds = append(conds, fmt.Sprintf("al.release_year >= $%d", argIdx))
		argIdx++
	}
	if p.YearTo != nil {
		args = append(args, *p.YearTo)
		conds = append(conds, fmt.Sprintf("al.release_year <= $%d", argIdx))
		argIdx++
	}

	orderBy := "ts_rank(al.search_vector, websearch_to_tsquery('english', $1)) DESC"
	switch p.SortBy {
	case "title":
		orderBy = "al.title ASC"
	case "year":
		orderBy = "al.release_year DESC NULLS LAST"
	}

	args = append(args, p.Limit)
	q := fmt.Sprintf(
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count, COALESCE(MAX(t.channels), 2) AS max_channels
FROM albums al
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t ON t.album_id = al.id%s
WHERE %s
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY %s LIMIT $%d`,
		extraJoins.String(), strings.Join(conds, " AND "), orderBy, argIdx)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// ListRecentlyPlayedAlbums returns distinct albums played by the user, ordered by most recent play.
func (s *Store) ListRecentlyPlayedAlbums(ctx context.Context, p ListRecentlyPlayedParams) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name AS artist_name, al.title, al.release_year,
		        al.label, al.cover_art_key, al.mbid, al.created_at,
		        COUNT(DISTINCT tr.id) AS track_count, COALESCE(MAX(tr.channels), 2) AS max_channels
		FROM (
		  SELECT DISTINCT ON (t.album_id) t.album_id, ph.played_at
		  FROM play_history ph
		  JOIN tracks t ON t.id = ph.track_id
		  WHERE ph.user_id = $1 AND t.album_id IS NOT NULL
		  ORDER BY t.album_id, ph.played_at DESC
		) ra
		JOIN albums al ON al.id = ra.album_id
		LEFT JOIN artists ar ON ar.id = al.artist_id
		LEFT JOIN tracks tr ON tr.album_id = al.id
		GROUP BY al.id, al.artist_id, ar.name, al.title, al.release_year,
		         al.label, al.cover_art_key, al.mbid, al.created_at, ra.played_at
		ORDER BY ra.played_at DESC
		LIMIT $2`,
		p.UserID, p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// ListRecentAlbums returns the most recently added albums.
func (s *Store) ListRecentAlbums(ctx context.Context, p ListRecentAlbumsParams) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name AS artist_name, al.title, al.release_year,
		        al.label, al.cover_art_key, al.mbid, al.created_at,
		        COUNT(t.id) AS track_count, COALESCE(MAX(t.channels), 2) AS max_channels
		FROM albums al
		LEFT JOIN artists ar ON ar.id = al.artist_id
		LEFT JOIN tracks t ON t.album_id = al.id
		GROUP BY al.id, al.artist_id, ar.name, al.title, al.release_year,
		         al.label, al.cover_art_key, al.mbid, al.created_at
		ORDER BY al.created_at DESC
		LIMIT $1`,
		p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// CountAlbums returns the number of distinct album groups (i.e. deduped by album_group_id).
func (s *Store) CountAlbums(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT DISTINCT COALESCE(album_group_id, id) FROM albums) s`).Scan(&count)
	return count, err
}

// GetAlbumByID returns an album by ID.
func (s *Store) GetAlbumByID(ctx context.Context, id string) (Album, error) {
	var alb Album
	row := s.pool.QueryRow(ctx, `SELECT id, artist_id, title, release_year, label, cover_art_key, mbid, album_type, release_date, release_group_mbid, enriched_at, album_group_id, edition, created_at, (SELECT COUNT(*) FROM tracks WHERE album_id = $1) as track_count, COALESCE((SELECT MAX(channels) FROM tracks WHERE album_id = $1), 2) AS max_channels FROM albums WHERE id = $1`, id)
	var artistID, label, coverArtKey, mbid, albumType, releaseDate, releaseGroupMbid, albumGroupID, edition sql.NullString
	var releaseYear sql.NullInt64
	var enrichedAt sql.NullTime
	err := row.Scan(&alb.ID, &artistID, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &albumType, &releaseDate, &releaseGroupMbid, &enrichedAt, &albumGroupID, &edition, &alb.CreatedAt, &alb.TrackCount, &alb.MaxChannels)
	if artistID.Valid {
		alb.ArtistID = &artistID.String
	}
	if releaseYear.Valid {
		y := int(releaseYear.Int64)
		alb.ReleaseYear = &y
	}
	if label.Valid {
		alb.Label = &label.String
	}
	if coverArtKey.Valid {
		alb.CoverArtKey = &coverArtKey.String
	}
	if mbid.Valid {
		alb.Mbid = &mbid.String
	}
	if albumType.Valid {
		alb.AlbumType = &albumType.String
	}
	if releaseDate.Valid {
		alb.ReleaseDate = &releaseDate.String
	}
	if releaseGroupMbid.Valid {
		alb.ReleaseGroupMbid = &releaseGroupMbid.String
	}
	if enrichedAt.Valid {
		alb.EnrichedAt = &enrichedAt.Time
	}
	if albumGroupID.Valid {
		alb.AlbumGroupID = &albumGroupID.String
	}
	if edition.Valid {
		alb.Edition = &edition.String
	}
	return alb, err
}

// ListAlbumVariants returns all albums sharing the same album_group_id, ordered by creation date.
func (s *Store) ListAlbumVariants(ctx context.Context, groupID string) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.title, al.edition, al.cover_art_key, al.created_at,
		        COUNT(t.id) AS track_count, COALESCE(MAX(t.channels), 2) AS max_channels
		 FROM albums al
		 LEFT JOIN tracks t ON t.album_id = al.id
		 WHERE al.album_group_id = $1
		 GROUP BY al.id, al.title, al.edition, al.cover_art_key, al.created_at
		 ORDER BY al.created_at ASC`,
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		var alb Album
		var edition, coverArtKey sql.NullString
		if err := rows.Scan(&alb.ID, &alb.Title, &edition, &coverArtKey, &alb.CreatedAt, &alb.TrackCount); err != nil {
			return nil, err
		}
		if edition.Valid {
			alb.Edition = &edition.String
		}
		if coverArtKey.Valid {
			alb.CoverArtKey = &coverArtKey.String
		}
		out = append(out, alb)
	}
	return out, rows.Err()
}

// ── Album enrichment ──────────────────────────────────────────────────────

// UpdateAlbumEnrichment updates an album with MusicBrainz metadata.
func (s *Store) UpdateAlbumEnrichment(ctx context.Context, p UpdateAlbumEnrichmentParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE albums SET mbid = COALESCE($2, mbid), label = COALESCE($3, label), album_type = $4, release_date = $5, release_group_mbid = $6, enriched_at = now() WHERE id = $1`,
		p.ID, p.Mbid, p.Label, p.AlbumType, p.ReleaseDate, p.ReleaseGroupMbid)
	return err
}

// UpdateAlbumCoverArt sets the cover_art_key for an album.
func (s *Store) UpdateAlbumCoverArt(ctx context.Context, albumID, coverArtKey string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE albums SET cover_art_key = $2 WHERE id = $1`,
		albumID, coverArtKey)
	return err
}

// SetAlbumGenres replaces all genre associations for an album.
func (s *Store) SetAlbumGenres(ctx context.Context, albumID string, genreIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)
	if _, err := tx.Exec(ctx, `DELETE FROM album_genres WHERE album_id = $1`, albumID); err != nil {
		return err
	}
	for _, gid := range genreIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO album_genres (album_id, genre_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, albumID, gid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListGenresByAlbum returns all genres for an album.
func (s *Store) ListGenresByAlbum(ctx context.Context, albumID string) ([]Genre, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT g.id, g.name FROM genres g JOIN album_genres ag ON ag.genre_id = g.id WHERE ag.album_id = $1 ORDER BY g.name`,
		albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGenres(rows)
}

// ListUnenrichedAlbums returns albums that haven't been enriched yet, with artist name.
// If force is true, returns all albums regardless of enrichment status.
func (s *Store) ListUnenrichedAlbums(ctx context.Context, limit int, force bool) ([]Album, error) {
	whereClause := "WHERE al.enriched_at IS NULL"
	if force {
		whereClause = ""
	}
	q := fmt.Sprintf(`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count, COALESCE(MAX(t.channels), 2) AS max_channels
FROM albums al
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t ON t.album_id = al.id
%s
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY al.title LIMIT $1`, whereClause)
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// ListAlbumsByGenre returns albums that have a given genre.
func (s *Store) ListAlbumsByGenre(ctx context.Context, genreID string, limit, offset int) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count, COALESCE(MAX(t.channels), 2) AS max_channels
FROM albums al
JOIN album_genres ag ON ag.album_id = al.id
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t ON t.album_id = al.id
WHERE ag.genre_id = $1
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY al.title LIMIT $2 OFFSET $3`,
		genreID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

// ── Scan helpers ───────────────────────────────────────────────────────────

func scanAlbums(rows pgx.Rows) ([]Album, error) {
	out := make([]Album, 0)
	for rows.Next() {
		var alb Album
		var artistID, artistName, label, coverArtKey, mbid sql.NullString
		var releaseYear sql.NullInt64
		var maxChannels sql.NullInt64
		if err := rows.Scan(&alb.ID, &artistID, &artistName, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &alb.CreatedAt, &alb.TrackCount, &maxChannels); err != nil {
			return nil, err
		}
		if artistID.Valid {
			alb.ArtistID = &artistID.String
		}
		if artistName.Valid {
			alb.ArtistName = &artistName.String
		}
		if releaseYear.Valid {
			y := int(releaseYear.Int64)
			alb.ReleaseYear = &y
		}
		if label.Valid {
			alb.Label = &label.String
		}
		if coverArtKey.Valid {
			alb.CoverArtKey = &coverArtKey.String
		}
		if mbid.Valid {
			alb.Mbid = &mbid.String
		}
		if maxChannels.Valid && maxChannels.Int64 > 2 {
			alb.MaxChannels = int(maxChannels.Int64)
		} else {
			alb.MaxChannels = 2
		}
		out = append(out, alb)
	}
	return out, rows.Err()
}
