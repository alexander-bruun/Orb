-- name: DeleteAlbumsWithoutCover :exec
DELETE FROM albums WHERE cover_art_key IS NULL;

-- name: DeleteTracksForAlbumsWithoutCover :exec
DELETE FROM tracks WHERE album_id IN (SELECT id FROM albums WHERE cover_art_key IS NULL);
