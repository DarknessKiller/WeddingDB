package handlers

import (
	"weddingdb/internal/middleware"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

// RegisterRoutes registers all API routes on the Fuego server.
func RegisterRoutes(
	s *fuego.Server,
	authService *services.AuthService,
	guestService *services.GuestService,
	tableService *services.TableService,
	weddingService *services.WeddingService,
	adminRepo *repository.AdminRepo,
	nonceStore *middleware.NonceStore,
) {
	authHandler := NewAuthHandler(authService)
	adminHandler := NewAdminHandler(adminRepo)
	guestHandler := NewGuestHandler(guestService)
	tableHandler := NewTableHandler(tableService)
	weddingHandler := NewWeddingHandler(weddingService)

	// Auth routes (public)
	fuego.Post(s, "/api/auth/login", authHandler.Login)
	fuego.Post(s, "/api/auth/refresh", authHandler.Refresh)
	fuego.Post(s, "/api/auth/logout", authHandler.Logout)

	// Protected routes
	fuego.Use(s, middleware.AuthMiddleware(authService, nonceStore))

	// Service admin routes
	fuego.Get(s, "/api/admins", adminHandler.List)
	fuego.Post(s, "/api/admins", adminHandler.Create)
	fuego.Delete(s, "/api/admins/{id}", adminHandler.Delete)

	// Wedding routes
	fuego.Get(s, "/api/weddings", weddingHandler.List)
	fuego.Post(s, "/api/weddings", weddingHandler.Create)
	fuego.Get(s, "/api/weddings/{id}", weddingHandler.Get)
	fuego.Put(s, "/api/weddings/{id}", weddingHandler.Update)
	fuego.Delete(s, "/api/weddings/{id}", weddingHandler.Delete)

	// Wedding-scoped routes
	fuego.Use(s, middleware.WeddingScopeMiddleware)

	// Tables
	fuego.Get(s, "/api/weddings/{wid}/tables", tableHandler.List)
	fuego.Post(s, "/api/weddings/{wid}/tables", tableHandler.Create)
	fuego.Put(s, "/api/weddings/{wid}/tables/{id}", tableHandler.Update)
	fuego.Delete(s, "/api/weddings/{wid}/tables/{id}", tableHandler.Delete)

	// Guests
	fuego.Get(s, "/api/weddings/{wid}/guests", guestHandler.List)
	fuego.Post(s, "/api/weddings/{wid}/guests", guestHandler.Create)
	fuego.Get(s, "/api/weddings/{wid}/guests/{id}", guestHandler.Get)
	fuego.Put(s, "/api/weddings/{wid}/guests/{id}", guestHandler.Update)
	fuego.Delete(s, "/api/weddings/{wid}/guests/{id}", guestHandler.Delete)
	fuego.Post(s, "/api/weddings/{wid}/guests/{id}/checkin", guestHandler.CheckIn)
	fuego.Post(s, "/api/weddings/{wid}/guests/{id}/checkout", guestHandler.CheckOut)
	fuego.Post(s, "/api/weddings/{wid}/guests/{id}/seat", guestHandler.AssignSeat)
	fuego.Get(s, "/api/weddings/{wid}/guests/search", guestHandler.Search)
	fuego.Get(s, "/api/weddings/{wid}/occupancy", guestHandler.Occupancy)
}
