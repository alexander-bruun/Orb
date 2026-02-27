-- db/schema.sql — Atlas source of truth

CREATE TABLE users (
    id            TEXT        PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE artists (
    id            TEXT        PRIMARY KEY,
    name          TEXT        NOT NULL,
    sort_name     TEXT        NOT NULL,
    mbid          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE albums (
    id            TEXT        PRIMARY KEY,
    artist_id     TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title         TEXT        NOT NULL,
    release_year  INT,
    label         TEXT,
    cover_art_key TEXT,
    mbid          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tracks (
    id            TEXT        PRIMARY KEY,
    album_id      TEXT        REFERENCES albums(id) ON DELETE SET NULL,
    artist_id     TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title         TEXT        NOT NULL,
    track_number  INT,
    disc_number   INT         NOT NULL DEFAULT 1,
    duration_ms   INT         NOT NULL,
    file_key      TEXT        NOT NULL,
    file_size     BIGINT      NOT NULL,
    format        TEXT        NOT NULL,    -- flac | wav | mp3
    bit_depth     INT,                    -- 16 | 24 | 32 (NULL for MP3)
    sample_rate   INT         NOT NULL,
    channels      INT         NOT NULL DEFAULT 2,
    bitrate_kbps  INT,
    seek_table    JSONB,                  -- precomputed frame offsets for seeking
    fingerprint   TEXT,
    lyrics        TEXT,                   -- LRC-format synced lyrics (optional)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-user library ownership
CREATE TABLE user_library (
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);

CREATE TABLE playlists (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    description   TEXT,
    cover_art_key TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE playlist_tracks (
    playlist_id   TEXT        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (playlist_id, track_id)
);

CREATE TABLE queue_entries (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    source        TEXT,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_favorites (
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);

CREATE TABLE play_history (
    id                BIGSERIAL   PRIMARY KEY,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id          TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    played_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_played_ms INT        NOT NULL
);

CREATE INDEX tracks_album_id_idx       ON tracks(album_id);
CREATE INDEX tracks_artist_id_idx      ON tracks(artist_id);
CREATE INDEX albums_artist_id_idx      ON albums(artist_id);
CREATE INDEX user_library_user_id_idx  ON user_library(user_id);
CREATE INDEX playlist_tracks_pl_idx    ON playlist_tracks(playlist_id, position);
CREATE INDEX queue_entries_user_idx    ON queue_entries(user_id, position);
CREATE INDEX play_history_user_idx     ON play_history(user_id, played_at DESC);
CREATE INDEX user_favorites_user_id_idx ON user_favorites(user_id);

-- Full-text search
ALTER TABLE tracks  ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, ''))) STORED;
ALTER TABLE albums  ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, ''))) STORED;
ALTER TABLE artists ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(name, ''))) STORED;

CREATE INDEX tracks_search_idx  ON tracks  USING GIN(search_vector);
CREATE INDEX albums_search_idx  ON albums  USING GIN(search_vector);
CREATE INDEX artists_search_idx ON artists USING GIN(search_vector);

-- Ingest state: tracks which files have been processed so re-runs skip unchanged files.
-- Keyed by absolute path; mtime_unix + file_size serve as a cheap change-detection
-- fingerprint so the ingest tool only hashes and re-processes files that have actually
-- changed on disk.
CREATE TABLE ingest_state (
    path        TEXT        PRIMARY KEY,
    mtime_unix  BIGINT      NOT NULL,
    file_size   BIGINT      NOT NULL,
    track_id    TEXT        NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
