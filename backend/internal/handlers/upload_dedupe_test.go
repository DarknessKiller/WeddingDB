package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Deleting one wedding's uploads must keep a file another wedding's index line
// still references.
func TestRemoveWeddingUploads_KeepsSharedFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("uploads", 0o755); err != nil {
		t.Fatal(err)
	}
	name := "abc12345-deadbeef.png"
	if err := os.WriteFile(filepath.Join("uploads", name), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	widA, widB := uuid.New(), uuid.New()
	index := "hash " + name + " 10 " + widA.String() + "\nhash " + name + " 0 " + widB.String() + "\n"
	if err := os.WriteFile(filepath.Join("uploads", ".hash-index"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	removeWeddingUploads(widA)

	if _, err := os.Stat(filepath.Join("uploads", name)); err != nil {
		t.Fatalf("shared file was removed: %v", err)
	}
	idx, err := os.ReadFile(filepath.Join("uploads", ".hash-index"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(idx), widA.String()) {
		t.Fatalf("index still names deleted scope:\n%s", idx)
	}
	if !strings.Contains(string(idx), widB.String()) {
		t.Fatalf("index lost remaining scope:\n%s", idx)
	}
}
