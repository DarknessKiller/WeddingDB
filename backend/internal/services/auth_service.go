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
	AdminID   uuid.UUID  `json:"sub"`
	WeddingID *uuid.UUID `json:"wid,omitempty"`
	Role      string     `json:"role"`
	JTI       string     `json:"jti"`
	IAT       int64      `json:"iat"`
	EXP       int64      `json:"exp"`
	jwt.RegisteredClaims
}

type AuthService struct {
	adminRepo   *repository.AdminRepo
	weddingRepo *repository.WeddingRepo
	tokenRepo   *repository.TokenRepo
	secret      []byte
}

func NewAuthService(adminRepo *repository.AdminRepo, weddingRepo *repository.WeddingRepo, tokenRepo *repository.TokenRepo, secret string) *AuthService {
	return &AuthService{
		adminRepo:   adminRepo,
		weddingRepo: weddingRepo,
		tokenRepo:   tokenRepo,
		secret:      []byte(secret),
	}
}

// LoginResult holds the data returned by Login.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	Role         string
	Name         string
	Weddings     []models.WeddingEvent
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	admin, err := s.adminRepo.FindByEmail(email)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	accessToken, err := s.generateAccessToken(admin, nil)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.generateRefreshToken(admin.ID)
	if err != nil {
		return nil, err
	}
	var weddings []models.WeddingEvent
	if admin.Role == "admin" {
		weddings, _ = s.weddingRepo.List()
	} else {
		weddings, _ = s.adminRepo.GetUserWeddings(admin.ID)
	}
	if weddings == nil {
		weddings = []models.WeddingEvent{}
	}
	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Role:         admin.Role,
		Name:         admin.Name,
		Weddings:     weddings,
	}, nil
}

// SelectWedding generates a new access token with the selected wedding embedded.
func (s *AuthService) SelectWedding(ctx context.Context, adminID uuid.UUID, weddingID uuid.UUID) (string, error) {
	admin, err := s.adminRepo.FindByID(adminID)
	if err != nil {
		return "", errors.New("admin not found")
	}
	// Verify access
	if admin.Role != "admin" {
		hasAccess, err := s.adminRepo.HasWeddingAccess(adminID, weddingID)
		if err != nil || !hasAccess {
			return "", errors.New("no access to this wedding")
		}
	}
	return s.generateAccessToken(admin, &weddingID)
}

// Refresh returns a new LoginResult with fresh tokens.
func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string) (*LoginResult, error) {
	token, err := s.tokenRepo.FindByToken(refreshTokenStr)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	admin, err := s.adminRepo.FindByID(token.AdminID)
	if err != nil {
		return nil, errors.New("admin not found")
	}
	// For refresh, keep the existing wedding context from the old token
	accessToken, err := s.generateAccessToken(admin, nil)
	if err != nil {
		return nil, err
	}
	newRefreshToken, err := s.generateRefreshToken(admin.ID)
	if err != nil {
		return nil, err
	}
	s.tokenRepo.DeleteByToken(refreshTokenStr)
	var weddings []models.WeddingEvent
	if admin.Role == "admin" {
		weddings, _ = s.weddingRepo.List()
	} else {
		weddings, _ = s.adminRepo.GetUserWeddings(admin.ID)
	}
	if weddings == nil {
		weddings = []models.WeddingEvent{}
	}
	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		Role:         admin.Role,
		Name:         admin.Name,
		Weddings:     weddings,
	}, nil
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

func (s *AuthService) generateAccessToken(admin *models.AdminUser, weddingID *uuid.UUID) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		AdminID:   admin.ID,
		WeddingID: weddingID,
		Role:      admin.Role,
		JTI:       uuid.New().String(),
		IAT:       now.Unix(),
		EXP:       now.Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *AuthService) generateRefreshToken(adminID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tokenStr := hex.EncodeToString(b)
	token := &models.RefreshToken{
		ID:        uuid.New(),
		AdminID:   adminID,
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.tokenRepo.Save(token); err != nil {
		return "", err
	}
	return tokenStr, nil
}
