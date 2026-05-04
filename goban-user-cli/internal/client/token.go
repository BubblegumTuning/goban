package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// generateToken creates a cryptographically secure API token.
func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	token := hex.EncodeToString(bytes)
	return token[:64], nil // Return first 64 chars (128 bits of entropy)
}

// hashToken creates a SHA256 hash of a token string.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
