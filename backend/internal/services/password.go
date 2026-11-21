package services

import (
	"crypto/sha256"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordLen is the user-facing limit, counted in characters.
const MaxPasswordLen = 72

// bcrypt refuses inputs over 72 bytes. Hashing a longer password's SHA-256
// first keeps the limit in characters: 72 Chinese characters are 216 bytes,
// and refusing those would contradict the form.
const (
	bcryptMaxBytes = 72
	sha256Prefix   = "{sha256}"
)

// HashPassword hashes a password that already passed the character checks.
func HashPassword(password string) (string, error) {
	if len(password) <= bcryptMaxBytes {
		h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		return string(h), err
	}
	sum := sha256.Sum256([]byte(password))
	h, err := bcrypt.GenerateFromPassword(sum[:], bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	// The marker tells ComparePassword how the input was prepared.
	return sha256Prefix + string(h), nil
}

// ComparePassword checks a password against a stored hash, including hashes
// made from a SHA-256 pre-hash.
func ComparePassword(hash, password string) error {
	if plain, ok := strings.CutPrefix(hash, sha256Prefix); ok {
		sum := sha256.Sum256([]byte(password))
		return compareHash([]byte(plain), sum[:])
	}
	return compareHash([]byte(hash), []byte(password))
}
