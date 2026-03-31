package scrobble

import (
	"context"
	"log/slog"
	"time"

	"github.com/alexander-bruun/orb/services/internal/store"
)

// TrackInfo holds the minimal track metadata needed for scrobbling.
type TrackInfo struct {
	Title       string
	Artist      string // main artist display name
	Album       string
	DurationMs  int
}

// Scrobbler orchestrates now-playing and scrobble submissions for a user.
type Scrobbler struct {
	db     *store.Store
	lastfm *LastFMClient
	lb     *LBClient
}

// New returns a Scrobbler. Pass nil lastfm if LASTFM_API_KEY/SECRET are not configured.
func New(db *store.Store, lastfm *LastFMClient) *Scrobbler {
	return &Scrobbler{db: db, lastfm: lastfm, lb: NewLBClient()}
}

// NowPlaying sends now-playing notifications to enabled services for the given user.
// Runs asynchronously; errors are logged and do not propagate.
func (s *Scrobbler) NowPlaying(userID string, track TrackInfo) {
	go func() {
		settings, err := s.db.GetScrobbleSettings(context.Background(), userID)
		if err != nil {
			slog.Warn("scrobble: failed to load settings for now-playing", "user_id", userID, "err", err)
			return
		}

		if s.lastfm != nil && settings.LastFMEnabled && settings.LastFMSessionKey() != "" {
			if err := s.lastfm.NowPlaying(
				settings.LastFMSessionKey(),
				track.Artist, track.Title, track.Album,
				track.DurationMs/1000,
			); err != nil {
				slog.Warn("scrobble: last.fm now-playing failed", "user_id", userID, "err", err)
			}
		}

		if settings.LBEnabled && settings.LBToken() != "" {
			if err := s.lb.NowPlaying(settings.LBToken(), track.Artist, track.Title, track.Album); err != nil {
				slog.Warn("scrobble: listenbrainz now-playing failed", "user_id", userID, "err", err)
			}
		}
	}()
}

// Scrobble submits a scrobble to enabled services. startedAt is when the track began playing.
// Runs asynchronously; errors are logged and do not propagate.
func (s *Scrobbler) Scrobble(userID string, track TrackInfo, startedAt time.Time) {
	go func() {
		settings, err := s.db.GetScrobbleSettings(context.Background(), userID)
		if err != nil {
			slog.Warn("scrobble: failed to load settings for scrobble", "user_id", userID, "err", err)
			return
		}

		if s.lastfm != nil && settings.LastFMEnabled && settings.LastFMSessionKey() != "" {
			if err := s.lastfm.Scrobble(
				settings.LastFMSessionKey(),
				track.Artist, track.Title, track.Album,
				startedAt, track.DurationMs/1000,
			); err != nil {
				slog.Warn("scrobble: last.fm scrobble failed", "user_id", userID, "err", err)
			}
		}

		if settings.LBEnabled && settings.LBToken() != "" {
			if err := s.lb.Scrobble(settings.LBToken(), track.Artist, track.Title, track.Album, startedAt); err != nil {
				slog.Warn("scrobble: listenbrainz scrobble failed", "user_id", userID, "err", err)
			}
		}
	}()
}

// LoveTrack sends a love event to Last.fm if configured.
func (s *Scrobbler) LoveTrack(userID string, track TrackInfo, loved bool) {
	if s.lastfm == nil {
		return
	}
	go func() {
		settings, err := s.db.GetScrobbleSettings(context.Background(), userID)
		if err != nil || !settings.LastFMEnabled || settings.LastFMSessionKey() == "" {
			return
		}
		if loved {
			if err := s.lastfm.LoveTrack(settings.LastFMSessionKey(), track.Artist, track.Title); err != nil {
				slog.Warn("scrobble: last.fm love failed", "user_id", userID, "err", err)
			}
		} else {
			if err := s.lastfm.UnloveTrack(settings.LastFMSessionKey(), track.Artist, track.Title); err != nil {
				slog.Warn("scrobble: last.fm unlove failed", "user_id", userID, "err", err)
			}
		}
	}()
}

// GetMobileSession delegates to the Last.fm client (used by the user settings handler).
func (s *Scrobbler) GetMobileSession(username, password string) (string, error) {
	if s.lastfm == nil {
		return "", nil
	}
	return s.lastfm.GetMobileSession(username, password)
}

// LastFMConfigured returns true if the server has Last.fm API credentials.
func (s *Scrobbler) LastFMConfigured() bool { return s.lastfm != nil }
