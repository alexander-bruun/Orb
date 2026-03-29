package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
	where, args := buildSmartPlaylistWhere(sp.Rules, sp.RuleMatch, sp.UserID)
	orderBy := buildSmartPlaylistOrderBy(sp.SortBy, sp.SortDir)

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
		ORDER BY %s
		%s`,
		where, orderBy, limitClause)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out, err := scanSmartPlaylistTracks(rows)
	if err != nil {
		return nil, err
	}
	// Mark the playlist as built.
	_, _ = s.pool.Exec(ctx, `UPDATE smart_playlists SET last_built_at=now() WHERE id=$1`, sp.ID)
	return out, nil
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
		sub := "(SELECT COUNT(*) FROM play_history ph WHERE ph.track_id = t.id AND ph.user_id = $1)"
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

// ── Regular playlist CRUD ──────────────────────────────────────────────────

func (s *Store) ListPlaylistsByUser(ctx context.Context, userID string) ([]Playlist, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, description, cover_art_key, is_public,
		        (SELECT COUNT(*) FROM playlist_tracks WHERE playlist_id = playlists.id)::int AS track_count,
		        created_at FROM playlists WHERE user_id = $1 ORDER BY updated_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlaylists(rows)
}

func (s *Store) CreatePlaylist(ctx context.Context, p CreatePlaylistParams) (Playlist, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO playlists (id, user_id, name, description, cover_art_key) VALUES ($1, $2, $3, $4, $5) RETURNING id, user_id, name, description, cover_art_key, is_public, 0 AS track_count, created_at`,
		p.ID, p.UserID, p.Name, p.Description, p.CoverArtKey)
	return scanPlaylist(row)
}

func (s *Store) GetPlaylistByID(ctx context.Context, id string) (Playlist, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, description, cover_art_key, is_public,
		        (SELECT COUNT(*) FROM playlist_tracks WHERE playlist_id = playlists.id)::int AS track_count,
		        created_at FROM playlists WHERE id = $1`,
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
		`UPDATE playlists SET name = $2, description = $3, cover_art_key = $4, is_public = $5, updated_at = now() WHERE id = $1`,
		p.ID, p.Name, p.Description, p.CoverArtKey, p.IsPublic)
	return err
}

func (s *Store) DeletePlaylist(ctx context.Context, p DeletePlaylistParams) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM playlists WHERE id = $1`, p.ID)
	return err
}

func (s *Store) GetMaxPlaylistPosition(ctx context.Context, playlistID string) (int, error) {
	var pos int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0)::int FROM playlist_tracks WHERE playlist_id = $1`, playlistID).Scan(&pos)
	return pos, err
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

// ── Scan helpers ───────────────────────────────────────────────────────────

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

func scanPlaylist(row pgx.Row) (Playlist, error) {
	var pl Playlist
	var desc, coverArtKey sql.NullString
	var createdAt time.Time
	if err := row.Scan(&pl.ID, &pl.UserID, &pl.Name, &desc, &coverArtKey, &pl.IsPublic, &pl.TrackCount, &createdAt); err != nil {
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
		if err := rows.Scan(&pl.ID, &pl.UserID, &pl.Name, &desc, &coverArtKey, &pl.IsPublic, &pl.TrackCount, &createdAt); err != nil {
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
