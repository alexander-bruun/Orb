-- name: UpsertArtist :one
INSERT INTO artists (id, name, sort_name, mbid)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
    SET name = EXCLUDED.name,
        sort_name = EXCLUDED.sort_name,
        mbid = EXCLUDED.mbid
RETURNING *;

-- name: GetArtistByID :one
SELECT * FROM artists WHERE id = $1;

-- name: ListArtists :many
SELECT * FROM artists
ORDER BY sort_name ASC
LIMIT $1 OFFSET $2;

-- name: SearchArtists :many
SELECT * FROM artists
WHERE search_vector @@ to_tsquery('english', $1)
ORDER BY ts_rank(search_vector, to_tsquery('english', $1)) DESC
LIMIT $2;
