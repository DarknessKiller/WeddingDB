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
	AccessToken         string
