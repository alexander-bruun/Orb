// Package collaboration handles playlist collaboration routes.
package collaboration

import (
	"encoding/json"
	"net/http"

	"github.com/alexander-bruun/orb/services/internal/activity"
	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
)

// Service handles playlist collaboration HTTP routes.
type Service struct {
	db      *store.Store
	emitter *activity.Emitter
}

// New returns a new collaboration Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// SetEmitter attaches an activity emitter.
func (s *Service) SetEmitter(e *activity.Emitter) { s.emitter = e }

// Routes registers collaboration endpoints.
// Mount under /playlists/{id}/collaborators and /playlists/invite/{token}.
func (s *Service) Routes(r chi.Router) {
	// Per-playlist collaboration management
	r.Route("/playlists/{id}/collaborators", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/invite", s.createInvite)
		r.Delete("/{user_id}", s.remove)
		r.Patch("/{user_id}", s.updateRole)
	})

	// Invite token redemption
	r.Get("/playlists/invite/{token}", s.redeemInvite)
}

// list returns all collaborators for a playlist.
func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	playlistID := chi.URLParam(r, "id")

	pl, err := s.db.GetPlaylistByID(r.Context(), playlistID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "playlist not found")
		return
	}

	if !s.canManage(r, pl, userID) {
		httputil.WriteErr(w, http.StatusForbidden, "forbidden")
		return
	}

	collabs, err := s.db.ListCollaborators(r.Context(), playlistID)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteOK(w, collabs)
}

type inviteReq struct {
	Role string `json:"role"` // "editor" | "viewer"
}

type inviteResp struct {
	Token string `json:"token"`
}

// createInvite generates a new invite token for the playlist.
func (s *Service) createInvite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	playlistID := chi.URLParam(r, "id")

	pl, err := s.db.GetPlaylistByID(r.Context(), playlistID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	if pl.UserID != userID {
		httputil.WriteErr(w, http.StatusForbidden, "only the owner can invite collaborators")
		return
	}

	var req inviteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Role = "editor"
	}
	if req.Role != "editor" && req.Role != "viewer" {
		req.Role = "editor"
	}

	token, err := s.db.CreatePlaylistInviteToken(r.Context(), playlistID, userID, req.Role)
	if err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, inviteResp{Token: token})
}

// remove removes a collaborator from a playlist.
func (s *Service) remove(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	playlistID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "user_id")

	pl, err := s.db.GetPlaylistByID(r.Context(), playlistID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	// Only owner can remove others; collaborators can remove themselves.
	if pl.UserID != userID && userID != targetUserID {
		httputil.WriteErr(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := s.db.RemoveCollaborator(r.Context(), playlistID, targetUserID); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateRoleReq struct {
	Role string `json:"role"`
}

// updateRole changes a collaborator's role.
func (s *Service) updateRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	playlistID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "user_id")

	pl, err := s.db.GetPlaylistByID(r.Context(), playlistID)
	if err != nil {
		httputil.WriteErr(w, http.StatusNotFound, "playlist not found")
		return
	}
	if pl.UserID != userID {
		httputil.WriteErr(w, http.StatusForbidden, "only the owner can change roles")
		return
	}

	var req updateRoleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Role != "editor" && req.Role != "viewer") {
		httputil.WriteErr(w, http.StatusBadRequest, "role must be 'editor' or 'viewer'")
		return
	}

	if err := s.db.UpdateCollaboratorRole(r.Context(), playlistID, targetUserID, req.Role); err != nil {
		httputil.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type redeemResp struct {
	PlaylistID string `json:"playlist_id"`
	Role       string `json:"role"`
}

// redeemInvite accepts a playlist invite token for the authenticated user.
func (s *Service) redeemInvite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	token := chi.URLParam(r, "token")

	inv, err := s.db.RedeemPlaylistInviteToken(r.Context(), token, userID)
	if err != nil {
		switch err {
		case store.ErrInviteAlreadyUsed:
			httputil.WriteErr(w, http.StatusConflict, "invite token already used")
		case store.ErrInviteExpired:
			httputil.WriteErr(w, http.StatusGone, "invite token expired")
		default:
			httputil.WriteErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if s.emitter != nil {
		meta := map[string]any{"playlist_id": inv.PlaylistID}
		if pl, err := s.db.GetPlaylistByID(r.Context(), inv.PlaylistID); err == nil {
			meta["playlist_name"] = pl.Name
		}
		s.emitter.Record(r.Context(), userID, "playlist_follow", "playlist", inv.PlaylistID, meta)
	}
	httputil.WriteOK(w, redeemResp{PlaylistID: inv.PlaylistID, Role: inv.Role})
}

// canManage returns true if userID is the owner or an accepted collaborator.
func (s *Service) canManage(r *http.Request, pl store.Playlist, userID string) bool {
	if pl.UserID == userID {
		return true
	}
	ok, _, _ := s.db.IsCollaborator(r.Context(), pl.ID, userID)
	return ok
}
