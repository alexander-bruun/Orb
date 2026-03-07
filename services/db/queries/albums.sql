-- name: UpsertAlbum :one
INSERT INTO albums (id, artist_id, title, release_year, label, cover_art_key, mbid)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE
    SET artist_id     = EXCLUDED.artist_id,
        title         = EXCLUDED.title,
        release_year  = EXCLUDED.release_year,
        label         = EXCLUDED.label,
        cover_art_key = COALESCE(EXCLUDED.cover_art_key, albums.cover_art_key),
        mbid          = EXCLUDED.mbid
RETURNING *;

-- name: GetAlbumByID :one
SELECT * FROM albums WHERE id = $1;

-- name: ListAlbums :many
SELECT * FROM albums
ORDER BY title ASC
LIMIT $1 OFFSET $2;

-- name: ListAlbumsByArtist :many
SELECT * FROM albums
WHERE artist_id = $1
ORDER BY release_year ASC, title ASC;

-- name: SearchAlbums :many
SELECT * FROM albums
WHERE search_vector @@ to_tsquery('english', $1)
ORDER BY ts_rank(search_vector, to_tsquery('english', $1)) DESC
LIMIT $2;
