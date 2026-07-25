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

	// Check if file with same hash already exists
	files, _ := os.ReadDir(uploadDir)
	for _, f := range files {
		existingPath := filepath.Join(uploadDir, f.Name())
		existingFile, err := os.Open(existingPath)
		if err != nil {
			continue
		}
		existingHasher := sha256.New()
		io.Copy(existingHasher, existingFile)
		existingFile.Close()
		existingHash := hex.EncodeToString(existingHasher.Sum(nil))
		if existingHash == hash {
			return map[string]any{
				"url":      "/uploads/" + f.Name(),
				"filename": f.Name(),
				"hash":     hash,
			}, nil
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
	file.Seek(0, 0)
	if _, err := dst.ReadFrom(file); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to save file"}
	}

	return map[string]any{
		"url":      "/uploads/" + filename,
		"filename": filename,
		"hash":     hash,
	}, nil
}
