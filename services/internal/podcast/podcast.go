package podcast

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
)

// Service handles podcast feed fetching, episode management, and background refreshes.
type Service struct {
	db  *store.Store
	obj objstore.ObjectStore
	fp  *gofeed.Parser
}

// New creates a new podcast Service.
func New(db *store.Store, obj objstore.ObjectStore) *Service {
	return &Service{
		db:  db,
		obj: obj,
		fp:  gofeed.NewParser(),
	}
}

// AddPodcastByRSS fetches the RSS feed at the given URL and adds it to the database.
func (s *Service) AddPodcastByRSS(ctx context.Context, rssURL string) (store.Podcast, error) {
	// Check if already exists
	if p, err := s.db.GetPodcastByRSS(ctx, rssURL); err == nil {
		return p, nil
	}

	feed, err := s.fp.ParseURLWithContext(rssURL, ctx)
	if err != nil {
		return store.Podcast{}, fmt.Errorf("parse feed: %w", err)
	}

	id := uuid.New().String()
	p := store.CreatePodcastParams{
		ID:          id,
		Title:       feed.Title,
		Description: &feed.Description,
		Author:      &feed.Author.Name,
		RssUrl:      rssURL,
		Link:        &feed.Link,
	}

	if feed.Author != nil {
		p.Author = &feed.Author.Name
	}
	if feed.Image != nil && feed.Image.URL != "" {
		// Download cover art
		resp, err := http.Get(feed.Image.URL)
		if err == nil && resp.StatusCode == http.StatusOK {
			key := fmt.Sprintf("podcasts/%s/cover.jpg", id)
			if err := s.obj.Put(ctx, key, resp.Body, resp.ContentLength); err == nil {
				p.CoverArtKey = &key
			}
			resp.Body.Close()
		}
	}

	if err := s.db.CreatePodcast(ctx, p); err != nil {
		return store.Podcast{}, fmt.Errorf("create podcast: %w", err)
	}

	// Fetch initial episodes
	if err := s.RefreshPodcast(ctx, id); err != nil {
		slog.Error("initial podcast refresh failed", "id", id, "err", err)
	}

	return s.db.GetPodcast(ctx, id)
}

// RefreshPodcast fetches the latest RSS feed and updates episodes in the DB.
func (s *Service) RefreshPodcast(ctx context.Context, id string) error {
	p, err := s.db.GetPodcast(ctx, id)
	if err != nil {
		return err
	}

	feed, err := s.fp.ParseURLWithContext(p.RssUrl, ctx)
	if err != nil {
		return fmt.Errorf("parse feed: %w", err)
	}

	for _, item := range feed.Items {
		if len(item.Enclosures) == 0 {
			continue
		}

		// Find first audio enclosure
		var audioEnc *gofeed.Enclosure
		for _, enc := range item.Enclosures {
			if strings.HasPrefix(enc.Type, "audio/") {
				audioEnc = enc
				break
			}
		}
		if audioEnc == nil {
			continue
		}

		durationMs := int64(0)
		if item.ITunesExt != nil && item.ITunesExt.Duration != "" {
			// Duration can be HH:MM:SS or SS or MM:SS
			parts := strings.Split(item.ITunesExt.Duration, ":")
			var d time.Duration
			switch len(parts) {
			case 1:
				if s, err := time.ParseDuration(parts[0] + "s"); err == nil {
					d = s
				}
			case 2:
				if m, err := time.ParseDuration(parts[0] + "m"); err == nil {
					if s, err := time.ParseDuration(parts[1] + "s"); err == nil {
						d = m + s
					}
				}
			case 3:
				if h, err := time.ParseDuration(parts[0] + "h"); err == nil {
					if m, err := time.ParseDuration(parts[1] + "m"); err == nil {
						if s, err := time.ParseDuration(parts[2] + "s"); err == nil {
							d = h + m + s
						}
					}
				}
			}
			durationMs = d.Milliseconds()
		}

		pubDate := time.Now()
		if item.PublishedParsed != nil {
			pubDate = *item.PublishedParsed
		}

		epID := uuid.New().String()
		_, err = s.db.UpsertPodcastEpisode(ctx, store.UpsertPodcastEpisodeParams{
			ID:          epID,
			PodcastID:   id,
			Title:       item.Title,
			Description: &item.Description,
			PubDate:     pubDate,
			Guid:        item.GUID,
			Link:        &item.Link,
			AudioUrl:    audioEnc.URL,
			DurationMs:  durationMs,
			FileSize:    0, // Will be updated on download or if we can parse it from enclosure
		})
		if err != nil {
			slog.Error("upsert episode failed", "podcast_id", id, "guid", item.GUID, "err", err)
		}
	}

	return nil
}

// DownloadEpisode downloads the audio file for an episode and stores it in objstore.
func (s *Service) DownloadEpisode(ctx context.Context, episodeID string) error {
	ep, err := s.db.GetPodcastEpisode(ctx, episodeID)
	if err != nil {
		return err
	}

	if ep.FileKey != nil {
		// Already downloaded
		exists, _ := s.obj.Exists(ctx, *ep.FileKey)
		if exists {
			return nil
		}
	}

	slog.Info("downloading podcast episode", "id", episodeID, "url", ep.AudioUrl)

	req, err := http.NewRequestWithContext(ctx, "GET", ep.AudioUrl, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	ext := ".mp3" // Default
	if ep.Format != nil {
		ext = "." + *ep.Format
	} else {
		// Try to guess from URL or Content-Type
		if strings.Contains(resp.Header.Get("Content-Type"), "mpeg") {
			ext = ".mp3"
		} else if strings.Contains(resp.Header.Get("Content-Type"), "aac") {
			ext = ".m4a"
		}
	}

	fileKey := fmt.Sprintf("podcasts/%s/%s%s", ep.PodcastID, ep.ID, ext)
	
	// We use Put which takes a reader.
	if err := s.obj.Put(ctx, fileKey, resp.Body, resp.ContentLength); err != nil {
		return fmt.Errorf("store file: %w", err)
	}

	format := strings.TrimPrefix(ext, ".")
	if err := s.db.UpdatePodcastEpisodeFile(ctx, episodeID, fileKey, resp.ContentLength, format); err != nil {
		return fmt.Errorf("update episode file info: %w", err)
	}

	slog.Info("downloaded podcast episode", "id", episodeID, "key", fileKey)
	return nil
}

// StartBackgroundWorker starts a loop that refreshes all podcasts periodically.
func (s *Service) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshAll(ctx)
		}
	}
}

func (s *Service) refreshAll(ctx context.Context) {
	podcasts, err := s.db.ListPodcasts(ctx)
	if err != nil {
		slog.Error("list podcasts for refresh failed", "err", err)
		return
	}

	for _, p := range podcasts {
		if err := s.RefreshPodcast(ctx, p.ID); err != nil {
			slog.Error("refresh podcast failed", "id", p.ID, "err", err)
		}
	}
}
