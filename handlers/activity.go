// Activity log handlers for Goban API - audit trail of ticket state changes.
package handlers

import (
	"log"
	"strconv"

	"goban/config"
	"goban/models"

	"github.com/gofiber/fiber/v2"
)

// handleGetActivityLogs retrieves activity logs for a specific ticket.
func handleGetActivityLogs(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	limitStr := c.Query("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 50 // Default to 50, max 100
	}

	logs, err := dbStore.GetActivityLogs(ticketID, limit)
	if err != nil {
		log.Printf("[HANDLER.ERROR] Failed to get activity logs for ticket %s: %v", ticketID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve activity logs"})
	}

	if len(logs) == 0 {
		return c.JSON(fiber.Map{
			"ticket_id":    ticketID,
			"activity_log": []*models.ActivityLog{},
			"total":        0,
		})
	}

	if config.Debug {
		log.Printf("[HANDLER.DEBUG] Retrieved %d activity logs for ticket %s", len(logs), ticketID)
	}
	return c.JSON(fiber.Map{
		"ticket_id":    ticketID,
		"activity_log": logs,
		"total":        len(logs),
	})
}

// RegisterActivityRoutes registers all activity-related endpoints.
func RegisterActivityRoutes(app *fiber.App) {
	// GET /api/v1/activity/:ticketId - Retrieve activity logs for a ticket
	// Using separate path prefix to avoid conflicts with /api/v1/tickets/:id routes
	app.Get("/api/v1/activity/:ticketId", handleGetActivityLogs)
	if config.Debug {
		log.Printf("DEBUG: Registered GET /api/v1/activity/:ticketId")
	}
}
