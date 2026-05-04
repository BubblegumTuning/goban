// Package services contains business logic for Goban operations.
package services

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"goban/models"
	"goban/store"
)

// Tx represents an active database transaction with scoped operations.
// Aliases store.Tx for backward compatibility and clarity.
type Tx = store.Tx

// ClaimService handles ticket claiming with permission checks and auto-release.
type ClaimService struct {
	store store.TicketStore
}

// NewClaimService creates a new ClaimService instance.
func NewClaimService(s store.TicketStore) *ClaimService {
	return &ClaimService{store: s}
}

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("ticket not found")
	ErrForbidden    = errors.New("forbidden")
)

// ClaimResult represents the result of a claim operation.
type ClaimResult struct {
	Ticket       *models.Ticket `json:"ticket"`
	AutoReleased []string       `json:"auto_released,omitempty"` // IDs of auto-released tickets
}

// Claim attempts to claim a ticket for a user with permission checks and auto-release.
//
// Permission rules:
// - HUMAN_ADMIN: Can claim any ticket (force)
// - OVERSEER_AI: Can claim any ticket
// - NORMAL_AI: Can only claim unassigned tickets or their own; cannot steal from other AIs
//
// Auto-release: If the requester already has an IN_PROGRESS ticket, it is released first.
// All operations are wrapped in an atomic transaction to prevent partial updates.
func (s *ClaimService) Claim(ticketID string, user *models.User) (*ClaimResult, error) {
	if user == nil || user.Name == "" {
		return nil, fmt.Errorf("%w: authenticated identity has no username", ErrUnauthorized)
	}

	result := &ClaimResult{}

	// Begin atomic transaction for auto-release + claim
	tx, err := s.store.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("[CLAIM.WARN] Transaction rollback failed: %v", rbErr)
			}
		}
	}()

	// Fetch target ticket within transaction
	targetTicket, err := tx.GetTicket(ticketID)
	if err != nil {
		if errors.Is(err, driver.ErrBadConn) || strings.Contains(err.Error(), "no such table") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to fetch ticket: %w", err)
	}
	if targetTicket == nil {
		return nil, ErrNotFound
	}

	// Check if already claimed by this agent (idempotent)
	if targetTicket.Assignee == user.Name {
		result.Ticket = targetTicket
		return result, nil
	}

	// Permission check using centralized User method
	if !user.CanClaim(targetTicket.Assignee, user.Name) {
		return nil, fmt.Errorf("%w: ticket owned by another AI; request human assistance", ErrForbidden)
	}

	// Auto-release: Find existing tickets in IN_PROGRESS columns assigned to this user
	existingTickets, err := tx.GetTicketsByColumnAndAssignee("inprogress", user.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing tickets: %w", err)
	}

	for _, existingTicket := range existingTickets {
		if existingTicket.ID == ticketID {
			// Reclaiming own ticket - idempotent, nothing to do except update column if needed
			continue
		}

		// Release the existing ticket (set back to TODO, clear assignee)
		prevColumn := existingTicket.Column
		existingTicket.Column = "todo-0" // Reset to TODO column
		existingTicket.Assignee = ""
		existingTicket.UpdatedAt = time.Now().Format(time.RFC3339)

		if err := tx.UpdateTicket(existingTicket); err != nil {
			return nil, fmt.Errorf("failed to auto-release ticket %s: %w", existingTicket.ID, err)
		}

		// Log activity for auto-release (reset event)
		newState := existingTicket.Column
		if err := logActivityToTx(tx, existingTicket.ID, models.ActivityReset, user.Name, &prevColumn,
			&newState, fmt.Sprintf(`{"reason":"auto_released_on_new_claim","new_ticket":"%s"}`, ticketID)); err != nil {
			return nil, fmt.Errorf("failed to log auto-release activity: %w", err)
		}

		result.AutoReleased = append(result.AutoReleased, existingTicket.ID)
		break // Only release one ticket per claim (most recent by iteration order)
	}

	// Claim the target ticket - move to IN_PROGRESS column and assign to user
	prevTargetColumn := targetTicket.Column
	targetTicket.Column = "inprogress-0"
	targetTicket.Assignee = user.Name
	targetTicket.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := tx.UpdateTicket(targetTicket); err != nil {
		return nil, fmt.Errorf("failed to claim ticket: %w", err)
	}

	// Log activity for ticket claim
	newColumn := "inprogress-0"
	if err := logActivityToTx(tx, ticketID, models.ActivityClaimed, user.Name, &prevTargetColumn,
		&newColumn, ""); err != nil {
		return nil, fmt.Errorf("failed to log claim activity: %w", err)
	}

	// Commit the transaction - all or nothing
	committed = true
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	result.Ticket = targetTicket
	return result, nil
}

// logActivityToTx logs an activity entry within a transaction.
func logActivityToTx(tx store.Tx, ticketID, eventType, actor string, prevState, newState *string, metadata string) error {
	logEntry := &models.ActivityLog{
		TicketID:  ticketID,
		EventType: eventType,
		Actor:     actor,
		PrevState: prevState,
		NewState:  newState,
		Metadata:  metadata,
	}

	id, err := tx.CreateActivityLog(logEntry)
	if err != nil {
		return fmt.Errorf("failed to log activity in transaction: %w", err)
	}

	log.Printf("[ACTIVITY.TX] Logged event '%s' for ticket %s by %s (id=%d)", eventType, ticketID, actor, id)
	return nil
}
