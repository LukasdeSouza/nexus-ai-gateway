// Package auth provides API key generation, cryptographic hashing, and validation.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	// KeyPrefix is the default prefix for Nexus Gateway keys.
	KeyPrefix = "ngk_"
)

// GenerateKey generates a cryptographically secure random API key.
// Returns the full plaintext key (shown to the user only once) and its display prefix.
func GenerateKey(env string) (plaintext string, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(b)

	var pfx string
	switch env {
	case "test", "development":
		pfx = "ngk_test_"
	case "live", "production":
		pfx = "ngk_live_"
	default:
		pfx = KeyPrefix
	}

	fullKey := pfx + encoded
	displayPrefix := ExtractPrefix(fullKey)

	return fullKey, displayPrefix, nil
}

// ExtractPrefix returns the first 12 characters of a key for safe display and logging.
func ExtractPrefix(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12] + "..."
}

// IsValidKeyFormat checks if key begins with the ngk_ prefix.
func IsValidKeyFormat(key string) bool {
	return strings.HasPrefix(key, KeyPrefix)
}
