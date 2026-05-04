// Package services contains business logic for Goban operations.
package services

import (
	"testing"

	"goban/models"
	"goban/testutil"
)

// setupClaimTest creates a mock store with test data for claim tests.
func setupClaimTest(t *testing.T) *testutil.MockStore {
	store := testutil.NewMockStore()
	if err := store.Init(); err != nil {
		t.Fatalf("Failed to initialize mock store: %v", err)
	}
	return store
}

// TestClaimTicket_PermissionChecks verifies role-based permission enforcement.
func TestClaimTicket_PermissionChecks(t *testing.T) {
	tests := []struct {
		name        string
		userRole    string
		ticketOwner string // Empty = unassigned
		wantErr     bool
		errContains string
	}{
		{
			name:        "HUMAN_ADMIN can claim any ticket",
			userRole:    models.RoleHumanAdmin,
			ticketOwner: "other-agent",
			wantErr:     false,
		},
		{
			name:        "OVERSEER_AI can claim any ticket",
			userRole:    models.RoleOverseerAI,
			ticketOwner: "other-agent",
			wantErr:     false,
		},
		{
			name:        "NORMAL_AI cannot steal another AI's ticket",
			userRole:    models.RoleNormalAI,
			ticketOwner: "other-agent",
			wantErr:     true,
			errContains: ErrForbidden.Error(),
		},
		{
			name:        "NORMAL_AI can claim unassigned ticket",
			userRole:    models.RoleNormalAI,
			ticketOwner: "",
			wantErr:     false,
		},
		{
			name:        "NORMAL_AI can claim own ticket (re-claim)",
			userRole:    models.RoleNormalAI,
			ticketOwner: "normal-agent",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupClaimTest(t)
			service := NewClaimService(store)

			// Create user
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

			// Create ticket with specified owner
			ticket := &models.Ticket{
				ID:       "ticket-perm-test",
				Title:    "Permission Test Ticket",
				Column:   "todo-0",
				BoardID:  "test-board",
				Assignee: tt.ticketOwner,
			}
			if err := store.CreateTicket(ticket); err != nil {
				t.Fatalf("Failed to create ticket: %v", err)
			}

			result, err := service.Claim("ticket-perm-test", user)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if result.Ticket.Assignee != userName {
					t.Errorf("Expected assignee '%s', got '%s'", userName, result.Ticket.Assignee)
				}
			}
		})
	}
}

// TestClaimTicket_AutoReleaseExistingInProgress verifies auto-release behavior.
func TestClaimTicket_AutoReleaseExistingInProgress(t *testing.T) {
	store := setupClaimTest(t)
	service := NewClaimService(store)

	// Create agent user
	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	// Create existing IN_PROGRESS ticket for this agent
	existingTicket := &models.Ticket{
		ID:       "ticket-existing",
		Title:    "Existing Ticket",
		Column:   "inprogress-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(existingTicket); err != nil {
		t.Fatalf("Failed to create existing ticket: %v", err)
	}

	// Create new ticket to claim
	newTicket := &models.Ticket{
		ID:      "ticket-new",
		Title:   "New Ticket",
		Column:  "todo-0",
		BoardID: "test-board",
	}
	if err := store.CreateTicket(newTicket); err != nil {
		t.Fatalf("Failed to create new ticket: %v", err)
	}

	// Claim new ticket - should auto-release existing one
	result, err := service.Claim("ticket-new", user)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// Verify new ticket is claimed
	if result.Ticket.Assignee != "test-agent" {
		t.Errorf("Expected assignee 'test-agent', got '%s'", result.Ticket.Assignee)
	}
	if result.Ticket.Column != "inprogress-0" {
		t.Errorf("Expected column 'inprogress-0', got '%s'", result.Ticket.Column)
	}

	// Verify auto-release occurred
	if len(result.AutoReleased) != 1 {
		t.Errorf("Expected 1 auto-released ticket, got %d", len(result.AutoReleased))
	}
	if result.AutoReleased[0] != "ticket-existing" {
		t.Errorf("Expected 'ticket-existing' to be released, got '%s'", result.AutoReleased[0])
	}

	// Verify existing ticket was actually reset
	releasedTicket, _ := store.GetTicket("ticket-existing")
	if releasedTicket.Column != "todo-0" {
		t.Errorf("Expected released ticket column 'todo-0', got '%s'", releasedTicket.Column)
	}
	if releasedTicket.Assignee != "" {
		t.Errorf("Expected released ticket to have empty assignee, got '%s'", releasedTicket.Assignee)
	}
}

// TestClaimTicket_ReclaimOwnTicketIsIdempotent verifies re-claiming own ticket doesn't cause issues.
func TestClaimTicket_ReclaimOwnTicketIsIdempotent(t *testing.T) {
	store := setupClaimTest(t)
	service := NewClaimService(store)

	// Create user and unassigned ticket (correct initial state for claiming)
	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	ticket := &models.Ticket{
		ID:      "ticket-own",
		Title:   "My Ticket",
		Column:  "todo-0", // Start in TODO and unassigned
		BoardID: "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// First claim - should assign and move to inprogress
	result1, err := service.Claim("ticket-own", user)
	if err != nil {
		t.Fatalf("First claim failed: %v", err)
	}
	if result1.Ticket.Assignee != "test-agent" {
		t.Errorf("Expected assignee 'test-agent', got '%s'", result1.Ticket.Assignee)
	}

	// Verify ticket is now in IN_PROGRESS after successful claim
	dbTicket, _ := store.GetTicket("ticket-own")
	if dbTicket.Column != "inprogress-0" {
		t.Errorf("After first claim, expected column 'inprogress-0', got '%s'", dbTicket.Column)
	}

	// Second claim (reclaim same ticket already in IN_PROGRESS and assigned to us) - should be idempotent
	result2, err := service.Claim("ticket-own", user)
	if err != nil {
		t.Fatalf("Reclaim failed: %v", err)
	}
	if result2.Ticket.Assignee != "test-agent" {
		t.Errorf("Expected assignee 'test-agent', got '%s'", result2.Ticket.Assignee)
	}
	if len(result2.AutoReleased) > 0 {
		t.Errorf("Reclaim should not trigger auto-release, got %d released", len(result2.AutoReleased))
	}

	// Verify ticket state unchanged after reclaim (idempotent behavior)
	dbTicketAfterReclaim, _ := store.GetTicket("ticket-own")
	if dbTicketAfterReclaim.Column != "inprogress-0" {
		t.Errorf("After reclaim, column should remain 'inprogress-0', got '%s'", dbTicketAfterReclaim.Column)
	}

	// Third claim - still idempotent, no auto-release triggered
	result3, err := service.Claim("ticket-own", user)
	if err != nil {
		t.Fatalf("Third claim failed: %v", err)
	}
	if len(result3.AutoReleased) > 0 {
		t.Error("Multiple reclaims should not trigger auto-release")
	}
}

// TestClaimTicket_NotFound verifies error handling for non-existent tickets.
func TestClaimTicket_NotFound(t *testing.T) {
	store := setupClaimTest(t)
	service := NewClaimService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create test-agent: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	_, err := service.Claim("ticket-nonexistent", user)
	if err == nil {
		t.Error("Expected error for non-existent ticket")
	}
	if !contains(err.Error(), ErrNotFound.Error()) {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestClaimTicket_Unauthorized verifies behavior when user is nil.
func TestClaimTicket_Unauthorized(t *testing.T) {
	store := setupClaimTest(t)
	service := NewClaimService(store)

	ticket := &models.Ticket{
		ID:      "ticket-unauth",
		Title:   "Unauthorized Test Ticket",
		Column:  "todo-0",
		BoardID: "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	_, err := service.Claim("ticket-unauth", nil)
	if err == nil {
		t.Error("Expected error for nil user")
	}
	if !contains(err.Error(), ErrUnauthorized.Error()) {
		t.Errorf("Expected 'unauthorized' error, got: %v", err)
	}
}

// Helper function to check if string contains substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
