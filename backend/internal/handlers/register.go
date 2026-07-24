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
	authHandler := NewAuthHandler(authService, adminRepo)
	adminHandler := NewAdminHandler(adminRepo)
	guestHandler := NewGuestHandler(guestService)
	tableHandler := NewTableHandler(tableService)
	weddingHandler := NewWeddingHandler(weddingService, adminRepo)

	// ── Public (no middleware) ──
	pub := fuego.Group(s, "/api/auth")
	fuego.Post(pub, "/login", authHandler.Login)
	fuego.Post(pub, "/refresh", authHandler.Refresh)
	fuego.Post(pub, "/logout", authHandler.Logout)
	fuego.Post(pub, "/register", authHandler.Register)

	// ── Public guest endpoints (kiosk, no auth) ──
	pubApi := fuego.Group(s, "/api/public")
	pubGuestHandler := NewPublicGuestHandler(guestService)
	pubScoped := fuego.Group(pubApi, "/weddings/{wid}")
	fuego.Get(pubScoped, "/guests", pubGuestHandler.List)
	fuego.Get(pubScoped, "/guests/search", pubGuestHandler.Search)
	fuego.Get(pubScoped, "/tables", tableHandler.List)

	// ── Auth-protected ──
	api := fuego.Group(s, "/api")
	fuego.Use(api, middleware.AuthMiddleware(authService))

	// Wedding selection
	fuego.Post(api, "/auth/select-wedding", authHandler.SelectWedding)

	// Admin CRUD
	fuego.Get(api, "/admins", adminHandler.List)
	fuego.Post(api, "/admins", adminHandler.Create)
	fuego.Delete(api, "/admins/{id}", adminHandler.Delete)
	fuego.Put(api, "/admins/{id}/weddings", adminHandler.AssignWeddings)
	fuego.Get(api, "/admins/{id}/weddings", adminHandler.GetUserWeddings)

	// Wedding CRUD
	fuego.Get(api, "/weddings", weddingHandler.List)
	fuego.Post(api, "/weddings", weddingHandler.Create)
	fuego.Get(api, "/weddings/{id}", weddingHandler.Get)
	fuego.Put(api, "/weddings/{id}", weddingHandler.Update)
	fuego.Delete(api, "/weddings/{id}", weddingHandler.Delete)

	// ── Auth + Wedding scope ──
	scoped := fuego.Group(api, "/weddings/{wid}")
	fuego.Use(scoped, middleware.WeddingScopeMiddleware)

	// Tables
	fuego.Get(scoped, "/tables", tableHandler.List)
	fuego.Post(scoped, "/tables", tableHandler.Create)
	fuego.Put(scoped, "/tables/{id}", tableHandler.Update)
	fuego.Delete(scoped, "/tables/{id}", tableHandler.Delete)

	// Guests
	fuego.Get(scoped, "/guests", guestHandler.List)
	fuego.Post(scoped, "/guests", guestHandler.Create)
	fuego.Get(scoped, "/guests/{id}", guestHandler.Get)
	fuego.Put(scoped, "/guests/{id}", guestHandler.Update)
	fuego.Delete(scoped, "/guests/{id}", guestHandler.Delete)
	fuego.Post(scoped, "/guests/{id}/checkin", guestHandler.CheckIn)
	fuego.Post(scoped, "/guests/{id}/checkout", guestHandler.CheckOut)
	fuego.Post(scoped, "/guests/{id}/seat", guestHandler.AssignSeat)
	fuego.Get(scoped, "/guests/search", guestHandler.Search)
	fuego.Get(scoped, "/occupancy", guestHandler.Occupancy)
}
