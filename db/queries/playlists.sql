-- name: CreatePlaylist :one
INSERT INTO playlists (id, user_id, name, description, cover_art_key)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPlaylistByID :one
SELECT * FROM playlists WHERE id = $1;

-- name: ListPlaylistsByUser :many
SELECT * FROM playlists
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: UpdatePlaylist :one
UPDATE playlists
SET name          = $2,
    description   = $3,
    cover_art_key = $4,
    updated_at    = now()
WHERE id = $1
RETURNING *;

-- name: DeletePlaylist :exec
DELETE FROM playlists WHERE id = $1 AND user_id = $2;

-- name: AddTrackToPlaylist :exec
INSERT INTO playlist_tracks (playlist_id, track_id, position)
VALUES ($1, $2, $3);

-- name: RemoveTrackFromPlaylist :exec
DELETE FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2;

-- name: ListPlaylistTracks :many
SELECT t.* FROM tracks t
JOIN playlist_tracks pt ON pt.track_id = t.id
WHERE pt.playlist_id = $1
ORDER BY pt.position ASC;

-- name: UpdatePlaylistTrackOrder :exec
UPDATE playlist_tracks SET position = $3
WHERE playlist_id = $1 AND track_id = $2;

-- name: GetMaxPlaylistPosition :one
SELECT COALESCE(MAX(position), 0)::int FROM playlist_tracks
WHERE playlist_id = $1;
