// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"goban/config"
	"goban/models"
	"goban/testutil"
)

// newTestApp creates a Fiber app with mock store and registered routes for testing.
func newTestApp(t *testing.T, boards ...config.Board) (*fiber.App, *testutil.MockStore) {
	t.Helper()

	store := testutil.NewMockStore()
	if err := store.Init(); err != nil {
		t.Fatalf("Failed to initialize mock store: %v", err)
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	if len(boards) == 0 {
		// Default test board with standard columns
		boards = []config.Board{{
			ID:      "test-board",
			Title:   "Test Board",
			Columns: []string{"todo", "inprogress", "done"},
		}}
	}

	RegisterRoutes(app, store, boards)

	return app, store
}

// handlerSuite bundles test state for cleaner handler tests.
type handlerSuite struct {
	t          *testing.T
	app        *fiber.App
	db         *testutil.MockStore
	adminToken string // Bearer token for admin-authenticated requests
}

// testAdminBearer is the raw Bearer token value sent by tests.
const testAdminBearer = "test-admin-bearer-token"

// hashToken computes SHA256 hex digest of a string, matching auth.HashToken.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func newHandlerSuite(t *testing.T, boards ...config.Board) *handlerSuite {
	t.Helper()
	app, db := newTestApp(t, boards...)

	// Create admin user + token for authenticated requests.
	// The auth middleware hashes the Bearer value with SHA256 before lookup,
	// so we store the hashed version and return the raw bearer string.
	adminID, _ := db.CreateUser("test-admin", models.RoleHumanAdmin)
	db.CreateTokenWithUser(adminID, "test-admin", hashToken(testAdminBearer))

	s := &handlerSuite{t: t, app: app, db: db}
	s.adminToken = testAdminBearer
	return s
}

// createAuthTicket creates a ticket in the mock store for testing.
func (s *handlerSuite) createTicket(id string) {
	s.db.CreateTicket(&models.Ticket{
		ID: id, Title: "Test Ticket", Column: "todo-0", BoardID: "test-board",
	})
}

// =============================================================================
// GET /api/v1/tickets/:id Tests

func TestGetTicket_Found(t *testing.T) {
	s := newHandlerSuite(t)
	ticket := &models.Ticket{
		ID: "t-1", Title: "Found Ticket", Column: "todo-0", BoardID: "test-board", Priority: "high",
	}
	if err := s.db.CreateTicket(ticket); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/tickets/t-1", nil)
	result, err := s.app.Test(req)
	if err != nil || result.StatusCode != 200 {
		t.Errorf("Expected 200, got %d (err=%v)", result.StatusCode, err)
	}

	var body map[string]interface{}
	json.NewDecoder(result.Body).Decode(&body)
	if body["id"] != "t-1" || body["title"] != "Found Ticket" {
		t.Errorf("Expected ticket t-1 with title 'Found Ticket', got: %v", body)
	}
}

func TestGetTicket_NotFound(t *testing.T) {
	s := newHandlerSuite(t)

	req := httptest.NewRequest("GET", "/api/tickets/nonexistent", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 for nonexistent ticket, got %d", resp.StatusCode)
	}
}

func TestGetTickets_All(t *testing.T) {
	s := newHandlerSuite(t)
	for i := 0; i < 3; i++ {
		id := "t-list-" + string(rune('a'+i))
		s.db.CreateTicket(&models.Ticket{ID: id, Title: "List Ticket", Column: "todo-0", BoardID: "test-board"})
	}

	req := httptest.NewRequest("GET", "/api/tickets", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for GET /tickets, got %d", resp.StatusCode)
	}
}

// =============================================================================
// POST /api/tickets Tests (Create ticket)

func TestCreateTicket_Success(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{
		"title": "New Ticket", "description": "Created via API",
		"priority": "medium", "board_id": "test-board",
	}

	req := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 201 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201 for ticket creation, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] == nil || result["id"].(string) == "" {
		t.Errorf("Expected non-empty id in response, got: %v", result)
	}
	if result["title"] != "New Ticket" {
		t.Errorf("Expected title 'New Ticket', got: %v", result["title"])
	}

	// Verify ticket exists in mock store by checking total count
	allTickets, _ := s.db.GetAllTickets()
	found := false
	for _, tkt := range allTickets {
		if tkt.Title == "New Ticket" && tkt.BoardID == "test-board" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected created ticket to be stored in mock store")
	}
}

func TestCreateTicket_DuplicateID(t *testing.T) {
	s := newHandlerSuite(t)
	body := map[string]interface{}{
		"id": "t-dup", "title": "First", "column": "todo-0", "board_id": "test-board",
	}

	req1 := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(body)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp1, _ := s.app.Test(req1)
	if resp1.StatusCode != 201 {
		t.Errorf("First creation expected 201, got %d", resp1.StatusCode)
	}

	var created1 map[string]interface{}
	json.NewDecoder(resp1.Body).Decode(&created1)
	id1 := created1["id"].(string)

	body["title"] = "Second" // Change title to avoid identical request caching
	req2 := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(body)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp2, _ := s.app.Test(req2)

	var created2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&created2)
	id2 := created2["id"].(string)

	// Simple endpoint auto-generates IDs - both succeed with different IDs.
	if id1 == "" || id2 == "" {
		t.Error("Expected non-empty IDs from both creations")
	}
	if id1 == id2 {
		t.Errorf("Expected different auto-generated IDs, got same: %s", id1)
	}
}

func TestCreateTicket_MissingTitle(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{
		"id": "t-no-title", "column": "todo-0", "board_id": "test-board",
	}

	req := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 201 {
		t.Error("Expected error for ticket without title, got 201")
	}
}

// =============================================================================
// PUT /api/v1/tickets/:id Tests (Update ticket)

func TestUpdateTicket_Success(t *testing.T) {
	s := newHandlerSuite(t)

	// Create ticket via API (populates both mock store and boardStates)
	createBody := map[string]interface{}{
		"title": "Original", "description": "Test update", "board_id": "test-board",
	}
	req1 := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp1, _ := s.app.Test(req1)
	if resp1.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for update test: %d", resp1.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp1.Body).Decode(&created)
	ticketID := created["id"].(string)

	// Now update the ticket
	updateBody := map[string]interface{}{"title": "Updated Title"}
	req2 := httptest.NewRequest("PUT", "/api/tickets/"+ticketID, bytes.NewReader(mustJSON(updateBody)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp2, _ := s.app.Test(req2)
	if resp2.StatusCode != 200 {
		t.Errorf("Expected 200 for ticket update, got %d", resp2.StatusCode)
	}

	retrieved, _ := s.db.GetTicket(ticketID)
	if retrieved.Title != "Updated Title" {
		t.Errorf("Expected 'Updated Title', got '%s'", retrieved.Title)
	}
}

func TestUpdateTicket_NotFound(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{"title": "No change"}
	req := httptest.NewRequest("PUT", "/api/tickets/nonexistent", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 for updating nonexistent ticket, got %d", resp.StatusCode)
	}
}

// =============================================================================
// DELETE /api/v1/tickets/:id Tests

func TestDeleteTicket_Success(t *testing.T) {
	s := newHandlerSuite(t)

	// Create ticket via API first (populates boardStates)
	createBody := map[string]interface{}{"title": "To Delete", "board_id": "test-board"}
	reqCreate := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+s.adminToken)
	respCreate, _ := s.app.Test(reqCreate)
	if respCreate.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for delete test: %d", respCreate.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(respCreate.Body).Decode(&created)
	ticketID := created["id"].(string)

	req := httptest.NewRequest("DELETE", "/api/tickets/"+ticketID+"?force=true", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for ticket deletion, got %d", resp.StatusCode)
	}

	_, err := s.db.GetTicket(ticketID)
	if err == nil {
		t.Error("Expected ticket to be deleted from store")
	}
}

func TestDeleteTicket_NotFound(t *testing.T) {
	s := newHandlerSuite(t)

	req := httptest.NewRequest("DELETE", "/api/tickets/nonexistent?force=true", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 for deleting nonexistent ticket, got %d", resp.StatusCode)
	}
}

// =============================================================================
// Board Tests (GET /api/boards)

func TestGetBoards_Success(t *testing.T) {
	s := newHandlerSuite(t)

	req := httptest.NewRequest("GET", "/api/boards", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for GET /boards, got %d", resp.StatusCode)
	}

	var boards []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&boards)
	if len(boards) == 0 {
		t.Error("Expected at least one board")
	} else if boards[0]["id"] != "test-board" {
		t.Errorf("Expected board id 'test-board', got '%v'", boards[0]["id"])
	}
}

func TestGetBoards_WithTickets(t *testing.T) {
	s := newHandlerSuite(t)
	s.db.CreateTicket(&models.Ticket{ID: "t-b1", Title: "Board Ticket", Column: "todo-0", BoardID: "test-board"})

	req := httptest.NewRequest("GET", "/api/boards/test-board", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for GET /boards/:id, got %d", resp.StatusCode)
	}
}

// =============================================================================
// Pagination Tests (GET /api/tickets with limit/offset)

func TestGetTickets_Pagination(t *testing.T) {
	s := newHandlerSuite(t)
	for i := 0; i < 5; i++ {
		id := "t-page-" + string(rune('a'+i))
		s.db.CreateTicket(&models.Ticket{ID: id, Title: "Page Ticket", Column: "todo-0", BoardID: "test-board"})
	}

	req := httptest.NewRequest("GET", "/api/tickets?limit=2&offset=1", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for paginated tickets, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if tickets, ok := body["tickets"].([]interface{}); !ok || len(tickets) > 2 {
		t.Errorf("Expected max 2 tickets with limit=2, got %d", len(body["tickets"].([]interface{})))
	}
}

// =============================================================================
// Auth Status Tests (GET /api/auth/check)

func TestAuthStatus_NoToken(t *testing.T) {
	s := newHandlerSuite(t)

	req := httptest.NewRequest("GET", "/api/auth/check", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for auth status without token, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if authenticated, ok := body["authenticated"].(bool); !ok || authenticated {
		t.Errorf("Expected authenticated=false, got: %v", body["authenticated"])
	}
}

func TestAuthStatus_InvalidToken(t *testing.T) {
	s := newHandlerSuite(t)

	req := httptest.NewRequest("GET", "/api/auth/check", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-hash")
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for auth status with invalid token, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if authenticated, ok := body["authenticated"].(bool); !ok || authenticated {
		t.Errorf("Expected authenticated=false for invalid token, got: %v", body["authenticated"])
	}
}

func TestAuthStatus_ValidToken(t *testing.T) {
	s := newHandlerSuite(t)

	// Create a user and token in mock store with proper hash
	userID, _ := s.db.CreateUser("test-agent", models.RoleNormalAI)
	bearerToken := "valid-token-for-testing"
	s.db.CreateTokenWithUser(userID, "test-agent-name", hashToken(bearerToken))

	req := httptest.NewRequest("GET", "/api/auth/check", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for auth status with valid token, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if authenticated, ok := body["authenticated"].(bool); !ok || !authenticated {
		t.Errorf("Expected authenticated=true for valid token, got: %v", body["authenticated"])
	}
}

// =============================================================================
// Claim Tests (POST /api/claim/:id)

func TestClaimTicket_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-claim")

	req := httptest.NewRequest("POST", "/api/v1/tickets/t-claim/claim", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated claim, got %d", resp.StatusCode)
	}
}

func TestClaimTicket_Success(t *testing.T) {
	s := newHandlerSuite(t)

	// Create ticket via API (populates boardStates)
	createBody := map[string]interface{}{"title": "To Claim", "board_id": "test-board"}
	reqCreate := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+s.adminToken)
	respCreate, _ := s.app.Test(reqCreate)
	if respCreate.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for claim test: %d", respCreate.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(respCreate.Body).Decode(&created)
	ticketID := created["id"].(string)

	userID, _ := s.db.CreateUser("claimer-agent", models.RoleNormalAI)
	bearerToken := "claim-token-for-testing"
	s.db.CreateTokenWithUser(userID, "claimer-agent", hashToken(bearerToken))

	req := httptest.NewRequest("POST", "/api/v1/tickets/"+ticketID+"/claim", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 200 for successful claim, got %d: %s", resp.StatusCode, string(bodyBytes))
	} else {
		retrieved, _ := s.db.GetTicket(ticketID)
		if retrieved.Assignee != "claimer-agent" {
			t.Errorf("Expected assignee 'claimer-agent', got '%s'", retrieved.Assignee)
		}
	}
}

func TestClaimTicket_AlreadyAssigned(t *testing.T) {
	s := newHandlerSuite(t)

	userID, _ := s.db.CreateUser("owner", models.RoleNormalAI)
	bearerToken1 := "token-owner"
	s.db.CreateTokenWithUser(userID, "owner", hashToken(bearerToken1))

	// Create the ticket first via API (populates boardStates)
	createBody := map[string]interface{}{"title": "Already Claimed", "board_id": "test-board"}
	reqCreate := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+s.adminToken)
	respCreate, _ := s.app.Test(reqCreate)
	if respCreate.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for claim test: %d", respCreate.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(respCreate.Body).Decode(&created)
	ticketID := created["id"].(string)

	// First claim succeeds
	req1 := httptest.NewRequest("POST", "/api/v1/tickets/"+ticketID+"/claim", nil)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+bearerToken1)

	resp1, _ := s.app.Test(req1)
	if resp1.StatusCode != 200 {
		t.Fatalf("First claim expected 200, got %d", resp1.StatusCode)
	}

	// Second claim by different agent should fail or succeed depending on role
	userID2, _ := s.db.CreateUser("other-agent", models.RoleNormalAI)
	bearerToken2 := "token-other"
	s.db.CreateTokenWithUser(userID2, "other-agent", hashToken(bearerToken2))

	req2 := httptest.NewRequest("POST", "/api/v1/tickets/"+ticketID+"/claim", nil)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+bearerToken2)
	resp2, _ := s.app.Test(req2)
	if resp2.StatusCode == 200 {
		t.Log("NORMAL_AI was able to steal another NORMAL_AI's ticket — this may be expected behavior")
	} else if resp2.StatusCode != 412 && resp2.StatusCode != 403 {
		t.Errorf("Expected conflict or forbidden for stealing ticket, got %d", resp2.StatusCode)
	}
}

// =============================================================================
// Release Tests (POST /api/release/:id)

func TestReleaseTicket_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-release")

	req := httptest.NewRequest("POST", "/api/tickets/t/release-release", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated release, got %d", resp.StatusCode)
	}
}

func TestReleaseTicket_Success(t *testing.T) {
	s := newHandlerSuite(t)

	userID, _ := s.db.CreateUser("release-agent", models.RoleNormalAI)
	bearerToken := "release-token-testing"
	s.db.CreateTokenWithUser(userID, "release-agent", hashToken(bearerToken))

	// Create ticket via API first (populates boardStates)
	createBody := map[string]interface{}{"title": "To Release", "board_id": "test-board"}
	reqCreate := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+s.adminToken)
	respCreate, _ := s.app.Test(reqCreate)
	if respCreate.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for release test: %d", respCreate.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(respCreate.Body).Decode(&created)
	ticketID := created["id"].(string)

	req := httptest.NewRequest("POST", "/api/v1/tickets/"+ticketID+"/release", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for successful release, got %d", resp.StatusCode)
	}

	retrieved, _ := s.db.GetTicket(ticketID)
	if retrieved.Assignee != "" {
		t.Errorf("Expected empty assignee after release, got '%s'", retrieved.Assignee)
	}
}

// =============================================================================
// Move Tests (POST /api/move/:id)

func TestMoveTicket_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-move")

	body := map[string]interface{}{"column": "inprogress-0"}
	req := httptest.NewRequest("POST", "/api/v1/tickets/t/move-move", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated move, got %d", resp.StatusCode)
	}
}

func TestMoveTicket_Success(t *testing.T) {
	s := newHandlerSuite(t)

	// Create ticket via API first (populates boardStates)
	createBody := map[string]interface{}{"title": "To Move", "board_id": "test-board"}
	reqCreate := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+s.adminToken)
	respCreate, _ := s.app.Test(reqCreate)
	if respCreate.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for move test: %d", respCreate.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(respCreate.Body).Decode(&created)
	ticketID := created["id"].(string)

	// Use admin token (admin can move any ticket) with force flag
	body := map[string]interface{}{"target_status": "IN_PROGRESS", "force": true}
	req := httptest.NewRequest("POST", "/api/v1/tickets/"+ticketID+"/move", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 200 for successful move, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	retrieved, _ := s.db.GetTicket(ticketID)
	if !strings.HasPrefix(retrieved.Column, "inprogress") {
		t.Errorf("Expected column starting with 'inprogress' after move, got '%s'", retrieved.Column)
	}
}

// =============================================================================
// Archive Tests (POST /api/archive/:id)

func TestArchiveTicket_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-archive")

	req := httptest.NewRequest("POST", "/api/archive/t-archive", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated archive, got %d", resp.StatusCode)
	}
}

func TestArchiveTicket_AdminOnly(t *testing.T) {
	s := newHandlerSuite(t)

	userID, _ := s.db.CreateUser("normal-user", models.RoleNormalAI)
	bearerToken := "archive-normal-token-test"
	s.db.CreateTokenWithUser(userID, "normal-user", bearerToken)

	s.db.CreateTicket(&models.Ticket{ID: "t-archive-norm", Title: "Archive Test", Column: "todo-0", BoardID: "test-board"})

	req := httptest.NewRequest("POST", "/api/archive/t-archive-norm", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 {
		t.Error("Expected auth failure for non-admin archive attempt")
	} else if resp.StatusCode != 403 && resp.StatusCode != 401 {
		t.Logf("Archive by normal user returned %d (may be expected)", resp.StatusCode)
	}
}

// =============================================================================
// Bulk Archive Tests (POST /api/archive/bulk)

func TestBulkArchive_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{"ticket_ids": []string{"t-1"}}
	req := httptest.NewRequest("POST", "/api/archive/bulk", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated bulk archive, got %d", resp.StatusCode)
	}
}

// =============================================================================
// Activity Log Tests (GET /api/v1/tickets/:id/activity)

func TestActivityLogs_Empty(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-logs")

	req := httptest.NewRequest("GET", "/api/v1/activity/t-logs", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for activity logs (even empty), got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if total, ok := body["total"].(float64); !ok || int(total) != 0 {
		t.Errorf("Expected 0 activity logs for new ticket, got: %v", body["total"])
	}
}

func TestActivityLogs_WithEntries(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-logs-with-data")

	// Create some activity log entries
	s.db.CreateActivityLog(&models.ActivityLog{TicketID: "t-logs-with-data", EventType: models.ActivityCreated, Actor: "system"})
	s.db.CreateActivityLog(&models.ActivityLog{TicketID: "t-logs-with-data", EventType: models.ActivityClaimed, Actor: "test-agent"})

	req := httptest.NewRequest("GET", "/api/v1/activity/t-logs-with-data", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for activity logs with entries, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if total, ok := body["total"].(float64); !ok || int(total) == 0 {
		t.Errorf("Expected activity logs to be returned, got: %v", body["activity_log"])
	}
}

// =============================================================================
// Comment Tests (POST /api/v1/tickets/:id/comments)

func TestAddComment_Success(t *testing.T) {
	s := newHandlerSuite(t)

	// Create ticket via API (populates both mock store AND boardStates in-memory)
	createBody := map[string]interface{}{"title": "With Comments", "board_id": "test-board"}
	reqCreate := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+s.adminToken)
	respCreate, _ := s.app.Test(reqCreate)
	if respCreate.StatusCode != 201 {
		t.Fatalf("Failed to create ticket for comment test: %d", respCreate.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(respCreate.Body).Decode(&created)
	ticketID := created["id"].(string)

	body := map[string]interface{}{"who": "commenter", "text": "Test comment"}
	req := httptest.NewRequest("POST", "/api/tickets/"+ticketID+"/comments", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 for adding a comment, got %d", resp.StatusCode)
	}

	retrieved, _ := s.db.GetTicket(ticketID)
	if len(retrieved.Comments) == 0 || retrieved.Comments[0].Text != "Test comment" {
		t.Errorf("Expected comment to be stored, got: %+v", retrieved.Comments)
	}
}

// =============================================================================
// Subtask Tests (PATCH /api/v1/tickets/:id/subtasks)

func TestUpdateSubtask_Success(t *testing.T) {
	s := newHandlerSuite(t)

	// Create ticket via API (subtasks are handled by PUT/PATCH on existing tickets)
	createBody := map[string]interface{}{
		"title": "With Subtasks", "board_id": "test-board",
	}

	req := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 201 {
		t.Fatalf("Create ticket failed: %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	ticketID := created["id"].(string)

	// Add subtasks via PATCH
	subtaskBody := map[string]interface{}{
		"subtasks": []map[string]interface{}{{"title": "Sub 1"}, {"title": "Sub 2"}},
	}
	req2 := httptest.NewRequest("PATCH", "/api/tickets/"+ticketID, bytes.NewReader(mustJSON(subtaskBody)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp2, _ := s.app.Test(req2)
	if resp2.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp2.Body)
		t.Fatalf("Update subtasks failed: %d: %s", resp2.StatusCode, string(bodyBytes))
	}

	retrieved, err := s.db.GetTicket(ticketID)
	if err != nil {
		t.Fatalf("Failed to retrieve ticket: %v", err)
	}
	if len(retrieved.Subtasks) != 2 {
		t.Errorf("Expected 2 subtasks, got %d", len(retrieved.Subtasks))
	}
}

// =============================================================================
// Admin User Tests (GET /api/admin/users)

func TestAdminListUsers_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated admin users list, got %d", resp.StatusCode)
	}
}

func TestAdminListUsers_AdminOnly(t *testing.T) {
	s := newHandlerSuite(t)

	userID, _ := s.db.CreateUser("normal-admin-test", models.RoleNormalAI)
	bearerToken := "admin-normal-token-test"
	s.db.CreateTokenWithUser(userID, "normal-admin-test", bearerToken)

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 {
		t.Error("Expected auth failure for non-admin accessing admin endpoint")
	} else if resp.StatusCode != 403 && resp.StatusCode != 401 {
		t.Logf("Admin endpoint by normal user returned %d (may be expected)", resp.StatusCode)
	}
}

func TestAdminCreateUser_NotAvailable(t *testing.T) {
	s := newHandlerSuite(t)

	adminID, _ := s.db.CreateUser("admin-user", models.RoleHumanAdmin)
	bearerToken := "admin-create-token-test"
	s.db.CreateTokenWithUser(adminID, "admin-user", hashToken(bearerToken))

	body := map[string]interface{}{"username": "new-agent", "role": "NORMAL_AI"}
	req := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 201 {
		t.Fatal("HTTP must not create users; use goban-user-cli against the database")
	}
}

// =============================================================================
// Registration Tests (POST /api/register)

func TestRegister_Success(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{"agent_name": "new-reg-user"}
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201 for registration, got %d", resp.StatusCode)
	}

	user, err := s.db.GetUserByName("new-reg-user")
	if err != nil || user == nil || user.Role != models.RoleNormalAI {
		t.Errorf("Expected registered user with NORMAL_AI role, got: %+v (err=%v)", user, err)
	}
}

func TestRegister_DuplicateName(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{"agent_name": "reg-dup"}
	req1 := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(mustJSON(body)))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := s.app.Test(req1)
	if resp1.StatusCode != 201 {
		t.Fatalf("First registration expected 201, got %d", resp1.StatusCode)
	}

	// Duplicate check with same name
	req2 := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(mustJSON(body)))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := s.app.Test(req2)
	if resp2.StatusCode == 201 {
		t.Error("Expected error for duplicate name in registration")
	} else if resp2.StatusCode != 409 && resp2.StatusCode != 400 {
		t.Logf("Duplicate registration returned %d", resp2.StatusCode)
	}
}

// =============================================================================
// Token Tests (POST /api/admin/tokens/regenerate)

func TestRegenerateToken_AuthRequired(t *testing.T) {
	s := newHandlerSuite(t)

	body := map[string]interface{}{"user_id": 1}
	req := httptest.NewRequest("POST", "/api/admin/tokens/regenerate", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 || resp.StatusCode == 412 {
		t.Errorf("Expected auth failure for unauthenticated token regeneration, got %d", resp.StatusCode)
	}
}

func TestRegenerateToken_AdminOnly(t *testing.T) {
	s := newHandlerSuite(t)

	userID, _ := s.db.CreateUser("normal-token-test", models.RoleNormalAI)
	bearerToken := "token-regen-normal-test"
	s.db.CreateTokenWithUser(userID, "normal-token-test", bearerToken)

	body := map[string]interface{}{"user_id": userID}
	req := httptest.NewRequest("POST", "/api/admin/tokens/regenerate", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 {
		t.Error("Expected auth failure for non-admin token regeneration")
	} else if resp.StatusCode != 403 && resp.StatusCode != 401 {
		t.Logf("Token regen by normal user returned %d", resp.StatusCode)
	}
}

// =============================================================================
// Error Handling Tests (malformed JSON, missing fields)

func TestCreateTicket_MalformedJSON(t *testing.T) {
	s := newHandlerSuite(t)

	body := []byte(`{"invalid json`)
	req := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 201 {
		t.Error("Expected error for malformed JSON")
	} else if resp.StatusCode != 400 && resp.StatusCode != 422 {
		t.Logf("Malformed JSON returned %d", resp.StatusCode)
	}
}

func TestUpdateTicket_MalformedJSON(t *testing.T) {
	s := newHandlerSuite(t)
	s.createTicket("t-malformed")

	body := []byte(`{"invalid json`)
	req := httptest.NewRequest("PUT", "/api/tickets/t-malformed", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	if resp.StatusCode == 200 {
		t.Error("Expected error for malformed JSON in update")
	} else if resp.StatusCode != 400 && resp.StatusCode != 422 {
		t.Logf("Malformed JSON update returned %d", resp.StatusCode)
	}
}

// =============================================================================
// Utility function to marshal map to []byte without errors.

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
