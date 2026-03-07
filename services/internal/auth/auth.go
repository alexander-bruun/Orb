// Package auth handles user registration, login, JWT issuance, and session management.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtTTL          = 15 * time.Minute
	refreshTTL      = 7 * 24 * time.Hour
	loginLimit      = 10 // max attempts per IP per window
	loginWindow     = time.Minute
	totpPendingTTL  = 5 * time.Minute // short window to complete 2FA after password
	backupCodeCount = 8
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

	// TOTP second-factor verification (uses temp token, no JWT needed).
	r.Post("/totp/verify", s.totpVerify)

	// Account-management endpoints require a valid JWT.
	r.Group(func(r chi.Router) {
		r.Use(JWTMiddleware(string(s.jwtSecret), s.kv))
		r.Patch("/password", s.changePassword)
		r.Patch("/email", s.changeEmail)

		// 2FA management.
		r.Get("/totp/status", s.totpStatus)
		r.Post("/totp/setup", s.totpSetup)
		r.Post("/totp/enable", s.totpEnable)
		r.Post("/totp/disable", s.totpDisable)
		r.Post("/totp/backup-codes/regenerate", s.totpRegenerateBackupCodes)
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

	// If TOTP is enabled, issue a short-lived pending token instead of full session.
	if user.TOTPEnabled {
		pendingToken := uuid.New().String()
		if err := s.kv.Set(r.Context(), kvkeys.TOTPPending(pendingToken), user.ID, totpPendingTTL).Err(); err != nil {
			writeErr(w, http.StatusInternalServerError, "session error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"totp_required": true,
			"temp_token":    pendingToken,
		})
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

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"username":      user.Username,
		"totp_required": false,
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

// --- TOTP handlers ---

// totpStatus returns whether 2FA is enabled for the authenticated user.
func (s *Service) totpStatus(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": user.TOTPEnabled})
}

type totpSetupReq struct {
	// no body params needed
}

// totpSetup generates a fresh TOTP secret and returns the provisioning URI for
// the authenticator app. The secret is persisted to the DB but 2FA is NOT yet
// enabled — the client must call /totp/enable with a valid code to activate it.
func (s *Service) totpSetup(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Orb",
		AccountName: user.Email,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "totp generate error")
		return
	}

	if err := s.db.SetTOTPSecret(r.Context(), userID, key.Secret()); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"secret":      key.Secret(),
		"otpauth_url": key.URL(),
	})
}

type totpEnableReq struct {
	Code string `json:"code"`
}

// totpEnable verifies the first TOTP code, permanently enables 2FA, and
// returns one-time backup codes (plaintext — shown once, never again).
func (s *Service) totpEnable(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())

	var req totpEnableReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, "code required")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if user.TOTPSecret == nil || *user.TOTPSecret == "" {
		writeErr(w, http.StatusBadRequest, "run /totp/setup first")
		return
	}
	if user.TOTPEnabled {
		writeErr(w, http.StatusConflict, "2FA already enabled")
		return
	}

	if !totp.Validate(req.Code, *user.TOTPSecret) {
		writeErr(w, http.StatusUnauthorized, "invalid TOTP code")
		return
	}

	plain, hashed, err := generateBackupCodes()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "backup code generation error")
		return
	}
	codesJSON, err := json.Marshal(hashed)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encoding error")
		return
	}

	if err := s.db.EnableTOTP(r.Context(), userID, string(codesJSON)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backup_codes": plain,
	})
}

type totpDisableReq struct {
	Password string `json:"password"`
	Code     string `json:"code"` // TOTP code OR backup code
}

// totpDisable disables 2FA after verifying the user's password and a TOTP code.
func (s *Service) totpDisable(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())

	var req totpDisableReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Password == "" || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "password and code required")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !user.TOTPEnabled {
		writeErr(w, http.StatusConflict, "2FA is not enabled")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid password")
		return
	}
	if !totp.Validate(req.Code, *user.TOTPSecret) {
		writeErr(w, http.StatusUnauthorized, "invalid TOTP code")
		return
	}

	if err := s.db.DisableTOTP(r.Context(), userID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type totpVerifyReq struct {
	TempToken string `json:"temp_token"`
	Code      string `json:"code"` // TOTP code OR backup code
}

// totpVerify completes the login flow when 2FA is enabled.
// The client sends the temp token from the login response plus the TOTP code.
// On success, a full session is established and tokens are returned.
func (s *Service) totpVerify(w http.ResponseWriter, r *http.Request) {
	var req totpVerifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.TempToken == "" || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "temp_token and code required")
		return
	}

	userID, err := s.kv.Get(r.Context(), kvkeys.TOTPPending(req.TempToken)).Result()
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if user.TOTPSecret == nil {
		writeErr(w, http.StatusInternalServerError, "totp not configured")
		return
	}

	// Try TOTP code first, then backup codes.
	validated := totp.Validate(req.Code, *user.TOTPSecret)
	if !validated {
		// Check backup codes.
		consumed, remaining, ok := consumeBackupCode(req.Code, user.TOTPBackupCodes)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "invalid TOTP code")
			return
		}
		_ = consumed
		codesJSON, _ := json.Marshal(remaining)
		_ = s.db.UpdateTOTPBackupCodes(r.Context(), userID, string(codesJSON))
	}

	// Consume the pending token so it can't be used again.
	s.kv.Del(r.Context(), kvkeys.TOTPPending(req.TempToken))

	accessToken, err := s.issueJWT(userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jwt error")
		return
	}
	refreshToken := uuid.New().String()

	pipe := s.kv.Pipeline()
	pipe.Set(r.Context(), kvkeys.Session(userID), "1", jwtTTL)
	pipe.Set(r.Context(), kvkeys.RefreshToken(refreshToken), userID, refreshTTL)
	if _, err := pipe.Exec(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}

	_ = s.db.UpdateLastLogin(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"username":      user.Username,
	})
}

type totpRegenerateBackupCodesReq struct {
	Code string `json:"code"`
}

// totpRegenerateBackupCodes generates fresh backup codes, replacing the old ones.
// Requires a valid TOTP code to prevent abuse.
func (s *Service) totpRegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())

	var req totpRegenerateBackupCodesReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, "code required")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !user.TOTPEnabled || user.TOTPSecret == nil {
		writeErr(w, http.StatusBadRequest, "2FA is not enabled")
		return
	}
	if !totp.Validate(req.Code, *user.TOTPSecret) {
		writeErr(w, http.StatusUnauthorized, "invalid TOTP code")
		return
	}

	plain, hashed, err := generateBackupCodes()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "backup code generation error")
		return
	}
	codesJSON, _ := json.Marshal(hashed)
	if err := s.db.UpdateTOTPBackupCodes(r.Context(), userID, string(codesJSON)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backup_codes": plain,
	})
}

// --- backup code helpers ---

// generateBackupCodes creates backupCodeCount random codes.
// Returns (plaintext_codes, sha256_hashed_hex_codes, error).
func generateBackupCodes() ([]string, []string, error) {
	plain := make([]string, backupCodeCount)
	hashed := make([]string, backupCodeCount)
	for i := range plain {
		b := make([]byte, 5) // 10 hex chars
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		code := fmt.Sprintf("%x", b) // 10-char lowercase hex
		plain[i] = code
		hashed[i] = hashBackupCode(code)
	}
	return plain, hashed, nil
}

func hashBackupCode(code string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.ReplaceAll(code, "-", ""))))
	return hex.EncodeToString(h[:])
}

// consumeBackupCode checks a user-supplied code against stored hashed codes.
// Returns (consumed_hash, remaining_hashes, ok).
func consumeBackupCode(code string, backupCodesJSON *string) (string, []string, bool) {
	if backupCodesJSON == nil {
		return "", nil, false
	}
	var stored []string
	if err := json.Unmarshal([]byte(*backupCodesJSON), &stored); err != nil {
		return "", nil, false
	}
	incoming := hashBackupCode(code)
	for i, h := range stored {
		if subtle.ConstantTimeCompare([]byte(incoming), []byte(h)) == 1 {
			remaining := append(stored[:i:i], stored[i+1:]...)
			return h, remaining, true
		}
	}
	return "", nil, false
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
