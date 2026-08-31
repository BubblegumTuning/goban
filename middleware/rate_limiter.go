// Package middleware provides shared HTTP middleware for Goban.
package middleware

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// StrictLimiter is used on auth-sensitive endpoints like /api/auth/login.
// 5 requests per minute — prevents brute force attacks.
func StrictLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			log.Printf("[RATE-LIMIT] Request blocked by strict limiter from %s to %s", c.IP(), c.Path())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate limit exceeded",
				"message": "too many attempts — try again later",
			})
		},
	})
}

// ModerateLimiter is used on POST /api/v1/register.
// 10 requests per minute — prevents abuse while allowing normal usage.
func ModerateLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			log.Printf("[RATE-LIMIT] Request blocked by moderate limiter from %s to %s", c.IP(), c.Path())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate limit exceeded",
				"message": "too many requests — try again later",
			})
		},
	})
}

// GameLimiter is used on Go game endpoints (/api/v1/go/*) for real-time multiplayer.
// 60 requests per minute — supports ~1 move/second with room for SSE, score checks, etc.
func GameLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			log.Printf("[RATE-LIMIT] Request blocked by game limiter from %s to %s", c.IP(), c.Path())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate limit exceeded",
				"message": "too many requests — slow down",
			})
		},
	})
}

// TicketActionLimiter is used on ticket mutation endpoints (move, claim, release).
// 30 requests per minute — allows reasonable workflow speed without abuse.
func TicketActionLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        30,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			log.Printf("[RATE-LIMIT] Request blocked by ticket action limiter from %s to %s", c.IP(), c.Path())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate limit exceeded",
				"message": "too many requests — slow down",
			})
		},
	})
}
