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
    id              TEXT        PRIMARY KEY,
    name            TEXT        NOT NULL,
    sort_name       TEXT        NOT NULL,
    mbid            TEXT,
    artist_type     TEXT,                    -- Person | Group | Orchestra | Choir | Character | Other
    country         TEXT,                    -- ISO 3166-1 alpha-2
    begin_date      TEXT,                    -- ISO date (YYYY or YYYY-MM-DD)
    end_date        TEXT,
    disambiguation  TEXT,
    image_key       TEXT,                    -- object-store key for artist photo
    enriched_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE albums (
    id                  TEXT        PRIMARY KEY,
    artist_id           TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title               TEXT        NOT NULL,
    release_year        INT,
    label               TEXT,
    cover_art_key       TEXT,
    mbid                TEXT,
    album_type          TEXT,                    -- Album | EP | Single | Compilation | Live | Remix | Soundtrack
    release_date        TEXT,                    -- full ISO date (YYYY-MM-DD) when available
    release_group_mbid  TEXT,
    enriched_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
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
    isrc          TEXT,                   -- International Standard Recording Code
    mbid          TEXT,                   -- MusicBrainz recording ID
    enriched_at   TIMESTAMPTZ,
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

-- Genre taxonomy and entity-genre relationships
CREATE TABLE genres (
    id   TEXT PRIMARY KEY,               -- deterministic: sha256("genre:" + lower(name))[:8]
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE artist_genres (
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    genre_id  TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (artist_id, genre_id)
);

CREATE TABLE album_genres (
    album_id TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    genre_id TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (album_id, genre_id)
);

CREATE TABLE track_genres (
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    genre_id TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (track_id, genre_id)
);

-- Related artists from MusicBrainz artist-rels
CREATE TABLE related_artists (
    artist_id  TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    related_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    rel_type   TEXT NOT NULL,            -- "member of band", "collaboration", etc.
    PRIMARY KEY (artist_id, related_id, rel_type)
);

CREATE INDEX tracks_album_id_idx       ON tracks(album_id);
CREATE INDEX tracks_artist_id_idx      ON tracks(artist_id);
CREATE INDEX albums_artist_id_idx      ON albums(artist_id);
CREATE INDEX user_library_user_id_idx  ON user_library(user_id);
CREATE INDEX playlist_tracks_pl_idx    ON playlist_tracks(playlist_id, position);
CREATE INDEX queue_entries_user_idx    ON queue_entries(user_id, position);
CREATE INDEX play_history_user_idx     ON play_history(user_id, played_at DESC);
CREATE INDEX user_favorites_user_id_idx ON user_favorites(user_id);
CREATE INDEX artist_genres_artist_idx  ON artist_genres(artist_id);
CREATE INDEX album_genres_album_idx    ON album_genres(album_id);
CREATE INDEX track_genres_track_idx    ON track_genres(track_id);
CREATE INDEX related_artists_idx       ON related_artists(artist_id);

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
