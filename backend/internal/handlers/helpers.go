package handlers

import (
	"context"
	"errors"
	"strings"
	"weddingdb/internal/middleware"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

// requireAdmin returns an error if the caller is not admin.
func requireAdmin(ctx context.Context) error {
	if RoleFromContext(ctx) != "admin" {
		return errUnauthorized("admin role required")
	}
	return nil
}

// requireWeddingAccess returns nil if the caller has access to the given wedding.
// admin has access to all weddings; user only to the one in their JWT.
func requireWeddingAccess(ctx context.Context, weddingID uuid.UUID) error {
	role := RoleFromContext(ctx)
	if role == "admin" {
		return nil
	}
	jwtWid := WeddingIDFromContext(ctx)
	if jwtWid == nil {
		return errForbidden("no wedding scope")
	}
	if *jwtWid != weddingID {
		return errForbidden("access denied to this wedding")
	}
	return nil
}

// isUniqueViolation reports whether err came from a unique index, so a
// concurrent duplicate maps to 409 instead of 500.
func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value violates unique constraint") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}

type unauthorizedError struct{ msg string }

func (e unauthorizedError) Error() string { return e.msg }

type forbiddenError struct{ msg string }

func (e forbiddenError) Error() string { return e.msg }

func errUnauthorized(msg string) error { return unauthorizedError{msg} }
func errForbidden(msg string) error    { return forbiddenError{msg} }
