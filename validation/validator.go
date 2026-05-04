// Package validation provides input validation helpers for handler boundaries.
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxTitleLen        = 256
	MaxDescriptionLen  = 4096
	MaxUsernameLen     = 32
	MinUsernameLen     = 1
	MaxAssigneeLen     = 32
	MaxLabelLen        = 64
	MaxLabelsCount     = 10
	MaxCommentBodyLen  = 4096
	MaxSubtaskTitleLen = 256
)

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
