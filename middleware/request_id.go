// Package middleware provides shared HTTP middleware for Goban.
package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
)

const requestIDKey = "request_id"

// RequestID generates a unique ID for every incoming request and attaches it
// to the Fiber context under key "request_id". It also sets the X-Request-ID
// response header so clients can correlate logs with their requests.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := make([]byte, 8)
		if _, err := rand.Read(requestID); err != nil {
			// Fallback: use a simple counter-like ID (should never happen in practice)
			c.Locals(requestIDKey, "unknown")
			return c.Next()
		}

		id := hex.EncodeToString(requestID)
		c.Locals(requestIDKey, id)
		c.Set("X-Request-ID", id)
		return c.Next()
	}
}
