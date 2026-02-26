package store

import (
	"context"
	"errors"
	"fmt"

	"database/sql"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store holds the connection pool.
// Services receive a Store; tests can substitute a mock.
type Store struct {
	pool *pgxpool.Pool
}

// Connect connects to Postgres using the given DSN and returns a Store.
func Connect(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{
		pool: pool,
	}, nil
}

// Close shuts down the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Ping checks that Postgres is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// GetUserByID returns a user by ID.
func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	var u User
	row := s.pool.QueryRow(ctx, `SELECT id, username, email, password_hash, created_at, last_login_at FROM users WHERE id = $1`, id)
	var lastLoginAt sql.NullTime
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &lastLoginAt)
	if lastLoginAt.Valid {
		u.LastLoginAt = &lastLoginAt.Time
	}
	return u, err
}

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

// UpsertAlbum inserts or updates an album.
func (s *Store) UpsertAlbum(ctx context.Context, p UpsertAlbumParams) (Album, error) {
	var alb Album
	row := s.pool.QueryRow(ctx, `INSERT INTO albums (id, artist_id, title, release_year, label, cover_art_key, mbid)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET artist_id = EXCLUDED.artist_id, title = EXCLUDED.title, release_year = EXCLUDED.release_year, label = EXCLUDED.label, cover_art_key = COALESCE(EXCLUDED.cover_art_key, albums.cover_art_key), mbid = EXCLUDED.mbid RETURNING id, artist_id, title, release_year, label, cover_art_key, mbid, created_at`,
		p.ID, p.ArtistID, p.Title, p.ReleaseYear, p.Label, p.CoverArtKey, p.Mbid)
	var artistID, label, coverArtKey, mbid sql.NullString
	var releaseYear sql.NullInt64
	err := row.Scan(&alb.ID, &artistID, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &alb.CreatedAt)
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
	return alb, err
}

func (s *Store) ListPlaylistsByUser(ctx context.Context, userID string) ([]Playlist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, description, cover_art_key, created_at FROM playlists WHERE user_id = $1 ORDER BY updated_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlaylists(rows)
}

func (s *Store) CreatePlaylist(ctx context.Context, p CreatePlaylistParams) (Playlist, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO playlists (id, user_id, name, description, cover_art_key) VALUES ($1, $2, $3, $4, $5) RETURNING id, user_id, name, description, cover_art_key, created_at`,
		p.ID, p.UserID, p.Name, p.Description, p.CoverArtKey)
	return scanPlaylist(row)
}

func (s *Store) GetPlaylistByID(ctx context.Context, id string) (Playlist, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, description, cover_art_key, created_at FROM playlists WHERE id = $1`,
		id)
	return scanPlaylist(row)
}

func (s *Store) ListPlaylistTracks(ctx context.Context, id string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
FROM tracks t
JOIN playlist_tracks pt ON pt.track_id = t.id
WHERE pt.playlist_id = $1
ORDER BY pt.position ASC`,
		id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) UpdatePlaylist(ctx context.Context, p UpdatePlaylistParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE playlists SET name = $2, description = $3, cover_art_key = $4, updated_at = now() WHERE id = $1`,
		p.ID, p.Name, p.Description, p.CoverArtKey)
	return err
}

func (s *Store) DeletePlaylist(ctx context.Context, p DeletePlaylistParams) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM playlists WHERE id = $1`, p.ID)
	return err
}

func (s *Store) ListTracksByUser(ctx context.Context, p ListTracksByUserParams) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
FROM tracks t
JOIN user_library ul ON ul.track_id = t.id
WHERE ul.user_id = $1
ORDER BY t.title ASC
LIMIT $2 OFFSET $3`,
		p.UserID, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) ListAlbums(ctx context.Context, p ListAlbumsParams) ([]Album, error) {
	// Build ORDER BY from a whitelist — no user input reaches the query string.
	var orderBy string
	switch p.SortBy {
	case "artist":
		orderBy = `regexp_replace(lower(coalesce(ar.sort_name, ar.name, '')), '^(the |a |an )\s*', '') ASC,` +
			` regexp_replace(lower(al.title), '^(the |a |an )\s*', '') ASC`
	case "year":
		orderBy = `al.release_year DESC NULLS LAST,` +
			` regexp_replace(lower(al.title), '^(the |a |an )\s*', '') ASC`
	default: // "title"
		orderBy = `regexp_replace(lower(al.title), '^(the |a |an )\s*', '') ASC`
	}
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count
FROM albums al
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t ON t.album_id = al.id
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY `+orderBy+` LIMIT $1 OFFSET $2`,
		p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

func (s *Store) ListArtists(ctx context.Context, p ListArtistsParams) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, sort_name, mbid, created_at FROM artists ORDER BY sort_name ASC LIMIT $1 OFFSET $2`,
		p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtists(rows)
}

func (s *Store) ListTracksByAlbum(ctx context.Context, albumID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, album_id, artist_id, title, track_number, disc_number, duration_ms, file_key, file_size, format, bit_depth, sample_rate, channels, bitrate_kbps, seek_table, fingerprint, created_at
FROM tracks
WHERE album_id = $1
ORDER BY disc_number ASC, track_number ASC`,
		albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) GetArtistByID(ctx context.Context, artistID string) (Artist, error) {
	var a Artist
	var mbid sql.NullString
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, sort_name, mbid, created_at FROM artists WHERE id = $1`,
		artistID)
	if err := row.Scan(&a.ID, &a.Name, &a.SortName, &mbid, &a.CreatedAt); err != nil {
		return Artist{}, err
	}
	if mbid.Valid {
		a.Mbid = &mbid.String
	}
	return a, nil
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

func (s *Store) RemoveTrackFromLibrary(ctx context.Context, userID, trackID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_library WHERE user_id = $1 AND track_id = $2`, userID, trackID)
	return err
}

func (s *Store) GetQueue(ctx context.Context, userID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
FROM tracks t
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
	row := s.pool.QueryRow(ctx, `SELECT id, album_id, artist_id, title, track_number, disc_number, duration_ms, file_key, file_size, format, bit_depth, sample_rate, channels, bitrate_kbps, seek_table, fingerprint, created_at FROM tracks WHERE id = $1`, id)
	var albumID, artistID, format sql.NullString
	var trackNumber, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprintVal sql.NullString
	var createdAt time.Time
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprintVal, &createdAt)
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

func (s *Store) GetMaxPlaylistPosition(ctx context.Context, playlistID string) (int, error) {
	var pos int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0)::int FROM playlist_tracks WHERE playlist_id = $1`, playlistID).Scan(&pos)
	return pos, err
}

func (s *Store) SearchTracks(ctx context.Context, p SearchTracksParams) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
FROM tracks t
LEFT JOIN artists ar ON ar.id = t.artist_id
WHERE t.search_vector @@ websearch_to_tsquery('english', $1)
   OR ar.search_vector @@ websearch_to_tsquery('english', $1)
ORDER BY ts_rank(t.search_vector, websearch_to_tsquery('english', $1)) DESC
LIMIT $2`,
		p.ToTsquery, p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (s *Store) SearchAlbums(ctx context.Context, p SearchAlbumsParams) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count
FROM albums al
LEFT JOIN artists ar ON ar.id = al.artist_id
LEFT JOIN tracks t ON t.album_id = al.id
WHERE al.search_vector @@ websearch_to_tsquery('english', $1)
GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
ORDER BY ts_rank(al.search_vector, websearch_to_tsquery('english', $1)) DESC LIMIT $2`,
		p.ToTsquery, p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlbums(rows)
}

func (s *Store) SearchArtists(ctx context.Context, p SearchArtistsParams) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, sort_name, mbid, created_at FROM artists
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

func (s *Store) AddTrackToPlaylist(ctx context.Context, p AddTrackToPlaylistParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO playlist_tracks (playlist_id, track_id, position) VALUES ($1, $2, $3)`,
		p.PlaylistID, p.TrackID, p.Position)
	return err
}

func (s *Store) RemoveTrackFromPlaylist(ctx context.Context, p RemoveTrackFromPlaylistParams) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2`,
		p.PlaylistID, p.TrackID)
	return err
}

func (s *Store) UpdatePlaylistTrackOrder(ctx context.Context, p UpdatePlaylistTrackOrderParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE playlist_tracks SET position = $3 WHERE playlist_id = $1 AND track_id = $2`,
		p.PlaylistID, p.TrackID, p.Position)
	return err
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
		`SELECT sub.id, sub.album_id, sub.artist_id, sub.title, sub.track_number, sub.disc_number,
		        sub.duration_ms, sub.file_key, sub.file_size, sub.format, sub.bit_depth,
		        sub.sample_rate, sub.channels, sub.bitrate_kbps, sub.seek_table, sub.fingerprint, sub.created_at
		FROM (
		  SELECT DISTINCT ON (ph.track_id)
		    t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		    t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		    t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		    ph.played_at
		  FROM play_history ph
		  JOIN tracks t ON t.id = ph.track_id
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		        t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
		FROM tracks t
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

// ListRecentlyPlayedAlbums returns distinct albums played by the user, ordered by most recent play.
func (s *Store) ListRecentlyPlayedAlbums(ctx context.Context, p ListRecentlyPlayedParams) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name AS artist_name, al.title, al.release_year,
		        al.label, al.cover_art_key, al.mbid, al.created_at,
		        COUNT(DISTINCT tr.id) AS track_count
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
		        COUNT(t.id) AS track_count
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
	row := s.pool.QueryRow(ctx, `SELECT id, username, email, password_hash, created_at, last_login_at FROM users WHERE email = $1`, email)
	var lastLoginAt sql.NullTime
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &lastLoginAt)
	if lastLoginAt.Valid {
		u.LastLoginAt = &lastLoginAt.Time
	}
	return u, err
}

// UpdateLastLogin updates the last login time for a user.
func (s *Store) UpdateLastLogin(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
	return err
}

// UpsertTrack inserts or updates a track.
func (s *Store) UpsertTrack(ctx context.Context, p UpsertTrackParams) (Track, error) {
	var t Track
	row := s.pool.QueryRow(ctx, `INSERT INTO tracks (
 id, album_id, artist_id, title, track_number, disc_number,
 duration_ms, file_key, file_size, format, bit_depth,
 sample_rate, channels, bitrate_kbps, seek_table, fingerprint)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (id) DO UPDATE SET album_id = EXCLUDED.album_id, artist_id = EXCLUDED.artist_id, title = EXCLUDED.title, track_number = EXCLUDED.track_number, disc_number = EXCLUDED.disc_number, duration_ms = EXCLUDED.duration_ms, file_key = EXCLUDED.file_key, file_size = EXCLUDED.file_size, format = EXCLUDED.format, bit_depth = EXCLUDED.bit_depth, sample_rate = EXCLUDED.sample_rate, channels = EXCLUDED.channels, bitrate_kbps = EXCLUDED.bitrate_kbps, seek_table = EXCLUDED.seek_table, fingerprint = EXCLUDED.fingerprint RETURNING id, album_id, artist_id, title, track_number, disc_number, duration_ms, file_key, file_size, format, bit_depth, sample_rate, channels, bitrate_kbps, seek_table, fingerprint, created_at`,
		p.ID, p.AlbumID, p.ArtistID, p.Title, p.TrackNumber, p.DiscNumber, p.DurationMs, p.FileKey, p.FileSize, p.Format, p.BitDepth, p.SampleRate, p.Channels, p.BitrateKbps, p.SeekTable, p.Fingerprint)
	var albumID, artistID, format sql.NullString
	var trackNumber, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprint sql.NullString
	var createdAt time.Time
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprint, &createdAt)
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

// GetTrackByFingerprint returns a track by fingerprint.
func (s *Store) GetTrackByFingerprint(ctx context.Context, fingerprint string) (Track, error) {
	var t Track
	row := s.pool.QueryRow(ctx, `SELECT id, album_id, artist_id, title, track_number, disc_number, duration_ms, file_key, file_size, format, bit_depth, sample_rate, channels, bitrate_kbps, seek_table, fingerprint, created_at FROM tracks WHERE fingerprint = $1`, fingerprint)
	var albumID, artistID, format sql.NullString
	var trackNumber, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprintVal sql.NullString
	var createdAt time.Time
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprintVal, &createdAt)
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

// AddFavorite marks a track as a favorite for a user.
func (s *Store) AddFavorite(ctx context.Context, p FavoriteParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_favorites (user_id, track_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		p.UserID, p.TrackID)
	return err
}

// RemoveFavorite removes a track from a user's favorites.
func (s *Store) RemoveFavorite(ctx context.Context, p FavoriteParams) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_favorites WHERE user_id = $1 AND track_id = $2`,
		p.UserID, p.TrackID)
	return err
}

// ListFavorites returns all favorited tracks for a user, newest first.
func (s *Store) ListFavorites(ctx context.Context, userID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
FROM tracks t
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

// CountAlbums returns the total number of albums.
func (s *Store) CountAlbums(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM albums`).Scan(&count)
	return count, err
}

// GetAlbumByID returns an album by ID.
func (s *Store) GetAlbumByID(ctx context.Context, id string) (Album, error) {
	var alb Album
	row := s.pool.QueryRow(ctx, `SELECT id, artist_id, title, release_year, label, cover_art_key, mbid, created_at, (SELECT COUNT(*) FROM tracks WHERE album_id = $1) as track_count FROM albums WHERE id = $1`, id)
	var artistID, label, coverArtKey, mbid sql.NullString
	var releaseYear sql.NullInt64
	err := row.Scan(&alb.ID, &artistID, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &alb.CreatedAt, &alb.TrackCount)
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
	return alb, err
}

// --- scan helpers ---

func scanTracks(rows pgx.Rows) ([]Track, error) {
	out := make([]Track, 0)
	for rows.Next() {
		var t Track
		var albumID, artistID, fileKey, format, fp sql.NullString
		var trackNumber, discNumber, durationMs, fileSize, sampleRate, channels, bitDepth, bitrateKbps sql.NullInt64
		var seekTable []byte
		var createdAt time.Time
		if err := rows.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fp, &createdAt); err != nil {
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
		t.CreatedAt = createdAt
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanAlbums(rows pgx.Rows) ([]Album, error) {
	out := make([]Album, 0)
	for rows.Next() {
		var alb Album
		var artistID, artistName, label, coverArtKey, mbid sql.NullString
		var releaseYear sql.NullInt64
		if err := rows.Scan(&alb.ID, &artistID, &artistName, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &alb.CreatedAt, &alb.TrackCount); err != nil {
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
		out = append(out, alb)
	}
	return out, rows.Err()
}

func scanArtists(rows pgx.Rows) ([]Artist, error) {
	out := make([]Artist, 0)
	for rows.Next() {
		var a Artist
		var mbid sql.NullString
		if err := rows.Scan(&a.ID, &a.Name, &a.SortName, &mbid, &a.CreatedAt); err != nil {
			return nil, err
		}
		if mbid.Valid {
			a.Mbid = &mbid.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanPlaylist(row pgx.Row) (Playlist, error) {
	var pl Playlist
	var desc, coverArtKey sql.NullString
	var createdAt time.Time
	if err := row.Scan(&pl.ID, &pl.UserID, &pl.Name, &desc, &coverArtKey, &createdAt); err != nil {
		return Playlist{}, err
	}
	if desc.Valid {
		pl.Description = desc.String
	}
	if coverArtKey.Valid {
		pl.CoverArtKey = &coverArtKey.String
	}
	pl.CreatedAt = createdAt.Format(time.RFC3339)
	return pl, nil
}

func scanPlaylists(rows pgx.Rows) ([]Playlist, error) {
	out := make([]Playlist, 0)
	for rows.Next() {
		var pl Playlist
		var desc, coverArtKey sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&pl.ID, &pl.UserID, &pl.Name, &desc, &coverArtKey, &createdAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			pl.Description = desc.String
		}
		if coverArtKey.Valid {
			pl.CoverArtKey = &coverArtKey.String
		}
		pl.CreatedAt = createdAt.Format(time.RFC3339)
		out = append(out, pl)
	}
	return out, rows.Err()
}

// ListPlaylistTopPlayedTracks returns the top 4 most played tracks in a playlist.
func (s *Store) ListPlaylistTopPlayedTracks(ctx context.Context, playlistID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at
FROM tracks t
JOIN playlist_tracks pt ON pt.track_id = t.id
LEFT JOIN (
  SELECT track_id, COUNT(*) AS play_count
  FROM play_history
  GROUP BY track_id
) ph ON ph.track_id = t.id
WHERE pt.playlist_id = $1
ORDER BY ph.play_count DESC NULLS LAST, pt.position ASC
LIMIT 4`,
		playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}
