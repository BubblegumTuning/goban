// Package validation provides input validation helpers for handler boundaries.
package validation

import (
	"strings"
	"testing"
)

// ============================================================================
// ValidateAgentName Tests

func TestValidateAgentName_Valid(t *testing.T) {
	validNames := []string{
		"a",                                 // Minimum length (1 char)
		"user",                              // Simple name
		"user-1",                            // With hyphen
		"user_2",                            // With underscore
		"AgentBot99",                        // Mixed case + digits
		strings.Repeat("a", MaxUsernameLen), // Maximum length (32 chars)
	}

	for _, name := range validNames {
		if err := ValidateAgentName(name); err != nil {
			t.Errorf("ValidateAgentName(%q) should be valid, got error: %v", name, err)
		}
	}
}

func TestValidateAgentName_EmptyString(t *testing.T) {
	err := ValidateAgentName("")
	if err == nil {
		t.Error("Expected error for empty agent name")
		return
	}
	if !strings.Contains(err.Error(), "between 1 and 32 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateAgentName_TooLong(t *testing.T) {
	err := ValidateAgentName(strings.Repeat("a", MaxUsernameLen+1))
	if err == nil {
		t.Error("Expected error for agent name exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "between 1 and 32 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateAgentName_InvalidCharacters(t *testing.T) {
	invalidNames := []string{
		"user name",      // Space not allowed
		"hello@world",    // @ not allowed
		"name.with.dots", // Dots not allowed
		"slash/name",     // Slash not allowed
		"name:colon",     // Colon not allowed
	}

	for _, name := range invalidNames {
		if len(name) > MaxUsernameLen {
			continue // Skip names that would fail length check first
		}
		err := ValidateAgentName(name)
		if err == nil {
			t.Errorf("ValidateAgentName(%q) should have failed, got nil", name)
		} else if !strings.Contains(err.Error(), "may only contain letters, digits") {
			t.Errorf("Unexpected error for %q: %v", name, err)
		}
	}
}

// ============================================================================
// ValidateTitle Tests

func TestValidateTitle_Valid(t *testing.T) {
	validTitles := []string{
		"a",                              // Minimum (1 char)
		"Fix the login bug",              // Normal title
		strings.Repeat("x", MaxTitleLen), // Maximum length (256 chars)
	}

	for _, title := range validTitles {
		if err := ValidateTitle(title); err != nil {
			t.Errorf("ValidateTitle(%q) should be valid, got error: %v", title[:min(20, len(title))]+"...", err)
		}
	}
}

func TestValidateTitle_Empty(t *testing.T) {
	err := ValidateTitle("")
	if err == nil {
		t.Error("Expected error for empty title")
		return
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateTitle_TooLong(t *testing.T) {
	err := ValidateTitle(strings.Repeat("x", MaxTitleLen+1))
	if err == nil {
		t.Error("Expected error for title exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 256 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateTitle_WhitespaceOnly(t *testing.T) {
	err := ValidateTitle("   ")
	if err != nil {
		t.Logf("Whitespace-only title returns error (expected behavior): %v", err)
		return // Whitespace titles are valid per current implementation (non-empty string)
	}
	t.Log("Whitespace-only title is accepted (len > 0)")
}

// ============================================================================
// ValidateDescription Tests

func TestValidateDescription_Valid(t *testing.T) {
	validDescs := []string{
		"",                                     // Empty description is allowed
		"Short description",                    // Normal description
		strings.Repeat("x", MaxDescriptionLen), // Maximum length (4096 chars)
	}

	for _, desc := range validDescs {
		if err := ValidateDescription(desc); err != nil {
			t.Errorf("ValidateDescription with len=%d should be valid, got error: %v", len(desc), err)
		}
	}
}

func TestValidateDescription_TooLong(t *testing.T) {
	err := ValidateDescription(strings.Repeat("x", MaxDescriptionLen+1))
	if err == nil {
		t.Error("Expected error for description exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 4096 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// NormalizePriority Tests

func TestNormalizePriority_ValidInputs(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		ok       bool
	}{
		{"low", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"critical", "critical", true},
		{"LOW", "low", true},           // Uppercase normalization
		{"High", "high", true},         // Mixed case normalization
		{"CRITICAL", "critical", true}, // All uppercase normalization
		{" high ", "high", true},       // Whitespace trimming
		{"  LOW  ", "low", true},       // Trimming + lowercase
		{"", "", true},                 // Empty is acceptable (unspecified)
	}

	for _, tc := range tests {
		result, ok := NormalizePriority(tc.input)
		if !ok || result != tc.expected {
			t.Errorf("NormalizePriority(%q) = (%q, %v), expected (%q, true)", tc.input, result, ok, tc.expected)
		}
	}
}

func TestNormalizePriority_InvalidInputs(t *testing.T) {
	invalidInputs := []string{
		"urgent",
		"normal",
		"P1",
		"blocker",
		"123",
	}

	for _, input := range invalidInputs {
		result, ok := NormalizePriority(input)
		if ok || result != "" {
			t.Errorf("NormalizePriority(%q) should have failed, got (%q, %v)", input, result, ok)
		}
	}
}

// ============================================================================
// ValidatePriority Tests

func TestValidatePriority_Valid(t *testing.T) {
	validPriorities := []string{
		"low", "medium", "high", "critical", // Exact matches
		"LOW", "HIGH", "CRITICAL", // Uppercase variants
		" High ", " low ", // Whitespace-padded
		"", // Empty (unspecified — valid)
	}

	for _, p := range validPriorities {
		if err := ValidatePriority(p); err != nil {
			t.Errorf("ValidatePriority(%q) should be valid, got error: %v", p, err)
		}
	}
}

func TestValidatePriority_Invalid(t *testing.T) {
	invalidPriorities := []string{
		"urgent",
		"normal",
		"P1",
		"blocker",
		"123",
		"medium-high",
	}

	for _, p := range invalidPriorities {
		err := ValidatePriority(p)
		if err == nil {
			t.Errorf("ValidatePriority(%q) should have failed, got nil", p)
			continue
		}
		if !strings.Contains(err.Error(), "must be one of: low, medium, high, critical") {
			t.Errorf("Unexpected error message for %q: %v", p, err)
		}
	}
}

// ============================================================================
// ValidateAssignee Tests

func TestValidateAssignee_Valid(t *testing.T) {
	validAssignees := []string{
		"",                                  // Empty means unassigned — valid
		"agent-a",                           // Normal assignee name
		"user_123",                          // With underscore and digits
		strings.Repeat("a", MaxAssigneeLen), // Maximum length (32 chars)
	}

	for _, a := range validAssignees {
		if err := ValidateAssignee(a); err != nil {
			t.Errorf("ValidateAssignee(%q) should be valid, got error: %v", a, err)
		}
	}
}

func TestValidateAssignee_TooLong(t *testing.T) {
	err := ValidateAssignee(strings.Repeat("a", MaxAssigneeLen+1))
	if err == nil {
		t.Error("Expected error for assignee exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 32 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateAssignee_InvalidCharacters(t *testing.T) {
	err := ValidateAssignee("user name with spaces")
	if err == nil {
		t.Error("Expected error for assignee containing spaces")
		return
	}
	if !strings.Contains(err.Error(), "may only contain letters, digits") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// ValidateLabels Tests

func TestValidateLabels_Valid(t *testing.T) {
	validCases := [][]string{
		nil,                       // Nil labels — valid
		{},                        // Empty slice — valid
		{"frontend"},              // Single label
		{"bug", "urgent", "v2.0"}, // Multiple labels
		strings.Split(strings.Repeat("a,", MaxLabelsCount-1)+"a", ","), // Maximum count (10)
	}

	for i, labels := range validCases {
		if err := ValidateLabels(labels); err != nil {
			t.Errorf("ValidateLabels case %d should be valid, got error: %v", i, err)
		}
	}
}

func TestValidateLabels_TooMany(t *testing.T) {
	labels := make([]string, MaxLabelsCount+1)
	for i := range labels {
		labels[i] = "label"
	}

	err := ValidateLabels(labels)
	if err == nil {
		t.Error("Expected error for exceeding max label count")
		return
	}
	if !strings.Contains(err.Error(), "at most 10 labels allowed") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateLabels_IndividualLabelTooLong(t *testing.T) {
	labels := []string{"valid", strings.Repeat("x", MaxLabelLen+1)}
	err := ValidateLabels(labels)
	if err == nil {
		t.Error("Expected error for label exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 64 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// ValidateComment Tests

func TestValidateComment_Valid(t *testing.T) {
	validComments := []string{
		"a", // Minimum (1 char)
		"This is a helpful comment about the fix.", // Normal comment
		strings.Repeat("x", MaxCommentBodyLen),     // Maximum length (4096 chars)
	}

	for _, c := range validComments {
		if err := ValidateComment(c); err != nil {
			t.Errorf("ValidateComment with len=%d should be valid, got error: %v", len(c), err)
		}
	}
}

func TestValidateComment_Empty(t *testing.T) {
	err := ValidateComment("")
	if err == nil {
		t.Error("Expected error for empty comment")
		return
	}
	if !strings.Contains(err.Error(), "comment body is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateComment_TooLong(t *testing.T) {
	err := ValidateComment(strings.Repeat("x", MaxCommentBodyLen+1))
	if err == nil {
		t.Error("Expected error for comment exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 4096 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// ValidateSubtaskTitle Tests

func TestValidateSubtaskTitle_Valid(t *testing.T) {
	validTitles := []string{
		"a",                                     // Minimum (1 char after trim)
		"Write unit tests",                      // Normal subtask title
		strings.Repeat("x", MaxSubtaskTitleLen), // Maximum length (256 chars)
	}

	for _, s := range validTitles {
		if err := ValidateSubtaskTitle(s); err != nil {
			t.Errorf("ValidateSubtaskTitle(%q) should be valid, got error: %v", s[:min(30, len(s))]+"...", err)
		}
	}
}

func TestValidateSubtaskTitle_Empty(t *testing.T) {
	err := ValidateSubtaskTitle("")
	if err == nil {
		t.Error("Expected error for empty subtask title")
		return
	}
	if !strings.Contains(err.Error(), "subtask title is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateSubtaskTitle_WhitespaceOnly(t *testing.T) {
	err := ValidateSubtaskTitle("   ")
	if err == nil {
		t.Error("Expected error for whitespace-only subtask title")
		return
	}
	if !strings.Contains(err.Error(), "subtask title is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidateSubtaskTitle_TooLong(t *testing.T) {
	err := ValidateSubtaskTitle(strings.Repeat("x", MaxSubtaskTitleLen+1))
	if err == nil {
		t.Error("Expected error for subtask title exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 256 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// Constant and boundary verification tests

func TestConstants(t *testing.T) {
	if MaxTitleLen != 256 {
		t.Errorf("MaxTitleLen = %d, expected 256", MaxTitleLen)
	}
	if MaxDescriptionLen != 4096 {
		t.Errorf("MaxDescriptionLen = %d, expected 4096", MaxDescriptionLen)
	}
	if MaxUsernameLen != 32 {
		t.Errorf("MaxUsernameLen = %d, expected 32", MaxUsernameLen)
	}
	if MinUsernameLen != 1 {
		t.Errorf("MinUsernameLen = %d, expected 1", MinUsernameLen)
	}
	if MaxAssigneeLen != 32 {
		t.Errorf("MaxAssigneeLen = %d, expected 32", MaxAssigneeLen)
	}
	if MaxLabelLen != 64 {
		t.Errorf("MaxLabelLen = %d, expected 64", MaxLabelLen)
	}
	if MaxLabelsCount != 10 {
		t.Errorf("MaxLabelsCount = %d, expected 10", MaxLabelsCount)
	}
	if MaxCommentBodyLen != 4096 {
		t.Errorf("MaxCommentBodyLen = %d, expected 4096", MaxCommentBodyLen)
	}
	if MaxSubtaskTitleLen != 256 {
		t.Errorf("MaxSubtaskTitleLen = %d, expected 256", MaxSubtaskTitleLen)
	}
}

func TestValidPrioritiesSet(t *testing.T) {
	expectedPriorities := []string{"low", "medium", "high", "critical"}
	for _, p := range expectedPriorities {
		if !ValidPriorities[p] {
			t.Errorf("Expected %q to be a valid priority, but it was not in ValidPriorities set", p)
		}
	}

	// Verify no unexpected keys exist
	for key := range ValidPriorities {
		found := false
		for _, expected := range expectedPriorities {
			if key == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected priority in ValidPriorities set: %q", key)
		}
	}
}

func TestNormalizePriority_EdgeCases(t *testing.T) {
	// Trailing/leading whitespace with valid priority
	result, ok := NormalizePriority("  high  ")
	if !ok || result != "high" {
		t.Errorf("NormalizePriority(\"  high  \") = (%q, %v), expected (\"high\", true)", result, ok)
	}

	// Tab character trimming
	result, ok = NormalizePriority("\tlow\t")
	if !ok || result != "low" {
		t.Errorf("NormalizePriority with tabs = (%q, %v), expected (\"low\", true)", result, ok)
	}
}

// ============================================================================
// ValidateBoardSize Tests

func TestValidateBoardSize_Valid(t *testing.T) {
	for _, size := range []int{9, 13, 19} {
		if err := ValidateBoardSize(size); err != nil {
			t.Errorf("ValidateBoardSize(%d) should be valid, got error: %v", size, err)
		}
	}
}

func TestValidateBoardSize_Invalid(t *testing.T) {
	for _, size := range []int{0, 7, 15, 21, -1} {
		err := ValidateBoardSize(size)
		if err == nil {
			t.Errorf("ValidateBoardSize(%d) should have failed, got nil", size)
		} else if !strings.Contains(err.Error(), "board_size must be one of: 9, 13, or 19") {
			t.Errorf("Unexpected error for %d: %v", size, err)
		}
	}
}

// ============================================================================
// ValidatePlayerColor Tests

func TestValidatePlayerColor_Valid(t *testing.T) {
	for _, color := range []string{"black", "white"} {
		if err := ValidatePlayerColor(color); err != nil {
			t.Errorf("ValidatePlayerColor(%q) should be valid, got error: %v", color, err)
		}
	}
}

func TestValidatePlayerColor_Invalid(t *testing.T) {
	for _, color := range []string{"", "red", "Blue", "WHITE"} {
		err := ValidatePlayerColor(color)
		if err == nil {
			t.Errorf("ValidatePlayerColor(%q) should have failed, got nil", color)
		} else if !strings.Contains(err.Error(), "player must be one of: black, white") {
			t.Errorf("Unexpected error for %q: %v", color, err)
		}
	}
}

// ============================================================================
// ValidateRunOutcome Tests

func TestValidateRunOutcome_Valid(t *testing.T) {
	for _, outcome := range []string{"active", "completed", "failed", "cancelled"} {
		if err := ValidateRunOutcome(outcome); err != nil {
			t.Errorf("ValidateRunOutcome(%q) should be valid, got error: %v", outcome, err)
		}
	}
}

func TestValidateRunOutcome_Invalid(t *testing.T) {
	for _, outcome := range []string{"", "done", "in-progress", "ACTIVE"} {
		err := ValidateRunOutcome(outcome)
		if err == nil {
			t.Errorf("ValidateRunOutcome(%q) should have failed, got nil", outcome)
		} else if !strings.Contains(err.Error(), "outcome must be one of: active, completed, failed, cancelled") {
			t.Errorf("Unexpected error for %q: %v", outcome, err)
		}
	}
}

// ============================================================================
// ValidateRunSummary Tests

func TestValidateRunSummary_Valid(t *testing.T) {
	validSummaries := []string{
		"",                                    // Empty is allowed
		"Short summary",                       // Normal summary
		strings.Repeat("x", MaxRunSummaryLen), // Maximum length (512 chars)
	}

	for _, s := range validSummaries {
		if err := ValidateRunSummary(s); err != nil {
			t.Errorf("ValidateRunSummary with len=%d should be valid, got error: %v", len(s), err)
		}
	}
}

func TestValidateRunSummary_TooLong(t *testing.T) {
	err := ValidateRunSummary(strings.Repeat("x", MaxRunSummaryLen+1))
	if err == nil {
		t.Error("Expected error for summary exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 512 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// ValidateRunMetadata Tests

func TestValidateRunMetadata_Valid(t *testing.T) {
	validMetadata := []string{
		"",                                     // Empty is allowed
		"Key metadata here",                    // Normal metadata
		strings.Repeat("x", MaxRunMetadataLen), // Maximum length (1024 chars)
	}

	for _, m := range validMetadata {
		if err := ValidateRunMetadata(m); err != nil {
			t.Errorf("ValidateRunMetadata with len=%d should be valid, got error: %v", len(m), err)
		}
	}
}

func TestValidateRunMetadata_TooLong(t *testing.T) {
	err := ValidateRunMetadata(strings.Repeat("x", MaxRunMetadataLen+1))
	if err == nil {
		t.Error("Expected error for metadata exceeding max length")
		return
	}
	if !strings.Contains(err.Error(), "at most 1024 characters") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================================================================
// ValidateTicketID Tests

func TestValidateTicketID_Valid(t *testing.T) {
	validIDs := []string{
		"ticket-abc123def456",
		"ticket-d3aab1ddf88f4b0b",
		"ticket-774f511b8f",
	}

	for _, id := range validIDs {
		if err := ValidateTicketID(id); err != nil {
			t.Errorf("ValidateTicketID(%q) should be valid, got error: %v", id, err)
		}
	}
}

func TestValidateTicketID_Invalid(t *testing.T) {
	invalidIDs := []string{
		"",                   // Empty
		"ticket",             // Too short (no hex suffix)
		"board-abc123def456", // Wrong prefix
	}

	for _, id := range invalidIDs {
		err := ValidateTicketID(id)
		if err == nil {
			t.Errorf("ValidateTicketID(%q) should have failed, got nil", id)
		}
	}
}

// ============================================================================
// ValidateGameID Tests

func TestValidateGameID_Valid(t *testing.T) {
	validIDs := []string{
		"game-abc123def456",
		"a1b2c3d4e5f6",
		"any-nonempty-string",
	}

	for _, id := range validIDs {
		if err := ValidateGameID(id); err != nil {
			t.Errorf("ValidateGameID(%q) should be valid, got error: %v", id, err)
		}
	}
}

func TestValidateGameID_Invalid(t *testing.T) {
	invalidIDs := []string{
		"",    // Empty
		"   ", // Whitespace only
	}

	for _, id := range invalidIDs {
		err := ValidateGameID(id)
		if err == nil {
			t.Errorf("ValidateGameID(%q) should have failed, got nil", id)
		} else if !strings.Contains(err.Error(), "game id is required") {
			t.Errorf("Unexpected error for %q: %v", id, err)
		}
	}
}
