package middleware

import (
	"net/http"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"

	"github.com/google/uuid"
)

// DecodeWIDString parses a UUID from a base64 URL-safe string or a plain UUID string.
func DecodeWIDString(s string) (uuid.UUID, error) {
	return utils.DecodeUUID(s)
}

// WeddingScopeMiddleware authorizes a request against the wedding in the URL.
// The token's wedding claim must match, and the caller must still be allowed:
// a revoked assignment or a deleted wedding must not keep working with an
// already-issued token.
func WeddingScopeMiddleware(adminRepo *repository.AdminRepo, weddingService *services.WeddingService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			urlWid, err := DecodeWIDString(r.PathValue("wid"))
			if err != nil {
				http.Error(w, `{"error":"Invalid wedding ID"}`, http.StatusBadRequest)
				return
			}

			role, _ := r.Context().Value(RoleKey).(string)
			if role == "admin" {
				// An admin may act on any existing wedding, but not on one that
				// was deleted while their token was still valid.
				if _, err := weddingService.Get(r.Context(), urlWid); err != nil {
					http.Error(w, `{"error":"Wedding not found"}`, http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			jwtWid, _ := r.Context().Value(WeddingIDKey).(*uuid.UUID)
			if jwtWid == nil {
				http.Error(w, `{"error":"No wedding scope"}`, http.StatusForbidden)
				return
			}
			if *jwtWid != urlWid {
				http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
				return
			}
			adminID, _ := r.Context().Value(AdminIDKey).(uuid.UUID)
			allowed, err := adminRepo.HasWeddingAccess(r.Context(), adminID, urlWid)
			if err != nil {
				http.Error(w, `{"error":"Access check failed"}`, http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
