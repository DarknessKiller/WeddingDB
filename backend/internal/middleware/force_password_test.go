package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func signFlaggedToken(t *testing.T, secret string, pwc bool) string {
	t.Helper()
	now := time.Now()
	claims := services.AccessClaims{
		AdminID:             uuid.New(),
		Role:                "admin",
		ForcePasswordChange: pwc,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	testutil.NoErr(t, err)
	return s
}

// A token flagged for a forced password change may reach only the
// change-password route; unflagged tokens are unaffected.
func TestAuthMiddleware_ForcePasswordChange(t *testing.T) {
	const secret = "test-secret"
	handler := AuthMiddleware(newAuthService(t, secret))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name string
		pwc  bool
		path string
		want int
	}{
		{"flagged, other route", true, "/api/weddings", http.StatusForbidden},
		{"flagged, admin route", true, "/api/users", http.StatusForbidden},
		{"flagged, change-password", true, ChangePasswordPath, http.StatusOK},
		{"unflagged, other route", false, "/api/weddings", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+signFlaggedToken(t, secret, tc.pwc))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
