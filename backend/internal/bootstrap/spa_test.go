package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A traversal path must not touch the filesystem: both an existing and a
// missing outside file answer 404, not 400-vs-200.
func TestSpaHandler_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>shell</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{path...}", spaHandler(dir))

	get := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}

	if rec := get("/app.js"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("asset status %d body %q", rec.Code, rec.Body.String())
	}
	if rec := get("/some/client/route"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "shell") {
		t.Fatalf("SPA fallback status %d body %q", rec.Code, rec.Body.String())
	}
	for _, target := range []string{"/..%2f..%2fetc%2fpasswd", "/..%2f..%2fno%2fsuch%2ffile"} {
		if rec := get(target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s status %d, want 404", target, rec.Code)
		}
	}
}
