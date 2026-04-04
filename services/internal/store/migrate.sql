-- Applied automatically on every API startup.
-- Clean schema: all columns defined in CREATE TABLE, no ALTER TABLE statements.

-- ── Users ──────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id                           TEXT        PRIMARY KEY,
    username                     TEXT        NOT NULL UNIQUE,
    email                        TEXT        NOT NULL UNIQUE,
    password_hash                TEXT        NOT NULL,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at                TIMESTAMPTZ,
    totp_secret                  TEXT,
    totp_enabled                 BOOLEAN     NOT NULL DEFAULT FALSE,
    totp_backup_codes            TEXT,
    is_admin                     BOOLEAN     NOT NULL DEFAULT FALSE,
    email_verified               BOOLEAN     NOT NULL DEFAULT FALSE,
    email_verification_token     TEXT,
    email_verification_expires_at TIMESTAMPTZ,
    is_active                    BOOLEAN     NOT NULL DEFAULT TRUE,
    storage_quota_bytes          BIGINT,
    profile_public               BOOLEAN     NOT NULL DEFAULT FALSE,
    bio                          TEXT,
    display_name                 TEXT,
    avatar_key                   TEXT,
    subsonic_password            TEXT
);

-- ── Artists ────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS artists (
    id             TEXT        PRIMARY KEY,
    name           TEXT        NOT NULL,
    sort_name      TEXT        NOT NULL,
    mbid           TEXT,
    artist_type    TEXT,
    country        TEXT,
    begin_date     TEXT,
    end_date       TEXT,
    disambiguation TEXT,
    image_key      TEXT,
    enriched_at    TIMESTAMPTZ,
    search_vector  tsvector,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Albums ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS albums (
    id                    TEXT        PRIMARY KEY,
    artist_id             TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title                 TEXT        NOT NULL,
    release_year          INT,
    label                 TEXT,
    cover_art_key         TEXT,
    mbid                  TEXT,
    album_type            TEXT,
    release_date          TEXT,
    release_group_mbid    TEXT,
    enriched_at           TIMESTAMPTZ,
    album_group_id        TEXT,
    edition               TEXT,
    search_vector         tsvector,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Tracks ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS tracks (
    id              TEXT        PRIMARY KEY,
    album_id        TEXT        REFERENCES albums(id) ON DELETE SET NULL,
    artist_id       TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title           TEXT        NOT NULL,
    track_number    INT,
    track_index     INT,
    disc_number     INT         NOT NULL DEFAULT 1,
    duration_ms     INT         NOT NULL,
    file_key        TEXT        NOT NULL,
    file_size       BIGINT      NOT NULL,
    format          TEXT        NOT NULL,
    bit_depth       INT,
    sample_rate     INT         NOT NULL,
    channels        INT         NOT NULL DEFAULT 2,
    bitrate_kbps    INT,
    seek_table      JSONB,
    fingerprint     TEXT,
    lyrics          TEXT,
    isrc            TEXT,
    mbid            TEXT,
    enriched_at     TIMESTAMPTZ,
    waveform_peaks  JSONB,
    audio_layouts   TEXT[]      NOT NULL DEFAULT '{"stereo"}',
    has_atmos       BOOLEAN     NOT NULL DEFAULT FALSE,
    audio_formats   JSONB,
    search_vector   tsvector,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Playlists ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS playlists (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    description   TEXT,
    cover_art_key TEXT,
    is_public     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id   TEXT        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    added_by      TEXT        REFERENCES users(id),
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (playlist_id, track_id)
);

-- ── User Library ───────────────────────────────────────────────────────────────

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

CREATE TABLE IF NOT EXISTS track_ratings (
    user_id   TEXT            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id  TEXT            NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    rating    SMALLINT        NOT NULL CHECK (rating BETWEEN 1 AND 5),
    rated_at  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);

-- ── Queue and History ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS queue_entries (
    id        BIGSERIAL       PRIMARY KEY,
    user_id   TEXT            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id  TEXT            NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position  INT             NOT NULL,
    source    TEXT,
    added_at  TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS play_history (
    id                 BIGSERIAL       PRIMARY KEY,
    user_id            TEXT            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id           TEXT            NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    played_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    duration_played_ms INT             NOT NULL
);

-- ── Ingest State ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ingest_state (
    path        TEXT        PRIMARY KEY,
    mtime_unix  BIGINT      NOT NULL,
    file_size   BIGINT      NOT NULL,
    track_id    TEXT        NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Genres ─────────────────────────────────────────────────────────────────────

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

-- ── Artist Relationships ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS related_artists (
    artist_id  TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    related_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    rel_type   TEXT NOT NULL,
    PRIMARY KEY (artist_id, related_id, rel_type)
);

CREATE TABLE IF NOT EXISTS track_featured_artists (
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    position  INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (track_id, artist_id)
);

-- ── Audio Features & Similarity ────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS track_features (
    track_id     TEXT        PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    bpm          REAL,
    key_estimate TEXT,
    replay_gain  REAL,
    extracted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS track_similarity (
    track_a TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    track_b TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    score   REAL NOT NULL,
    PRIMARY KEY (track_a, track_b),
    CHECK (track_a < track_b)
);

-- ── User Streaming Preferences ────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS user_streaming_prefs (
    user_id                  TEXT        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    max_bitrate_kbps         INT,
    max_sample_rate          INT,
    max_bit_depth            INT,
    wifi_max_bitrate_kbps    INT,
    wifi_max_sample_rate     INT,
    wifi_max_bit_depth       INT,
    mobile_max_bitrate_kbps  INT,
    mobile_max_sample_rate   INT,
    mobile_max_bit_depth     INT,
    transcode_format         TEXT,
    wifi_transcode_format    TEXT,
    mobile_transcode_format  TEXT,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Equalizer Profiles ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS eq_profiles (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    bands      JSONB       NOT NULL DEFAULT '[]',
    is_default BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_eq_default (
    user_id    TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES eq_profiles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_genre_eq (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    genre_id   TEXT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES eq_profiles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, genre_id)
);

-- ── Smart Playlists ───────────────────────────────────────────────────────────

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
    system        BOOLEAN     NOT NULL DEFAULT FALSE,
    last_built_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Playlist Collaboration ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS playlist_collaborators (
    playlist_id TEXT        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT        NOT NULL DEFAULT 'editor',
    invited_by  TEXT        NOT NULL REFERENCES users(id),
    invited_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at TIMESTAMPTZ,
    PRIMARY KEY (playlist_id, user_id)
);

CREATE TABLE IF NOT EXISTS playlist_invite_tokens (
    token       TEXT        PRIMARY KEY,
    playlist_id TEXT        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    invited_by  TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT        NOT NULL DEFAULT 'editor',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT now() + INTERVAL '7 days',
    used_at     TIMESTAMPTZ
);

-- ── Podcasts ───────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS podcasts (
    id            TEXT        PRIMARY KEY,
    title         TEXT        NOT NULL,
    description   TEXT,
    author        TEXT,
    rss_url       TEXT        NOT NULL UNIQUE,
    link          TEXT,
    cover_art_key TEXT,
    search_vector tsvector,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS podcast_episodes (
    id          TEXT        PRIMARY KEY,
    podcast_id  TEXT        NOT NULL REFERENCES podcasts(id) ON DELETE CASCADE,
    title       TEXT        NOT NULL,
    description TEXT,
    pub_date    TIMESTAMPTZ NOT NULL,
    guid        TEXT        NOT NULL,
    link        TEXT,
    audio_url   TEXT        NOT NULL,
    duration_ms BIGINT      NOT NULL DEFAULT 0,
    file_key    TEXT,
    file_size   BIGINT      NOT NULL DEFAULT 0,
    format      TEXT,
    search_vector tsvector,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (podcast_id, guid)
);

CREATE TABLE IF NOT EXISTS podcast_subscriptions (
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    podcast_id TEXT        NOT NULL REFERENCES podcasts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, podcast_id)
);

CREATE TABLE IF NOT EXISTS podcast_episode_progress (
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    episode_id  TEXT        NOT NULL REFERENCES podcast_episodes(id) ON DELETE CASCADE,
    position_ms BIGINT      NOT NULL DEFAULT 0,
    completed   BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id)
);

-- ── Audiobooks ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS audiobook_narrators (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    sort_name  TEXT        NOT NULL,
    image_key  TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audiobooks (
    id                 TEXT        PRIMARY KEY,
    title              TEXT        NOT NULL,
    edition            TEXT,
    author_id          TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    cover_art_key      TEXT,
    description        TEXT,
    series             TEXT,
    series_index       REAL,
    series_source      TEXT,
    series_confidence  REAL,
    published_year     INT,
    isbn               TEXT,
    asin               TEXT,
    subtitle           TEXT,
    ol_key             TEXT,
    file_key           TEXT,
    file_size          BIGINT      NOT NULL,
    format             TEXT        NOT NULL,
    duration_ms        BIGINT      NOT NULL,
    fingerprint        TEXT,
    search_vector      tsvector,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
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
    chapter_num  INT         NOT NULL DEFAULT 0,
    file_key     TEXT
);

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

-- ── Social: Follows & Activity ─────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS user_follows (
    follower_id  TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id  TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id)
);

CREATE TABLE IF NOT EXISTS user_activity (
    id          TEXT        PRIMARY KEY,
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   TEXT        NOT NULL,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Admin & Scrobbling ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invite_tokens (
    token      TEXT        PRIMARY KEY,
    email      TEXT        NOT NULL,
    created_by TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    used_by    TEXT        REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL   PRIMARY KEY,
    actor_id    TEXT        REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT        NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    detail      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS site_settings (
    key        TEXT        PRIMARY KEY,
    value      TEXT        NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_scrobble_settings (
    user_id         TEXT        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    lastfm_enabled  BOOLEAN     NOT NULL DEFAULT FALSE,
    lastfm_session_key TEXT,
    lastfm_username TEXT,
    lb_enabled      BOOLEAN     NOT NULL DEFAULT FALSE,
    lb_token        TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Webhooks ──────────────────────────────────────────────────────────────────

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

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id           BIGSERIAL   PRIMARY KEY,
    webhook_id   TEXT        NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event        TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    status_code  INT,
    error        TEXT,
    delivered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Indexes ────────────────────────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS albums_artist_id_idx ON albums(artist_id);
CREATE INDEX IF NOT EXISTS albums_group_id_idx ON albums(album_group_id);

CREATE INDEX IF NOT EXISTS tracks_album_id_idx ON tracks(album_id);
CREATE INDEX IF NOT EXISTS tracks_artist_id_idx ON tracks(artist_id);

CREATE INDEX IF NOT EXISTS user_library_user_id_idx ON user_library(user_id);
CREATE INDEX IF NOT EXISTS user_favorites_user_id_idx ON user_favorites(user_id);
CREATE INDEX IF NOT EXISTS track_ratings_user_idx ON track_ratings(user_id);

CREATE INDEX IF NOT EXISTS playlist_tracks_pl_idx ON playlist_tracks(playlist_id, position);
CREATE INDEX IF NOT EXISTS queue_entries_user_idx ON queue_entries(user_id, position);
CREATE INDEX IF NOT EXISTS play_history_user_idx ON play_history(user_id, played_at DESC);

CREATE INDEX IF NOT EXISTS artists_search_idx ON artists USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS albums_search_idx ON albums USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS tracks_search_idx ON tracks USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS podcasts_search_idx ON podcasts USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS podcast_episodes_search_idx ON podcast_episodes USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS audiobooks_search_idx ON audiobooks USING GIN(search_vector);

CREATE INDEX IF NOT EXISTS artist_genres_artist_idx ON artist_genres(artist_id);
CREATE INDEX IF NOT EXISTS album_genres_album_idx ON album_genres(album_id);
CREATE INDEX IF NOT EXISTS track_genres_track_idx ON track_genres(track_id);

CREATE INDEX IF NOT EXISTS related_artists_idx ON related_artists(artist_id);
CREATE INDEX IF NOT EXISTS track_featured_artists_track_idx ON track_featured_artists(track_id);

CREATE INDEX IF NOT EXISTS track_similarity_a_idx ON track_similarity(track_a, score DESC);
CREATE INDEX IF NOT EXISTS track_similarity_b_idx ON track_similarity(track_b, score DESC);

CREATE INDEX IF NOT EXISTS podcast_episodes_podcast_id_idx ON podcast_episodes(podcast_id, pub_date DESC);
CREATE INDEX IF NOT EXISTS podcast_subscriptions_user_idx ON podcast_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS podcast_episode_progress_user_idx ON podcast_episode_progress(user_id);

CREATE INDEX IF NOT EXISTS audiobook_chapters_book_idx ON audiobook_chapters(audiobook_id, chapter_num);
CREATE INDEX IF NOT EXISTS audiobook_narrator_links_idx ON audiobook_narrator_links(audiobook_id);
CREATE INDEX IF NOT EXISTS audiobook_progress_user_idx ON audiobook_progress(user_id);
CREATE INDEX IF NOT EXISTS audiobook_bookmarks_user_idx ON audiobook_bookmarks(user_id, audiobook_id);
CREATE INDEX IF NOT EXISTS audiobooks_author_idx ON audiobooks(author_id);

CREATE INDEX IF NOT EXISTS eq_profiles_user_idx ON eq_profiles(user_id);
CREATE INDEX IF NOT EXISTS user_genre_eq_user_idx ON user_genre_eq(user_id);
CREATE INDEX IF NOT EXISTS smart_playlists_user_idx ON smart_playlists(user_id);
CREATE INDEX IF NOT EXISTS playlist_collaborators_user_idx ON playlist_collaborators(user_id);

CREATE INDEX IF NOT EXISTS user_follows_followee_idx ON user_follows(followee_id);
CREATE INDEX IF NOT EXISTS user_activity_user_created ON user_activity(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS user_activity_created ON user_activity(created_at DESC);

CREATE INDEX IF NOT EXISTS invite_tokens_email_idx ON invite_tokens(email);
CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_actor_idx ON audit_logs(actor_id);

CREATE INDEX IF NOT EXISTS webhook_deliveries_webhook_idx ON webhook_deliveries(webhook_id, delivered_at DESC);

-- Backfill search_vector for rows created before this column was populated.
-- WHERE search_vector IS NULL makes this idempotent on every startup.
UPDATE artists SET search_vector = to_tsvector('english', name || ' ' || COALESCE(sort_name, '')) WHERE search_vector IS NULL;
UPDATE albums  SET search_vector = to_tsvector('english', title)                                   WHERE search_vector IS NULL;
UPDATE tracks  SET search_vector = to_tsvector('english', title)                                   WHERE search_vector IS NULL;
