-- name: RecordPlay :exec
INSERT INTO play_history (user_id, track_id, duration_played_ms)
VALUES ($1, $2, $3);

-- name: ListRecentlyPlayed :many
SELECT DISTINCT ON (ph.track_id) t.*, ph.played_at
FROM play_history ph
JOIN tracks t ON t.id = ph.track_id
WHERE ph.user_id = $1
ORDER BY ph.track_id, ph.played_at DESC
LIMIT $2;
