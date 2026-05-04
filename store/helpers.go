// Package store provides database abstraction for Goban persistence.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// safeMarshal handles JSON marshaling with proper error handling.
func safeMarshal(v interface{}, fieldName string) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", fieldName, err)
	}
	return data, nil
}

// parseTimeFromRFC3339 converts an RFC3339 string to time.Time.
func parseTimeFromRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		log.Printf("Warning: Failed to parse time %s: %v", s, err)
		return time.Now()
	}
	return t
}

// =============================================================================
// Password Hashing (bcrypt) - Added 2026-04-22 for ticket-e0a4c2d9d8

// HashPassword creates a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword compares a plaintext password with its bcrypt hash.
func VerifyPassword(hash, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify password: %w", err)
	}
	return true, nil
}
