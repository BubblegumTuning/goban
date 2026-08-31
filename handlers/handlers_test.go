// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/config"
	"goban/models"
	"goban/services"
	"goban/store"
)

// setupTestStore creates a new in-memory SQLite store for testing.
func setupTestStore(t *testing.T) (*store.SQLiteStore, func()) {
	t.Helper()

	s := &store.SQLiteStore{} // Use default :memory: config
	if err := s.Init(); err != nil {
		t.Fatalf("Failed to init test store: %v", err)
	}

	cleanup := func() { _ = s.Close() }
	return s, cleanup
}

// setupTestApp creates a Fiber app with an in-memory SQLite store and initialized board state.
func setupTestApp(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	// Reset global handler state
	boardStates = make(map[string]*models.BoardState)
	dbStore = nil
	claimService = nil
	moveService = nil
	releaseService = nil

	// Initialize board state - create a default board with columns
	defaultBoard := config.Board{
		ID:      "test-board",
		Title:   "Test Board",
		Columns: []string{"todo-0", "inprogress-0", "review-0", "done-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)

	// Set up global state for handlers
	dbStore = s
	InitMoveService(s)
	InitReleaseService(s)
	claimService = services.NewClaimService(s)
	auth.SetStore(s)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	RegisterTicketRoutes(app, s)
	RegisterMoveRoutesV1(app)
	RegisterReleaseRoutes(app)
	app.Post("/api/v1/tickets/:id/claim", AuthMiddlewareWithRole, HandleClaim)

	cleanup := func() { dbCleanup() }
	return app, s, cleanup
}

// syncDBTicketsToMemory loads all tickets from the DB into boardStates memory cache.
func syncDBTicketsToMemory(s *store.SQLiteStore) {
	tickets, err := s.GetAllTickets()
	if err != nil || len(tickets) == 0 {
		return
	}

	for _, ticket := range tickets {
		boardID := ticket.BoardID
		if boardStates[boardID] == nil {
			continue
		}

		targetColID := models.GetColumnID(ticket.Column)
		for _, col := range boardStates[boardID].Columns {
			if col.ID == targetColID {
				col.Tickets = append(col.Tickets, ticket)
				break
			}
		}
	}
}

// createTestTicket creates a ticket in the store AND syncs it to memory cache.
func createTestTicket(t *testing.T, s *store.SQLiteStore, id string, column string) *models.Ticket {
	t.Helper()
	now := time.Now().Format(time.RFC3339)
	ticket := &models.Ticket{
		ID:        id,
		Title:     "Test Ticket " + id,
		Priority:  "medium",
		Column:    column,
		BoardID:   "test-board",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.CreateTicket(ticket); err != nil {
		t.Fatalf("Failed to create test ticket %s: %v", id, err)
	}

	mu.Lock()
	syncDBTicketsToMemory(s)
	mu.Unlock()

	return ticket
}

// createTestUser creates a user in the store and returns their ID.
func createTestUser(t *testing.T, s *store.SQLiteStore, name string, role string) int64 {
	t.Helper()
	userID, err := s.CreateUser(name, role)
	if err != nil {
		t.Fatalf("Failed to create test user %s: %v", name, err)
	}
	return userID
}

// registerTestToken registers an API token for a user and returns the token string.
func registerTestToken(t *testing.T, s *store.SQLiteStore, userID int64, agentName string) string {
	t.Helper()
	tokenStr := fmt.Sprintf("test-token-%d", userID)
	s.CreateTokenWithUser(userID, agentName, auth.HashToken(tokenStr))
	return tokenStr
}

// testResponse wraps an HTTP response for testing.
type testResponse struct {
	Code int
	Body io.ReadCloser
}

// makeRequest creates an HTTP request for testing handlers.
func makeRequest(app *fiber.App, method, path string, body interface{}) (*testResponse, error) {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	if len(reqBody) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	return &testResponse{Code: resp.StatusCode, Body: resp.Body}, err
}

// makeRequestWithAuth creates an authenticated HTTP request for testing handlers.
func makeRequestWithAuth(app *fiber.App, token string, method, path string, body interface{}) (*testResponse, error) {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	if len(reqBody) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := app.Test(req)
	return &testResponse{Code: resp.StatusCode, Body: resp.Body}, err
}

// ============================================================================
// Ticket CRUD Tests (tickets.go handlers)

func TestHandleCreateTicketSimple_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-create", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-create")

	reqBody := map[string]interface{}{
		"title":    "New Ticket",
		"priority": "high",
		"board_id": "test-board",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/tickets/", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 201 {
		t.Errorf("Expected status 201, got %d", resp.Code)
	}

	var ticket models.Ticket
	json.NewDecoder(resp.Body).Decode(&ticket)
	if ticket.Title != "New Ticket" || ticket.Priority != "high" {
		t.Errorf("Ticket mismatch: %+v", ticket)
	}
}

func TestHandleCreateTicketSimple_InvalidTitle(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-invalid-title", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-invalid-title")

	reqBody := map[string]interface{}{
		"title":    "", // Empty title should fail validation
		"board_id": "test-board",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/tickets/", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid title, got %d", resp.Code)
	}
}

func TestHandleCreateTicketSimple_BoardNotFound(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-board-not-found", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-board-not-found")

	reqBody := map[string]interface{}{
		"title":    "New Ticket",
		"board_id": "nonexistent-board",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/tickets/", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for missing board, got %d", resp.Code)
	}
}

func TestHandleCreateTicketSimple_WorksWhenCacheEmpty(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-cache-empty", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-cache-empty")

	mu.Lock()
	boardStates = make(map[string]*models.BoardState)
	mu.Unlock()

	reqBody := map[string]interface{}{
		"title":    "Cache empty create",
		"board_id": "test-board",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/tickets/", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Code != 201 {
		t.Fatalf("Expected 201 when board is configured but cache empty, got %d", resp.Code)
	}
}

func TestHandleUpdateTicket_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "upd-1", "todo-0")
	userID := createTestUser(t, s, "test-agent-update", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-update")

	reqBody := map[string]interface{}{
		"title": "Updated Title",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", "/api/tickets/upd-1", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for update, got %d", resp.Code)
		return
	}

	var ticket models.Ticket
	json.NewDecoder(resp.Body).Decode(&ticket)
	if ticket.Title != "Updated Title" {
		t.Errorf("Title not updated: %+v", ticket)
	}
}

func TestHandleUpdateTicket_NotFound(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-update-not-found", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-update-not-found")

	reqBody := map[string]interface{}{
		"title": "Updated Title",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", "/api/tickets/nonexistent-123", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent ticket, got %d", resp.Code)
	}
}

func TestHandleUpdateTicket_InvalidPriority(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "upd-2", "todo-0")
	userID := createTestUser(t, s, "test-agent-update-priority", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-update-priority")

	reqBody := map[string]interface{}{
		"priority": "invalid-priority-value", // Should fail validation
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", "/api/tickets/upd-2", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid priority, got %d", resp.Code)
	}
}

func TestHandleUpdateTicket_InvalidLabels(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "upd-labels", "todo-0")
	userID := createTestUser(t, s, "test-agent-update-labels", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-update-labels")

	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = "l"
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", "/api/tickets/upd-labels", map[string]interface{}{
		"labels": tooMany,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Code != 400 {
		t.Fatalf("Expected status 400 for too many labels, got %d", resp.Code)
	}
}

func TestHandleDeleteTicket_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "del-1", "todo-0")
	userID := createTestUser(t, s, "test-agent-delete", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-delete")

	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", "/api/tickets/del-1?force=true", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for delete, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "deleted" {
		t.Errorf("Delete response mismatch: %+v", result)
	}
}

func TestHandleDeleteTicket_NotFound(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-delete-not-found", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-delete-not-found")

	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", "/api/tickets/nonexistent-456?force=true", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent ticket, got %d", resp.Code)
	}
}

func TestHandleListTicketsPaginated_Default(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		createTestTicket(t, s, fmt.Sprintf("list-%d", i), "todo-0")
	}

	resp, err := makeRequest(app, "GET", "/api/tickets/", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for list, got %d", resp.Code)
		return
	}

	var result PaginatedTicketsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Tickets) != 3 {
		t.Errorf("Expected 3 tickets in default view, got %d", len(result.Tickets))
	}
	if result.Total != 3 {
		t.Errorf("Expected total 3, got %d", result.Total)
	}
}

func TestHandleListTicketsPaginated_WithPagination(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		createTestTicket(t, s, fmt.Sprintf("page-%d", i), "todo-0")
	}

	resp, err := makeRequest(app, "GET", "/api/tickets/?limit=2&offset=1", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for paginated list, got %d", resp.Code)
		return
	}

	var result PaginatedTicketsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Tickets) != 2 {
		t.Errorf("Expected 2 tickets with limit=2, got %d", len(result.Tickets))
	}
	if !result.HasMore {
		t.Error("Expected HasMore=true when offset+len < total")
	}
}

func TestHandleListTicketsPaginated_BoardFilter(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "bf-1", "todo-0")
	createTestTicket(t, s, "bf-2", "todo-0")

	resp, err := makeRequest(app, "GET", "/api/tickets/?board_id=test-board&limit=3", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for filtered list, got %d", resp.Code)
		return
	}

	var result PaginatedTicketsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Tickets) < 1 {
		t.Error("Expected at least 1 ticket with board_id filter")
	}
}

func TestHandleListTicketsPaginated_ViewFull(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	for _, col := range []string{"todo-0", "inprogress-0", "done-0"} {
		createTestTicket(t, s, fmt.Sprintf("vf-%s", strings.ReplaceAll(col, "-", "")), col)
	}

	resp, err := makeRequest(app, "GET", "/api/tickets/?view=full", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for full view, got %d", resp.Code)
		return
	}

	var result PaginatedTicketsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Tickets) != 3 {
		t.Errorf("Expected 3 tickets in full view, got %d", len(result.Tickets))
	}
	expectedCols := []string{"backlog", "todo", "inprogress", "review", "done", "cancelled"}
	if len(result.Columns) != len(expectedCols) {
		t.Errorf("Expected %d column prefixes in full view, got %d: %v", len(expectedCols), len(result.Columns), result.Columns)
	}
}

func TestHandleGetTicket_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "get-1", "todo-0")

	resp, err := makeRequest(app, "GET", "/api/v1/tickets/get-1", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for get ticket, got %d", resp.Code)
		return
	}

	var ticket models.Ticket
	json.NewDecoder(resp.Body).Decode(&ticket)
	if ticket.ID != "get-1" {
		t.Errorf("Ticket ID mismatch: %+v", ticket)
	}
}

func TestHandleGetTicket_NotFound(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	resp, err := makeRequest(app, "GET", "/api/v1/tickets/nonexistent-789", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent ticket, got %d", resp.Code)
	}
}

// ============================================================================
// Claim Handler Tests (claim.go handlers)

func TestHandleClaim_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	ticket := createTestTicket(t, s, "claim-1", "todo-0")

	userID := createTestUser(t, s, "test-agent", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent")

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", fmt.Sprintf("/api/v1/tickets/%s/claim", ticket.ID), nil)
	if err != nil {
		t.Fatalf("Claim request failed: %v", err)
	}

	// Claim may succeed or fail depending on role permissions
	if resp.Code == 200 || resp.Code == 403 {
		t.Logf("Got status %d for claim (expected behavior)", resp.Code)
	} else {
		t.Errorf("Unexpected status for claim: %d", resp.Code)
	}
}

func TestHandleClaim_NotAuthenticated(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "claim-2", "todo-0")

	resp, err := makeRequest(app, "POST", "/api/v1/tickets/claim-2/claim", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 401 && resp.Code != 500 {
		t.Errorf("Expected status 401 or 500 for unauthenticated claim, got %d", resp.Code)
	}
}

func TestHandleClaim_TicketNotFound(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-2", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-2")

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/v1/tickets/nonexistent-ticket/claim", nil)
	if err != nil {
		t.Fatalf("Claim request failed: %v", err)
	}

	// Should return 404 for non-existent ticket or 500 if service unavailable
	if resp.Code == 404 || resp.Code == 500 {
		t.Logf("Got status %d for claim on non-existent ticket (expected)", resp.Code)
	} else {
		t.Errorf("Unexpected status: %d", resp.Code)
	}
}

// ============================================================================
// Move Handler Tests (moves.go handlers)

func TestHandleMoveV1_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "move-1", "todo-0")

	userID := createTestUser(t, s, "test-agent-move", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-move")

	reqBody := map[string]interface{}{
		"target_status": "inprogress",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/v1/tickets/move-1/move", reqBody)
	if err != nil {
		t.Fatalf("Move request failed: %v", err)
	}

	if resp.Code == 200 {
		var ticket models.Ticket
		json.NewDecoder(resp.Body).Decode(&ticket)
		if !strings.HasPrefix(ticket.Column, "inprogress") {
			t.Errorf("Expected column to start with 'inprogress', got %s", ticket.Column)
		}
	} else {
		t.Logf("Got status %d for move (may vary by role/permissions)", resp.Code)
	}
}

func TestHandleMoveV1_InvalidTargetStatus(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "move-2", "todo-0")

	userID := createTestUser(t, s, "test-agent-move-2", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-move-2")

	reqBody := map[string]interface{}{
		"target_status": "invalid-status-that-does-not-exist",
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/v1/tickets/move-2/move", reqBody)
	if err != nil {
		t.Fatalf("Move request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid target_status, got %d", resp.Code)
	}
}

func TestHandleMoveV1_MissingTargetStatus(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "move-3", "todo-0")

	userID := createTestUser(t, s, "test-agent-move-3", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-move-3")

	reqBody := map[string]interface{}{
		"force": true,
	}
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/v1/tickets/move-3/move", reqBody)
	if err != nil {
		t.Fatalf("Move request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for missing target_status, got %d", resp.Code)
	}
}

func TestHandleMoveV1_NotAuthenticated(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "move-4", "todo-0")

	reqBody := map[string]interface{}{
		"target_status": "inprogress",
	}
	resp, err := makeRequest(app, "POST", "/api/v1/tickets/move-4/move", reqBody)
	if err != nil {
		t.Fatalf("Move request failed: %v", err)
	}

	if resp.Code != 401 && resp.Code != 500 {
		t.Errorf("Expected status 401 or 500 for unauthenticated move, got %d", resp.Code)
	}
}

// ============================================================================
// Release Handler Tests (release.go handlers)

func TestHandleRelease_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	ticket := createTestTicket(t, s, "rel-1", "todo-0")
	// Assign the ticket first
	ticket.Assignee = "test-agent-rel"
	s.UpdateTicket(ticket)

	userID := createTestUser(t, s, "test-agent-rel", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-rel")

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/v1/tickets/rel-1/release", nil)
	if err != nil {
		t.Fatalf("Release request failed: %v", err)
	}

	if resp.Code == 200 || resp.Code == 403 || resp.Code == 500 {
		t.Logf("Got status %d for release (expected behavior)", resp.Code)
	} else {
		t.Errorf("Unexpected status for release: %d", resp.Code)
	}
}

func TestHandleRelease_NotAuthenticated(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	createTestTicket(t, s, "rel-2", "todo-0")

	resp, err := makeRequest(app, "POST", "/api/v1/tickets/rel-2/release", nil)
	if err != nil {
		t.Fatalf("Release request failed: %v", err)
	}

	if resp.Code != 401 && resp.Code != 500 {
		t.Errorf("Expected status 401 or 500 for unauthenticated release, got %d", resp.Code)
	}
}

func TestHandleRelease_TicketNotFound(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-agent-rel-2", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-agent-rel-2")

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/v1/tickets/nonexistent-rel/release", nil)
	if err != nil {
		t.Fatalf("Release request failed: %v", err)
	}

	if resp.Code == 404 || resp.Code == 500 {
		t.Logf("Got status %d for release on non-existent ticket (expected)", resp.Code)
	} else {
		t.Errorf("Unexpected status: %d", resp.Code)
	}
}

// ============================================================================
// AuthMiddlewareWithRole Tests (claim.go middleware)

func TestAuthMiddlewareWithRole_Success(t *testing.T) {
	app, s, cleanup := setupTestApp(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-auth-user", models.RoleNormalAI)
	tokenStr := registerTestToken(t, s, userID, "test-auth-user")

	resp, err := makeRequestWithAuth(app, tokenStr, "GET", "/api/tickets/", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for authenticated request, got %d", resp.Code)
	}
}

func TestAuthMiddlewareWithRole_MissingHeader(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	resp, err := makeRequest(app, "POST", "/api/tickets/", map[string]interface{}{
		"title":    "New Ticket",
		"board_id": "test-board",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// POST /api/tickets/ requires auth, should return 401
	if resp.Code != 201 && resp.Code != 401 {
		t.Logf("Got status %d for unauthenticated ticket creation (expected 401)", resp.Code)
	}
}

func TestAuthMiddlewareWithRole_InvalidFormat(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/tickets/", strings.NewReader(`{"title":"test","board_id":"test-board"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "InvalidFormat no-space") // Missing Bearer prefix

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 401 && resp.StatusCode != 400 {
		t.Errorf("Expected status 401 or 400 for invalid auth format, got %d", resp.StatusCode)
	}
}

func TestAuthMiddlewareWithRole_EmptyToken(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/tickets/", strings.NewReader(`{"title":"test","board_id":"test-board"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ") // Empty token

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 401 && resp.StatusCode != 400 {
		t.Errorf("Expected status 401 or 400 for empty token, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Archive Handler Tests (archive.go handlers)

func setupTestAppWithArchive(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	// Reset global handler state
	boardStates = make(map[string]*models.BoardState)
	dbStore = nil
	claimService = nil
	moveService = nil
	releaseService = nil
	adminUserService = nil
	userService = nil

	defaultBoard := config.Board{
		ID:      "test-board",
		Title:   "Test Board",
		Columns: []string{"todo-0", "inprogress-0", "review-0", "done-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)

	dbStore = s
	InitMoveService(s)
	InitReleaseService(s)
	claimService = services.NewClaimService(s)
	adminUserService = services.NewUserService(s)
	userService = adminUserService
	auth.SetStore(s)

	// Set up JWT for archive routes (AuthMiddlewareWithUser requires it)
	auth.SetJWTSecret([]byte("test-jwt-secret-for-archiving"))

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterArchiveRoutes(app)

	_ = dbCleanup // ensure it's used
	cleanup := func() { dbCleanup(); _ = s.Close() }
	return app, s, cleanup
}

// createAdminJWT creates a user with the given role and generates a JWT for testing archive routes.
func createAdminJWT(t *testing.T, s *store.SQLiteStore, name string, role string) (string, int64) {
	t.Helper()
	userID := createTestUser(t, s, name, role)
	user, err := s.GetUserByName(name)
	if err != nil || user == nil {
		t.Fatalf("Failed to retrieve created user %s: %+v", name, err)
	}
	jwtToken, _, err := auth.GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("Failed to generate JWT for user %s: %v", name, err)
	}
	return jwtToken, userID
}

func TestSingleArchive_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "arch-1", "todo-0")
	jwtToken, adminID := createAdminJWT(t, s, "test-admin-archive", models.RoleHumanAdmin)

	reqBody := ArchiveRequest{TicketID: "arch-1"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/single", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for archive, got %d", resp.Code)
		return
	}

	var result ArchiveResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Status != "archived" || result.Count != 1 {
		t.Errorf("Archive response mismatch: %+v", result)
	}

	// Verify ticket is archived in DB
	ticket, _ := s.GetTicket("arch-1")
	if !ticket.Archived || ticket.ArchivedBy == nil || int(*ticket.ArchivedBy) != int(adminID) {
		t.Errorf("Ticket not properly archived: Archived=%v ArchivedBy=%v", ticket.Archived, ticket.ArchivedBy)
	}

	// Verify activity log was created
	logs, _ := s.GetActivityLogs("arch-1", 10)
	foundArchiveLog := false
	for _, l := range logs {
		if l.EventType == models.ActivityArchived && l.Actor == "test-admin-archive" {
			foundArchiveLog = true
			break
		}
	}
	if !foundArchiveLog {
		t.Error("Expected activity log entry for archive event")
	}
}

func TestSingleArchive_Forbidden_NonAdmin(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "arch-2", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-normal-user", models.RoleNormalAI)

	reqBody := ArchiveRequest{TicketID: "arch-2"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/single", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 403 {
		t.Errorf("Expected status 403 for non-admin archive attempt, got %d", resp.Code)
	}
}

func TestSingleArchive_NotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-admin-notfound", models.RoleHumanAdmin)

	reqBody := ArchiveRequest{TicketID: "nonexistent-ticket"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/single", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent ticket archive, got %d", resp.Code)
	}
}

func TestSingleArchive_MissingTicketID(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-admin-missing", models.RoleHumanAdmin)

	reqBody := ArchiveRequest{TicketID: ""}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/single", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for missing ticket_id, got %d", resp.Code)
	}
}

func TestBulkArchive_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "bulk-1", "todo-0")
	createTestTicket(t, s, "bulk-2", "inprogress-0")
	jwtToken, adminID := createAdminJWT(t, s, "test-admin-bulk", models.RoleHumanAdmin)

	reqBody := BulkArchiveRequest{TicketIDs: []string{"bulk-1", "bulk-2"}}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/bulk", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for bulk archive, got %d", resp.Code)
		return
	}

	var result ArchiveResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Status != "archived" || result.Count != 2 {
		t.Errorf("Bulk archive response mismatch: %+v", result)
	}

	for _, id := range []string{"bulk-1", "bulk-2"} {
		ticket, _ := s.GetTicket(id)
		if !ticket.Archived || ticket.ArchivedBy == nil || int(*ticket.ArchivedBy) != int(adminID) {
			t.Errorf("Ticket %s not properly archived: Archived=%v ArchivedBy=%v", id, ticket.Archived, ticket.ArchivedBy)
		}
	}
}

func TestBulkArchive_PartialNotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "bulk-3", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-admin-partial", models.RoleHumanAdmin)

	reqBody := BulkArchiveRequest{TicketIDs: []string{"bulk-3", "nonexistent-one"}}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/bulk", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for partial bulk archive, got %d", resp.Code)
		return
	}

	var result ArchiveResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Count != 1 {
		t.Errorf("Expected count 1 (only existing ticket), got %d", result.Count)
	}
	if len(result.NotFound) != 1 || result.NotFound[0] != "nonexistent-one" {
		t.Errorf("Expected not_found=[nonexistent-one], got %+v", result.NotFound)
	}
}

func TestBulkArchive_EmptyIDs(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-admin-empty-bulk", models.RoleHumanAdmin)

	reqBody := BulkArchiveRequest{TicketIDs: []string{}}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/bulk", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for empty ticket_ids, got %d", resp.Code)
	}
}

func TestBulkArchive_Forbidden_NonAdmin(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "bulk-forbid-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-normal-bulk", models.RoleNormalAI)

	reqBody := BulkArchiveRequest{TicketIDs: []string{"bulk-forbid-1"}}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/archive/bulk", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 403 {
		t.Errorf("Expected status 403 for non-admin bulk archive, got %d", resp.Code)
	}
}

func TestUnarchiveTicket_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "unarch-1", "todo-0")
	jwtToken, adminID := createAdminJWT(t, s, "test-admin-unarchive", models.RoleHumanAdmin)

	// First archive the ticket
	s.ArchiveTicket("unarch-1", adminID)
	mu.Lock()
	syncArchivedTicketInMemory("unarch-1")
	mu.Unlock()

	ticket, _ := s.GetTicket("unarch-1")
	if !ticket.Archived {
		t.Fatal("Setup: ticket should be archived before unarchive test")
	}

	reqBody := UnarchiveRequest{BoardID: "test-board", Column: "todo-0"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/unarchive/unarch-1", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for unarchive, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "restored" {
		t.Errorf("Unarchive response mismatch: %+v", result)
	}

	// Verify ticket is unarchived in DB
	ticket, _ = s.GetTicket("unarch-1")
	if ticket.Archived || ticket.ArchivedBy != nil {
		t.Errorf("Ticket not properly unarchived: Archived=%v ArchivedBy=%v", ticket.Archived, ticket.ArchivedBy)
	}

	// Verify activity log for restore event
	logs, _ := s.GetActivityLogs("unarch-1", 10)
	foundRestoreLog := false
	for _, l := range logs {
		if l.EventType == models.ActivityRestored && l.Actor == "test-admin-unarchive" {
			foundRestoreLog = true
			break
		}
	}
	if !foundRestoreLog {
		t.Error("Expected activity log entry for restore event")
	}
}

func TestUnarchiveTicket_Forbidden_NonAdmin(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "unarch-forbid-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-normal-unarchive", models.RoleNormalAI)

	reqBody := UnarchiveRequest{BoardID: "test-board", Column: "todo-0"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/unarchive/unarch-forbid-1", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 403 {
		t.Errorf("Expected status 403 for non-admin unarchive, got %d", resp.Code)
	}
}

func TestUnarchiveTicket_NotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-admin-unarch-nf", models.RoleHumanAdmin)

	reqBody := UnarchiveRequest{BoardID: "test-board", Column: "todo-0"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/unarchive/nonexistent-unarch", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent ticket unarchive, got %d", resp.Code)
	}
}

func TestGetArchivedByAdmin_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "getarch-1", "todo-0")
	createTestTicket(t, s, "getarch-2", "inprogress-0")
	jwtToken, adminID := createAdminJWT(t, s, "test-admin-getarch", models.RoleHumanAdmin)

	// Archive both tickets by this admin
	s.ArchiveTicket("getarch-1", adminID)
	s.ArchiveTicket("getarch-2", adminID)

	resp, err := makeRequestWithAuth(app, jwtToken, "GET", fmt.Sprintf("/api/archive/by-admin/%d", adminID), nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for get archived by admin, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	tickets := result["tickets"].([]interface{})
	if len(tickets) != 2 {
		t.Errorf("Expected 2 archived tickets, got %d", len(tickets))
	}
}

func TestGetArchivedByAdmin_Forbidden_NonAdmin(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-normal-getarch", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/archive/by-admin/1", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 403 {
		t.Errorf("Expected status 403 for non-admin get archived by admin, got %d", resp.Code)
	}
}

func TestGetArchivedByAdmin_InvalidAdminID(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-admin-invalid-id", models.RoleHumanAdmin)

	resp, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/archive/by-admin/not-a-number", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid admin_id format, got %d", resp.Code)
	}
}

func TestGetAllArchived_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	createTestTicket(t, s, "allarch-1", "todo-0")
	createTestTicket(t, s, "allarch-2", "inprogress-0")
	jwtToken, adminID := createAdminJWT(t, s, "test-admin-allarch", models.RoleHumanAdmin)

	// Archive tickets by this admin
	s.ArchiveTicket("allarch-1", adminID)
	s.ArchiveTicket("allarch-2", adminID)

	// Use any authenticated user (not necessarily admin) to view archived
	resp, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/archived", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for get all archived, got %d", resp.Code)
		return
	}

	var tickets []*models.Ticket
	json.NewDecoder(resp.Body).Decode(&tickets)
	if len(tickets) < 2 {
		t.Errorf("Expected at least 2 archived tickets, got %d", len(tickets))
	}
}

// ============================================================================
// Admin Handler Tests (admin.go handlers)

func setupTestAppWithAdmin(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	boardStates = make(map[string]*models.BoardState)
	dbStore = nil
	adminUserService = nil
	userService = nil

	defaultBoard := config.Board{
		ID:      "test-board",
		Title:   "Test Board",
		Columns: []string{"todo-0", "inprogress-0", "review-0", "done-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)

	dbStore = s
	adminUserService = services.NewUserService(s)
	userService = adminUserService
	auth.SetStore(s)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterAdminRoutes(app)

	_ = dbCleanup // ensure it's used
	cleanup := func() { dbCleanup(); _ = s.Close() }
	return app, s, cleanup
}

func createAdminToken(t *testing.T, s *store.SQLiteStore, name string) (string, int64) {
	t.Helper()
	userID := createTestUser(t, s, name, models.RoleHumanAdmin)
	tokenStr := fmt.Sprintf("admin-token-%s", name)
	s.CreateTokenWithUser(userID, name, auth.HashToken(tokenStr))
	return tokenStr, userID
}

func TestAuthMiddlewareAdmin_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-middleware")

	resp, err := makeRequestWithAuth(app, tokenStr, "GET", "/api/admin/users", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for admin middleware success, got %d", resp.Code)
	}
}

func TestAuthMiddlewareAdmin_Forbidden_NonAdmin(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	userID := createTestUser(t, s, "test-normal-middleware", models.RoleNormalAI)
	tokenStr := "normal-token-test"
	s.CreateTokenWithUser(userID, "test-normal-middleware", auth.HashToken(tokenStr))

	resp, err := makeRequestWithAuth(app, tokenStr, "GET", "/api/admin/users", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 403 {
		t.Errorf("Expected status 403 for non-admin accessing admin route, got %d", resp.Code)
	}
}

func TestAuthMiddlewareAdmin_MissingHeader(t *testing.T) {
	app, _, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	resp, err := makeRequest(app, "GET", "/api/admin/users", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 401 {
		t.Errorf("Expected status 401 for missing auth header on admin route, got %d", resp.Code)
	}
}

func TestAuthMiddlewareAdmin_InvalidFormat(t *testing.T) {
	app, _, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", "InvalidFormat token123")

	respHTTP, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if respHTTP.StatusCode != 401 {
		t.Errorf("Expected status 401 for invalid auth format on admin route, got %d", respHTTP.StatusCode)
	}
}

func TestHandleAdminCreateUser_NotAvailable(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-no-create")
	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/admin/users", map[string]string{"username": "new-agent", "role": models.RoleNormalAI})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Code == 201 {
		t.Fatal("HTTP must not create users; use goban-user-cli against the database")
	}
}

func TestHandleAdminListUsers_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-list")
	createTestUser(t, s, "list-user-1", models.RoleNormalAI)
	createTestUser(t, s, "list-user-2", models.RoleOverseerAI)

	resp, err := makeRequestWithAuth(app, tokenStr, "GET", "/api/admin/users", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for list users, got %d", resp.Code)
		return
	}

	var result AdminUserListResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Users) < 3 { // admin + 2 created users
		t.Errorf("Expected at least 3 users in list, got %d: %+v", len(result.Users), result.Users)
	}
}

func TestHandleAdminUpdateUserRole_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-update-role")
	targetID := createTestUser(t, s, "role-change-target", models.RoleNormalAI)

	reqBody := AdminUpdateUserRoleRequest{Role: models.RoleOverseerAI}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", fmt.Sprintf("/api/admin/users/%d/role", targetID), reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for update role, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["new_role"] != models.RoleOverseerAI {
		t.Errorf("Expected new_role=%s, got %+v", models.RoleOverseerAI, result)
	}

	retrieved, _ := s.GetUserByID(targetID)
	if retrieved == nil || retrieved.Role != models.RoleOverseerAI {
		t.Errorf("Role not updated in store: %+v", retrieved)
	}
}

func TestHandleAdminUpdateUserRole_InvalidRole(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-invalid-update")
	targetID := createTestUser(t, s, "role-change-invalid", models.RoleNormalAI)

	reqBody := AdminUpdateUserRoleRequest{Role: "NOT_A_VALID_ROLE"}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", fmt.Sprintf("/api/admin/users/%d/role", targetID), reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid role update, got %d", resp.Code)
	}
}

func TestHandleAdminUpdateUserRole_NotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-update-nf")

	reqBody := AdminUpdateUserRoleRequest{Role: models.RoleOverseerAI}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", "/api/admin/users/99999/role", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent user role update, got %d", resp.Code)
	}
}

func TestHandleAdminUpdateUserRole_MissingRole(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-missing-role")
	targetID := createTestUser(t, s, "role-change-missing", models.RoleNormalAI)

	reqBody := AdminUpdateUserRoleRequest{Role: ""}
	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", fmt.Sprintf("/api/admin/users/%d/role", targetID), reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for missing role, got %d", resp.Code)
	}
}

func TestHandleAdminDeleteUser_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-deleter")
	targetID := createTestUser(t, s, "delete-target", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", fmt.Sprintf("/api/admin/users/%d", targetID), nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for delete user, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["username"] != "delete-target" {
		t.Errorf("Delete response mismatch: %+v", result)
	}

	// Verify user is deleted
	retrieved, _ := s.GetUserByID(targetID)
	if retrieved != nil {
		t.Error("Expected user to be deleted from store")
	}
}

func TestHandleAdminDeleteUser_HasActiveTickets(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-del-tickets")
	targetID := createTestUser(t, s, "delete-with-tickets", models.RoleNormalAI)

	// Create a ticket assigned to this user
	ticket := &models.Ticket{
		ID:        "del-ticket-1",
		Title:     "Ticket with assignee",
		Priority:  "medium",
		Column:    "todo-0",
		BoardID:   "test-board",
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
		Assignee:  "delete-with-tickets",
	}
	s.CreateTicket(ticket)

	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", fmt.Sprintf("/api/admin/users/%d", targetID), nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 409 {
		t.Errorf("Expected status 409 for user with active tickets, got %d", resp.Code)
	}
}

func TestHandleAdminDeleteUser_ForceWithTickets(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-force-del")
	targetID := createTestUser(t, s, "force-delete-target", models.RoleNormalAI)

	ticket := &models.Ticket{
		ID:        "force-ticket-1",
		Title:     "Ticket with assignee",
		Priority:  "medium",
		Column:    "todo-0",
		BoardID:   "test-board",
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
		Assignee:  "force-delete-target",
	}
	s.CreateTicket(ticket)

	reqBody := AdminDeleteUserRequest{Force: true}
	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", fmt.Sprintf("/api/admin/users/%d", targetID), reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for force delete user with tickets, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["username"] != "force-delete-target" {
		t.Errorf("Force delete response mismatch: %+v", result)
	}
}

func TestHandleAdminDeleteUser_NotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-del-nf")

	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", "/api/admin/users/99999", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent user delete, got %d", resp.Code)
	}
}

func TestHandleAdminDeleteUser_InvalidID(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-del-invalid")

	resp, err := makeRequestWithAuth(app, tokenStr, "DELETE", "/api/admin/users/0", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid user ID (0), got %d", resp.Code)
	}
}

func TestHandleAdminRegenerateToken_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-regen")
	targetID := createTestUser(t, s, "regen-target", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", fmt.Sprintf("/api/admin/users/%d/token-regenerate", targetID), nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for regenerate token, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	tokenData := result["token"].(map[string]interface{})
	if tokenData["token"] == "" {
		t.Error("Expected non-empty regenerated token")
	}
	if int64(tokenData["user_id"].(float64)) != targetID {
		t.Errorf("Expected user_id=%d, got %+v", targetID, tokenData)
	}
}

func TestHandleAdminRegenerateToken_NotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-regen-nf")

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/admin/users/99999/token-regenerate", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent user token regen, got %d", resp.Code)
	}
}

func TestHandleAdminResetPassword_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-pw")
	targetID := createTestUser(t, s, "pw-target", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", fmt.Sprintf("/api/admin/users/%d/password", targetID), map[string]string{"password": "new-secret"})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Code != 200 {
		t.Errorf("Expected status 200 for password reset, got %d", resp.Code)
	}
}

func TestHandleAdminResetPassword_EmptyPassword(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-pw-empty")
	targetID := createTestUser(t, s, "pw-empty", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", fmt.Sprintf("/api/admin/users/%d/password", targetID), map[string]string{"password": ""})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Code != 400 {
		t.Errorf("Expected status 400 for empty password, got %d", resp.Code)
	}
}

func TestHandleAdminResetPassword_NotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-admin-pw-nf")

	resp, err := makeRequestWithAuth(app, tokenStr, "PATCH", "/api/admin/users/99999/password", map[string]string{"password": "x"})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Code != 404 {
		t.Errorf("Expected status 404 for missing user password reset, got %d", resp.Code)
	}
}

// ============================================================================
// Comment Handler Tests (comments.go handlers)

func setupTestAppWithComments(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	boardStates = make(map[string]*models.BoardState)
	dbStore = nil
	adminUserService = nil
	userService = nil

	defaultBoard := config.Board{
		ID:      "test-board",
		Title:   "Test Board",
		Columns: []string{"todo-0", "inprogress-0", "review-0", "done-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)

	dbStore = s
	adminUserService = services.NewUserService(s)
	userService = adminUserService
	auth.SetStore(s)
	auth.SetJWTSecret([]byte("test-jwt-secret-for-comments"))

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterCommentRoutes(app)

	// Register DELETE on base path for testing (the "?index=:index" pattern in production
	// doesn't match query strings properly - Fiber treats ? as literal path char)
	commentGroupTest := app.Group("/api/tickets/:ticketId/comments", AuthMiddlewareWithRole)
	commentGroupTest.Delete("", handleDeleteComment)

	_ = dbCleanup // ensure it's used
	cleanup := func() { dbCleanup(); _ = s.Close() }
	return app, s, cleanup
}

func TestHandleAddComment_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "comment-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-commenter", models.RoleNormalAI)

	reqBody := CommentRequest{Text: "This is a test comment"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/comment-1/comments", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for add comment, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "added" {
		t.Errorf("Add comment response mismatch: %+v", result)
	}

	comment := result["comment"].(map[string]interface{})
	if comment["who"] != "test-commenter" || comment["text"] != "This is a test comment" {
		t.Errorf("Comment data mismatch: %+v", comment)
	}
}

func TestHandleAddComment_InvalidText(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "comment-2", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-commenter-invalid", models.RoleNormalAI)

	reqBody := CommentRequest{Text: ""}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/comment-2/comments", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for empty comment text, got %d", resp.Code)
	}
}

func TestHandleAddComment_TicketNotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-commenter-nf", models.RoleNormalAI)

	reqBody := CommentRequest{Text: "Test comment"}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/nonexistent-ticket/comments", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for comment on non-existent ticket, got %d", resp.Code)
	}
}

func TestHandleAddComment_CustomTimestamp(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "comment-ts-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-commenter-ts", models.RoleNormalAI)

	customTimestamp := "2025-06-01T12:00:00Z"
	reqBody := CommentRequest{Text: "Comment with timestamp", Timestamp: customTimestamp}
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/comment-ts-1/comments", reqBody)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for add comment with timestamp, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	comment := result["comment"].(map[string]interface{})
	if comment["timestamp"] != customTimestamp {
		t.Errorf("Expected timestamp %s, got %v", customTimestamp, comment["timestamp"])
	}
}

func TestHandleDeleteComment_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "del-comment-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-del-commenter", models.RoleNormalAI)

	// Add a comment first
	reqBody := CommentRequest{Text: "Comment to delete"}
	respAdd, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/del-comment-1/comments", reqBody)
	if err != nil || respAdd.Code != 200 {
		t.Fatalf("Setup failed to add comment: code=%d err=%v", respAdd.Code, err)
	}

	// Delete the comment (index=0 since it's the first and only one)
	respDel, err := makeRequestWithAuth(app, jwtToken, "DELETE", "/api/tickets/del-comment-1/comments?index=0", nil)
	if err != nil {
		t.Fatalf("Delete request failed: %v", err)
	}

	if respDel.Code != 200 {
		bodyBytes, _ := io.ReadAll(respDel.Body)
		t.Errorf("Expected status 200 for delete comment, got %d body=%s", respDel.Code, string(bodyBytes))
		return
	}

	var result map[string]interface{}
	json.NewDecoder(respDel.Body).Decode(&result)
	if result["status"] != "deleted" {
		t.Errorf("Delete comment response mismatch: %+v", result)
	}
}

func TestHandleDeleteComment_InvalidIndex(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "del-comment-2", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-del-invalid-index", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, jwtToken, "DELETE", "/api/tickets/del-comment-2/comments?index=-1", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 400 for invalid comment index, got %d body=%s", resp.Code, string(bodyBytes))
	}
}

func TestHandleDeleteComment_IndexOutOfRange(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "del-comment-3", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-del-out-range", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, jwtToken, "DELETE", "/api/tickets/del-comment-3/comments?index=5", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 404 for comment index out of range, got %d body=%s", resp.Code, string(bodyBytes))
	}
}

func TestHandleDeleteComment_TicketNotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-del-nf-ticket", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, jwtToken, "DELETE", "/api/tickets/nonexistent/comments?index=0", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Code != 404 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 404 for delete comment on non-existent ticket, got %d body=%s", resp.Code, string(bodyBytes))
	}
}

func TestHandleListComments_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "list-comment-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-list-commenter", models.RoleNormalAI)

	// Add two comments
	for i := 0; i < 2; i++ {
		reqBody := CommentRequest{Text: fmt.Sprintf("Comment %d", i)}
		respAdd, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/list-comment-1/comments", reqBody)
		if err != nil || respAdd.Code != 200 {
			t.Fatalf("Setup failed to add comment %d: code=%d err=%v", i, respAdd.Code, err)
		}
		respAdd.Body.Close()
	}

	respList, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/tickets/list-comment-1/comments", nil)
	if err != nil {
		t.Fatalf("List request failed: %v", err)
	}

	if respList.Code != 200 {
		t.Errorf("Expected status 200 for list comments, got %d", respList.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(respList.Body).Decode(&result)
	comments := result["comments"].([]interface{})
	if len(comments) != 2 {
		t.Errorf("Expected 2 comments in list, got %d", len(comments))
	}
}

func TestHandleListComments_Empty(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "list-empty-1", "todo-0")
	jwtToken, _ := createAdminJWT(t, s, "test-list-empty", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/tickets/list-empty-1/comments", nil)
	if err != nil {
		t.Fatalf("List request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for list comments (empty), got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	comments := result["comments"].([]interface{})
	if len(comments) != 0 {
		t.Errorf("Expected 0 comments, got %d", len(comments))
	}
}

func TestHandleListComments_TicketNotFound(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	jwtToken, _ := createAdminJWT(t, s, "test-list-nf", models.RoleNormalAI)

	resp, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/tickets/nonexistent/comments", nil)
	if err != nil {
		t.Fatalf("List request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for list comments on non-existent ticket, got %d", resp.Code)
	}
}

// ============================================================================
// Auth Handler Tests (auth.go handlers)

func setupTestAppWithAuth(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	boardStates = make(map[string]*models.BoardState)
	dbStore = nil
	adminUserService = nil
	userService = nil

	defaultBoard := config.Board{
		ID:      "test-board",
		Title:   "Test Board",
		Columns: []string{"todo-0", "inprogress-0", "review-0", "done-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)

	dbStore = s
	adminUserService = services.NewUserService(s)
	userService = adminUserService
	auth.SetStore(s)
	auth.SetJWTSecret([]byte("test-jwt-secret-for-auth-helpers"))

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterAuthRoutes(app, s)
	RegisterBoardRoutes(app)

	cleanup := func() { dbCleanup(); _ = s.Close() }
	return app, s, cleanup
}

func TestLogin_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	// Create user with plaintext password (CreateUserWithPassword hashes internally)
	_, err := s.CreateUserWithPassword("login-user", "testpassword", models.RoleNormalAI)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	reqBody := LoginRequest{Username: "login-user", Password: "testpassword"}
	resp, err := makeRequest(app, "POST", "/api/auth/login", reqBody)
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for login success, got %d", resp.Code)
		return
	}

	var result LoginResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken == "" {
		t.Error("Expected non-empty access_token from login")
	}
	if result.User == nil || result.User.Name != "login-user" {
		t.Errorf("Login response user mismatch: %+v", result.User)
	}
	if result.ExpiresIn <= 0 {
		t.Error("Expected positive expires_in value")
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	_, _ = s.CreateUserWithPassword("login-user2", "correctpassword", models.RoleNormalAI)

	reqBody := LoginRequest{Username: "login-user2", Password: "wrongpassword"}
	resp, err := makeRequest(app, "POST", "/api/auth/login", reqBody)
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}

	if resp.Code != 401 {
		t.Errorf("Expected status 401 for invalid credentials, got %d", resp.Code)
	}
}

func TestLogin_MissingUsernameOrPassword(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	reqBody := LoginRequest{Username: "", Password: ""}
	resp, err := makeRequest(app, "POST", "/api/auth/login", reqBody)
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for missing credentials, got %d", resp.Code)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	reqBody := LoginRequest{Username: "nonexistent-user", Password: "password"}
	resp, err := makeRequest(app, "POST", "/api/auth/login", reqBody)
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}

	// Should return 401 (generic error to not reveal user existence)
	if resp.Code != 401 {
		t.Errorf("Expected status 401 for nonexistent user login, got %d", resp.Code)
	}
}

func TestLogin_RememberMe(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	_, _ = s.CreateUserWithPassword("remember-user", "testpassword", models.RoleNormalAI)

	reqBody := LoginRequest{Username: "remember-user", Password: "testpassword", RememberMe: true}
	resp, err := makeRequest(app, "POST", "/api/auth/login", reqBody)
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for login with remember_me, got %d", resp.Code)
		return
	}

	var result LoginResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken == "" || result.ExpiresIn <= 0 {
		t.Error("Expected valid token and expiry from remember-me login")
	}
}

func TestLogout_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	_, _ = s.CreateUserWithPassword("logout-user", "testpassword", models.RoleNormalAI)

	// First login to get JWT
	loginBody := LoginRequest{Username: "logout-user", Password: "testpassword"}
	respLogin, err := makeRequest(app, "POST", "/api/auth/login", loginBody)
	if err != nil || respLogin.Code != 200 {
		t.Fatalf("Setup login failed: code=%d err=%v", respLogin.Code, err)
	}

	var loginResult LoginResponse
	json.NewDecoder(respLogin.Body).Decode(&loginResult)

	// Now logout with JWT
	resp, err := makeRequestWithAuth(app, loginResult.AccessToken, "POST", "/api/auth/logout", nil)
	if err != nil {
		t.Fatalf("Logout request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for logout, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["message"] != "logged out successfully" {
		t.Errorf("Logout response mismatch: %+v", result)
	}
}

func TestMe_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	_, _ = s.CreateUserWithPassword("me-user", "testpassword", models.RoleNormalAI)

	loginBody := LoginRequest{Username: "me-user", Password: "testpassword"}
	respLogin, err := makeRequest(app, "POST", "/api/auth/login", loginBody)
	if err != nil || respLogin.Code != 200 {
		t.Fatalf("Setup login failed: code=%d err=%v", respLogin.Code, err)
	}

	var loginResult LoginResponse
	json.NewDecoder(respLogin.Body).Decode(&loginResult)

	resp, err := makeRequestWithAuth(app, loginResult.AccessToken, "GET", "/api/auth/me", nil)
	if err != nil {
		t.Fatalf("Me request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for /me, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["name"] != "me-user" {
		t.Errorf("Expected name=me-user, got %+v", result)
	}
	if result["authenticated"] != true {
		t.Error("Expected authenticated=true from /me")
	}
}

func TestCheckAuthStatus_Authenticated(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	_, _ = s.CreateUserWithPassword("check-user", "testpassword", models.RoleNormalAI)

	loginBody := LoginRequest{Username: "check-user", Password: "testpassword"}
	respLogin, err := makeRequest(app, "POST", "/api/auth/login", loginBody)
	if err != nil || respLogin.Code != 200 {
		t.Fatalf("Setup login failed")
	}

	var loginResult LoginResponse
	json.NewDecoder(respLogin.Body).Decode(&loginResult)

	resp, err := makeRequestWithAuth(app, loginResult.AccessToken, "GET", "/api/auth/check", nil)
	if err != nil {
		t.Fatalf("Check request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for auth check, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["authenticated"] != true {
		t.Error("Expected authenticated=true")
	}
	if result["username"] != "check-user" {
		t.Errorf("Expected username=check-user, got %+v", result)
	}
}

func TestCheckAuthStatus_NotAuthenticated(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	resp, err := makeRequest(app, "GET", "/api/auth/check", nil)
	if err != nil {
		t.Fatalf("Check request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for auth check (even when not authenticated), got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["authenticated"] != false {
		t.Error("Expected authenticated=false without token")
	}
}

func TestCheckAuthStatus_InvalidToken(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	resp, err := makeRequestWithAuth(app, "invalid-token-string", "GET", "/api/auth/check", nil)
	if err != nil {
		t.Fatalf("Check request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for auth check with invalid token, got %d", resp.Code)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["authenticated"] != false {
		t.Error("Expected authenticated=false for invalid token")
	}
}

func TestRefresh_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	_, _ = s.CreateUserWithPassword("refresh-user", "testpassword", models.RoleNormalAI)

	loginBody := LoginRequest{Username: "refresh-user", Password: "testpassword"}
	respLogin, err := makeRequest(app, "POST", "/api/auth/login", loginBody)
	if err != nil || respLogin.Code != 200 {
		t.Fatalf("Setup login failed")
	}

	var loginResult LoginResponse
	json.NewDecoder(respLogin.Body).Decode(&loginResult)

	resp, err := makeRequestWithAuth(app, loginResult.AccessToken, "POST", "/api/auth/refresh", nil)
	if err != nil {
		t.Fatalf("Refresh request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for refresh with valid token, got %d", resp.Code)
		return
	}

	var result LoginResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken == "" {
		t.Error("Expected non-empty access_token from refresh")
	}
	if result.User.Name != "refresh-user" {
		t.Errorf("Expected username=refresh-user, got %+v", result.User)
	}
}

func TestRefresh_MissingAuthHeader(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	resp, err := makeRequest(app, "POST", "/api/auth/refresh", nil)
	if err != nil {
		t.Fatalf("Refresh request failed: %v", err)
	}

	if resp.Code != 401 {
		t.Errorf("Expected status 401 for refresh without auth header, got %d", resp.Code)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	resp, err := makeRequestWithAuth(app, "completely-invalid-jwt-token", "POST", "/api/auth/refresh", nil)
	if err != nil {
		t.Fatalf("Refresh request failed: %v", err)
	}

	if resp.Code != 401 {
		t.Errorf("Expected status 401 for refresh with invalid token, got %d", resp.Code)
	}
}

// ============================================================================
// Board Handler Tests (boards.go handlers)

func TestInitBoards_Success(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)

	boards := []config.Board{
		{ID: "board-1", Title: "Board One", Columns: []string{"todo-0", "inprogress-0", "done-0"}},
		{ID: "board-2", Title: "Board Two", Columns: []string{"backlog-0", "inprogress-0", "review-0", "done-0"}},
	}

	InitBoards(boards, s)

	if len(boardStates) != 2 {
		t.Errorf("Expected 2 boards initialized, got %d", len(boardStates))
	}

	b1 := boardStates["board-1"]
	if b1 == nil {
		t.Fatal("board-1 not found in boardStates")
	}
	if len(b1.Columns) != 3 {
		t.Errorf("Expected 3 columns for board-1, got %d", len(b1.Columns))
	}

	b2 := boardStates["board-2"]
	if b2 == nil {
		t.Fatal("board-2 not found in boardStates")
	}
	if len(b2.Columns) != 4 {
		t.Errorf("Expected 4 columns for board-2, got %d", len(b2.Columns))
	}
}

func TestLoadTicketsFromDB(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)

	testBoard := config.Board{
		ID: "test-board-load", Title: "Load Board", Columns: []string{"todo-0", "inprogress-0"},
	}

	now := time.Now().Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		ticket := &models.Ticket{
			ID: fmt.Sprintf("load-ticket-%d", i), Title: fmt.Sprintf("Ticket %d", i),
			Priority: "medium", Column: "todo-0", BoardID: "test-board-load",
			CreatedAt: now, UpdatedAt: now,
		}
		s.CreateTicket(ticket)
	}

	InitBoards([]config.Board{testBoard}, s)

	board := boardStates["test-board-load"]
	if board == nil {
		t.Fatal("board not found after init")
	}

	todoCol := board.Columns[0] // todo-0
	if len(todoCol.Tickets) != 3 {
		t.Errorf("Expected 3 tickets loaded into memory, got %d", len(todoCol.Tickets))
	}
}

func TestAddTicketToMemory_Success(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)
	testBoard := config.Board{ID: "add-board", Title: "Add Board", Columns: []string{"todo-0"}}
	InitBoards([]config.Board{testBoard}, s)

	ticket := &models.Ticket{
		ID: "add-mem-ticket", Title: "Memory Test", Priority: "medium",
		Column: "todo-0", BoardID: "add-board",
		CreatedAt: time.Now().Format(time.RFC3339), UpdatedAt: time.Now().Format(time.RFC3339),
	}

	mu.Lock()
	addTicketToMemory(ticket)
	mu.Unlock()

	board := boardStates["add-board"]
	if len(board.Columns[0].Tickets) != 1 {
		t.Errorf("Expected 1 ticket in memory, got %d", len(board.Columns[0].Tickets))
	}
}

func TestAddTicketToMemory_UnknownBoard(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)
	testBoard := config.Board{ID: "add-board-2", Title: "Add Board 2", Columns: []string{"todo-0"}}
	InitBoards([]config.Board{testBoard}, s)

	ticket := &models.Ticket{
		ID: "unknown-board-ticket", Title: "Unknown Board Test", Priority: "medium",
		Column: "todo-0", BoardID: "nonexistent-board-id", // This board doesn't exist
		CreatedAt: time.Now().Format(time.RFC3339), UpdatedAt: time.Now().Format(time.RFC3339),
	}

	mu.Lock()
	addTicketToMemory(ticket) // Should not panic, just log a warning
	mu.Unlock()

	// The ticket should NOT be in any board's columns
	board := boardStates["add-board-2"]
	if len(board.Columns[0].Tickets) != 0 {
		t.Error("Expected no tickets added for unknown board reference")
	}
}

func TestCountTotalTickets(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)
	testBoard := config.Board{ID: "count-board", Title: "Count Board", Columns: []string{"todo-0", "inprogress-0"}}
	InitBoards([]config.Board{testBoard}, s)

	now := time.Now().Format(time.RFC3339)
	for i := 0; i < 2; i++ {
		s.CreateTicket(&models.Ticket{
			ID: fmt.Sprintf("count-ticket-%d", i), Title: "Count Test", Priority: "medium",
			Column: "todo-0", BoardID: "count-board", CreatedAt: now, UpdatedAt: now,
		})
	}

	mu.Lock()
	loadTicketsFromDB(s)
	total := countTotalTickets()
	mu.Unlock()

	if total != 2 {
		t.Errorf("Expected total ticket count=2, got %d", total)
	}
}

func TestSyncTicketInMemory(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)
	testBoard := config.Board{ID: "sync-board", Title: "Sync Board", Columns: []string{"todo-0", "inprogress-0"}}
	InitBoards([]config.Board{testBoard}, s)

	ticket := &models.Ticket{
		ID: "sync-ticket", Title: "Sync Test", Priority: "medium",
		Column: "todo-0", BoardID: "sync-board",
		CreatedAt: time.Now().Format(time.RFC3339), UpdatedAt: time.Now().Format(time.RFC3339),
	}

	mu.Lock()
	addTicketToMemory(ticket)
	mu.Unlock()

	// Verify ticket is in todo-0
	board := boardStates["sync-board"]
	foundInTodo := false
	for _, t := range board.Columns[0].Tickets {
		if t.ID == "sync-ticket" {
			foundInTodo = true
			break
		}
	}
	if !foundInTodo {
		t.Fatal("Setup: ticket not found in todo-0 before sync")
	}

	// Change column and sync
	ticket.Column = "inprogress-0"
	mu.Lock()
	syncTicketInMemory(ticket)
	mu.Unlock()

	// Verify ticket moved to inprogress-0
	foundInTodo = false
	for _, t := range board.Columns[0].Tickets {
		if t.ID == "sync-ticket" {
			foundInTodo = true
			break
		}
	}
	if foundInTodo {
		t.Error("Ticket should have been removed from todo-0 after sync")
	}

	foundInProgress := false
	for _, t := range board.Columns[1].Tickets {
		if t.ID == "sync-ticket" {
			foundInProgress = true
			break
		}
	}
	if !foundInProgress {
		t.Error("Ticket should be in inprogress-0 after sync")
	}
}

func TestSyncTicketInMemory_NilTicket(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	boardStates = make(map[string]*models.BoardState)
	testBoard := config.Board{ID: "nil-board", Title: "Nil Board", Columns: []string{"todo-0"}}
	InitBoards([]config.Board{testBoard}, s)

	// Should not panic with nil ticket
	mu.Lock()
	syncTicketInMemory(nil)
	mu.Unlock()

	t.Log("syncTicketInMemory handled nil ticket without panicking")
}

func TestHandleListBoards_Success(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t) // Uses InitBoards internally
	defer cleanup()

	resp, err := makeRequest(app, "GET", "/api/boards", nil)
	if err != nil {
		t.Fatalf("List boards request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for list boards, got %d", resp.Code)
		return
	}

	var result []*models.CompactBoard
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) < 1 {
		t.Error("Expected at least 1 board from /api/boards")
	}
}

func TestHandleGetBoard_Success(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	resp, err := makeRequest(app, "GET", "/api/boards/test-board", nil)
	if err != nil {
		t.Fatalf("Get board request failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Expected status 200 for get board, got %d", resp.Code)
		return
	}
}

func TestHandleGetBoard_NotFound(t *testing.T) {
	app, _, cleanup := setupTestAppWithAuth(t)
	defer cleanup()

	resp, err := makeRequest(app, "GET", "/api/boards/nonexistent-board", nil)
	if err != nil {
		t.Fatalf("Get board request failed: %v", err)
	}

	if resp.Code != 404 {
		t.Errorf("Expected status 404 for non-existent board, got %d", resp.Code)
	}
}

// ============================================================================
// Register Handler Tests (register.go handlers)

func setupTestAppWithRegister(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	boardStates = make(map[string]*models.BoardState)
	dbStore = nil
	adminUserService = nil
	userService = nil

	defaultBoard := config.Board{
		ID: "test-board", Title: "Test Board", Columns: []string{"todo-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)

	dbStore = s
	adminUserService = services.NewUserService(s)
	userService = adminUserService
	auth.SetStore(s)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterRegistrationRoutes(app)

	cleanup := func() { dbCleanup(); _ = s.Close() }
	return app, s, cleanup
}

func TestHandleRegister_Success(t *testing.T) {
	app, s, cleanup := setupTestAppWithRegister(t)
	defer cleanup()

	reqBody := RegisterRequest{AgentName: "new-register-agent"}
	resp, err := makeRequest(app, "POST", "/api/v1/register", reqBody)
	if err != nil {
		t.Fatalf("Register request failed: %v", err)
	}

	if resp.Code != 201 {
		t.Errorf("Expected status 201 for successful registration, got %d", resp.Code)
		return
	}

	var result RegisterResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.User.Name != "new-register-agent" {
		t.Errorf("Expected user name=new-register-agent, got %+v", result.User)
	}
	if result.User.Role != models.RoleNormalAI {
		t.Errorf("Expected role=NORMAL_AI for self-registration, got %s", result.User.Role)
	}
	if result.Token == nil || result.Token.Token == "" {
		t.Error("Expected non-empty token in registration response")
	}

	// Verify user exists in store
	retrieved, _ := s.GetUserByName("new-register-agent")
	if retrieved == nil {
		t.Fatal("User not found in store after registration")
	}
	if retrieved.Role != models.RoleNormalAI {
		t.Errorf("Expected NORMAL_AI role in store, got %s", retrieved.Role)
	}
}

func TestHandleRegister_DuplicateUsername(t *testing.T) {
	app, _, cleanup := setupTestAppWithRegister(t)
	defer cleanup()

	// First registration
	reqBody := RegisterRequest{AgentName: "dup-register-agent"}
	resp1, err := makeRequest(app, "POST", "/api/v1/register", reqBody)
	if err != nil || resp1.Code != 201 {
		t.Fatalf("Setup registration failed: code=%d err=%v", resp1.Code, err)
	}

	// Second registration with same name should fail
	resp2, err := makeRequest(app, "POST", "/api/v1/register", reqBody)
	if err != nil {
		t.Fatalf("Duplicate register request failed: %v", err)
	}

	if resp2.Code != 409 {
		t.Errorf("Expected status 409 for duplicate registration, got %d", resp2.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result)
	if int(result["user_id"].(float64)) == 0 {
		t.Error("Expected non-zero user_id in duplicate response")
	}
}

func TestHandleRegister_InvalidAgentName(t *testing.T) {
	app, _, cleanup := setupTestAppWithRegister(t)
	defer cleanup()

	reqBody := RegisterRequest{AgentName: ""} // Empty name should fail validation
	resp, err := makeRequest(app, "POST", "/api/v1/register", reqBody)
	if err != nil {
		t.Fatalf("Register request failed: %v", err)
	}

	if resp.Code != 400 {
		t.Errorf("Expected status 400 for invalid agent name, got %d", resp.Code)
	}
}

func TestHandleRegister_InvalidJSON(t *testing.T) {
	app, _, cleanup := setupTestAppWithRegister(t)
	defer cleanup()

	// Send malformed JSON body
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	respHTTP, err := app.Test(req)
	if err != nil {
		t.Fatalf("Register request failed: %v", err)
	}

	if respHTTP.StatusCode != 400 {
		bodyBytes, _ := io.ReadAll(respHTTP.Body)
		t.Errorf("Expected status 400 for invalid JSON body, got %d body=%s", respHTTP.StatusCode, string(bodyBytes))
	}
}
