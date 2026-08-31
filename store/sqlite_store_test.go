package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"goban/config"
	"goban/models"
)

// testCounter generates unique DB filenames to avoid collisions.
var testCounter int64

func nextTestID() int64 {
	testCounter++
	return testCounter
}

// newTestStore creates an isolated SQLite store for testing using a temp file.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpFile := fmt.Sprintf("test_db_%d.db", nextTestID())
	store := &SQLiteStore{config: config.Config{DBPath: tmpFile}}
	if err := store.Init(); err != nil {
		t.Fatalf("Failed to init test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(tmpFile)
	})
	return store
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// =============================================================================
// Ticket CRUD Tests

func TestSQLiteStore_CreateAndGetTicket(t *testing.T) {
	s := newTestStore(t)

	ticket := &models.Ticket{
		ID:          "test-ticket-1",
		Title:       "Test Ticket",
		Description: "A test ticket",
		Priority:    "high",
		Column:      "todo-0",
		BoardID:     "board-1",
	}

	err := s.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	retrieved, err := s.GetTicket("test-ticket-1")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if retrieved.Title != "Test Ticket" {
		t.Errorf("Expected title 'Test Ticket', got '%s'", retrieved.Title)
	}
	if retrieved.Priority != "high" {
		t.Errorf("Expected priority 'high', got '%s'", retrieved.Priority)
	}
}

func TestSQLiteStore_GetTicket_NotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetTicket("nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected sql.ErrNoRows for nonexistent ticket, got: %v", err)
	}
}

func TestSQLiteStore_UpdateTicket(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{
		ID: "upd-1", Title: "Original", Column: "todo-0", BoardID: "b1",
	})

	ticket, _ := s.GetTicket("upd-1")
	ticket.Title = "Updated"
	ticket.Column = "inprogress-0"

	err := s.UpdateTicket(ticket)
	if err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}

	retrieved, _ := s.GetTicket("upd-1")
	if retrieved.Title != "Updated" || retrieved.Column != "inprogress-0" {
		t.Errorf("Update not persisted: title=%s column=%s", retrieved.Title, retrieved.Column)
	}
}

func TestSQLiteStore_DeleteTicket(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{ID: "del-1", Title: "Delete me", Column: "todo-0", BoardID: "b1"})

	err := s.DeleteTicket("del-1")
	if err != nil {
		t.Fatalf("DeleteTicket failed: %v", err)
	}

	_, err = s.GetTicket("del-1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Error("Expected ticket to be deleted, still accessible")
	}
}

func TestSQLiteStore_DeleteTicket_NotFound(t *testing.T) {
	s := newTestStore(t)

	err := s.DeleteTicket("nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected sql.ErrNoRows for nonexistent ticket deletion, got: %v", err)
	}
}

func TestSQLiteStore_GetAllTickets(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		s.CreateTicket(&models.Ticket{
			ID: fmt.Sprintf("all-%d", i), Title: fmt.Sprintf("Ticket %d", i),
			Column: "todo-0", BoardID: "b1",
		})
	}

	tickets, err := s.GetAllTickets()
	if err != nil {
		t.Fatalf("GetAllTickets failed: %v", err)
	}
	if len(tickets) != 5 {
		t.Errorf("Expected 5 tickets, got %d", len(tickets))
	}

	// Archived tickets should not appear in GetAllTickets
	adminID, _ := s.CreateUser("archive-admin", models.RoleHumanAdmin)
	s.CreateTicket(&models.Ticket{ID: "archived-1", Title: "Archived", Column: "todo-0", BoardID: "b1"})
	err = s.ArchiveTicket("archived-1", adminID)
	if err != nil {
		t.Fatalf("ArchiveTicket failed: %v", err)
	}

	tickets, err = s.GetAllTickets()
	if err != nil {
		t.Fatalf("GetAllTickets after archive failed: %v", err)
	}
	if len(tickets) != 5 {
		t.Errorf("Expected 5 non-archived tickets, got %d (archived should be excluded)", len(tickets))
	}
}

// =============================================================================
// Pagination Tests

func TestSQLiteStore_GetPaginatedTickets_DefaultLimit(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 30; i++ {
		s.CreateTicket(&models.Ticket{
			ID: fmt.Sprintf("page-%d", i), Title: fmt.Sprintf("Page %d", i),
			Column: "todo-0", BoardID: "b1",
		})
	}

	tickets, total, err := s.GetPaginatedTickets(Pagination{})
	if err != nil {
		t.Fatalf("GetPaginatedTickets failed: %v", err)
	}
	if total != 30 {
		t.Errorf("Expected total 30, got %d", total)
	}
	if len(tickets) > 50 {
		t.Errorf("Default limit should be 50, got %d tickets", len(tickets))
	}
}

func TestSQLiteStore_GetPaginatedTickets_CustomLimitOffset(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 10; i++ {
		s.CreateTicket(&models.Ticket{
			ID: fmt.Sprintf("p-%d", i), Title: fmt.Sprintf("P %d", i),
			Column: "todo-0", BoardID: "b1",
		})
	}

	page1, total, err := s.GetPaginatedTickets(Pagination{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("Page 1 failed: %v", err)
	}
	if len(page1) != 3 || total != 10 {
		t.Errorf("Page 1: expected 3 tickets (total=10), got %d (total=%d)", len(page1), total)
	}

	page2, _, _ := s.GetPaginatedTickets(Pagination{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("Page 2 failed: %v", err)
	}
	if len(page2) != 3 {
		t.Errorf("Page 2: expected 3 tickets, got %d", len(page2))
	}

	pageLast, _, _ := s.GetPaginatedTickets(Pagination{Limit: 3, Offset: 9})
	if err != nil {
		t.Fatalf("Last page failed: %v", err)
	}
	if len(pageLast) != 1 {
		t.Errorf("Last page: expected 1 ticket, got %d", len(pageLast))
	}

	// Beyond total should return empty
	pageEmpty, _, _ := s.GetPaginatedTickets(Pagination{Limit: 3, Offset: 100})
	if err != nil {
		t.Fatalf("Beyond range failed: %v", err)
	}
	if len(pageEmpty) != 0 {
		t.Errorf("Expected empty page beyond range, got %d tickets", len(pageEmpty))
	}
}

func TestSQLiteStore_GetPaginatedTickets_ExcludesArchived(t *testing.T) {
	s := newTestStore(t)

	adminID, _ := s.CreateUser("pag-admin", models.RoleHumanAdmin)
	s.CreateTicket(&models.Ticket{ID: "pa-1", Title: "Active 1", Column: "todo-0", BoardID: "b1"})
	s.CreateTicket(&models.Ticket{ID: "pa-2", Title: "To Archive", Column: "todo-0", BoardID: "b1"})
	err := s.ArchiveTicket("pa-2", adminID)
	if err != nil {
		t.Fatalf("ArchiveTicket failed: %v", err)
	}

	_, total, err := s.GetPaginatedTickets(Pagination{})
	if err != nil {
		t.Fatalf("GetPaginatedTickets failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Expected total 1 (excluding archived), got %d", total)
	}
}

// =============================================================================
// Ticket Filtering Tests

func TestSQLiteStore_GetTicketsWithFilter(t *testing.T) {
	s := newTestStore(t)

	for _, col := range []string{"todo-0", "inprogress-0", "done-0"} {
		s.CreateTicket(&models.Ticket{
			ID: fmt.Sprintf("f-%s", col), Title: "Filter test", Column: col, BoardID: "b1",
		})
	}

	tickets, total, err := s.GetTicketsWithFilter([]string{"todo"}, Pagination{})
	if err != nil {
		t.Fatalf("GetTicketsWithFilter failed: %v", err)
	}
	if len(tickets) != 1 || total != 1 {
		t.Errorf("Expected 1 todo ticket, got %d (total=%d)", len(tickets), total)
	}

	// Multiple column prefixes
	tickets2, total2, _ := s.GetTicketsWithFilter([]string{"todo", "inprogress"}, Pagination{})
	if len(tickets2) != 2 || total2 != 2 {
		t.Errorf("Expected 2 tickets (todo+inprogress), got %d (total=%d)", len(tickets2), total2)
	}

	// No filter returns all non-archived
	tickets3, total3, _ := s.GetTicketsWithFilter([]string{}, Pagination{})
	if len(tickets3) != 3 || total3 != 3 {
		t.Errorf("Expected 3 tickets (no filter), got %d (total=%d)", len(tickets3), total3)
	}
}

func TestSQLiteStore_GetTicketsByColumnAndAssignee(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{ID: "fa-1", Title: "T1", Column: "todo-0", Assignee: "alice", BoardID: "b1"})
	s.CreateTicket(&models.Ticket{ID: "fa-2", Title: "T2", Column: "todo-0", Assignee: "bob", BoardID: "b1"})
	s.CreateTicket(&models.Ticket{ID: "fa-3", Title: "T3", Column: "inprogress-0", Assignee: "alice", BoardID: "b1"})

	// Filter by column only
	tickets, err := s.GetTicketsByColumnAndAssignee("todo", "")
	if err != nil {
		t.Fatalf("Filter by column failed: %v", err)
	}
	if len(tickets) != 2 {
		t.Errorf("Expected 2 todo tickets, got %d", len(tickets))
	}

	// Filter by assignee only
	tickets2, _ := s.GetTicketsByColumnAndAssignee("", "alice")
	if err != nil {
		t.Fatalf("Filter by assignee failed: %v", err)
	}
	if len(tickets2) != 2 {
		t.Errorf("Expected 2 alice tickets, got %d", len(tickets2))
	}

	// Both filters
	tickets3, _ := s.GetTicketsByColumnAndAssignee("todo", "alice")
	if err != nil {
		t.Fatalf("Combined filter failed: %v", err)
	}
	if len(tickets3) != 1 || tickets3[0].ID != "fa-1" {
		t.Errorf("Expected fa-1, got ID=%s count=%d", tickets3[0].ID, len(tickets3))
	}

	// Empty filters returns all active tickets
	tickets4, _ := s.GetTicketsByColumnAndAssignee("", "")
	if err != nil {
		t.Fatalf("Empty filter failed: %v", err)
	}
	if len(tickets4) != 3 {
		t.Errorf("Expected 3 tickets with empty filters, got %d", len(tickets4))
	}
}

func TestSQLiteStore_GetTicketsByAssignee(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{ID: "ga-1", Title: "T1", Column: "todo-0", Assignee: "alice", BoardID: "b1"})
	s.CreateTicket(&models.Ticket{ID: "ga-2", Title: "T2", Column: "inprogress-0", Assignee: "bob", BoardID: "b1"})

	tickets, err := s.GetTicketsByAssignee("alice")
	if err != nil {
		t.Fatalf("GetTicketsByAssignee failed: %v", err)
	}
	if len(tickets) != 1 || tickets[0].ID != "ga-1" {
		t.Errorf("Expected ga-1 for alice, got count=%d id=%s", len(tickets), tickets[0].ID)
	}

	// Nonexistent assignee returns empty slice
	tickets2, _ := s.GetTicketsByAssignee("charlie")
	if err != nil {
		t.Fatalf("Empty result failed: %v", err)
	}
	if len(tickets2) != 0 {
		t.Errorf("Expected empty for nonexistent assignee, got %d", len(tickets2))
	}
}

// =============================================================================
// Token Tests

func TestSQLiteStore_CreateAndValidateToken(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateToken("test-agent", "abc123hash")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero token ID")
	}

	token, err := s.ValidateToken("abc123hash")
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if token.AgentName != "test-agent" {
		t.Errorf("Expected agent 'test-agent', got '%s'", token.AgentName)
	}
}

func TestSQLiteStore_CreateTokenWithUser(t *testing.T) {
	s := newTestStore(t)

	userID, _ := s.CreateUser("token-user", models.RoleNormalAI)

	id, err := s.CreateTokenWithUser(userID, "user-agent", "hash-with-user")
	if err != nil {
		t.Fatalf("CreateTokenWithUser failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero token ID")
	}

	token, _ := s.ValidateToken("hash-with-user")
	if token.UserID != userID {
		t.Errorf("Expected UserID %d, got %d", userID, token.UserID)
	}
}

func TestSQLiteStore_ValidateToken_NotFound(t *testing.T) {
	s := newTestStore(t)

	token, err := s.ValidateToken("nonexistent-hash")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected sql.ErrNoRows for invalid token hash, got: %v", err)
	}
	if token != nil {
		t.Error("ValidateToken should return nil token on error")
	}
}

func TestSQLiteStore_UpdateTokenLastUsed(t *testing.T) {
	s := newTestStore(t)

	s.CreateToken("update-agent", "hash-update")

	err := s.UpdateTokenLastUsed("hash-update")
	if err != nil {
		t.Fatalf("UpdateTokenLastUsed failed: %v", err)
	}

	token, _ := s.ValidateToken("hash-update")
	if token.LastUsed == nil {
		t.Error("Expected LastUsed to be set after UpdateTokenLastUsed")
	}
}

func TestSQLiteStore_DeleteToken(t *testing.T) {
	s := newTestStore(t)

	s.CreateToken("delete-agent", "hash-del")

	count, err := s.DeleteToken("delete-agent")
	if err != nil {
		t.Fatalf("DeleteToken failed: %v", err)
	}
	if count == 0 {
		t.Error("Expected at least 1 row affected by DeleteToken")
	}

	token, err := s.ValidateToken("hash-del")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Error("Deleted token should not be findable")
	}
	if token != nil {
		t.Error("ValidateToken should return nil for deleted token")
	}
}

func TestSQLiteStore_DeleteToken_NotFound(t *testing.T) {
	s := newTestStore(t)

	count, _ := s.DeleteToken("nonexistent-agent")
	if count != 0 {
		t.Errorf("Expected 0 rows affected for nonexistent agent, got %d", count)
	}
}

func TestSQLiteStore_ListTokens(t *testing.T) {
	s := newTestStore(t)

	s.CreateToken("list-a", "hash-a")
	s.CreateToken("list-b", "hash-b")

	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("Expected 2 tokens, got %d", len(tokens))
	}

	// Empty list when no tokens
	s2 := newTestStore(t)
	empty, _ := s2.ListTokens()
	if len(empty) != 0 {
		t.Errorf("ListTokens should return empty slice when no tokens exist, got %d", len(empty))
	}
}

func TestSQLiteStore_UpdateTokenUserID(t *testing.T) {
	s := newTestStore(t)

	userID, _ := s.CreateUser("link-user", models.RoleNormalAI)
	s.CreateToken("link-agent", "hash-link")

	err := s.UpdateTokenUserID("hash-link", userID)
	if err != nil {
		t.Fatalf("UpdateTokenUserID failed: %v", err)
	}

	token, _ := s.ValidateToken("hash-link")
	if token.UserID != userID {
		t.Errorf("Expected UserID %d after update, got %d", userID, token.UserID)
	}
}

func TestSQLiteStore_GetUserByToken(t *testing.T) {
	s := newTestStore(t)

	userID, _ := s.CreateUser("token-user-2", models.RoleNormalAI)
	s.CreateTokenWithUser(userID, "token-agent-2", "hash-token-user")

	user, err := s.GetUserByToken("hash-token-user")
	if err != nil {
		t.Fatalf("GetUserByToken failed: %v", err)
	}
	if user.ID != userID || user.Name != "token-user-2" {
		t.Errorf("Expected user ID=%d name='token-user-2', got ID=%d name='%s'", userID, user.ID, user.Name)
	}

	// Token without user linkage returns sql.ErrNoRows
	s.CreateToken("no-link-agent", "hash-no-link")
	user2, err := s.GetUserByToken("hash-no-link")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for token without user, got: %v", err)
	}
	if user2 != nil {
		t.Error("GetUserByToken should return nil for unlinked token")
	}
}

// =============================================================================
// User CRUD Tests

func TestSQLiteStore_CreateAndGetUser(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateUser("test-user", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero user ID")
	}

	user, _ := s.GetUserByID(id)
	if user.Name != "test-user" || user.Role != models.RoleNormalAI {
		t.Errorf("User mismatch: name=%s role=%s", user.Name, user.Role)
	}

	userByName, err := s.GetUserByName("test-user")
	if err != nil {
		t.Fatalf("GetUserByName failed: %v", err)
	}
	if userByName.ID != id {
		t.Errorf("GetUserByName returned wrong ID: expected %d got %d", id, userByName.ID)
	}
}

func TestSQLiteStore_CreateUser_InvalidRole(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateUser("bad-role-user", "INVALID_ROLE")
	if err == nil {
		t.Error("Expected error for invalid role, got none")
	}
	if !strings.Contains(err.Error(), "invalid role") {
		t.Errorf("Expected 'invalid role' in error, got: %v", err)
	}
}

func TestSQLiteStore_CreateUserWithPassword(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateUserWithPassword("pw-user", "password123", models.RoleHumanAdmin)
	if err != nil {
		t.Fatalf("CreateUserWithPassword failed: %v", err)
	}

	user, _ := s.GetUserByName("pw-user")
	if user == nil || user.Role != models.RoleHumanAdmin {
		t.Error("Expected user with HUMAN_ADMIN role")
	}
	if user.PasswordHash == "" {
		t.Error("Password hash should be set for CreateUserWithPassword")
	}

	// Verify bcrypt hash works
	ok, _ := VerifyPassword(user.PasswordHash, "password123")
	if !ok {
		t.Error("Stored password hash does not match original password")
	}
}

func TestSQLiteStore_GetUser_NotFound(t *testing.T) {
	s := newTestStore(t)

	user, err := s.GetUserByID(9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for nonexistent user ID, got: %v", err)
	}
	if user != nil {
		t.Error("GetUserByID should return nil on error")
	}

	user2, err := s.GetUserByName("nonexistent-user")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for nonexistent username, got: %v", err)
	}
	if user2 != nil {
		t.Error("GetUserByName should return nil on error")
	}
}

func TestSQLiteStore_UpdateUserRole(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateUser("role-user", models.RoleNormalAI)

	err := s.UpdateUserRole(id, models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}

	user, _ := s.GetUserByID(id)
	if user.Role != models.RoleOverseerAI {
		t.Errorf("Expected role OVERSEER_AI, got '%s'", user.Role)
	}

	// Invalid role should fail
	err = s.UpdateUserRole(id, "INVALID")
	if err == nil {
		t.Error("Expected error for invalid role update, got none")
	}

	// Nonexistent user ID returns ErrNoRows
	err = s.UpdateUserRole(9999, models.RoleNormalAI)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for nonexistent user update, got: %v", err)
	}
}

func TestSQLiteStore_UpdateUserPassword(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateUserWithPassword("upw-user", "old-password", models.RoleNormalAI)

	err := s.UpdateUserPassword(id, "new-password")
	if err != nil {
		t.Fatalf("UpdateUserPassword failed: %v", err)
	}

	user, _ := s.GetUserByName("upw-user")
	ok, _ := VerifyPassword(user.PasswordHash, "new-password")
	if !ok {
		t.Error("Updated password hash does not match new password")
	}

	// Old password should no longer work
	ok2, _ := VerifyPassword(user.PasswordHash, "old-password")
	if ok2 {
		t.Error("Old password still matches after update")
	}

	// Nonexistent user returns ErrNoRows
	err = s.UpdateUserPassword(9999, "test-pw")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for nonexistent user password update, got: %v", err)
	}
}

func TestSQLiteStore_DeleteUser(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateUser("del-user", models.RoleNormalAI)

	err := s.DeleteUser(id)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	user, err := s.GetUserByID(id)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Error("Deleted user should not be findable")
	}
	if user != nil {
		t.Error("GetUserByID should return nil for deleted user")
	}

	// Nonexistent user returns ErrNoRows
	err = s.DeleteUser(9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for nonexistent user deletion, got: %v", err)
	}
}

func TestSQLiteStore_ListUsers(t *testing.T) {
	s := newTestStore(t)

	s.CreateUser("list-user-1", models.RoleNormalAI)
	s.CreateUser("list-user-2", models.RoleOverseerAI)

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}

	// Verify roles are correct
	for _, u := range users {
		found := false
		for _, expectedRole := range []string{models.RoleNormalAI, models.RoleOverseerAI} {
			if u.Role == expectedRole {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected user role: '%s'", u.Role)
		}
	}

	// Empty list when no users exist
	s2 := newTestStore(t)
	empty, _ := s2.ListUsers()
	if len(empty) != 0 {
		t.Errorf("ListUsers should return empty slice when no users exist, got %d", len(empty))
	}
}

// =============================================================================
// Archive Tests

func TestSQLiteStore_ArchiveAndUnarchiveTicket(t *testing.T) {
	s := newTestStore(t)

	adminID, _ := s.CreateUser("arc-admin", models.RoleHumanAdmin)
	s.CreateTicket(&models.Ticket{ID: "arc-1", Title: "Archive me", Column: "todo-0", BoardID: "b1"})

	err := s.ArchiveTicket("arc-1", adminID)
	if err != nil {
		t.Fatalf("ArchiveTicket failed: %v", err)
	}

	// Archived ticket should appear in GetArchivedTickets
	archived, _ := s.GetArchivedTickets("b1")
	if len(archived) != 1 || archived[0].ID != "arc-1" {
		t.Errorf("Expected arc-1 in archived list, got count=%d", len(archived))
	}

	// Should not appear in GetAllTickets
	all, _ := s.GetAllTickets()
	for _, tk := range all {
		if tk.ID == "arc-1" {
			t.Error("Archived ticket should not appear in GetAllTickets")
			break
		}
	}

	// Unarchive
	err = s.UnarchiveTicket("arc-1")
	if err != nil {
		t.Fatalf("UnarchiveTicket failed: %v", err)
	}

	all2, _ := s.GetAllTickets()
	found := false
	for _, tk := range all2 {
		if tk.ID == "arc-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Unarchived ticket should appear in GetAllTickets")
	}
}

func TestSQLiteStore_ArchiveTicket_NotFound(t *testing.T) {
	s := newTestStore(t)

	adminID, _ := s.CreateUser("nf-admin", models.RoleHumanAdmin)
	err := s.ArchiveTicket("nonexistent", adminID)
	if err == nil {
		t.Error("Expected error for archiving nonexistent ticket")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' in error, got: %v", err)
	}
}

func TestSQLiteStore_UnarchiveTicket_NotFound(t *testing.T) {
	s := newTestStore(t)

	err := s.UnarchiveTicket("nonexistent")
	if err == nil {
		t.Error("Expected error for unarchiving nonexistent ticket")
	}
}

func TestSQLiteStore_BulkArchive(t *testing.T) {
	s := newTestStore(t)

	adminID, _ := s.CreateUser("bulk-admin", models.RoleHumanAdmin)

	for i := 0; i < 3; i++ {
		s.CreateTicket(&models.Ticket{
			ID: fmt.Sprintf("ba-%d", i), Title: fmt.Sprintf("Bulk %d", i), Column: "todo-0", BoardID: "b1",
		})
	}

	err := s.ArchiveTicketsBulk([]string{"ba-0", "ba-1"}, adminID)
	if err != nil {
		t.Fatalf("ArchiveTicketsBulk failed: %v", err)
	}

	archived, _ := s.GetArchivedByAdmin(adminID)
	if len(archived) != 2 {
		t.Errorf("Expected 2 archived tickets, got %d", len(archived))
	}

	// ba-2 should still be active
	all, _ := s.GetAllTickets()
	found := false
	for _, tk := range all {
		if tk.ID == "ba-2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ba-2 should still be active after partial bulk archive")
	}

	// Empty list should succeed without error
	errEmpty := s.ArchiveTicketsBulk([]string{}, adminID)
	if errEmpty != nil {
		t.Errorf("Expected no error for empty bulk archive, got: %v", errEmpty)
	}
}

func TestSQLiteStore_GetArchivedByAdmin(t *testing.T) {
	s := newTestStore(t)

	adminA, _ := s.CreateUser("ga-admin-a", models.RoleHumanAdmin)
	adminB, _ := s.CreateUser("ga-admin-b", models.RoleHumanAdmin)
	s.CreateTicket(&models.Ticket{ID: "ga-1", Title: "GA 1", Column: "todo-0", BoardID: "b1"})
	s.CreateTicket(&models.Ticket{ID: "ga-2", Title: "GA 2", Column: "todo-0", BoardID: "b1"})

	s.ArchiveTicket("ga-1", adminA)
	s.ArchiveTicket("ga-2", adminB)

	byAdminA, _ := s.GetArchivedByAdmin(adminA)
	if len(byAdminA) != 1 || byAdminA[0].ID != "ga-1" {
		t.Errorf("Expected ga-1 archived by admin A, got count=%d", len(byAdminA))
	}

	byAdminB, _ := s.GetArchivedByAdmin(adminB)
	if len(byAdminB) != 1 || byAdminB[0].ID != "ga-2" {
		t.Errorf("Expected ga-2 archived by admin B, got count=%d", len(byAdminB))
	}

	// Nonexistent admin returns empty list
	byNobody, err := s.GetArchivedByAdmin(999)
	if err != nil {
		t.Fatalf("GetArchivedByAdmin failed: %v", err)
	}
	if len(byNobody) != 0 {
		t.Errorf("Expected empty for nonexistent admin, got %d", len(byNobody))
	}
}

func TestSQLiteStore_GetAllArchivedTickets(t *testing.T) {
	s := newTestStore(t)

	adminA, _ := s.CreateUser("gaa-admin-a", models.RoleHumanAdmin)
	adminB, _ := s.CreateUser("gaa-admin-b", models.RoleHumanAdmin)
	s.CreateTicket(&models.Ticket{ID: "gaa-1", Title: "GA 1", Column: "todo-0", BoardID: "b1"})
	s.CreateTicket(&models.Ticket{ID: "gaa-2", Title: "GA 2", Column: "todo-0", BoardID: "b2"})

	s.ArchiveTicket("gaa-1", adminA)
	s.ArchiveTicket("gaa-2", adminB)

	allArchived, err := s.GetAllArchivedTickets()
	if err != nil {
		t.Fatalf("GetAllArchivedTickets failed: %v", err)
	}
	if len(allArchived) != 2 {
		t.Errorf("Expected 2 archived tickets across boards, got %d", len(allArchived))
	}

	// Empty when no archived tickets exist
	s2 := newTestStore(t)
	empty, _ := s2.GetAllArchivedTickets()
	if len(empty) != 0 {
		t.Errorf("GetAllArchivedTickets should return empty slice when none archived, got %d", len(empty))
	}
}

// =============================================================================
// Transaction Tests

func TestSQLiteStore_TransactionCommit(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{ID: "tx-1", Title: "Before TX", Column: "todo-0", BoardID: "b1"})

	tx, err := s.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	ticket, _ := tx.GetTicket("tx-1")
	ticket.Title = "Modified in TX"
	err = tx.UpdateTicket(ticket)
	if err != nil {
		t.Fatalf("UpdateTicket in TX failed: %v", err)
	}

	// Before commit, main store should still show original value
	beforeCommit, _ := s.GetTicket("tx-1")
	if beforeCommit.Title != "Before TX" {
		t.Error("Transaction changes should not be visible before commit")
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// After commit, changes should persist
	afterCommit, _ := s.GetTicket("tx-1")
	if afterCommit.Title != "Modified in TX" {
		t.Errorf("Expected 'Modified in TX' after commit, got '%s'", afterCommit.Title)
	}
}

func TestSQLiteStore_TransactionRollback(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{ID: "rb-1", Title: "Original", Column: "todo-0", BoardID: "b1"})

	tx, err := s.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	ticket, _ := tx.GetTicket("rb-1")
	ticket.Title = "Modified in TX"
	err = tx.UpdateTicket(ticket)
	if err != nil {
		t.Fatalf("UpdateTicket in TX failed: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// After rollback, original value should persist
	afterRollback, _ := s.GetTicket("rb-1")
	if afterRollback.Title != "Original" {
		t.Errorf("Expected 'Original' after rollback, got '%s'", afterRollback.Title)
	}
}

func TestSQLiteStore_TransactivityLog(t *testing.T) {
	s := newTestStore(t)

	tx, err := s.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	logID, err := tx.CreateActivityLog(&models.ActivityLog{
		TicketID:  "tx-log-ticket",
		EventType: models.ActivityClaimed,
		Actor:     "test-agent",
	})
	if err != nil {
		t.Fatalf("CreateActivityLog in TX failed: %v", err)
	}
	if logID == 0 {
		t.Error("Expected non-zero activity log ID")
	}

	tx.Rollback() // Rollback so the log is not persisted

	logs, _ := s.GetActivityLogs("tx-log-ticket", 50)
	if len(logs) != 0 {
		t.Errorf("Rolled back transaction should not persist activity logs, got %d", len(logs))
	}
}

// =============================================================================
// Activity Log Tests

func TestSQLiteStore_CreateAndGetActivityLog(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateActivityLog(&models.ActivityLog{
		TicketID:  "log-ticket-1",
		EventType: models.ActivityClaimed,
		Actor:     "hermes",
		PrevState: strPtr("todo"),
		NewState:  strPtr("inprogress"),
		Metadata:  `{"subtasks":3}`,
	})
	if err != nil {
		t.Fatalf("CreateActivityLog failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero activity log ID")
	}

	logs, err := s.GetActivityLogs("log-ticket-1", 50)
	if err != nil {
		t.Fatalf("GetActivityLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("Expected 1 activity log, got %d", len(logs))
	}
	log := logs[0]
	if log.EventType != models.ActivityClaimed || log.Actor != "hermes" {
		t.Errorf("Activity log mismatch: event=%s actor=%s", log.EventType, log.Actor)
	}
	if *log.PrevState != "todo" || *log.NewState != "inprogress" {
		t.Errorf("States mismatch: prev=%s new=%s", *log.PrevState, *log.NewState)
	}
}

func TestSQLiteStore_GetActivityLogs_Limit(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 10; i++ {
		s.CreateActivityLog(&models.ActivityLog{
			TicketID: "limit-ticket", EventType: models.ActivityMoved, Actor: fmt.Sprintf("actor-%d", i),
		})
	}

	logs, err := s.GetActivityLogs("limit-ticket", 3)
	if err != nil {
		t.Fatalf("GetActivityLogs with limit failed: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("Expected 3 logs with limit=3, got %d", len(logs))
	}

	// Default limit when 0 is passed — returns all since only 10 exist (default=50 > actual count)
	logs2, _ := s.GetActivityLogs("limit-ticket", 0)
	if len(logs2) != 10 { // default limit of 50, but only 10 logs exist
		t.Errorf("Expected all 10 logs with default limit, got %d", len(logs2))
	}

	// Max limit capping at 100
	logs3, _ := s.GetActivityLogs("limit-ticket", 999)
	if len(logs3) != 10 { // only 10 exist, but max is capped at 100
		t.Errorf("Expected all 10 logs with limit=999 (capped at 100), got %d", len(logs3))
	}

	// Nonexistent ticket returns empty slice
	logs4, _ := s.GetActivityLogs("nonexistent-ticket", 50)
	if err != nil {
		t.Fatalf("GetActivityLogs for nonexistent failed: %v", err)
	}
	if len(logs4) != 0 {
		t.Errorf("Expected empty logs for nonexistent ticket, got %d", len(logs4))
	}
}

// =============================================================================
// Nullable Field Tests (regression tests for known bugs)

func TestSQLiteStore_ArchivedByNullableScan(t *testing.T) {
	s := newTestStore(t)

	ticket := &models.Ticket{ID: "null-1", Title: "No archived_by", Column: "todo-0", BoardID: "b1"}
	err := s.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	retrieved, err := s.GetTicket("null-1")
	if err != nil {
		t.Fatalf("GetTicket with null archived_by failed (regression for nullable scan bug): %v", err)
	}
	if retrieved.ArchivedBy != nil {
		t.Errorf("Expected ArchivedBy to be nil, got %d", *retrieved.ArchivedBy)
	}

	// Now archive and verify ArchivedBy is set
	adminID, _ := s.CreateUser("ab-admin", models.RoleHumanAdmin)
	s.ArchiveTicket("null-1", adminID)
	retrieved2, _ := s.GetTicket("null-1")
	if retrieved2.ArchivedBy == nil || *retrieved2.ArchivedBy != int(adminID) {
		t.Errorf("Expected ArchivedBy=%d after archive, got %v", adminID, retrieved2.ArchivedBy)
	}
}

func TestSQLiteStore_DueDateNullable(t *testing.T) {
	s := newTestStore(t)

	dueDate := "2026-12-31T23:59:59Z"
	ticket := &models.Ticket{ID: "due-1", Title: "Has due date", Column: "todo-0", BoardID: "b1", DueDate: &dueDate}

	err := s.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket with due_date failed: %v", err)
	}

	retrieved, _ := s.GetTicket("due-1")
	if retrieved.DueDate == nil || *retrieved.DueDate != dueDate {
		t.Errorf("Expected DueDate='%s', got %v", dueDate, retrieved.DueDate)
	}

	// Ticket without due date should have nil
	s.CreateTicket(&models.Ticket{ID: "due-2", Title: "No due date", Column: "todo-0", BoardID: "b1"})
	retrieved2, _ := s.GetTicket("due-2")
	if retrieved2.DueDate != nil {
		t.Errorf("Expected nil DueDate for ticket without due date, got '%s'", *retrieved2.DueDate)
	}
}

// =============================================================================
// Close Tests

func TestSQLiteStore_Close(t *testing.T) {
	s := newTestStore(t)

	err := s.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// After close, operations should fail
	err = s.CreateTicket(&models.Ticket{ID: "after-close", Title: "Should fail", Column: "todo-0", BoardID: "b1"})
	if err == nil {
		t.Error("Expected error after Close, got none")
	}
}

func TestSQLiteStore_CloseNilDB(t *testing.T) {
	s := &SQLiteStore{} // No Init() called
	err := s.Close()
	if err != nil {
		t.Errorf("Close on uninitialized store should not fail, got: %v", err)
	}
}

// =============================================================================
// Duplicate Constraint Tests

func TestSQLiteStore_TicketDuplicateID(t *testing.T) {
	s := newTestStore(t)

	err1 := s.CreateTicket(&models.Ticket{ID: "dup-1", Title: "First", Column: "todo-0", BoardID: "b1"})
	if err1 != nil {
		t.Fatalf("First CreateTicket failed: %v", err1)
	}

	err2 := s.CreateTicket(&models.Ticket{ID: "dup-1", Title: "Duplicate", Column: "todo-0", BoardID: "b1"})
	if err2 == nil {
		t.Error("Expected error for duplicate ticket ID, got none")
	}
}

func TestSQLiteStore_UserDuplicateName(t *testing.T) {
	s := newTestStore(t)

	_, err1 := s.CreateUser("dup-user", models.RoleNormalAI)
	if err1 != nil {
		t.Fatalf("First CreateUser failed: %v", err1)
	}

	_, err2 := s.CreateUser("dup-user", models.RoleOverseerAI)
	if err2 == nil {
		t.Error("Expected error for duplicate username, got none")
	}
}

func TestSQLiteStore_TokenDuplicateHash(t *testing.T) {
	s := newTestStore(t)

	_, err1 := s.CreateToken("agent-a", "same-hash")
	if err1 != nil {
		t.Fatalf("First CreateToken failed: %v", err1)
	}

	_, err2 := s.CreateToken("agent-b", "same-hash")
	if err2 == nil {
		t.Error("Expected error for duplicate token hash, got none")
	}
}

func TestSQLiteStore_TokenDuplicateAgentName(t *testing.T) {
	s := newTestStore(t)

	_, err1 := s.CreateToken("dup-agent", "hash-1")
	if err1 != nil {
		t.Fatalf("First CreateToken failed: %v", err1)
	}

	_, err2 := s.CreateToken("dup-agent", "hash-2")
	if err2 == nil {
		t.Error("Expected error for duplicate agent_name (UNIQUE constraint), got none")
	}
}

// =============================================================================
// Init Tests

func TestSQLiteStore_InitCreatesTables(t *testing.T) {
	s := newTestStore(t)

	// Verify tables exist by inserting and querying each one
	err := s.CreateTicket(&models.Ticket{ID: "init-test", Title: "Init test", Column: "todo-0", BoardID: "b1"})
	if err != nil {
		t.Fatalf("CreateTicket failed after Init (tables may not exist): %v", err)
	}

	_, err = s.CreateUser("init-user", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed after Init: %v", err)
	}

	_, err = s.CreateToken("init-agent", "init-hash")
	if err != nil {
		t.Fatalf("CreateToken failed after Init: %v", err)
	}

	_, err = s.CreateActivityLog(&models.ActivityLog{TicketID: "init-test", EventType: models.ActivityClaimed, Actor: "test"})
	if err != nil {
		t.Fatalf("CreateActivityLog failed after Init: %v", err)
	}
}

func TestSQLiteStore_ReInit(t *testing.T) {
	s := newTestStore(t)

	// Re-init should succeed (CREATE TABLE IF NOT EXISTS)
	err := s.Init()
	if err != nil {
		t.Fatalf("Re-Init failed: %v", err)
	}

	// Data should still be accessible after re-init
	s.CreateTicket(&models.Ticket{ID: "reinit-test", Title: "After re-init", Column: "todo-0", BoardID: "b1"})
	tk, _ := s.GetTicket("reinit-test")
	if tk == nil || tk.Title != "After re-init" {
		t.Error("Data should persist after re-init")
	}
}

// =============================================================================
// Ticket JSON Fields Tests (labels, subtasks, comments)

func TestSQLiteStore_TicketJSONFields(t *testing.T) {
	s := newTestStore(t)

	labels := []string{"bug", "urgent"}
	subtasks := []models.Subtask{{Title: "Sub 1"}}
	comments := []models.Comment{{Who: "alice", Text: "Comment 1"}}

	ticket := &models.Ticket{
		ID:       "json-1",
		Title:    "JSON Fields Test",
		Column:   "todo-0",
		BoardID:  "b1",
		Labels:   labels,
		Subtasks: subtasks,
		Comments: comments,
	}

	err := s.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket with JSON fields failed: %v", err)
	}

	retrieved, _ := s.GetTicket("json-1")
	if len(retrieved.Labels) != 2 || retrieved.Labels[0] != "bug" {
		t.Errorf("Labels mismatch after roundtrip: %v", retrieved.Labels)
	}
	if len(retrieved.Subtasks) != 1 {
		t.Errorf("Expected 1 subtask, got %d", len(retrieved.Subtasks))
	}
	if len(retrieved.Comments) != 1 || retrieved.Comments[0].Text != "Comment 1" {
		t.Errorf("Comments mismatch: %v", retrieved.Comments)
	}
}

// =============================================================================
// Foreign Key Constraint Tests

func TestSQLiteStore_FKConstraintArchivedBy(t *testing.T) {
	s := newTestStore(t)

	s.CreateTicket(&models.Ticket{ID: "fk-1", Title: "FK test", Column: "todo-0", BoardID: "b1"})

	// Archive with a user ID that exists in the users table
	adminID, _ := s.CreateUser("admin", models.RoleHumanAdmin)
	err := s.ArchiveTicket("fk-1", adminID)
	if err != nil {
		t.Fatalf("ArchiveTicket with valid FK failed: %v", err)
	}

	retrieved, _ := s.GetTicket("fk-1")
	if retrieved.ArchivedBy == nil || *retrieved.ArchivedBy != int(adminID) {
		t.Errorf("Expected ArchivedBy=%d after archive, got %v", adminID, retrieved.ArchivedBy)
	}
}

// =============================================================================
// Time-based Tests

func TestSQLiteStore_TicketTimestamps(t *testing.T) {
	s := newTestStore(t)

	now := time.Now()
	ticket := &models.Ticket{
		ID: "time-1", Title: "Time test", Column: "todo-0", BoardID: "b1",
		CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}

	err := s.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	retrieved, _ := s.GetTicket("time-1")
	if retrieved.CreatedAt == "" || retrieved.UpdatedAt == "" {
		t.Error("CreatedAt and UpdatedAt should not be zero after roundtrip")
	}
}

func TestSQLiteStore_UserTimestamps(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateUser("time-user", models.RoleNormalAI)
	time.Sleep(10 * time.Millisecond) // Small delay to ensure timestamps differ

	user, _ := s.GetUserByID(id)
	if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		t.Error("User CreatedAt and UpdatedAt should not be zero")
	}
}

func TestSQLiteStore_CreateOrGetTicket(t *testing.T) {
	store := newTestStore(t)
	defer os.Remove(store.config.DBPath)

	// First call with idempotency key should create a new ticket
	ticket1 := &models.Ticket{
		ID:             models.GenerateTicketID(),
		Title:          "Idempotent Ticket",
		Description:    "Created via CreateOrGetTicket",
		Priority:       "medium",
		Column:         "todo-0",
		BoardID:        "test-board-1",
		IdempotencyKey: "unique-key-abc",
	}
	result1, err := store.CreateOrGetTicket(ticket1)
	if err != nil {
		t.Fatalf("CreateOrGetTicket(1st): unexpected error: %v", err)
	}

	// Should be a newly created ticket with generated ID
	if result1.ID == "" {
		t.Fatal("Expected non-empty ticket ID")
	}
	expectedID := result1.ID

	// Second call with same idempotency key should return the existing ticket
	ticket2 := &models.Ticket{
		ID:             models.GenerateTicketID(),
		Title:          "Duplicate Ticket",
		Description:    "Should not be created",
		Priority:       "low",
		Column:         "todo-0",
		BoardID:        "test-board-1",
		IdempotencyKey: "unique-key-abc",
	}
	result2, err := store.CreateOrGetTicket(ticket2)
	if err != nil {
		t.Fatalf("CreateOrGetTicket(2nd): unexpected error: %v", err)
	}

	// Should return the same ticket (same ID, original title preserved)
	if result2.ID != expectedID {
		t.Errorf("Expected existing ticket ID %s, got %s", expectedID, result2.ID)
	}
	if result2.Title != "Idempotent Ticket" {
		t.Errorf("Expected original title 'Idempotent Ticket', got %q", result2.Title)
	}

	// Call with no idempotency key should always create a new ticket
	ticket3 := &models.Ticket{
		ID:          models.GenerateTicketID(),
		Title:       "No Key Ticket",
		Description: "Should be created fresh each time",
		Column:      "todo-0",
		BoardID:     "test-board-1",
	}
	result3, err := store.CreateOrGetTicket(ticket3)
	if err != nil {
		t.Fatalf("CreateOrGetTicket(no key): unexpected error: %v", err)
	}

	// Call again with no key should create another new ticket (different ID)
	ticket4 := &models.Ticket{
		ID:      models.GenerateTicketID(),
		Title:   "Another No Key Ticket",
		Column:  "todo-0",
		BoardID: "test-board-1",
	}
	result4, err := store.CreateOrGetTicket(ticket4)
	if err != nil {
		t.Fatalf("CreateOrGetTicket(no key 2nd): unexpected error: %v", err)
	}

	if result3.ID == result4.ID {
		t.Errorf("Expected different IDs for tickets without idempotency key, both got %s", result3.ID)
	}

	// Verify that archived tickets don't block new creation with same key
	// Create a user first for the FK constraint on archived_by
	adminID, err := store.CreateUser("test-admin", models.RoleHumanAdmin)
	if err != nil {
		t.Fatalf("CreateUser: unexpected error: %v", err)
	}

	archivedTicket := &models.Ticket{
		ID:             models.GenerateTicketID(),
		Title:          "Archived Ticket",
		Column:         "todo-0",
		BoardID:        "test-board-1",
		IdempotencyKey: "unique-key-xyz",
	}
	r, err := store.CreateOrGetTicket(archivedTicket)
	if err != nil {
		t.Fatalf("CreateOrGetTicket (archived setup): unexpected error: %v", err)
	}

	// Archive it with valid admin user ID
	err = store.ArchiveTicket(r.ID, adminID)
	if err != nil {
		t.Fatalf("ArchiveTicket: unexpected error: %v", err)
	}

	// Now create with same key should succeed (archived ticket is ignored)
	newAfterArchived := &models.Ticket{
		ID:             models.GenerateTicketID(),
		Title:          "New After Archived",
		Column:         "todo-0",
		BoardID:        "test-board-1",
		IdempotencyKey: "unique-key-xyz",
	}
	r2, err := store.CreateOrGetTicket(newAfterArchived)
	if err != nil {
		t.Fatalf("CreateOrGetTicket (after archive): unexpected error: %v", err)
	}

	// Should be a new ticket, not the archived one
	if r2.ID == r.ID {
		t.Errorf("Expected new ticket ID after archiving old one with same key")
	}
	if r2.Title != "New After Archived" {
		t.Errorf("Expected title 'New After Archived', got %q", r2.Title)
	}
}
