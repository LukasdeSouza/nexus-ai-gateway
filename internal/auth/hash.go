package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// HashKey computes the SHA-256 hash of the plaintext API key.
// Hex encoding ensures uniform string representation for PostgreSQL indexed lookups.
func HashKey(plaintext string) (string, error) {
	if plaintext == "" {
		return "", fmt.Errorf("plaintext key cannot be empty")
	}
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:]), nil
}

// MustHashKey returns the key hash or panics (useful in tests/migrations).
func MustHashKey(plaintext string) string {
	h, err := HashKey(plaintext)
	if err != nil {
		panic(err)
	}
	return h
}

// CompareKey compares a plaintext key with a stored hash in constant time.
func CompareKey(plaintext, storedHash string) bool {
	computed, err := HashKey(plaintext)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}
