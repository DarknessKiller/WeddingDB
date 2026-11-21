package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var magicBytes = map[string][]byte{
	".jpg":  {0xff, 0xd8, 0xff},
	".jpeg": {0xff, 0xd8, 0xff},
	".png":  {0x89, 0x50, 0x4e, 0x47},
	".gif":  {0x47, 0x49, 0x46, 0x38},
	".webp": {0x52, 0x49, 0x46, 0x46}, // RIFF header
}

// matchesMagic reports whether head (the file's first bytes) fits ext. An SVG
// may open with an XML prolog or straight at <svg (Figma, most exporters).
func matchesMagic(ext string, head []byte) bool {
	if ext == ".svg" {
		head = bytes.TrimLeft(bytes.TrimPrefix(head, []byte("\xef\xbb\xbf")), " \t\r\n")
		return bytes.HasPrefix(head, []byte("<?xml")) || bytes.HasPrefix(head, []byte("<svg"))
	}
	if ext == ".webp" {
		// RIFF is a generic container (WAV, AVI); WebP adds "WEBP" after the size.
		return len(head) >= 12 && bytes.HasPrefix(head, magicBytes[ext]) && string(head[8:12]) == "WEBP"
	}
	return bytes.HasPrefix(head, magicBytes[ext])
}

// maxUploadFileBytes is the advertised per-file limit.
const maxUploadFileBytes = 5 * 1024 * 1024

// maxUploadBytes caps the whole multipart envelope: the file limit plus 1 MiB
// of multipart framing and headers.
const maxUploadBytes = maxUploadFileBytes + 1024*1024

// uploadQuotaPerScope caps stored bytes per scope (a wedding, or the shared
// "global" admin namespace). Kiosk artwork is small; a filled disk takes the
// whole service down.
const uploadQuotaPerScope = 100 * 1024 * 1024

// uploadRateLimit caps how many uploads one scope may start per window.
const (
	uploadRateLimit  = 30
	uploadRateWindow = time.Hour
)

type UploadHandler struct {
	mu        sync.Mutex
	windows   map[string]*uploadWindow
	hasAccess func(ctx context.Context, userID, weddingID uuid.UUID) (bool, error)
}

type uploadWindow struct {
	start time.Time
	count int
}

// NewUploadHandler takes the DB wedding-assignment check (AdminRepo.HasWeddingAccess).
func NewUploadHandler(hasAccess func(ctx context.Context, userID, weddingID uuid.UUID) (bool, error)) *UploadHandler {
	return &UploadHandler{windows: make(map[string]*uploadWindow), hasAccess: hasAccess}
}

// allow applies a fixed-window rate limit per scope so one wedding cannot
// burn bandwidth and disk in a burst.
func (h *UploadHandler) allow(scope string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	w, ok := h.windows[scope]
	if !ok || now.Sub(w.start) >= uploadRateWindow {
		h.windows[scope] = &uploadWindow{start: now, count: 1}
		return true
	}
	if w.count >= uploadRateLimit {
		return false
	}
	w.count++
	return true
}

// closeLogged closes c and logs a failure the caller cannot act on.
func closeLogged(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Warn("close failed", "err", err)
	}
}

func (h *UploadHandler) Upload(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	// Uploads feed kiosk settings, so only admins or users scoped to a wedding
	// may upload, and the user must still be assigned to it: the token's
	// wedding claim outlives an unassign until it expires.
	if RoleFromContext(ctx) != "admin" {
		wid := WeddingIDFromContext(ctx)
		if wid == nil {
			return nil, fuego.ForbiddenError{Title: "Select a wedding before uploading"}
		}
		allowed, err := h.hasAccess(ctx, AdminIDFromContext(ctx), *wid)
		if err != nil {
			return nil, fmt.Errorf("upload access check: %w", err)
		}
		if !allowed {
			return nil, fuego.ForbiddenError{Title: "access denied to this wedding"}
		}
	}
	// Namespace the hash index, quota and rate limit per wedding: an upload may
	// only reuse a file from its own wedding, never another tenant's. Only an
	// admin with no wedding selected falls back to the shared global scope.
	scope := "global"
	if wid := WeddingIDFromContext(c.Context()); wid != nil {
		scope = wid.String()
	}
	if !h.allow(scope) {
		return nil, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: "Too many uploads, try again later"}
	}
	r := c.Request()
	r.Body = http.MaxBytesReader(c.Response(), r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return nil, fuego.HTTPError{Status: http.StatusRequestEntityTooLarge, Title: "File too large (max 5 MiB)"}
		}
		return nil, fuego.BadRequestError{Title: "No file provided"}
	}
	defer closeLogged(file)
	if header.Size > maxUploadFileBytes {
		return nil, fuego.HTTPError{Status: http.StatusRequestEntityTooLarge, Title: "File too large (max 5 MiB)"}
	}

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true}
	if !allowed[ext] {
		return nil, fuego.BadRequestError{Title: "File type not allowed. Use JPG, PNG, GIF, WebP, or SVG"}
	}

	// Validate magic bytes (mmap-backed multipart Files can legally return io.EOF on the last read)
	headerBuf := make([]byte, 16)
	n, err := file.Read(headerBuf)
	if err != nil && n == 0 {
		return nil, fuego.BadRequestError{Title: "Failed to read file"}
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to read file"}
	}
	if !matchesMagic(ext, headerBuf[:n]) {
		return nil, fuego.BadRequestError{Title: "File content does not match extension"}
	}

	// Create uploads directory
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to create upload directory"}
	}

	// Compute SHA256 hash of file content
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to read file"}
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	// The index is both the dedupe table and the quota ledger. Read it under
	// the same lock that appends to it, or two concurrent uploads of the same
	// bytes both miss the existing entry and store a copy.
	indexPath := filepath.Join(uploadDir, ".hash-index")
	lockFile, err := os.OpenFile(indexPath+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	defer closeLogged(lockFile)
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	defer func() {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
			slog.Warn("unlock upload index failed", "err", err)
		}
	}()

	existing, used, err := scanUploadIndex(indexPath, uploadDir, scope, hash)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fuego.InternalServerError{Title: "Failed to read upload index"}
	}
	if existing != "" {
		return map[string]any{
			"url":      "/uploads/" + existing,
			"filename": existing,
			"hash":     hash,
		}, nil
	}
	if used+header.Size > uploadQuotaPerScope {
		return nil, fuego.HTTPError{Status: http.StatusRequestEntityTooLarge, Title: "Upload quota exceeded for this wedding"}
	}

	// Save new file with hash as prefix
	filename := fmt.Sprintf("%s-%s%s", hash[:8], uuid.New().String()[:8], ext)
	filePath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	defer closeLogged(dst)

	// Reset file pointer to beginning for writing
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	if _, err := dst.ReadFrom(file); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}

	f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// The index is the quota ledger: a file missing from it is never counted,
	// so an upload whose index entry cannot be written is removed, not kept.
	unindexed := func(err error) error {
		slog.Error("upload index write failed; removing unindexed file", "file", filename, "err", err)
		if rmErr := os.Remove(filePath); rmErr != nil {
			slog.Warn("remove unindexed upload failed", "file", filename, "err", rmErr)
		}
		return fuego.InternalServerError{Title: "Failed to save file"}
	}
	if err != nil {
		return nil, unindexed(err)
	}
	if _, err := fmt.Fprintf(f, "%s %s %s\n", scope, hash, filename); err != nil {
		closeLogged(f)
		return nil, unindexed(err)
	}
	if err := f.Close(); err != nil {
		return nil, unindexed(err)
	}

	c.SetStatus(201)
	return map[string]any{
		"url":      "/uploads/" + filename,
		"filename": filename,
		"hash":     hash,
	}, nil
}

// scanUploadIndex returns the stored filename for hash in scope and the total
// bytes that scope currently occupies.
func scanUploadIndex(indexPath, uploadDir, scope, hash string) (string, int64, error) {
	idx, err := os.ReadFile(indexPath)
	if err != nil {
		return "", 0, err
	}
	var existing string
	var used int64
	for line := range strings.SplitSeq(string(idx), "\n") {
		parts := strings.SplitN(line, " ", 3)
		// Old entries have two fields and belong to the global namespace.
		entryScope, entryHash, entryName := "global", "", ""
		if len(parts) == 3 {
			entryScope, entryHash, entryName = parts[0], parts[1], parts[2]
		} else if len(parts) == 2 {
			entryHash, entryName = parts[0], parts[1]
		}
		if entryName == "" || entryScope != scope {
			continue
		}
		if fi, err := os.Stat(filepath.Join(uploadDir, entryName)); err == nil {
			used += fi.Size()
		}
		if entryHash == hash {
			existing = entryName
		}
	}
	return existing, used, nil
}
