package handlers

import (
	"net/http"
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
	layoutService *services.LayoutService,
	reportService *services.ReportService,
	adminRepo *repository.AdminRepo,
) {
	authHandler := NewAuthHandler(authService, adminRepo)
	adminHandler := NewAdminHandler(adminRepo, authService)
	guestHandler := NewGuestHandler(guestService)
	tableHandler := NewTableHandler(tableService)
	weddingHandler := NewWeddingHandler(weddingService, adminRepo)
	layoutHandler := NewLayoutHandler(layoutService, tableService, weddingService)
	uploadHandler := NewUploadHandler()

	// ── Public (no middleware) ──
	pub := fuego.Group(s, "/api/auth")
	fuego.Post(pub, "/login", authHandler.Login)
	fuego.Post(pub, "/refresh", authHandler.Refresh)
	fuego.Post(pub, "/logout", authHandler.Logout)
	fuego.Post(pub, "/register", authHandler.Register)

	// ── Public guest endpoints (kiosk, no auth) ──
	pubApi := fuego.Group(s, "/api/public")
	pubGuestHandler := NewPublicGuestHandler(guestService)
	pubKioskHandler := NewPublicKioskHandler(weddingService)
	pubScoped := fuego.Group(pubApi, "/weddings/{wid}")
	fuego.Get(pubScoped, "/guests", pubGuestHandler.List)
	fuego.Get(pubScoped, "/guests/search", pubGuestHandler.Search,
		fuego.OptionQuery("q", "Search query"),
	)
	fuego.Get(pubScoped, "/tables", tableHandler.List)
	fuego.Get(pubScoped, "/layout", layoutHandler.Get)
	fuego.Get(pubScoped, "/kiosk", pubKioskHandler.GetKioskSettings)

	// ── Auth-protected ──
	api := fuego.Group(s, "/api")
	fuego.Use(api, middleware.AuthMiddleware(authService))

	// Wedding selection
	fuego.Post(api, "/auth/select-wedding", authHandler.SelectWedding)
	fuego.Post(api, "/auth/change-password", authHandler.ChangePassword)

	// User CRUD
	fuego.Get(api, "/users", adminHandler.List)
	fuego.Post(api, "/users", adminHandler.Create)
	fuego.Delete(api, "/users/{id}", adminHandler.Delete)
	fuego.Put(api, "/users/{id}/weddings", adminHandler.AssignWeddings)
	fuego.Get(api, "/users/{id}/weddings", adminHandler.GetUserWeddings)
	fuego.Post(api, "/users/{id}/reset-password", adminHandler.ResetPassword)
	fuego.Put(api, "/users/{id}/role", adminHandler.UpdateRole)
	fuego.Put(api, "/users/{id}/revoke", adminHandler.RevokeUser)

	// Wedding CRUD
	fuego.Get(api, "/weddings", weddingHandler.List)
	fuego.Post(api, "/weddings", weddingHandler.Create)
	fuego.Get(api, "/weddings/{id}", weddingHandler.Get)
	fuego.Put(api, "/weddings/{id}", weddingHandler.Update)
	fuego.Put(api, "/weddings/{id}/kiosk", weddingHandler.UpdateKioskSettings)
	fuego.Delete(api, "/weddings/{id}", weddingHandler.Delete)

	// File upload
	fuego.Post(api, "/upload", uploadHandler.Upload)

	// ── Auth + Wedding scope ──
	scoped := fuego.Group(api, "/weddings/{wid}")
	fuego.Use(scoped, middleware.WeddingScopeMiddleware)

	// Tables
	fuego.Get(scoped, "/tables", tableHandler.List)
	fuego.Post(scoped, "/tables", tableHandler.Create)
	fuego.Put(scoped, "/tables/{id}", tableHandler.Update)
	fuego.Delete(scoped, "/tables/{id}", tableHandler.Delete)

	// Guests
	fuego.Get(scoped, "/guests", guestHandler.List,
		fuego.OptionQuery("cursor", "Pagination cursor"),
		fuego.OptionQueryInt("limit", "Number of items per page"),
	)
	fuego.Post(scoped, "/guests", guestHandler.Create)
	fuego.Get(scoped, "/guests/{id}", guestHandler.Get)
	fuego.Put(scoped, "/guests/{id}", guestHandler.Update)
	fuego.Delete(scoped, "/guests/{id}", guestHandler.Delete)
	fuego.Post(scoped, "/guests/{id}/checkin", guestHandler.CheckIn)
	fuego.Post(scoped, "/guests/{id}/checkout", guestHandler.CheckOut)
	fuego.Post(scoped, "/guests/{id}/seat", guestHandler.AssignSeat)
	fuego.Delete(scoped, "/guests/{id}/seat", guestHandler.UnassignSeat)
	fuego.Get(scoped, "/guests/search", guestHandler.Search,
		fuego.OptionQuery("q", "Search query"),
	)
	fuego.Post(scoped, "/guests/import", guestHandler.BulkImport)
	fuego.Get(scoped, "/occupancy", guestHandler.Occupancy)

	// Layout
	fuego.Get(scoped, "/layout", layoutHandler.Get)
	fuego.Patch(scoped, "/layout", layoutHandler.Save)

	// Reports
	reportHandler := NewReportHandler(reportService)
	s.Mux.Handle("GET /api/weddings/{wid}/reports/angpao",
		middleware.AuthMiddleware(authService)(middleware.WeddingScopeMiddleware(http.HandlerFunc(reportHandler.ExportAngpao))))
}
