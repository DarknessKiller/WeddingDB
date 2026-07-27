package utils

import (
	"encoding/base64"
	"strings"

	"github.com/google/uuid"
)

// EncodeUUID encodes a UUID to a URL-safe base64 string.
func EncodeUUID(id uuid.UUID) string {
	return strings.TrimRight(strings.NewReplacer("+", "_", "/", "-").Replace(
		base64.StdEncoding.EncodeToString(id[:])), "=")
}
