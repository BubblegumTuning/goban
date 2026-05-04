// Board initialization and retrieval handlers.
package handlers

import (
	"log"
	"sort"

	"github.com/gofiber/fiber/v2"
	"goban/config"
	"goban/models"
)

func InitBoards(boards []config.Board, store TicketStore) {
	boardStates = make(map[string]*models.BoardState)

	for _, boardConfig := range boards {
		boardState := &models.BoardState{
			ID:    boardConfig.ID,
			Title: boardConfig.Title,
			Desc:  boardConfig.Desc,
		}

		for _, colTitle := range boardConfig.Columns {
			colID := models.GetColumnID(colTitle)
			boardState.Columns = append(boardState.Columns, &models.Column{
				ID:      colID,
				Title:   colTitle,
				Tickets: make([]*models.Ticket, 0),
			})
		}

		boardStates[boardConfig.ID] = boardState
		log.Printf("Initialized board: %s (%d columns)", boardConfig.ID, len(boardState.Columns))
	}

	loadTicketsFromDB(store)
	// [AUDIT]: Global boardStates map (pointers) + mu mutex used for all handlers.
	// TicketIndex rebuild/update/remove was disabled ('index never read') to avoid kanban-index-sync-bugs
	// and go-range-iteration-copy-pitfall desync risks between memory/DB/frontend. Current mutex-protected
	// global state follows "working over fancy". Index var remains in handlers.go but unused.

	log.Printf("Loaded %d boards with %d total tickets", len(boards), countTotalTickets())
}

func loadTicketsFromDB(store TicketStore) {
	tickets, err := store.GetAllTickets()
	if err != nil {
		log.Printf("Warning: Failed to load tickets from DB: %v", err)
		return
	}

	for _, ticket := range tickets {
		addTicketToMemory(ticket)
	}

	log.Printf("Loaded %d tickets from database", len(tickets))
}

func addTicketToMemory(ticket *models.Ticket) {
	board, exists := boardStates[ticket.BoardID]
	if !exists {
		log.Printf("Warning: Ticket %s references unknown board %s", ticket.ID, ticket.BoardID)
		return
	}

	targetColumnID := models.GetColumnID(ticket.Column)

	for _, col := range board.Columns {
		if col.ID == targetColumnID {
			col.Tickets = append(col.Tickets, ticket)
			break
		}
	}
}

func countTotalTickets() int {
	count := 0
	for _, board := range boardStates {
		for _, col := range board.Columns {
			count += len(col.Tickets)
		}
	}
	return count
}

// syncTicketInMemory updates the in-memory boardStates cache to match database state.
// This function MUST be called with mu.Lock() held by the caller.
// It removes the ticket from its old column and adds it to the new column based on ticket.Column.
func syncTicketInMemory(ticket *models.Ticket) {
	if ticket == nil {
		return
	}

	board, exists := boardStates[ticket.BoardID]
	if !exists {
		log.Printf("Warning: syncTicketInMemory - Board %s not found for ticket %s",
			ticket.BoardID, ticket.ID)
		return
	}

	targetColumnID := models.GetColumnID(ticket.Column)

	// Step 1: Remove ticket from ALL columns (clean slate approach)
	for _, col := range board.Columns {
		for i, t := range col.Tickets {
			if t.ID == ticket.ID {
				col.Tickets = append(col.Tickets[:i], col.Tickets[i+1:]...)
				break
			}
		}
	}

	// Step 2: Add ticket to the correct column based on its current Column field
	for _, col := range board.Columns {
		if col.ID == targetColumnID {
			col.Tickets = append(col.Tickets, ticket)
			log.Printf("syncTicketInMemory: Ticket %s synced to column %s", ticket.ID, targetColumnID)
			return
		}
	}

	log.Printf("Warning: syncTicketInMemory - Column %s not found for ticket %s",
		targetColumnID, ticket.ID)
}

func handleListBoards(c *fiber.Ctx) error {
	mu.RLock()
	defer mu.RUnlock()

	_, truncateDesc := models.GetCompactLevel(c)

	// Collect board IDs and sort for consistent ordering
	boardIDs := make([]string, 0, len(boardStates))
	for id := range boardStates {
		boardIDs = append(boardIDs, id)
	}
	sort.Strings(boardIDs)

	result := make([]*models.CompactBoard, 0, len(boardIDs))
	for _, id := range boardIDs {
		board := boardStates[id]
		cboard := &models.CompactBoard{
			ID:        board.ID,
			Title:     board.Title,
			Desc:      board.Desc,
			TicketIDs: make([]string, 0),
		}

		for _, col := range board.Columns {
			ccol := &models.CompactColumn{
				ID:      col.ID,
				Title:   col.Title,
				Tickets: make([]*models.CompactTicket, 0), // Initialize as empty array, not null
			}

			for _, ticket := range col.Tickets {
				if !ticket.Archived {
					ccol.Tickets = append(ccol.Tickets, ticket.ToCompact(truncateDesc))
					cboard.TicketIDs = append(cboard.TicketIDs, ticket.ID)
				}
			}

			cboard.Columns = append(cboard.Columns, ccol)
		}

		result = append(result, cboard)
	}

	return c.JSON(result)
}

func handleGetBoard(c *fiber.Ctx) error {
	boardID := c.Params("boardID")

	mu.RLock()
	defer mu.RUnlock()

	board, exists := boardStates[boardID]
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Board not found"})
	}

	compact, truncateDesc := models.GetCompactLevel(c)

	if compact {
		cboard := &models.CompactBoard{
			ID:        board.ID,
			Title:     board.Title,
			Desc:      board.Desc,
			TicketIDs: make([]string, 0),
		}

		for _, col := range board.Columns {
			ccol := &models.CompactColumn{
				ID:      col.ID,
				Title:   col.Title,
				Tickets: make([]*models.CompactTicket, 0), // Initialize as empty array, not null
			}

			for _, ticket := range col.Tickets {
				if !ticket.Archived {
					ccol.Tickets = append(ccol.Tickets, ticket.ToCompact(truncateDesc))
					cboard.TicketIDs = append(cboard.TicketIDs, ticket.ID)
				}
			}

			cboard.Columns = append(cboard.Columns, ccol)
		}

		return c.JSON(cboard)
	}

	return c.JSON(board)
}

// RegisterBoardRoutes registers all board-related endpoints.
func RegisterBoardRoutes(app *fiber.App) {
	// Board read endpoints are public — no auth required.
	app.Get("/api/boards", handleListBoards)
	app.Get("/api/boards/:boardID", handleGetBoard)
}
