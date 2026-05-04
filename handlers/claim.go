// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/config"
	"goban/middleware"
	"goban/models"
	"goban/services"
	"goban/sse"
)

var claimService *services.ClaimService

// dbStore is shared from handlers.go (same package)

// AuthMiddlewareWithRole validates Bearer token and stores user info in context.
func AuthMiddlewareWithRole(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return auth.SendAuthError(c, "missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return auth.SendAuthError(c, "invalid authorization format (expected 'Bearer <token>')")
	}

	tokenString := parts[1]
	if tokenString == "" {
		return auth.SendAuthError(c, "empty token")
	}

	// Try JWT first (web UI / human login tokens)
	if claims, err := auth.VerifyJWT(tokenString); err == nil {
		// Validate that the token contains required identity fields
		if claims.UserID == 0 || claims.Username == "" {
			return auth.SendAuthError(c, "invalid or expired token: missing user identity in JWT")
		}
		user := &models.User{
			ID:   claims.UserID,
			Name: claims.Username,
			Role: claims.Role,
		}
		c.Locals("user", user)
		return c.Next()
	}

	// Fallback: API token path (regenerated admin tokens that carry human_admin role)
	if user, err := auth.ValidateTokenWithRole(tokenString); err == nil {
		c.Locals("user", user)
		return c.Next()
	}

	// Neither path succeeded
	return auth.SendAuthError(c, "invalid or expired token (neither JWT nor API token accepted)")
}

// HandleClaim handles POST /api/v1/tickets/:id/claim
func HandleClaim(c *fiber.Ctx) error {
	log.Printf("[CLAIM] HandleClaim called - claimService is nil: %v", claimService == nil)
	if claimService == nil {
		return auth.SendAuthError(c, "service unavailable")
	}

	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		log.Printf("[CLAIM] Auth failed - ok=%v, user=%v", ok, user)
		return auth.SendAuthError(c, "User not authenticated")
	}

	ticketID := c.Params("id")
	log.Printf("[CLAIM] Claim request: ticket=%s user=%s role=%s", ticketID, user.Name, user.Role)

	result, err := claimService.Claim(ticketID, user)
	if err != nil {
		// Check for specific error types to return proper status codes
		if errors.Is(err, services.ErrUnauthorized) {
			return auth.SendAuthError(c, "Unauthorized")
		}
		if errors.Is(err, services.ErrNotFound) {
			return c.Status(404).JSON(fiber.Map{
				"error":   "not_found",
				"message": "Ticket not found",
			})
		}
		if errors.Is(err, services.ErrForbidden) {
			return c.Status(403).JSON(fiber.Map{
				"error":   "forbidden",
				"message": err.Error(),
			})
		}
		// Generic error
		log.Printf("Claim failed: ticket=%s user=%s error=%v", ticketID, user.Name, err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": err.Error(),
		})
	}

	log.Printf("Claim succeeded: ticket=%s user=%s auto_released=%v",
		ticketID, user.Name, result.AutoReleased)

	mu.Lock()
	syncTicketInMemory(result.Ticket)
	sse.Emit("claim", ticketID, result.Ticket.BoardID, fiber.Map{
		"assignee": result.Ticket.Assignee,
	})
	if len(result.AutoReleased) > 0 {
		for _, releasedID := range result.AutoReleased {
			if releasedTicket, err := dbStore.GetTicket(releasedID); err == nil && releasedTicket != nil {
				syncTicketInMemory(releasedTicket)
				sse.Emit("release", releasedID, releasedTicket.BoardID, fiber.Map{
					"assignee": releasedTicket.Assignee,
				})
			}
		}
	}
	mu.Unlock()

	return c.JSON(result)
}

// RegisterClaimRoutes registers claim-related routes.
func RegisterClaimRoutes(app *fiber.App) {
	claimGroup := app.Group("/api/v1/tickets/:id")
	claimGroup.Use(middleware.ModerateLimiter())
	claimGroup.Post("/claim", AuthMiddlewareWithRole, HandleClaim)
	if config.Debug {
		log.Println("DEBUG: Registered POST /api/v1/tickets/:id/claim with rate limiting")
	}
}
