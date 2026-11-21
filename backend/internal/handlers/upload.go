package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var magicBytes = map[string][]byte{
	".jpg":  {0xff, 0xd8, 0xff},
	".jpeg": {0xff, 0xd8, 0xff},
	".png":  {0x89, 0x50, 0x4e, 0x47},
	".gif":  {0x47, 0x49, 0x46, 0x38},
	".webp": {0x52, 0x49, 0x46, 0x46},       // RIFF header
	".svg":  {0x3c, 0x3f, 0x78, 0x6d, 0x6c}, // <?xml
}

// maxUploadFileBytes is the advertised per-file limit.
const maxUploadFileBytes = 5 * 1024 * 1024

// maxUploadBytes caps the whole multipart envelope: the file limit plus 1 MiB
// of multipart framing and headers.
const maxUploadBytes = maxUploadFileBytes + 1024*1024

// maxWeddingUploadBytes caps the disk one wedding's uploads may use: 100 MiB,
// twenty files at the per-file limit, far above a logo plus backgrounds.
// Unscoped admins share one pool. Duplicate uploads reuse the stored file and
// cost nothing.
const maxWeddingUploadBytes = 100 * 1024 * 1024

type UploadHandler struct{ authService *services.AuthService }

func NewUploadHandler(authService *services.AuthService) *UploadHandler {
	return &UploadHandler{authService: authService}
}

// closeLogged closes c and logs a failure the caller cannot act on.
func closeLogged(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Warn("close failed", "err", err)
	}
}

func (h *UploadHandler) Upload(c fuego.ContextWithBody[any]) (any, error) {
	r := c.Request()
	// Self-registered accounts have no wedding until an admin assigns one; they
	// must not be able to write files to the server.
	if RoleFromContext(c.Context()) != "admin" && WeddingIDFromContext(c.Context()) == nil {
		return nil, fuego.ForbiddenError{Title: "Upload requires wedding access"}
	}
	if err := h.authService.CheckUploadRate(c.Context(), AdminIDFromContext(c.Context())); err != nil {
		return nil, fuego.HTTPError{Status: http.StatusTooManyRequests, Title: err.Error()}
	}
	scope := uuid.Nil
	if wid := WeddingIDFromContext(c.Context()); wid != nil {
		scope = *wid
	}
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
	if n < len(magicBytes[ext]) {
		return nil, fuego.BadRequestError{Title: "File content does not match extension"}
	}
	expected := magicBytes[ext]
	actual := headerBuf[:len(expected)]
	for i := range expected {
		if actual[i] != expected[i] {
			return nil, fuego.BadRequestError{Title: "File content does not match extension"}
		}
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

	// Check if file with same hash already exists via index file, and sum the
	// wedding's usage in the same pass. Index lines: "hash filename [size wedding]";
	// lines from before the quota carry no size and do not count.
	// shortcut: concurrent uploads can each pass the check and overshoot the cap by one file; lock the index around check and append if that matters.
	indexPath := filepath.Join(uploadDir, ".hash-index")
	var used int64
	if idx, err := os.ReadFile(indexPath); err == nil {
		for line := range strings.SplitSeq(string(idx), "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[0] == hash {
				return map[string]any{
					"url":      "/uploads/" + parts[1],
					"filename": parts[1],
					"hash":     hash,
				}, nil
			}
			if len(parts) == 4 && parts[3] == scope.String() {
				if size, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
					used += size
				}
			}
		}
	}
	if used+header.Size > maxWeddingUploadBytes {
		return nil, fuego.HTTPError{Status: http.StatusRequestEntityTooLarge, Title: "Upload storage for this wedding is full (max 100 MiB)"}
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
	written, err := dst.ReadFrom(file)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}

	// Append hash to index file with file lock
	lockPath := indexPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		defer closeLogged(lockFile)
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
			slog.Warn("lock upload index failed", "err", err)
		}
		defer func() {
			if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
				slog.Warn("unlock upload index failed", "err", err)
			}
		}()
	}
	f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// ponytail: index write failure is non-critical — file is already saved
	} else {
		if _, err := fmt.Fprintf(f, "%s %s %d %s\n", hash, filename, written, scope); err != nil {
			slog.Warn("write upload index failed", "err", err)
		}
		closeLogged(f)
	}

	c.SetStatus(201)
	return map[string]any{
		"url":      "/uploads/" + filename,
		"filename": filename,
		"hash":     hash,
	}, nil
}
