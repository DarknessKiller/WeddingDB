package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct{ adminRepo *repository.AdminRepo }

func NewAdminHandler(adminRepo *repository.AdminRepo) *AdminHandler {
	return &AdminHandler{adminRepo: adminRepo}
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
	if err := validatePassword(body.Password); err != nil {
		return nil, err
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
	}
	if err := h.adminRepo.Create(ctx, admin); err != nil {
		return nil, err
	}
	// Assign weddings if provided
	if len(body.Weddings) > 0 {
		var ids []uuid.UUID
		for _, ws := range body.Weddings {
			if id, err := DecodeID(ws); err == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			h.adminRepo.SetUserWeddings(ctx, admin.ID, ids)
		}
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
	if target.Role == "admin" {
		adminCount, err := h.adminRepo.CountByRole(ctx, "admin")
		if err != nil {
			return nil, fuego.InternalServerError{Title: "Failed to check admin count"}
		}
		if adminCount <= 1 {
			return nil, fuego.BadRequestError{Title: "Cannot delete the last admin"}
		}
	}
	if err := h.adminRepo.Delete(ctx, id); err != nil {
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
	var ids []uuid.UUID
	for _, ws := range body.Weddings {
		if wid, err := DecodeID(ws); err == nil {
			ids = append(ids, wid)
		}
	}
	if err := h.adminRepo.SetUserWeddings(ctx, admin.ID, ids); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to assign weddings"}
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
	admin, err := h.adminRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	admin.Password = string(hash)
	if err := h.adminRepo.Update(ctx, admin); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update password"}
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
	// Prevent demoting the last admin
	if admin.Role == "admin" && body.Role == "user" {
		adminCount, err := h.adminRepo.CountByRole(ctx, "admin")
		if err != nil {
			return nil, fuego.InternalServerError{Title: "Failed to check admin count"}
		}
		if adminCount <= 1 {
			return nil, fuego.BadRequestError{Title: "Cannot demote the last admin"}
		}
	}
	admin.Role = body.Role
	if err := h.adminRepo.Update(ctx, admin); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update role"}
	}
	return admin, nil
}
