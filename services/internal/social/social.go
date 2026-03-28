// Package social provides activity feeds, user follows, public profiles, and avatar upload.
package social

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Service handles social HTTP routes.
type Service struct {
	db  *store.Store
	obj objstore.ObjectStore
}

// New returns a new social Service.
func New(db *store.Store, obj objstore.ObjectStore) *Service {
	return &Service{db: db, obj: obj}
}

// Routes registers social endpoints.
func (s *Service) Routes(r chi.Router) {
	// Activity feed + follow endpoints
	r.Get("/social/feed", s.feed)
	r.Get("/social/activity/{username}", s.userActivity)
	r.Post("/social/follow/{username}", s.follow)
	r.Delete("/social/follow/{username}", s.unfollow)
	r.Get("/social/followers", s.myFollowers)
	r.Get("/social/following", s.myFollowing)
	r.Get("/social/has-activity", s.hasActivity)

	// Public profiles (no auth required, but optionally enrich with is_following)
	r.Get("/profile/{username}", s.publicProfile)
	r.Get("/profile/{username}/playlists", s.publicPlaylists)
	r.Get("/profile/{username}/activity", s.publicActivity)
	r.Get("/profile/{username}/stats", s.publicStats)

	// Profile self-edit + avatar upload
	r.Patch("/user/profile", s.updateProfile)
	r.Post("/user/avatar", s.uploadAvatar)
	r.Get("/user/profile", s.getProfile)
}

// ── Activity feed ─────────────────────────────────────────────────────────────

func (s *Service) feed(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit, offset := httputil.Pagination(r, 20, 100)
	rows, err := s.db.GetFeedForUser(r.Context(), userID, limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []store.ActivityRow{}
	}
	httputil.WriteOK(w, rows)
}

// resolveProfile fetches a profile, skipping the public filter when the
// caller is the profile owner (so you can always view your own page).
func (s *Service) resolveProfile(r *http.Request, username string) (*store.PublicProfile, error) {
	callerID := auth.UserIDFromCtx(r.Context())
	// Always fetch without filter first to get the user ID.
	profile, err := s.db.GetPublicProfile(r.Context(), username, false)
	if err != nil {
		return nil, err
	}
	// Enforce public filter for anyone who is not the owner.
	if !profile.ProfilePublic && profile.ID != callerID {
		return nil, store.ErrProfileNotPublic
	}
	return profile, nil
}

func (s *Service) userActivity(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	profile, err := s.resolveProfile(r, username)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "user not found")
		return
	}
	limit, offset := httputil.Pagination(r, 20, 100)
	rows, err := s.db.GetActivityForUser(r.Context(), profile.ID, limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []store.ActivityRow{}
	}
	httputil.WriteOK(w, rows)
}

func (s *Service) hasActivity(w http.ResponseWriter, r *http.Request) {
	ok, err := s.db.HasAnyActivity(r.Context())
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, map[string]bool{"has_activity": ok})
}

// ── Follow / unfollow ─────────────────────────────────────────────────────────

func (s *Service) follow(w http.ResponseWriter, r *http.Request) {
	callerID := auth.UserIDFromCtx(r.Context())
	username := chi.URLParam(r, "username")

	target, err := s.db.GetPublicProfile(r.Context(), username, true)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "user not found")
		return
	}
	if target.ID == callerID {
		httputil.WriteErr(w, http.StatusBadRequest, "cannot follow yourself")
		return
	}
	if err := s.db.FollowUser(r.Context(), callerID, target.ID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) unfollow(w http.ResponseWriter, r *http.Request) {
	callerID := auth.UserIDFromCtx(r.Context())
	username := chi.URLParam(r, "username")

	target, err := s.db.GetPublicProfile(r.Context(), username, true)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "user not found")
		return
	}
	if err := s.db.UnfollowUser(r.Context(), callerID, target.ID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) myFollowers(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	users, err := s.db.ListFollowers(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, users)
}

func (s *Service) myFollowing(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	users, err := s.db.ListFollowing(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, users)
}

// ── Public profiles ───────────────────────────────────────────────────────────

func (s *Service) publicProfile(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	profile, err := s.resolveProfile(r, username)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "profile not found or not public")
		return
	}

	callerID := auth.UserIDFromCtx(r.Context())
	resp := map[string]any{
		"profile": profile,
	}
	if callerID != "" && callerID != profile.ID {
		following, _ := s.db.IsFollowing(r.Context(), callerID, profile.ID)
		resp["is_following"] = following
	}
	httputil.WriteOK(w, resp)
}

func (s *Service) publicPlaylists(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	profile, err := s.resolveProfile(r, username)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "profile not found or not public")
		return
	}
	pls, err := s.db.GetUserPublicPlaylists(r.Context(), profile.ID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pls == nil {
		pls = []store.Playlist{}
	}
	httputil.WriteOK(w, pls)
}

func (s *Service) publicActivity(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	profile, err := s.resolveProfile(r, username)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "profile not found or not public")
		return
	}
	limit, offset := httputil.Pagination(r, 20, 100)
	rows, err := s.db.GetActivityForUser(r.Context(), profile.ID, limit, offset)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []store.ActivityRow{}
	}
	httputil.WriteOK(w, rows)
}

func (s *Service) publicStats(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	profile, err := s.resolveProfile(r, username)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "profile not found or not public")
		return
	}
	stats, err := s.db.GetUserPublicStats(r.Context(), profile.ID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, stats)
}

// ── Profile self-edit ─────────────────────────────────────────────────────────

type updateProfileReq struct {
	DisplayName   string `json:"display_name"`
	Bio           string `json:"bio"`
	ProfilePublic bool   `json:"profile_public"`
}

func (s *Service) getProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	displayName, bio, profilePublic, avatarKey, err := s.db.GetUserProfileFields(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, map[string]any{
		"display_name":   displayName,
		"bio":            bio,
		"profile_public": profilePublic,
		"avatar_key":     avatarKey,
	})
}

func (s *Service) updateProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	var req updateProfileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := s.db.UpdateUserProfile(r.Context(), userID, req.DisplayName, req.Bio, req.ProfilePublic); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	displayName, bio, profilePublic, avatarKey, err := s.db.GetUserProfileFields(r.Context(), userID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, map[string]any{
		"display_name":   displayName,
		"bio":            bio,
		"profile_public": profilePublic,
		"avatar_key":     avatarKey,
	})
}

// ── Avatar upload ─────────────────────────────────────────────────────────────

const maxAvatarSize = 2 << 20 // 2 MB

func (s *Service) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())

	if err := r.ParseMultipartForm(maxAvatarSize); err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		httputil.WriteErr(w, http.StatusBadRequest, "missing avatar file")
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > maxAvatarSize {
		httputil.WriteErr(w, http.StatusRequestEntityTooLarge, "avatar must be ≤2 MB")
		return
	}

	ct := header.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		httputil.WriteErr(w, http.StatusUnsupportedMediaType, "avatar must be an image")
		return
	}

	ext := "jpg"
	if strings.Contains(ct, "png") {
		ext = "png"
	} else if strings.Contains(ct, "webp") {
		ext = "webp"
	}

	key := fmt.Sprintf("avatars/%s.%s", uuid.New().String(), ext)
	if err := s.obj.Put(r.Context(), key, io.LimitReader(file, maxAvatarSize), header.Size); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, "failed to store avatar")
		return
	}

	if err := s.db.SetUserAvatar(r.Context(), userID, key); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"avatar_key": key})
}
