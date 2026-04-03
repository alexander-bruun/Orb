// Package spotify provides the Spotify OAuth callback handler.
// Credentials are read from env vars (SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET,
// SPOTIFY_FRONTEND_URL) or from the site_settings DB table, whichever is set.
// Env vars take priority over DB values.
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
	"github.com/alexander-bruun/orb/services/internal/store"
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
	kv *redis.Client
	db *store.Store
}

// New returns a Service. Routes are always mounted; the config endpoint
// reports whether credentials are actually configured.
func New(kv *redis.Client, db *store.Store) *Service {
	return &Service{kv: kv, db: db}
}

// spotifyCreds holds resolved credentials for a single request.
type spotifyCreds struct {
	clientID     string
	clientSecret string
	frontendBase string
	redirectURI  string // explicit override; empty → derive from request
}

// creds resolves credentials: env vars take priority, then DB.
func (s *Service) creds(ctx context.Context) spotifyCreds {
	id := os.Getenv("SPOTIFY_CLIENT_ID")
	secret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	frontend := os.Getenv("SPOTIFY_FRONTEND_URL")
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")

	if id == "" && s.db != nil {
		vals, _ := s.db.GetSiteSettings(ctx, []string{
			"spotify_client_id", "spotify_client_secret", "spotify_frontend_url", "spotify_redirect_uri",
		})
		id = vals["spotify_client_id"]
		secret = vals["spotify_client_secret"]
		frontend = vals["spotify_frontend_url"]
		if redirectURI == "" {
			redirectURI = vals["spotify_redirect_uri"]
		}
	}
	return spotifyCreds{
		clientID:     id,
		clientSecret: secret,
		frontendBase: strings.TrimRight(frontend, "/"),
		redirectURI:  redirectURI,
	}
}

// Routes mounts GET /auth/spotify, GET /auth/spotify/callback, GET /auth/spotify/config.
func (s *Service) Routes(r chi.Router) {
	r.Get("/spotify", s.begin)
	r.Get("/spotify/callback", s.callback)
	r.Get("/spotify/config", s.config)
}

// config returns whether Spotify OAuth is currently configured.
func (s *Service) config(w http.ResponseWriter, r *http.Request) {
	c := s.creds(r.Context())
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"enabled": c.clientID != ""})
}

// begin generates a state token, stores it in Redis, and redirects to Spotify.
func (s *Service) begin(w http.ResponseWriter, r *http.Request) {
	c := s.creds(r.Context())
	if c.clientID == "" {
		httputil.WriteErr(w, http.StatusServiceUnavailable, "Spotify not configured")
		return
	}

	state, err := randomHex(16)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "state generation failed")
		return
	}
	if err := s.kv.Set(r.Context(), stateKeyPfx+state, "1", stateTTL).Err(); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "state storage failed")
		return
	}

	cbURI := c.redirectURI
	if cbURI == "" {
		cbURI = callbackURI(r)
	}
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {c.clientID},
		"scope":         {spotifyScope},
		"redirect_uri":  {cbURI},
		"state":         {state},
	}
	http.Redirect(w, r, authURL+"?"+params.Encode(), http.StatusFound)
}

// callback exchanges the code for an access token and redirects the browser
// to the frontend with the token in the URL fragment so it stays off logs.
func (s *Service) callback(w http.ResponseWriter, r *http.Request) {
	c := s.creds(r.Context())
	q := r.URL.Query()

	if errMsg := q.Get("error"); errMsg != "" {
		redirectFrontend(w, r, c.frontendBase, "", "Spotify: "+errMsg)
		return
	}

	state := q.Get("state")
	code := q.Get("code")

	ctx := r.Context()
	key := stateKeyPfx + state
	if err := s.kv.GetDel(ctx, key).Err(); err != nil {
		redirectFrontend(w, r, c.frontendBase, "", "invalid or expired state")
		return
	}

	cbURI := c.redirectURI
	if cbURI == "" {
		cbURI = callbackURI(r)
	}
	token, err := exchangeCode(ctx, c.clientID, c.clientSecret, code, cbURI)
	if err != nil {
		redirectFrontend(w, r, c.frontendBase, "", err.Error())
		return
	}

	redirectFrontend(w, r, c.frontendBase, token, "")
}

// exchangeCode performs the server-side token exchange.
func exchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (string, error) {
	body := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
func callbackURI(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/auth/spotify/callback", scheme, r.Host)
}

// redirectFrontend sends the browser back to the SPA.
// On success: /playlists#spotify_token=TOKEN
// On error:   /playlists?spotify_error=MSG
func redirectFrontend(w http.ResponseWriter, r *http.Request, frontendBase, token, errMsg string) {
	base := frontendBase
	if base == "" {
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
