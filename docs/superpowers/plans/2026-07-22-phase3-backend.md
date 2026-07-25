# Phase 3 Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Go REST API with Fuego, GORM + PostgreSQL, Redis for nonce, JWT auth with two admin roles, multi-tenant isolation.

**Architecture:** Layered Handler → Service → Repository. Fuego for HTTP, GORM for PostgreSQL, Redis for nonce/replay prevention. Bun serves SvelteKit SSR frontend calling Go API.

**Tech Stack:** Go 1.22+, Fuego, GORM, PostgreSQL, Redis, bcrypt, JWT (golang-jwt/jwt/v5), Google UUID

## Global Constraints

- Go 1.22+ (match actual_helper)
- Fuego for HTTP handlers
- GORM for PostgreSQL
- Redis for nonce storage (auto TTL)
- JWT HS256 with `JWT_SECRET` env
- bcrypt for password hashing
- All IDs base64-encoded in API responses (json:"-" on uint IDs)
- JSON tags short: n=name, e=email, x=pax, etc.
- Struct names explicit: WeddingEvent, AdminUser, BanquetTable, GuestRecord
- Handler → Service → Repository layered pattern
- Follow actual_helper structure

---

## Task 1: Go Module + Dependencies

**Files:**
- Create: `backend/go.mod`
- Create: `backend/.env.example`

**Interfaces:**
- Produces: Go module ready for imports

- [ ] **Step 1: Initialize Go module**

```bash
mkdir -p backend/cmd/server backend/internal/{bootstrap,config,handlers,middleware,models,repository,services,utils}
cd backend
go mod init weddingdb
```

- [ ] **Step 2: Install dependencies**

```bash
cd backend
go get github.com/go-fuego/fuego
go get gorm.io/gorm
go get gorm.io/driver/postgres
go get github.com/redis/go-redis/v9
go get golang.org/x/crypto
go get github.com/golang-jwt/jwt/v5
go get github.com/google/uuid
go get github.com/joho/godotenv
```

- [ ] **Step 3: Create .env.example**

```bash
cat > backend/.env.example << 'EOF'
DATABASE_URL=postgresql://user:pass@localhost:5432/weddingdb
REDIS_URL=redis://localhost:6379
JWT_SECRET=change-me-to-a-random-secret
PORT=8080
EOF
```

- [ ] **Step 4: Verify dependencies**

```bash
cd backend && go mod tidy
```

Expected: no errors, go.sum generated

- [ ] **Step 5: Commit**

```bash
cd backend && git add go.mod go.sum .env.example
git commit -m "chore: init go module with dependencies"
```

---

## Task 2: Config Package

**Files:**
- Create: `backend/internal/config/config.go`
- Create: `backend/internal/config/server.go`

**Interfaces:**
- Produces: `config.Env` struct, `config.LoadEnv()`, `config.NewFuegoServer()`

- [ ] **Step 1: Create config.go**

```go
package config

import (
	"os"
	"github.com/joho/godotenv"
)

type Env struct {
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	Port        string
}

func LoadEnv() Env {
	godotenv.Load()
	return Env{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        getEnv("PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Create server.go**

```go
package config

import (
	"github.com/go-fuego/fuego"
)

func NewFuegoServer(env Env) *fuego.Server {
	return fuego.NewServer(
		":" + env.Port,
	)
}
```

- [ ] **Step 3: Commit**

```bash
cd backend && git add internal/config/
git commit -m "feat(config): env loading and fuego server factory"
```

---

## Task 3: Utils Package

**Files:**
- Create: `backend/internal/utils/encoding.go`

**Interfaces:**
- Produces: `utils.EncodeID(uint) string`, `utils.DecodeID(string) (uint, error)`

- [ ] **Step 1: Create encoding.go**

```go
package utils

import (
	"encoding/base64"
	"strconv"
)

func EncodeID(id uint) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(id), 10)))
}

func DecodeID(encoded string) (uint, error) {
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
```

- [ ] **Step 2: Commit**

```bash
cd backend && git add internal/utils/
git commit -m "feat(utils): base64 ID encoding/decoding"
```

---

## Task 4: Models

**Files:**
- Create: `backend/internal/models/wedding_event.go`
- Create: `backend/internal/models/admin_user.go`
- Create: `backend/internal/models/banquet_table.go`
- Create: `backend/internal/models/guest_record.go`
- Create: `backend/internal/models/refresh_token.go`

**Interfaces:**
- Produces: All GORM model structs

- [ ] **Step 1: Create wedding_event.go**

```go
package models

import "time"

type WeddingEvent struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	Name      string    `gorm:"size:255;not null" json:"n"`
	Date      time.Time `json:"d"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
}
```

- [ ] **Step 2: Create admin_user.go**

```go
package models

import "time"

type AdminUser struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	WeddingID *uint     `gorm:"index" json:"-"`
	Email     string    `gorm:"size:255;not null" json:"e"`
	Password  string    `gorm:"size:255;not null" json:"-"`
	Name      string    `gorm:"size:255" json:"n"`
	Role      string    `gorm:"size:20;not null" json:"rl"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
}
```

- [ ] **Step 3: Create banquet_table.go**

```go
package models

import "time"

type BanquetTable struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	WeddingID uint      `gorm:"index;not null" json:"-"`
	Name      string    `gorm:"size:100;not null" json:"n"`
	Capacity  int       `gorm:"not null" json:"cap"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	IsVip     bool      `json:"v"`
	Zone      string    `gorm:"size:20" json:"z"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
}
```

- [ ] **Step 4: Create guest_record.go**

```go
package models

import "time"

type GuestRecord struct {
	ID          uint       `gorm:"primaryKey" json:"-"`
	WeddingID   uint       `gorm:"index;not null" json:"-"`
	Name        string     `gorm:"size:255;not null" json:"n"`
	Phone       string     `gorm:"size:50" json:"p"`
	Email       string     `gorm:"size:255" json:"e"`
	Pax         int        `gorm:"not null;default:1" json:"x"`
	TableID     *uint      `gorm:"index" json:"-"`
	SeatNum     *int       json:"-"`
	RSVP        string     `gorm:"size:20;default:no_response" json:"r"`
	CheckedInAt *time.Time `json:"cia"`
	Notes       string     `gorm:"type:text" json:"nt"`
	Dietary     []string   `gorm:"type:text[]" json:"d"`
	IsVip       bool       `json:"v"`
	AngbaoAmt   *int       `json:"a"`
	GiftItem    *string    `gorm:"size:255" json:"g"`
	CreatedAt   time.Time  `json:"c"`
	UpdatedAt   time.Time  `json:"u"`
}
```

- [ ] **Step 5: Create refresh_token.go**

```go
package models

import "time"

type RefreshToken struct {
	ID        uint      `gorm:"primaryKey"`
	AdminID   uint      `gorm:"index;not null"`
	Token     string    `gorm:"size:255;uniqueIndex;not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
}
```

- [ ] **Step 6: Commit**

```bash
cd backend && git add internal/models/
git commit -m "feat(models): GORM models for wedding, admin, table, guest, refresh token"
```

---

## Task 5: Repository Layer

**Files:**
- Create: `backend/internal/repository/admin_repo.go`
- Create: `backend/internal/repository/wedding_repo.go`
- Create: `backend/internal/repository/table_repo.go`
- Create: `backend/internal/repository/guest_repo.go`
- Create: `backend/internal/repository/token_repo.go`

**Interfaces:**
- Produces: Repository structs with GORM query methods

- [ ] **Step 1: Create admin_repo.go**

```go
package repository

import "weddingdb/internal/models"

type AdminRepo struct{ db *gorm.DB }

func NewAdminRepo(db *gorm.DB) *AdminRepo {
	return &AdminRepo{db: db}
}

func (r *AdminRepo) FindByEmail(email string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.Where("email = ?", email).First(&admin).Error
	return &admin, err
}

func (r *AdminRepo) FindByID(id uint) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.First(&admin, id).Error
	return &admin, err
}

func (r *AdminRepo) Create(admin *models.AdminUser) error {
	return r.db.Create(admin).Error
}

func (r *AdminRepo) List() ([]models.AdminUser, error) {
	var admins []models.AdminUser
	err := r.db.Find(&admins).Error
	return admins, err
}

func (r *AdminRepo) Update(admin *models.AdminUser) error {
	return r.db.Save(admin).Error
}

func (r *AdminRepo) Delete(id uint) error {
	return r.db.Delete(&models.AdminUser{}, id).Error
}
```

- [ ] **Step 2: Create wedding_repo.go**

```go
package repository

import "weddingdb/internal/models"

type WeddingRepo struct{ db *gorm.DB }

func NewWeddingRepo(db *gorm.DB) *WeddingRepo {
	return &WeddingRepo{db: db}
}

func (r *WeddingRepo) FindByID(id uint) (*models.WeddingEvent, error) {
	var w models.WeddingEvent
	err := r.db.First(&w, id).Error
	return &w, err
}

func (r *WeddingRepo) List() ([]models.WeddingEvent, error) {
	var weddings []models.WeddingEvent
	err := r.db.Find(&weddings).Error
	return weddings, err
}

func (r *WeddingRepo) Create(w *models.WeddingEvent) error {
	return r.db.Create(w).Error
}

func (r *WeddingRepo) Update(w *models.WeddingEvent) error {
	return r.db.Save(w).Error
}

func (r *WeddingRepo) Delete(id uint) error {
	return r.db.Delete(&models.WeddingEvent{}, id).Error
}
```

- [ ] **Step 3: Create table_repo.go**

```go
package repository

import "weddingdb/internal/models"

type TableRepo struct{ db *gorm.DB }

func NewTableRepo(db *gorm.DB) *TableRepo {
	return &TableRepo{db: db}
}

func (r *TableRepo) ListByWedding(weddingID uint) ([]models.BanquetTable, error) {
	var tables []models.BanquetTable
	err := r.db.Where("wedding_id = ?", weddingID).Find(&tables).Error
	return tables, err
}

func (r *TableRepo) FindByID(id, weddingID uint) (*models.BanquetTable, error) {
	var t models.BanquetTable
	err := r.db.Where("id = ? AND wedding_id = ?", id, weddingID).First(&t).Error
	return &t, err
}

func (r *TableRepo) Create(t *models.BanquetTable) error {
	return r.db.Create(t).Error
}

func (r *TableRepo) Update(t *models.BanquetTable) error {
	return r.db.Save(t).Error
}

func (r *TableRepo) Delete(id, weddingID uint) error {
	return r.db.Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.BanquetTable{}).Error
}
```

- [ ] **Step 4: Create guest_repo.go**

```go
package repository

import (
	"fmt"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type GuestRepo struct{ db *gorm.DB }

func NewGuestRepo(db *gorm.DB) *GuestRepo {
	return &GuestRepo{db: db}
}

func (r *GuestRepo) ListByWedding(weddingID uint, offset, limit int) ([]models.GuestRecord, int64, error) {
	var guests []models.GuestRecord
	var total int64
	r.db.Model(&models.GuestRecord{}).Where("wedding_id = ?", weddingID).Count(&total)
	err := r.db.Where("wedding_id = ?", weddingID).Offset(offset).Limit(limit).Find(&guests).Error
	return guests, total, err
}

func (r *GuestRepo) FindByID(id, weddingID uint) (*models.GuestRecord, error) {
	var g models.GuestRecord
	err := r.db.Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error
	return &g, err
}

func (r *GuestRepo) SearchByWedding(weddingID uint, query string) ([]models.GuestRecord, error) {
	var guests []models.GuestRecord
	q := fmt.Sprintf("%%%s%%", query)
	err := r.db.Where("wedding_id = ? AND (name ILIKE ? OR phone ILIKE ? OR email ILIKE ?)",
		weddingID, q, q, q).Limit(20).Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) Create(g *models.GuestRecord) error {
	return r.db.Create(g).Error
}

func (r *GuestRepo) Update(g *models.GuestRecord) error {
	return r.db.Save(g).Error
}

func (r *GuestRepo) Delete(id, weddingID uint) error {
	return r.db.Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.GuestRecord{}).Error
}

func (r *GuestRepo) TableOccupancy(weddingID uint) ([]TableOccupancy, error) {
	type row struct {
		TableID uint
		Pax     int
	}
	var rows []row
	err := r.db.Model(&models.GuestRecord{}).
		Select("table_id, SUM(pax) as pax").
		Where("wedding_id = ? AND table_id IS NOT NULL", weddingID).
		Group("table_id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	var result []TableOccupancy
	for _, r := range rows {
		result = append(result, TableOccupancy{TableID: r.TableID, Pax: r.Pax})
	}
	return result, nil
}

type TableOccupancy struct {
	TableID uint
	Pax     int
}
```

- [ ] **Step 5: Create token_repo.go**

```go
package repository

import (
	"time"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type TokenRepo struct{ db *gorm.DB }

func NewTokenRepo(db *gorm.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

func (r *TokenRepo) Save(token *models.RefreshToken) error {
	return r.db.Create(token).Error
}

func (r *TokenRepo) FindByToken(token string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := r.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&t).Error
	return &t, err
}

func (r *TokenRepo) DeleteByToken(token string) error {
	return r.db.Where("token = ?", token).Delete(&models.RefreshToken{}).Error
}

func (r *TokenRepo) DeleteExpired() error {
	return r.db.Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{}).Error
}
```

- [ ] **Step 6: Commit**

```bash
cd backend && git add internal/repository/
git commit -m "feat(repository): GORM repos for admin, wedding, table, guest, token"
```

---

## Task 6: Nonce Store (Redis)

**Files:**
- Create: `backend/internal/middleware/nonce.go`

**Interfaces:**
- Produces: `NonceStore` struct with `MarkUsed()`, `IsUsed()`

- [ ] **Step 1: Create nonce.go**

```go
package middleware

import (
	"context"
	"time"
	"github.com/redis/go-redis/v9"
)

type NonceStore struct {
	client *redis.Client
}

func NewNonceStore(client *redis.Client) *NonceStore {
	return &NonceStore{client: client}
}

func (s *NonceStore) MarkUsed(ctx context.Context, jti string, ttl time.Duration) error {
	return s.client.Set(ctx, "nonce:"+jti, "1", ttl).Err()
}

func (s *NonceStore) IsUsed(ctx context.Context, jti string) bool {
	exists, err := s.client.Exists(ctx, "nonce:"+jti).Result()
	return err == nil && exists > 0
}
```

- [ ] **Step 2: Commit**

```bash
cd backend && git add internal/middleware/nonce.go
git commit -m "feat(middleware): Redis nonce store for replay prevention"
```

---

## Task 7: Auth Service

**Files:**
- Create: `backend/internal/services/auth_service.go`

**Interfaces:**
- Produces: `AuthService` with `Login()`, `Refresh()`, `Logout()`, `ValidateToken()`

- [ ] **Step 1: Create auth_service.go**

```go
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
	RegisteredClaims
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
```

- [ ] **Step 2: Commit**

```bash
cd backend && git add internal/services/auth_service.go
git commit -m "feat(services): auth service with JWT, bcrypt, refresh token rotation"
```

---

## Task 8: Guest Service

**Files:**
- Create: `backend/internal/services/guest_service.go`

**Interfaces:**
- Produces: `GuestService` with CRUD + check-in + seat validation

- [ ] **Step 1: Create guest_service.go**

```go
package services

import (
	"errors"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type GuestService struct {
	guestRepo *repository.GuestRepo
	tableRepo *repository.TableRepo
}

func NewGuestService(guestRepo *repository.GuestRepo, tableRepo *repository.TableRepo) *GuestService {
	return &GuestService{guestRepo: guestRepo, tableRepo: tableRepo}
}

func (s *GuestService) List(weddingID uint, offset, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(weddingID, offset, limit)
}

func (s *GuestService) Get(id, weddingID uint) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(id, weddingID)
}

func (s *GuestService) Create(g *models.GuestRecord) error {
	return s.guestRepo.Create(g)
}

func (s *GuestService) Update(g *models.GuestRecord) error {
	return s.guestRepo.Update(g)
}

func (s *GuestService) Delete(id, weddingID uint) error {
	return s.guestRepo.Delete(id, weddingID)
}

func (s *GuestService) Search(weddingID uint, query string) ([]models.GuestRecord, error) {
	return s.guestRepo.SearchByWedding(weddingID, query)
}

func (s *GuestService) AssignSeat(guestID, weddingID, tableID uint, seatNum int) error {
	guest, err := s.guestRepo.FindByID(guestID, weddingID)
	if err != nil {
		return err
	}
	table, err := s.tableRepo.FindByID(tableID, weddingID)
	if err != nil {
		return errors.New("table not found")
	}
	if seatNum < 1 || seatNum+guest.Pax-1 > table.Capacity {
		return errors.New("seat range exceeds table capacity")
	}
	guest.TableID = &tableID
	guest.SeatNum = &seatNum
	return s.guestRepo.Update(guest)
}

func (s *GuestService) CheckIn(id, weddingID uint) error {
	guest, err := s.guestRepo.FindByID(id, weddingID)
	if err != nil {
		return err
	}
	now := time.Now()
	guest.CheckedInAt = &now
	return s.guestRepo.Update(guest)
}

func (s *GuestService) CheckOut(id, weddingID uint) error {
	guest, err := s.guestRepo.FindByID(id, weddingID)
	if err != nil {
		return err
	}
	guest.CheckedInAt = nil
	return s.guestRepo.Update(guest)
}

func (s *GuestService) Occupancy(weddingID uint) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(weddingID)
}
```

- [ ] **Step 2: Commit**

```bash
cd backend && git add internal/services/guest_service.go
git commit -m "feat(services): guest service with seat validation and check-in/out"
```

---

## Task 9: Table + Wedding Services

**Files:**
- Create: `backend/internal/services/table_service.go`
- Create: `backend/internal/services/wedding_service.go`

**Interfaces:**
- Produces: `TableService`, `WeddingService`

- [ ] **Step 1: Create table_service.go**

```go
package services

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type TableService struct {
	tableRepo *repository.TableRepo
}

func NewTableService(tableRepo *repository.TableRepo) *TableService {
	return &TableService{tableRepo: tableRepo}
}

func (s *TableService) List(weddingID uint) ([]models.BanquetTable, error) {
	return s.tableRepo.ListByWedding(weddingID)
}

func (s *TableService) Get(id, weddingID uint) (*models.BanquetTable, error) {
	return s.tableRepo.FindByID(id, weddingID)
}

func (s *TableService) Create(t *models.BanquetTable) error {
	return s.tableRepo.Create(t)
}

func (s *TableService) Update(t *models.BanquetTable) error {
	return s.tableRepo.Update(t)
}

func (s *TableService) Delete(id, weddingID uint) error {
	return s.tableRepo.Delete(id, weddingID)
}
```

- [ ] **Step 2: Create wedding_service.go**

```go
package services

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type WeddingService struct {
	weddingRepo *repository.WeddingRepo
}

func NewWeddingService(weddingRepo *repository.WeddingRepo) *WeddingService {
	return &WeddingService{weddingRepo: weddingRepo}
}

func (s *WeddingService) List() ([]models.WeddingEvent, error) {
	return s.weddingRepo.List()
}

func (s *WeddingService) Get(id uint) (*models.WeddingEvent, error) {
	return s.weddingRepo.FindByID(id)
}

func (s *WeddingService) Create(w *models.WeddingEvent) error {
	return s.weddingRepo.Create(w)
}

func (s *WeddingService) Update(w *models.WeddingEvent) error {
	return s.weddingRepo.Update(w)
}

func (s *WeddingService) Delete(id uint) error {
	return s.weddingRepo.Delete(id)
}
```

- [ ] **Step 3: Commit**

```bash
cd backend && git add internal/services/table_service.go internal/services/wedding_service.go
git commit -m "feat(services): table and wedding services"
```

---

## Task 10: Middleware

**Files:**
- Create: `backend/internal/middleware/auth.go`
- Create: `backend/internal/middleware/wedding_scope.go`
- Create: `backend/internal/middleware/cors.go`

**Interfaces:**
- Produces: `AuthMiddleware()`, `NonceMiddleware()`, `WeddingScopeMiddleware`, `CORSMiddleware`

- [ ] **Step 1: Create auth.go**

```go
package middleware

import (
	"net/http"
	"strings"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

func AuthMiddleware(authService *services.AuthService, nonceStore *NonceStore) func(fuego.Handler) fuego.Handler {
	return func(next fuego.Handler) fuego.Handler {
		return func(c *fuego.ContextWithBody[any]) (any, error) {
			token := extractBearer(c.Request())
			if token == "" {
				return nil, fuego.UnauthorizedError{Title: "Missing token"}
			}
			claims, err := authService.ValidateToken(token)
			if err != nil {
				return nil, fuego.UnauthorizedError{Title: "Invalid token"}
			}
			if nonceStore.IsUsed(c.Context(), claims.JTI) {
				return nil, fuego.UnauthorizedError{Title: "Token reused"}
			}
			ttl := time.Until(time.Unix(claims.EXP, 0))
			nonceStore.MarkUsed(c.Context(), claims.JTI, ttl)

			c.Set("adminId", claims.AdminID)
			c.Set("weddingId", claims.WeddingID)
			c.Set("role", claims.Role)
			return next(c)
		}
	}
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
```

- [ ] **Step 2: Create wedding_scope.go**

```go
package middleware

import (
	"weddingdb/internal/utils"
	"github.com/go-fuego/fuego"
)

func WeddingScopeMiddleware(next fuego.Handler) fuego.Handler {
	return func(c *fuego.ContextWithBody[any]) (any, error) {
		role := c.Get("role").(string)
		if role == "service_admin" {
			return next(c)
		}
		jwtWid := c.Get("weddingId").(*uint)
		if jwtWid == nil {
			return nil, fuego.ForbiddenError{Title: "No wedding scope"}
		}
		urlWid, err := utils.DecodeID(c.PathParam("wid"))
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
		}
		if *jwtWid != urlWid {
			return nil, fuego.ForbiddenError{Title: "Access denied"}
		}
		return next(c)
	}
}
```

- [ ] **Step 3: Create cors.go**

```go
package middleware

import "github.com/go-fuego/fuego"

func CORSMiddleware(next fuego.Handler) fuego.Handler {
	return func(c *fuego.ContextWithBody[any]) (any, error) {
		c.Response().Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		c.Response().Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Response().Header().Set("Access-Control-Allow-Credentials", "true")
		if c.Request().Method == "OPTIONS" {
			c.Response().WriteHeader(204)
			return nil, nil
		}
		return next(c)
	}
}
```

- [ ] **Step 4: Commit**

```bash
cd backend && git add internal/middleware/
git commit -m "feat(middleware): auth with nonce, wedding scope, CORS"
```

---

## Task 11: Handlers

**Files:**
- Create: `backend/internal/handlers/auth.go`
- Create: `backend/internal/handlers/guest.go`
- Create: `backend/internal/handlers/table.go`
- Create: `backend/internal/handlers/wedding.go`
- Create: `backend/internal/handlers/admin.go`
- Create: `backend/internal/handlers/register.go`

**Interfaces:**
- Produces: All Fuego handlers + route registration

- [ ] **Step 1: Create auth.go**

```go
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
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *AuthHandler) Login(c fuego.ContextWithBody[LoginRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	access, refresh, err := h.authService.Login(c.Context(), body.Email, body.Password)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{AccessToken: access, RefreshToken: refresh}, nil
}

func (h *AuthHandler) Refresh(c fuego.ContextWithBody[RefreshRequest]) (TokenResponse, error) {
	body, err := c.Body()
	if err != nil {
		return TokenResponse{}, fuego.BadRequestError{Title: "Invalid request"}
	}
	access, refresh, err := h.authService.Refresh(c.Context(), body.RefreshToken)
	if err != nil {
		return TokenResponse{}, fuego.UnauthorizedError{Title: err.Error()}
	}
	return TokenResponse{AccessToken: access, RefreshToken: refresh}, nil
}

func (h *AuthHandler) Logout(c fuego.ContextWithBody[RefreshRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	h.authService.Logout(body.RefreshToken)
	return nil, nil
}
```

- [ ] **Step 2: Create guest.go**

```go
package handlers

import (
	"weddingdb/internal/services"
	"weddingdb/internal/utils"
	"github.com/go-fuego/fuego"
)

type GuestHandler struct{ guestService *services.GuestService }

func NewGuestHandler(guestService *services.GuestService) *GuestHandler {
	return &GuestHandler{guestService: guestService}
}

type GuestCreateRequest struct {
	Name    string  `json:"n"`
	Phone   string  `json:"p"`
	Email   string  `json:"e"`
	Pax     int     `json:"x"`
	RSVP    string  `json:"r"`
	IsVip   bool    `json:"v"`
	Notes   string  `json:"nt"`
	Dietary []string `json:"d"`
}

func (h *GuestHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	guests, total, err := h.guestService.List(wid, 0, 100)
	if err != nil {
		return nil, err
	}
	return map[string]any{"guests": guests, "total": total}, nil
}

func (h *GuestHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	guest, err := h.guestService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	return guest, nil
}

func (h *GuestHandler) Create(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	guest := &models.GuestRecord{
		WeddingID: wid,
		Name:      body.Name,
		Phone:     body.Phone,
		Email:     body.Email,
		Pax:       body.Pax,
		RSVP:      body.RSVP,
		IsVip:     body.IsVip,
		Notes:     body.Notes,
		Dietary:   body.Dietary,
	}
	if err := h.guestService.Create(guest); err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) Update(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	guest, err := h.guestService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	guest.Name = body.Name
	guest.Phone = body.Phone
	guest.Email = body.Email
	guest.Pax = body.Pax
	guest.RSVP = body.RSVP
	guest.IsVip = body.IsVip
	guest.Notes = body.Notes
	guest.Dietary = body.Dietary
	if err := h.guestService.Update(guest); err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	if err := h.guestService.Delete(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) CheckIn(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	if err := h.guestService.CheckIn(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) CheckOut(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	if err := h.guestService.CheckOut(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) Search(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	query := c.QueryParam("q")
	guests, err := h.guestService.Search(wid, query)
	if err != nil {
		return nil, err
	}
	return guests, nil
}

func (h *GuestHandler) Occupancy(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	occ, err := h.guestService.Occupancy(wid)
	if err != nil {
		return nil, err
	}
	return occ, nil
}
```

- [ ] **Step 3: Create table.go**

```go
package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"
	"github.com/go-fuego/fuego"
)

type TableHandler struct{ tableService *services.TableService }

func NewTableHandler(tableService *services.TableService) *TableHandler {
	return &TableHandler{tableService: tableService}
}

type TableRequest struct {
	Name     string  `json:"n"`
	Capacity int     `json:"cap"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	IsVip    bool    `json:"v"`
	Zone     string  `json:"z"`
}

func (h *TableHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	tables, err := h.tableService.List(wid)
	if err != nil {
		return nil, err
	}
	return tables, nil
}

func (h *TableHandler) Create(c fuego.ContextWithBody[TableRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	table := &models.BanquetTable{
		WeddingID: wid,
		Name:      body.Name,
		Capacity:  body.Capacity,
		X:         body.X,
		Y:         body.Y,
		IsVip:     body.IsVip,
		Zone:      body.Zone,
	}
	if err := h.tableService.Create(table); err != nil {
		return nil, err
	}
	return table, nil
}

func (h *TableHandler) Update(c fuego.ContextWithBody[TableRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	table, err := h.tableService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Table not found"}
	}
	table.Name = body.Name
	table.Capacity = body.Capacity
	table.X = body.X
	table.Y = body.Y
	table.IsVip = body.IsVip
	table.Zone = body.Zone
	if err := h.tableService.Update(table); err != nil {
		return nil, err
	}
	return table, nil
}

func (h *TableHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	id, _ := utils.DecodeID(c.PathParam("id"))
	if err := h.tableService.Delete(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}
```

- [ ] **Step 4: Create wedding.go**

```go
package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"
	"github.com/go-fuego/fuego"
)

type WeddingHandler struct{ weddingService *services.WeddingService }

func NewWeddingHandler(weddingService *services.WeddingService) *WeddingHandler {
	return &WeddingHandler{weddingService: weddingService}
}

type WeddingRequest struct {
	Name string    `json:"n"`
	Date time.Time `json:"d"`
}

func (h *WeddingHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	return h.weddingService.List()
}

func (h *WeddingHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	id, _ := utils.DecodeID(c.PathParam("id"))
	return h.weddingService.Get(id)
}

func (h *WeddingHandler) Create(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w := &models.WeddingEvent{Name: body.Name, Date: body.Date}
	if err := h.weddingService.Create(w); err != nil {
		return nil, err
	}
	return w, nil
}

func (h *WeddingHandler) Update(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	id, _ := utils.DecodeID(c.PathParam("id"))
	w, err := h.weddingService.Get(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	w.Name = body.Name
	w.Date = body.Date
	if err := h.weddingService.Update(w); err != nil {
		return nil, err
	}
	return w, nil
}

func (h *WeddingHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	id, _ := utils.DecodeID(c.PathParam("id"))
	return nil, h.weddingService.Delete(id)
}
```

- [ ] **Step 5: Create admin.go**

```go
package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/utils"
	"github.com/go-fuego/fuego"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct{ adminRepo *repository.AdminRepo }

func NewAdminHandler(adminRepo *repository.AdminRepo) *AdminHandler {
	return &AdminHandler{adminRepo: adminRepo}
}

type AdminRequest struct {
	Email    string `json:"e"`
	Password string `json:"pw,omitempty"`
	Name     string `json:"n"`
	Role     string `json:"rl"`
	WeddingID *uint `json:"wid,omitempty"`
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

func (h *AdminHandler) Update(c fuego.ContextWithBody[AdminRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	id, _ := utils.DecodeID(c.PathParam("id"))
	admin, err := h.adminRepo.FindByID(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Admin not found"}
	}
	admin.Email = body.Email
	admin.Name = body.Name
	admin.Role = body.Role
	admin.WeddingID = body.WeddingID
	if body.Password != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		admin.Password = string(hash)
	}
	if err := h.adminRepo.Update(admin); err != nil {
		return nil, err
	}
	return admin, nil
}

func (h *AdminHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	id, _ := utils.DecodeID(c.PathParam("id"))
	return nil, h.adminRepo.Delete(id)
}
```

- [ ] **Step 6: Create register.go**

```go
package handlers

import (
	"weddingdb/internal/middleware"
	"weddingdb/internal/services"
	"github.com/go-fuego/fuego"
)

func RegisterRoutes(
	server *fuego.Server,
	authHandler *AuthHandler,
	adminHandler *AdminHandler,
	weddingHandler *WeddingHandler,
	tableHandler *TableHandler,
	guestHandler *GuestHandler,
	authService *services.AuthService,
	nonceStore *middleware.NonceStore,
) {
	auth := middleware.AuthMiddleware(authService, nonceStore)

	// Public
	fuego.Post(server, "/api/v1/auth/login", authHandler.Login)
	fuego.Post(server, "/api/v1/auth/refresh", authHandler.Refresh)
	fuego.Post(server, "/api/v1/auth/logout", authHandler.Logout)

	// Service Admin
	fuego.Use(server, auth)
	fuego.Get(server, "/api/v1/admins", adminHandler.List)
	fuego.Post(server, "/api/v1/admins", adminHandler.Create)
	fuego.Put(server, "/api/v1/admins/{id}", adminHandler.Update)
	fuego.Delete(server, "/api/v1/admins/{id}", adminHandler.Delete)

	fuego.Get(server, "/api/v1/weddings", weddingHandler.List)
	fuego.Post(server, "/api/v1/weddings", weddingHandler.Create)
	fuego.Get(server, "/api/v1/weddings/{id}", weddingHandler.Get)
	fuego.Put(server, "/api/v1/weddings/{id}", weddingHandler.Update)
	fuego.Delete(server, "/api/v1/weddings/{id}", weddingHandler.Delete)

	// Wedding Admin (scoped)
	fuego.Use(server, middleware.WeddingScopeMiddleware)
	fuego.Get(server, "/api/v1/weddings/{wid}/tables", tableHandler.List)
	fuego.Post(server, "/api/v1/weddings/{wid}/tables", tableHandler.Create)
	fuego.Put(server, "/api/v1/weddings/{wid}/tables/{id}", tableHandler.Update)
	fuego.Delete(server, "/api/v1/weddings/{wid}/tables/{id}", tableHandler.Delete)

	fuego.Get(server, "/api/v1/weddings/{wid}/guests", guestHandler.List)
	fuego.Post(server, "/api/v1/weddings/{wid}/guests", guestHandler.Create)
	fuego.Get(server, "/api/v1/weddings/{wid}/guests/{id}", guestHandler.Get)
	fuego.Put(server, "/api/v1/weddings/{wid}/guests/{id}", guestHandler.Update)
	fuego.Delete(server, "/api/v1/weddings/{wid}/guests/{id}", guestHandler.Delete)
	fuego.Post(server, "/api/v1/weddings/{wid}/guests/{id}/checkin", guestHandler.CheckIn)
	fuego.Post(server, "/api/v1/weddings/{wid}/guests/{id}/checkout", guestHandler.CheckOut)
	fuego.Get(server, "/api/v1/weddings/{wid}/guests/search", guestHandler.Search)
	fuego.Get(server, "/api/v1/weddings/{wid}/occupancy", guestHandler.Occupancy)
}
```

- [ ] **Step 7: Commit**

```bash
cd backend && git add internal/handlers/
git commit -m "feat(handlers): Fuego handlers for auth, admin, wedding, table, guest"
```

---

## Task 12: Bootstrap + main.go

**Files:**
- Create: `backend/internal/bootstrap/bootstrap.go`
- Create: `backend/cmd/server/main.go`

**Interfaces:**
- Produces: Application entry point, wires all dependencies

- [ ] **Step 1: Create bootstrap.go**

```go
package bootstrap

import (
	"context"
	"log"
	"weddingdb/internal/config"
	"weddingdb/internal/handlers"
	"weddingdb/internal/middleware"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type App struct {
	Server       *fuego.Server
	DB           *gorm.DB
	Redis        *redis.Client
	AuthService  *services.AuthService
	NonceStore   *middleware.NonceStore
	Handlers     *Handlers
}

type Handlers struct {
	Auth    *handlers.AuthHandler
	Admin   *handlers.AdminHandler
	Wedding *handlers.WeddingHandler
	Table   *handlers.TableHandler
	Guest   *handlers.GuestHandler
}

func Init(env config.Env) *App {
	db, err := gorm.Open(postgres.Open(env.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	db.AutoMigrate(
		&models.WeddingEvent{},
		&models.AdminUser{},
		&models.BanquetTable{},
		&models.GuestRecord{},
		&models.RefreshToken{},
	)

	rdb := redis.NewClient(&redis.Options{Addr: env.RedisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	adminRepo := repository.NewAdminRepo(db)
	weddingRepo := repository.NewWeddingRepo(db)
	tableRepo := repository.NewTableRepo(db)
	guestRepo := repository.NewGuestRepo(db)
	tokenRepo := repository.NewTokenRepo(db)

	authService := services.NewAuthService(adminRepo, tokenRepo, env.JWTSecret)
	nonceStore := middleware.NewNonceStore(rdb)
	tableService := services.NewTableService(tableRepo)
	guestService := services.NewGuestService(guestRepo, tableRepo)
	weddingService := services.NewWeddingService(weddingRepo)

	h := &Handlers{
		Auth:    handlers.NewAuthHandler(authService),
		Admin:   handlers.NewAdminHandler(adminRepo),
		Wedding: handlers.NewWeddingHandler(weddingService),
		Table:   handlers.NewTableHandler(tableService),
		Guest:   handlers.NewGuestHandler(guestService),
	}

	server := config.NewFuegoServer(env)

	return &App
		Server:      server,
		DB:          db,
		Redis:       rdb,
		AuthService: authService,
		NonceStore:  nonceStore,
		Handlers:    h,
	}
}
```

- [ ] **Step 2: Create main.go**

```go
package main

import (
	"log"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"
	"weddingdb/internal/handlers"
	"weddingdb/internal/middleware"
)

func main() {
	env := config.LoadEnv()
	app := bootstrap.Init(env)

	handlers.RegisterRoutes(
		app.Server,
		app.Handlers.Auth,
		app.Handlers.Admin,
		app.Handlers.Wedding,
		app.Handlers.Table,
		app.Handlers.Guest,
		app.AuthService,
		app.NonceStore,
	)

	fuego.Use(app.Server, middleware.CORSMiddleware)

	log.Printf("Server starting on :%s", env.Port)
	if err := app.Server.Run(); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 3: Verify build**

```bash
cd backend && go build ./cmd/server/
```

Expected: no errors

- [ ] **Step 4: Commit**

```bash
cd backend && git add internal/bootstrap/ cmd/server/
git commit -m "feat(bootstrap): wire dependencies and entry point"
```

---

## Task 13: Frontend Auth Store + API Client

**Files:**
- Modify: `frontend/src/lib/stores/index.ts`
- Create: `frontend/src/lib/api/client.ts`

**Interfaces:**
- Produces: `auth` store, `apiFetch()` function

- [ ] **Step 1: Update stores/index.ts**

```ts
// Add auth state
let accessToken = $state<string | null>(null);
let refreshToken = $state<string | null>(null);
let role = $state<string | null>(null);

export function getAuth() {
    return { accessToken, refreshToken, role };
}

export function setAuth(access: string, refresh: string, r: string) {
    accessToken = access;
    refreshToken = refresh;
    role = r;
}

export function clearAuth() {
    accessToken = null;
    refreshToken = null;
    role = null;
}
```

- [ ] **Step 2: Create api/client.ts**

```ts
import { getAuth, setAuth } from '$lib/stores';

const BASE = 'http://localhost:8080';

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
    const { accessToken } = getAuth();
    let res = await fetch(`${BASE}${path}`, {
        ...opts,
        headers: {
            'Content-Type': 'application/json',
            ...opts.headers,
            ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
        },
    });
    if (res.status === 401) {
        const { refreshToken } = getAuth();
        if (refreshToken) {
            const refreshRes = await fetch(`${BASE}/api/v1/auth/refresh`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refreshToken }),
            });
            if (refreshRes.ok) {
                const data = await refreshRes.json();
                setAuth(data.accessToken, data.refreshToken, getAuth().role);
                res = await fetch(`${BASE}${path}`, {
                    ...opts,
                    headers: {
                        'Content-Type': 'application/json',
                        ...opts.headers,
                        Authorization: `Bearer ${data.accessToken}`,
                    },
                });
            }
        }
    }
    return res;
}
```

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/lib/stores/index.ts src/lib/api/
git commit -m "feat(frontend): auth store and API client with token refresh"
```

---

## Task 14: Remove localStorage Table Logic

**Files:**
- Modify: `frontend/src/lib/constants/index.ts`

**Interfaces:**
- Removes: `getTableDefinitions()`, `setTableDefinitions()`, localStorage logic
- Produces: Static `DEFAULT_TABLES` export

- [ ] **Step 1: Simplify constants/index.ts**

Remove all localStorage logic. Export only static defaults. API fetches tables from backend.

```ts
export const DEFAULT_TABLES: BanquetTable[] = [
    // ... same table definitions ...
];
```

- [ ] **Step 2: Commit**

```bash
cd frontend && git add src/lib/constants/index.ts
git commit -m "refactor(constants): remove localStorage table logic, use API"
```

---

## Task 15: Final Cleanup + README Update

**Files:**
- Modify: `README.md`
- Modify: `.gitignore`

- [ ] **Step 1: Update README.md**

Add backend section with setup instructions.

- [ ] **Step 2: Update .gitignore**

Add `backend/.env`

- [ ] **Step 3: Commit**

```bash
git add README.md .gitignore
git commit -m "docs: update README with backend setup, add .env to gitignore"
```
