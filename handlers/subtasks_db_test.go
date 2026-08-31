package handlers

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/config"
	"goban/models"
	"goban/services"
	"goban/store"
)

func setupTestAppWithSubtasks(t *testing.T) (*fiber.App, *store.SQLiteStore, func()) {
	t.Helper()

	s, dbCleanup := setupTestStore(t)

	boardStates = make(map[string]*models.BoardState)
	dbStore = nil

	defaultBoard := config.Board{
		ID:      "test-board",
		Title:   "Test Board",
		Columns: []string{"todo-0", "inprogress-0", "review-0", "done-0"},
	}
	InitBoards([]config.Board{defaultBoard}, s)
	dbStore = s
	auth.SetStore(s)
	auth.SetJWTSecret([]byte("test-jwt-secret-for-subtasks"))
	adminUserService = services.NewUserService(s)
	userService = adminUserService

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterSubtaskRoutes(app)

	cleanup := func() { dbCleanup(); _ = s.Close() }
	return app, s, cleanup
}

func TestHandleAddSubtask_WorksWhenCacheEmpty(t *testing.T) {
	app, s, cleanup := setupTestAppWithSubtasks(t)
	defer cleanup()

	createTestTicket(t, s, "subtask-cache-1", "todo-0")
	mu.Lock()
	boardStates = make(map[string]*models.BoardState)
	mu.Unlock()

	jwtToken, _ := createAdminJWT(t, s, "test-subtasker-cache", models.RoleNormalAI)
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/subtask-cache-1/subtasks", SubtaskRequest{Title: "from db"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Code != 200 {
		t.Fatalf("expected 200 when ticket is only in DB, got %d", resp.Code)
	}

	got, err := s.GetTicket("subtask-cache-1")
	if err != nil || got == nil {
		t.Fatalf("GetTicket: %v %#v", err, got)
	}
	if len(got.Subtasks) != 1 || got.Subtasks[0].Title != "from db" {
		t.Fatalf("subtasks not persisted: %+v", got.Subtasks)
	}
}
