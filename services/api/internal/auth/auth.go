// Package auth handles user registration, login, JWT issuance, and session management.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/pkg/kvkeys"
	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtTTL      = 15 * time.Minute
	refreshTTL  = 7 * 24 * time.Hour
	loginLimit  = 10 // max attempts per IP per window
	loginWindow = time.Minute
)

// Service handles auth HTTP routes.
type Service struct {
	db        *store.Store
	kv        *redis.Client
	jwtSecret []byte
}

// New returns a new auth Service.
func New(db *store.Store, kv *redis.Client, jwtSecret string) *Service {
	return &Service{db: db, kv: kv, jwtSecret: []byte(jwtSecret)}
}

// Routes registers auth endpoints on the given router.
func (s *Service) Routes(r chi.Router) {
	r.Get("/setup", s.setup)
	r.Post("/register", s.register)
	r.Post("/login", s.login)
	r.Post("/refresh", s.refresh)
	r.Post("/logout", s.logout)

	// Account-management endpoints require a valid JWT.
	r.Group(func(r chi.Router) {
		r.Use(JWTMiddleware(string(s.jwtSecret), s.kv))
		r.Patch("/password", s.changePassword)
		r.Patch("/email", s.changeEmail)
	})
}

// --- handlers ---

func (s *Service) setup(w http.ResponseWriter, r *http.Request) {
	has, err := s.db.HasAnyUser(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"setup_required": !has})
}

type registerReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) register(w http.ResponseWriter, r *http.Request) {
	has, err := s.db.HasAnyUser(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if has {
		writeErr(w, http.StatusForbidden, "registration is closed")
		return
	}

	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "username, email, and password required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	user, err := s.db.CreateUser(r.Context(), store.CreateUserParams{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
	})
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeErr(w, http.StatusConflict, "username or email already exists")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": user.ID, "username": user.Username})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	// Rate limit.
	attempts, _ := s.kv.Incr(r.Context(), kvkeys.LoginAttempts(ip)).Result()
	if attempts == 1 {
		s.kv.Expire(r.Context(), kvkeys.LoginAttempts(ip), loginWindow)
	}
	if attempts > loginLimit {
		writeErr(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}

	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	user, err := s.db.GetUserByEmail(r.Context(), req.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, err := s.issueJWT(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt error")
		return
	}
	refreshToken := uuid.New().String()

	// Store session and refresh token in KeyVal.
	pipe := s.kv.Pipeline()
	pipe.Set(r.Context(), kvkeys.Session(user.ID), "1", jwtTTL)
	pipe.Set(r.Context(), kvkeys.RefreshToken(refreshToken), user.ID, refreshTTL)
	if _, err := pipe.Exec(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}

	_ = s.db.UpdateLastLogin(r.Context(), user.ID)

	writeJSON(w, http.StatusOK, map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"username":      user.Username,
	})
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Service) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	userID, err := s.kv.Get(r.Context(), kvkeys.RefreshToken(req.RefreshToken)).Result()
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	accessToken, err := s.issueJWT(userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt error")
		return
	}
	// Rotate refresh token.
	newRefresh := uuid.New().String()
	pipe := s.kv.Pipeline()
	pipe.Del(r.Context(), kvkeys.RefreshToken(req.RefreshToken))
	pipe.Set(r.Context(), kvkeys.Session(userID), "1", jwtTTL)
	pipe.Set(r.Context(), kvkeys.RefreshToken(newRefresh), userID, refreshTTL)
	if _, err := pipe.Exec(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"access_token":  accessToken,
		"refresh_token": newRefresh,
	})
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())
	if userID == "" {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	s.kv.Del(r.Context(), kvkeys.Session(userID))
	w.WriteHeader(http.StatusNoContent)
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Service) changePassword(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())

	var req changePasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeErr(w, http.StatusBadRequest, "current_password and new_password required")
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeErr(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := s.db.UpdateUserPassword(r.Context(), userID, string(hash)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type changeEmailReq struct {
	NewEmail        string `json:"new_email"`
	CurrentPassword string `json:"current_password"`
}

func (s *Service) changeEmail(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())

	var req changeEmailReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.NewEmail == "" || req.CurrentPassword == "" {
		writeErr(w, http.StatusBadRequest, "new_email and current_password required")
		return
	}
	if !strings.Contains(req.NewEmail, "@") {
		writeErr(w, http.StatusBadRequest, "invalid email address")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeErr(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	if err := s.db.UpdateUserEmail(r.Context(), userID, req.NewEmail); err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeErr(w, http.StatusConflict, "email already in use")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- JWT ---

type claims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}

func (s *Service) issueJWT(userID string) (string, error) {
	now := time.Now()
	c := claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.jwtSecret)
}

// --- Middleware ---

type ctxKey string

const ctxUserID ctxKey = "user_id"

// JWTMiddleware validates Bearer tokens and injects the user ID into the context.
func JWTMiddleware(secret string, kv *redis.Client) func(http.Handler) http.Handler {
	key := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdr := r.Header.Get("Authorization")
			var tokenStr string
			if strings.HasPrefix(hdr, "Bearer ") {
				tokenStr = strings.TrimPrefix(hdr, "Bearer ")
			} else {
				// Accept token via query param (for signed URLs) or cookie.
				tokenStr = r.URL.Query().Get("token")
				if tokenStr == "" {
					if c, err := r.Cookie("access_token"); err == nil {
						tokenStr = c.Value
					}
				}
			}
			if tokenStr == "" {
				writeErr(w, http.StatusUnauthorized, "missing token")
				return
			}
			var c claims
			tok, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.New("unexpected signing method")
				}
				return key, nil
			})
			if err != nil || !tok.Valid {
				writeErr(w, http.StatusUnauthorized, "invalid token")
				return
			}
			// Check session is still active (not logged out).
			exists, err := kv.Exists(r.Context(), kvkeys.Session(c.UserID)).Result()
			if err != nil || exists == 0 {
				writeErr(w, http.StatusUnauthorized, "session expired")
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, c.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromCtx extracts the authenticated user ID from the request context.
func UserIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxUserID).(string)
	return v
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
