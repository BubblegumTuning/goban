// Ticket CRUD handlers for Goban API.
package handlers

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/models"
	"goban/sse"
	"goban/store"
	"goban/validation"
)

// normalizePriority returns the canonical lowercase form of a priority value.
// Assumes validation.ValidatePriority has already been called on the input.
func normalizePriority(priority string) string {
	if norm, ok := validation.NormalizePriority(priority); ok {
		return norm
	}
	return priority
}

func handleDeleteTicket(c *fiber.Ctx) error {
	id := c.Params("id")

	// Require force=true query parameter as a safety guard against accidental deletions.
	force := c.Query("force", "false")
	if force != "true" && force != "1" && force != "yes" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "confirmation_required",
			"message": "Deletion requires ?force=true as a safety confirmation.",
		})
	}

	mu.Lock()
	defer mu.Unlock()

	// Authoritative lookup: query DB first so delete works even if ticket
	// is missing from stale in-memory boardStates cache (common after moves/restarts).
	// This matches the pattern used by move/claim/release services.
	var ticket *models.Ticket
	if dbStore != nil {
		var err error
		ticket, err = dbStore.GetTicket(id)
		if err != nil || ticket == nil {
			return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
		}
	}

	// Remove from in-memory state if present (handles cache desync safely)
	for _, state := range boardStates {
		for _, col := range state.Columns {
			for idx, t := range col.Tickets {
				if t.ID == id {
					col.Tickets = append(col.Tickets[:idx], col.Tickets[idx+1:]...)
					break
				}
			}
		}
	}

	// Persist the deletion to the database
	if dbStore != nil {
		if err := dbStore.DeleteTicket(id); err != nil {
			log.Printf("ERROR: Failed to delete ticket %s from database: %v", id, err)
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to persist deletion: %v", err)})
		}
	}

	log.Printf("Deleted ticket %s", id)

	// Emit SSE with details from DB (reliable boardID + title)
	boardID := "unknown"
	title := ""
	if ticket != nil {
		boardID = ticket.BoardID
		title = ticket.Title
	}
	sse.Emit("delete", id, boardID, fiber.Map{
		"title": title,
	})

	return c.JSON(fiber.Map{"status": "deleted"})
}

func handleUpdateTicket(c *fiber.Ctx) error {
	id := c.Params("id")

	// Partial update support to prevent clearing unspecified fields
	// Uses pointers so BodyParser only sets fields present in JSON payload.
	type UpdateRequest struct {
		Title       *string           `json:"title"`
		Description *string           `json:"description"`
		Priority    *string           `json:"priority"`
		Assignee    *string           `json:"assignee"`
		DueDate     *string           `json:"due_date"`
		Subtasks    *[]models.Subtask `json:"subtasks"`
		Labels      *[]string         `json:"labels"`
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate input fields
	if req.Title != nil {
		if err := validation.ValidateTitle(*req.Title); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if req.Description != nil {
		if err := validation.ValidateDescription(*req.Description); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if req.Priority != nil {
		if err := validation.ValidatePriority(*req.Priority); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if req.Assignee != nil {
		if err := validation.ValidateAssignee(*req.Assignee); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if req.Labels != nil {
		if err := validation.ValidateLabels(*req.Labels); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if req.Subtasks != nil {
		for _, st := range *req.Subtasks {
			if err := validation.ValidateSubtaskTitle(st.Title); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}
	}

	mu.Lock()
	defer mu.Unlock()

	t, err := loadTicketFromDB(id)
	if err != nil || t == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
	}

	if req.Title != nil {
		t.Title = *req.Title
	}
	if req.Description != nil {
		t.Description = *req.Description
	}
	if req.Priority != nil {
		t.Priority = normalizePriority(*req.Priority)
	}
	if req.Assignee != nil {
		t.Assignee = *req.Assignee
	}
	if req.DueDate != nil {
		val := *req.DueDate
		t.DueDate = &val
	}
	if req.Subtasks != nil {
		t.Subtasks = *req.Subtasks
	}
	if req.Labels != nil {
		t.Labels = *req.Labels
	}
	t.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := saveTicketToDB(t); err != nil {
		log.Printf("ERROR: Failed to persist ticket %s to database: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to persist update: %v", err)})
	}
	replaceTicketInCache(t)

	log.Printf("Updated ticket %s (partial update) - persisted to DB", id)

	sse.Emit("update", id, t.BoardID, fiber.Map{
		"title":  t.Title,
		"column": t.Column,
	})

	return c.JSON(t)
}

func saveTicketToDB(ticket *models.Ticket) error {
	if dbStore != nil {
		return dbStore.UpdateTicket(ticket)
	}
	return nil
}

// CreateRequestSimple is for the simplified /api/tickets endpoint (production compatible)
type CreateRequestSimple struct {
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Assignee       string   `json:"assignee,omitempty"`
	DueDate        string   `json:"due_date,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	BoardID        string   `json:"board_id"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	Column         string   `json:"column,omitempty"` // Target column (e.g., "backlog", "todo", "inprogress")
}

// handleCreateTicketSimple is the production-compatible endpoint that takes board_id in body
func handleCreateTicketSimple(c *fiber.Ctx) error {
	var req CreateRequestSimple

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate input fields
	if err := validation.ValidateTitle(req.Title); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidateDescription(req.Description); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidatePriority(req.Priority); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidateAssignee(req.Assignee); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidateLabels(req.Labels); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	boardID := req.BoardID
	if boardID == "" {
		boardID = "human-to-ai" // Default board
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := knownBoardIDs[boardID]; !ok {
		return c.Status(404).JSON(fiber.Map{"error": "Board not found"})
	}

	board := boardStates[boardID]

	// Get idempotency key from header or query parameter if not in body
	idempKey := req.IdempotencyKey
	if idempKey == "" {
		idempKey = c.Get("X-Idempotency-Key")
	}
	if idempKey == "" {
		idempKey = c.Query("key")
	}

	// Default to "todo" if no column specified; resolve to canonical ID via GetColumnID.
	column := req.Column
	if column == "" {
		column = "todo"
	}

	newTicket := &models.Ticket{
		ID:             models.GenerateTicketID(),
		Title:          req.Title,
		Description:    req.Description,
		Priority:       normalizePriority(req.Priority),
		Assignee:       req.Assignee,
		Labels:         req.Labels,
		BoardID:        boardID,
		IdempotencyKey: idempKey,
		CreatedAt:      time.Now().Format(time.RFC3339),
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}

	// Resolve the column to its canonical ID and assign it
	newTicket.Column = models.GetColumnID(column)

	// Set DueDate only if provided (needs pointer conversion)
	if req.DueDate != "" {
		newTicket.DueDate = &req.DueDate
	}

	targetColID := models.GetColumnID(newTicket.Column)
	if board != nil {
		for _, col := range board.Columns {
			if col.ID == targetColID {
				col.Tickets = append(col.Tickets, newTicket)
				break
			}
		}
	}

	// Persist to database — use CreateOrGetTicket for idempotent creation
	var createdTicket *models.Ticket
	if dbStore != nil {
		var createErr error
		createdTicket, createErr = dbStore.CreateOrGetTicket(newTicket)
		if createErr != nil {
			log.Printf("ERROR: Failed to create ticket via CreateOrGetTicket: %v", createErr)
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save ticket: %v", createErr)})
		}
	} else {
		createdTicket = newTicket
	}

	log.Printf("Created ticket %s on board %s (simple endpoint)", createdTicket.ID, boardID)

	sse.Emit("create", createdTicket.ID, boardID, fiber.Map{
		"title":  createdTicket.Title,
		"column": createdTicket.Column,
	})

	// Return 201 for newly created, 200 for existing (idempotent lookup)
	if createdTicket == newTicket {
		return c.Status(201).JSON(createdTicket)
	}
	return c.Status(200).JSON(createdTicket)
}

// PaginatedTicketsResponse wraps paginated tickets with metadata.
type PaginatedTicketsResponse struct {
	Tickets []*models.Ticket `json:"tickets"`
	Columns []string         `json:"columns"` // Which column prefixes are included in this response
	Total   int64            `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	HasMore bool             `json:"has_more"`
}

// handleListTicketsPaginated returns tickets with pagination and optional column filtering.
func handleListTicketsPaginated(ticketStore store.TicketStore) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		// Parse board_id filter from query parameters (optional)
		boardID := c.Query("board_id")

		// Parse view mode from query parameters
		includeBacklog := c.Query("include") == "backlog"
		viewFull := c.Query("view") == "full"

		// Determine which column prefixes to include based on view mode
		var allowedColumns []string
		if viewFull {
			allowedColumns = []string{"backlog", "todo", "inprogress", "review", "done", "cancelled"}
		} else if includeBacklog {
			allowedColumns = []string{"backlog", "todo"}
		} else {
			allowedColumns = []string{"todo"} // Default: TODO only (lightweight view)
		}

		// Parse pagination params
		limitStr := c.Query("limit", "50")
		offsetStr := c.Query("offset", "0")

		var limit, offset int
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		} else if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		} else if limit <= 0 {
			limit = 50
		}
		if limit > 1000 {
			limit = 1000 // max cap
		}
		if offset < 0 {
			offset = 0
		}

		p := store.Pagination{Limit: limit, Offset: offset}

		tickets, totalCount, err := ticketStore.GetTicketsWithFilter(allowedColumns, p)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch tickets: %v", err)})
		}

		// Apply board_id filter if specified (filter in memory after fetching from DB)
		if boardID != "" && len(tickets) > 0 {
			filtered := make([]*models.Ticket, 0, len(tickets))
			for _, t := range tickets {
				if t.BoardID == boardID {
					filtered = append(filtered, t)
				}
			}
			tickets = filtered
			totalCount = int64(len(tickets)) // Update count to reflect filtered results
		}

		resp := PaginatedTicketsResponse{
			Tickets: tickets,
			Columns: allowedColumns,
			Total:   totalCount,
			Limit:   limit,
			Offset:  offset,
			HasMore: int64(offset)+int64(len(tickets)) < totalCount,
		}

		return c.JSON(resp)
	}
}

// GetSingleTicketResponse wraps a ticket for single-ticket responses.
type GetSingleTicketResponse struct {
	Ticket *models.Ticket `json:"ticket"`
}

// HandleGetTicket returns a single ticket by ID from the database.
func HandleGetTicket(c *fiber.Ctx) error {
	id := c.Params("id")

	ticket, err := dbStore.GetTicket(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": fmt.Sprintf("Ticket not found: %v", err)})
	}

	return c.JSON(ticket)
}

// RegisterTicketRoutes sets up all ticket-related API routes.
func RegisterTicketRoutes(app *fiber.App, ts store.TicketStore) {
	// Public read endpoints — no auth required.
	publicTickets := app.Group("/api/tickets")
	publicTickets.Get("/", handleListTicketsPaginated(ts))
	publicTickets.Get("/:id", HandleGetTicket)

	// Write endpoints — require authentication.
	ticketGroup := app.Group("/api/tickets", AuthMiddlewareWithRole)
	ticketGroup.Post("/", func(c *fiber.Ctx) error { return handleCreateTicketSimple(c) })
	ticketGroup.Put("/:id", func(c *fiber.Ctx) error { return handleUpdateTicket(c) })
	ticketGroup.Patch("/:id", func(c *fiber.Ctx) error { return handleUpdateTicket(c) })
	ticketGroup.Delete("/:id", func(c *fiber.Ctx) error { return handleDeleteTicket(c) })

	// Task link endpoints (POST/GET/DELETE /api/tickets/:id/links)
	ticketGroup.Post("/:id/links", handleAddLink)
	publicTickets.Get("/:id/links", handleGetLinks)
	ticketGroup.Delete("/:id/links", handleRemoveLink)

	// Run history endpoints (POST/GET/PUT /api/tickets/:id/runs)
	ticketGroup.Post("/:id/runs", handleCreateRun)
	publicTickets.Get("/:id/runs", handleGetRuns)
	publicTickets.Get("/:id/runs/active", handleGetActiveRun)
	ticketGroup.Put("/:id/runs", handleUpdateRun)

	// Public v1 read endpoints — no auth required.
	app.Get("/api/v1/tickets", handleListTicketsPaginated(ts))

	// Get single ticket by ID (v1 API - public read)
	app.Get("/api/v1/tickets/:id", HandleGetTicket)
}
