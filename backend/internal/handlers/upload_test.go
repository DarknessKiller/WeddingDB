package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/middleware"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// runUpload drives the upload controller via fuego's HTTPHandler, isolated in a
// temp CWD so ./uploads stays out of the repo tree.
func runUpload(t *testing.T, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	return runUploadAs(t, filename, content, "admin", nil)
}

func runUploadAs(t *testing.T, filename string, content []byte, role string, wid *uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	t.Chdir(t.TempDir())
	return serveUpload(t, newUploadHandler(t), filename, content, role, wid)
}

func newUploadHandler(t *testing.T) *UploadHandler {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	return NewUploadHandler(services.NewAuthService(nil, nil, nil, "secret", rdb))
}

func serveUpload(t *testing.T, h *UploadHandler, filename string, content []byte, role string, wid *uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
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
	req.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(req.Context(), middleware.RoleKey, role)
	if wid != nil {
		ctx = context.WithValue(ctx, middleware.WeddingIDKey, wid)
	}
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler := fuego.HTTPHandler[any, any, any](s, h.Upload, fuego.BaseRoute{Path: "/upload", Method: http.MethodPost})
	handler(rec, req)
	return rec
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

func TestUpload_RequiresWeddingAccess(t *testing.T) {
	// A freshly self-registered account has no wedding scope yet.
	rec := runUploadAs(t, "a.jpg", []byte{0xff, 0xd8, 0xff}, "user", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_WeddingQuota(t *testing.T) {
	t.Chdir(t.TempDir())
	wid, other := uuid.New(), uuid.New()
	testutil.NoErr(t, os.MkdirAll("uploads", 0o755))
	full := fmt.Sprintf("aaaa old.png %d %s\nlegacy legacy.png\n", maxWeddingUploadBytes-2, wid)
	testutil.NoErr(t, os.WriteFile("uploads/.hash-index", []byte(full), 0o644))
	h := newUploadHandler(t)

	tests := []struct {
		name    string
		wid     uuid.UUID
		content []byte
		want    int
	}{
		{"over quota", wid, []byte{0xff, 0xd8, 0xff, 0x01}, http.StatusRequestEntityTooLarge},
		{"other wedding", other, []byte{0xff, 0xd8, 0xff, 0x02}, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveUpload(t, h, "a.jpg", tt.content, "user", &tt.wid)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
	idx, err := os.ReadFile("uploads/.hash-index")
	testutil.NoErr(t, err)
	if !strings.Contains(string(idx), " 4 "+other.String()+"\n") {
		t.Fatalf("index line lacks size and wedding: %q", idx)
	}
}

func TestUpload_RateLimited(t *testing.T) {
	t.Chdir(t.TempDir())
	h := newUploadHandler(t)
	wid := uuid.New()
	for i := range 21 {
		rec := serveUpload(t, h, "a.jpg", []byte{0xff, 0xd8, 0xff, byte(i)}, "user", &wid)
		if i < 20 && rec.Code != http.StatusCreated {
			t.Fatalf("upload %d: status = %d", i, rec.Code)
		}
		if i == 20 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("upload 21: status = %d, want 429", rec.Code)
		}
	}
}
