package utils

import (
	"encoding/base64"

	"github.com/google/uuid"
)

// EncodeUUID encodes a UUID to a URL-safe base64 string (no padding).
func EncodeUUID(id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(id[:])
}

// DecodeUUID parses a plain UUID string or its URL-safe base64 form, with or
// without padding. It is the one decoder for wedding, guest, table and cursor
// IDs.
func DecodeUUID(s string) (uuid.UUID, error) {
	if id, err := uuid.Parse(s); err == nil {
		return id, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		if decoded, err = base64.URLEncoding.DecodeString(s); err != nil {
			return uuid.Nil, err
		}
	}
	return uuid.FromBytes(decoded)
}
