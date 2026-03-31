// Package user handles user account preferences.
package user

import (
	"encoding/json"
	"net/http"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Service handles user preference routes.
type Service struct {
	db *store.Store
	kv *redis.Client
}

// New returns a new user Service.
func New(db *store.Store, kv *redis.Client) *Service {
	return &Service{db: db, kv: kv}
}

// Routes registers user endpoints on the given router (requires JWT middleware).
func (s *Service) Routes(r chi.Router) {
	r.Get("/streaming-prefs", s.getStreamingPrefs)
	r.Put("/streaming-prefs", s.putStreamingPrefs)

	// EQ profiles
	r.Get("/eq-profiles", s.listEQProfiles)
	r.Post("/eq-profiles", s.createEQProfile)
	r.Get("/eq-profiles/{id}", s.getEQProfile)
	r.Put("/eq-profiles/{id}", s.updateEQProfile)
	r.Delete("/eq-profiles/{id}", s.deleteEQProfile)
	r.Post("/eq-profiles/{id}/default", s.setDefaultEQProfile)
	r.Delete("/eq-profiles/{id}/default", s.clearDefaultEQProfile)

	// Genre → EQ mappings
	r.Get("/genre-eq", s.listGenreEQ)
	r.Put("/genre-eq/{genre_id}", s.setGenreEQ)
	r.Delete("/genre-eq/{genre_id}", s.deleteGenreEQ)

	// Subsonic API password (plaintext, used for token-based auth)
	r.Get("/subsonic-password", s.getSubsonicPassword)
	r.Put("/subsonic-password", s.putSubsonicPassword)
	r.Delete("/subsonic-password", s.deleteSubsonicPassword)
}

// getStreamingPrefs returns the authenticated user's streaming preferences.
func (s *Service) getStreamingPrefs(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	prefs, err := s.db.GetUserStreamingPrefs(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, prefs)
}

// updateStreamingPrefsReq is the body for PUT /user/streaming-prefs.
// All fields are optional; sending null clears the limit for that network tier.
// The top-level fields are the "any network" defaults; wifi_* and mobile_* fields
// override them when the client reports it is on that specific network type.
type updateStreamingPrefsReq struct {
	// Any-network defaults
	MaxBitrateKbps  *int    `json:"max_bitrate_kbps"`  // kbps, null = unlimited
	MaxSampleRate   *int    `json:"max_sample_rate"`   // Hz, null = unlimited (advisory)
	MaxBitDepth     *int    `json:"max_bit_depth"`     // e.g. 16/24, null = unlimited (advisory)
	TranscodeFormat *string `json:"transcode_format"`  // null = no transcoding; "mp3"/"aac"/"opus"
	// Wi-Fi specific overrides (nil = inherit default)
	WifiMaxBitrateKbps  *int    `json:"wifi_max_bitrate_kbps"`
	WifiMaxSampleRate   *int    `json:"wifi_max_sample_rate"`
	WifiMaxBitDepth     *int    `json:"wifi_max_bit_depth"`
	WifiTranscodeFormat *string `json:"wifi_transcode_format"`
	// Mobile/cellular specific overrides (nil = inherit default)
	MobileMaxBitrateKbps  *int    `json:"mobile_max_bitrate_kbps"`
	MobileMaxSampleRate   *int    `json:"mobile_max_sample_rate"`
	MobileMaxBitDepth     *int    `json:"mobile_max_bit_depth"`
	MobileTranscodeFormat *string `json:"mobile_transcode_format"`
}

// putStreamingPrefs upserts streaming quality preferences for the authenticated user.
func (s *Service) putStreamingPrefs(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req updateStreamingPrefsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate that all non-nil int values are positive.
	type check struct {
		v    *int
		name string
	}
	checks := []check{
		{req.MaxBitrateKbps, "max_bitrate_kbps"},
		{req.MaxSampleRate, "max_sample_rate"},
		{req.MaxBitDepth, "max_bit_depth"},
		{req.WifiMaxBitrateKbps, "wifi_max_bitrate_kbps"},
		{req.WifiMaxSampleRate, "wifi_max_sample_rate"},
		{req.WifiMaxBitDepth, "wifi_max_bit_depth"},
		{req.MobileMaxBitrateKbps, "mobile_max_bitrate_kbps"},
		{req.MobileMaxSampleRate, "mobile_max_sample_rate"},
		{req.MobileMaxBitDepth, "mobile_max_bit_depth"},
	}
	for _, c := range checks {
		if c.v != nil && *c.v <= 0 {
			httputil.WriteErr(w, http.StatusBadRequest, c.name+" must be positive or null")
			return
		}
	}

	// Validate transcode format values.
	validTranscodeFormats := map[string]bool{"mp3": true, "aac": true, "opus": true}
	for _, f := range []*string{req.TranscodeFormat, req.WifiTranscodeFormat, req.MobileTranscodeFormat} {
		if f != nil && !validTranscodeFormats[*f] {
			httputil.WriteErr(w, http.StatusBadRequest, "transcode_format must be one of: mp3, aac, opus (or null)")
			return
		}
	}

	prefs, err := s.db.UpsertUserStreamingPrefs(r.Context(), store.UpsertUserStreamingPrefsParams{
		UserID:                userID,
		MaxBitrateKbps:        req.MaxBitrateKbps,
		MaxSampleRate:         req.MaxSampleRate,
		MaxBitDepth:           req.MaxBitDepth,
		WifiMaxBitrateKbps:    req.WifiMaxBitrateKbps,
		WifiMaxSampleRate:     req.WifiMaxSampleRate,
		WifiMaxBitDepth:       req.WifiMaxBitDepth,
		MobileMaxBitrateKbps:  req.MobileMaxBitrateKbps,
		MobileMaxSampleRate:   req.MobileMaxSampleRate,
		MobileMaxBitDepth:     req.MobileMaxBitDepth,
		TranscodeFormat:       req.TranscodeFormat,
		WifiTranscodeFormat:   req.WifiTranscodeFormat,
		MobileTranscodeFormat: req.MobileTranscodeFormat,
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}

	// Invalidate the KV cache entry so the stream handler picks up the new prefs.
	s.kv.Del(r.Context(), kvkeys.UserStreamingPrefs(userID))

	httputil.WriteJSON(w, http.StatusOK, prefs)
}

// ──────────────────────────────────────────────────────────────
// EQ Profile handlers
// ──────────────────────────────────────────────────────────────

func (s *Service) listEQProfiles(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	profiles, err := s.db.ListEQProfiles(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, profiles)
}

func (s *Service) getEQProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := chi.URLParam(r, "id")
	p, err := s.db.GetEQProfile(r.Context(), id, userID)
	if err != nil {
		if err.Error() == "eq profile not found" {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

type eqProfileReq struct {
	Name      string         `json:"name"`
	Bands     []store.EQBand `json:"bands"`
	IsDefault bool           `json:"is_default"`
}

func (s *Service) createEQProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req eqProfileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.db.CreateEQProfile(r.Context(), store.CreateEQProfileParams{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      req.Name,
		Bands:     req.Bands,
		IsDefault: req.IsDefault,
	})
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, p)
}

func (s *Service) updateEQProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := chi.URLParam(r, "id")
	var req eqProfileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.db.UpdateEQProfile(r.Context(), store.UpdateEQProfileParams{
		ID:     id,
		UserID: userID,
		Name:   req.Name,
		Bands:  req.Bands,
	})
	if err != nil {
		if err.Error() == "eq profile not found" {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (s *Service) deleteEQProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteEQProfile(r.Context(), id, userID); err != nil {
		if err.Error() == "eq profile not found" {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) setDefaultEQProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.db.SetDefaultEQProfile(r.Context(), id, userID); err != nil {
		if err.Error() == "eq profile not found" {
			httputil.WriteErr(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) clearDefaultEQProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := s.db.ClearDefaultEQProfile(r.Context(), userID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ──────────────────────────────────────────────────────────────
// Genre → EQ mapping handlers
// ──────────────────────────────────────────────────────────────

func (s *Service) listGenreEQ(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	mappings, err := s.db.ListGenreEQMappings(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, mappings)
}

func (s *Service) setGenreEQ(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	genreID := chi.URLParam(r, "genre_id")
	var req struct {
		ProfileID string `json:"profile_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ProfileID == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "profile_id is required")
		return
	}
	if err := s.db.SetGenreEQMapping(r.Context(), userID, genreID, req.ProfileID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) deleteGenreEQ(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	genreID := chi.URLParam(r, "genre_id")
	if err := s.db.DeleteGenreEQMapping(r.Context(), userID, genreID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Subsonic password ──────────────────────────────────────────────────────────

// getSubsonicPassword returns whether a Subsonic password is set (never returns the value).
func (s *Service) getSubsonicPassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	pw, err := s.db.GetSubsonicPassword(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"set": pw != ""})
}

// putSubsonicPassword sets the Subsonic password for the authenticated user.
func (s *Service) putSubsonicPassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "password required")
		return
	}
	if err := s.db.SetSubsonicPassword(r.Context(), userID, body.Password); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteSubsonicPassword clears the Subsonic password for the authenticated user.
func (s *Service) deleteSubsonicPassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	if userID == "" {
		httputil.WriteErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := s.db.SetSubsonicPassword(r.Context(), userID, ""); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
