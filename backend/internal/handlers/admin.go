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
	return h.adminRepo.List()
}

func (h *AdminHandler) Create(c fuego.ContextWithBody[AdminRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
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
	id := DecodeID(c.PathParam("id"))
	if err := h.adminRepo.Delete(id); err != nil {
		return nil, err
	}
	return nil, nil
}
