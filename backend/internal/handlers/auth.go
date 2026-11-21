package handlers

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"os"
	"unicode"
	"unicode/utf8"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// maxLen answers 400 when value has more than limit characters, before the
// column's varchar limit turns it into an opaque 500.
func maxLen(field, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return fuego.BadRequestError{Title: fmt.Sprintf("%s must be at most %d characters", field, limit)}
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fuego.BadRequestError{Title: "Password must be at least 8 characters"}
	}
	var hasLetter, hasDigit, hasSymbol bool
	for _, c := range password {
		switch {
		case unicode.IsLetter(c):
			hasLetter = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSymbol = true
		}
	}
	if !hasLetter || !hasDigit || !hasSymbol {
		return fuego.BadRequestError{Title: "Password must contain at least one letter, one number, and one symbol"}
	}
	return nil
}

type AuthHandler struct {
	authService *services.AuthService
	adminRepo   *repository.AdminRepo
}

func NewAuthHandler(authService *services.AuthService, adminRepo *repository.AdminRepo) *AuthHandler {
	return &AuthHandler{authService: authService, adminRepo: adminRepo}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type WeddingInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Date string `json:"date"`
}

type TokenResponse struct {
	AccessToken         string        `json:"accessToken"`
	RefreshToken        string        `json:"refreshToken"`
	Role                string        `json:"role"`
	Name                string        `json:"name"`
	Weddings            []WeddingInfo `json:"weddings"`
	ForcePasswordChange bool          `json:"forcePasswordChange"`
}

type SelectWeddingRequest struct {
	WeddingID string `json:"weddingId"`
	// RefreshToken scopes the selection to this device's session.
	RefreshToken string `json:"refreshToken"`
}

type SelectWeddingResponse struct {
	AccessToken string `json:"accessToken"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func weddingInfos(weddings []models.WeddingEvent) []WeddingInfo {
	out := make([]WeddingInfo, len(weddings))
	for i, w := range weddings {
		out[i] = WeddingInfo{ID: utils.EncodeUUID(w.ID), Name: w.Name, Date: w.Date.Format("2006-01-02")}
	}
	return out
}

func (h *AuthHandler) Login(c fuego.ContextWithBody[LoginRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	ip := clientIP(c.Request())
	if err := h.authService.CheckAuthRate(c.Context(), ip); err != nil {
		return TokenResponse{}, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	if err := h.authService.CheckLoginIPRate(c.Context(), ip); err != nil {
		return TokenResponse{}, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	if err := h.authService.CheckLoginAccountRate(c.Context(), body.Email, ip); err != nil {
		return TokenResponse{}, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	result, err := h.authService.Login(c.Context(), body.Email, body.Password)
	if errors.Is(err, services.ErrInvalidCredentials) {
		h.authService.RecordLoginFailure(c.Context(), body.Email, ip)
		return TokenResponse{}, fuego.UnauthorizedError{Title: "invalid credentials"}
	}
	if errors.Is(err, services.ErrSessionsChanged) {
		return TokenResponse{}, fuego.ConflictError{Title: err.Error()}
	}
	if err != nil {
		slog.Error("login failed", "err", err)
		return TokenResponse{}, fuego.InternalServerError{Title: "Login failed"}
	}
	return TokenResponse{
		AccessToken:         result.AccessToken,
		RefreshToken:        result.RefreshToken,
		Role:                result.Role,
		Name:                result.Name,
		Weddings:            weddingInfos(result.Weddings),
		ForcePasswordChange: result.ForcePasswordChange,
	}, nil
}

func (h *AuthHandler) SelectWedding(c fuego.ContextWithBody[SelectWeddingRequest]) (SelectWeddingResponse, error) {
	body, err := c.Body()
	if err != nil {
		return SelectWeddingResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	weddingID, err := middleware.DecodeWIDString(body.WeddingID)
	if err != nil {
		return SelectWeddingResponse{}, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	adminID := AdminIDFromContext(c.Context())
	accessToken, err := h.authService.SelectWedding(c.Context(), adminID, weddingID, body.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrNoWeddingAccess):
			return SelectWeddingResponse{}, fuego.ForbiddenError{Title: "No access to this wedding"}
		case errors.Is(err, services.ErrAdminNotFound), errors.Is(err, services.ErrSessionNotFound):
			return SelectWeddingResponse{}, fuego.UnauthorizedError{Title: "Unauthorized"}
		}
		slog.Error("select wedding failed", "user", adminID, "err", err)
		return SelectWeddingResponse{}, fuego.InternalServerError{Title: "Internal error"}
	}
	return SelectWeddingResponse{AccessToken: accessToken}, nil
}

func (h *AuthHandler) Refresh(c fuego.ContextWithBody[RefreshRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	// Extract old access token from Authorization header for blacklisting on rotation
	oldAccessToken := middleware.ExtractBearer(c.Request())
	if err := h.authService.CheckAuthRate(c.Context(), clientIP(c.Request())); err != nil {
		return TokenResponse{}, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	result, err := h.authService.Refresh(c.Context(), body.RefreshToken, oldAccessToken)
	if errors.Is(err, services.ErrInvalidRefreshToken) || errors.Is(err, services.ErrAdminNotFound) {
		return TokenResponse{}, fuego.UnauthorizedError{Title: "invalid refresh token"}
	}
	if err != nil {
		slog.Error("refresh failed", "err", err)
		return TokenResponse{}, fuego.InternalServerError{Title: "Refresh failed"}
	}
	return TokenResponse{
		AccessToken:         result.AccessToken,
		RefreshToken:        result.RefreshToken,
		Role:                result.Role,
		Name:                result.Name,
		Weddings:            weddingInfos(result.Weddings),
		ForcePasswordChange: result.ForcePasswordChange,
	}, nil
}

func (h *AuthHandler) Logout(c fuego.ContextWithBody[RefreshRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	// Public endpoint with no authentication: rate limit it so the refresh-token
	// store cannot be churned by unauthenticated callers.
	if err := h.authService.CheckAuthRate(c.Context(), clientIP(c.Request())); err != nil {
		return nil, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}

	// Extract access token from Authorization header for blacklisting
	accessToken := middleware.ExtractBearer(c.Request())

	if err := h.authService.Logout(c.Context(), body.RefreshToken, accessToken); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to logout"}
	}
	c.SetStatus(204)
	return nil, nil
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	Password        string `json:"password"`
}

func (h *AuthHandler) ChangePassword(c fuego.ContextWithBody[ChangePasswordRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if err := validatePassword(body.Password); err != nil {
		return nil, err
	}
	adminID := AdminIDFromContext(c.Context())
	ctx := c.Context()
	admin, err := h.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}
	// A forced change follows an admin reset, so the old password is not
	// known. Only a token minted under the reset (pwc claim, which needs the
	// reset password to obtain) may skip the check: a token from before the
	// reset must still prove the password, or a stolen one could take over
	// the account. The claim, not the DB flag, decides, so a forced change
	// whose revoke failed can be retried from the same page.
	// Same per-account lockout as Login, so a stolen access token cannot
	// brute-force the current password.
	forced := ForcePasswordChangeFromContext(ctx)
	if !forced {
		ip := clientIP(c.Request())
		if err := h.authService.CheckLoginAccountRate(ctx, admin.Email, ip); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
		}
		if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(body.CurrentPassword)); err != nil {
			h.authService.RecordLoginFailure(ctx, admin.Email, ip)
			return nil, fuego.UnauthorizedError{Title: "Current password is incorrect"}
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	if err := h.adminRepo.SetPassword(ctx, admin.ID, string(hash), false); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update password"}
	}
	// Revoke all existing sessions so compromised tokens are invalidated.
	// The hash is already saved, but reporting success while old sessions
	// still work would tell the user a stolen session is gone when it isn't.
	if err := h.authService.RevokeUserTokens(ctx, adminID); err != nil {
		slog.Error("revoke tokens after password change failed", "user", adminID, "err", err)
		if forced {
			return nil, errRevokeAfterForcedChange
		}
		return nil, errRevokeAfterPassword
	}
	return map[string]any{"message": "Password updated"}, nil
}

// errRevokeAfterPassword answers a password write whose session revoke
// failed. Changing it again, with the new password as current, retries the revoke.
var errRevokeAfterPassword = fuego.InternalServerError{Title: "Password saved, but signing out other sessions failed. Change it again, entering your new password as the current one, to retry."}

// errRevokeAfterForcedChange is the same for a forced change, whose page
// asks for no current password: setting it again retries the revoke.
var errRevokeAfterForcedChange = fuego.InternalServerError{Title: "Password saved, but signing out other sessions failed. Set it again to retry."}

// errRevokeAfterReset answers an admin reset whose session revoke failed.
// Resetting again retries the revoke.
var errRevokeAfterReset = fuego.InternalServerError{Title: "Password saved, but signing out the user's sessions failed. Reset it again to retry."}

// loginLimitError maps a failed login rate-limit check: an unreadable limit
// is an outage (503), anything else is the limit being hit (429).
func loginLimitError(err error) error {
	if errors.Is(err, services.ErrRateLimitUnavailable) {
		return fuego.HTTPError{Status: http.StatusServiceUnavailable, Title: err.Error()}
	}
	return fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
}

func (h *AuthHandler) Register(c fuego.ContextWithBody[RegisterRequest]) (any, error) {
	if os.Getenv("ALLOW_REGISTRATION") != "true" {
		return nil, fuego.ForbiddenError{Title: "Registration is disabled. Ask an admin for an account."}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Email == "" || body.Password == "" || body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name, email, and password are required"}
	}
	// Validate email format
	addr, err := mail.ParseAddress(body.Email)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid email format"}
	}
	// "Bob <bob@x.com>" parses too; keep only the address, or logging in
	// with bob@x.com would never match the stored value.
	body.Email = addr.Address
	if err := cmp.Or(maxLen("Name", body.Name, 255), maxLen("Email", body.Email, 255), validatePassword(body.Password)); err != nil {
		return nil, err
	}
	ctx := c.Context()
	if err := h.authService.CheckAuthRate(ctx, clientIP(c.Request())); err != nil {
		return nil, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	if err := h.authService.CheckCredentialIPRate(ctx, clientIP(c.Request())); err != nil {
		return nil, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	existing, err := h.adminRepo.FindByEmail(ctx, body.Email)
	if err != nil {
		slog.Warn("email lookup during registration failed", "err", err)
	}
	if existing != nil && existing.ID != uuid.Nil {
		return nil, fuego.ConflictError{Title: "Email already registered"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	admin := &models.AdminUser{
		Email:    body.Email,
		Password: string(hash),
		Name:     body.Name,
		Role:     "user",
	}
	if err := h.adminRepo.Create(ctx, admin); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to create account"}
	}
	c.SetStatus(201)
	return map[string]any{"id": admin.ID.String(), "email": admin.Email, "name": admin.Name, "role": admin.Role}, nil
}

// clientIP strips the port from RemoteAddr (already XFF-normalized by
// ProxyAwareMiddleware when behind a trusted proxy). An IPv6 client is keyed
// by its /64: one host usually owns the whole prefix and can rotate through
// it, so a per-address key would never reach any rate limit.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Is6() || addr.Is4In6() {
		return host
	}
	prefix, err := addr.WithZone("").Prefix(64)
	if err != nil {
		return host
	}
	return prefix.String()
}
