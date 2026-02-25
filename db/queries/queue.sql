-- name: GetQueue :many
SELECT t.* FROM tracks t
JOIN queue_entries qe ON qe.track_id = t.id
WHERE qe.user_id = $1
ORDER BY qe.position ASC;

-- name: ClearQueue :exec
DELETE FROM queue_entries WHERE user_id = $1;

-- name: InsertQueueEntry :exec
INSERT INTO queue_entries (user_id, track_id, position, source)
VALUES ($1, $2, $3, $4);

-- name: GetMaxQueuePosition :one
SELECT COALESCE(MAX(position), 0)::int FROM queue_entries WHERE user_id = $1;

-- name: GetMinQueuePosition :one
SELECT COALESCE(MIN(position), 0)::int FROM queue_entries WHERE user_id = $1;
