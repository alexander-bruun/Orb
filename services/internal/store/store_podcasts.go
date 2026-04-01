package store

import (
	"context"

	pgx "github.com/jackc/pgx/v5"
)

// ListPodcasts returns all podcasts.
func (s *Store) ListPodcasts(ctx context.Context) ([]Podcast, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, description, author, rss_url, link, cover_art_key, created_at, updated_at
		FROM podcasts
		ORDER BY title ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Podcast])
}

// GetPodcast returns a podcast by ID.
func (s *Store) GetPodcast(ctx context.Context, id string) (Podcast, error) {
	var p Podcast
	err := s.pool.QueryRow(ctx, `
		SELECT id, title, description, author, rss_url, link, cover_art_key, created_at, updated_at
		FROM podcasts
		WHERE id = $1
	`, id).Scan(&p.ID, &p.Title, &p.Description, &p.Author, &p.RssUrl, &p.Link, &p.CoverArtKey, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// GetPodcastByRSS returns a podcast by RSS URL.
func (s *Store) GetPodcastByRSS(ctx context.Context, rssURL string) (Podcast, error) {
	var p Podcast
	err := s.pool.QueryRow(ctx, `
		SELECT id, title, description, author, rss_url, link, cover_art_key, created_at, updated_at
		FROM podcasts
		WHERE rss_url = $1
	`, rssURL).Scan(&p.ID, &p.Title, &p.Description, &p.Author, &p.RssUrl, &p.Link, &p.CoverArtKey, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// CreatePodcast inserts a new podcast.
func (s *Store) CreatePodcast(ctx context.Context, p CreatePodcastParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO podcasts (id, title, description, author, rss_url, link, cover_art_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, p.ID, p.Title, p.Description, p.Author, p.RssUrl, p.Link, p.CoverArtKey)
	return err
}

// UpsertPodcastEpisode inserts or updates an episode.
func (s *Store) UpsertPodcastEpisode(ctx context.Context, e UpsertPodcastEpisodeParams) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO podcast_episodes (id, podcast_id, title, description, pub_date, guid, link, audio_url, duration_ms, file_key, file_size, format)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (podcast_id, guid) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			pub_date = EXCLUDED.pub_date,
			link = EXCLUDED.link,
			audio_url = EXCLUDED.audio_url,
			duration_ms = CASE WHEN EXCLUDED.duration_ms > 0 THEN EXCLUDED.duration_ms ELSE podcast_episodes.duration_ms END,
			file_key = COALESCE(podcast_episodes.file_key, EXCLUDED.file_key),
			file_size = CASE WHEN EXCLUDED.file_size > 0 THEN EXCLUDED.file_size ELSE podcast_episodes.file_size END,
			format = COALESCE(podcast_episodes.format, EXCLUDED.format)
		RETURNING id
	`, e.ID, e.PodcastID, e.Title, e.Description, e.PubDate, e.Guid, e.Link, e.AudioUrl, e.DurationMs, e.FileKey, e.FileSize, e.Format).Scan(&id)
	return id, err
}

// ListPodcastEpisodes returns episodes for a podcast, ordered by pub_date DESC.
func (s *Store) ListPodcastEpisodes(ctx context.Context, podcastID string, limit, offset int32) ([]PodcastEpisode, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, podcast_id, title, description, pub_date, guid, link, audio_url, duration_ms, file_key, file_size, format, created_at
		FROM podcast_episodes
		WHERE podcast_id = $1
		ORDER BY pub_date DESC
		LIMIT $2 OFFSET $3
	`, podcastID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[PodcastEpisode])
}

// GetPodcastEpisode returns an episode by ID.
func (s *Store) GetPodcastEpisode(ctx context.Context, id string) (PodcastEpisode, error) {
	var e PodcastEpisode
	err := s.pool.QueryRow(ctx, `
		SELECT id, podcast_id, title, description, pub_date, guid, link, audio_url, duration_ms, file_key, file_size, format, created_at
		FROM podcast_episodes
		WHERE id = $1
	`, id).Scan(&e.ID, &e.PodcastID, &e.Title, &e.Description, &e.PubDate, &e.Guid, &e.Link, &e.AudioUrl, &e.DurationMs, &e.FileKey, &e.FileSize, &e.Format, &e.CreatedAt)
	return e, err
}

// SubscribeUserToPodcast subscribes a user to a podcast.
func (s *Store) SubscribeUserToPodcast(ctx context.Context, userID, podcastID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO podcast_subscriptions (user_id, podcast_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, userID, podcastID)
	return err
}

// UnsubscribeUserFromPodcast unsubscribes a user from a podcast.
func (s *Store) UnsubscribeUserFromPodcast(ctx context.Context, userID, podcastID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM podcast_subscriptions
		WHERE user_id = $1 AND podcast_id = $2
	`, userID, podcastID)
	return err
}

// ListUserSubscriptions returns all podcasts a user is subscribed to.
func (s *Store) ListUserSubscriptions(ctx context.Context, userID string) ([]Podcast, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.title, p.description, p.author, p.rss_url, p.link, p.cover_art_key, p.created_at, p.updated_at
		FROM podcasts p
		JOIN podcast_subscriptions s ON p.id = s.podcast_id
		WHERE s.user_id = $1
		ORDER BY p.title ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Podcast])
}

// UpsertPodcastEpisodeProgress updates per-user progress for an episode.
func (s *Store) UpsertPodcastEpisodeProgress(ctx context.Context, p UpsertPodcastEpisodeProgressParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO podcast_episode_progress (user_id, episode_id, position_ms, completed, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, episode_id) DO UPDATE SET
			position_ms = EXCLUDED.position_ms,
			completed = EXCLUDED.completed,
			updated_at = EXCLUDED.updated_at
	`, p.UserID, p.EpisodeID, p.PositionMs, p.Completed)
	return err
}

// GetPodcastEpisodeProgress returns progress for a user and episode.
func (s *Store) GetPodcastEpisodeProgress(ctx context.Context, userID, episodeID string) (PodcastEpisodeProgress, error) {
	var p PodcastEpisodeProgress
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, episode_id, position_ms, completed, updated_at
		FROM podcast_episode_progress
		WHERE user_id = $1 AND episode_id = $2
	`, userID, episodeID).Scan(&p.UserID, &p.EpisodeID, &p.PositionMs, &p.Completed, &p.UpdatedAt)
	return p, err
}

// ListInProgressEpisodes returns episodes the user has started but not finished.
func (s *Store) ListInProgressEpisodes(ctx context.Context, userID string, limit int) ([]PodcastEpisode, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.podcast_id, e.title, e.description, e.pub_date, e.guid, e.link, e.audio_url, e.duration_ms, e.file_key, e.file_size, e.format, e.created_at
		FROM podcast_episodes e
		JOIN podcast_episode_progress p ON e.id = p.episode_id
		WHERE p.user_id = $1 AND p.completed = FALSE AND p.position_ms > 0
		ORDER BY p.updated_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[PodcastEpisode])
}

// UpdatePodcastEpisodeFile updates the file information for an episode after download.
func (s *Store) UpdatePodcastEpisodeFile(ctx context.Context, episodeID string, fileKey string, fileSize int64, format string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE podcast_episodes
		SET file_key = $1, file_size = $2, format = $3
		WHERE id = $4
	`, fileKey, fileSize, format, episodeID)
	return err
}
