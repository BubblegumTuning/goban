package validate

import (
	"fmt"
	"strings"
)

// ValidateTicketID checks that a ticket ID is non-empty, uses valid characters,
// and meets minimum length requirements. Returns nil if valid.
func ValidateTicketID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("ticket ID cannot be empty")
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("ticket ID contains invalid character '%c'", r)
		}
	}
	if len(id) < 3 {
		return fmt.Errorf("ticket ID must be at least 3 characters long")
	}
	return nil
}

// ValidateBoardID checks that a board ID is non-empty and uses valid characters.
func ValidateBoardID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("board ID cannot be empty")
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("board ID contains invalid character '%c'", r)
		}
	}
	if len(id) < 1 {
		return fmt.Errorf("board ID must be at least 1 character long")
	}
	return nil
}

// ValidateTitle checks that a title is non-empty after trimming and within max length.
func ValidateTitle(title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if len(title) > 500 {
		return fmt.Errorf("title must be at most 500 characters (got %d)", len(title))
	}
	return nil
}

// ValidateStatus maps user-friendly status names to API status values.
// Returns the canonical API status string or an error for invalid input.
func ValidateStatus(statusStr string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(statusStr)) {
	case "backlog", "backlogs":
		return "BACKLOG", nil
	case "to do", "todo":
		return "TODO", nil
	case "in progress", "inprogress", "ip":
		return "IN_PROGRESS", nil
	case "review", "reviews":
		return "REVIEW", nil
	case "done", "doned":
		return "DONE", nil
	case "cancelled", "cancel", "canceled":
		return "CANCELLED", nil
	default:
		return "", fmt.Errorf("invalid status '%s'. Use: 'backlog', 'todo', 'inprogress', 'review', 'done', or 'cancelled'", statusStr)
	}
}

// ColumnPrefixFromAPIStatus converts an API status value to the column base name
// used by Ticket.MatchesColumn() for verification.
func ColumnPrefixFromAPIStatus(apiStatus string) string {
	switch apiStatus {
	case "BACKLOG":
		return "backlog"
	case "TODO":
		return "todo"
	case "IN_PROGRESS":
		return "inprogress"
	case "REVIEW":
		return "review"
	case "DONE":
		return "done"
	case "CANCELLED":
		return "cancelled"
	default:
		return strings.ToLower(apiStatus)
	}
}
