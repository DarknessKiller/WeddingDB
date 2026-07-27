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
	if err := requireAdmin(c.Context()); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	return h.adminRepo.List()
}

func (h *AdminHandler) Create(c fuego.ContextWithBody[AdminRequest]) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if err := validatePassword(body.Password); err != nil {
		return nil, err
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
	if err := h.adminRepo.Create(admin); err != nil {
		return nil, err
	}
	// Assign weddings if provided
	if len(body.Weddings) > 0 {
		var ids []uuid.UUID
		for _, ws := range body.Weddings {
			if id, err := uuid.Parse(ws); err == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			h.adminRepo.SetUserWeddings(admin.ID, ids)
		}
	}
	return admin, nil
}

func (h *AdminHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	adminID := AdminIDFromContext(c.Context())
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if id == adminID {
		return nil, fuego.BadRequestError{Title: "Cannot delete your own account"}
	}
	if err := h.adminRepo.Delete(id); err != nil {
		return nil, err
	}
	return nil, nil
}

type AssignWeddingsRequest struct {
	Weddings []string `json:"weddings"`
}

func (h *AdminHandler) AssignWeddings(c fuego.ContextWithBody[AssignWeddingsRequest]) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
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
	admin, err := h.adminRepo.FindByID(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Admin not found"}
	}
	var ids []uuid.UUID
	for _, ws := range body.Weddings {
		if wid, err := uuid.Parse(ws); err == nil {
			ids = append(ids, wid)
		}
	}
	if err := h.adminRepo.SetUserWeddings(admin.ID, ids); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to assign weddings"}
	}
	return admin, nil
}

func (h *AdminHandler) GetUserWeddings(c fuego.ContextNoBody) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	weddings, err := h.adminRepo.GetUserWeddings(id)
	if err != nil {
		return nil, err
	}
	return weddings, nil
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

func (h *AdminHandler) ResetPassword(c fuego.ContextWithBody[ResetPasswordRequest]) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
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
	admin, err := h.adminRepo.FindByID(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "User not found"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	admin.Password = string(hash)
	if err := h.adminRepo.Update(admin); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update password"}
	}
	return map[string]any{"message": "Password updated"}, nil
}
