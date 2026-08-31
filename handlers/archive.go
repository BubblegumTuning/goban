// Package handlers provides HTTP request handlers for Goban.
package handlers

import (
	"fmt"
	"log"
	"strconv"

	"goban/auth"
	"goban/models"
	"goban/sse"

	"github.com/gofiber/fiber/v2"
)

// ArchiveRequest represents the request body for archiving a ticket.
type ArchiveRequest struct {
	TicketID string `json:"ticket_id"`
}

// BulkArchiveRequest represents the request body for bulk archiving.
type BulkArchiveRequest struct {
	TicketIDs []string `json:"ticket_ids"`
}

// ArchiveResponse represents the response from an archive operation.
type ArchiveResponse struct {
	Status   string   `json:"status"`
	Count    int      `json:"count,omitempty"`
	NotFound []string `json:"not_found,omitempty"`
}

// SingleArchive handles POST /api/archive/single - archives a single ticket (admin only).
func SingleArchive(c *fiber.Ctx) error {
	// Get authenticated user from context (set by AuthMiddlewareWithUser)
	userID := c.Locals("user_id").(int64)
	username := c.Locals("username").(string)
	role := c.Locals("role").(string)

	// Check admin permission
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "forbidden: Only HUMAN_ADMIN or OVERSEER_AI can archive tickets",
		})
	}

	var req ArchiveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.TicketID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ticket_id is required"})
	}

	mu.Lock()
	defer mu.Unlock()

	// Fetch ticket before archiving to get boardID and title for SSE/activity log
	ticket, getErr := dbStore.GetTicket(req.TicketID)
	if getErr != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("ticket not found: %s", req.TicketID),
		})
	}

	err := dbStore.ArchiveTicket(req.TicketID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "failed to archive ticket",
			"details": err.Error(),
		})
	}

	// Sync in-memory state: mark as archived and remove from board columns
	syncArchivedTicketInMemory(req.TicketID)

	// Activity log (A2)
	prevActive := "active"
	newArchived := "archived"
	if _, err := dbStore.CreateActivityLog(&models.ActivityLog{
		TicketID:  req.TicketID,
		EventType: models.ActivityArchived,
		Actor:     username,
		PrevState: &prevActive,
		NewState:  &newArchived,
	}); err != nil {
		log.Printf("Warning: Failed to create archive activity log for ticket %s: %v", req.TicketID, err)
	}

	// SSE event (A3) — emitted after DB commit to avoid broadcasting uncommitted changes
	sse.Emit("archive", req.TicketID, ticket.BoardID, fiber.Map{"title": ticket.Title})

	return c.JSON(ArchiveResponse{Status: "archived", Count: 1})
}

// BulkArchive handles POST /api/archive/bulk - archives multiple tickets (admin only).
func BulkArchive(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int64)
	username := c.Locals("username").(string)
	role := c.Locals("role").(string)

	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "forbidden: Only HUMAN_ADMIN or OVERSEER_AI can archive tickets",
		})
	}

	var req BulkArchiveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if len(req.TicketIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ticket_ids array is required and must not be empty",
		})
	}

	mu.Lock()
	defer mu.Unlock()

	// Check which tickets exist before archiving; store info for activity log + SSE
	notFound := make([]string, 0)
	var existingIDs []string
	existingTickets := make(map[string]*models.Ticket)
	for _, ticketID := range req.TicketIDs {
		ticket, getErr := dbStore.GetTicket(ticketID)
		if getErr != nil {
			notFound = append(notFound, ticketID)
		} else {
			existingIDs = append(existingIDs, ticketID)
			existingTickets[ticketID] = ticket
		}
	}

	// Archive only existing tickets
	var err error
	if len(existingIDs) > 0 {
		err = dbStore.ArchiveTicketsBulk(existingIDs, userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "failed to archive tickets",
				"details": err.Error(),
			})
		}

		// Sync in-memory state: mark each archived ticket and remove from board columns
		for _, ticketID := range existingIDs {
			syncArchivedTicketInMemory(ticketID)
		}

		// Activity logs + SSE events for each archived ticket (A2+A3)
		prevActive := "active"
		newArchived := "archived"
		for _, ticketID := range existingIDs {
			ticket := existingTickets[ticketID]
			if _, err := dbStore.CreateActivityLog(&models.ActivityLog{
				TicketID:  ticketID,
				EventType: models.ActivityArchived,
				Actor:     username,
				PrevState: &prevActive,
				NewState:  &newArchived,
			}); err != nil {
				log.Printf("Warning: Failed to create archive activity log for ticket %s: %v", ticketID, err)
			}

			sse.Emit("archive", ticketID, ticket.BoardID, fiber.Map{"title": ticket.Title})
		}
	}

	return c.JSON(ArchiveResponse{Status: "archived", Count: len(existingIDs), NotFound: notFound})
}

// GetArchivedByAdmin handles GET /api/archive/by-admin/:adminID - lists tickets archived by an admin (admin only).
func GetArchivedByAdmin(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int64)
	role := c.Locals("role").(string)

	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "forbidden: Only HUMAN_ADMIN or OVERSEER_AI can view archived tickets by admin",
		})
	}

	adminIDStr := c.Params("admin_id")
	if adminIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "admin_id is required"})
	}

	adminID, err := strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid admin_id format"})
	}

	mu.Lock()
	defer mu.Unlock()

	tickets, err := dbStore.GetArchivedByAdmin(adminID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "failed to retrieve archived tickets",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"tickets": tickets, "archived_by_user_id": userID})
}

// GetAllArchived handles GET /api/archived - lists all archived tickets (auth required).
func GetAllArchived(c *fiber.Ctx) error {
	_ = c.Locals("user_id").(int64) // Ensure user is authenticated

	mu.RLock()
	defer mu.RUnlock()

	// Get board_id from query param, or empty string to get all boards
	boardID := c.Query("board_id", "")

	var tickets []*models.Ticket
	var err error

	if boardID != "" {
		tickets, err = dbStore.GetArchivedTickets(boardID)
	} else {
		// Get archived from all boards by querying each known board
		// For simplicity, just return all archived tickets regardless of board
		tickets, err = dbStore.GetAllArchivedTickets()
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "failed to retrieve archived tickets",
			"details": err.Error(),
		})
	}

	return c.JSON(tickets)
}

// UnarchiveRequest represents the request body for restoring an archived ticket.
type UnarchiveRequest struct {
	BoardID string `json:"board_id"`
	Column  string `json:"column"`
}

// UnarchiveTicket handles POST /api/unarchive/:ticketId - restores a ticket from archive (admin only).
func UnarchiveTicket(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int64)
	username := c.Locals("username").(string)
	role := c.Locals("role").(string)

	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "forbidden: Only HUMAN_ADMIN or OVERSEER_AI can unarchive tickets",
		})
	}

	ticketID := c.Params("ticket_id")
	if ticketID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ticket_id is required"})
	}

	var req UnarchiveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	mu.Lock()
	defer mu.Unlock()

	// Fetch ticket before unarchiving to get boardID and title for SSE/activity log
	preTicket, preErr := dbStore.GetTicket(ticketID)
	if preErr != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("ticket not found: %s", ticketID),
		})
	}

	// Determine target board/column for SSE event
	targetBoardID := preTicket.BoardID
	if req.BoardID != "" {
		targetBoardID = req.BoardID
	}
	targetColumn := req.Column
	if targetColumn == "" {
		targetColumn = preTicket.Column
	}

	// First, unarchive the ticket (clear archived flags)
	err := dbStore.UnarchiveTicket(ticketID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "failed to unarchive ticket",
			"details": err.Error(),
		})
	}

	// Then update board_id and column if provided
	if req.BoardID != "" || req.Column != "" {
		ticket, getErr := dbStore.GetTicket(ticketID)
		if getErr == nil && ticket != nil {
			if req.BoardID != "" {
				ticket.BoardID = req.BoardID
			}
			if req.Column != "" {
				ticket.Column = models.GetColumnID(req.Column)
			}
			ticket.Archived = false
			ticket.ArchivedAt = nil
			updateErr := dbStore.UpdateTicket(ticket)
			if updateErr != nil {
				log.Printf("ERROR: failed to update ticket after unarchive %s: %v", ticketID, updateErr)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error":   "failed to update ticket metadata after unarchive",
					"details": updateErr.Error(),
				})
			}
		}
	}

	// Activity log (A2)
	prevArchived := "archived"
	newRestored := fmt.Sprintf("restored to %s/%s", targetBoardID, targetColumn)
	if _, err := dbStore.CreateActivityLog(&models.ActivityLog{
		TicketID:  ticketID,
		EventType: models.ActivityRestored,
		Actor:     username,
		PrevState: &prevArchived,
		NewState:  &newRestored,
	}); err != nil {
		log.Printf("Warning: Failed to create restore activity log for ticket %s: %v", ticketID, err)
	}

	// SSE event (A3) — emitted after DB commit
	sse.Emit("unarchive", ticketID, targetBoardID, fiber.Map{"column": targetColumn})

	return c.JSON(fiber.Map{"status": "restored", "ticket_id": ticketID, "restored_by_user_id": userID})
}

// syncArchivedTicketInMemory marks a ticket as archived and removes it from boardStates.
// This function MUST be called with mu.Lock() held by the caller.
func syncArchivedTicketInMemory(ticketID string) {
	for _, board := range boardStates {
		for _, col := range board.Columns {
			for i, t := range col.Tickets {
				if t.ID == ticketID {
					t.Archived = true
					col.Tickets = append(col.Tickets[:i], col.Tickets[i+1:]...)
					log.Printf("syncArchivedTicketInMemory: Removed archived ticket %s from board %s column %s", ticketID, board.ID, col.ID)
					return
				}
			}
		}
	}
	log.Printf("Warning: syncArchivedTicketInMemory - Ticket %s not found in memory", ticketID)
}

// RegisterArchiveRoutes sets up archive-related API endpoints.
func RegisterArchiveRoutes(app *fiber.App) {
	log.Println("[ARCHIVE] Registering archive routes...")

	// Register all archive routes directly on app to avoid group prefix conflicts.
	// Note: /api/archived must be registered BEFORE any wildcard or broader patterns,
	// and using direct registration avoids Fiber's issue with overlapping group prefixes
	// (/api/archive vs /api/archived).

	app.Post("/api/archive/single", auth.AuthMiddlewareWithUser, SingleArchive)                 // Archive single ticket
	app.Post("/api/archive/bulk", auth.AuthMiddlewareWithUser, BulkArchive)                     // Bulk archive tickets
	app.Get("/api/archive/by-admin/:admin_id", auth.AuthMiddlewareWithUser, GetArchivedByAdmin) // List archived by admin ID
	app.Get("/api/archived", auth.AuthMiddlewareWithUser, GetAllArchived)                       // List all archived tickets (frontend)
	app.Post("/api/unarchive/:ticket_id", auth.AuthMiddlewareWithUser, UnarchiveTicket)         // Restore ticket from archive
}
