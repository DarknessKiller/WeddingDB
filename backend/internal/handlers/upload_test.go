package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-fuego/fuego"
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

func TestUpload_LargeFileAcceptsBeyondOld5MBLimit(t *testing.T) {
	// 6MB jpg — above the deleted hardcoded cap; proves the proxy-owned policy.
	content := make([]byte, 6*1024*1024)
	content[0], content[1], content[2] = 0xff, 0xd8, 0xff // jpeg magic prefix
	rec := runUpload(t, "big.jpg", content)
	if rec.Code < 200 || rec.Code >= 300 { // success statuses (fuego POST → 201)
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["url"]; !ok {
		t.Fatalf("missing url in response: %s", rec.Body.String())
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
