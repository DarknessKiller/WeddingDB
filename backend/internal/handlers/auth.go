package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"golang.org/x/crypto/bcrypt"
)

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

type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	WeddingID    *uint  `json:"weddingId,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) Login(c fuego.ContextWithBody[LoginRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	access, refresh, role, name, weddingID, err := h.authService.Login(c.Context(), body.Email, body.Password)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{AccessToken: access, RefreshToken: refresh, Role: role, Name: name, WeddingID: weddingID}, nil
}

func (h *AuthHandler) Refresh(c fuego.ContextWithBody[RefreshRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	access, refresh, role, name, weddingID, err := h.authService.Refresh(c.Context(), body.RefreshToken)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{AccessToken: access, RefreshToken: refresh, Role: role, Name: name, WeddingID: weddingID}, nil
}

func (h *AuthHandler) Logout(c fuego.ContextWithBody[RefreshRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	h.authService.Logout(body.RefreshToken)
	return nil, nil
}

func (h *AuthHandler) Register(c fuego.ContextWithBody[RegisterRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Email == "" || body.Password == "" || body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name, email, and password are required"}
	}
	// Check if email already taken
	existing, _ := h.adminRepo.FindByEmail(body.Email)
	if existing != nil && existing.ID != 0 {
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
		Role:     "wedding_admin",
	}
	if err := h.adminRepo.Create(admin); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to create account"}
	}
	return map[string]any{"id": admin.ID, "email": admin.Email, "name": admin.Name, "role": admin.Role}, nil
}
