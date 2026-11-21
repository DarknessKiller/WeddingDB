package handlers

import (
	"context"
	"log/slog"
	"weddingdb/internal/middleware"
	"weddingdb/internal/repository"

	"github.com/go-fuego/fuego"

	"github.com/google/uuid"
)

// AdminIDFromContext extracts admin ID from the request context.
func AdminIDFromContext(ctx context.Context) uuid.UUID {
	v, _ := ctx.Value(middleware.AdminIDKey).(uuid.UUID)
	return v
}

// WeddingIDFromContext extracts wedding ID from the request context.
func WeddingIDFromContext(ctx context.Context) *uuid.UUID {
	v, _ := ctx.Value(middleware.WeddingIDKey).(*uuid.UUID)
	return v
}

// RoleFromContext extracts role from the request context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(middleware.RoleKey).(string)
	return v
}

// DecodeWID parses a UUID from the path param "wid", supporting base64-encoded IDs.
func DecodeWID(c interface{ PathParam(string) string }) (uuid.UUID, error) {
	return middleware.DecodeWIDString(c.PathParam("wid"))
}

// DecodeID parses a UUID from a base64-encoded string.
func DecodeID(s string) (uuid.UUID, error) {
	return middleware.DecodeWIDString(s)
}

// requireAdmin answers 403 when the caller is signed in but not an admin. Not
// 401: the client reads 401 as an expired session and rotates its tokens.
func requireAdmin(ctx context.Context) error {
	if RoleFromContext(ctx) != "admin" {
		return fuego.ForbiddenError{Title: "admin role required"}
	}
	return nil
}

// requireWeddingAccess returns nil if the caller has access to the given wedding.
// admin has access to all weddings; user only to the one in their JWT, and only
// while still assigned to it in the DB (unassignment must not wait for token expiry).
func requireWeddingAccess(ctx context.Context, adminRepo *repository.AdminRepo, weddingID uuid.UUID) error {
	role := RoleFromContext(ctx)
	if role == "admin" {
		return nil
	}
	jwtWid := WeddingIDFromContext(ctx)
	if jwtWid == nil {
		return fuego.ForbiddenError{Title: "no wedding scope"}
	}
	if *jwtWid != weddingID {
		return fuego.ForbiddenError{Title: "access denied to this wedding"}
	}
	allowed, err := adminRepo.HasWeddingAccess(ctx, AdminIDFromContext(ctx), weddingID)
	if err != nil {
		slog.Error("wedding access check failed", "wedding", weddingID, "err", err)
		return fuego.InternalServerError{Title: "Access check failed"}
	}
	if !allowed {
		return fuego.ForbiddenError{Title: "access denied to this wedding"}
	}
	return nil
}
