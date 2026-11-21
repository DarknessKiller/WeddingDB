package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	handler := fuego.HTTPHandler[any, any, any](s, NewUploadHandler(allowAllWeddings).Upload, fuego.BaseRoute{Path: "/upload", Method: http.MethodPost})
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

func TestUpload_ScopeQuotaRejected(t *testing.T) {
	t.Chdir(t.TempDir())
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	filler, err := os.Create(filepath.Join(uploadDir, "filler.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := filler.Truncate(uploadQuotaPerScope); err != nil {
		t.Fatal(err)
	}
	if err := filler.Close(); err != nil {
		t.Fatal(err)
	}
	index := "global " + strings.Repeat("0", 64) + " filler.jpg\n"
	if err := os.WriteFile(filepath.Join(uploadDir, ".hash-index"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := doUpload(t, context.WithValue(context.Background(), middleware.RoleKey, "admin"), "a.jpg", []byte{0xff, 0xd8, 0xff})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body = %s", rec.Code, rec.Body.String())
	}
}

// An admin working inside a wedding uses that wedding's quota, not the shared
// global one, so a full global namespace never blocks kiosk setup.
func TestUpload_AdminInWeddingUsesWeddingQuota(t *testing.T) {
	t.Chdir(t.TempDir())
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	filler, err := os.Create(filepath.Join(uploadDir, "filler.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := filler.Truncate(uploadQuotaPerScope); err != nil {
		t.Fatal(err)
	}
	if err := filler.Close(); err != nil {
		t.Fatal(err)
	}
	index := "global " + strings.Repeat("0", 64) + " filler.jpg\n"
	if err := os.WriteFile(filepath.Join(uploadDir, ".hash-index"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	wid := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	ctx := context.WithValue(context.WithValue(context.Background(), middleware.RoleKey, "admin"), middleware.WeddingIDKey, &wid)
	rec := doUpload(t, ctx, "a.jpg", []byte{0xff, 0xd8, 0xff})
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status = %d, want 2xx, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_RateLimitedPerScope(t *testing.T) {
	h := NewUploadHandler(allowAllWeddings)
	for range uploadRateLimit {
		if !h.allow("w1") {
			t.Fatal("upload within the limit was rejected")
		}
	}
	if h.allow("w1") {
		t.Fatal("upload over the rate limit was allowed")
	}
	if !h.allow("w2") {
		t.Fatal("rate limit must apply per scope, not globally")
	}
}

// Two concurrent uploads of identical bytes must produce one stored file.
func TestUpload_ConcurrentSameContentDedupes(t *testing.T) {
	t.Chdir(t.TempDir())
	content := []byte{0xff, 0xd8, 0xff}
	ctx := context.WithValue(context.Background(), middleware.RoleKey, "admin")

	var wg sync.WaitGroup
	names := make([]string, 2)
	for i := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := doUpload(t, ctx, "logo.jpg", content)
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Errorf("decode response: %v", err)
				return
			}
			names[i], _ = body["filename"].(string)
		}()
	}
	wg.Wait()
	if names[0] == "" || names[0] != names[1] {
		t.Fatalf("concurrent uploads returned different files: %v", names)
	}

	entries, err := os.ReadDir("./uploads")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			files++
		}
	}
	if files != 1 {
		t.Fatalf("stored %d files for identical content, want 1", files)
	}
}

// The index is the quota ledger: if the entry cannot be written, the stored
// file must not stay behind uncounted.
func TestUpload_IndexWriteFailureRemovesFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("uploads", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("uploads", ".hash-index"), nil, 0o444); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), middleware.RoleKey, "admin")
	rec := doUpload(t, ctx, "a.png", []byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	matches, err := filepath.Glob(filepath.Join("uploads", "*.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unindexed upload left on disk: %v", matches)
	}
}

// SVGs without an XML prolog are the common case; wrong content still fails.
func TestUpload_SVGMagic(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		ok   bool
	}{
		"prolog":      {`<?xml version="1.0"?><svg/>`, true},
		"bare svg":    {`<svg xmlns="http://www.w3.org/2000/svg"/>`, true},
		"bom + space": {"\xef\xbb\xbf\n  <svg/>", true},
		"html":        {`<html><script>x</script>`, false},
	} {
		t.Run(name, func(t *testing.T) {
			rec := runUpload(t, "logo.svg", []byte(tc.body))
			if got := rec.Code == http.StatusCreated; got != tc.ok {
				t.Fatalf("status = %d, want accepted=%v", rec.Code, tc.ok)
			}
		})
	}
}

func TestUpload_WebPMagic(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		ok   bool
	}{
		"webp":      {"RIFF\x10\x00\x00\x00WEBPVP8 ", true},
		"wav":       {"RIFF\x10\x00\x00\x00WAVEfmt ", false},
		"bare riff": {"RIFF", false},
	} {
		t.Run(name, func(t *testing.T) {
			rec := runUpload(t, "bg.webp", []byte(tc.body))
			if got := rec.Code == http.StatusCreated; got != tc.ok {
				t.Fatalf("status = %d, want accepted=%v", rec.Code, tc.ok)
			}
		})
	}
}

func allowAllWeddings(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }

// A user unassigned from a wedding still holds a token naming it until it
// expires; upload must re-check the assignment, not trust the claim.
func TestUpload_UnassignedUserRefused(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	deny := func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, nil }
	handler := fuego.HTTPHandler[any, any, any](s, NewUploadHandler(deny).Upload, fuego.BaseRoute{Path: "/upload", Method: http.MethodPost})
	wid := uuid.New()
	ctx := context.WithValue(context.Background(), middleware.RoleKey, "user")
	ctx = context.WithValue(ctx, middleware.WeddingIDKey, &wid)
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("")).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}
