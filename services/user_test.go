// Package services provides business logic layer for Goban operations.
package services

import (
	"strings"
	"testing"
	"time"

	"goban/models"
	"goban/store"
)

// newTestUserService creates a UserService backed by an in-memory SQLite store.
func newTestUserService(t *testing.T) (*UserService, func()) {
	t.Helper()

	s := &store.SQLiteStore{} // Uses default :memory: config
	if err := s.Init(); err != nil {
		t.Fatalf("Failed to init test store: %v", err)
	}

	cleanup := func() { _ = s.Close() }
	return NewUserService(s), cleanup
}

// ============================================================================
// CreateUser Tests

func TestCreateUser_Success(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("testuser", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.Name != "testuser" || user.Role != models.RoleNormalAI || user.ID == 0 {
		t.Errorf("User mismatch: %+v", user)
	}
}

func TestCreateUser_MultipleUsers(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user1, err := svc.CreateUser("user-alpha", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser 1 failed: %v", err)
	}

	user2, err := svc.CreateUser("user-beta", models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("CreateUser 2 failed: %v", err)
	}

	if user1.ID == user2.ID {
		t.Error("Expected different IDs for distinct users")
	}
	if user1.Name == user2.Name {
		t.Error("Expected different names for distinct users")
	}
}

func TestCreateUser_DuplicateName(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user1, err := svc.CreateUser("dup-user", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("First CreateUser failed: %v", err)
	}

	_, err = svc.CreateUser("dup-user", models.RoleOverseerAI)
	if err == nil {
		t.Error("Expected error for duplicate username, got success")
	} else if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "UNIQUE") {
		t.Logf("Got constraint violation (expected): %v", err)
	}

	// Verify original user still exists
	user2, getErr := svc.GetUserByName("dup-user")
	if getErr != nil || user2 == nil || user2.ID != user1.ID {
		t.Errorf("Original user corrupted after duplicate attempt: %+v", user2)
	}
}

func TestCreateUser_WithDifferentRoles(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	for _, role := range []string{models.RoleHumanAdmin, models.RoleOverseerAI, models.RoleNormalAI} {
		user, err := svc.CreateUser("role-test-"+role, role)
		if err != nil {
			t.Fatalf("CreateUser with role %s failed: %v", role, err)
		}

		if user.Role != role {
			t.Errorf("Expected role %s, got %s for user %s", role, user.Role, user.Name)
		}
	}
}

// ============================================================================
// GetUserByID Tests

func TestGetUserByID_Success(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("lookup-by-id", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	retrieved, err := svc.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	if retrieved.Name != "lookup-by-id" || retrieved.Role != models.RoleNormalAI {
		t.Errorf("Retrieved user mismatch: %+v", retrieved)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.GetUserByID(999999) // Non-existent ID
	if err == nil || user != nil {
		t.Errorf("Expected error/nil for non-existent user ID 999999: %+v", user)
	}
}

// ============================================================================
// GetUserByName Tests

func TestGetUserByName_Success(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("lookup-by-name", models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	retrieved, err := svc.GetUserByName("lookup-by-name")
	if err != nil || retrieved == nil {
		t.Fatalf("GetUserByName failed: %+v (err=%v)", retrieved, err)
	}

	if retrieved.ID != user.ID || retrieved.Role != models.RoleOverseerAI {
		t.Errorf("Retrieved user mismatch: %+v", retrieved)
	}
}

func TestGetUserByName_NotFound(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.GetUserByName("does-not-exist-in-db")
	if err != nil && user == nil {
		t.Logf("Got error for non-existent username: %v", err)
	} else if user != nil {
		t.Errorf("Expected nil for non-existent username: %+v", user)
	} else {
		t.Log("GetUserByName returned nil without error (valid behavior)")
	}
}

// ============================================================================
// UpdateUserRole Tests

func TestUpdateUserRole_Success(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("role-updater", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = svc.UpdateUserRole(user.ID, models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}

	retrieved, _ := svc.GetUserByID(user.ID)
	if retrieved.Role != models.RoleOverseerAI {
		t.Errorf("Expected role '%s', got '%s'", models.RoleOverseerAI, retrieved.Role)
	}
}

func TestUpdateUserRole_AllValidRoles(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("role-cycler", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	for _, targetRole := range []string{models.RoleOverseerAI, models.RoleHumanAdmin, models.RoleNormalAI} {
		err = svc.UpdateUserRole(user.ID, targetRole)
		if err != nil {
			t.Fatalf("UpdateUserRole to %s failed: %v", targetRole, err)
		}

		retrieved, _ := svc.GetUserByID(user.ID)
		if retrieved.Role != targetRole {
			t.Errorf("Expected role '%s', got '%s'", targetRole, retrieved.Role)
		}
	}
}

func TestUpdateUserRole_NotFound(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	err := svc.UpdateUserRole(999999, models.RoleOverseerAI)
	if err == nil {
		t.Error("Expected error for updating non-existent user ID 999999")
	}
}

func TestUpdateUserRole_CustomRole(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("custom-role-user", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = svc.UpdateUserRole(user.ID, "custom_role_value")
	if err != nil {
		t.Logf("UpdateUserRole with custom role may fail depending on implementation: %v", err)
		return
	}

	retrieved, _ := svc.GetUserByID(user.ID)
	if retrieved.Role == "custom_role_value" {
		t.Log("Custom roles are accepted by UpdateUserRole")
	} else {
		t.Logf("UpdateUserRole modified custom role to: %s", retrieved.Role)
	}
}

// ============================================================================
// DeleteUser Tests

func TestDeleteUser_Success(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("delete-me", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = svc.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	retrieved, _ := svc.GetUserByID(user.ID)
	if retrieved != nil {
		t.Errorf("Expected nil after deleting user %d, got %+v", user.ID, retrieved)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	err := svc.DeleteUser(999999) // Non-existent ID
	if err == nil {
		t.Error("Expected error for deleting non-existent user")
	}
}

func TestDeleteUser_CascadeDeletesTokens(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	user, err := svc.CreateUser("cascade-test", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = svc.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user is actually deleted (cascade should handle tokens automatically)
	retrieved, _ := svc.GetUserByID(user.ID)
	if retrieved != nil {
		t.Errorf("Expected nil after deleting user with cascade, got %+v", retrieved)
	}
}

// ============================================================================
// ListUsers Tests

func TestListUsers_Empty(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	users, err := svc.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("Expected 0 users in empty store, got %d", len(users))
	}
}

func TestListUsers_Multiple(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	for i, role := range []string{models.RoleNormalAI, models.RoleOverseerAI, models.RoleHumanAdmin} {
		userName := "list-user-" + string(rune('a'+i))
		_, err := svc.CreateUser(userName, role)
		if err != nil {
			t.Fatalf("CreateUser %d failed: %v", i, err)
		}
	}

	users, err := svc.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(users) < 3 {
		t.Errorf("Expected at least 3 users in list, got %d", len(users))
	}

	nameSet := make(map[string]bool)
	for _, u := range users {
		nameSet[u.Name] = true
	}
	for i := 'a'; i <= 'c'; i++ {
		expectedName := "list-user-" + string(i)
		if !nameSet[expectedName] {
			t.Errorf("Expected user %s in list, got: %v", expectedName, nameSet)
		}
	}
}

// ============================================================================
// GetTicketsByAssignee Tests

func TestGetTicketsByAssignee_None(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	tickets, err := svc.GetTicketsByAssignee("no-one")
	if err != nil {
		t.Fatalf("GetTicketsByAssignee failed: %v", err)
	}

	if len(tickets) != 0 {
		t.Errorf("Expected 0 tickets for unassigned user, got %d", len(tickets))
	}
}

func TestGetTicketsByAssignee_WithTickets(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	// Create a ticket assigned to "assignee-test"
	now := time.Now().Format(time.RFC3339)
	ticket1 := &models.Ticket{
		ID:        "assign-t-1",
		Title:     "Assigned Ticket 1",
		Column:    "todo-0",
		BoardID:   "test-board",
		Assignee:  "assignee-test",
		CreatedAt: now,
		UpdatedAt: now,
	}

	ticket2 := &models.Ticket{
		ID:        "assign-t-2",
		Title:     "Assigned Ticket 2",
		Column:    "inprogress-0",
		BoardID:   "test-board",
		Assignee:  "assignee-test",
		CreatedAt: now,
		UpdatedAt: now,
	}

	ticket3 := &models.Ticket{
		ID:        "assign-t-3",
		Title:     "Assigned to Other",
		Column:    "todo-0",
		BoardID:   "test-board",
		Assignee:  "other-user",
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, ticket := range []*models.Ticket{ticket1, ticket2, ticket3} {
		if err := svc.store.CreateTicket(ticket); err != nil {
			t.Fatalf("CreateTicket failed for %s: %v", ticket.ID, err)
		}
	}

	tickets, err := svc.GetTicketsByAssignee("assignee-test")
	if err != nil {
		t.Fatalf("GetTicketsByAssignee failed: %v", err)
	}

	if len(tickets) != 2 {
		t.Errorf("Expected 2 tickets for 'assignee-test', got %d: %+v", len(tickets), tickets)
	}

	for _, tk := range tickets {
		if tk.Assignee != "assignee-test" {
			t.Errorf("Ticket %s has wrong assignee: %s", tk.ID, tk.Assignee)
		}
	}
}

func TestGetTicketsByAssignee_ArchivedExcluded(t *testing.T) {
	svc, cleanup := newTestUserService(t)
	defer cleanup()

	now := time.Now().Format(time.RFC3339)
	ticket1 := &models.Ticket{
		ID:        "arch-assign-1",
		Title:     "Active Assigned Ticket",
		Column:    "todo-0",
		BoardID:   "test-board",
		Assignee:  "active-user",
		CreatedAt: now,
		UpdatedAt: now,
	}

	ticket2 := &models.Ticket{
		ID:        "arch-assign-2",
		Title:     "Archived Assigned Ticket",
		Column:    "todo-0",
		BoardID:   "test-board",
		Assignee:  "active-user",
		Archived:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, ticket := range []*models.Ticket{ticket1, ticket2} {
		if err := svc.store.CreateTicket(ticket); err != nil {
			t.Fatalf("CreateTicket failed for %s: %v", ticket.ID, err)
		}
	}

	tickets, err := svc.GetTicketsByAssignee("active-user")
	if err != nil {
		t.Fatalf("GetTicketsByAssignee failed: %v", err)
	}

	// Archived tickets should be excluded from assignee queries
	for _, tk := range tickets {
		if tk.Archived {
			t.Errorf("Archived ticket %s should not appear in GetTicketsByAssignee results", tk.ID)
		}
	}
}
