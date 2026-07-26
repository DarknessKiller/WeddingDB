package middleware

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// DecodeWIDString parses a UUID from a string, supporting both standard UUID
// format and base64-encoded IDs (with URL-safe to standard encoding mapping).
func DecodeWIDString(s string) (uuid.UUID, error) {
	if id, err := uuid.Parse(s); err == nil {
		return id, nil
	}
	b64 := strings.ReplaceAll(s, "_", "+")
	b64 = strings.ReplaceAll(b64, "-", "/")
	switch len(b64) % 4 {
	case 2:
		b64 += "=="
	case 3:
		b64 += "="
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return uuid.Nil, err
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
