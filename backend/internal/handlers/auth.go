package handlers

import (
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type AuthHandler struct{ authService *services.AuthService }

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
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
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *AuthHandler) Login(c fuego.ContextWithBody[LoginRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	access, refresh, role, name, err := h.authService.Login(c.Context(), body.Email, body.Password)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{AccessToken: access, RefreshToken: refresh, Role: role, Name: name}, nil
}

func (h *AuthHandler) Refresh(c fuego.ContextWithBody[RefreshRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	access, refresh, role, name, err := h.authService.Refresh(c.Context(), body.RefreshToken)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{AccessToken: access, RefreshToken: refresh, Role: role, Name: name}, nil
}

func (h *AuthHandler) Logout(c fuego.ContextWithBody[RefreshRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	h.authService.Logout(body.RefreshToken)
	return nil, nil
}
