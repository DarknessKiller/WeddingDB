package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
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

type WeddingInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Date string `json:"date"`
}

type TokenResponse struct {
	AccessToken  string        `json:"accessToken"`
	RefreshToken string        `json:"refreshToken"`
	Role         string        `json:"role"`
	Name         string        `json:"name"`
	Weddings     []WeddingInfo `json:"weddings"`
}

type SelectWeddingRequest struct {
	WeddingID string `json:"weddingId"`
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
		out[i] = WeddingInfo{ID: w.ID.String(), Name: w.Name, Date: w.Date.Format("2006-01-02")}
	}
	return out
}

func (h *AuthHandler) Login(c fuego.ContextWithBody[LoginRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	result, err := h.authService.Login(c.Context(), body.Email, body.Password)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Role:         result.Role,
		Name:         result.Name,
		Weddings:     weddingInfos(result.Weddings),
	}, nil
}

func (h *AuthHandler) SelectWedding(c fuego.ContextWithBody[SelectWeddingRequest]) (SelectWeddingResponse, error) {
	body, err := c.Body()
	if err != nil {
		return SelectWeddingResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	weddingID, err := uuid.Parse(body.WeddingID)
	if err != nil {
		return SelectWeddingResponse{}, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	adminID := AdminIDFromContext(c.Context())
	accessToken, err := h.authService.SelectWedding(c.Context(), adminID, weddingID)
	if err != nil {
		return SelectWeddingResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return SelectWeddingResponse{AccessToken: accessToken}, nil
}

func (h *AuthHandler) Refresh(c fuego.ContextWithBody[RefreshRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	result, err := h.authService.Refresh(c.Context(), body.RefreshToken)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Role:         result.Role,
		Name:         result.Name,
		Weddings:     weddingInfos(result.Weddings),
	}, nil
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
	existing, _ := h.adminRepo.FindByEmail(body.Email)
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
	if err := h.adminRepo.Create(admin); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to create account"}
	}
	return map[string]any{"id": admin.ID.String(), "email": admin.Email, "name": admin.Name, "role": admin.Role}, nil
}
