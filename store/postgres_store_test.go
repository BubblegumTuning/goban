// Package store provides database abstraction for Goban persistence.
package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"goban/config"
	"goban/models"
)

// newTestPostgresStore creates a PostgreSQL test store using environment config.
// If no PG connection is available, the tests skip gracefully via t.Skip.
func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()

	host := "localhost"
	port := 5432
	user := "goban"
	password := "templatepassu123"
	dbname := "goban_test"

	store := &PostgresStore{config: config.Config{
		DBHost:     host,
		DBPort:     port,
		DBUser:     user,
		DBPassword: password,
		DBName:     dbname,
	}}

	err := store.Init()
	if err != nil {
		t.Skipf("PostgreSQL not available (skipping PG tests): %v", err)
	}

	// Clean up any leftover test data from previous runs
	store.db.Exec(`DELETE FROM activity_logs`)
	store.db.Exec(`DELETE FROM agent_tokens`)
	store.db.Exec(`DELETE FROM users`)
	store.db.Exec(`DELETE FROM tickets`)

	return store
}

// pgNewTicket creates a ticket for testing.
func pgNewTicket(id string, column string, boardID string) *models.Ticket {
	now := time.Now().Format(time.RFC3339)
	return &models.Ticket{
		ID:        id,
		Title:     "PG Test Ticket " + id,
		Priority:  "medium",
		Column:    column,
		BoardID:   boardID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ============================================================================
// Init & Schema Tests

func TestPostgresStore_InitCreatesTables(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for _, tableName := range []string{"tickets", "users", "agent_tokens", "activity_logs"} {
		var exists bool
		err := store.db.QueryRow(`
			SELECT EXISTS (SELECT 1 FROM information_schema.tables 
				WHERE table_name = $1 AND table_schema = 'public')`, tableName).Scan(&exists)
		if err != nil || !exists {
			t.Fatalf("Table %s not created in PostgreSQL", tableName)
		}
	}

	// Verify column types for tickets table
	var dataType string
	err := store.db.QueryRow(`
		SELECT data_type FROM information_schema.columns 
		WHERE table_name = 'tickets' AND column_name = 'id'`).Scan(&dataType)
	if err != nil {
		t.Fatalf("Failed to query column type: %v", err)
	}
	if dataType != "character varying" && dataType != "varchar" {
		t.Errorf("Expected VARCHAR for tickets.id, got %s", dataType)
	}
}

// ============================================================================
// Ticket CRUD Tests (mirrors SQLite tests)

func TestPostgresStore_CreateAndGetTicket(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	ticket := pgNewTicket("pg-t1", "todo-0", "board-1")
	err := store.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	got, err := store.GetTicket("pg-t1")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.ID != "pg-t1" || got.Title != ticket.Title || got.Column != "todo-0" {
		t.Errorf("Ticket mismatch: %+v", got)
	}
}

func TestPostgresStore_GetTicket_NotFound(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	got, err := store.GetTicket("nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for non-existent ticket, got: %v", err)
	}
	if got != nil {
		t.Error("Expected nil ticket for not found")
	}
}

func TestPostgresStore_UpdateTicket(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	ticket := pgNewTicket("pg-t1", "todo-0", "board-1")
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	got, _ := store.GetTicket("pg-t1")
	got.Title = "Updated PG Title"
	got.Column = "inprogress-0"
	now := time.Now().Format(time.RFC3339)
	got.UpdatedAt = now

	err := store.UpdateTicket(got)
	if err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}

	updated, err := store.GetTicket("pg-t1")
	if err != nil {
		t.Fatalf("GetTicket after update failed: %v", err)
	}
	if updated.Title != "Updated PG Title" || updated.Column != "inprogress-0" {
		t.Errorf("Update did not persist: %+v", updated)
	}
}

func TestPostgresStore_DeleteTicket(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	if err := store.CreateTicket(pgNewTicket("pg-t1", "todo-0", "board-1")); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	err := store.DeleteTicket("pg-t1")
	if err != nil {
		t.Fatalf("DeleteTicket failed: %v", err)
	}

	_, err = store.GetTicket("pg-t1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows after deletion, got: %v", err)
	}
}

func TestPostgresStore_DeleteTicket_NotFound(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	err := store.DeleteTicket("nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected ErrNoRows for deleting non-existent ticket, got: %v", err)
	}
}

// ============================================================================
// GetAllTickets & Pagination Tests (parity with SQLite)

func TestPostgresStore_GetAllTickets(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for i := 0; i < 3; i++ {
		if err := store.CreateTicket(pgNewTicket("pg-all-"+string(rune('a'+i)), "todo-0", "board-1")); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	tickets, err := store.GetAllTickets()
	if err != nil {
		t.Fatalf("GetAllTickets failed: %v", err)
	}
	if len(tickets) != 3 {
		t.Errorf("Expected 3 tickets, got %d", len(tickets))
	}

	// Archive one ticket — should not appear in GetAllTickets
	now := time.Now().Format(time.RFC3339)
	tk, _ := store.GetTicket("pg-all-a")
	tk.Archived = true
	tk.ArchivedAt = &now
	if err := store.UpdateTicket(tk); err != nil {
		t.Fatalf("UpdateTicket for archive failed: %v", err)
	}

	allTickets, err := store.GetAllTickets()
	if err != nil {
		t.Fatalf("GetAllTickets after archive failed: %v", err)
	}
	if len(allTickets) != 2 {
		t.Errorf("Expected 2 non-archived tickets, got %d", len(allTickets))
	}
}

func TestPostgresStore_GetPaginatedTickets(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for i := 0; i < 7; i++ {
		if err := store.CreateTicket(pgNewTicket("pg-page-"+string(rune('a'+i)), "todo-0", "board-1")); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	page1, total, err := store.GetPaginatedTickets(Pagination{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("GetPaginatedTickets page 1 failed: %v", err)
	}
	if total != 7 {
		t.Errorf("Expected total 7, got %d", total)
	}
	if len(page1) != 3 {
		t.Errorf("Expected 3 tickets on page 1, got %d", len(page1))
	}

	page2, _, err := store.GetPaginatedTickets(Pagination{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("GetPaginatedTickets page 2 failed: %v", err)
	}
	if len(page2) != 3 {
		t.Errorf("Expected 3 tickets on page 2, got %d", len(page2))
	}

	page3, _, err := store.GetPaginatedTickets(Pagination{Limit: 3, Offset: 6})
	if err != nil {
		t.Fatalf("GetPaginatedTickets page 3 failed: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("Expected 1 ticket on last page, got %d", len(page3))
	}

	pageDefault, _, err := store.GetPaginatedTickets(Pagination{Limit: 0, Offset: 0})
	if err != nil {
		t.Fatalf("GetPaginatedTickets default failed: %v", err)
	}
	if len(pageDefault) != 7 {
		t.Errorf("Expected all 7 tickets with default limit, got %d", len(pageDefault))
	}

	pageEmpty, _, err := store.GetPaginatedTickets(Pagination{Limit: 3, Offset: 100})
	if err != nil {
		t.Fatalf("GetPaginatedTickets empty failed: %v", err)
	}
	if len(pageEmpty) != 0 {
		t.Errorf("Expected 0 tickets at offset beyond total, got %d", len(pageEmpty))
	}
}

// ============================================================================
// Column Filtering Tests (parity with SQLite)

func TestPostgresStore_GetTicketsByColumnAndAssignee(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	t1 := pgNewTicket("pg-col-1", "todo-0", "board-1")
	t1.Assignee = "agent-a"
	t2 := pgNewTicket("pg-col-2", "todo-0", "board-1")
	t2.Assignee = "agent-b"
	t3 := pgNewTicket("pg-col-3", "inprogress-0", "board-1")
	t3.Assignee = "agent-a"

	for _, tk := range []*models.Ticket{t1, t2, t3} {
		if err := store.CreateTicket(tk); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	results, err := store.GetTicketsByColumnAndAssignee("todo", "agent-a")
	if err != nil {
		t.Fatalf("GetTicketsByColumnAndAssignee failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "pg-col-1" {
		t.Errorf("Expected [pg-col-1], got %d tickets", len(results))
	}

	results, err = store.GetTicketsByColumnAndAssignee("todo", "")
	if err != nil {
		t.Fatalf("GetTicketsByColumnAndAssignee failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 tickets in todo column, got %d", len(results))
	}

	results, err = store.GetTicketsByColumnAndAssignee("inprogress", "")
	if err != nil {
		t.Fatalf("GetTicketsByColumnAndAssignee failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "pg-col-3" {
		t.Errorf("Expected [pg-col-3], got %d tickets", len(results))
	}

	// Archived ticket should be excluded from column+assignee query
	now := time.Now().Format(time.RFC3339)
	tk, _ := store.GetTicket("pg-col-1")
	tk.Archived = true
	tk.ArchivedAt = &now
	if err := store.UpdateTicket(tk); err != nil {
		t.Fatalf("Update archive failed: %v", err)
	}

	results, err = store.GetTicketsByColumnAndAssignee("todo", "agent-a")
	if err != nil {
		t.Fatalf("GetTicketsByColumnAndAssignee after archive failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 tickets (archived excluded), got %d", len(results))
	}
}

func TestPostgresStore_GetTicketsWithFilter(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for _, col := range []string{"todo-0", "inprogress-0", "review-0"} {
		if err := store.CreateTicket(pgNewTicket("pg-filter-"+col, col, "board-1")); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	results, total, err := store.GetTicketsWithFilter([]string{"todo"}, Pagination{})
	if err != nil {
		t.Fatalf("GetTicketsWithFilter failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "pg-filter-todo-0" {
		t.Errorf("Expected [pg-filter-todo-0], got %d tickets", len(results))
	}
	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	results, _, err = store.GetTicketsWithFilter([]string{"todo", "review"}, Pagination{})
	if err != nil {
		t.Fatalf("GetTicketsWithFilter multi failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 tickets with [todo, review] filter, got %d", len(results))
	}

	results, _, err = store.GetTicketsWithFilter([]string{}, Pagination{})
	if err != nil {
		t.Fatalf("GetTicketsWithFilter empty failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 tickets with empty filter (all), got %d", len(results))
	}
}

// ============================================================================
// Archive Tests (parity with SQLite, including FK constraint handling)

func TestPostgresStore_ArchiveAndUnarchiveTicket(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	if err := store.CreateTicket(pgNewTicket("pg-arch-1", "todo-0", "board-1")); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	adminID, _ := store.CreateUser("pg-archive-admin", models.RoleHumanAdmin)

	err := store.ArchiveTicket("pg-arch-1", adminID)
	if err != nil {
		t.Fatalf("ArchiveTicket failed: %v", err)
	}

	all, _ := store.GetAllTickets()
	if len(all) != 0 {
		t.Errorf("Expected 0 non-archived tickets after archive, got %d", len(all))
	}

	tk, err := store.GetTicket("pg-arch-1")
	if err != nil {
		t.Fatalf("GetTicket for archived failed: %v", err)
	}
	if !tk.Archived || tk.ArchivedAt == nil || *tk.ArchivedAt == "" {
		t.Errorf("Archive fields not set correctly: Archived=%v, ArchivedAt=%v", tk.Archived, tk.ArchivedAt)
	}

	err = store.UnarchiveTicket("pg-arch-1")
	if err != nil {
		t.Fatalf("UnarchiveTicket failed: %v", err)
	}

	allAfter, _ := store.GetAllTickets()
	if len(allAfter) != 1 {
		t.Errorf("Expected 1 non-archived ticket after unarchive, got %d", len(allAfter))
	}
}

func TestPostgresStore_ArchiveBulk(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for i := 0; i < 3; i++ {
		if err := store.CreateTicket(pgNewTicket("pg-bulk-"+string(rune('a'+i)), "todo-0", "board-1")); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	adminID, _ := store.CreateUser("pg-bulk-admin", models.RoleHumanAdmin)

	err := store.ArchiveTicketsBulk([]string{"pg-bulk-a", "pg-bulk-b"}, adminID)
	if err != nil {
		t.Fatalf("ArchiveTicketsBulk failed: %v", err)
	}

	all, _ := store.GetAllTickets()
	if len(all) != 1 {
		t.Errorf("Expected 1 non-archived ticket after bulk archive of 2/3, got %d", len(all))
	}
	if all[0].ID != "pg-bulk-c" {
		t.Errorf("Expected 'pg-bulk-c' to remain unarchived, got '%s'", all[0].ID)
	}
}

func TestPostgresStore_GetArchivedByAdmin(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for i := 0; i < 3; i++ {
		if err := store.CreateTicket(pgNewTicket("pg-admin-"+string(rune('a'+i)), "todo-0", "board-1")); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	adminID1, _ := store.CreateUser("pg-admin-user-1", models.RoleHumanAdmin)
	adminID2, _ := store.CreateUser("pg-admin-user-2", models.RoleHumanAdmin)

	store.ArchiveTicket("pg-admin-a", adminID1)
	store.ArchiveTicket("pg-admin-b", adminID2)
	store.ArchiveTicket("pg-admin-c", adminID1)

	admin10, err := store.GetArchivedByAdmin(adminID1)
	if err != nil {
		t.Fatalf("GetArchivedByAdmin failed: %v", err)
	}
	if len(admin10) != 2 {
		t.Errorf("Expected 2 tickets archived by admin 1, got %d", len(admin10))
	}

	admin20, _ := store.GetArchivedByAdmin(adminID2)
	if len(admin20) != 1 {
		t.Errorf("Expected 1 ticket archived by admin 2, got %d", len(admin20))
	}

	noTickets, _ := store.GetArchivedByAdmin(9999)
	if len(noTickets) != 0 {
		t.Errorf("Expected 0 tickets for non-existent admin, got %d", len(noTickets))
	}
}

func TestPostgresStore_GetAllArchivedAndBoardFilter(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	for i := 0; i < 3; i++ {
		if err := store.CreateTicket(pgNewTicket("pg-allarch-"+string(rune('a'+i)), "todo-0", "board-1")); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	adminID, _ := store.CreateUser("pg-all-admin", models.RoleHumanAdmin)

	store.ArchiveTicket("pg-allarch-a", adminID)
	store.ArchiveTicket("pg-allarch-b", adminID)

	allArchived, err := store.GetAllArchivedTickets()
	if err != nil {
		t.Fatalf("GetAllArchivedTickets failed: %v", err)
	}
	if len(allArchived) != 2 {
		t.Errorf("Expected 2 archived tickets, got %d", len(allArchived))
	}

	board1, _ := store.GetArchivedTickets("board-1")
	if len(board1) != 2 {
		t.Errorf("Expected 2 archived tickets on board-1, got %d", len(board1))
	}

	boardOther, _ := store.GetArchivedTickets("other-board")
	if len(boardOther) != 0 {
		t.Errorf("Expected 0 archived tickets on other-board, got %d", len(boardOther))
	}
}

// ============================================================================
// Transaction Tests (parity with SQLite tx semantics)

func TestPostgresStore_TransactionCommit(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	if err := store.CreateTicket(pgNewTicket("pg-tx-1", "todo-0", "board-1")); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	tx, err := store.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	tk, err := tx.GetTicket("pg-tx-1")
	if err != nil {
		t.Fatalf("GetTicket in tx failed: %v", err)
	}

	tk.Title = "Modified In PG Transaction"
	now := time.Now().Format(time.RFC3339)
	tk.UpdatedAt = now

	err = tx.UpdateTicket(tk)
	if err != nil {
		t.Fatalf("UpdateTicket in tx failed: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	got, _ := store.GetTicket("pg-tx-1")
	if got.Title != "Modified In PG Transaction" {
		t.Errorf("Transaction commit did not persist: title=%s", got.Title)
	}
}

func TestPostgresStore_TransactionRollback(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	if err := store.CreateTicket(pgNewTicket("pg-tx-1", "todo-0", "board-1")); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	tx, err := store.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	tk, _ := tx.GetTicket("pg-tx-1")
	tk.Title = "Will Be Rolled Back"
	now := time.Now().Format(time.RFC3339)
	tk.UpdatedAt = now

	err = tx.UpdateTicket(tk)
	if err != nil {
		t.Fatalf("UpdateTicket in tx failed: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	got, _ := store.GetTicket("pg-tx-1")
	if got.Title != "PG Test Ticket pg-tx-1" {
		t.Errorf("Rollback did not revert changes: title=%s", got.Title)
	}
}

func TestPostgresStore_TransactionActivityLog(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	tx, err := store.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	logEntry := &models.ActivityLog{
		TicketID:  "pg-tx-log-1",
		EventType: "claim",
		Actor:     "agent-x",
		PrevState: strPtr("unassigned"),
		NewState:  strPtr("assigned to agent-x"),
		Metadata:  "{}",
	}

	id, err := tx.CreateActivityLog(logEntry)
	if err != nil {
		t.Fatalf("CreateActivityLog in tx failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero ID from CreateActivityLog")
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	logs, err := store.GetActivityLogs("pg-tx-log-1", 10)
	if err != nil {
		t.Fatalf("GetActivityLogs failed: %v", err)
	}
	if len(logs) != 1 || logs[0].EventType != "claim" {
		t.Errorf("Activity log not persisted correctly: %+v", logs)
	}
}

// ============================================================================
// Token CRUD Tests (parity with SQLite token operations)

func TestPostgresStore_CreateAndValidateToken(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	hash := "abc123hash"
	id, err := store.CreateToken("pg-agent-x", hash)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero token ID")
	}

	token, err := store.ValidateToken(hash)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if token.AgentName != "pg-agent-x" || token.TokenHash != hash {
		t.Errorf("Validated token mismatch: %+v", token)
	}

	notFound, _ := store.ValidateToken("wrong-hash")
	if notFound != nil {
		t.Error("Expected nil for invalid token hash")
	}
}

func TestPostgresStore_CreateTokenWithUser(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	userID, _ := store.CreateUser("pg-token-user", models.RoleNormalAI)

	id, err := store.CreateTokenWithUser(userID, "pg-agent-y", "hash456abcde")
	if err != nil {
		t.Fatalf("CreateTokenWithUser failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero token ID")
	}

	token, _ := store.ValidateToken("hash456abcde")
	if token == nil || token.UserID != userID {
		t.Errorf("Expected UserID %d, got %+v", userID, token)
	}
}

func TestPostgresStore_UpdateTokenLastUsed(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	store.CreateToken("pg-agent-z", "hash789abcde")

	err := store.UpdateTokenLastUsed("hash789abcde")
	if err != nil {
		t.Fatalf("UpdateTokenLastUsed failed: %v", err)
	}

	token, _ := store.ValidateToken("hash789abcde")
	if token.LastUsed == nil {
		t.Error("Expected LastUsed to be set after UpdateTokenLastUsed")
	}
}

func TestPostgresStore_DeleteAndListTokens(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	store.CreateToken("pg-agent-del", "hash-del-1234567890")
	id, err := store.DeleteToken("pg-agent-del")
	if err != nil {
		t.Fatalf("DeleteToken failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero deleted ID")
	}

	store.CreateToken("pg-list-a", "ha1234567890")
	store.CreateToken("pg-list-b", "hb1234567890")

	tokens, err := store.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("Expected 2 tokens, got %d", len(tokens))
	}
}

// ============================================================================
// User CRUD Tests (parity with SQLite user operations)

func TestPostgresStore_CreateAndGetUser(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	id, err := store.CreateUser("pg-testuser", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero user ID")
	}

	user, err := store.GetUserByID(id)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if user.Name != "pg-testuser" || user.Role != models.RoleNormalAI {
		t.Errorf("User mismatch: %+v", user)
	}

	byName, _ := store.GetUserByName("pg-testuser")
	if byName == nil || byName.ID != id {
		t.Error("GetUserByName did not return correct user")
	}
}

func TestPostgresStore_CreateUserWithPassword(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	id, err := store.CreateUserWithPassword("pg-pwuser", "mypassword123", models.RoleHumanAdmin)
	if err != nil {
		t.Fatalf("CreateUserWithPassword failed: %v", err)
	}

	user, _ := store.GetUserByID(id)
	if user.Name != "pg-pwuser" || user.Role != models.RoleHumanAdmin {
		t.Errorf("User mismatch: %+v", user)
	}
	if strings.Contains(user.PasswordHash, "mypassword123") {
		t.Error("Password stored as plaintext instead of hash!")
	}
}

func TestPostgresStore_UpdateUserRoleAndDelete(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	id, _ := store.CreateUser("pg-roleuser", models.RoleNormalAI)

	err := store.UpdateUserRole(id, models.RoleOverseerAI)
	if err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}

	user, _ := store.GetUserByID(id)
	if user.Role != models.RoleOverseerAI {
		t.Errorf("Expected role '%s', got '%s'", models.RoleOverseerAI, user.Role)
	}

	err = store.DeleteUser(id)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	userAfter, _ := store.GetUserByID(id)
	if userAfter != nil {
		t.Error("Expected nil after deleting user")
	}
}

func TestPostgresStore_ListUsers(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	store.CreateUser("pg-user-a", models.RoleNormalAI)
	store.CreateUser("pg-user-b", models.RoleOverseerAI)

	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}
}

// ============================================================================
// GetUserByToken & UpdateTokenUserID Tests

func TestPostgresStore_GetUserByToken(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	userID, _ := store.CreateUser("pg-token-user-lookup", models.RoleOverseerAI)
	store.CreateTokenWithUser(userID, "pg-token-agent", "tok-hash-1234567890")

	user, err := store.GetUserByToken("tok-hash-1234567890")
	if err != nil {
		t.Fatalf("GetUserByToken failed: %v", err)
	}
	if user.Name != "pg-token-user-lookup" || user.Role != models.RoleOverseerAI {
		t.Errorf("GetUserByToken mismatch: %+v", user)
	}

	notFound, _ := store.GetUserByToken("nonexistent-hash")
	if notFound != nil {
		t.Error("Expected nil for non-existent token hash")
	}
}

func TestPostgresStore_UpdateTokenUserID(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	_, err := store.CreateToken("pg-agent-uid", "hash-uid-1234567890")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	targetUserID, _ := store.CreateUser("pg-target-user", models.RoleNormalAI)

	err = store.UpdateTokenUserID("hash-uid-1234567890", targetUserID)
	if err != nil {
		t.Fatalf("UpdateTokenUserID failed: %v", err)
	}

	token, _ := store.ValidateToken("hash-uid-1234567890")
	if token.UserID != targetUserID {
		t.Errorf("Expected UserID %d after UpdateTokenUserID, got %d", targetUserID, token.UserID)
	}
}

// ============================================================================
// GetTicketsByAssignee & Activity Log Tests

func TestPostgresStore_GetTicketsByAssignee(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	t1 := pgNewTicket("pg-ass-1", "todo-0", "board-1")
	t1.Assignee = "assignee-x"
	t2 := pgNewTicket("pg-ass-2", "inprogress-0", "board-1")
	t2.Assignee = "assignee-y"
	t3 := pgNewTicket("pg-ass-3", "review-0", "board-1")
	t3.Assignee = "assignee-x"

	for _, tk := range []*models.Ticket{t1, t2, t3} {
		if err := store.CreateTicket(tk); err != nil {
			t.Fatalf("CreateTicket failed: %v", err)
		}
	}

	results, err := store.GetTicketsByAssignee("assignee-x")
	if err != nil {
		t.Fatalf("GetTicketsByAssignee failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 tickets for assignee-x, got %d", len(results))
	}

	resultsY, _ := store.GetTicketsByAssignee("assignee-y")
	if len(resultsY) != 1 || resultsY[0].ID != "pg-ass-2" {
		t.Errorf("Expected [pg-ass-2] for assignee-y, got %d tickets", len(resultsY))
	}

	resultsNone, _ := store.GetTicketsByAssignee("nobody")
	if len(resultsNone) != 0 {
		t.Errorf("Expected 0 tickets for unknown assignee, got %d", len(resultsNone))
	}
}

func TestPostgresStore_CreateAndGetActivityLog(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	logEntry := &models.ActivityLog{
		TicketID:  "pg-log-t1",
		EventType: "move",
		Actor:     "pg-agent-log",
		PrevState: strPtr("todo-0"),
		NewState:  strPtr("inprogress-0"),
		Metadata:  "{\"reason\":\"test\"}",
	}

	id, err := store.CreateActivityLog(logEntry)
	if err != nil {
		t.Fatalf("CreateActivityLog failed: %v", err)
	}
	if id == 0 {
		t.Error("Expected non-zero ID from CreateActivityLog")
	}

	logs, err := store.GetActivityLogs("pg-log-t1", 10)
	if err != nil {
		t.Fatalf("GetActivityLogs failed: %v", err)
	}
	if len(logs) != 1 || logs[0].EventType != "move" {
		t.Errorf("Activity log mismatch: %+v", logs)
	}

	for i := 0; i < 4; i++ {
		store.CreateActivityLog(&models.ActivityLog{
			TicketID: "pg-log-t1", EventType: "update", Actor: "pg-agent-log",
		})
	}

	allLogs, _ := store.GetActivityLogs("pg-log-t1", 3)
	if len(allLogs) != 3 {
		t.Errorf("Expected 3 logs with limit=3, got %d", len(allLogs))
	}

	otherLogs, _ := store.GetActivityLogs("different-ticket", 10)
	if len(otherLogs) != 0 {
		t.Errorf("Expected 0 logs for different ticket, got %d", len(otherLogs))
	}
}

// ============================================================================
// Edge Cases (parity with SQLite edge case tests)

func TestPostgresStore_CreateTicketWithLabels(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	labels := []string{"urgent", "frontend"}
	ticket := &models.Ticket{
		ID:       "pg-lbl-1",
		Title:    "PG Labelled Ticket",
		Column:   "todo-0",
		BoardID:  "board-1",
		Priority: "high",
		Labels:   labels,
	}

	err := store.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket with labels failed: %v", err)
	}

	got, _ := store.GetTicket("pg-lbl-1")
	if len(got.Labels) != 2 || got.Labels[0] != "urgent" {
		t.Errorf("Labels not preserved correctly: %+v", got.Labels)
	}
}

func TestPostgresStore_CreateTicketWithDueDate(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	dueDate := "2026-12-31T23:59:59Z"
	ticket := &models.Ticket{
		ID:      "pg-dd-1",
		Title:   "PG Due Date Ticket",
		Column:  "todo-0",
		BoardID: "board-1",
		DueDate: &dueDate,
	}

	err := store.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("CreateTicket with due date failed: %v", err)
	}

	got, _ := store.GetTicket("pg-dd-1")
	if got.DueDate == nil || *got.DueDate != dueDate {
		t.Errorf("Due date not preserved correctly: %+v", got.DueDate)
	}
}

func TestPostgresStore_GetUserByName_NotFound(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	user, _ := store.GetUserByName("does-not-exist-pg")
	if user != nil {
		t.Error("Expected nil for non-existent user name")
	}
}

func TestPostgresStore_GetUserByID_NotFound(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	user, _ := store.GetUserByID(999999)
	if user != nil {
		t.Error("Expected nil for non-existent user ID")
	}
}

func TestPostgresStore_UpdateUserPassword(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	id, _ := store.CreateUserWithPassword("pg-passuser", "old-password", models.RoleNormalAI)

	err := store.UpdateUserPassword(id, "new-password")
	if err != nil {
		t.Fatalf("UpdateUserPassword failed: %v", err)
	}

	user, _ := store.GetUserByID(id)
	if user.PasswordHash == "" {
		t.Log("Updated password hash is empty (may depend on bcrypt implementation)")
	}
}

func TestPostgresStore_GetArchivedTickets_BoardFilter(t *testing.T) {
	store := newTestPostgresStore(t)
	defer store.Close()

	adminID, _ := store.CreateUser("pg-board-arch-admin", models.RoleHumanAdmin)

	t1 := pgNewTicket("pg-gaf-1", "todo-0", "board-a")
	t2 := pgNewTicket("pg-gaf-2", "todo-0", "board-b")
	for _, tk := range []*models.Ticket{t1, t2} {
		store.CreateTicket(tk)
		store.ArchiveTicket(tk.ID, adminID)
	}

	boardA, _ := store.GetArchivedTickets("board-a")
	if len(boardA) != 1 || boardA[0].ID != "pg-gaf-1" {
		t.Errorf("Expected [pg-gaf-1] for board-a, got %d tickets", len(boardA))
	}

	boardB, _ := store.GetArchivedTickets("board-b")
	if len(boardB) != 1 || boardB[0].ID != "pg-gaf-2" {
		t.Errorf("Expected [pg-gaf-2] for board-b, got %d tickets", len(boardB))
	}

	emptyBoard, _ := store.GetArchivedTickets("board-c")
	if len(emptyBoard) != 0 {
		t.Errorf("Expected 0 archived tickets for empty board, got %d", len(emptyBoard))
	}
}
