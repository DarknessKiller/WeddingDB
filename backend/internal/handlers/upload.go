package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var magicBytes = map[string][]byte{
	".jpg":  {0xff, 0xd8, 0xff},
	".jpeg": {0xff, 0xd8, 0xff},
	".png":  {0x89, 0x50, 0x4e, 0x47},
	".gif":  {0x47, 0x49, 0x46, 0x38},
	".webp": {0x52, 0x49, 0x46, 0x46}, // RIFF header
	".svg":  {0x3c, 0x3f, 0x78, 0x6d, 0x6c}, // <?xml
}

type UploadHandler struct{}

func NewUploadHandler() *UploadHandler {
	return &UploadHandler{}
}

func (h *UploadHandler) Upload(c fuego.ContextWithBody[any]) (any, error) {
	file, header, err := c.Request().FormFile("file")
	if err != nil {
		return nil, fuego.BadRequestError{Title: "No file provided"}
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true}
	if !allowed[ext] {
		return nil, fuego.BadRequestError{Title: "File type not allowed. Use JPG, PNG, GIF, WebP, or SVG"}
	}

	// Validate magic bytes
	headerBuf := make([]byte, 16)
	n, err := file.Read(headerBuf)
	if err != nil {
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

	// Validate size (max 5MB)
	if header.Size > 5*1024*1024 {
		return nil, fuego.BadRequestError{Title: "File too large. Max 5MB"}
	}

	// Create uploads directory
	uploadDir := "./uploads"
	os.MkdirAll(uploadDir, 0755)

	// Compute SHA256 hash of file content
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to read file"}
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	// Check if file with same hash already exists via index file
	indexPath := filepath.Join(uploadDir, ".hash-index")
	if idx, err := os.ReadFile(indexPath); err == nil {
		for _, line := range strings.Split(string(idx), "\n") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 && parts[0] == hash {
				return map[string]any{
					"url":      "/uploads/" + parts[1],
					"filename": parts[1],
					"hash":     hash,
				}, nil
			}
		}
	}

	// Save new file with hash as prefix
	filename := fmt.Sprintf("%s-%s%s", hash[:8], uuid.New().String()[:8], ext)
	filePath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	defer dst.Close()

	// Reset file pointer to beginning for writing
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}
	if _, err := dst.ReadFrom(file); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}

	// Append hash to index file
	f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", hash, filename)
		f.Close()
	}

	return map[string]any{
		"url":      "/uploads/" + filename,
		"filename": filename,
		"hash":     hash,
	}, nil
}
