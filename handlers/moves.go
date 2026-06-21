// Move handlers for Goban API - v1 only (legacy endpoints removed).
package handlers

import (
	"errors"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/middleware"
	"goban/models"
	"goban/services"
	"goban/sse"
	"goban/store"
)

var moveService *services.MoveService

// InitMoveService initializes the move service with the store.
func InitMoveService(ticketStore store.TicketStore) {
	moveService = services.NewMoveService(ticketStore)
}

// MoveRequestV1 is for POST /api/v1/tickets/:id/move.
type MoveRequestV1 struct {
	TargetStatus string `json:"target_status"`
	Force        bool   `json:"force,omitempty"`
}

// HandleMoveV1 handles POST /api/v1/tickets/:id/move with permission checks.
func HandleMoveV1(c *fiber.Ctx) error {
	log.Println("[MOVE-V1] Handler called!")
	if moveService == nil {
		log.Println("[MOVE-V1] ERROR: moveService is nil")
		return c.Status(500).JSON(fiber.Map{
			"error":   "service_unavailable",
			"message": "Move service not initialized",
		})
	}

	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		log.Printf("[MOVE-V1] ERROR: auth failed - ok=%v user=%v\n", ok, user)
		return auth.SendAuthError(c, "User not authenticated")
	}

	ticketID := c.Params("id")
	log.Printf("[MOVE-V1] ticket_id=%s user=%s role=%s", ticketID, user.Name, user.Role)

	var req MoveRequestV1
	if err := c.BodyParser(&req); err != nil {
		log.Printf("[MOVE-V1] ERROR: body parse failed: %v\n", err)
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	log.Printf("[MOVE-V1] parsed request - target_status=%s force=%v", req.TargetStatus, req.Force)

	if req.TargetStatus == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "bad_request",
			"message": "target_status is required",
		})
	}

	// Early rejection: validate target status against known values before entering service layer.
	statusValid := false
	for _, s := range services.VALID_STATUSES {
		if req.TargetStatus == s {
			statusValid = true
			break
		}
	}
	if !statusValid {
		return c.Status(400).JSON(fiber.Map{
			"error":   "bad_request",
			"message": fmt.Sprintf("invalid target_status: %q; must be one of %v", req.TargetStatus, services.VALID_STATUSES),
		})
	}

	result, err := moveService.Move(ticketID, services.MoveRequest{
		TargetStatus: req.TargetStatus,
		Force:        req.Force,
	}, user)
	if err != nil {
		log.Printf("[MOVE-V1] ERROR from MoveService: %v\n", err)
		if errors.Is(err, services.ErrUnauthorized) {
			return auth.SendAuthError(c, "Unauthorized")
		}
		if errors.Is(err, services.ErrNotFound) {
			return c.Status(404).JSON(fiber.Map{
				"error":   "not_found",
				"message": err.Error(),
			})
		}
		if errors.Is(err, services.ErrForbidden) {
			return c.Status(403).JSON(fiber.Map{
				"error":   "forbidden",
				"message": err.Error(),
			})
		}
		if errors.Is(err, services.ErrArchived) {
			return c.Status(403).JSON(fiber.Map{
				"error":   "forbidden",
				"message": err.Error(),
			})
		}
		if errors.Is(err, services.ErrInvalidTransition) || errors.Is(err, services.ErrInvalidStatus) {
			return c.Status(400).JSON(fiber.Map{
				"error":   "bad_request",
				"message": err.Error(),
			})
		}
		log.Printf("[MOVE-V1] Generic error: ticket=%s user=%s error=%v", ticketID, user.Name, err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": err.Error(),
		})
	}

	log.Printf("[MOVE-V1] Move succeeded: ticket=%s new_status=%s column=%s",
		ticketID, req.TargetStatus, result.Ticket.Column)

	mu.Lock()
	syncTicketInMemory(result.Ticket)
	sse.Emit("move", ticketID, result.Ticket.BoardID, fiber.Map{
		"column": result.Ticket.Column,
	})
	mu.Unlock()

	log.Printf("[MOVE-V1] Returning JSON response for ticket %s", ticketID)
	return c.JSON(result.Ticket)
}

// RegisterMoveRoutesV1 registers move-related routes.
func RegisterMoveRoutesV1(app *fiber.App) {
	moveGroup := app.Group("/api/v1/tickets/:id")
	moveGroup.Use(middleware.ModerateLimiter())
	moveGroup.Post("/move", AuthMiddlewareWithRole, HandleMoveV1)
}
