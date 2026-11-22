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

	// Check the hash index and the quota, write the file and append the index
	// entry under one exclusive lock, so concurrent uploads cannot both pass
	// the quota check or leave an uncounted file. Index lines: "hash filename
	// [size wedding]"; lines from before the quota carry no size and do not
	// count.
	indexPath := filepath.Join(uploadDir, ".hash-index")
	lockPath := indexPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
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

	var used int64
	var dedupName string
	var scopeHasHash bool
	if idx, err := os.ReadFile(indexPath); err == nil {
		for line := range strings.SplitSeq(string(idx), "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[0] == hash {
				dedupName = parts[1]
				if len(parts) == 4 && parts[3] == scope.String() {
					scopeHasHash = true
				}
			}
			if len(parts) == 4 && parts[3] == scope.String() {
				if size, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
					used += size
				}
			}
		}
	}
	if dedupName != "" {
		// Reused file costs no quota, but the scope still needs its own index
		// line: deleting one scope's uploads must not remove a file another
		// scope still references.
		if !scopeHasHash {
			if err := appendIndexLine(indexPath, hash, dedupName, 0, scope); err != nil {
				slog.Warn("record reused upload failed", "file", dedupName, "err", err)
			}
		}
		return map[string]any{
			"url":      "/uploads/" + dedupName,
			"filename": dedupName,
			"hash":     hash,
		}, nil
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
	if _, err := file.Seek(0, 0); err != nil {
		closeLogged(dst)
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	written, err := dst.ReadFrom(file)
	closeLogged(dst)
	if err != nil {
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("remove partial upload failed", "file", filename, "err", removeErr)
		}
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}

	f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("remove unindexed upload failed", "file", filename, "err", removeErr)
		}
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	if _, err := fmt.Fprintf(f, "%s %s %d %s\n", hash, filename, written, scope); err != nil {
		closeLogged(f)
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("remove unindexed upload failed", "file", filename, "err", removeErr)
		}
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	closeLogged(f)

	c.SetStatus(201)
	return map[string]any{
		"url":      "/uploads/" + filename,
		"filename": filename,
		"hash":     hash,
	}, nil
}

// removeWeddingUploads deletes every indexed file belonging to a wedding and
// drops its index entries. Failures are logged: the database delete already
// committed, so a leftover file must not turn it into an error.
func removeWeddingUploads(weddingID uuid.UUID) {
	uploadDir := "./uploads"
	indexPath := filepath.Join(uploadDir, ".hash-index")
	lockPath := indexPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		slog.Warn("open upload index lock failed", "err", err)
		return
	}
	defer closeLogged(lockFile)
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		slog.Warn("lock upload index failed", "err", err)
		return
	}
	defer func() {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
			slog.Warn("unlock upload index failed", "err", err)
		}
	}()

	idx, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	kept := make([]string, 0, len(idx)/32)
	var removed []string
	for line := range strings.SplitSeq(string(idx), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 4 && parts[3] == weddingID.String() {
			removed = append(removed, parts[1])
			continue
		}
		if line != "" {
			kept = append(kept, line)
		}
	}
	// A file may be referenced by another scope's line; only delete the ones
	// no kept line still names.
	stillReferenced := make(map[string]bool, len(kept))
	for _, line := range kept {
		if parts := strings.Fields(line); len(parts) >= 2 {
			stillReferenced[parts[1]] = true
		}
	}
	tmp := indexPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(kept, "\n")+"\n"), 0644); err != nil {
		slog.Warn("write upload index failed", "err", err)
		return
	}
	if err := os.Rename(tmp, indexPath); err != nil {
		slog.Warn("replace upload index failed", "err", err)
		return
	}
	for _, name := range removed {
		if stillReferenced[name] {
			continue
		}
		if err := os.Remove(filepath.Join(uploadDir, name)); err != nil {
			slog.Warn("remove wedding upload failed", "file", name, "err", err)
		}
	}
}

// appendIndexLine records one scope's use of an already-stored file.
func appendIndexLine(indexPath, hash, filename string, size int64, scope uuid.UUID) error {
	f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "%s %s %d %s\n", hash, filename, size, scope)
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}
