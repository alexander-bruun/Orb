package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// buildSmartPlaylistWhere takes the playlist rules and rule-match mode, and
// builds the WHERE clause with parameter placeholders. The returned args slice
// starts with userID at $1 (already included). The returned SQL fragment is
// safe to interpolate into a WHERE clause.
func buildSmartPlaylistWhere(rules []SmartPlaylistRule, ruleMatch string, userID string) (string, []any) {
	args := []any{userID}
	argN := 2

	conds := make([]string, 0, len(rules))
	for _, rule := range rules {
		cond, newArgs, newN := smartRuleToCond(rule, args, argN)
		if cond == "" {
			continue
		}
		conds = append(conds, cond)
		args = newArgs
		argN = newN
	}

	join := " AND "
	if ruleMatch == "any" {
		join = " OR "
	}

	where := "TRUE"
	if len(conds) > 0 {
		where = "(" + strings.Join(conds, join) + ")"
	}

	return where, args
}

// buildSmartPlaylistOrderBy maps sort field names to SQL ORDER BY clauses
// (including direction and NULLS LAST).
func buildSmartPlaylistOrderBy(sortField string, sortDir string) string {
	orderField := "t.title"
	switch sortField {
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
	if sortDir == "desc" {
		dir = "DESC"
	}
	return fmt.Sprintf("%s %s NULLS LAST", orderField, dir)
}

// scanSmartPlaylistTracks scans result rows into Track structs. The rows must
// match the column list produced by EvaluateSmartPlaylist's SELECT.
func scanSmartPlaylistTracks(rows pgx.Rows) ([]Track, error) {
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
	return out, rows.Err()
}
