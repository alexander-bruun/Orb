-- Applied automatically on every API startup.
-- All statements are idempotent (IF NOT EXISTS / ADD COLUMN IF NOT EXISTS).

CREATE TABLE IF NOT EXISTS users (
    id            TEXT        PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS artists (
    id            TEXT        PRIMARY KEY,
    name          TEXT        NOT NULL,
    sort_name     TEXT        NOT NULL,
    mbid          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS albums (
    id            TEXT        PRIMARY KEY,
    artist_id     TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title         TEXT        NOT NULL,
    release_year  INT,
    label         TEXT,
    cover_art_key TEXT,
    mbid          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tracks (
    id            TEXT        PRIMARY KEY,
    album_id      TEXT        REFERENCES albums(id) ON DELETE SET NULL,
    artist_id     TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title         TEXT        NOT NULL,
    track_number  INT,
    disc_number   INT         NOT NULL DEFAULT 1,
    duration_ms   INT         NOT NULL,
    file_key      TEXT        NOT NULL,
    file_size     BIGINT      NOT NULL,
    format        TEXT        NOT NULL,
    bit_depth     INT,
    sample_rate   INT         NOT NULL,
    channels      INT         NOT NULL DEFAULT 2,
    bitrate_kbps  INT,
    seek_table    JSONB,
    fingerprint   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_library (
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);

CREATE TABLE IF NOT EXISTS user_favorites (
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);

CREATE TABLE IF NOT EXISTS playlists (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    description   TEXT,
    cover_art_key TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id   TEXT        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (playlist_id, track_id)
);

CREATE TABLE IF NOT EXISTS queue_entries (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    source        TEXT,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS play_history (
    id                 BIGSERIAL   PRIMARY KEY,
    user_id            TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id           TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    played_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_played_ms INT         NOT NULL
);

CREATE TABLE IF NOT EXISTS ingest_state (
    path        TEXT        PRIMARY KEY,
    mtime_unix  BIGINT      NOT NULL,
    file_size   BIGINT      NOT NULL,
    track_id    TEXT        NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX IF NOT EXISTS tracks_album_id_idx        ON tracks(album_id);
CREATE INDEX IF NOT EXISTS tracks_artist_id_idx       ON tracks(artist_id);
CREATE INDEX IF NOT EXISTS albums_artist_id_idx       ON albums(artist_id);
CREATE INDEX IF NOT EXISTS user_library_user_id_idx   ON user_library(user_id);
CREATE INDEX IF NOT EXISTS user_favorites_user_id_idx ON user_favorites(user_id);
CREATE INDEX IF NOT EXISTS playlist_tracks_pl_idx     ON playlist_tracks(playlist_id, position);
CREATE INDEX IF NOT EXISTS queue_entries_user_idx     ON queue_entries(user_id, position);
CREATE INDEX IF NOT EXISTS play_history_user_idx      ON play_history(user_id, played_at DESC);

-- Full-text search columns (ADD COLUMN IF NOT EXISTS skips silently when already present)
ALTER TABLE tracks  ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, ''))) STORED;
ALTER TABLE albums  ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, ''))) STORED;
ALTER TABLE artists ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(name, ''))) STORED;

CREATE INDEX IF NOT EXISTS tracks_search_idx  ON tracks  USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS albums_search_idx  ON albums  USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS artists_search_idx ON artists USING GIN(search_vector);

-- Synced lyrics stored in LRC format (nullable — not all tracks have lyrics)
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS lyrics TEXT;

-- MusicBrainz enrichment columns
ALTER TABLE artists ADD COLUMN IF NOT EXISTS artist_type     TEXT;
ALTER TABLE artists ADD COLUMN IF NOT EXISTS country         TEXT;
ALTER TABLE artists ADD COLUMN IF NOT EXISTS begin_date      TEXT;
ALTER TABLE artists ADD COLUMN IF NOT EXISTS end_date        TEXT;
ALTER TABLE artists ADD COLUMN IF NOT EXISTS disambiguation  TEXT;
ALTER TABLE artists ADD COLUMN IF NOT EXISTS enriched_at     TIMESTAMPTZ;

ALTER TABLE albums ADD COLUMN IF NOT EXISTS album_type          TEXT;
ALTER TABLE albums ADD COLUMN IF NOT EXISTS release_date        TEXT;
ALTER TABLE albums ADD COLUMN IF NOT EXISTS release_group_mbid  TEXT;
ALTER TABLE albums ADD COLUMN IF NOT EXISTS enriched_at         TIMESTAMPTZ;

ALTER TABLE tracks ADD COLUMN IF NOT EXISTS isrc        TEXT;
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS mbid        TEXT;
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS enriched_at TIMESTAMPTZ;

-- Genre taxonomy
CREATE TABLE IF NOT EXISTS genres (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS artist_genres (
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    genre_id  TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (artist_id, genre_id)
);

CREATE TABLE IF NOT EXISTS album_genres (
    album_id TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    genre_id TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (album_id, genre_id)
);

CREATE TABLE IF NOT EXISTS track_genres (
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    genre_id TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (track_id, genre_id)
);

CREATE TABLE IF NOT EXISTS related_artists (
    artist_id  TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    related_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    rel_type   TEXT NOT NULL,
    PRIMARY KEY (artist_id, related_id, rel_type)
);

ALTER TABLE artists ADD COLUMN IF NOT EXISTS image_key TEXT;

CREATE INDEX IF NOT EXISTS artist_genres_artist_idx ON artist_genres(artist_id);
CREATE INDEX IF NOT EXISTS album_genres_album_idx   ON album_genres(album_id);
CREATE INDEX IF NOT EXISTS track_genres_track_idx   ON track_genres(track_id);
CREATE INDEX IF NOT EXISTS related_artists_idx      ON related_artists(artist_id);
