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
	"goban/store"
)

var releaseService *services.ReleaseService

// InitReleaseService initializes the release service with the store.
func InitReleaseService(ticketStore store.TicketStore) {
	releaseService = services.NewReleaseService(ticketStore)
}

// HandleRelease handles POST /api/v1/tickets/:id/release
func HandleRelease(c *fiber.Ctx) error {
	if releaseService == nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "service_unavailable",
			"message": "Release service not initialized",
		})
	}

	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return auth.SendAuthError(c, "User not authenticated")
	}

	ticketID := c.Params("id")
	log.Printf("Release request: ticket=%s user=%s role=%s", ticketID, user.Name, user.Role)

	result, err := releaseService.Release(ticketID, user)
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
		if errors.Is(err, services.ErrArchived) {
			return c.Status(403).JSON(fiber.Map{
				"error":   "forbidden",
				"message": err.Error(),
			})
		}
		if errors.Is(err, services.ErrForbidden) {
			return c.Status(403).JSON(fiber.Map{
				"error":   "forbidden",
				"message": err.Error(),
			})
		}
		// Generic error
		log.Printf("Release failed: ticket=%s user=%s error=%v", ticketID, user.Name, err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": err.Error(),
		})
	}

	log.Printf("Release succeeded: ticket=%s user=%s released_as=%s",
		ticketID, user.Name, result.ReleasedAs)

	// Sync in-memory cache to ensure Web UI displays updated data
	mu.Lock()
	syncTicketInMemory(result.Ticket)
	sse.Emit("release", ticketID, result.Ticket.BoardID, fiber.Map{
		"assignee": result.Ticket.Assignee,
	})
	mu.Unlock()

	return c.JSON(result.Ticket)
}

// RegisterReleaseRoutes registers release-related routes.
func RegisterReleaseRoutes(app *fiber.App) {
	releaseGroup := app.Group("/api/v1/tickets/:id")
	releaseGroup.Use(middleware.ModerateLimiter())
	releaseGroup.Post("/release", AuthMiddlewareWithRole, HandleRelease)
}
