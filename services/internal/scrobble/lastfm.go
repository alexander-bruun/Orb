// Package scrobble provides Last.fm and ListenBrainz scrobbling clients.
package scrobble

import (
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const lastfmAPIURL = "https://ws.audioscrobbler.com/2.0/"

// LastFMClient holds app credentials for the Last.fm API.
type LastFMClient struct {
	APIKey    string
	APISecret string
	http      *http.Client
}

// NewLastFMClient returns a client using the given app credentials.
func NewLastFMClient(apiKey, apiSecret string) *LastFMClient {
	return &LastFMClient{
		APIKey:    apiKey,
		APISecret: apiSecret,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// sign computes the MD5 API signature for the given params map.
// The params map must NOT include "format" or "callback".
func (c *LastFMClient) sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(c.APISecret)
	sum := md5.Sum([]byte(sb.String()))
	return fmt.Sprintf("%x", sum)
}

func (c *LastFMClient) post(params map[string]string) ([]byte, error) {
	params["api_key"] = c.APIKey
	params["api_sig"] = c.sign(params)
	params["format"] = "json"

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	resp, err := c.http.PostForm(lastfmAPIURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// lastfmError is a partial response shape used to detect API errors.
type lastfmError struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

func checkLastFMError(body []byte) error {
	var e lastfmError
	if err := json.Unmarshal(body, &e); err == nil && e.Error != 0 {
		return fmt.Errorf("last.fm error %d: %s", e.Error, e.Message)
	}
	return nil
}

// GetMobileSession authenticates a user with username + password and returns
// a session key. Call this once; store the session key per user.
func (c *LastFMClient) GetMobileSession(username, password string) (sessionKey string, err error) {
	body, err := c.post(map[string]string{
		"method":   "auth.getMobileSession",
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	if err := checkLastFMError(body); err != nil {
		return "", err
	}
	var resp struct {
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("last.fm: failed to parse session response: %w", err)
	}
	if resp.Session.Key == "" {
		return "", fmt.Errorf("last.fm: empty session key in response")
	}
	return resp.Session.Key, nil
}

// NowPlaying sends a now-playing notification for the given track.
func (c *LastFMClient) NowPlaying(sessionKey, artist, track, album string, durationSecs int) error {
	params := map[string]string{
		"method": "track.updateNowPlaying",
		"sk":     sessionKey,
		"artist": artist,
		"track":  track,
	}
	if album != "" {
		params["album"] = album
	}
	if durationSecs > 0 {
		params["duration"] = fmt.Sprintf("%d", durationSecs)
	}
	body, err := c.post(params)
	if err != nil {
		return err
	}
	return checkLastFMError(body)
}

// Scrobble submits a scrobble for the given track.
// startedAt is the UTC time playback began.
func (c *LastFMClient) Scrobble(sessionKey, artist, track, album string, startedAt time.Time, durationSecs int) error {
	params := map[string]string{
		"method":    "track.scrobble",
		"sk":        sessionKey,
		"artist[0]": artist,
		"track[0]":  track,
		"timestamp[0]": fmt.Sprintf("%d", startedAt.Unix()),
	}
	if album != "" {
		params["album[0]"] = album
	}
	if durationSecs > 0 {
		params["duration[0]"] = fmt.Sprintf("%d", durationSecs)
	}
	body, err := c.post(params)
	if err != nil {
		return err
	}
	return checkLastFMError(body)
}

// lastfmLoveResp is used to detect errors from love/unlove.
type lastfmLoveResp struct {
	XMLName xml.Name `xml:"lfm"`
	Status  string   `xml:"status,attr"`
	Error   struct {
		Code int    `xml:"code,attr"`
		Text string `xml:",chardata"`
	} `xml:"error"`
}

// LoveTrack loves a track on Last.fm.
func (c *LastFMClient) LoveTrack(sessionKey, artist, track string) error {
	body, err := c.post(map[string]string{
		"method": "track.love",
		"sk":     sessionKey,
		"artist": artist,
		"track":  track,
	})
	if err != nil {
		return err
	}
	return checkLastFMError(body)
}

// UnloveTrack unloves a track on Last.fm.
func (c *LastFMClient) UnloveTrack(sessionKey, artist, track string) error {
	body, err := c.post(map[string]string{
		"method": "track.unlove",
		"sk":     sessionKey,
		"artist": artist,
		"track":  track,
	})
	if err != nil {
		return err
	}
	return checkLastFMError(body)
}
