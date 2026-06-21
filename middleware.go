// Goban Middleware - Authentication, logging, and other cross-cutting concerns
package main

import (
	"github.com/gofiber/fiber/v2"
	"goban/config"
)

// DebugLogger creates a debug middleware that logs requests when enabled
func DebugLogger(logFilter *config.LogFilter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Next()
	}
}

// Note: AuthMiddleware was removed as dead code. It was never registered in main.go
// and contained incomplete placeholder logic with TODO comments. Use auth.AuthMiddleware
// from the auth package for proper token validation instead.
