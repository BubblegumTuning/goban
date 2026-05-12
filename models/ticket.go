// Package models provides ticket-related utilities and index management.
package models

// NOTE: Duplicate GetColumnID/GenerateTicketID
// functions previously existed in a dead utils.go (package main, with
// 'Don e' typo). utils.go has been removed entirely. All logic now lives
// here in models/ only. This resolves the DRY violation. See goban_code_audit.md.
//
// This file serves as the single source of truth per project conventions.

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// Column ID mapping - single source of truth. Only backlog/todo/inprogress/review/done/cancelled permitted (per note).
// Maps various column name formats to their canonical IDs with suffix "-0" for board 0.
var columnTitleToID = map[string]string{
	// LEGACY FORMATS (lowercase, no suffix) - maintained for backward compatibility with existing tickets in database
	// These were normalized via migration normalize_column_values.sql
	"todo":       "todo-0",
	"inprogress": "inprogress-0",
	"done":       "done-0",

	// PROPER CASE TITLES (display names) → lowercase canonical IDs with -0 suffix
	// All column IDs are now consistently lowercase for API compatibility
	"To Do":       "todo-0",
	"Todo":        "todo-0", // Infrastructure board variant
	"In Progress": "inprogress-0",
	"Done":        "done-0",
	"Review":      "review-0",
	"Backlog":     "backlog-0",
	"Cancelled":   "cancelled-0",

	// OTHER STANDARD COLUMNS (lowercase variants) - fallback mappings for edge cases
	"review":   "review-0",
	"blocked":  "blocked-0",
	"archived": "archived-0",
}

// GetColumnID returns the canonical column ID for a given title.
// Handles both column titles ("Review") and full IDs ("Review-0").
// If the input is not found in columnTitleToID, falls back to appending "-0" suffix.
func GetColumnID(title string) string {
	// Optimization: if input already looks like a full column ID (ends with -0), return as-is
	if len(title) > 2 && title[len(title)-2:] == "-0" {
		return title
	}
	if id, exists := columnTitleToID[title]; exists {
		return id
	}
	// Fallback: append "-0" to any unknown column title (graceful handling of new columns)
	return fmt.Sprintf("%s-0", title)
}

// TicketIndex is a thread-safe O(1) lookup index for tickets.
type TicketIndex struct {
	mu    sync.RWMutex
	index map[string]*TicketLocation // ticketID -> location
}

// NewTicketIndex creates and initializes a new ticket index.
func NewTicketIndex() *TicketIndex {
	return &TicketIndex{
		index: make(map[string]*TicketLocation),
	}
}

// Rebuild rebuilds the entire index from board states.
func (ti *TicketIndex) Rebuild(boardStates map[string]*BoardState) {
	ti.mu.Lock()
	defer ti.mu.Unlock()

	ti.index = make(map[string]*TicketLocation)

	for boardID, board := range boardStates {
		for _, col := range board.Columns {
			for _, ticket := range col.Tickets {
				ti.index[ticket.ID] = &TicketLocation{
					Ticket:   ticket,
					BoardID:  boardID,
					ColumnID: col.ID,
				}
			}
		}
	}
}

// Find looks up a ticket by ID and returns its location.
func (ti *TicketIndex) Find(ticketID string) (*TicketLocation, bool) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	loc, exists := ti.index[ticketID]
	return loc, exists
}

// Update updates or inserts a ticket in the index.
func (ti *TicketIndex) Update(ticketID, boardID, columnID string, ticket *Ticket) {
	ti.mu.Lock()
	defer ti.mu.Unlock()

	ti.index[ticketID] = &TicketLocation{
		Ticket:   ticket,
		BoardID:  boardID,
		ColumnID: columnID,
	}
}

// Remove removes a ticket from the index.
func (ti *TicketIndex) Remove(ticketID string) {
	ti.mu.Lock()
	defer ti.mu.Unlock()

	delete(ti.index, ticketID)
}

// GenerateTicketID creates a collision-free ticket ID using crypto/rand.
// Format: ticket-YYYYMMDD-<80-bit hex hash>.
// Entropy: 5 bytes (80 bits) → ~3 billion tickets before birthday paradox concerns.
// Acceptable for current scale; increase to 8 bytes if system grows significantly.
func GenerateTicketID() string {
	// Use 5 random bytes for ~80 bits of entropy
	bytes := make([]byte, 5)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based if crypto fails
		return fmt.Sprintf("ticket-%s-0", time.Now().Format("20060102"))
	}

	hash := fmt.Sprintf("%02x%02x%02x%02x%02x", bytes[0], bytes[1], bytes[2], bytes[3], bytes[4])
	return fmt.Sprintf("ticket-%s-%s", time.Now().Format("20060102"), hash)
}

// ValidateNoDuplicateTickets checks for duplicates across all boards.
func ValidateNoDuplicateTickets(boardStates map[string]*BoardState) bool {
	seen := make(map[string]bool)

	for _, board := range boardStates {
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if seen[t.ID] {
					return false // Duplicate found
				}
				seen[t.ID] = true
			}
		}
	}

	return true
}
