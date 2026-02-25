-- name: MarkTrackIngested :exec
INSERT INTO ingest_state (track_id, ingested_at) VALUES ($1, now()) ON CONFLICT (track_id) DO NOTHING;

-- name: IsTrackIngested :one
SELECT EXISTS (SELECT 1 FROM ingest_state WHERE track_id = $1);
