// Package musicbrainz provides a rate-limited client for the MusicBrainz API.
// See https://musicbrainz.org/doc/MusicBrainz_API for documentation.
package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	baseURL   = "https://musicbrainz.org/ws/2"
	userAgent = "Orb/1.0 (https://github.com/alexander-bruun/orb)"
)

// Client is a rate-limited MusicBrainz API client.
type Client struct {
	http    *http.Client
	mu      sync.Mutex
	lastReq time.Time
}

// New creates a new MusicBrainz client with rate limiting.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// throttle enforces the MusicBrainz rate limit of 1 request per second.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elapsed := time.Since(c.lastReq); elapsed < time.Second {
		time.Sleep(time.Second - elapsed)
	}
	c.lastReq = time.Now()
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	c.throttle()

	u := baseURL + path
	if strings.Contains(u, "?") {
		u += "&fmt=json"
	} else {
		u += "?fmt=json"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 {
		// Rate limited — back off and retry once.
		time.Sleep(2 * time.Second)
		c.mu.Lock()
		c.lastReq = time.Now()
		c.mu.Unlock()
		return c.get(ctx, path)
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("musicbrainz: not found: %s", path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("musicbrainz: http %d for %s", resp.StatusCode, path)
	}
	return io.ReadAll(resp.Body)
}

// --- Response types ---

// ArtistSearchResponse is the top-level response from /artist/?query=...
type ArtistSearchResponse struct {
	Artists []ArtistResult `json:"artists"`
}

// ArtistResult represents a single artist from the MusicBrainz API.
type ArtistResult struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	SortName       string       `json:"sort-name"`
	Type           string       `json:"type"`
	Country        string       `json:"country"`
	Disambiguation string       `json:"disambiguation"`
	Score          int          `json:"score"`
	LifeSpan       LifeSpan     `json:"life-span"`
	Genres         []MBGenre    `json:"genres"`
	Tags           []MBTag      `json:"tags"`
	Relations      []MBRelation `json:"relations"`
}

// LifeSpan represents an artist's active dates.
type LifeSpan struct {
	Begin string `json:"begin"`
	End   string `json:"end"`
	Ended bool   `json:"ended"`
}

// MBGenre is a curated genre from MusicBrainz.
type MBGenre struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MBTag is a user-submitted tag from MusicBrainz.
type MBTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MBRelation represents a relationship between entities.
type MBRelation struct {
	Type       string        `json:"type"`
	TargetType string        `json:"target-type"`
	Artist     *ArtistResult `json:"artist,omitempty"`
	URL        *MBURL        `json:"url,omitempty"`
}

// MBURL is a URL resource from a MusicBrainz relationship.
type MBURL struct {
	Resource string `json:"resource"`
}

// ReleaseGroupSearchResponse is the top-level response from /release-group/?query=...
type ReleaseGroupSearchResponse struct {
	ReleaseGroups []ReleaseGroupResult `json:"release-groups"`
}

// ReleaseGroupResult represents a release group (album) from MusicBrainz.
type ReleaseGroupResult struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	PrimaryType string   `json:"primary-type"`
	FirstRelease string  `json:"first-release-date"`
	Score       int       `json:"score"`
	Genres      []MBGenre `json:"genres"`
	Tags        []MBTag   `json:"tags"`
	Releases    []struct {
		ID        string `json:"id"`
		Date      string `json:"date"`
		LabelInfo []struct {
			Label struct {
				Name string `json:"name"`
			} `json:"label"`
		} `json:"label-info"`
	} `json:"releases"`
}

// RecordingSearchResponse is the top-level response from /recording/?query=...
type RecordingSearchResponse struct {
	Recordings []RecordingResult `json:"recordings"`
}

// RecordingResult represents a recording (track) from MusicBrainz.
type RecordingResult struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Score  int       `json:"score"`
	ISRCs  []string  `json:"isrcs"`
	Genres []MBGenre `json:"genres"`
	Tags   []MBTag   `json:"tags"`
}

// --- Search methods ---

// SearchArtist searches MusicBrainz for an artist by name.
func (c *Client) SearchArtist(ctx context.Context, name string) (*ArtistSearchResponse, error) {
	path := fmt.Sprintf("/artist/?query=artist:%s&limit=5", url.QueryEscape(quoteQuery(name)))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var resp ArtistSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("musicbrainz: parse artist search: %w", err)
	}
	return &resp, nil
}

// GetArtist fetches full artist details by MBID, including genres, tags, and artist relationships.
func (c *Client) GetArtist(ctx context.Context, mbid string) (*ArtistResult, error) {
	path := fmt.Sprintf("/artist/%s?inc=genres+tags+artist-rels+url-rels", url.PathEscape(mbid))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var result ArtistResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("musicbrainz: parse artist: %w", err)
	}
	return &result, nil
}

// SearchReleaseGroup searches for a release group (album) by title and artist.
func (c *Client) SearchReleaseGroup(ctx context.Context, title, artist string) (*ReleaseGroupSearchResponse, error) {
	q := fmt.Sprintf("releasegroup:%s AND artist:%s", quoteQuery(title), quoteQuery(artist))
	path := fmt.Sprintf("/release-group/?query=%s&limit=5", url.QueryEscape(q))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var resp ReleaseGroupSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("musicbrainz: parse release-group search: %w", err)
	}
	return &resp, nil
}

// GetReleaseGroup fetches release group details by MBID, including genres, tags, and releases (for label info).
func (c *Client) GetReleaseGroup(ctx context.Context, mbid string) (*ReleaseGroupResult, error) {
	path := fmt.Sprintf("/release-group/%s?inc=genres+tags+releases", url.PathEscape(mbid))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var result ReleaseGroupResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("musicbrainz: parse release-group: %w", err)
	}
	return &result, nil
}

// SearchRecording searches for a recording (track) by title and artist.
func (c *Client) SearchRecording(ctx context.Context, title, artist string) (*RecordingSearchResponse, error) {
	q := fmt.Sprintf("recording:%s AND artist:%s", quoteQuery(title), quoteQuery(artist))
	path := fmt.Sprintf("/recording/?query=%s&limit=5", url.QueryEscape(q))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var resp RecordingSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("musicbrainz: parse recording search: %w", err)
	}
	return &resp, nil
}

// GetRecording fetches recording details by MBID, including genres, tags, and ISRCs.
func (c *Client) GetRecording(ctx context.Context, mbid string) (*RecordingResult, error) {
	path := fmt.Sprintf("/recording/%s?inc=genres+tags+isrcs", url.PathEscape(mbid))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var result RecordingResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("musicbrainz: parse recording: %w", err)
	}
	return &result, nil
}

// quoteQuery wraps a value in quotes for Lucene query syntax.
func quoteQuery(s string) string {
	// Escape internal quotes and wrap in double-quotes.
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
