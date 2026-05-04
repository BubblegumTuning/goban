// Package services contains business logic for Goban operations.
package services

import (
	"errors"
	"fmt"
	"log"

	"goban/models"
	"goban/store"
)

var ErrUnreleased = errors.New("ticket not assigned")

// ReleaseService handles ticket releases with permission checks.
type ReleaseService struct {
	store store.TicketStore
}

// NewReleaseService creates a new ReleaseService instance.
func NewReleaseService(s store.TicketStore) *ReleaseService {
	return &ReleaseService{store: s}
}

// ReleaseResult represents the result of a release operation.
type ReleaseResult struct {
	Ticket     *models.Ticket `json:"ticket"`
	ReleasedAs string         `json:"released_as,omitempty"` // The name of the user who was released
}

// Release releases a ticket by clearing its assignee and resetting to TODO column.
//
// Permission rules:
// - HUMAN_ADMIN: Can release any ticket
// - OVERSEER_AI: Can release any ticket
// - NORMAL_AI: Can only release their own tickets (where they are the assignee)
func (s *ReleaseService) Release(ticketID string, user *models.User) (*ReleaseResult, error) {
	if user == nil {
		return nil, ErrUnauthorized
	}

	// Begin atomic transaction for release operation
	tx, err := s.store.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("[RELEASE.WARN] Transaction rollback failed: %v", rbErr)
			}
		}
	}()

	// Fetch target ticket within transaction
	ticket, err := tx.GetTicket(ticketID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticket: %w", err)
	}
	if ticket == nil {
		return nil, ErrNotFound
	}

	// Reject archived tickets — they should not re-enter active circulation
	if ticket.Archived {
		return nil, fmt.Errorf("%w: cannot release an archived ticket", ErrArchived)
	}

	// Idempotency check: If ticket is already unassigned, return early regardless of permissions
	if ticket.Assignee == "" {
		return &ReleaseResult{Ticket: ticket}, nil
	}

	// Permission check for assigned tickets using centralized User method
	if !user.CanRelease(ticket.Assignee, user.Name) {
		return nil, fmt.Errorf("%w: Only the assignee or an admin/overseer can release this ticket", ErrForbidden)
	}

	result := &ReleaseResult{}

	// Remember who was released
	result.ReleasedAs = ticket.Assignee

	// Execute release within transaction - reset to TODO column and clear assignee
	prevColumn := ticket.Column
	ticket.Column = "todo-0"
	ticket.Assignee = ""

	if err := tx.UpdateTicket(ticket); err != nil {
		return nil, fmt.Errorf("failed to release ticket: %w", err)
	}

	// Log activity for ticket release (within transaction)
	newState := ticket.Column
	if err := logActivityToTx(tx, ticketID, models.ActivityReset, user.Name, &prevColumn,
		&newState, ""); err != nil {
		return nil, fmt.Errorf("failed to log release activity: %w", err)
	}

	// Commit the transaction
	committed = true
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	result.Ticket = ticket
	return result, nil
}
