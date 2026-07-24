package middleware

import (
	"net/http"

	"github.com/google/uuid"
)

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
		urlWid, err := uuid.Parse(r.PathValue("wid"))
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
