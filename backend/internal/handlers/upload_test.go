package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/middleware"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

// runUpload drives the upload controller via fuego's HTTPHandler, isolated in a
// temp CWD so ./uploads stays out of the repo tree.
func runUpload(t *testing.T, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	return runUploadAs(t, context.WithValue(context.Background(), middleware.RoleKey, "admin"), filename, content)
}

// runUploadAs runs the upload with ctx standing in for AuthMiddleware's identity.
func runUploadAs(t *testing.T, ctx context.Context, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	t.Chdir(t.TempDir())
	return doUpload(t, ctx, filename, content)
}

// doUpload drives the upload controller in the current working directory.
func doUpload(t *testing.T, ctx context.Context, filename string, content []byte) *httptest.ResponseRecorder {
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

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	handler := fuego.HTTPHandler[any, any, any](s, NewUploadHandler().Upload, fuego.BaseRoute{Path: "/upload", Method: http.MethodPost})
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

// A freshly registered user has no wedding yet and must not be able to host files.
func TestUpload_UserWithoutWeddingForbidden(t *testing.T) {
	ctx := context.WithValue(context.Background(), middleware.RoleKey, "user")
	rec := runUploadAs(t, ctx, "a.jpg", []byte{0xff, 0xd8, 0xff})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_UserWithWeddingAllowed(t *testing.T) {
	wid := uuid.New()
	ctx := context.WithValue(context.Background(), middleware.RoleKey, "user")
	ctx = context.WithValue(ctx, middleware.WeddingIDKey, &wid)
	rec := runUploadAs(t, ctx, "a.jpg", []byte{0xff, 0xd8, 0xff})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_RejectsUnknownExtension(t *testing.T) {
	rec := runUpload(t, "x.exe", []byte("MZ"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// Identical bytes uploaded by two different weddings must not share a file:
// otherwise one tenant's upload resolves to another tenant's URL.
func TestUpload_HashIndexScopedPerWedding(t *testing.T) {
	t.Chdir(t.TempDir())
	content := []byte{0xff, 0xd8, 0xff}
	upload := func(wid uuid.UUID) map[string]any {
		ctx := context.WithValue(context.Background(), middleware.RoleKey, "user")
		ctx = context.WithValue(ctx, middleware.WeddingIDKey, &wid)
		rec := doUpload(t, ctx, "logo.jpg", content)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return body
	}

	first := upload(uuid.New())
	second := upload(uuid.New())
	if first["filename"] == second["filename"] {
		t.Fatalf("different weddings shared upload %v", first["filename"])
	}

	// Re-uploading in the same wedding still dedupes.
	wedding := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	if a, b := upload(wedding), upload(wedding); a["filename"] != b["filename"] {
		t.Fatalf("same-wedding dedupe failed: %v != %v", a["filename"], b["filename"])
	}
}
