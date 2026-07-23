package handlers

import (
	"weddingdb/internal/middleware"
	"weddingdb/internal/utils"
)

// AdminIDFromContext extracts admin ID from the request context.
func AdminIDFromContext(r interface{ Context() interface{ Value(any) any } }) uint {
	v, _ := r.Context().Value(middleware.AdminIDKey).(uint)
	return v
}

// WeddingIDFromContext extracts wedding ID from the request context.
func WeddingIDFromContext(r interface{ Context() interface{ Value(any) any } }) *uint {
	v, _ := r.Context().Value(middleware.WeddingIDKey).(*uint)
	return v
}

// RoleFromContext extracts role from the request context.
func RoleFromContext(r interface{ Context() interface{ Value(any) any } }) string {
	v, _ := r.Context().Value(middleware.RoleKey).(string)
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


