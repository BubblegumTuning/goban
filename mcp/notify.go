package mcp

import (
	"github.com/gofiber/fiber/v2"
	"goban/config"
	"goban/models"
	"goban/sse"
)

// afterTicketWrite is an optional hook (HTTP server wires it to the
// in-memory board cache). SSE is emitted regardless of whether a hook is set.
var afterTicketWrite func(*models.Ticket)

// SetAfterTicketWrite registers a callback invoked after a successful MCP
// mutation that changed a ticket. Pass nil to clear.
func SetAfterTicketWrite(fn func(*models.Ticket)) {
	afterTicketWrite = fn
}

var allowedBoards []config.Board

// SetAllowedBoards restricts MCP create_ticket to these board IDs.
// Empty/nil means no restriction (used by unit tests that omit config).
func SetAllowedBoards(boards []config.Board) {
	allowedBoards = boards
}

func boardAllowed(id string) bool {
	if len(allowedBoards) == 0 {
		return true
	}
	for _, b := range allowedBoards {
		if b.ID == id {
			return true
		}
	}
	return false
}

func announce(eventType string, t *models.Ticket, payload fiber.Map) {
	if t == nil {
		return
	}
	if payload == nil {
		payload = fiber.Map{}
	}
	sse.Emit(eventType, t.ID, t.BoardID, payload)
	if afterTicketWrite != nil {
		afterTicketWrite(t)
	}
}
