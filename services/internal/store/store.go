package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"database/sql"
	"time"

	"github.com/google/uuid"
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

// ── Smart playlist CRUD ────────────────────────────────────────────────────

func (s *Store) ListSmartPlaylistsByUser(ctx context.Context, userID string) ([]SmartPlaylist, error) {
	if err := s.EnsureSystemPlaylists(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, COALESCE(description,''), rules, rule_match, sort_by, sort_dir, limit_count, system, last_built_at, created_at, updated_at
		 FROM smart_playlists WHERE user_id = $1 ORDER BY system DESC, updated_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSmartPlaylists(rows)
}

func (s *Store) GetSmartPlaylistByID(ctx context.Context, id string) (SmartPlaylist, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, COALESCE(description,''), rules, rule_match, sort_by, sort_dir, limit_count, system, last_built_at, created_at, updated_at
		 FROM smart_playlists WHERE id = $1`,
		id)
	return scanSmartPlaylist(row)
}

func (s *Store) CreateSmartPlaylist(ctx context.Context, p CreateSmartPlaylistParams) (SmartPlaylist, error) {
	rulesJSON, err := json.Marshal(p.Rules)
	if err != nil {
		return SmartPlaylist{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO smart_playlists (id, user_id, name, description, rules, rule_match, sort_by, sort_dir, limit_count, system)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, user_id, name, COALESCE(description,''), rules, rule_match, sort_by, sort_dir, limit_count, system, last_built_at, created_at, updated_at`,
		p.ID, p.UserID, p.Name, p.Description, rulesJSON, p.RuleMatch, p.SortBy, p.SortDir, p.LimitCount, p.System)
	return scanSmartPlaylist(row)
}

func (s *Store) UpdateSmartPlaylist(ctx context.Context, p UpdateSmartPlaylistParams) (SmartPlaylist, error) {
	rulesJSON, err := json.Marshal(p.Rules)
	if err != nil {
		return SmartPlaylist{}, err
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE smart_playlists SET name=$2, description=$3, rules=$4, rule_match=$5, sort_by=$6, sort_dir=$7, limit_count=$8, updated_at=now()
		 WHERE id=$1
		 RETURNING id, user_id, name, COALESCE(description,''), rules, rule_match, sort_by, sort_dir, limit_count, system, last_built_at, created_at, updated_at`,
		p.ID, p.Name, p.Description, rulesJSON, p.RuleMatch, p.SortBy, p.SortDir, p.LimitCount)
	return scanSmartPlaylist(row)
}

func (s *Store) DeleteSmartPlaylist(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM smart_playlists WHERE id=$1 AND system=false`, id)
	return err
}

// systemPlaylistDef defines an auto-generated habit-based smart playlist.
type systemPlaylistDef struct {
	name        string
	description string
	rules       []SmartPlaylistRule
	ruleMatch   string
	sortBy      string
	sortDir     string
	limitCount  *int
}

func intPtr(n int) *int { return &n }

// systemPlaylistDefs are the habit-based playlists created automatically per user.
var systemPlaylistDefs = []systemPlaylistDef{
	{
		name:        "Most Played",
		description: "Your most listened-to tracks of all time",
		rules:       []SmartPlaylistRule{{Field: "play_count", Op: "gte", Value: "1"}},
		ruleMatch:   "all",
		sortBy:      "play_count",
		sortDir:     "desc",
		limitCount:  intPtr(50),
	},
	{
		name:        "Top Rated",
		description: "Tracks you've rated 4 stars or higher",
		rules:       []SmartPlaylistRule{{Field: "rating", Op: "gte", Value: "4"}},
		ruleMatch:   "all",
		sortBy:      "rating",
		sortDir:     "desc",
		limitCount:  intPtr(50),
	},
	{
		name:        "Recently Added",
		description: "Tracks added to your library in the last 30 days",
		rules:       []SmartPlaylistRule{{Field: "days_since_added", Op: "lte", Value: "30"}},
		ruleMatch:   "all",
		sortBy:      "added_at",
		sortDir:     "desc",
		limitCount:  intPtr(50),
	},
	{
		name:        "Never Played",
		description: "Tracks in your library you haven't listened to yet",
		rules:       []SmartPlaylistRule{{Field: "play_count", Op: "lte", Value: "0"}},
		ruleMatch:   "all",
		sortBy:      "added_at",
		sortDir:     "desc",
		limitCount:  intPtr(100),
	},
	{
		name:        "Forgotten Favorites",
		description: "Tracks you used to play a lot but haven't touched in over 3 months",
		rules: []SmartPlaylistRule{
			{Field: "play_count", Op: "gte", Value: "5"},
			{Field: "days_since_played", Op: "gte", Value: "90"},
		},
		ruleMatch:  "all",
		sortBy:     "play_count",
		sortDir:    "desc",
		limitCount: intPtr(50),
	},
}

// EnsureSystemPlaylists creates the habit-based system playlists for a user if they don't exist yet.
func (s *Store) EnsureSystemPlaylists(ctx context.Context, userID string) error {
	for _, def := range systemPlaylistDefs {
		var count int
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM smart_playlists WHERE user_id=$1 AND system=true AND name=$2`,
			userID, def.name).Scan(&count)
		if err != nil || count > 0 {
			continue
		}
		rulesJSON, err := json.Marshal(def.rules)
		if err != nil {
			continue
		}
		_, _ = s.pool.Exec(ctx,
			`INSERT INTO smart_playlists (id, user_id, name, description, rules, rule_match, sort_by, sort_dir, limit_count, system)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true)
			 ON CONFLICT DO NOTHING`,
			uuid.New().String(), userID, def.name, def.description,
			rulesJSON, def.ruleMatch, def.sortBy, def.sortDir, def.limitCount)
	}
	return nil
}

// EvaluateSmartPlaylist executes the filter rules and returns matching tracks.
// Only tracks in the user's library are eligible.
func (s *Store) EvaluateSmartPlaylist(ctx context.Context, sp SmartPlaylist) ([]Track, error) {
	args := []any{sp.UserID}
	argN := 2

	conds := make([]string, 0, len(sp.Rules))
	for _, rule := range sp.Rules {
		cond, newArgs, newN := smartRuleToCond(rule, args, argN)
		if cond == "" {
			continue
		}
		conds = append(conds, cond)
		args = newArgs
		argN = newN
	}

	join := " AND "
	if sp.RuleMatch == "any" {
		join = " OR "
	}

	where := "TRUE"
	if len(conds) > 0 {
		where = "(" + strings.Join(conds, join) + ")"
	}

	orderField := "t.title"
	switch sp.SortBy {
	case "year":
		orderField = "a.release_year"
	case "artist":
		orderField = "ar.name"
	case "duration_ms":
		orderField = "t.duration_ms"
	case "play_count":
		orderField = "play_count"
	case "rating":
		orderField = "user_rating"
	case "added_at":
		orderField = "t.created_at"
	}
	dir := "ASC"
	if sp.SortDir == "desc" {
		dir = "DESC"
	}

	limitClause := ""
	if sp.LimitCount != nil && *sp.LimitCount > 0 {
		limitClause = fmt.Sprintf("LIMIT %d", *sp.LimitCount)
	}

	q := fmt.Sprintf(`
		SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		       t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		       t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		       COALESCE(tf.replay_gain, 0) AS replay_gain_track,
		       ar.name AS artist_name, a.title AS album_name,
		       COALESCE((SELECT COUNT(*) FROM play_history ph WHERE ph.track_id = t.id AND ph.user_id = $1), 0) AS play_count,
		       (SELECT rating FROM track_ratings tr WHERE tr.track_id = t.id AND tr.user_id = $1) AS user_rating
		FROM tracks t
		LEFT JOIN albums a ON a.id = t.album_id
		LEFT JOIN artists ar ON ar.id = t.artist_id
		LEFT JOIN track_features tf ON tf.track_id = t.id
		WHERE %s
		ORDER BY %s %s NULLS LAST
		%s`,
		where, orderField, dir, limitClause)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Track{}
	for rows.Next() {
		var t Track
		var albumID, artistID, fp, artistName, albumName sql.NullString
		var trackNumber, bitDepth, bitrateKbps sql.NullInt64
		var replayGain sql.NullFloat64
		var userRating sql.NullInt64
		var playCount int64
		var seekTable []byte

		if err := rows.Scan(
			&t.ID, &albumID, &artistID, &t.Title,
			&trackNumber, &t.DiscNumber, &t.DurationMs,
			&t.FileKey, &t.FileSize, &t.Format,
			&bitDepth, &t.SampleRate, &t.Channels, &bitrateKbps,
			&seekTable, &fp, &t.CreatedAt,
			&replayGain, &artistName, &albumName, &playCount, &userRating,
		); err != nil {
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
		if bitDepth.Valid {
			n := int(bitDepth.Int64)
			t.BitDepth = &n
		}
		if bitrateKbps.Valid {
			n := int(bitrateKbps.Int64)
			t.BitrateKbps = &n
		}
		if artistName.Valid {
			t.ArtistName = &artistName.String
		}
		if albumName.Valid {
			t.AlbumName = &albumName.String
		}
		if replayGain.Valid {
			t.ReplayGainTrack = &replayGain.Float64
		}
		out = append(out, t)
	}
	// Mark the playlist as built.
	_, _ = s.pool.Exec(ctx, `UPDATE smart_playlists SET last_built_at=now() WHERE id=$1`, sp.ID)
	return out, rows.Err()
}

// smartRuleToCond converts one SmartPlaylistRule to a SQL condition fragment.
func smartRuleToCond(r SmartPlaylistRule, args []any, n int) (string, []any, int) {
	placeholder := func(v any) (string, int) {
		args = append(args, v)
		ph := fmt.Sprintf("$%d", n)
		n++
		return ph, n
	}

	switch r.Field {
	case "genre":
		// Match track_genres first; fall back to album_genres so that tracks
		// whose genre came from MusicBrainz album enrichment are also found.
		var cmp string
		switch r.Op {
		case "contains", "not_contains":
			ph, _ := placeholder("%" + r.Value + "%")
			cmp = fmt.Sprintf("lower(g.name) LIKE lower(%s)", ph)
		default: // "is", "is_not"
			ph, _ := placeholder(r.Value)
			cmp = fmt.Sprintf("lower(g.name) = lower(%s)", ph)
		}
		subq := fmt.Sprintf(
			`EXISTS (SELECT 1 FROM track_genres tg JOIN genres g ON g.id = tg.genre_id WHERE tg.track_id = t.id AND %s`+
				` UNION SELECT 1 FROM album_genres ag JOIN genres g ON g.id = ag.genre_id WHERE ag.album_id = t.album_id AND %s)`,
			cmp, cmp)
		if r.Op == "is_not" || r.Op == "not_contains" {
			return "NOT " + subq, args, n
		}
		return subq, args, n
	case "artist":
		ph, _ := placeholder("%" + r.Value + "%")
		switch r.Op {
		case "contains":
			return fmt.Sprintf("lower(ar.name) LIKE lower(%s)", ph), args, n
		case "not_contains":
			return fmt.Sprintf("lower(ar.name) NOT LIKE lower(%s)", ph), args, n
		case "is":
			args[len(args)-1] = r.Value
			return fmt.Sprintf("lower(ar.name) = lower(%s)", ph), args, n
		case "is_not":
			args[len(args)-1] = r.Value
			return fmt.Sprintf("lower(ar.name) != lower(%s)", ph), args, n
		}
	case "album":
		ph, _ := placeholder("%" + r.Value + "%")
		switch r.Op {
		case "contains":
			return fmt.Sprintf("lower(a.title) LIKE lower(%s)", ph), args, n
		case "not_contains":
			return fmt.Sprintf("lower(a.title) NOT LIKE lower(%s)", ph), args, n
		case "is":
			args[len(args)-1] = r.Value
			return fmt.Sprintf("lower(a.title) = lower(%s)", ph), args, n
		case "is_not":
			args[len(args)-1] = r.Value
			return fmt.Sprintf("lower(a.title) != lower(%s)", ph), args, n
		}
	case "format":
		ph, _ := placeholder(r.Value)
		if r.Op == "is_not" {
			return fmt.Sprintf("t.format != %s", ph), args, n
		}
		return fmt.Sprintf("t.format = %s", ph), args, n
	case "year":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		return fmt.Sprintf("a.release_year %s %s", op, ph), args, n
	case "bit_depth":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		return fmt.Sprintf("t.bit_depth %s %s", op, ph), args, n
	case "duration_ms":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		return fmt.Sprintf("t.duration_ms %s %s", op, ph), args, n
	case "play_count":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		sub := fmt.Sprintf("(SELECT COUNT(*) FROM play_history ph WHERE ph.track_id = t.id AND ph.user_id = $1)")
		return fmt.Sprintf("%s %s %s", sub, op, ph), args, n
	case "rating":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		sub := "(SELECT rating FROM track_ratings tr WHERE tr.track_id = t.id AND tr.user_id = $1)"
		return fmt.Sprintf("%s %s %s", sub, op, ph), args, n
	case "days_since_played":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		sub := "(SELECT EXTRACT(EPOCH FROM (now() - MAX(ph.played_at)))/86400 FROM play_history ph WHERE ph.track_id = t.id AND ph.user_id = $1)"
		return fmt.Sprintf("%s %s %s", sub, op, ph), args, n
	case "days_since_added":
		ph, _ := placeholder(r.Value)
		op := sqlOp(r.Op)
		if op == "" {
			return "", args, n
		}
		return fmt.Sprintf("EXTRACT(EPOCH FROM (now() - t.created_at))/86400 %s %s", op, ph), args, n
	}
	return "", args, n
}

func sqlOp(op string) string {
	switch op {
	case "is", "eq":
		return "="
	case "is_not", "neq":
		return "!="
	case "gt":
		return ">"
	case "lt":
		return "<"
	case "gte":
		return ">="
	case "lte":
		return "<="
	}
	return ""
}

func scanSmartPlaylists(rows pgx.Rows) ([]SmartPlaylist, error) {
	out := []SmartPlaylist{}
	for rows.Next() {
		sp, err := scanSmartPlaylistRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSmartPlaylist(row scanner) (SmartPlaylist, error) {
	return scanSmartPlaylistRow(row.Scan)
}

func scanSmartPlaylistRow(scan func(...any) error) (SmartPlaylist, error) {
	var sp SmartPlaylist
	var rulesJSON []byte
	var desc sql.NullString
	var limitCount sql.NullInt64
	var lastBuiltAt sql.NullTime

	if err := scan(
		&sp.ID, &sp.UserID, &sp.Name, &desc,
		&rulesJSON, &sp.RuleMatch, &sp.SortBy, &sp.SortDir,
		&limitCount, &sp.System, &lastBuiltAt, &sp.CreatedAt, &sp.UpdatedAt,
	); err != nil {
		return sp, err
	}
	sp.Description = desc.String
	if limitCount.Valid {
		n := int(limitCount.Int64)
		sp.LimitCount = &n
	}
	if lastBuiltAt.Valid {
		sp.LastBuiltAt = &lastBuiltAt.Time
	}
	if err := json.Unmarshal(rulesJSON, &sp.Rules); err != nil {
		sp.Rules = []SmartPlaylistRule{}
	}
	return sp, nil
}

// ── Regular playlist CRUD ──────────────────────────────────────────────────

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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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
			SELECT al.id, al.artist_id, ar.name AS ar_name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) AS track_count,
			       ROW_NUMBER() OVER (
			           PARTITION BY COALESCE(al.album_group_id, al.id)
			           ORDER BY (al.cover_art_key IS NULL) ASC, al.created_at ASC
			       ) AS rn
			FROM albums al
			LEFT JOIN artists ar ON ar.id = al.artist_id
			LEFT JOIN tracks t ON t.album_id = al.id
			GROUP BY al.id, al.artist_id, ar.id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at
		)
		SELECT id, artist_id, ar_name, title, release_year, label, cover_art_key, mbid, created_at, track_count
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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

func (s *Store) RemoveTrackFromLibrary(ctx context.Context, userID, trackID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_library WHERE user_id = $1 AND track_id = $2`, userID, trackID)
	return err
}

func (s *Store) GetQueue(ctx context.Context, userID string) ([]Track, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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
	row := s.pool.QueryRow(ctx, `SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.isrc, t.mbid, t.enriched_at, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm FROM tracks t LEFT JOIN track_features tf ON tf.track_id = t.id WHERE t.id = $1`, id)
	var albumID, artistID, format sql.NullString
	var trackNumber, discNumber, durationMs, sampleRate, channels sql.NullInt64
	var fileKey sql.NullString
	var fileSize sql.NullInt64
	var bitDepth, bitrateKbps sql.NullInt64
	var seekTable []byte
	var fingerprintVal, isrc, mbid sql.NullString
	var enrichedAt sql.NullTime
	var createdAt time.Time
	var replayGain, bpm sql.NullFloat64
	err := row.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fingerprintVal, &isrc, &mbid, &enrichedAt, &createdAt, &replayGain, &bpm)
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
	return t, err
}

// GetTracksByIDs returns tracks for the given IDs in a single batch query.
// Tracks are returned in insertion order; missing IDs are silently omitted.
func (s *Store) GetTracksByIDs(ctx context.Context, ids []string) ([]Track, error) {
	if len(ids) == 0 {
		return []Track{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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

func (s *Store) GetMaxPlaylistPosition(ctx context.Context, playlistID string) (int, error) {
	var pos int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0)::int FROM playlist_tracks WHERE playlist_id = $1`, playlistID).Scan(&pos)
	return pos, err
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track
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
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count
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
		        sub.sample_rate, sub.channels, sub.bitrate_kbps, sub.seek_table, sub.fingerprint, sub.created_at, sub.replay_gain_track, sub.bpm
		FROM (
		  SELECT DISTINCT ON (ph.track_id)
		    t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		    t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		    t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		    COALESCE(tf.replay_gain, 0) AS replay_gain_track,
		    COALESCE(tf.bpm, 0) AS bpm,
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth,
		        t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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

// DeleteIngestStateForAlbum removes ingest_state rows for all tracks belonging
// to the given album and returns the filesystem paths that were deleted.
// Call this before a targeted rescan so the files are not skipped by upToDate.
func (s *Store) DeleteIngestStateForAlbum(ctx context.Context, albumID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM ingest_state
		 WHERE track_id IN (SELECT id FROM tracks WHERE album_id = $1)
		 RETURNING path`,
		albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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

// CountAlbums returns the number of distinct album groups (i.e. deduped by album_group_id).
func (s *Store) CountAlbums(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT DISTINCT COALESCE(album_group_id, id) FROM albums) s`).Scan(&count)
	return count, err
}

// GetAlbumByID returns an album by ID.
func (s *Store) GetAlbumByID(ctx context.Context, id string) (Album, error) {
	var alb Album
	row := s.pool.QueryRow(ctx, `SELECT id, artist_id, title, release_year, label, cover_art_key, mbid, album_type, release_date, release_group_mbid, enriched_at, album_group_id, edition, created_at, (SELECT COUNT(*) FROM tracks WHERE album_id = $1) as track_count FROM albums WHERE id = $1`, id)
	var artistID, label, coverArtKey, mbid, albumType, releaseDate, releaseGroupMbid, albumGroupID, edition sql.NullString
	var releaseYear sql.NullInt64
	var enrichedAt sql.NullTime
	err := row.Scan(&alb.ID, &artistID, &alb.Title, &releaseYear, &label, &coverArtKey, &mbid, &albumType, &releaseDate, &releaseGroupMbid, &enrichedAt, &albumGroupID, &edition, &alb.CreatedAt, &alb.TrackCount)
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
		        COUNT(t.id) AS track_count
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

// --- scan helpers ---

func scanTracks(rows pgx.Rows) ([]Track, error) {
	out := make([]Track, 0)
	for rows.Next() {
		var t Track
		var albumID, artistID, fileKey, format, fp sql.NullString
		var trackNumber, discNumber, durationMs, fileSize, sampleRate, channels, bitDepth, bitrateKbps sql.NullInt64
		var seekTable []byte
		var createdAt time.Time
		var replayGain, bpm sql.NullFloat64
		if err := rows.Scan(&t.ID, &albumID, &artistID, &t.Title, &trackNumber, &discNumber, &durationMs, &fileKey, &fileSize, &format, &bitDepth, &sampleRate, &channels, &bitrateKbps, &seekTable, &fp, &createdAt, &replayGain, &bpm); err != nil {
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
		if replayGain.Valid && replayGain.Float64 != 0 {
			rg := replayGain.Float64
			t.ReplayGainTrack = &rg
		}
		if bpm.Valid && bpm.Float64 != 0 {
			v := bpm.Float64
			t.BPM = &v
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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
FROM tracks t
LEFT JOIN track_features tf ON tf.track_id = t.id
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

// ---------------------------------------------------------------------------
// Enrichment methods
// ---------------------------------------------------------------------------

// UpdateArtistEnrichment updates an artist with MusicBrainz metadata.
func (s *Store) UpdateArtistEnrichment(ctx context.Context, p UpdateArtistEnrichmentParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE artists SET mbid = COALESCE($2, mbid), artist_type = $3, country = $4, begin_date = $5, end_date = $6, disambiguation = $7, image_key = COALESCE($8, image_key), enriched_at = now() WHERE id = $1`,
		p.ID, p.Mbid, p.ArtistType, p.Country, p.BeginDate, p.EndDate, p.Disambiguation, p.ImageKey)
	return err
}

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

// UpdateTrackEnrichment updates a track with MusicBrainz metadata.
func (s *Store) UpdateTrackEnrichment(ctx context.Context, p UpdateTrackEnrichmentParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tracks SET mbid = COALESCE($2, mbid), isrc = $3, enriched_at = now() WHERE id = $1`,
		p.ID, p.Mbid, p.Isrc)
	return err
}

// ---------------------------------------------------------------------------
// Genre methods
// ---------------------------------------------------------------------------

// UpsertGenre inserts a genre or does nothing if it already exists.
func (s *Store) UpsertGenre(ctx context.Context, id, name string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO genres (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		id, name)
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

// SetAlbumGenres replaces all genre associations for an album.
func (s *Store) SetAlbumGenres(ctx context.Context, albumID string, genreIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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

// SetTrackFeaturedArtists replaces all featured-artist associations for a track.
// artistIDs are stored in order (position = index).
func (s *Store) SetTrackFeaturedArtists(ctx context.Context, trackID string, artistIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
	defer tx.Rollback(ctx)
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

// ---------------------------------------------------------------------------
// Related artists
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Batch queries for enrichment
// ---------------------------------------------------------------------------

// ListUnenrichedArtists returns artists that haven't been enriched yet.
// If force is true, returns all artists regardless of enrichment status.
func (s *Store) ListUnenrichedArtists(ctx context.Context, limit int, force bool) ([]Artist, error) {
	q := `SELECT id, name, sort_name, mbid, created_at FROM artists`
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

// ListUnenrichedAlbums returns albums that haven't been enriched yet, with artist name.
// If force is true, returns all albums regardless of enrichment status.
func (s *Store) ListUnenrichedAlbums(ctx context.Context, limit int, force bool) ([]Album, error) {
	whereClause := "WHERE al.enriched_at IS NULL"
	if force {
		whereClause = ""
	}
	q := fmt.Sprintf(`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count
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

// ListUnenrichedTracks returns tracks that haven't been enriched yet.
// If force is true, returns all tracks regardless of enrichment status.
func (s *Store) ListUnenrichedTracks(ctx context.Context, limit int, force bool) ([]Track, error) {
	q := `SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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

// ListArtistsByGenre returns artists that have a given genre.
func (s *Store) ListArtistsByGenre(ctx context.Context, genreID string, limit, offset int) ([]Artist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.name, a.sort_name, a.mbid, a.created_at
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

// ListAlbumsByGenre returns albums that have a given genre.
func (s *Store) ListAlbumsByGenre(ctx context.Context, genreID string, limit, offset int) ([]Album, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT al.id, al.artist_id, ar.name, al.title, al.release_year, al.label, al.cover_art_key, al.mbid, al.created_at, COUNT(t.id) as track_count
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

// GetGenreByID returns a single genre by ID.
func (s *Store) GetGenreByID(ctx context.Context, id string) (Genre, error) {
	var g Genre
	err := s.pool.QueryRow(ctx, `SELECT id, name FROM genres WHERE id = $1`, id).Scan(&g.ID, &g.Name)
	return g, err
}

// ---------------------------------------------------------------------------
// Track similarity & recommendation methods
// ---------------------------------------------------------------------------

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
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number, t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate, t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at, COALESCE(tf.replay_gain, 0) AS replay_gain_track, COALESCE(tf.bpm, 0) AS bpm
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
	defer tx.Rollback(ctx)

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
	defer tx.Rollback(ctx)

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
			`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
			        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
			        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
			        COALESCE(tf.replay_gain, 0) AS replay_gain_track, s.score, ar.name AS artist_name
			 FROM (
			     SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = $1
			     UNION ALL
			     SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = $1
			 ) s
			 JOIN tracks t ON t.id = s.similar_id
			 LEFT JOIN track_features tf ON tf.track_id = t.id
			 LEFT JOIN artists ar ON ar.id = t.artist_id
			 WHERE t.album_id != $3
			 ORDER BY s.score DESC
			 LIMIT $2`,
			trackID, limit, excludeAlbumID)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
			        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
			        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
			        COALESCE(tf.replay_gain, 0) AS replay_gain_track, s.score, ar.name AS artist_name
			 FROM (
			     SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = $1
			     UNION ALL
			     SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = $1
			 ) s
			 JOIN tracks t ON t.id = s.similar_id
			 LEFT JOIN track_features tf ON tf.track_id = t.id
			 LEFT JOIN artists ar ON ar.id = t.artist_id
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
		 SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, c.score, ar.name AS artist_name
		 FROM candidates c
		 JOIN tracks t ON t.id = c.similar_id
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
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
// the provided track IDs (already in queue).
func (s *Store) AutoplayAfter(ctx context.Context, userID, trackID string, exclude []string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, s.score, ar.name AS artist_name
		 FROM (
		     SELECT track_b AS similar_id, score FROM track_similarity WHERE track_a = $1
		     UNION ALL
		     SELECT track_a AS similar_id, score FROM track_similarity WHERE track_b = $1
		 ) s
		 JOIN tracks t ON t.id = s.similar_id
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 WHERE t.id != ALL($2::text[])
		 ORDER BY s.score DESC
		 LIMIT $3`,
		trackID, exclude, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

// RecommendForArtist returns tracks from the given artist and related artists,
// ordered by similarity score, excluding tracks played in the last 24 hours.
func (s *Store) RecommendForArtist(ctx context.Context, artistID, userID string, limit int) ([]TrackWithScore, error) {
	rows, err := s.pool.Query(ctx,
		`WITH artist_tracks AS (
		     -- tracks by this artist
		     SELECT t.id AS track_id FROM tracks t WHERE t.artist_id = $1
		 ),
		 related AS (
		     -- related artists (one hop)
		     SELECT related_id FROM related_artists WHERE artist_id = $1
		 ),
		 candidate_ids AS (
		     SELECT track_id FROM artist_tracks
		     UNION
		     SELECT t.id FROM tracks t JOIN related r ON t.artist_id = r.related_id
		 ),
		 scored AS (
		     SELECT c.track_id,
		            COALESCE(MAX(s.score), 0.5) AS score
		     FROM candidate_ids c
		     LEFT JOIN LATERAL (
		         SELECT score FROM track_similarity
		         WHERE track_a = c.track_id OR track_b = c.track_id
		         ORDER BY score DESC LIMIT 1
		     ) s ON TRUE
		     GROUP BY c.track_id
		 )
		 SELECT t.id, t.album_id, t.artist_id, t.title, t.track_number, t.disc_number,
		        t.duration_ms, t.file_key, t.file_size, t.format, t.bit_depth, t.sample_rate,
		        t.channels, t.bitrate_kbps, t.seek_table, t.fingerprint, t.created_at,
		        COALESCE(tf.replay_gain, 0) AS replay_gain_track, sc.score, ar.name AS artist_name
		 FROM scored sc
		 JOIN tracks t ON t.id = sc.track_id
		 LEFT JOIN track_features tf ON tf.track_id = t.id
		 LEFT JOIN artists ar ON ar.id = t.artist_id
		 WHERE t.id NOT IN (
		     SELECT track_id FROM play_history
		     WHERE user_id = $2 AND played_at > now() - interval '24 hours'
		 )
		 ORDER BY sc.score DESC, random()
		 LIMIT $3`,
		artistID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracksWithScore(rows)
}

func scanTracksWithScore(rows pgx.Rows) ([]TrackWithScore, error) {
	out := []TrackWithScore{}
	for rows.Next() {
		var tw TrackWithScore
		var albumID, artistID, fp, artistName sql.NullString
		var trackNumber, bitDepth, bitrateKbps sql.NullInt64
		var discNumber, durationMs, sampleRate, channels sql.NullInt64
		var fileKey, format sql.NullString
		var fileSize int64
		var seekTable []byte
		var createdAt time.Time
		var replayGain sql.NullFloat64

		if err := rows.Scan(
			&tw.ID, &albumID, &artistID, &tw.Title,
			&trackNumber, &discNumber, &durationMs,
			&fileKey, &fileSize, &format,
			&bitDepth, &sampleRate, &channels, &bitrateKbps,
			&seekTable, &fp, &createdAt,
			&replayGain, &tw.Score, &artistName,
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
		out = append(out, tw)
	}
	return out, rows.Err()
}

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
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, nil // return empty prefs (no limits)
		}
		return p, err
	}
	nullIntToPtr := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	nullStrToPtr := func(n sql.NullString) *string {
		if !n.Valid {
			return nil
		}
		return &n.String
	}
	p.MaxBitrateKbps = nullIntToPtr(maxBitrate)
	p.MaxSampleRate = nullIntToPtr(maxSR)
	p.MaxBitDepth = nullIntToPtr(maxBD)
	p.WifiMaxBitrateKbps = nullIntToPtr(wifiMaxBitrate)
	p.WifiMaxSampleRate = nullIntToPtr(wifiMaxSR)
	p.WifiMaxBitDepth = nullIntToPtr(wifiMaxBD)
	p.MobileMaxBitrateKbps = nullIntToPtr(mobileMaxBitrate)
	p.MobileMaxSampleRate = nullIntToPtr(mobileMaxSR)
	p.MobileMaxBitDepth = nullIntToPtr(mobileMaxBD)
	p.TranscodeFormat = nullStrToPtr(transcodeFmt)
	p.WifiTranscodeFormat = nullStrToPtr(wifiTranscodeFmt)
	p.MobileTranscodeFormat = nullStrToPtr(mobileTranscodeFmt)
	p.UpdatedAt = updatedAt
	return p, nil
}

// UpsertUserStreamingPrefs inserts or updates streaming preferences for a user.
func (s *Store) UpsertUserStreamingPrefs(ctx context.Context, p UpsertUserStreamingPrefsParams) (UserStreamingPrefs, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO user_streaming_prefs
		     (user_id, max_bitrate_kbps, max_sample_rate, max_bit_depth,
		      wifi_max_bitrate_kbps, wifi_max_sample_rate, wifi_max_bit_depth,
		      mobile_max_bitrate_kbps, mobile_max_sample_rate, mobile_max_bit_depth,
		      transcode_format, wifi_transcode_format, mobile_transcode_format,
		      updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
		 ON CONFLICT (user_id) DO UPDATE SET
		     max_bitrate_kbps        = EXCLUDED.max_bitrate_kbps,
		     max_sample_rate         = EXCLUDED.max_sample_rate,
		     max_bit_depth           = EXCLUDED.max_bit_depth,
		     wifi_max_bitrate_kbps   = EXCLUDED.wifi_max_bitrate_kbps,
		     wifi_max_sample_rate    = EXCLUDED.wifi_max_sample_rate,
		     wifi_max_bit_depth      = EXCLUDED.wifi_max_bit_depth,
		     mobile_max_bitrate_kbps = EXCLUDED.mobile_max_bitrate_kbps,
		     mobile_max_sample_rate  = EXCLUDED.mobile_max_sample_rate,
		     mobile_max_bit_depth    = EXCLUDED.mobile_max_bit_depth,
		     transcode_format        = EXCLUDED.transcode_format,
		     wifi_transcode_format   = EXCLUDED.wifi_transcode_format,
		     mobile_transcode_format = EXCLUDED.mobile_transcode_format,
		     updated_at              = now()
		 RETURNING max_bitrate_kbps, max_sample_rate, max_bit_depth,
		           wifi_max_bitrate_kbps, wifi_max_sample_rate, wifi_max_bit_depth,
		           mobile_max_bitrate_kbps, mobile_max_sample_rate, mobile_max_bit_depth,
		           transcode_format, wifi_transcode_format, mobile_transcode_format,
		           updated_at`,
		p.UserID, p.MaxBitrateKbps, p.MaxSampleRate, p.MaxBitDepth,
		p.WifiMaxBitrateKbps, p.WifiMaxSampleRate, p.WifiMaxBitDepth,
		p.MobileMaxBitrateKbps, p.MobileMaxSampleRate, p.MobileMaxBitDepth,
		p.TranscodeFormat, p.WifiTranscodeFormat, p.MobileTranscodeFormat)
	var (
		maxBitrate, maxSR, maxBD                   sql.NullInt64
		wifiMaxBitrate, wifiMaxSR, wifiMaxBD       sql.NullInt64
		mobileMaxBitrate, mobileMaxSR, mobileMaxBD sql.NullInt64
		transcodeFmt, wifiTranscodeFmt, mobileTranscodeFmt sql.NullString
		updatedAt                                  time.Time
	)
	if err := row.Scan(
		&maxBitrate, &maxSR, &maxBD,
		&wifiMaxBitrate, &wifiMaxSR, &wifiMaxBD,
		&mobileMaxBitrate, &mobileMaxSR, &mobileMaxBD,
		&transcodeFmt, &wifiTranscodeFmt, &mobileTranscodeFmt,
		&updatedAt,
	); err != nil {
		return UserStreamingPrefs{}, err
	}
	nullIntToPtr := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	nullStrToPtr := func(n sql.NullString) *string {
		if !n.Valid {
			return nil
		}
		return &n.String
	}
	out := UserStreamingPrefs{UserID: p.UserID, UpdatedAt: updatedAt}
	out.MaxBitrateKbps = nullIntToPtr(maxBitrate)
	out.MaxSampleRate = nullIntToPtr(maxSR)
	out.MaxBitDepth = nullIntToPtr(maxBD)
	out.WifiMaxBitrateKbps = nullIntToPtr(wifiMaxBitrate)
	out.WifiMaxSampleRate = nullIntToPtr(wifiMaxSR)
	out.WifiMaxBitDepth = nullIntToPtr(wifiMaxBD)
	out.MobileMaxBitrateKbps = nullIntToPtr(mobileMaxBitrate)
	out.MobileMaxSampleRate = nullIntToPtr(mobileMaxSR)
	out.MobileMaxBitDepth = nullIntToPtr(mobileMaxBD)
	out.TranscodeFormat = nullStrToPtr(transcodeFmt)
	out.WifiTranscodeFormat = nullStrToPtr(wifiTranscodeFmt)
	out.MobileTranscodeFormat = nullStrToPtr(mobileTranscodeFmt)
	return out, nil
}

// ──────────────────────────────────────────────────────────────
// EQ Profiles
// ──────────────────────────────────────────────────────────────

// scanEQProfile scans a row into an EQProfile; bandsJSON must already be scanned.
func scanEQProfile(id, userID, name string, bandsJSON []byte, isDefault bool, createdAt, updatedAt time.Time) (EQProfile, error) {
	p := EQProfile{
		ID:        id,
		UserID:    userID,
		Name:      name,
		IsDefault: isDefault,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if len(bandsJSON) > 0 {
		if err := json.Unmarshal(bandsJSON, &p.Bands); err != nil {
			return p, fmt.Errorf("unmarshal eq bands: %w", err)
		}
	}
	if p.Bands == nil {
		p.Bands = []EQBand{}
	}
	return p, nil
}

// ListEQProfiles returns all EQ profiles owned by userID.
func (s *Store) ListEQProfiles(ctx context.Context, userID string) ([]EQProfile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, bands, is_default, created_at, updated_at
		   FROM eq_profiles WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EQProfile
	for rows.Next() {
		var (
			id, uid, name        string
			bandsJSON            []byte
			isDefault            bool
			createdAt, updatedAt time.Time
		)
		if err := rows.Scan(&id, &uid, &name, &bandsJSON, &isDefault, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p, err := scanEQProfile(id, uid, name, bandsJSON, isDefault, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []EQProfile{}
	}
	return out, rows.Err()
}

// GetEQProfile returns a single EQ profile by id, scoped to userID.
func (s *Store) GetEQProfile(ctx context.Context, id, userID string) (EQProfile, error) {
	var (
		rid, uid, name       string
		bandsJSON            []byte
		isDefault            bool
		createdAt, updatedAt time.Time
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, bands, is_default, created_at, updated_at
		   FROM eq_profiles WHERE id = $1 AND user_id = $2`, id, userID).
		Scan(&rid, &uid, &name, &bandsJSON, &isDefault, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EQProfile{}, fmt.Errorf("eq profile not found")
		}
		return EQProfile{}, err
	}
	return scanEQProfile(rid, uid, name, bandsJSON, isDefault, createdAt, updatedAt)
}

// CreateEQProfile inserts a new EQ profile.
// If IsDefault is true, all other profiles for the user are unset as default first.
func (s *Store) CreateEQProfile(ctx context.Context, p CreateEQProfileParams) (EQProfile, error) {
	bandsJSON, err := json.Marshal(p.Bands)
	if err != nil {
		return EQProfile{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EQProfile{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if p.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE eq_profiles SET is_default = FALSE WHERE user_id = $1`, p.UserID); err != nil {
			return EQProfile{}, err
		}
	}

	var (
		id, uid, name        string
		retBands             []byte
		isDefault            bool
		createdAt, updatedAt time.Time
	)
	err = tx.QueryRow(ctx,
		`INSERT INTO eq_profiles (id, user_id, name, bands, is_default)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, name, bands, is_default, created_at, updated_at`,
		p.ID, p.UserID, p.Name, bandsJSON, p.IsDefault).
		Scan(&id, &uid, &name, &retBands, &isDefault, &createdAt, &updatedAt)
	if err != nil {
		return EQProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EQProfile{}, err
	}
	return scanEQProfile(id, uid, name, retBands, isDefault, createdAt, updatedAt)
}

// UpdateEQProfile updates the name and bands of an existing EQ profile.
func (s *Store) UpdateEQProfile(ctx context.Context, p UpdateEQProfileParams) (EQProfile, error) {
	bandsJSON, err := json.Marshal(p.Bands)
	if err != nil {
		return EQProfile{}, err
	}
	var (
		id, uid, name        string
		retBands             []byte
		isDefault            bool
		createdAt, updatedAt time.Time
	)
	err = s.pool.QueryRow(ctx,
		`UPDATE eq_profiles SET name = $3, bands = $4, updated_at = now()
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, name, bands, is_default, created_at, updated_at`,
		p.ID, p.UserID, p.Name, bandsJSON).
		Scan(&id, &uid, &name, &retBands, &isDefault, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EQProfile{}, fmt.Errorf("eq profile not found")
		}
		return EQProfile{}, err
	}
	return scanEQProfile(id, uid, name, retBands, isDefault, createdAt, updatedAt)
}

// DeleteEQProfile removes an EQ profile and any genre mappings that reference it.
func (s *Store) DeleteEQProfile(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM eq_profiles WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("eq profile not found")
	}
	return nil
}

// SetDefaultEQProfile marks a profile as the user's default,
// clearing the is_default flag on all other profiles for that user.
func (s *Store) SetDefaultEQProfile(ctx context.Context, id, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`UPDATE eq_profiles SET is_default = FALSE WHERE user_id = $1`, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE eq_profiles SET is_default = TRUE WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("eq profile not found")
	}
	return tx.Commit(ctx)
}

// ClearDefaultEQProfile removes the default flag from all profiles for a user.
func (s *Store) ClearDefaultEQProfile(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE eq_profiles SET is_default = FALSE WHERE user_id = $1`, userID)
	return err
}

// GetDefaultEQProfile returns the user's default EQ profile, or nil if none is set.
func (s *Store) GetDefaultEQProfile(ctx context.Context, userID string) (*EQProfile, error) {
	var (
		id, uid, name        string
		bandsJSON            []byte
		isDefault            bool
		createdAt, updatedAt time.Time
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, bands, is_default, created_at, updated_at
		   FROM eq_profiles WHERE user_id = $1 AND is_default = TRUE LIMIT 1`, userID).
		Scan(&id, &uid, &name, &bandsJSON, &isDefault, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p, err := scanEQProfile(id, uid, name, bandsJSON, isDefault, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ──────────────────────────────────────────────────────────────
// Genre → EQ Profile mappings
// ──────────────────────────────────────────────────────────────

// ListGenreEQMappings returns all genre→profile mappings for a user.
func (s *Store) ListGenreEQMappings(ctx context.Context, userID string) ([]GenreEQMapping, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT uge.user_id, uge.genre_id, g.name, uge.profile_id
		   FROM user_genre_eq uge
		   JOIN genres g ON g.id = uge.genre_id
		  WHERE uge.user_id = $1
		  ORDER BY g.name ASC`, userID)
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

// ---- Admin analytics ----

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

// ---- Invite tokens ----

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

// ---- Audit log ----

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

// ---- Site settings ----

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

// ---- Storage / artwork ----

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

// ---- Webhooks ----

// CreateWebhook inserts a new webhook endpoint.
func (s *Store) CreateWebhook(ctx context.Context, p CreateWebhookParams) (Webhook, error) {
	var w Webhook
	err := s.pool.QueryRow(ctx, `
		INSERT INTO webhooks (id, url, secret, events, description)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, url, secret, events, enabled, description, created_at, updated_at
	`, p.ID, p.URL, p.Secret, p.Events, p.Description).
		Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// ListWebhooks returns all webhooks ordered by creation time descending.
func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret, events, enabled, description, created_at, updated_at
		FROM webhooks ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Webhook, 0)
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, w)
	}
	return results, rows.Err()
}

// GetWebhook returns a single webhook by ID.
func (s *Store) GetWebhook(ctx context.Context, id string) (Webhook, error) {
	var w Webhook
	err := s.pool.QueryRow(ctx, `
		SELECT id, url, secret, events, enabled, description, created_at, updated_at
		FROM webhooks WHERE id = $1
	`, id).Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// UpdateWebhook updates an existing webhook.
func (s *Store) UpdateWebhook(ctx context.Context, p UpdateWebhookParams) (Webhook, error) {
	var w Webhook
	err := s.pool.QueryRow(ctx, `
		UPDATE webhooks SET url=$2, secret=$3, events=$4, enabled=$5, description=$6, updated_at=now()
		WHERE id=$1
		RETURNING id, url, secret, events, enabled, description, created_at, updated_at
	`, p.ID, p.URL, p.Secret, p.Events, p.Enabled, p.Description).
		Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// DeleteWebhook removes a webhook and its delivery history.
func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

// ListWebhooksForEvent returns all enabled webhooks subscribed to the given event.
func (s *Store) ListWebhooksForEvent(ctx context.Context, event string) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret, events, enabled, description, created_at, updated_at
		FROM webhooks WHERE enabled = true AND $1 = ANY(events)
	`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Webhook, 0)
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, w)
	}
	return results, rows.Err()
}

// CreateWebhookDelivery records a webhook delivery attempt.
func (s *Store) CreateWebhookDelivery(ctx context.Context, p CreateWebhookDeliveryParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (webhook_id, event, payload, status_code, error)
		VALUES ($1, $2, $3, $4, $5)
	`, p.WebhookID, p.Event, p.Payload, p.StatusCode, p.Error)
	return err
}

// ListWebhookDeliveries returns recent delivery attempts for a webhook.
func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]WebhookDelivery, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_id, event, payload, status_code, error, delivered_at
		FROM webhook_deliveries WHERE webhook_id = $1
		ORDER BY delivered_at DESC LIMIT $2
	`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]WebhookDelivery, 0)
	for rows.Next() {
		var d WebhookDelivery
		var statusCode sql.NullInt32
		var errMsg sql.NullString
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.Payload, &statusCode, &errMsg, &d.DeliveredAt); err != nil {
			return nil, err
		}
		if statusCode.Valid {
			v := int(statusCode.Int32)
			d.StatusCode = &v
		}
		if errMsg.Valid {
			d.Error = &errMsg.String
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
