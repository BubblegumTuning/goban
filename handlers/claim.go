// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/middleware"
	"goban/models"
	"goban/services"
	"goban/sse"
)

var claimService *services.ClaimService

// dbStore is shared from handlers.go (same package)

// AuthMiddlewareWithRole is the write-path auth middleware. It is the same
// JWT-or-API-token check as auth.AuthMiddlewareWithUser (sets user and scalars).
func AuthMiddlewareWithRole(c *fiber.Ctx) error {
	return auth.AuthMiddlewareWithUser(c)
}

// HandleClaim handles POST /api/v1/tickets/:id/claim
func HandleClaim(c *fiber.Ctx) error {
	if claimService == nil {
		return auth.SendAuthError(c, "service unavailable")
	}

	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return auth.SendAuthError(c, "User not authenticated")
	}

	ticketID := c.Params("id")

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
	claimGroup.Use(middleware.TicketActionLimiter())
	claimGroup.Post("/claim", AuthMiddlewareWithRole, HandleClaim)
}
