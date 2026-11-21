package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"weddingdb/internal/testutil"
)

func TestStaticHandler(t *testing.T) {
	dir := t.TempDir()
	testutil.NoErr(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{path...}", staticHandler(dir))

	cases := []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/w123/guests", http.StatusOK},               // client-side route gets the SPA shell
		{"/api/weddings/x/typo", http.StatusNotFound}, // an API typo must not get HTML 200
		{"/api", http.StatusNotFound},
		{"/apiary", http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("GET %s = %d, want %d", tc.path, rec.Code, tc.want)
		}
	}
}
