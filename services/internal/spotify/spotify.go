// Package spotify provides the Spotify OAuth callback handler.
// The server admin sets SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET once;
// users just click "Connect Spotify" and log in through Spotify's own UI.
package spotify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const (
	stateTTL     = 10 * time.Minute
	stateKeyPfx  = "spotify_state:"
	spotifyScope = "playlist-read-private playlist-read-collaborative"
	authURL      = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
)

// Service handles the Spotify OAuth routes.
type Service struct {
	kv           *redis.Client
	clientID     string
	clientSecret string
	// frontendBase is where the browser lands after the callback
	// (e.g. "http://localhost:5173"). Falls back to same-origin if empty.
	frontendBase string
}

// New returns a Service. Returns nil when SPOTIFY_CLIENT_ID is not set
// so callers can skip mounting the routes entirely.
func New(kv *redis.Client) *Service {
	id := os.Getenv("SPOTIFY_CLIENT_ID")
	if id == "" {
		return nil
	}
	return &Service{
		kv:           kv,
		clientID:     id,
		clientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		frontendBase: strings.TrimRight(os.Getenv("SPOTIFY_FRONTEND_URL"), "/"),
	}
}

// Enabled reports whether Spotify OAuth is configured.
func (s *Service) Enabled() bool { return s != nil && s.clientID != "" }

// Routes mounts GET /auth/spotify and GET /auth/spotify/callback.
func (s *Service) Routes(r chi.Router) {
	r.Get("/spotify", s.begin)
	r.Get("/spotify/callback", s.callback)
	// Simple check endpoint so the frontend can show/hide the button.
	r.Get("/spotify/config", s.config)
}

// config returns whether Spotify is configured.
func (s *Service) config(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"enabled": s.Enabled()})
}

// begin generates a state token, stores it in Redis, and redirects to Spotify.
func (s *Service) begin(w http.ResponseWriter, r *http.Request) {
	state, err := randomHex(16)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "state generation failed")
		return
	}
	if err := s.kv.Set(r.Context(), stateKeyPfx+state, "1", stateTTL).Err(); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "state storage failed")
		return
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {s.clientID},
		"scope":         {spotifyScope},
		"redirect_uri":  {s.callbackURI(r)},
		"state":         {state},
	}
	http.Redirect(w, r, authURL+"?"+params.Encode(), http.StatusFound)
}

// callback exchanges the code for an access token and redirects the browser
// to the frontend with the token in the URL fragment so it stays off logs.
func (s *Service) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if errMsg := q.Get("error"); errMsg != "" {
		s.redirectFrontend(w, r, "", "Spotify: "+errMsg)
		return
	}

	state := q.Get("state")
	code := q.Get("code")

	// Verify state
	ctx := r.Context()
	key := stateKeyPfx + state
	if err := s.kv.GetDel(ctx, key).Err(); err != nil {
		s.redirectFrontend(w, r, "", "invalid or expired state")
		return
	}

	// Exchange code for token
	token, err := s.exchangeCode(ctx, code, s.callbackURI(r))
	if err != nil {
		s.redirectFrontend(w, r, "", err.Error())
		return
	}

	s.redirectFrontend(w, r, token, "")
}

// exchangeCode performs the server-side token exchange.
func (s *Service) exchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	body := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if data.Error != "" {
		return "", fmt.Errorf("%s: %s", data.Error, data.ErrorDesc)
	}
	return data.AccessToken, nil
}

// callbackURI builds the absolute redirect URI for this request.
func (s *Service) callbackURI(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	host := r.Host
	return fmt.Sprintf("%s://%s/auth/spotify/callback", scheme, host)
}

// redirectFrontend sends the browser back to the SPA.
// On success: /playlists#spotify_token=TOKEN
// On error:   /playlists?spotify_error=MSG
func (s *Service) redirectFrontend(w http.ResponseWriter, r *http.Request, token, errMsg string) {
	base := s.frontendBase
	if base == "" {
		// Same origin as the API (single-origin deployment).
		scheme := "https"
		if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			scheme = "http"
		}
		base = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	var dest string
	if token != "" {
		dest = base + "/playlists#spotify_token=" + url.QueryEscape(token)
	} else {
		dest = base + "/playlists?spotify_error=" + url.QueryEscape(errMsg)
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
