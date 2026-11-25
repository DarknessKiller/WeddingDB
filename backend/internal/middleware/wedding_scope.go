package middleware

import (
	"net/http"

	"weddingdb/internal/utils"

	"github.com/google/uuid"
)

// DecodeWIDString parses a UUID from a base64 URL-safe string or a plain UUID string.
var DecodeWIDString = utils.DecodeUUID

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
