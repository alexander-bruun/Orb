-- name: UpsertTrack :one
INSERT INTO tracks (
    id, album_id, artist_id, title, track_number, disc_number,
    duration_ms, file_key, file_size, format, bit_depth,
    sample_rate, channels, bitrate_kbps, seek_table, fingerprint
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16
)
ON CONFLICT (id) DO UPDATE
    SET album_id      = EXCLUDED.album_id,
        artist_id     = EXCLUDED.artist_id,
        title         = EXCLUDED.title,
        track_number  = EXCLUDED.track_number,
        disc_number   = EXCLUDED.disc_number,
        duration_ms   = EXCLUDED.duration_ms,
        file_key      = EXCLUDED.file_key,
        file_size     = EXCLUDED.file_size,
        format        = EXCLUDED.format,
        bit_depth     = EXCLUDED.bit_depth,
        sample_rate   = EXCLUDED.sample_rate,
        channels      = EXCLUDED.channels,
        bitrate_kbps  = EXCLUDED.bitrate_kbps,
        seek_table    = EXCLUDED.seek_table,
        fingerprint   = EXCLUDED.fingerprint
RETURNING *;

-- name: GetTrackByID :one
SELECT * FROM tracks WHERE id = $1;

-- name: GetTrackByFingerprint :one
SELECT * FROM tracks WHERE fingerprint = $1;

-- name: ListTracksByAlbum :many
SELECT * FROM tracks
WHERE album_id = $1
ORDER BY disc_number ASC, track_number ASC;

-- name: ListTracksByUser :many
SELECT t.* FROM tracks t
JOIN user_library ul ON ul.track_id = t.id
WHERE ul.user_id = $1
ORDER BY t.title ASC
LIMIT $2 OFFSET $3;

-- name: SearchTracks :many
SELECT t.* FROM tracks t
JOIN user_library ul ON ul.track_id = t.id
WHERE ul.user_id = $1
  AND t.search_vector @@ to_tsquery('english', $2)
ORDER BY ts_rank(t.search_vector, to_tsquery('english', $2)) DESC
LIMIT $3;

-- name: AddTrackToLibrary :exec
INSERT INTO user_library (user_id, track_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveTrackFromLibrary :exec
DELETE FROM user_library WHERE user_id = $1 AND track_id = $2;
