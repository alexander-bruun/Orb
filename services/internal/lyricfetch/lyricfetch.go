// Inspired by: https://github.com/jacquesh/foo_openlyrics, credits go to: Jacques Heunis

// Package lyricfetch searches external providers for synced (LRC) lyrics.
// Inspired by foo_openlyrics: tries LRCLIB first (free, no API key, supports
// duration-matched exact lookup), then falls back to NetEase Music.
package lyricfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result holds the lyrics text (LRC format) returned by a provider.
type Result struct {
	LRC      string // Synced lyrics in LRC format (empty if only plain available)
	Plain    string // Unsynced plain-text lyrics (fallback)
	Provider string // Which provider returned this result
}

// Search queries external providers for lyrics matching the given track.
// It tries providers in priority order and returns the first hit.
func Search(ctx context.Context, artist, album, title string, durationMs int) (*Result, error) {
	durationSec := durationMs / 1000

	// 1. LRCLIB exact get (uses duration for disambiguation — best match)
	if res, err := lrclibGet(ctx, artist, album, title, durationSec); err == nil && res != nil {
		return res, nil
	}

	// 2. LRCLIB search (without duration)
	if res, err := lrclibSearch(ctx, artist, album, title); err == nil && res != nil {
		return res, nil
	}

	// 3. NetEase Music
	if res, err := neteaseSearch(ctx, artist, title); err == nil && res != nil {
		return res, nil
	}

	return nil, fmt.Errorf("no lyrics found for %s - %s", artist, title)
}

// ---------------------------------------------------------------------------
// LRCLIB  (https://lrclib.net)
// ---------------------------------------------------------------------------

const lrclibBase = "https://lrclib.net/api"

func lrclibGet(ctx context.Context, artist, album, title string, durationSec int) (*Result, error) {
	u := fmt.Sprintf("%s/get?artist_name=%s&album_name=%s&track_name=%s&duration=%d",
		lrclibBase,
		url.QueryEscape(artist),
		url.QueryEscape(album),
		url.QueryEscape(title),
		durationSec,
	)
	body, err := httpGet(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	return parseLRCLibItem(body)
}

func lrclibSearch(ctx context.Context, artist, album, title string) (*Result, error) {
	u := fmt.Sprintf("%s/search?artist_name=%s&album_name=%s&track_name=%s",
		lrclibBase,
		url.QueryEscape(artist),
		url.QueryEscape(album),
		url.QueryEscape(title),
	)
	body, err := httpGet(ctx, u, nil)
	if err != nil {
		return nil, err
	}

	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil || len(items) == 0 {
		return nil, fmt.Errorf("no results")
	}

	// Take first result that has synced lyrics; fall back to first with plain.
	var plainFallback *Result
	for i, raw := range items {
		if i >= 3 {
			break // foo_openlyrics uses RESULT_LIMIT = 3
		}
		res, err := parseLRCLibItem(raw)
		if err != nil {
			continue
		}
		if res.LRC != "" {
			return res, nil
		}
		if plainFallback == nil && res.Plain != "" {
			plainFallback = res
		}
	}
	if plainFallback != nil {
		return plainFallback, nil
	}
	return nil, fmt.Errorf("no results")
}

func parseLRCLibItem(data []byte) (*Result, error) {
	var item struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	if item.SyncedLyrics == "" && item.PlainLyrics == "" {
		return nil, fmt.Errorf("empty lyrics")
	}
	return &Result{
		LRC:      item.SyncedLyrics,
		Plain:    item.PlainLyrics,
		Provider: "lrclib",
	}, nil
}

// ---------------------------------------------------------------------------
// NetEase Music  (https://music.163.com)
// ---------------------------------------------------------------------------

const neteaseBase = "https://music.163.com/api"

func neteaseSearch(ctx context.Context, artist, title string) (*Result, error) {
	// Step 1: search for song ID
	searchURL := neteaseBase + "/search/get"
	form := url.Values{
		"s":      {artist + " " + title},
		"type":   {"1"},
		"offset": {"0"},
		"limit":  {"5"},
	}
	headers := map[string]string{
		"Referer":      "https://music.163.com",
		"Cookie":       "appver=2.0.2",
		"Content-Type": "application/x-www-form-urlencoded",
		"X-Real-IP":    "202.96.0.0", // foo_openlyrics: spoof Chinese IP for better results
	}

	body, err := httpPost(ctx, searchURL, form.Encode(), headers)
	if err != nil {
		return nil, err
	}

	var searchResp struct {
		Result struct {
			Songs []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"songs"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil || len(searchResp.Result.Songs) == 0 {
		return nil, fmt.Errorf("no netease results")
	}

	// Step 2: fetch lyrics by song ID
	songID := searchResp.Result.Songs[0].ID
	lyricURL := fmt.Sprintf("%s/song/lyric?tv=-1&kv=-1&lv=-1&os=pc&id=%d", neteaseBase, songID)

	body, err = httpGet(ctx, lyricURL, headers)
	if err != nil {
		return nil, err
	}

	var lyricResp struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	if err := json.Unmarshal(body, &lyricResp); err != nil {
		return nil, err
	}
	lrc := strings.TrimSpace(lyricResp.LRC.Lyric)
	if lrc == "" {
		return nil, fmt.Errorf("empty netease lyrics")
	}
	return &Result{LRC: lrc, Provider: "netease"}, nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

var client = &http.Client{Timeout: 10 * time.Second}

func httpGet(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "orb/1.0 (https://github.com/alexander-bruun/orb)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func httpPost(ctx context.Context, rawURL, body string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "orb/1.0 (https://github.com/alexander-bruun/orb)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
