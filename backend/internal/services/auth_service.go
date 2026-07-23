package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AccessClaims struct {
	AdminID   uint   `json:"sub"`
	WeddingID *uint  `json:"wid,omitempty"`
	Role      string `json:"role"`
	JTI       string `json:"jti"`
	IAT       int64  `json:"iat"`
	EXP       int64  `json:"exp"`
	jwt.RegisteredClaims
}

type AuthService struct {
	adminRepo *repository.AdminRepo
	tokenRepo *repository.TokenRepo
	secret    []byte
}

func NewAuthService(adminRepo *repository.AdminRepo, tokenRepo *repository.TokenRepo, secret string) *AuthService {
	return &AuthService{
		adminRepo: adminRepo,
		tokenRepo: tokenRepo,
		secret:    []byte(secret),
	}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, string, error) {
	admin, err := s.adminRepo.FindByEmail(email)
	if err != nil {
		return "", "", errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err != nil {
		return "", "", errors.New("invalid credentials")
	}
	accessToken, err := s.generateAccessToken(admin)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := s.generateRefreshToken(admin.ID)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string) (string, string, error) {
	token, err := s.tokenRepo.FindByToken(refreshTokenStr)
	if err != nil {
		return "", "", errors.New("invalid refresh token")
	}
	s.tokenRepo.DeleteByToken(refreshTokenStr)
	admin, err := s.adminRepo.FindByID(token.AdminID)
	if err != nil {
		return "", "", errors.New("admin not found")
	}
	accessToken, err := s.generateAccessToken(admin)
	if err != nil {
		return "", "", err
	}
	newRefreshToken, err := s.generateRefreshToken(admin.ID)
	if err != nil {
		return "", "", err
	}
	return accessToken, newRefreshToken, nil
}

func (s *AuthService) Logout(refreshToken string) error {
	return s.tokenRepo.DeleteByToken(refreshToken)
}

func (s *AuthService) ValidateToken(tokenStr string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *AuthService) generateAccessToken(admin *models.AdminUser) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		AdminID:   admin.ID,
		WeddingID: admin.WeddingID,
		Role:      admin.Role,
		JTI:       uuid.New().String(),
		IAT:       now.Unix(),
		EXP:       now.Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *AuthService) generateRefreshToken(adminID uint) (string, error) {
	b := make([]byte, 32)
	rand.Read(b)
	tokenStr := hex.EncodeToString(b)
	token := &models.RefreshToken{
		AdminID:   adminID,
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.tokenRepo.Save(token); err != nil {
		return "", err
	}
	return tokenStr, nil
}
