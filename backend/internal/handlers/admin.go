package handlers

import (
	"cmp"
	"errors"
	"log/slog"
	"net/mail"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct {
	adminRepo   *repository.AdminRepo
	authService *services.AuthService
}

func NewAdminHandler(adminRepo *repository.AdminRepo, authService *services.AuthService) *AdminHandler {
	return &AdminHandler{adminRepo: adminRepo, authService: authService}
}

type AdminRequest struct {
	Email    string   `json:"email"`
	Password string   `json:"password,omitempty"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Weddings []string `json:"weddings,omitempty"`
}

func (h *AdminHandler) List(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	return h.adminRepo.List(ctx)
}

func (h *AdminHandler) Create(c fuego.ContextWithBody[AdminRequest]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Email == "" || body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name and email are required"}
	}
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
	// The unique index would otherwise surface as an opaque 500.
	if _, err := h.adminRepo.FindByEmail(ctx, body.Email); err == nil {
		return nil, fuego.ConflictError{Title: "Email already registered"}
	}
	// Validate role field
	if body.Role != "admin" && body.Role != "user" {
		body.Role = "user"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	admin := &models.AdminUser{
		Email:    body.Email,
		Password: string(hash),
		Name:     body.Name,
		Role:     body.Role,
		// Admin chose this password, so the user must replace it on first login.
		ForcePasswordChange: true,
	}
	ids, err := decodeWeddingIDs(body.Weddings)
	if err != nil {
		return nil, err
	}
	if err := h.adminRepo.CreateWithWeddings(ctx, admin, ids); err != nil {
		return nil, err
	}
	c.SetStatus(201)
	return admin, nil
}

func (h *AdminHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	adminID := AdminIDFromContext(ctx)
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if id == adminID {
		return nil, fuego.BadRequestError{Title: "Cannot delete your own account"}
	}
	target, err := h.adminRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}
	// Early check so the last admin's sessions are not revoked for a refused
	// delete; adminRepo.Delete re-checks under lock to close the race.
	if target.Role == "admin" {
		adminCount, err := h.adminRepo.CountByRole(ctx, "admin")
		if err != nil {
			return nil, fuego.InternalServerError{Title: "Failed to check admin count"}
		}
		if adminCount <= 1 {
			return nil, fuego.BadRequestError{Title: "Cannot delete the last admin"}
		}
	}
	// Revoke first: ValidateToken never checks that the user still exists, so
	// a deleted user's access tokens and open SSE streams live on otherwise.
	if err := h.authService.RevokeUserTokens(ctx, id); err != nil {
		slog.Error("revoke tokens before user delete failed", "user", id, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to revoke user sessions"}
	}
	if err := h.adminRepo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLastAdmin) {
			return nil, fuego.BadRequestError{Title: "Cannot delete the last admin"}
		}
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

type AssignWeddingsRequest struct {
	Weddings []string `json:"weddings"`
}

func (h *AdminHandler) AssignWeddings(c fuego.ContextWithBody[AssignWeddingsRequest]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	admin, err := h.adminRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Admin not found"}
	}
	ids, err := decodeWeddingIDs(body.Weddings)
	if err != nil {
		return nil, err
	}
	// Fetch the old grants so removing access can revoke live sessions.
	oldWeddings, err := h.adminRepo.GetUserWeddings(ctx, admin.ID)
	if err != nil {
		slog.Warn("fetch current wedding grants failed", "user", admin.ID, "err", err)
	}
	if err := h.adminRepo.SetUserWeddings(ctx, admin.ID, ids); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to assign weddings"}
	}
	// Unknown old grants: revoke anyway rather than risk a lingering scoped token.
	revoke := err != nil
	kept := make(map[uuid.UUID]bool, len(ids))
	for _, wid := range ids {
		kept[wid] = true
	}
	for _, w := range oldWeddings {
		if !kept[w.ID] {
			revoke = true
			break
		}
	}
	if revoke {
		// Refresh also re-checks access, so a Redis failure only leaves the
		// old scoped token valid until its next refresh; log, don't fail.
		if err := h.authService.RevokeUserTokens(ctx, admin.ID); err != nil {
			slog.Warn("revoke tokens after wedding unassign failed", "user", admin.ID, "err", err)
		}
	}
	return admin, nil
}

func (h *AdminHandler) GetUserWeddings(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	weddings, err := h.adminRepo.GetUserWeddings(ctx, id)
	if err != nil {
		return nil, err
	}
	return weddings, nil
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

func (h *AdminHandler) ResetPassword(c fuego.ContextWithBody[ResetPasswordRequest]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if err := validatePassword(body.Password); err != nil {
		return nil, err
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	// Resetting your own password here would skip the current-password proof;
	// account owners use /api/auth/change-password instead.
	if id == AdminIDFromContext(ctx) {
		return nil, fuego.BadRequestError{Title: "Use change-password to change your own password"}
	}
	admin, err := h.adminRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	// The admin chose and knows this password, so the user must replace it.
	if err := h.adminRepo.SetPassword(ctx, admin.ID, string(hash), true); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update password"}
	}
	// Kill sessions minted under the old password. Password is already
	// updated — a Redis failure must not fail the request; Refresh
	// re-validates access anyway.
	if err := h.authService.RevokeUserTokens(ctx, id); err != nil {
		slog.Warn("revoke tokens after admin password reset failed", "user", id, "err", err)
	}
	return map[string]any{"message": "Password updated"}, nil
}

type UpdateRoleRequest struct {
	Role string `json:"role"`
}

func (h *AdminHandler) UpdateRole(c fuego.ContextWithBody[UpdateRoleRequest]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Role != "admin" && body.Role != "user" {
		return nil, fuego.BadRequestError{Title: "Role must be admin or user"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	// Prevent self-demotion
	callerID := AdminIDFromContext(ctx)
	if id == callerID && body.Role != "admin" {
		return nil, fuego.BadRequestError{Title: "Cannot change your own role"}
	}
	// Fetch user once for guard check and update
	admin, err := h.adminRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}
	// Early check so the last admin's sessions are not revoked for a refused
	// demotion; adminRepo.UpdateRole re-checks under lock to close the race.
	if admin.Role == "admin" && body.Role != "admin" {
		adminCount, err := h.adminRepo.CountByRole(ctx, "admin")
		if err != nil {
			return nil, fuego.InternalServerError{Title: "Failed to check admin count"}
		}
		if adminCount <= 1 {
			return nil, fuego.BadRequestError{Title: "Cannot demote the last admin"}
		}
	}
	// Revoke first, like Delete: live tokens carry the role claim, so a revoke
	// that failed after the change would leave a demoted admin with admin
	// tokens until they expire.
	if err := h.authService.RevokeUserTokens(ctx, id); err != nil {
		slog.Error("revoke tokens before role change failed", "user", id, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to revoke user sessions"}
	}
	if err := h.adminRepo.UpdateRole(ctx, id, body.Role); err != nil {
		if errors.Is(err, repository.ErrLastAdmin) {
			return nil, fuego.BadRequestError{Title: "Cannot demote the last admin"}
		}
		return nil, fuego.InternalServerError{Title: "Failed to update role"}
	}
	admin.Role = body.Role
	return admin, nil
}

// RevokeUser bumps the target user's token version in Redis, invalidating all their active tokens.
func (h *AdminHandler) RevokeUser(c fuego.ContextNoBody) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}

	callerID := AdminIDFromContext(c.Context())
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}

	// Prevent self-revoke
	if id == callerID {
		return nil, fuego.BadRequestError{Title: "Cannot revoke your own tokens"}
	}

	// Verify user exists
	if _, err := h.adminRepo.FindByID(c.Context(), id); err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}

	if err := h.authService.RevokeUserTokens(c.Context(), id); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to revoke user tokens"}
	}

	return map[string]any{"message": "User tokens revoked"}, nil
}

// decodeWeddingIDs parses every wedding ID or rejects the request. Silently
// dropping a bad one would grant (or, on reassign, revoke) the wrong set.
func decodeWeddingIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, ws := range raw {
		id, err := DecodeID(ws)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid wedding ID: " + ws}
		}
		ids = append(ids, id)
	}
	return ids, nil
}
