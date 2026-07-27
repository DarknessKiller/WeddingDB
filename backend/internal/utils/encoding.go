package utils

import (
	"encoding/base64"

	"github.com/google/uuid"
)

// EncodeUUID encodes a UUID to a URL-safe base64 string (no padding).
func EncodeUUID(id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(id[:])
}
