// Package validation provides input validation helpers for handler boundaries.
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxTitleLen          = 256
	MaxDescriptionLen    = 4096
	MaxUsernameLen       = 32
	MinUsernameLen       = 1
	MaxAssigneeLen       = 32
	MaxLabelLen          = 64
	MaxLabelsCount       = 10
	MaxCommentBodyLen    = 4096
	MaxSubtaskTitleLen   = 256
	MaxRunSummaryLen     = 512
	MaxRunMetadataLen    = 1024
)

// ValidBoardSizes is the set of acceptable Go board sizes.
var ValidBoardSizes = map[int]bool{9: true, 13: true, 19: true}

// ValidPlayerColors is the set of acceptable player color values.
var ValidPlayerColors = map[string]bool{"black": true, "white": true}

// ValidRunOutcomes is the set of acceptable run outcome values.
var ValidRunOutcomes = map[string]bool{
	"active":    true,
	"completed": true,
	"failed":    true,
	"cancelled": true,
}

// ValidPriorities is the set of acceptable priority values.
var ValidPriorities = map[string]bool{
	"low": true, "medium": true, "high": true, "critical": true,
}

// usernameRegex allows alphanumeric characters, hyphens, and underscores.
var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateAgentName checks that a username/agent name is well-formed.
func ValidateAgentName(name string) error {
	if len(name) < MinUsernameLen || len(name) > MaxUsernameLen {
		return fmt.Errorf("agent_name must be between %d and %d characters", MinUsernameLen, MaxUsernameLen)
	}
	if !usernameRegex.MatchString(name) {
		return fmt.Errorf("agent_name may only contain letters, digits, hyphens, and underscores")
	}
	return nil
}

// ValidateTitle checks that a ticket title is present and within length limits.
func ValidateTitle(title string) error {
	if len(title) == 0 {
		return fmt.Errorf("title is required")
	}
	if len(title) > MaxTitleLen {
		return fmt.Errorf("title must be at most %d characters", MaxTitleLen)
	}
	return nil
}

// ValidateDescription checks that a description (if provided) is within length limits.
func ValidateDescription(desc string) error {
	if len(desc) > MaxDescriptionLen {
		return fmt.Errorf("description must be at most %d characters", MaxDescriptionLen)
	}
	return nil
}

// NormalizePriority converts a priority string to its canonical lowercase form.
// Returns ("", false) if the input is not a valid priority.
func NormalizePriority(priority string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(priority))
	if lower == "" {
		return "", true // Empty means unspecified — acceptable
	}
	if ValidPriorities[lower] {
		return lower, true
	}
	return "", false
}

// ValidatePriority checks that a priority value is one of the accepted options.
func ValidatePriority(priority string) error {
	if _, ok := NormalizePriority(priority); !ok {
		return fmt.Errorf("priority must be one of: low, medium, high, critical")
	}
	return nil
}

// ValidateAssignee checks that an assignee name is well-formed (or empty for unassigned).
func ValidateAssignee(assignee string) error {
	if assignee == "" {
		return nil // Empty means unassigned — acceptable
	}
	if len(assignee) > MaxAssigneeLen {
		return fmt.Errorf("assignee must be at most %d characters", MaxAssigneeLen)
	}
	if !usernameRegex.MatchString(assignee) {
		return fmt.Errorf("assignee may only contain letters, digits, hyphens, and underscores")
	}
	return nil
}

// ValidateLabels checks that labels are within count and length limits.
func ValidateLabels(labels []string) error {
	if len(labels) > MaxLabelsCount {
		return fmt.Errorf("at most %d labels allowed", MaxLabelsCount)
	}
	for _, label := range labels {
		if len(label) > MaxLabelLen {
			return fmt.Errorf("each label must be at most %d characters", MaxLabelLen)
		}
	}
	return nil
}

// ValidateComment checks that a comment body (if provided) is within length limits.
func ValidateComment(text string) error {
	if len(text) == 0 {
		return fmt.Errorf("comment body is required")
	}
	if len(text) > MaxCommentBodyLen {
		return fmt.Errorf("comment body must be at most %d characters", MaxCommentBodyLen)
	}
	return nil
}

// ValidateSubtaskTitle checks that a subtask title is present and within length limits.
func ValidateSubtaskTitle(title string) error {
	if len(strings.TrimSpace(title)) == 0 {
		return fmt.Errorf("subtask title is required")
	}
	if len(title) > MaxSubtaskTitleLen {
		return fmt.Errorf("subtask title must be at most %d characters", MaxSubtaskTitleLen)
	}
	return nil
}

// ValidateBoardSize checks that a Go board size is one of the accepted values.
func ValidateBoardSize(size int) error {
	if !ValidBoardSizes[size] {
		return fmt.Errorf("board_size must be one of: 9, 13, or 19")
	}
	return nil
}

// ValidatePlayerColor checks that a player color is "black" or "white".
func ValidatePlayerColor(player string) error {
	if !ValidPlayerColors[player] {
		return fmt.Errorf("player must be one of: black, white")
	}
	return nil
}

// ValidateRunOutcome checks that a run outcome value is accepted.
func ValidateRunOutcome(outcome string) error {
	if !ValidRunOutcomes[outcome] {
		return fmt.Errorf("outcome must be one of: active, completed, failed, cancelled")
	}
	return nil
}

// ValidateRunSummary checks that a run summary (if provided) is within length limits.
func ValidateRunSummary(summary string) error {
	if len(summary) > MaxRunSummaryLen {
		return fmt.Errorf("summary must be at most %d characters", MaxRunSummaryLen)
	}
	return nil
}

// ValidateRunMetadata checks that run metadata (if provided) is within length limits.
func ValidateRunMetadata(metadata string) error {
	if len(metadata) > MaxRunMetadataLen {
		return fmt.Errorf("metadata must be at most %d characters", MaxRunMetadataLen)
	}
	return nil
}

// ValidateTicketID checks that a ticket ID matches the expected format (ticket-[hex]).
func ValidateTicketID(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("ticket id is required")
	}
	if !strings.HasPrefix(id, "ticket-") || len(id) < 10 {
		return fmt.Errorf("invalid ticket id format: must start with 'ticket-' and contain a hex suffix")
	}
	return nil
}

// ValidateGameID checks that a game ID is non-empty.
func ValidateGameID(id string) error {
	if len(strings.TrimSpace(id)) == 0 {
		return fmt.Errorf("game id is required")
	}
	return nil
}
