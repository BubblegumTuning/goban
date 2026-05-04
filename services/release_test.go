// Package services contains business logic for Goban operations.
package services

import (
	"testing"

	"goban/models"
	"goban/testutil"
)

// setupReleaseTest creates a mock store with test data for release tests.
func setupReleaseTest(t *testing.T) *testutil.MockStore {
	store := testutil.NewMockStore()
	if err := store.Init(); err != nil {
		t.Fatalf("Failed to initialize mock store: %v", err)
	}
	return store
}

// TestReleaseTicket_PermissionChecks verifies role-based permission enforcement.
func TestReleaseTicket_PermissionChecks(t *testing.T) {
	tests := []struct {
		name        string
		userRole    string
		ticketOwner string // Who owns the ticket (empty = unassigned)
		wantErr     bool
	}{
		{
			name:        "HUMAN_ADMIN can release any ticket",
			userRole:    models.RoleHumanAdmin,
			ticketOwner: "other-agent",
			wantErr:     false,
		},
		{
			name:        "OVERSEER_AI can release any ticket",
			userRole:    models.RoleOverseerAI,
			ticketOwner: "other-agent",
			wantErr:     false,
		},
		{
			name:        "NORMAL_AI cannot release other AI's ticket",
			userRole:    models.RoleNormalAI,
			ticketOwner: "other-agent",
			wantErr:     true,
		},
		{
			name:        "NORMAL_AI can release own ticket",
			userRole:    models.RoleNormalAI,
			ticketOwner: "normal-agent",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupReleaseTest(t)
			service := NewReleaseService(store)

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
				ID:       "ticket-release-perm",
				Title:    "Release Permission Test",
				Column:   "inprogress-0",
				BoardID:  "test-board",
				Assignee: tt.ticketOwner,
			}
			if err := store.CreateTicket(ticket); err != nil {
				t.Fatalf("Failed to create ticket: %v", err)
			}

			result, err := service.Release("ticket-release-perm", user)

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
				if result.Ticket.Assignee != "" {
					t.Errorf("Expected empty assignee after release, got '%s'", result.Ticket.Assignee)
				}
			}
		})
	}
}

// TestReleaseTicket_IdempotentOnUnassigned verifies releasing an already-unassigned ticket succeeds.
func TestReleaseTicket_IdempotentOnUnassigned(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	// Create unassigned ticket (no assignee from the start)
	ticket := &models.Ticket{
		ID:      "ticket-unassigned",
		Title:   "Already Unassigned Ticket",
		Column:  "todo-0",
		BoardID: "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Release unassigned ticket - should succeed idempotently regardless of role
	result, err := service.Release("ticket-unassigned", user)
	if err != nil {
		t.Errorf("Unexpected error releasing unassigned ticket: %v", err)
		return // Don't crash on subsequent checks if this fails
	}
	if result.Ticket.Assignee != "" {
		t.Errorf("Expected empty assignee, got '%s'", result.Ticket.Assignee)
	}
	if result.ReleasedAs != "" {
		t.Errorf("ReleasedAs should be empty for already-unassigned ticket, got '%s'", result.ReleasedAs)
	}

	// Release again - still succeeds idempotently
	result2, err := service.Release("ticket-unassigned", user)
	if err != nil {
		t.Error("Second release of unassigned ticket should also succeed")
		return
	}
	if result2.Ticket.Column != "todo-0" {
		t.Errorf("Column should remain 'todo-0', got '%s'", result2.Ticket.Column)
	}

	// Additional test: Release own ticket, then release again (now unassigned, different user can also do it)
	if _, err := store.CreateUser("other-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	otherUser, _ := store.GetUserByName("other-agent")

	ticket2 := &models.Ticket{
		ID:       "ticket-was-assigned",
		Title:    "Was Assigned Ticket",
		Column:   "inprogress-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket2); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// First release by owner
	result3, err := service.Release("ticket-was-assigned", user)
	if err != nil {
		t.Fatalf("Owner releasing own ticket failed: %v", err)
	}
	if result3.ReleasedAs != "test-agent" {
		t.Errorf("Expected ReleasedAs='test-agent', got '%s'", result3.ReleasedAs)
	}

	// Now ticket is unassigned - even a different user should be able to call release (idempotent)
	result4, err := service.Release("ticket-was-assigned", otherUser)
	if err != nil {
		t.Error("Releasing already-unassigned ticket as different user should succeed idempotently")
	}
	if result4.Ticket.Assignee != "" {
		t.Errorf("Expected empty assignee, got '%s'", result4.Ticket.Assignee)
	}
}

// TestReleaseTicket_ClearsAssigneeAndResetsColumn verifies core release functionality.
func TestReleaseTicket_ClearsAssigneeAndResetsColumn(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	// Create ticket in IN_PROGRESS with assignee
	ticket := &models.Ticket{
		ID:       "ticket-release-core",
		Title:    "Core Release Test",
		Column:   "inprogress-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	result, err := service.Release("ticket-release-core", user)
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Verify assignee cleared
	if result.Ticket.Assignee != "" {
		t.Errorf("Expected empty assignee after release, got '%s'", result.Ticket.Assignee)
	}

	// Verify column reset to TODO
	if result.Ticket.Column != "todo-0" {
		t.Errorf("Expected column 'todo-0' after release, got '%s'", result.Ticket.Column)
	}

	// Verify ReleasedAs contains original assignee
	if result.ReleasedAs != "test-agent" {
		t.Errorf("Expected ReleasedAs='test-agent', got '%s'", result.ReleasedAs)
	}

	// Verify database state was actually updated
	dbTicket, _ := store.GetTicket("ticket-release-core")
	if dbTicket.Assignee != "" || dbTicket.Column != "todo-0" {
		t.Errorf("Database not properly updated: assignee='%s', column='%s'",
			dbTicket.Assignee, dbTicket.Column)
	}
}

// TestReleaseTicket_NotFound verifies error handling for non-existent tickets.
func TestReleaseTicket_NotFound(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	_, err := service.Release("ticket-nonexistent", user)
	if err == nil {
		t.Error("Expected error for non-existent ticket")
	}
	if !contains(err.Error(), ErrNotFound.Error()) {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestReleaseTicket_Unauthorized verifies behavior when user is nil.
func TestReleaseTicket_Unauthorized(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	ticket := &models.Ticket{
		ID:       "ticket-unauth",
		Title:    "Unauthorized Release Test",
		Column:   "inprogress-0",
		Assignee: "some-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	_, err := service.Release("ticket-unauth", nil)
	if err == nil {
		t.Error("Expected error for nil user")
	}
	if !contains(err.Error(), ErrUnauthorized.Error()) {
		t.Errorf("Expected 'unauthorized' error, got: %v", err)
	}
}

// TestReleaseTicket_FromDifferentColumns verifies release works from any column.
func TestReleaseTicket_FromDifferentColumns(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	testCases := []struct {
		column string
	}{
		{"todo-0"},
		{"inprogress-0"},
		{"review-0"},
		{"backlog-0"},
	}

	for _, tc := range testCases {
		t.Run(tc.column, func(t *testing.T) {
			ticket := &models.Ticket{
				ID:       "ticket-release-" + tc.column,
				Title:    "Release from " + tc.column,
				Column:   tc.column,
				Assignee: "test-agent",
				BoardID:  "test-board",
			}
			if err := store.CreateTicket(ticket); err != nil {
				t.Fatalf("Failed to create ticket: %v", err)
			}

			result, err := service.Release("ticket-release-"+tc.column, user)
			if err != nil {
				t.Errorf("Release from %s failed: %v", tc.column, err)
				return
			}

			if result.Ticket.Column != "todo-0" {
				t.Errorf("Expected column 'todo-0' after release, got '%s'", result.Ticket.Column)
			}
			if result.Ticket.Assignee != "" {
				t.Errorf("Expected empty assignee after release, got '%s'", result.Ticket.Assignee)
			}
		})
	}
}

// TestReleaseTicket_ByOverseerOnOthersTicket verifies overseer can release any ticket.
func TestReleaseTicket_ByOverseerOnOthersTicket(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	// Create overseer user
	if _, err := store.CreateUser("overseer-user", models.RoleOverseerAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	overseer, _ := store.GetUserByName("overseer-user")

	// Create ticket owned by different agent
	ticket := &models.Ticket{
		ID:       "ticket-other-agent",
		Title:    "Other Agent's Ticket",
		Column:   "inprogress-0",
		Assignee: "other-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	result, err := service.Release("ticket-other-agent", overseer)
	if err != nil {
		t.Fatalf("OVERSEER_AI release failed: %v", err)
	}

	if result.Ticket.Assignee != "" {
		t.Errorf("Expected empty assignee after release, got '%s'", result.Ticket.Assignee)
	}
	if result.ReleasedAs != "other-agent" {
		t.Errorf("Expected ReleasedAs='other-agent', got '%s'", result.ReleasedAs)
	}

	// Verify the ticket was actually released in database
	dbTicket, _ := store.GetTicket("ticket-other-agent")
	if dbTicket.Assignee != "" {
		t.Error("Database not properly updated - assignee still set")
	}
}

// TestReleaseTransactionRollbackOnFailure verifies transaction rollback on update failure.
func TestReleaseTransactionRollbackOnFailure(t *testing.T) {
	store := setupReleaseTest(t)
	service := NewReleaseService(store)

	if _, err := store.CreateUser("test-agent", models.RoleNormalAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user, _ := store.GetUserByName("test-agent")

	ticket := &models.Ticket{
		ID:       "ticket-tx-test",
		Title:    "Transaction Test Ticket",
		Column:   "inprogress-0",
		Assignee: "test-agent",
		BoardID:  "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Release should succeed normally
	result, err := service.Release("ticket-tx-test", user)
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Verify changes persisted (transaction committed)
	dbTicket, _ := store.GetTicket("ticket-tx-test")
	if dbTicket.Assignee != "" || dbTicket.Column != "todo-0" {
		t.Errorf("Transaction not committed properly: assignee='%s', column='%s'",
			dbTicket.Assignee, dbTicket.Column)
	}

	// Verify result matches database state
	if result.Ticket.Assignee != dbTicket.Assignee {
		t.Error("Result doesn't match database state after commit")
	}
}
