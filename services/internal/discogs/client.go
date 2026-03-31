// Package discogs provides a rate-limited client for the Discogs API.
// See https://www.discogs.com/developers for documentation.
package discogs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	baseURL   = "https://api.discogs.com"
	userAgent = "Orb/1.0 +https://github.com/alexander-bruun/orb"
	// Discogs allows 60 authenticated requests/min — 1 req/sec keeps us safe.
	minInterval = time.Second
)

// Client is a rate-limited Discogs API client.
type Client struct {
	http    *http.Client
	token   string
	mu      sync.Mutex
	lastReq time.Time
}

// New creates a new Discogs client. token is the personal access token from
// https://www.discogs.com/settings/developers; may be empty for unauthenticated
// requests (25 req/min limit applies).
func New(token string) *Client {
	return &Client{
		http:  &http.Client{Timeout: 15 * time.Second},
		token: token,
	}
}

// throttle enforces the rate limit (1 request per second).
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elapsed := time.Since(c.lastReq); elapsed < minInterval {
		time.Sleep(minInterval - elapsed)
	}
	c.lastReq = time.Now()
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	c.throttle()

	u := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Discogs token="+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("discogs: response close failed", "err", closeErr)
		}
	}()

	if resp.StatusCode == 429 {
		// Rate limited — back off and retry once.
		time.Sleep(5 * time.Second)
		c.mu.Lock()
		c.lastReq = time.Now()
		c.mu.Unlock()
		return c.get(ctx, path)
	}
	if resp.StatusCode == 404 {
		return nil, nil // not found — caller treats nil body as no result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discogs: http %d for %s", resp.StatusCode, path)
	}
	return io.ReadAll(resp.Body)
}

// FetchImage downloads a Discogs image URL, sending the auth token as required.
func (c *Client) FetchImage(ctx context.Context, imageURL string) ([]byte, error) {
	if imageURL == "" {
		return nil, nil
	}
	c.throttle()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Discogs token="+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("discogs: image close failed", "err", closeErr)
		}
	}()
	if resp.StatusCode != 200 {
		return nil, nil
	}
	return io.ReadAll(resp.Body)
}

// --- Response types ---

// SearchResponse is the top-level response from /database/search.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

// SearchResult is a single entry from a Discogs database search.
type SearchResult struct {
	ID          int      `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Thumb       string   `json:"thumb"`
	CoverImage  string   `json:"cover_image"`
	Genres      []string `json:"genre"`
	Styles      []string `json:"style"`
	Year        string   `json:"year"`
	Country     string   `json:"country"`
}

// ArtistResult is the response from GET /artists/{id}.
type ArtistResult struct {
	ID      int          `json:"id"`
	Name    string       `json:"name"`
	Profile string       `json:"profile"`
	Images  []ImageEntry `json:"images"`
	URLs    []string     `json:"urls"`
	Members []struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	} `json:"members"`
	Aliases []struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	} `json:"aliases"`
}

// MasterResult is the response from GET /masters/{id}.
type MasterResult struct {
	ID         int          `json:"id"`
	Title      string       `json:"title"`
	Year       int          `json:"year"`
	Genres     []string     `json:"genres"`
	Styles     []string     `json:"styles"`
	Images     []ImageEntry `json:"images"`
	Artists    []struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	} `json:"artists"`
}

// ReleaseResult is the response from GET /releases/{id}.
type ReleaseResult struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Year   int    `json:"year"`
	Labels []struct {
		Name   string `json:"name"`
		CatNo  string `json:"catno"`
	} `json:"labels"`
	Genres  []string     `json:"genres"`
	Styles  []string     `json:"styles"`
	Images  []ImageEntry `json:"images"`
	Country string       `json:"country"`
}

// ImageEntry is an image in a Discogs response.
type ImageEntry struct {
	Type   string `json:"type"` // "primary" or "secondary"
	URI    string `json:"uri"`
	URI150 string `json:"uri150"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// --- Search and fetch methods ---

// SearchArtists searches Discogs for artists matching name.
func (c *Client) SearchArtists(ctx context.Context, name string) (*SearchResponse, error) {
	path := fmt.Sprintf("/database/search?q=%s&type=artist", url.QueryEscape(name))
	body, err := c.get(ctx, path)
	if err != nil || body == nil {
		return nil, err
	}
	var resp SearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("discogs: parse artist search: %w", err)
	}
	return &resp, nil
}

// SearchMasters searches Discogs for master releases by title and artist.
func (c *Client) SearchMasters(ctx context.Context, title, artist string) (*SearchResponse, error) {
	path := fmt.Sprintf("/database/search?release_title=%s&artist=%s&type=master",
		url.QueryEscape(title), url.QueryEscape(artist))
	body, err := c.get(ctx, path)
	if err != nil || body == nil {
		return nil, err
	}
	var resp SearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("discogs: parse master search: %w", err)
	}
	return &resp, nil
}

// GetArtist fetches full artist details by Discogs artist ID.
func (c *Client) GetArtist(ctx context.Context, id int) (*ArtistResult, error) {
	body, err := c.get(ctx, fmt.Sprintf("/artists/%d", id))
	if err != nil || body == nil {
		return nil, err
	}
	var result ArtistResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("discogs: parse artist: %w", err)
	}
	return &result, nil
}

// GetMaster fetches full master release details by Discogs master ID.
func (c *Client) GetMaster(ctx context.Context, id int) (*MasterResult, error) {
	body, err := c.get(ctx, fmt.Sprintf("/masters/%d", id))
	if err != nil || body == nil {
		return nil, err
	}
	var result MasterResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("discogs: parse master: %w", err)
	}
	return &result, nil
}

// titleMatch returns true if a and b are close enough to be considered the same
// (case-insensitive, strips common punctuation differences).
func titleMatch(a, b string) bool {
	norm := func(s string) string {
		s = strings.ToLower(s)
		var out strings.Builder
		for _, r := range s {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
				out.WriteRune(r)
			}
		}
		return strings.TrimSpace(out.String())
	}
	return norm(a) == norm(b)
}
