package handlers

import (
	"context"
	"weddingdb/internal/middleware"
	"weddingdb/internal/utils"
)

// AdminIDFromContext extracts admin ID from the request context.
func AdminIDFromContext(ctx context.Context) uint {
	v, _ := ctx.Value(middleware.AdminIDKey).(uint)
	return v
}

// WeddingIDFromContext extracts wedding ID from the request context.
func WeddingIDFromContext(ctx context.Context) *uint {
	v, _ := ctx.Value(middleware.WeddingIDKey).(*uint)
	return v
}

// RoleFromContext extracts role from the request context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(middleware.RoleKey).(string)
	return v
}

// DecodeWID decodes wedding ID from path param "wid".
func DecodeWID(c interface{ PathParam(string) string }) uint {
	wid, _ := utils.DecodeID(c.PathParam("wid"))
	return wid
}

// DecodeID decodes a base64 ID string.
func DecodeID(encoded string) uint {
	id, _ := utils.DecodeID(encoded)
	return id
}

// requireServiceAdmin returns an error if the caller is not service_admin.
func requireServiceAdmin(ctx context.Context) error {
	if RoleFromContext(ctx) != "service_admin" {
		return errUnauthorized("service_admin role required")
	}
	return nil
}

// requireWeddingAccess returns nil if the caller has access to the given wedding.
// service_admin has access to all weddings; wedding_admin only to their own.
func requireWeddingAccess(ctx context.Context, weddingID uint) error {
	role := RoleFromContext(ctx)
	if role == "service_admin" {
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

type unauthorizedError struct{ msg string }
func (e unauthorizedError) Error() string { return e.msg }

type forbiddenError struct{ msg string }
func (e forbiddenError) Error() string { return e.msg }

func errUnauthorized(msg string) error { return unauthorizedError{msg} }
func errForbidden(msg string) error    { return forbiddenError{msg} }
