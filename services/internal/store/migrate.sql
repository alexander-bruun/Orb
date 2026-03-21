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

ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret       TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled      BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_backup_codes TEXT;

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

-- Audio features for similarity computation
CREATE TABLE IF NOT EXISTS track_features (
    track_id     TEXT PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    bpm          REAL,
    key_estimate TEXT,
    replay_gain  REAL,
    extracted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Migrate existing chromaprint columns to the new in-house feature set.
ALTER TABLE track_features DROP COLUMN IF EXISTS chromaprint;
ALTER TABLE track_features DROP COLUMN IF EXISTS chromaprint_dur;
ALTER TABLE track_features ADD COLUMN IF NOT EXISTS bpm          REAL;
ALTER TABLE track_features ADD COLUMN IF NOT EXISTS key_estimate TEXT;
ALTER TABLE track_features ADD COLUMN IF NOT EXISTS replay_gain  REAL;

CREATE TABLE IF NOT EXISTS track_similarity (
    track_a  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    track_b  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    score    REAL NOT NULL,
    PRIMARY KEY (track_a, track_b),
    CHECK (track_a < track_b)
);

CREATE INDEX IF NOT EXISTS track_similarity_a_idx ON track_similarity(track_a, score DESC);
CREATE INDEX IF NOT EXISTS track_similarity_b_idx ON track_similarity(track_b, score DESC);

-- Track featured artists: artists that appear as "feat." in the track title.
-- The title stored in the tracks table is the clean version (feat. part stripped).
CREATE TABLE IF NOT EXISTS track_featured_artists (
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    position  INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (track_id, artist_id)
);

CREATE INDEX IF NOT EXISTS track_featured_artists_track_idx ON track_featured_artists(track_id);

-- Album variants: group editions of the same album and store edition label.
ALTER TABLE albums ADD COLUMN IF NOT EXISTS album_group_id TEXT;
ALTER TABLE albums ADD COLUMN IF NOT EXISTS edition        TEXT;
CREATE INDEX IF NOT EXISTS albums_group_id_idx ON albums(album_group_id);

-- Per-user streaming quality preferences.
CREATE TABLE IF NOT EXISTS user_streaming_prefs (
    user_id          TEXT        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    max_bitrate_kbps INT,          -- NULL = no limit; enforced as byte-rate throttle (any network)
    max_sample_rate  INT,          -- NULL = no limit; informational / client advisory (any network)
    max_bit_depth    INT,          -- NULL = no limit; informational / client advisory (any network)
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Network-specific quality overrides (override the any-network defaults above).
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS wifi_max_bitrate_kbps   INT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS wifi_max_sample_rate    INT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS wifi_max_bit_depth      INT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS mobile_max_bitrate_kbps INT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS mobile_max_sample_rate  INT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS mobile_max_bit_depth    INT;
-- On-the-fly transcode target format per network tier. NULL = no transcoding (pass-through + throttle).
-- Supported values: "mp3", "aac", "opus".
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS transcode_format        TEXT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS wifi_transcode_format   TEXT;
ALTER TABLE user_streaming_prefs ADD COLUMN IF NOT EXISTS mobile_transcode_format TEXT;

-- Equalizer profiles.
-- bands is a JSONB array of {frequency: number, gain: number, type: string}.
CREATE TABLE IF NOT EXISTS eq_profiles (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    bands      JSONB       NOT NULL DEFAULT '[]',
    is_default BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS eq_profiles_user_idx ON eq_profiles(user_id);

-- Per-genre EQ profile mapping: when a track's album/artist genre matches,
-- activate the corresponding profile automatically.
CREATE TABLE IF NOT EXISTS user_genre_eq (
    user_id    TEXT NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    genre_id   TEXT NOT NULL REFERENCES genres(id)   ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES eq_profiles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, genre_id)
);

CREATE INDEX IF NOT EXISTS user_genre_eq_user_idx ON user_genre_eq(user_id);

-- Admin flag: grant a user elevated access to analytics and admin endpoints.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

-- Email verification.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified            BOOLEAN     NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verification_token TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verification_expires_at TIMESTAMPTZ;

-- Pre-generated waveform peak data produced by audiowaveform during ingest.
-- Stored as a compact JSON float array (0–1 range, ~200 values).
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS waveform_peaks JSONB;

-- User lifecycle: active flag and storage quota.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active           BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS storage_quota_bytes BIGINT;

-- Invite tokens: admins generate these to allow new user registration.
CREATE TABLE IF NOT EXISTS invite_tokens (
    token      TEXT        PRIMARY KEY,
    email      TEXT        NOT NULL,
    created_by TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    used_by    TEXT        REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS invite_tokens_email_idx ON invite_tokens(email);

-- Audit log: immutable record of admin and user actions.
CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL   PRIMARY KEY,
    actor_id    TEXT        REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT        NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    detail      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_actor_idx      ON audit_logs(actor_id);

-- Site settings: runtime configuration stored in the database.
CREATE TABLE IF NOT EXISTS site_settings (
    key        TEXT        PRIMARY KEY,
    value      TEXT        NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Webhooks: outgoing HTTP callbacks for system events.
CREATE TABLE IF NOT EXISTS webhooks (
    id          TEXT        PRIMARY KEY,
    url         TEXT        NOT NULL,
    secret      TEXT        NOT NULL,
    events      TEXT[]      NOT NULL DEFAULT '{}',
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Webhook delivery log: records every outbound delivery attempt.
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id           BIGSERIAL   PRIMARY KEY,
    webhook_id   TEXT        NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event        TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    status_code  INT,
    error        TEXT,
    delivered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS webhook_deliveries_webhook_idx ON webhook_deliveries(webhook_id, delivered_at DESC);

-- Smart playlists: saved filter rules that evaluate to a dynamic track list.
-- rules is a JSONB array of {field, op, value} objects.
-- rule_match: 'all' (AND) | 'any' (OR).
CREATE TABLE IF NOT EXISTS smart_playlists (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    description   TEXT,
    rules         JSONB       NOT NULL DEFAULT '[]',
    rule_match    TEXT        NOT NULL DEFAULT 'all',
    sort_by       TEXT        NOT NULL DEFAULT 'title',
    sort_dir      TEXT        NOT NULL DEFAULT 'asc',
    limit_count   INT,
    system        BOOLEAN     NOT NULL DEFAULT false,
    last_built_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE smart_playlists ADD COLUMN IF NOT EXISTS system BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS smart_playlists_user_idx ON smart_playlists(user_id);

-- Track ratings: per-user 1–5 star rating for tracks.
CREATE TABLE IF NOT EXISTS track_ratings (
    user_id   TEXT NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    rating    SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    rated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);
CREATE INDEX IF NOT EXISTS track_ratings_user_idx ON track_ratings(user_id);

-- ── Audiobooks ────────────────────────────────────────────────────────────────
-- Audiobooks are a separate content type from music. They have chapters,
-- narrators, series info, and per-user progress / bookmarks.

CREATE TABLE IF NOT EXISTS audiobook_narrators (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    sort_name  TEXT        NOT NULL,
    image_key  TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audiobooks (
    id             TEXT        PRIMARY KEY,
    title          TEXT        NOT NULL,
    edition        TEXT,
    author_id      TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    cover_art_key  TEXT,
    description    TEXT,
    series         TEXT,
    series_index   REAL,
    published_year INT,
    isbn           TEXT,
    ol_key         TEXT,        -- Open Library work key, e.g. "/works/OL82563W"
    file_key       TEXT        NOT NULL,
    file_size      BIGINT      NOT NULL,
    format         TEXT        NOT NULL,
    duration_ms    BIGINT      NOT NULL,
    fingerprint    TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audiobook_narrator_links (
    audiobook_id TEXT NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
    narrator_id  TEXT NOT NULL REFERENCES audiobook_narrators(id) ON DELETE CASCADE,
    position     INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (audiobook_id, narrator_id)
);

CREATE TABLE IF NOT EXISTS audiobook_chapters (
    id           TEXT        PRIMARY KEY,
    audiobook_id TEXT        NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    start_ms     BIGINT      NOT NULL,
    end_ms       BIGINT      NOT NULL,
    chapter_num  INT         NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS audiobook_chapters_book_idx ON audiobook_chapters(audiobook_id, chapter_num);

CREATE TABLE IF NOT EXISTS audiobook_progress (
    user_id      TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    audiobook_id TEXT        NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
    position_ms  BIGINT      NOT NULL DEFAULT 0,
    completed    BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, audiobook_id)
);

CREATE TABLE IF NOT EXISTS audiobook_bookmarks (
    id           TEXT        PRIMARY KEY,
    user_id      TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    audiobook_id TEXT        NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
    position_ms  BIGINT      NOT NULL,
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audiobook_ingest_state (
    path         TEXT        PRIMARY KEY,
    mtime_unix   BIGINT      NOT NULL,
    file_size    BIGINT      NOT NULL,
    audiobook_id TEXT        NOT NULL,
    ingested_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audiobooks_author_idx        ON audiobooks(author_id);
CREATE INDEX IF NOT EXISTS audiobook_narrator_links_idx ON audiobook_narrator_links(audiobook_id);
CREATE INDEX IF NOT EXISTS audiobook_progress_user_idx  ON audiobook_progress(user_id);
CREATE INDEX IF NOT EXISTS audiobook_bookmarks_user_idx ON audiobook_bookmarks(user_id, audiobook_id);

-- Full-text search for audiobooks
ALTER TABLE audiobooks ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(series, ''))) STORED;
CREATE INDEX IF NOT EXISTS audiobooks_search_idx ON audiobooks USING GIN(search_vector);

-- Multi-file audiobook support: each chapter can have its own stored file.
-- NULL means the chapter is a time-range inside the parent audiobook's file_key (M4B mode).
-- Non-NULL means the chapter is streamed from its own file (directory/MP3 mode).
ALTER TABLE audiobook_chapters ADD COLUMN IF NOT EXISTS file_key TEXT;

-- The audiobook's file_key is now optional (NULL for directory-based multi-file books).
ALTER TABLE audiobooks ALTER COLUMN file_key DROP NOT NULL;

-- Track how series was determined.
ALTER TABLE audiobooks ADD COLUMN IF NOT EXISTS series_source TEXT;
ALTER TABLE audiobooks ADD COLUMN IF NOT EXISTS series_confidence REAL;

-- Store edition/variant labels like "Unabridged".
ALTER TABLE audiobooks ADD COLUMN IF NOT EXISTS edition TEXT;

-- Amazon Standard Identification Number extracted from folder-name annotation "[B0015T963C]".
ALTER TABLE audiobooks ADD COLUMN IF NOT EXISTS asin TEXT;

-- Subtitle extracted from folder-name or metadata (e.g. "Book Title - Subtitle Here").
ALTER TABLE audiobooks ADD COLUMN IF NOT EXISTS subtitle TEXT;
