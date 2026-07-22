package utils

import (
	"encoding/base64"
	"strconv"
)

func EncodeID(id uint) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(id), 10)))
}

func DecodeID(encoded string) (uint, error) {
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
