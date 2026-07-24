package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/go-fuego/fuego"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct{ adminRepo *repository.AdminRepo }

func NewAdminHandler(adminRepo *repository.AdminRepo) *AdminHandler {
	return &AdminHandler{adminRepo: adminRepo}
}

type AdminRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password,omitempty"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	WeddingID *uint  `json:"weddingId,omitempty"`
}

func (h *AdminHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	role := RoleFromContext(c.Context())
	if role != "service_admin" {
		return nil, fuego.UnauthorizedError{Title: "service_admin role required"}
	}
	return h.adminRepo.List()
}

func (h *AdminHandler) Create(c fuego.ContextWithBody[AdminRequest]) (any, error) {
	role := RoleFromContext(c.Context())
	if role != "service_admin" {
		return nil, fuego.UnauthorizedError{Title: "service_admin role required"}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Password == "" {
		return nil, fuego.BadRequestError{Title: "Password is required"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to hash password"}
	}
	admin := &models.AdminUser{
		Email:     body.Email,
		Password:  string(hash),
		Name:      body.Name,
		Role:      body.Role,
		WeddingID: body.WeddingID,
	}
	if err := h.adminRepo.Create(admin); err != nil {
		return nil, err
	}
	return admin, nil
}

func (h *AdminHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	role := RoleFromContext(c.Context())
	if role != "service_admin" {
		return nil, fuego.UnauthorizedError{Title: "service_admin role required"}
	}
	adminID := AdminIDFromContext(c.Context())
	id := DecodeID(c.PathParam("id"))
	// Prevent self-deletion
	if id == adminID {
		return nil, fuego.BadRequestError{Title: "Cannot delete your own account"}
	}
	if err := h.adminRepo.Delete(id); err != nil {
		return nil, err
	}
	return nil, nil
}

type AssignWeddingRequest struct {
	WeddingID *uint `json:"weddingId"`
}

func (h *AdminHandler) AssignWedding(c fuego.ContextWithBody[AssignWeddingRequest]) (any, error) {
	role := RoleFromContext(c.Context())
	if role != "service_admin" {
		return nil, fuego.UnauthorizedError{Title: "service_admin role required"}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	id := DecodeID(c.PathParam("id"))
	admin, err := h.adminRepo.FindByID(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Admin not found"}
	}
	admin.WeddingID = body.WeddingID
	if err := h.adminRepo.Update(admin); err != nil {
		return nil, err
	}
	return admin, nil
}
