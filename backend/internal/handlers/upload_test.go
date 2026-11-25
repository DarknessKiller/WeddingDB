package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"weddingdb/internal/middleware"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-fuego/fuego"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// runUpload drives the upload controller via fuego's HTTPHandler, isolated in a
// temp CWD so ./uploads stays out of the repo tree.
func runUpload(t *testing.T, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	t.Chdir(t.TempDir())

	s := fuego.NewServer(fuego.WithAddr(":0"))

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req = req.WithContext(context.WithValue(req.Context(), middleware.RoleKey, "admin"))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	handler := fuego.HTTPHandler[any, any, any](s, NewUploadHandler().Upload, fuego.BaseRoute{Path: "/upload", Method: http.MethodPost})
	handler(rec, req)
	return rec
}

func TestUpload_RegisteredRoutesRequireWedding(t *testing.T) {
	t.Chdir(t.TempDir())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	const secret = "test-secret"
	auth := services.NewAuthService(nil, nil, nil, secret, rdb)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	RegisterRoutes(s, auth, nil, nil, nil, nil, nil, nil)
	wid := uuid.New()
	for _, tc := range []struct {
		name    string
		role    string
		wedding *uuid.UUID
		status  int
	}{
		{"unassigned user", "user", nil, http.StatusForbidden},
		{"assigned user", "user", &wid, http.StatusCreated},
		{"admin", "admin", nil, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := services.AccessClaims{AdminID: uuid.New(), Role: tc.role, WeddingID: tc.wedding, RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
			testutil.NoErr(t, err)
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			file, err := mw.CreateFormFile("file", "test.jpg")
			testutil.NoErr(t, err)
			_, err = file.Write([]byte{0xff, 0xd8, 0xff})
			testutil.NoErr(t, err)
			testutil.NoErr(t, mw.Close())
			req := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			s.Mux.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.wedding == nil && tc.role == "user" {
				if _, err := os.Stat("uploads"); !os.IsNotExist(err) {
					t.Fatalf("upload directory created or stat failed: %v", err)
				}
			}
		})
	}
}

// TestUpload_SVGAccepted covers the three legal SVG starts: a bare <svg root, a
// UTF-8 BOM, and an XML prolog.
func TestUpload_SVGAccepted(t *testing.T) {
	for name, content := range map[string][]byte{
		"bare":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"bom":    append([]byte{0xef, 0xbb, 0xbf}, `<svg xmlns="http://www.w3.org/2000/svg"></svg>`...),
		"prolog": []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
	} {
		t.Run(name, func(t *testing.T) {
			rec := runUpload(t, "logo.svg", content)
			if rec.Code < 200 || rec.Code >= 300 {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUpload_SVGWithWrongContentRejected(t *testing.T) {
	rec := runUpload(t, "logo.svg", []byte(`definitely not svg`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_TinyFileNotRejectedByEOFRead(t *testing.T) {
	// 3 bytes == the full JPEG magic; pre-fix this died as "Failed to read file"
	// because io.Read returns io.EOF on the final short read.
	rec := runUpload(t, "a.jpg", []byte{0xff, 0xd8, 0xff})
	if rec.Code < 200 || rec.Code >= 300 { // success statuses (fuego POST → 201)
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_OverLimitRejected413(t *testing.T) {
	content := make([]byte, 6*1024*1024)
	content[0], content[1], content[2] = 0xff, 0xd8, 0xff // jpeg magic prefix
	rec := runUpload(t, "big.jpg", content)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestUpload_MagicMismatchRejected(t *testing.T) {
	rec := runUpload(t, "evil.png", []byte("definitely not a png"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_RejectsUnknownExtension(t *testing.T) {
	rec := runUpload(t, "x.exe", []byte("MZ"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
