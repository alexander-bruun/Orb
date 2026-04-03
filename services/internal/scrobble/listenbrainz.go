package scrobble

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const lbAPIURL = "https://api.listenbrainz.org/1/submit-listens"

// LBClient is a minimal ListenBrainz API client.
type LBClient struct {
	http *http.Client
}

// NewLBClient returns a new ListenBrainz client.
func NewLBClient() *LBClient {
	return &LBClient{http: &http.Client{Timeout: 10 * time.Second}}
}

type lbTrackMetadata struct {
	ArtistName  string `json:"artist_name"`
	TrackName   string `json:"track_name"`
	ReleaseName string `json:"release_name,omitempty"`
}

type lbListen struct {
	ListenedAt    int64           `json:"listened_at,omitempty"`
	TrackMetadata lbTrackMetadata `json:"track_metadata"`
}

type lbPayload struct {
	ListenType string     `json:"listen_type"` // "playing_now" | "single"
	Payload    []lbListen `json:"payload"`
}

func (c *LBClient) submit(token string, payload lbPayload) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, lbAPIURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("listenbrainz: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// NowPlaying sends a playing_now notification to ListenBrainz.
func (c *LBClient) NowPlaying(token, artist, track, album string) error {
	return c.submit(token, lbPayload{
		ListenType: "playing_now",
		Payload: []lbListen{
			{TrackMetadata: lbTrackMetadata{ArtistName: artist, TrackName: track, ReleaseName: album}},
		},
	})
}

// Scrobble submits a single listen to ListenBrainz.
func (c *LBClient) Scrobble(token, artist, track, album string, startedAt time.Time) error {
	return c.submit(token, lbPayload{
		ListenType: "single",
		Payload: []lbListen{
			{
				ListenedAt:    startedAt.Unix(),
				TrackMetadata: lbTrackMetadata{ArtistName: artist, TrackName: track, ReleaseName: album},
			},
		},
	})
}
