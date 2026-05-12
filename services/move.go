// Package services contains business logic for Goban operations.
package services

import (
	"errors"
	"fmt"
	"log"

	"goban/models"
	"goban/store"
)

// Terminal statuses that require force=true or privileged role to exit.
var TERMINAL_STATUSES = []string{"DONE", "CANCELLED"}

// Valid status values.
var VALID_STATUSES = []string{"BACKLOG", "TODO", "IN_PROGRESS", "REVIEW", "DONE", "CANCELLED"}

// MoveService handles ticket moves with permission checks and transition validation.
type MoveService struct {
	store store.TicketStore
}

// NewMoveService creates a new MoveService instance.
func NewMoveService(s store.TicketStore) *MoveService {
	return &MoveService{store: s}
}

var (
	ErrInvalidTransition = errors.New("invalid transition")
	ErrInvalidStatus     = errors.New("invalid status")
)

// MoveRequest represents a move request.
type MoveRequest struct {
	TargetStatus string `json:"target_status"`
	Force        bool   `json:"force,omitempty"`
}

// MoveResult represents the result of a move operation.
type MoveResult struct {
	Ticket *models.Ticket `json:"ticket"`
}

// IsValidTransition checks if a status transition is valid.
func IsValidTransition(current, target string, force bool, role string) error {
	// Allow same status (idempotent)
	if current == target {
		return nil
	}

	// Check terminal state exit
	for _, term := range TERMINAL_STATUSES {
		if current == term && !force {
			// HUMAN_ADMIN can bypass force requirement
			if role != models.RoleHumanAdmin {
				return fmt.Errorf("%w: Cannot move from terminal status '%s' without force=true", ErrInvalidTransition, current)
			}
		}
	}

	// Validate target status
	for _, s := range VALID_STATUSES {
		if target == s {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrInvalidStatus, target)
}

// StatusToColumnID maps a status string to its column ID.
func StatusToColumnID(status string) string {
	switch status {
	case "BACKLOG":
		return "backlog-0"
	case "TODO":
		return "todo-0"
	case "IN_PROGRESS":
		return "inprogress-0"
	case "REVIEW":
		return "review-0"
	case "DONE":
		return "done-0"
	case "CANCELLED":
		return "cancelled-0"
	default:
		return ""
	}
}

// Move moves a ticket to a target status with permission checks and transition validation.
//
// Permission rules:
// - HUMAN_ADMIN: Can move any ticket, bypasses terminal state restrictions
// - OVERSEER_AI: Can move any ticket, but respects terminal state rules (unless force=true)
// - NORMAL_AI: Can only move their own tickets, respects all transition rules
//
// Transition rules:
// - All transitions between non-terminal states are allowed
// - Exiting terminal states (DONE, CANCELLED) requires force=true or HUMAN_ADMIN role
func (s *MoveService) Move(ticketID string, req MoveRequest, user *models.User) (*MoveResult, error) {
	if user == nil {
		return nil, ErrUnauthorized
	}

	// Begin atomic transaction for move operation
	tx, err := s.store.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("[MOVE.WARN] Transaction rollback failed: %v", rbErr)
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

	// Reject archived tickets — they should not be moved while archived
	if ticket.Archived {
		return nil, fmt.Errorf("%w: cannot move an archived ticket", ErrArchived)
	}

	currentStatus := ColumnIDToStatus(ticket.Column)

	// Early return if target status matches current — skip transaction, activity log, and SSE.
	if currentStatus == req.TargetStatus {
		return &MoveResult{Ticket: ticket}, nil
	}

	// Permission check
	canMove := false
	if user.Role == models.RoleHumanAdmin || user.Role == models.RoleOverseerAI {
		canMove = true // Privileged roles can move any ticket
	} else if ticket.Assignee == user.Name {
		canMove = true // Assignee can move their own ticket
	}

	if !canMove {
		return nil, fmt.Errorf("%w: Only the assignee or an admin/overseer can move this ticket", ErrForbidden)
	}

	// Validate transition
	if err := IsValidTransition(currentStatus, req.TargetStatus, req.Force, user.Role); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
	}

	// Execute move within transaction
	targetColumnID := StatusToColumnID(req.TargetStatus)
	if targetColumnID == "" {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, req.TargetStatus)
	}

	ticket.Column = targetColumnID
	if err := tx.UpdateTicket(ticket); err != nil {
		return nil, fmt.Errorf("failed to update ticket: %w", err)
	}

	// Log activity for ticket move (within transaction)
	if err := logActivityToTx(tx, ticketID, models.ActivityMoved, user.Name, &currentStatus,
		&req.TargetStatus, ""); err != nil {
		return nil, fmt.Errorf("failed to log move activity: %w", err)
	}

	// Commit the transaction
	committed = true
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &MoveResult{Ticket: ticket}, nil
}

// ColumnIDToStatus maps a column ID back to its status string.
func ColumnIDToStatus(columnID string) string {
	switch columnID {
	case "backlog-0":
		return "BACKLOG"
	case "todo-0":
		return "TODO"
	case "inprogress-0":
		return "IN_PROGRESS"
	case "review-0":
		return "REVIEW"
	case "done-0":
		return "DONE"
	case "cancelled-0":
		return "CANCELLED"
	default:
		// Default to TODO for unknown columns (backward compatibility)
		return "TODO"
	}
}
