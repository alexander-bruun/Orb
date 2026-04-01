package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ── Track CRUD ─────────────────────────────────────────────────────────────

func (s *Store) ListTracksByUser(ctx context.Context, p ListTracksByUserParams) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
ORDER BY t.title ASC
LIMIT $1 OFFSET $2`,
		p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) ListTracksByAlbum(ctx context.Context, albumID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
WHERE t.album_id = $1
ORDER BY t.disc_number ASC, t.track_number ASC`,
		albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// TopTracksByArtist returns the most-played tracks for an artist, ordered by
// global play count descending, falling back to track_number order.
func (s *Store) TopTracksByArtist(ctx context.Context, artistID string, limit int) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		        t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm,
		        t.audio_layouts, t.has_atmos, t.audio_formats
		FROM tracks t
		LEFT JOIN track_features tf ON tf.track_id = t.id
		LEFT JOIN (
		  SELECT track_id, COUNT(*) AS play_count
		  FROM play_history
		  GROUP BY track_id
		) ph ON ph.track_id = t.id
		WHERE t.artist_id = $1
		ORDER BY COALESCE(ph.play_count, 0) DESC, t.track_number ASC
		LIMIT $2`,
		artistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) RemoveTrackFromLibrary(ctx context.Context, userID, trackID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_library WHERE user_id = $1 AND track_id = $2`, userID, trackID)
	return err
}

func (s *Store) GetQueue(ctx context.Context, userID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
JOIN queue_entries qe ON qe.track_id = t.id
WHERE qe.user_id = $1
ORDER BY qe.position ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) ClearQueue(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM queue_entries WHERE user_id = $1`, userID)
	return err
}

func (s *Store) InsertQueueEntry(ctx context.Context, p InsertQueueEntryParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO queue_entries (user_id, track_id, position, source) VALUES ($1, $2, $3, $4)`,
		p.UserID, p.TrackID, p.Position, p.Source)
	return err
}

func (s *Store) GetMinQueuePosition(ctx context.Context, userID string) (int, error) {
	var pos int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MIN(position), 0)::int FROM queue_entries WHERE user_id = $1`, userID).Scan(&pos)
	return pos, err
}

func (s *Store) GetMaxQueuePosition(ctx context.Context, userID string) (int, error) {
	var pos int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0)::int FROM queue_entries WHERE user_id = $1`, userID).Scan(&pos)
	return pos, err
}

// GetTrackByID returns a track by ID.
func (s *Store) GetTrackByID(ctx context.Context, id string) (Track, error) {
	var t Track
	row := s.pool.QueryRow(ctx, `SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.isrc, t.mbid, t.enriched_at, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats FROM tracks t LEFT JOIN track_features tf ON tf.track_id = t.id WHERE t.id = $1`, id)
	var albumID, artistID, format sql.NullString
	var trackNumber, trackIndex, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprintVal, isrc, mbid sql.NullString
	var enrichedAt sql.NullTime
	var createdAt time.Time
	var replayGain, bpm sql.NullFloat64
	var audioLayouts []string
	var hasAtmos sql.NullBool
	var audioFormatsRaw []byte
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &trackIndex, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprintVal, &isrc, &mbid, &enrichedAt, &createdAt, &replayGain, &bpm, &audioLayouts, &hasAtmos, &audioFormatsRaw)
	if albumID.Valid {
		t.AlbumID = &albumID.String
	}
	if artistID.Valid {
		t.ArtistID = &artistID.String
	}
	if trackNumber.Valid {
		n := int(trackNumber.Int64)
		t.TrackNumber = &n
	}
	if trackIndex.Valid {
		n := int(trackIndex.Int64)
		t.TrackIndex = &n
	}
	t.DiscNumber = int(discNumber.Int64)
	t.DurationMs = int(durationMs.Int64)
	if fileKey.Valid {
		t.FileKey = fileKey.String
	}
	t.FileSize = fileSize.Int64
	if format.Valid {
		t.Format = format.String
	}
	if bitDepth.Valid {
		n := int(bitDepth.Int64)
		t.BitDepth = &n
	}
	t.SampleRate = int(sampleRate.Int64)
	t.Channels = int(channels.Int64)
	if bitrateKbps.Valid {
		n := int(bitrateKbps.Int64)
		t.BitrateKbps = &n
	}
	t.SeekTable = seekTable
	if fingerprintVal.Valid {
		t.Fingerprint = fingerprintVal.String
	}
	if isrc.Valid {
		t.Isrc = &isrc.String
	}
	if mbid.Valid {
		t.Mbid = &mbid.String
	}
	if enrichedAt.Valid {
		t.EnrichedAt = &enrichedAt.Time
	}
	if replayGain.Valid && replayGain.Float64 != 0 {
		rg := replayGain.Float64
		t.ReplayGainTrack = &rg
	}
	if bpm.Valid && bpm.Float64 != 0 {
		v := bpm.Float64
		t.BPM = &v
	}
	t.CreatedAt = createdAt
	if len(audioLayouts) > 0 {
		t.AudioLayouts = audioLayouts
	}
	if hasAtmos.Valid {
		t.HasAtmos = hasAtmos.Bool
	}
	if len(audioFormatsRaw) > 0 {
		_ = json.Unmarshal(audioFormatsRaw, &t.AudioFormats)
	}
	return t, err
}

// GetTracksByIDs returns tracks for the given IDs in a single batch query.
// Tracks are returned in insertion order; missing IDs are silently omitted.
func (s *Store) GetTracksByIDs(ctx context.Context, ids []string) ([]Track, error) {
	if len(ids) == 0 {
		return []Track{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
		FROM tracks t
		LEFT JOIN track_features tf ON tf.track_id = t.id
		WHERE t.id = ANY($1)`,
		ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) SearchTracks(ctx context.Context, p SearchTracksParams) ([]Track, error) {
	args := []any{p.ToTsquery}
	argIdx := 2

	extraJoins := strings.Builder{}
	conds := []string{"(t.search_vector @@ websearch_to_tsquery('english', $1) OR ar.search_vector @@ websearch_to_tsquery('english', $1))"}
	needAlbumJoin := p.YearFrom != nil || p.YearTo != nil || p.SortBy == "year"

	if p.Genre != "" {
		extraJoins.WriteString(" JOIN track_genres tg ON tg.track_id = t.id JOIN genres g ON g.id = tg.genre_id")
		args = append(args, strings.ToLower(p.Genre))
		conds = append(conds, fmt.Sprintf("LOWER(g.name) = $%d", argIdx))
		argIdx++
	}
	if needAlbumJoin {
		extraJoins.WriteString(" LEFT JOIN albums al_f ON al_f.id = t.album_id")
	}
	if p.YearFrom != nil {
		args = append(args, *p.YearFrom)
		conds = append(conds, fmt.Sprintf("al_f.release_year >= $%d", argIdx))
		argIdx++
	}
	if p.YearTo != nil {
		args = append(args, *p.YearTo)
		conds = append(conds, fmt.Sprintf("al_f.release_year <= $%d", argIdx))
		argIdx++
	}
	if p.Format != "" {
		args = append(args, strings.ToLower(p.Format))
		conds = append(conds, fmt.Sprintf("LOWER(t.format) = $%d", argIdx))
		argIdx++
	}
	if p.BitrateMin != nil {
		args = append(args, *p.BitrateMin)
		conds = append(conds, fmt.Sprintf("t.bitrate_kbps >= $%d", argIdx))
		argIdx++
	}
	if p.BitrateMax != nil {
		args = append(args, *p.BitrateMax)
		conds = append(conds, fmt.Sprintf("t.bitrate_kbps <= $%d", argIdx))
		argIdx++
	}
	if p.BPMMin != nil {
		args = append(args, *p.BPMMin)
		conds = append(conds, fmt.Sprintf("tf.bpm >= $%d", argIdx))
		argIdx++
	}
	if p.BPMMax != nil {
		args = append(args, *p.BPMMax)
		conds = append(conds, fmt.Sprintf("tf.bpm <= $%d", argIdx))
		argIdx++
	}

	orderBy := "ts_rank(t.search_vector, websearch_to_tsquery('english', $1)) DESC"
	switch p.SortBy {
	case "title":
		orderBy = "t.title ASC"
	case "year":
		orderBy = "al_f.release_year DESC NULLS LAST"
	case "bitrate":
		orderBy = "t.bitrate_kbps DESC NULLS LAST"
	case "duration":
		orderBy = "t.duration_ms ASC"
	case "bpm":
		orderBy = "tf.bpm ASC NULLS LAST"
	}

	args = append(args, p.Limit)
	q := fmt.Sprintf(
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
LEFT JOIN artists ar ON ar.id = t.artist_id%s
WHERE %s
ORDER BY %s
LIMIT $%d`,
		extraJoins.String(), strings.Join(conds, " AND "), orderBy, argIdx)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// RecordPlay records a track play event for a user.
func (s *Store) RecordPlay(ctx context.Context, p RecordPlayParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO play_history (user_id, track_id, duration_played_ms) VALUES ($1, $2, $3)`,
		p.UserID, p.TrackID, p.DurationPlayedMs)
	return err
}

func (s *Store) ListRecentlyPlayed(ctx context.Context, p ListRecentlyPlayedParams) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT sub.id, sub.album_id, sub.artist_id, sub.title, sub.track_number, sub.track_index, sub.disc_number,
		        sub.duration_ms, sub.file_key, sub.file_size, sub.format, sub.bit_depth,
		        sub.sample_rate, sub.channels, sub.bitrate_kbps, sub.seek_table, sub.fingerprint, sub.created_at, sub.replay_gain_track, sub.bpm, sub.audio_layouts, sub.has_atmos, sub.audio_formats
		FROM (
		  SELECT DISTINCT ON (ph.track_id)
		    t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		    t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		    t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		    COALESCE(tf.replay_gain, 0) AS replay_gain_track,
		    COALESCE(tf.bpm, 0) AS bpm,
		    t.audio_layouts, t.has_atmos, t.audio_formats,
		    ph.played_at
		  FROM play_history ph
		  JOIN tracks t ON t.id = ph.track_id
		  LEFT JOIN track_features tf ON tf.track_id = t.id
		  WHERE ph.user_id = $1
		    AND ($3::timestamptz IS NULL OR ph.played_at >= $3)
		    AND ($4::timestamptz IS NULL OR ph.played_at < $4)
		  ORDER BY ph.track_id, ph.played_at DESC
		) sub
		ORDER BY sub.played_at DESC
		LIMIT $2`,
		p.UserID, p.Limit, p.From, p.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// ListMostPlayed returns the most-played tracks for a user in the given date range.
func (s *Store) ListMostPlayed(ctx context.Context, p ListMostPlayedParams) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		        t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
		FROM tracks t
		LEFT JOIN track_features tf ON tf.track_id = t.id
		JOIN (
		  SELECT track_id, COUNT(*) AS play_count
		  FROM play_history
		  WHERE user_id = $1
		    AND ($3::timestamptz IS NULL OR played_at >= $3)
		    AND ($4::timestamptz IS NULL OR played_at < $4)
		  GROUP BY track_id
		) ph ON ph.track_id = t.id
		ORDER BY ph.play_count DESC
		LIMIT $2`,
		p.UserID, p.Limit, p.From, p.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// UpsertTrack inserts or updates a track.
func (s *Store) UpsertTrack(ctx context.Context, p UpsertTrackParams) (Track, error) {
	var t Track
	layouts := p.AudioLayouts
	if len(layouts) == 0 {
		layouts = []string{"stereo"}
	}
	row := s.pool.QueryRow(ctx, `INSERT INTO tracks (
 id, album_id, artist_id, title, track_number, track_index, disc_number,
 duration_ms, file_key, file_size, format, bit_depth,
 sample_rate, channels, audio_layouts, bitrate_kbps, seek_table, fingerprint)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
ON CONFLICT (id) DO UPDATE SET album_id = EXCLUDED.album_id, artist_id = EXCLUDED.artist_id, title = EXCLUDED.title, track_number = EXCLUDED.track_number, track_index = EXCLUDED.track_index, disc_number = EXCLUDED.disc_number, duration_ms = EXCLUDED.duration_ms, file_key = EXCLUDED.file_key, file_size = EXCLUDED.file_size, format = EXCLUDED.format, bit_depth = EXCLUDED.bit_depth, sample_rate = EXCLUDED.sample_rate, channels = EXCLUDED.channels, audio_layouts = EXCLUDED.audio_layouts, bitrate_kbps = EXCLUDED.bitrate_kbps, seek_table = EXCLUDED.seek_table, fingerprint = EXCLUDED.fingerprint RETURNING id, album_id, artist_id, title, track_number, track_index, disc_number, duration_ms, file_key, file_size, format, bit_depth, sample_rate, channels, bitrate_kbps, seek_table, fingerprint, created_at`,
		p.ID, p.AlbumID, p.ArtistID, p.Title, p.TrackNumber, p.TrackIndex, p.DiscNumber, p.DurationMs, p.FileKey, p.FileSize, p.Format, p.BitDepth, p.SampleRate, p.Channels, layouts, p.BitrateKbps, p.SeekTable, p.Fingerprint)
	var albumID, artistID, format sql.NullString
	var trackNumber, trackIndex, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprint sql.NullString
	var createdAt time.Time
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &trackIndex, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprint, &createdAt)
	if albumID.Valid {
		t.AlbumID = &albumID.String
	}
	if artistID.Valid {
		t.ArtistID = &artistID.String
	}
	if trackNumber.Valid {
		n := int(trackNumber.Int64)
		t.TrackNumber = &n
	}
	if trackIndex.Valid {
		n := int(trackIndex.Int64)
		t.TrackIndex = &n
	}
	t.DiscNumber = int(discNumber.Int64)
	t.DurationMs = int(durationMs.Int64)
	if fileKey.Valid {
		t.FileKey = fileKey.String
	}
	t.FileSize = fileSize.Int64
	if format.Valid {
		t.Format = format.String
	}
	if bitDepth.Valid {
		n := int(bitDepth.Int64)
		t.BitDepth = &n
	}
	t.SampleRate = int(sampleRate.Int64)
	t.Channels = int(channels.Int64)
	if bitrateKbps.Valid {
		n := int(bitrateKbps.Int64)
		t.BitrateKbps = &n
	}
	t.SeekTable = seekTable
	if fingerprint.Valid {
		t.Fingerprint = fingerprint.String
	}
	t.CreatedAt = createdAt
	return t, err
}

// AddTrackToLibrary adds a track to a user's library.
func (s *Store) AddTrackToLibrary(ctx context.Context, p AddTrackToLibraryParams) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_library (user_id, track_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, p.UserID, p.TrackID)
	return err
}

// ingestStateSchema is the canonical DDL for the ingest_state table.
// LoadIngestState recreates the table from this if it detects a schema mismatch,
// so the ingest tool survives upgrades without requiring a manual volume wipe.
const ingestStateSchema = `
CREATE TABLE ingest_state (
    path        TEXT        PRIMARY KEY,
    mtime_unix  BIGINT      NOT NULL,
    file_size   BIGINT      NOT NULL,
    track_id    TEXT        NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// LoadIngestState returns all rows from ingest_state as a slice.
// The ingest tool calls this once at startup and keeps the result in memory,
// so no per-file DB queries are needed during a scan.
//
// If the table has a stale schema (e.g. after an upgrade), it is dropped and
// recreated automatically. All files will be treated as new on that run, but
// UpsertTrack is idempotent so no data is lost.
func (s *Store) LoadIngestState(ctx context.Context) ([]IngestStateRow, error) {
	const q = `SELECT path, mtime_unix, file_size, track_id FROM ingest_state`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		// SQLSTATE 42703 = undefined_column, 42P01 = undefined_table.
		// Both indicate a schema mismatch — recreate and return empty state.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "42703" || pgErr.Code == "42P01") {
			if _, err2 := s.pool.Exec(ctx, `DROP TABLE IF EXISTS ingest_state`); err2 != nil {
				return nil, fmt.Errorf("drop stale ingest_state: %w", err2)
			}
			if _, err2 := s.pool.Exec(ctx, ingestStateSchema); err2 != nil {
				return nil, fmt.Errorf("recreate ingest_state: %w", err2)
			}
			return nil, nil // empty state; all files will be re-ingested this run
		}
		return nil, err
	}
	defer rows.Close()

	var out []IngestStateRow
	for rows.Next() {
		var r IngestStateRow
		if err := rows.Scan(&r.Path, &r.MtimeUnix, &r.FileSize, &r.TrackID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertIngestState records (or updates) a file's ingest state.
func (s *Store) UpsertIngestState(ctx context.Context, r IngestStateRow) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO ingest_state (path, mtime_unix, file_size, track_id, ingested_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (path) DO UPDATE
    SET mtime_unix  = EXCLUDED.mtime_unix,
        file_size   = EXCLUDED.file_size,
        track_id    = EXCLUDED.track_id,
        ingested_at = EXCLUDED.ingested_at`,
		r.Path, r.MtimeUnix, r.FileSize, r.TrackID)
	return err
}

// BatchUpsertIngestState bulk-upserts ingest state rows in a single round trip.
func (s *Store) BatchUpsertIngestState(ctx context.Context, rows []IngestStateRow) error {
	if len(rows) == 0 {
		return nil
	}
	// Deduplicate by path (last writer wins) so that PostgreSQL's
	// ON CONFLICT DO UPDATE never sees the same constrained value twice
	// within the same statement, which it rejects with SQLSTATE 21000.
	seen := make(map[string]int, len(rows))
	deduped := rows[:0:0]
	for _, r := range rows {
		if idx, ok := seen[r.Path]; ok {
			deduped[idx] = r
		} else {
			seen[r.Path] = len(deduped)
			deduped = append(deduped, r)
		}
	}
	query := `INSERT INTO ingest_state (path, mtime_unix, file_size, track_id, ingested_at) VALUES `
	args := make([]any, 0, len(deduped)*4)
	for i, r := range deduped {
		if i > 0 {
			query += ", "
		}
		n := i * 4
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, now())", n+1, n+2, n+3, n+4)
		args = append(args, r.Path, r.MtimeUnix, r.FileSize, r.TrackID)
	}
	query += ` ON CONFLICT (path) DO UPDATE SET mtime_unix = EXCLUDED.mtime_unix, file_size = EXCLUDED.file_size, track_id = EXCLUDED.track_id, ingested_at = EXCLUDED.ingested_at`
	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

// IngestStatePath pairs a filesystem path with its associated track ID.
type IngestStatePath struct {
	Path    string
	TrackID string
}

// DeleteIngestStateForAlbum removes ingest_state rows for all tracks belonging
// to the given album and returns the path/trackID pairs that were deleted.
// Call this before a targeted rescan so the files are not skipped by upToDate.
func (s *Store) DeleteIngestStateForAlbum(ctx context.Context, albumID string) ([]IngestStatePath, error) {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM ingest_state
		 WHERE track_id IN (SELECT id FROM tracks WHERE album_id = $1)
		 RETURNING path, track_id`,
		albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IngestStatePath
	for rows.Next() {
		var r IngestStatePath
		if err := rows.Scan(&r.Path, &r.TrackID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// DeleteTracksAndCleanup removes the given tracks by ID and cascades to clean
// up any albums and artists that become empty as a result. Returns object-store
// keys (track file_key + album cover_art_key + artist image_key) for callers
// to delete from the object store.
func (s *Store) DeleteTracksAndCleanup(ctx context.Context, trackIDs []string) ([]string, error) {
	if len(trackIDs) == 0 {
		return nil, nil
	}

	var objKeys []string

	// Collect file keys before deletion.
	fkRows, err := s.pool.Query(ctx, `SELECT file_key FROM tracks WHERE id = ANY($1)`, trackIDs)
	if err == nil {
		for fkRows.Next() {
			var k string
			if fkRows.Scan(&k) == nil && k != "" {
				objKeys = append(objKeys, k)
			}
		}
		fkRows.Close()
	}

	if _, err := s.pool.Exec(ctx, `DELETE FROM tracks WHERE id = ANY($1)`, trackIDs); err != nil {
		return nil, fmt.Errorf("delete tracks: %w", err)
	}

	// Remove albums that have no remaining tracks.
	albumRows, err := s.pool.Query(ctx,
		`DELETE FROM albums
		 WHERE id NOT IN (SELECT DISTINCT album_id FROM tracks WHERE album_id IS NOT NULL)
		 RETURNING cover_art_key`)
	if err == nil {
		for albumRows.Next() {
			var k sql.NullString
			if albumRows.Scan(&k) == nil && k.Valid && k.String != "" {
				objKeys = append(objKeys, k.String)
			}
		}
		albumRows.Close()
	}

	// Remove artists with no remaining tracks, albums, or audiobooks.
	artistRows, err := s.pool.Query(ctx,
		`DELETE FROM artists
		 WHERE id NOT IN (
		     SELECT DISTINCT artist_id FROM tracks     WHERE artist_id IS NOT NULL
		     UNION
		     SELECT DISTINCT artist_id FROM albums     WHERE artist_id IS NOT NULL
		     UNION
		     SELECT DISTINCT author_id  FROM audiobooks WHERE author_id IS NOT NULL
		 )
		 RETURNING image_key`)
	if err == nil {
		for artistRows.Next() {
			var k sql.NullString
			if artistRows.Scan(&k) == nil && k.Valid && k.String != "" {
				objKeys = append(objKeys, k.String)
			}
		}
		artistRows.Close()
	}

	return objKeys, nil
}

// PruneOrphanedTracks removes tracks whose source files are no longer present
// on disk. foundPaths is the complete set of audio file paths collected during
// the current scan. Ingest-state rows whose paths are absent from that set are
// considered orphaned. The method also removes empty albums and artists left
// behind after deletion, and returns the object-store keys that callers should
// delete.
func (s *Store) PruneOrphanedTracks(ctx context.Context, foundPaths []string) (int, []string, error) {
	foundSet := make(map[string]struct{}, len(foundPaths))
	for _, p := range foundPaths {
		foundSet[p] = struct{}{}
	}

	stateRows, err := s.LoadIngestState(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("prune tracks: load ingest state: %w", err)
	}

	var orphanPaths []string
	var orphanTrackIDs []string
	for _, r := range stateRows {
		if _, ok := foundSet[r.Path]; !ok {
			orphanPaths = append(orphanPaths, r.Path)
			if r.TrackID != "" {
				orphanTrackIDs = append(orphanTrackIDs, r.TrackID)
			}
		}
	}
	if len(orphanTrackIDs) == 0 {
		return 0, nil, nil
	}

	// Collect track file_keys before deletion for caller objstore cleanup.
	var objKeys []string
	fkRows, err := s.pool.Query(ctx, `SELECT file_key FROM tracks WHERE id = ANY($1)`, orphanTrackIDs)
	if err == nil {
		for fkRows.Next() {
			var k string
			if fkRows.Scan(&k) == nil && k != "" {
				objKeys = append(objKeys, k)
			}
		}
		fkRows.Close()
	}

	// Remove orphaned ingest-state entries.
	if _, err := s.pool.Exec(ctx, `DELETE FROM ingest_state WHERE path = ANY($1)`, orphanPaths); err != nil {
		return 0, nil, fmt.Errorf("prune ingest_state: %w", err)
	}

	// Delete the tracks (cascades to playlist_tracks, play_history, etc.).
	if _, err := s.pool.Exec(ctx, `DELETE FROM tracks WHERE id = ANY($1)`, orphanTrackIDs); err != nil {
		return 0, nil, fmt.Errorf("prune orphaned tracks: %w", err)
	}

	// Delete albums that have no remaining tracks and collect their cover art keys.
	albumRows, err := s.pool.Query(ctx,
		`DELETE FROM albums
		 WHERE id NOT IN (SELECT DISTINCT album_id FROM tracks WHERE album_id IS NOT NULL)
		 RETURNING cover_art_key`)
	if err == nil {
		for albumRows.Next() {
			var k sql.NullString
			if albumRows.Scan(&k) == nil && k.Valid && k.String != "" {
				objKeys = append(objKeys, k.String)
			}
		}
		albumRows.Close()
	}

	// Delete artists with no remaining tracks, albums, or audiobooks.
	// Also collect their image keys.
	artistRows, err := s.pool.Query(ctx,
		`DELETE FROM artists
		 WHERE id NOT IN (
		     SELECT DISTINCT artist_id FROM tracks   WHERE artist_id IS NOT NULL
		     UNION
		     SELECT DISTINCT artist_id FROM albums   WHERE artist_id IS NOT NULL
		     UNION
		     SELECT DISTINCT author_id  FROM audiobooks WHERE author_id IS NOT NULL
		 )
		 RETURNING image_key`)
	if err == nil {
		for artistRows.Next() {
			var k sql.NullString
			if artistRows.Scan(&k) == nil && k.Valid && k.String != "" {
				objKeys = append(objKeys, k.String)
			}
		}
		artistRows.Close()
	}

	return len(orphanTrackIDs), objKeys, nil
}

// GetTrackByFingerprint returns a track by fingerprint.
func (s *Store) GetTrackByFingerprint(ctx context.Context, fingerprint string) (Track, error) {
	var t Track
	row := s.pool.QueryRow(ctx, `SELECT id, album_id, artist_id, title, track_number, track_index, disc_number, duration_ms, file_key, file_size, format, bit_depth, sample_rate, channels, bitrate_kbps, seek_table, fingerprint, created_at FROM tracks WHERE fingerprint = $1`, fingerprint)
	var albumID, artistID, format sql.NullString
	var trackNumber, trackIndex, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprintVal sql.NullString
	var createdAt time.Time
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &trackIndex, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprintVal, &createdAt)
	if albumID.Valid {
		t.AlbumID = &albumID.String
	}
	if artistID.Valid {
		t.ArtistID = &artistID.String
	}
	if trackNumber.Valid {
		n := int(trackNumber.Int64)
		t.TrackNumber = &n
	}
	if trackIndex.Valid {
		n := int(trackIndex.Int64)
		t.TrackIndex = &n
	}
	t.DiscNumber = int(discNumber.Int64)
	t.DurationMs = int(durationMs.Int64)
	if fileKey.Valid {
		t.FileKey = fileKey.String
	}
	t.FileSize = fileSize.Int64
	if format.Valid {
		t.Format = format.String
	}
	if bitDepth.Valid {
		n := int(bitDepth.Int64)
		t.BitDepth = &n
	}
	t.SampleRate = int(sampleRate.Int64)
	t.Channels = int(channels.Int64)
	if bitrateKbps.Valid {
		n := int(bitrateKbps.Int64)
		t.BitrateKbps = &n
	}
	t.SeekTable = seekTable
	if fingerprintVal.Valid {
		t.Fingerprint = fingerprintVal.String
	}
	t.CreatedAt = createdAt
	return t, err
}

// ── Favorites ──────────────────────────────────────────────────────────────

func (s *Store) AddFavorite(ctx context.Context, p FavoriteParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_favorites (user_id, track_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		p.UserID, p.TrackID)
	return err
}

func (s *Store) RemoveFavorite(ctx context.Context, p FavoriteParams) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_favorites WHERE user_id = $1 AND track_id = $2`,
		p.UserID, p.TrackID)
	return err
}

// ListFavorites returns all favorited tracks for a user, newest first.
func (s *Store) ListFavorites(ctx context.Context, userID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
JOIN user_favorites uf ON uf.track_id = t.id
WHERE uf.user_id = $1
ORDER BY uf.added_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// ListFavoriteIDs returns all favorited track IDs for a user.
func (s *Store) ListFavoriteIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT track_id FROM user_favorites WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ── Ratings ────────────────────────────────────────────────────────────────

// SetRating upserts a 1–5 star rating for a track.
func (s *Store) SetRating(ctx context.Context, p RateTrackParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO track_ratings (user_id, track_id, rating)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, track_id) DO UPDATE SET rating = EXCLUDED.rating, rated_at = now()`,
		p.UserID, p.TrackID, p.Rating)
	return err
}

// DeleteRating removes a track rating for a user.
func (s *Store) DeleteRating(ctx context.Context, userID, trackID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM track_ratings WHERE user_id = $1 AND track_id = $2`,
		userID, trackID)
	return err
}

// ListRatings returns a map of track_id → rating for all rated tracks of a user.
func (s *Store) ListRatings(ctx context.Context, userID string) (map[string]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT track_id, rating FROM track_ratings WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int)
	for rows.Next() {
		var trackID string
		var rating int
		if err := rows.Scan(&trackID, &rating); err != nil {
			return nil, err
		}
		result[trackID] = rating
	}
	return result, rows.Err()
}

// ── Waveform & Lyrics ─────────────────────────────────────────────────────

// UpsertTrackWaveform stores pre-generated waveform peak data for a track.
func (s *Store) UpsertTrackWaveform(ctx context.Context, trackID string, peaks []float32) error {
	b, err := json.Marshal(peaks)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE tracks SET waveform_peaks = $1 WHERE id = $2`, b, trackID)
	return err
}

// GetTrackWaveform returns pre-generated waveform peaks for a track.
// Returns nil peaks (no error) when waveform data has not been generated yet.
func (s *Store) GetTrackWaveform(ctx context.Context, trackID string) ([]float32, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT waveform_peaks FROM tracks WHERE id = $1`, trackID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var peaks []float32
	if err := json.Unmarshal(raw, &peaks); err != nil {
		return nil, err
	}
	return peaks, nil
}

// GetTrackLyrics returns the raw LRC lyrics string for a track.
// Returns an empty string if the track exists but has no lyrics set.
func (s *Store) GetTrackLyrics(ctx context.Context, trackID string) (string, error) {
	var lyrics sql.NullString
	err := s.pool.QueryRow(ctx, `SELECT lyrics FROM tracks WHERE id = $1`, trackID).Scan(&lyrics)
	if err != nil {
		return "", err
	}
	return lyrics.String, nil
}

// SetTrackLyrics stores LRC lyrics text for a track.
func (s *Store) SetTrackLyrics(ctx context.Context, trackID, lyrics string) error {
	_, err := s.pool.Exec(ctx, `UPDATE tracks SET lyrics = $1 WHERE id = $2`, lyrics, trackID)
	return err
}

// UpdateTrackEnrichment updates a track with MusicBrainz metadata.
func (s *Store) UpdateTrackEnrichment(ctx context.Context, p UpdateTrackEnrichmentParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tracks SET mbid = COALESCE($2, mbid), isrc = $3, enriched_at = now() WHERE id = $1`,
		p.ID, p.Mbid, p.Isrc)
	return err
}

// ── Genre methods ──────────────────────────────────────────────────────────

// UpsertGenre inserts a genre or does nothing if it already exists.
func (s *Store) UpsertGenre(ctx context.Context, id, name string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO genres (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		id, name)
	return err
}

// SetTrackFeaturedArtists replaces all featured-artist associations for a track.
// artistIDs are stored in order (position = index).
func (s *Store) SetTrackFeaturedArtists(ctx context.Context, trackID string, artistIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)
	if _, err := tx.Exec(ctx, `DELETE FROM track_featured_artists WHERE track_id = $1`, trackID); err != nil {
		return err
	}
	for i, aid := range artistIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO track_featured_artists (track_id, artist_id, position) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			trackID, aid, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListFeaturedArtistsByTrack returns featured artists for a single track.
func (s *Store) ListFeaturedArtistsByTrack(ctx context.Context, trackID string) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.name, a.sort_name, a.mbid, a.created_at
		 FROM artists a
		 JOIN track_featured_artists tfa ON tfa.artist_id = a.id
		 WHERE tfa.track_id = $1
		 ORDER BY tfa.position ASC`,
		trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtists(rows)
}

// ListFeaturedArtistsByTracks returns featured artists for a batch of tracks,
// keyed by track ID. A single query is issued for the whole batch.
func (s *Store) ListFeaturedArtistsByTracks(ctx context.Context, trackIDs []string) (map[string][]Artist, error) {
	if len(trackIDs) == 0 {
		return map[string][]Artist{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT tfa.track_id, a.id, a.name, a.sort_name, a.mbid, a.created_at
		 FROM track_featured_artists tfa
		 JOIN artists a ON a.id = tfa.artist_id
		 WHERE tfa.track_id = ANY($1)
		 ORDER BY tfa.track_id, tfa.position ASC`,
		trackIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]Artist)
	for rows.Next() {
		var trackID string
		var a Artist
		var mbid sql.NullString
		if err := rows.Scan(&trackID, &a.ID, &a.Name, &a.SortName, &mbid, &a.CreatedAt); err != nil {
			return nil, err
		}
		if mbid.Valid {
			a.Mbid = &mbid.String
		}
		result[trackID] = append(result[trackID], a)
	}
	return result, rows.Err()
}

// SetTrackGenres replaces all genre associations for a track.
func (s *Store) SetTrackGenres(ctx context.Context, trackID string, genreIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)
	if _, err := tx.Exec(ctx, `DELETE FROM track_genres WHERE track_id = $1`, trackID); err != nil {
		return err
	}
	for _, gid := range genreIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO track_genres (track_id, genre_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, trackID, gid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListGenresByTrack returns all genres for a track.
func (s *Store) ListGenresByTrack(ctx context.Context, trackID string) ([]Genre, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT g.id, g.name FROM genres g JOIN track_genres tg ON tg.genre_id = g.id WHERE tg.track_id = $1 ORDER BY g.name`,
		trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGenres(rows)
}

// ListGenres returns all genres ordered by name.
func (s *Store) ListGenres(ctx context.Context) ([]Genre, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name FROM genres ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGenres(rows)
}

func scanGenres(rows pgx.Rows) ([]Genre, error) {
	out := make([]Genre, 0)
	for rows.Next() {
		var g Genre
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGenreByID returns a single genre by ID.
func (s *Store) GetGenreByID(ctx context.Context, id string) (Genre, error) {
	var g Genre
	err := s.pool.QueryRow(ctx, `SELECT id, name FROM genres WHERE id = $1`, id).Scan(&g.ID, &g.Name)
	return g, err
}

// ListUnenrichedTracks returns tracks that haven't been enriched yet.
// If force is true, returns all tracks regardless of enrichment status.
func (s *Store) ListUnenrichedTracks(ctx context.Context, limit int, force bool) ([]Track, error) {
	q := `SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id`
	if !force {
		q += ` WHERE t.enriched_at IS NULL`
	}
	q += ` ORDER BY t.title LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// ── Track similarity & recommendation methods ─────────────────────────────

// UpsertTrackFeatures stores in-house audio features for a track.
func (s *Store) UpsertTrackFeatures(ctx context.Context, trackID string, bpm float64, keyEstimate string, replayGain float64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO track_features (track_id, bpm, key_estimate, replay_gain)
		 VALUES ($1, NULLIF($2, 0), NULLIF($3, ''), NULLIF($4, 0))
		 ON CONFLICT (track_id) DO UPDATE
		   SET bpm          = COALESCE(NULLIF($2, 0), track_features.bpm),
		       key_estimate = COALESCE(NULLIF($3, ''), track_features.key_estimate),
		       replay_gain  = COALESCE(NULLIF($4, 0), track_features.replay_gain),
		       extracted_at = now()`,
		trackID, bpm, keyEstimate, replayGain)
	return err
}

// ListAllTrackFeatures returns all rows from track_features.
func (s *Store) ListAllTrackFeatures(ctx context.Context) ([]TrackFeatures, error) {
	rows, err := s.pool.Query(ctx, `SELECT track_id, COALESCE(bpm, 0), COALESCE(key_estimate, ''), COALESCE(replay_gain, 0) FROM track_features`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackFeatures
	for rows.Next() {
		var f TrackFeatures
		if err := rows.Scan(&f.TrackID, &f.BPM, &f.KeyEstimate, &f.ReplayGain); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListAllTrackFeaturesMap returns track features indexed by track ID for bulk
// similarity computation. Tracks without a features row are omitted.
func (s *Store) ListAllTrackFeaturesMap(ctx context.Context) (map[string]TrackFeatures, error) {
	all, err := s.ListAllTrackFeatures(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]TrackFeatures, len(all))
	for _, f := range all {
		m[f.TrackID] = f
	}
	return m, nil
}

// ListAllTracks returns paginated tracks ordered by title (for DLNA browsing).
func (s *Store) ListAllTracks(ctx context.Context, limit, offset int32) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm, t.audio_layouts, t.has_atmos, t.audio_formats
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
ORDER BY t.title ASC
LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// ListAllTracksBasic returns minimal track info for bulk similarity computation.
func (s *Store) ListAllTracksBasic(ctx context.Context) ([]TrackBasic, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, COALESCE(artist_id, ''), COALESCE(album_id, ''), duration_ms FROM tracks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackBasic
	for rows.Next() {
		var t TrackBasic
		if err := rows.Scan(&t.ID, &t.ArtistID, &t.AlbumID, &t.DurationMs); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllTrackGenresMap returns a map of track_id → []genre_id.
func (s *Store) ListAllTrackGenresMap(ctx context.Context) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT track_id, genre_id FROM track_genres`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string][]string)
	for rows.Next() {
		var tid, gid string
		if err := rows.Scan(&tid, &gid); err != nil {
			return nil, err
		}
		m[tid] = append(m[tid], gid)
	}
	return m, rows.Err()
}

// ListAllAlbumGenresMap returns a map of album_id → []genre_id.
func (s *Store) ListAllAlbumGenresMap(ctx context.Context) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT album_id, genre_id FROM album_genres`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string][]string)
	for rows.Next() {
		var aid, gid string
		if err := rows.Scan(&aid, &gid); err != nil {
			return nil, err
		}
		m[aid] = append(m[aid], gid)
	}
	return m, rows.Err()
}

// ListAllArtistGenresMap returns a map of artist_id → []genre_id.
func (s *Store) ListAllArtistGenresMap(ctx context.Context) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT artist_id, genre_id FROM artist_genres`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string][]string)
	for rows.Next() {
		var aid, gid string
		if err := rows.Scan(&aid, &gid); err != nil {
			return nil, err
		}
		m[aid] = append(m[aid], gid)
	}
	return m, rows.Err()
}

// ListAllTrackInfosFull returns all tracks with the album and artist metadata
// needed for the multi-signal similarity algorithm. A single joined query is
// used to minimise round-trips.
func (s *Store) ListAllTrackInfosFull(ctx context.Context) ([]TrackInfoFull, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id,
			COALESCE(t.artist_id, '')     AS artist_id,
			COALESCE(t.album_id, '')      AS album_id,
			t.title,
			t.duration_ms,
			t.format,
			COALESCE(t.bit_depth, 0)      AS bit_depth,
			t.sample_rate,
			t.channels,
			COALESCE(t.bitrate_kbps, 0)   AS bitrate_kbps,
			COALESCE(al.release_year, 0)  AS release_year,
			COALESCE(al.album_type, '')   AS album_type,
			COALESCE(al.album_group_id, '') AS album_group_id,
			COALESCE(ar.country, '')      AS country,
			COALESCE(ar.artist_type, '')  AS artist_type
		FROM tracks t
		LEFT JOIN albums  al ON al.id = t.album_id
		LEFT JOIN artists ar ON ar.id = t.artist_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackInfoFull
	for rows.Next() {
		var f TrackInfoFull
		if err := rows.Scan(
			&f.ID, &f.ArtistID, &f.AlbumID, &f.Title, &f.DurationMs,
			&f.Format, &f.BitDepth, &f.SampleRate, &f.Channels, &f.BitrateKbps,
			&f.ReleaseYear, &f.AlbumType, &f.AlbumGroupID, &f.Country, &f.ArtistType,
		); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListAllFeaturedArtistsMap returns a map of track_id → []artistID for every
// track that has featured-artist entries.
func (s *Store) ListAllFeaturedArtistsMap(ctx context.Context) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT track_id, artist_id FROM track_featured_artists ORDER BY track_id, position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var trackID, artistID string
		if err := rows.Scan(&trackID, &artistID); err != nil {
			return nil, err
		}
		out[trackID] = append(out[trackID], artistID)
	}
	return out, rows.Err()
}

// ListCoPlayCounts returns canonical (track_a < track_b) pairs of tracks that
// were played by the same user within a 30-minute window, together with the
// count of distinct users that co-played them.  Only pairs played by at least
// one user are returned; pairs with very few co-plays still contribute a small
// behavioral signal.
func (s *Store) ListCoPlayCounts(ctx context.Context) ([]CoPlayPair, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			ph1.track_id  AS track_a,
			ph2.track_id  AS track_b,
			COUNT(DISTINCT ph1.user_id) AS coplay_count
		FROM play_history ph1
		JOIN play_history ph2
			ON  ph2.user_id  = ph1.user_id
			AND ph2.track_id > ph1.track_id
			AND ph2.played_at BETWEEN ph1.played_at - INTERVAL '30 minutes'
			                      AND ph1.played_at + INTERVAL '30 minutes'
		GROUP BY ph1.track_id, ph2.track_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoPlayPair
	for rows.Next() {
		var cp CoPlayPair
		if err := rows.Scan(&cp.TrackA, &cp.TrackB, &cp.Count); err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, rows.Err()
}

// BatchUpsertSimilarity bulk-inserts similarity rows. Uses a batch for efficiency.
func (s *Store) BatchUpsertSimilarity(ctx context.Context, rows []TrackSimilarityRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)

	// Clear existing data and re-insert (faster than individual ON CONFLICT for full recompute).
	if _, err := tx.Exec(ctx, `DELETE FROM track_similarity`); err != nil {
		return err
	}

	const batchSize = 500
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		query := `INSERT INTO track_similarity (track_a, track_b, score) VALUES `
		args := make([]any, 0, len(batch)*3)
		for j, r := range batch {
			if j > 0 {
				query += ", "
			}
			n := j * 3
			query += fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3)
			args = append(args, r.TrackA, r.TrackB, r.Score)
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// HasSimilarityData reports whether the track_similarity table contains any rows.
// Used to decide between a full recompute (empty table) and an incremental update.
func (s *Store) HasSimilarityData(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM track_similarity LIMIT 1)`).Scan(&exists)
	return exists, err
}

// UpsertSimilarityIncremental inserts or updates similarity rows without clearing
// the whole table first. Used for incremental updates when only a subset of
// tracks has changed. Existing pairs that are not in `rows` are left untouched.
func (s *Store) UpsertSimilarityIncremental(ctx context.Context, rows []TrackSimilarityRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)

	const batchSize = 500
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		query := `INSERT INTO track_similarity (track_a, track_b, score) VALUES `
		args := make([]any, 0, len(batch)*3)
		for j, r := range batch {
			if j > 0 {
				query += ", "
			}
			n := j * 3
			query += fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3)
			args = append(args, r.TrackA, r.TrackB, r.Score)
		}
		query += ` ON CONFLICT (track_a, track_b) DO UPDATE SET score = EXCLUDED.score`
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// ListSimilarTracks returns tracks similar to the given track, ordered by score.
// If excludeAlbumID is non-empty, tracks belonging to that album are excluded.
func (s *Store) ListSimilarTracks(ctx context.Context, trackID string, limit int, excludeAlbumID string) ([]TrackWithScore, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if excludeAlbumID != "" {
		rows, err = s.pool.Query(ctx,
			`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
			        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
			        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
			        COALESCE(tf.replay_gain, 0) AS replay_gain_track, s.score, ar.name AS artist_name,
			        al.title AS album_name, al.cover_art_key
			 FROM (
			     SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = $1
			     UNION ALL
			     SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = $1
			 ) s
			 JOIN tracks t ON t.id = s.similar_id
			 LEFT JOIN track_features tf ON tf.track_id = t.id
			 LEFT JOIN artists ar ON ar.id = t.artist_id
			 LEFT JOIN albums al ON al.id = t.album_id
			 WHERE t.album_id != $3
			 ORDER BY s.score DESC
			 LIMIT $2`,
			trackID, limit, excludeAlbumID)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
			        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
			        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
			        COALESCE(tf.replay_gain, 0) AS replay_gain_track, s.score, ar.name AS artist_name,
			        al.title AS album_name, al.cover_art_key
			 FROM (
			     SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = $1
			     UNION ALL
			     SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = $1
			 ) s
			 JOIN tracks t ON t.id = s.similar_id
			 LEFT JOIN track_features tf ON tf.track_id = t.id
			 LEFT JOIN artists ar ON ar.id = t.artist_id
			 LEFT JOIN albums al ON al.id = t.album_id
			 ORDER BY s.score DESC
			 LIMIT $2`,
			trackID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

// RecommendForUser returns personalized recommendations based on recent listening.
func (s *Store) RecommendForUser(ctx context.Context, userID string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`WITH recent AS (
		     SELECT DISTINCT ON (track_id) track_id
		     FROM play_history
		     WHERE user_id = $1
		     ORDER BY track_id, played_at DESC
		     LIMIT 20
		 ),
		 candidates AS (
		     SELECT s.similar_id, MAX(s.score) AS score
		     FROM recent r
		     CROSS JOIN LATERAL (
		         SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = r.track_id
		         UNION ALL
		         SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = r.track_id
		     ) s
		     WHERE s.similar_id NOT IN (SELECT track_id FROM recent)
		       AND s.similar_id NOT IN (
		           SELECT track_id FROM play_history
		           WHERE user_id = $1 AND played_at > now() - interval '24 hours'
		       )
		     GROUP BY s.similar_id
		 )
		 SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, c.score, ar.name AS artist_name,
		        al.title AS album_name, al.cover_art_key
		 FROM candidates c
		 JOIN tracks t ON t.id = c.similar_id
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 LEFT JOIN albums al ON al.id = t.album_id
		 ORDER BY c.score DESC
		 LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

// AutoplayAfter returns tracks to auto-play after the given track, excluding
// the provided track IDs (already in queue). When no pre-computed similarity
// data exists it falls back to tracks by the same artist, then random tracks
// from the user's library.
func (s *Store) AutoplayAfter(ctx context.Context, userID, trackID string, exclude []string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, s.score, ar.name AS artist_name,
		        al.title AS album_name, al.cover_art_key
		 FROM (
		     SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = $1
		     UNION ALL
		     SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = $1
		 ) s
		 JOIN tracks t ON t.id = s.similar_id
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 LEFT JOIN albums al ON al.id = t.album_id
		 WHERE t.id != ALL($2::text[])
		 ORDER BY s.score DESC, random()
		 LIMIT $3`,
		trackID, exclude, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result, err := scanTracksWithScore(rows)
	if err != nil {
		return nil, err
	}
	if len(result) > 0 {
		return result, nil
	}

	// Fallback: tracks by the same artist, ordered randomly.
	result, err = s.autoplayFallbackArtist(ctx, trackID, exclude, limit)
	if err != nil {
		return nil, err
	}
	if len(result) > 0 {
		return result, nil
	}

	// Last resort: random tracks from the user's library.
	return s.autoplayFallbackRandom(ctx, userID, exclude, limit)
}

// autoplayFallbackArtist returns random tracks by the same artist as trackID.
func (s *Store) autoplayFallbackArtist(ctx context.Context, trackID string, exclude []string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, 0::float8 AS score, ar.name AS artist_name,
		        al.title AS album_name, al.cover_art_key
		 FROM tracks t
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 LEFT JOIN albums al ON al.id = t.album_id
		 WHERE t.artist_id = (SELECT artist_id FROM tracks WHERE id = $1)
		   AND t.id != $1
		   AND t.id != ALL($2::text[])
		 ORDER BY random()
		 LIMIT $3`,
		trackID, exclude, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

// autoplayFallbackRandom returns random tracks from the user's library.
func (s *Store) autoplayFallbackRandom(ctx context.Context, userID string, exclude []string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, 0::float8 AS score, ar.name AS artist_name,
		        al.title AS album_name, al.cover_art_key
		 FROM tracks t
		 JOIN user_library ul ON ul.track_id = t.id AND ul.user_id = $1
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 LEFT JOIN albums al ON al.id = t.album_id
		 WHERE t.id != ALL($2::text[])
		 ORDER BY random()
		 LIMIT $3`,
		userID, exclude, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

// RecommendForArtist returns tracks similar to the given artist's most popular tracks.
func (s *Store) RecommendForArtist(ctx context.Context, artistID, userID string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`WITH seed AS (
		     SELECT t.id AS track_id
		     FROM tracks t
		     LEFT JOIN play_history ph ON ph.track_id = t.id
		     WHERE t.artist_id = $1
		     GROUP BY t.id
		     ORDER BY COUNT(ph.id) DESC
		     LIMIT 5
		 ),
		 candidates AS (
		     SELECT sc.similar_id, MAX(sc.score) AS score
		     FROM seed
		     CROSS JOIN LATERAL (
		         SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = seed.track_id
		         UNION ALL
		         SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = seed.track_id
		     ) sc
		     WHERE sc.similar_id NOT IN (SELECT track_id FROM seed)
		       AND sc.similar_id NOT IN (
		           SELECT track_id FROM play_history
		           WHERE user_id = $2 AND played_at > now() - interval '2 hours'
		       )
		     GROUP BY sc.similar_id
		 )
		 SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.track_index, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, sc.score, ar.name AS artist_name,
		        al.title AS album_name, al.cover_art_key
		 FROM candidates sc
		 JOIN tracks t ON t.id = sc.similar_id
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 LEFT JOIN albums al ON al.id = t.album_id
		 ORDER BY sc.score DESC, random()
		 LIMIT $3`,
		artistID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

// ── Scan helpers ───────────────────────────────────────────────────────────

func scanTracks(rows pgx.Rows) ([]Track, error) {
	out := make([]Track, 0)
	for rows.Next() {
		var t Track
		var albumID, artistID, fileKey, format, fp sql.NullString
		var trackNumber, trackIndex, discNumber, durationMs, fileSize, sampleRate, channels, bitDepth, bitrateKbps sql.NullInt64
		var seekTable []byte
		var createdAt time.Time
		var replayGain, bpm sql.NullFloat64
		var audioLayouts []string
		var hasAtmos sql.NullBool
		var audioFormatsRaw []byte
		if err := rows.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &trackIndex, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fp, &createdAt, &replayGain, &bpm, &audioLayouts, &hasAtmos, &audioFormatsRaw); err != nil {
			return nil, err
		}
		if albumID.Valid {
			t.AlbumID = &albumID.String
		}
		if artistID.Valid {
			t.ArtistID = &artistID.String
		}
		if trackNumber.Valid {
			n := int(trackNumber.Int64)
			t.TrackNumber = &n
		}
		if trackIndex.Valid {
			n := int(trackIndex.Int64)
			t.TrackIndex = &n
		}
		t.DiscNumber = int(discNumber.Int64)
		t.DurationMs = int(durationMs.Int64)
		if fileKey.Valid {
			t.FileKey = fileKey.String
		}
		t.FileSize = fileSize.Int64
		if format.Valid {
			t.Format = format.String
		}
		if bitDepth.Valid {
			n := int(bitDepth.Int64)
			t.BitDepth = &n
		}
		t.SampleRate = int(sampleRate.Int64)
		t.Channels = int(channels.Int64)
		if bitrateKbps.Valid {
			n := int(bitrateKbps.Int64)
			t.BitrateKbps = &n
		}
		t.SeekTable = seekTable
		if fp.Valid {
			t.Fingerprint = fp.String
		}
		if replayGain.Valid && replayGain.Float64 != 0 {
			rg := replayGain.Float64
			t.ReplayGainTrack = &rg
		}
		if bpm.Valid && bpm.Float64 != 0 {
			v := bpm.Float64
			t.BPM = &v
		}
		t.CreatedAt = createdAt
		if len(audioLayouts) > 0 {
			t.AudioLayouts = audioLayouts
		}
		if hasAtmos.Valid {
			t.HasAtmos = hasAtmos.Bool
		}
		if len(audioFormatsRaw) > 0 {
			_ = json.Unmarshal(audioFormatsRaw, &t.AudioFormats)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanTracksWithScore(rows pgx.Rows) ([]TrackWithScore, error) {
	out := []TrackWithScore{}
	for rows.Next() {
		var tw TrackWithScore
		var albumID, artistID, fp, artistName sql.NullString
		var trackNumber, trackIndex, bitDepth, bitrateKbps sql.NullInt64
		var discNumber, durationMs, sampleRate, channels sql.NullInt64
		var fileKey, format sql.NullString
		var fileSize int64
		var seekTable []byte
		var createdAt time.Time
		var replayGain sql.NullFloat64
		var albumName, coverArtKey sql.NullString

		if err := rows.Scan(
			&tw.ID, &albumID, &artistID, &tw.Title,
			&trackNumber, &trackIndex, &discNumber, &durationMs,
			&fileKey, &fileSize, &format,
			&bitDepth, &sampleRate, &channels, &bitrateKbps,
			&seekTable, &fp, &createdAt,
			&replayGain, &tw.Score, &artistName,
			&albumName, &coverArtKey,
		); err != nil {
			return nil, err
		}
		if albumID.Valid {
			tw.AlbumID = &albumID.String
		}
		if artistID.Valid {
			tw.ArtistID = &artistID.String
		}
		if trackNumber.Valid {
			n := int(trackNumber.Int64)
			tw.TrackNumber = &n
		}
		if trackIndex.Valid {
			n := int(trackIndex.Int64)
			tw.TrackIndex = &n
		}
		tw.DiscNumber = int(discNumber.Int64)
		tw.DurationMs = int(durationMs.Int64)
		if fileKey.Valid {
			tw.FileKey = fileKey.String
		}
		tw.FileSize = fileSize
		if format.Valid {
			tw.Format = format.String
		}
		if bitDepth.Valid {
			n := int(bitDepth.Int64)
			tw.BitDepth = &n
		}
		tw.SampleRate = int(sampleRate.Int64)
		tw.Channels = int(channels.Int64)
		if bitrateKbps.Valid {
			n := int(bitrateKbps.Int64)
			tw.BitrateKbps = &n
		}
		tw.SeekTable = seekTable
		if fp.Valid {
			tw.Fingerprint = fp.String
		}
		tw.CreatedAt = createdAt
		if replayGain.Valid && replayGain.Float64 != 0 {
			rg := replayGain.Float64
			tw.ReplayGainTrack = &rg
		}
		if artistName.Valid {
			tw.ArtistName = &artistName.String
		}
		if albumName.Valid {
			tw.AlbumName = &albumName.String
		}
		if coverArtKey.Valid {
			tw.CoverArtKey = coverArtKey.String
		}
		out = append(out, tw)
	}
	return out, rows.Err()
}

// ListTrackIDsByAlbum returns all track IDs for an album (used by waveform/metadata jobs).
func (s *Store) ListTrackIDsByAlbum(ctx context.Context, albumID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM tracks WHERE album_id = $1 ORDER BY disc_number, track_number`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetTrackFileKey returns just the file_key for a track (used by waveform regeneration jobs).
func (s *Store) GetTrackFileKey(ctx context.Context, trackID string) (string, error) {
	var key string
	err := s.pool.QueryRow(ctx, `SELECT file_key FROM tracks WHERE id = $1`, trackID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return key, err
}
