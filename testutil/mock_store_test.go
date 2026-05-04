package testutil

import (
	"testing"

	"goban/models"
)

// TestMockStoreBasicOperations verifies basic CRUD operations work.
func TestMockStoreBasicOperations(t *testing.T) {
	store := NewMockStore()

	if err := store.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create a test ticket
	ticket := &models.Ticket{
		ID:          "ticket-test-001",
		Title:       "Test Ticket",
		Description: "A test ticket for mock store verification",
		Priority:    "medium",
		Assignee:    "",
		Column:      "todo-0",
		BoardID:     "test-board",
	}

	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	// Retrieve the ticket
	retrieved, err := store.GetTicket("ticket-test-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetTicket returned nil for existing ticket")
	}
	if retrieved.Title != "Test Ticket" {
		t.Errorf("Expected title 'Test Ticket', got '%s'", retrieved.Title)
	}

	// Update the ticket
	retrieved.Assignee = "test-agent"
	retrieved.Column = "inprogress-0"
	if err := store.UpdateTicket(retrieved); err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}

	// Verify update
	retrieved2, _ := store.GetTicket("ticket-test-001")
	if retrieved2.Assignee != "test-agent" {
		t.Errorf("Expected assignee 'test-agent', got '%s'", retrieved2.Assignee)
	}
	if retrieved2.Column != "inprogress-0" {
		t.Errorf("Expected column 'inprogress-0', got '%s'", retrieved2.Column)
	}

	// Test GetAllTickets
	all, err := store.GetAllTickets()
	if err != nil {
		t.Fatalf("GetAllTickets failed: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("Expected 1 ticket, got %d", len(all))
	}

	// Test DeleteTicket
	if err := store.DeleteTicket("ticket-test-001"); err != nil {
		t.Fatalf("DeleteTicket failed: %v", err)
	}

	deleted, _ := store.GetTicket("ticket-test-001")
	if deleted != nil {
		t.Error("GetTicket returned non-nil for deleted ticket")
	}

	if err := store.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestMockStoreTransaction verifies transaction commit/rollback behavior.
func TestMockStoreTransaction(t *testing.T) {
	store := NewMockStore()

	// Create initial ticket
	ticket := &models.Ticket{
		ID:      "ticket-tx-test",
		Title:   "Transaction Test Ticket",
		Column:  "todo-0",
		BoardID: "test-board",
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	// Start transaction and modify ticket
	tx, err := store.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	ticketInTx, _ := tx.GetTicket("ticket-tx-test")
	ticketInTx.Column = "inprogress-0"
	ticketInTx.Assignee = "tx-agent"
	if err := tx.UpdateTicket(ticketInTx); err != nil {
		t.Fatalf("UpdateTicket in transaction failed: %v", err)
	}

	// Verify main store is unchanged during transaction
	mainStoreTicket, _ := store.GetTicket("ticket-tx-test")
	if mainStoreTicket.Column == "inprogress-0" {
		t.Error("Main store changed before commit - transaction isolation broken")
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify changes persisted after commit
	afterCommit, _ := store.GetTicket("ticket-tx-test")
	if afterCommit.Column != "inprogress-0" || afterCommit.Assignee != "tx-agent" {
		t.Errorf("Changes not persisted after commit: column=%s, assignee=%s",
			afterCommit.Column, afterCommit.Assignee)
	}

	// Test rollback - start new transaction
	tx2, _ := store.BeginTx()
	ticketInTx2, _ := tx2.GetTicket("ticket-tx-test")
	ticketInTx2.Column = "done-0"
	if err := tx2.UpdateTicket(ticketInTx2); err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}

	// Rollback transaction
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify rollback - should still be inprogress-0
	afterRollback, _ := store.GetTicket("ticket-tx-test")
	if afterRollback.Column == "done-0" {
		t.Error("Rollback did not discard changes")
	}
	if afterRollback.Column != "inprogress-0" {
		t.Errorf("Expected column 'inprogress-0' after rollback, got '%s'", afterRollback.Column)
	}
}

// TestMockStoreUsers verifies user CRUD operations for RBAC testing.
func TestMockStoreUsers(t *testing.T) {
	store := NewMockStore()

	// Create users with different roles
	_, err := store.CreateUser("hermes", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = store.CreateUser("overseer-01", models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = store.CreateUser("admin", models.RoleHumanAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Retrieve by name
	user, err := store.GetUserByName("hermes")
	if err != nil {
		t.Fatalf("GetUserByName failed: %v", err)
	}
	if user == nil || user.Name != "hermes" {
		t.Error("GetUserByName returned wrong user")
	}
	if user.Role != models.RoleNormalAI {
		t.Errorf("Expected role 'NORMAL_AI', got '%s'", user.Role)
	}

	// Update role
	err = store.UpdateUserRole(user.ID, models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}

	updatedUser, _ := store.GetUserByID(user.ID)
	if updatedUser.Role != models.RoleOverseerAI {
		t.Errorf("Expected role 'OVERSEER_AI' after update, got '%s'", updatedUser.Role)
	}

	// List users
	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}

	// Delete user
	err = store.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	usersAfterDelete, _ := store.ListUsers()
	if len(usersAfterDelete) != 2 {
		t.Errorf("Expected 2 users after delete, got %d", len(usersAfterDelete))
	}
}

// TestMockStoreGetTicketsByColumnAndAssignee verifies filtering operations.
func TestMockStoreGetTicketsByColumnAndAssignee(t *testing.T) {
	store := NewMockStore()

	// Create tickets with different columns and assignees
	tickets := []*models.Ticket{
		{ID: "ticket-1", Title: "Ticket 1", Column: "todo-0", Assignee: ""},
		{ID: "ticket-2", Title: "Ticket 2", Column: "inprogress-0", Assignee: "agent-a"},
		{ID: "ticket-3", Title: "Ticket 3", Column: "inprogress-0", Assignee: "agent-b"},
		{ID: "ticket-4", Title: "Ticket 4", Column: "done-0", Assignee: "agent-a"},
	}

	for _, ticket := range tickets {
		if err := store.CreateTicket(ticket); err != nil {
			t.Fatalf("CreateTicket failed for %s: %v", ticket.ID, err)
		}
	}

	// Find all IN_PROGRESS tickets for agent-a
	result, err := store.GetTicketsByColumnAndAssignee("inprogress", "agent-a")
	if err != nil {
		t.Fatalf("GetTicketsByColumnAndAssignee failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "ticket-2" {
		t.Errorf("Expected [ticket-2], got %d tickets", len(result))
	}

	// Find all IN_PROGRESS tickets (any assignee)
	result2, _ := store.GetTicketsByColumnAndAssignee("inprogress", "")
	if len(result2) != 2 {
		t.Errorf("Expected 2 inprogress tickets, got %d", len(result2))
	}
}
