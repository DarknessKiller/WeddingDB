package middleware

import (
	"encoding/base64"
	"net/http"

	"github.com/google/uuid"
)

// DecodeWIDString parses a UUID from a base64 URL-safe string or a plain UUID string.
func DecodeWIDString(s string) (uuid.UUID, error) {
	// Try plain UUID first (e.g. "550e8400-e29b-41d4-a716-446655440000")
	if id, err := uuid.Parse(s); err == nil {
		return id, nil
	}
	// Try base64 URL-safe (with or without padding)
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		// Try with standard padding
		decoded, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			return uuid.Nil, err
		}
	}
	return uuid.FromBytes(decoded)
}

func WeddingScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(RoleKey).(string)
		if role == "admin" {
			next.ServeHTTP(w, r)
			return
		}
		jwtWid, _ := r.Context().Value(WeddingIDKey).(*uuid.UUID)
		if jwtWid == nil {
			http.Error(w, `{"error":"No wedding scope"}`, http.StatusForbidden)
			return
		}
		urlWid, err := DecodeWIDString(r.PathValue("wid"))
		if err != nil {
			http.Error(w, `{"error":"Invalid wedding ID"}`, http.StatusBadRequest)
			return
		}
		if *jwtWid != urlWid {
			http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
