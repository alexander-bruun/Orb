// Package openlibrary provides a client for the Open Library API.
// It is used to enrich audiobook metadata (description, ISBN, cover art).
// The API is completely free and requires no authentication key.
// Docs: https://openlibrary.org/developers/api
package openlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	baseURL      = "https://openlibrary.org"
	coversURL    = "https://covers.openlibrary.org"
	searchPath   = "/search.json"
	userAgent    = "Orb/1.0 (self-hosted audiobook server; contact via github.com/alexander-bruun/orb)"
	requestDelay = 200 * time.Millisecond // gentle rate-limiting
)

// Client is an Open Library API client.
type Client struct {
	http *http.Client
}

// New creates a new Open Library client.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// BookResult holds enrichment data returned for a single audiobook.
type BookResult struct {
	OLKey         string // e.g. "/works/OL82563W"
	ISBN          string
	Description   string
	PublishedYear int
	CoverID       int64 // Open Library cover ID; 0 = no cover
	Subjects      []string
	Series        []string
}

// searchDoc is the per-book document from the /search.json response.
type searchDoc struct {
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	AuthorName    []string `json:"author_name"`
	FirstPublishY int      `json:"first_publish_year"`
	ISBN          []string `json:"isbn"`
	CoverI        int64    `json:"cover_i"`
	Subject       []string `json:"subject"`
}

type searchResponse struct {
	NumFound int         `json:"numFound"`
	Docs     []searchDoc `json:"docs"`
}

// Search queries Open Library for a book by title and optional author.
// Returns nil if no suitable match is found.
func (c *Client) Search(ctx context.Context, title, author string) (*BookResult, error) {
	params := url.Values{}
	params.Set("title", title)
	if author != "" {
		params.Set("author", author)
	}
	params.Set("fields", "key,title,author_name,first_publish_year,isbn,cover_i,subject")
	params.Set("limit", "3")

	reqURL := baseURL + searchPath + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library search: HTTP %d", resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("open library decode: %w", err)
	}
	if len(sr.Docs) == 0 {
		return nil, nil
	}

	doc := sr.Docs[0]
	if !strings.EqualFold(stripParens(doc.Title), stripParens(title)) && !looseTitleMatch(doc.Title, title) {
		slog.Debug("open library: title mismatch, skipping", "got", doc.Title, "want", title)
		return nil, nil
	}

	result := &BookResult{
		OLKey:         doc.Key,
		PublishedYear: doc.FirstPublishY,
		CoverID:       doc.CoverI,
		Subjects:      doc.Subject,
	}
	if len(doc.ISBN) > 0 {
		result.ISBN = doc.ISBN[0]
	}

	// Fetch work details to get description and series.
	time.Sleep(requestDelay)
	desc, series, err := c.fetchWorkDetails(ctx, doc.Key)
	if err != nil {
		slog.Debug("open library: work detail fetch failed", "key", doc.Key, "err", err)
	} else {
		result.Description = desc
		result.Series = series
	}

	return result, nil
}

// FetchCoverArt downloads the cover image for a given Open Library cover ID.
// Returns nil when the ID is 0 or the request fails (soft error).
func (c *Client) FetchCoverArt(ctx context.Context, coverID int64) ([]byte, error) {
	if coverID <= 0 {
		return nil, nil
	}
	imgURL := fmt.Sprintf("%s/b/id/%d-L.jpg", coversURL, coverID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library cover: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// workDetail holds the description/series from a /works/{key}.json response.
type workDetail struct {
	Description interface{} `json:"description"` // string OR {value: string}
	Series      interface{} `json:"series"`      // string OR []string
}

func (c *Client) fetchWorkDetails(ctx context.Context, workKey string) (string, []string, error) {
	if workKey == "" {
		return "", nil, nil
	}
	reqURL := baseURL + workKey + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var wd workDetail
	if err := json.NewDecoder(resp.Body).Decode(&wd); err != nil {
		return "", nil, err
	}

	series := parseSeriesField(wd.Series)
	switch v := wd.Description.(type) {
	case string:
		return v, series, nil
	case map[string]interface{}:
		if s, ok := v["value"].(string); ok {
			return s, series, nil
		}
	}
	return "", series, nil
}

func parseSeriesField(v interface{}) []string {
	switch s := v.(type) {
	case string:
		if strings.TrimSpace(s) != "" {
			return []string{strings.TrimSpace(s)}
		}
	case []interface{}:
		var out []string
		for _, item := range s {
			if str, ok := item.(string); ok {
				str = strings.TrimSpace(str)
				if str != "" {
					out = append(out, str)
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func stripParens(s string) string {
	if idx := strings.Index(s, "("); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

func looseTitleMatch(a, b string) bool {
	normalize := func(s string) string {
		s = strings.ToLower(stripParens(s))
		s = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
				return r
			}
			return -1
		}, s)
		return strings.TrimSpace(s)
	}
	na, nb := normalize(a), normalize(b)
	return strings.Contains(na, nb) || strings.Contains(nb, na)
}
