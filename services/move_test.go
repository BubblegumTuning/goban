// Package services contains business logic for Goban operations.
package services

import (
	"testing"

	"goban/models"
	"goban/testutil"
)

// setupMoveTest creates a mock store with test data for move tests.
func setupMoveTest(t *testing.T) *testutil.MockStore {
	store := testutil.NewMockStore()
	if err := store.Init(); err != nil {
		t.Fatalf("Failed to initialize mock store: %v", err)
	}
	return store
}

// TestIsValidTransition verifies all valid and invalid status transitions.
func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		target    string
		force     bool
		role      string
		wantErr   bool
		isInvalid bool // Specifically ErrInvalidTransition vs other errors
	}{
		// Valid transitions (non-terminal)
		{"TODO to IN_PROGRESS", "TODO", "IN_PROGRESS", false, models.RoleNormalAI, false, false},
		{"IN_PROGRESS to REVIEW", "IN_PROGRESS", "REVIEW", false, models.RoleNormalAI, false, false},
		{"REVIEW to DONE", "REVIEW", "DONE", false, models.RoleNormalAI, false, false},
		{"BACKLOG to TODO", "BACKLOG", "TODO", false, models.RoleNormalAI, false, false},
		{"Same status (idempotent)", "TODO", "TODO", false, models.RoleNormalAI, false, false},

		// Invalid: Terminal state exit without force
		{"DONE to TODO without force", "DONE", "TODO", false, models.RoleNormalAI, true, true},
		{"CANCELLED to TODO without force", "CANCELLED", "TODO", false, models.RoleNormalAI, true, true},

		// Valid: Terminal state exit with force=true
		{"DONE to TODO with force", "DONE", "TODO", true, models.RoleNormalAI, false, false},
		{"CANCELLED to BACKLOG with force", "CANCELLED", "BACKLOG", true, models.RoleNormalAI, false, false},

		// Valid: HUMAN_ADMIN can bypass terminal restrictions
		{"HUMAN_ADMIN DONE to TODO without force", "DONE", "TODO", false, models.RoleHumanAdmin, false, false},
		{"OVERSEER_AI cannot bypass terminal", "DONE", "TODO", false, models.RoleOverseerAI, true, true},

		// Invalid: Bad target status
		{"Invalid target status", "TODO", "INVALID_STATUS", false, models.RoleNormalAI, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidTransition(tt.current, tt.target, tt.force, tt.role)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.isInvalid && err != nil && !contains(err.Error(), ErrInvalidTransition.Error()) {
				t.Errorf("Expected invalid transition error, got: %v", err)
			}
		})
	}
}

// TestMoveTicket_PermissionChecks verifies role-based permission enforcement for moves.
func TestMoveTicket_PermissionChecks(t *testing.T) {
	tests := []struct {
		name        string
		userRole    string
		ticketOwner string // Empty = unassigned
		wantErr     bool
	}{
		{"HUMAN_ADMIN can move any ticket", models.RoleHumanAdmin, "other-agent", false},
		{"OVERSEER_AI can move any ticket", models.RoleOverseerAI, "other-agent", false},
		{"NORMAL_AI cannot move other AI's ticket", models.RoleNormalAI, "other-agent", true},
		{"NORMAL_AI can move own ticket", models.RoleNormalAI, "normal-agent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupMoveTest(t)
			service := NewMoveService(store)

			userName := "normal-agent"
			if tt.userRole == models.RoleHumanAdmin {
				userName = "admin-user"
			} else if tt.userRole == models.RoleOverseerAI {
				userName = "overseer-user"
			}

			if _, err := store.CreateUser(userName, tt.userRole); err != nil {
				t.Fatalf("Failed to create user: %v", err)
			}
			user, _ := store.GetUserByName(userName)

			ticket := &models.Ticket{
				ID:       "ticket-move-perm",
				Title:    "Move Permission Test",
				Column:   "todo-0", // TODO status
				BoardID:  "test-board",
				Assignee: tt.ticketOwner,
			}
			if err := store.CreateTicket(ticket); err != nil {
				t.Fatalf("Failed to create ticket: %v", err)
			}

			req := MoveRequest{TargetStatus: "IN_PROGRESS"}
			result, err := service.Move("ticket-move-perm", req, user)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if result.Ticket.Column != "inprogress-0" {
					t.Errorf("Expected column 'inprogress-0', got '%s'", result.Ticket.Column)
				}
			}
		})
	}
}

// TestMoveTicket_TransitionValidation verifies transition flow validation.
func TestMoveTicket_TransitionValidation(t *testing.T) {
	store := setupMoveTest(t)
	service := NewMoveService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	// Create ticket in TODO
	ticket := &models.Ticket{
		ID:       "ticket-transition",
		Title:    "Transition Test",
		Column:   "todo-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Valid flow: TODO -> IN_PROGRESS -> REVIEW -> DONE
	req1 := MoveRequest{TargetStatus: "IN_PROGRESS"}
	result1, err := service.Move("ticket-transition", req1, user)
	if err != nil {
		t.Fatalf("TODO->IN_PROGRESS failed: %v", err)
	}
	if result1.Ticket.Column != "inprogress-0" {
		t.Errorf("Expected 'inprogress-0', got '%s'", result1.Ticket.Column)
	}

	req2 := MoveRequest{TargetStatus: "REVIEW"}
	result2, err := service.Move("ticket-transition", req2, user)
	if err != nil {
		t.Fatalf("IN_PROGRESS->REVIEW failed: %v", err)
	}
	if result2.Ticket.Column != "review-0" {
		t.Errorf("Expected 'review-0', got '%s'", result2.Ticket.Column)
	}

	req3 := MoveRequest{TargetStatus: "DONE"}
	result3, err := service.Move("ticket-transition", req3, user)
	if err != nil {
		t.Fatalf("REVIEW->DONE failed: %v", err)
	}
	if result3.Ticket.Column != "done-0" {
		t.Errorf("Expected 'done-0', got '%s'", result3.Ticket.Column)
	}

	// Now in DONE (terminal) - should fail without force
	req4 := MoveRequest{TargetStatus: "TODO"}
	_, err = service.Move("ticket-transition", req4, user)
	if err == nil {
		t.Error("Expected error when moving from DONE without force")
	}

	// With force=true, should succeed
	req5 := MoveRequest{TargetStatus: "TODO", Force: true}
	result5, err := service.Move("ticket-transition", req5, user)
	if err != nil {
		t.Fatalf("DONE->TODO with force failed: %v", err)
	}
	if result5.Ticket.Column != "todo-0" {
		t.Errorf("Expected 'todo-0', got '%s'", result5.Ticket.Column)
	}

	// Invalid status should fail
	req6 := MoveRequest{TargetStatus: "INVALID_STATUS"}
	_, err = service.Move("ticket-transition", req6, user)
	if err == nil {
		t.Error("Expected error for invalid target status")
	}
}

// TestMoveTicket_NotFound verifies error handling for non-existent tickets.
func TestMoveTicket_NotFound(t *testing.T) {
	store := setupMoveTest(t)
	service := NewMoveService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	req := MoveRequest{TargetStatus: "IN_PROGRESS"}
	_, err := service.Move("ticket-nonexistent", req, user)
	if err == nil {
		t.Error("Expected error for non-existent ticket")
	}
	if !contains(err.Error(), ErrNotFound.Error()) {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestMoveTicket_Unauthorized verifies behavior when user is nil.
func TestMoveTicket_Unauthorized(t *testing.T) {
	store := setupMoveTest(t)
	service := NewMoveService(store)

	ticket := &models.Ticket{
		ID:      "ticket-unauth",
		Title:   "Unauthorized Move Test",
		Column:  "todo-0",
		BoardID: "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	req := MoveRequest{TargetStatus: "IN_PROGRESS"}
	_, err := service.Move("ticket-unauth", req, nil)
	if err == nil {
		t.Error("Expected error for nil user")
	}
	if !contains(err.Error(), ErrUnauthorized.Error()) {
		t.Errorf("Expected 'unauthorized' error, got: %v", err)
	}
}

// TestStatusToColumnID verifies all status to column ID mappings.
func TestStatusToColumnID(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"BACKLOG", "backlog-0"},
		{"TODO", "todo-0"},
		{"IN_PROGRESS", "inprogress-0"},
		{"REVIEW", "review-0"},
		{"DONE", "done-0"},
		{"CANCELLED", "cancelled-0"},
		{"INVALID_STATUS", ""}, // Unknown status returns empty string
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := StatusToColumnID(tt.status)
			if got != tt.want {
				t.Errorf("StatusToColumnID(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// TestColumnIDToStatus verifies all column ID to status mappings.
func TestColumnIDToStatus(t *testing.T) {
	tests := []struct {
		column string
		want   string
	}{
		{"backlog-0", "BACKLOG"},
		{"todo-0", "TODO"},
		{"inprogress-0", "IN_PROGRESS"},
		{"review-0", "REVIEW"},
		{"done-0", "DONE"},
		{"cancelled-0", "CANCELLED"},
		{"unknown-column", "TODO"}, // Unknown defaults to TODO for backward compatibility
	}

	for _, tt := range tests {
		t.Run(tt.column, func(t *testing.T) {
			got := ColumnIDToStatus(tt.column)
			if got != tt.want {
				t.Errorf("ColumnIDToStatus(%q) = %q, want %q", tt.column, got, tt.want)
			}
		})
	}
}

// TestMoveTicket_TerminalStateCancel verifies CANCELLED state handling.
func TestMoveTicket_TerminalStateCancel(t *testing.T) {
	store := setupMoveTest(t)
	service := NewMoveService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	ticket := &models.Ticket{
		ID:       "ticket-cancel-test",
		Title:    "Cancel Test Ticket",
		Column:   "todo-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Move to CANCELLED (terminal state)
	req1 := MoveRequest{TargetStatus: "CANCELLED"}
	result1, err := service.Move("ticket-cancel-test", req1, user)
	if err != nil {
		t.Fatalf("TODO->CANCELLED failed: %v", err)
	}
	if result1.Ticket.Column != "cancelled-0" {
		t.Errorf("Expected 'cancelled-0', got '%s'", result1.Ticket.Column)
	}

	// Cannot exit CANCELLED without force
	req2 := MoveRequest{TargetStatus: "TODO"}
	_, err = service.Move("ticket-cancel-test", req2, user)
	if err == nil {
		t.Error("Expected error when moving from CANCELLED without force")
	}

	// HUMAN_ADMIN can exit terminal states without force
	if _, err := store.CreateUser("admin-user", models.RoleHumanAdmin); err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	admin, _ := store.GetUserByName("admin-user")

	req3 := MoveRequest{TargetStatus: "TODO"}
	result3, err := service.Move("ticket-cancel-test", req3, admin)
	if err != nil {
		t.Fatalf("HUMAN_ADMIN CANCELLED->TODO failed: %v", err)
	}
	if result3.Ticket.Column != "todo-0" {
		t.Errorf("Expected 'todo-0', got '%s'", result3.Ticket.Column)
	}
}

// TestMoveTicket_BacklogHandling verifies BACKLOG column support.
func TestMoveTicket_BacklogHandling(t *testing.T) {
	store := setupMoveTest(t)
	service := NewMoveService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	ticket := &models.Ticket{
		ID:       "ticket-backlog-test",
		Title:    "Backlog Test Ticket",
		Column:   "backlog-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Move from BACKLOG to TODO
	req := MoveRequest{TargetStatus: "TODO"}
	result, err := service.Move("ticket-backlog-test", req, user)
	if err != nil {
		t.Fatalf("BACKLOG->TODO failed: %v", err)
	}
	if result.Ticket.Column != "todo-0" {
		t.Errorf("Expected 'todo-0', got '%s'", result.Ticket.Column)
	}

	// Verify current status is now TODO (for subsequent moves)
	currentStatus := ColumnIDToStatus(result.Ticket.Column)
	if currentStatus != "TODO" {
		t.Errorf("Expected current status 'TODO', got '%s'", currentStatus)
	}
}
