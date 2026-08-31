package handlers

import (
	"testing"

	"goban/models"
)

func TestHandleAddComment_WorksWhenCacheEmpty(t *testing.T) {
	app, s, cleanup := setupTestAppWithComments(t)
	defer cleanup()

	createTestTicket(t, s, "comment-cache-1", "todo-0")
	mu.Lock()
	boardStates = make(map[string]*models.BoardState)
	mu.Unlock()

	jwtToken, _ := createAdminJWT(t, s, "test-commenter-cache", models.RoleNormalAI)
	resp, err := makeRequestWithAuth(app, jwtToken, "POST", "/api/tickets/comment-cache-1/comments", CommentRequest{Text: "from db"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Code != 200 {
		t.Fatalf("expected 200 when ticket is only in DB, got %d", resp.Code)
	}

	got, err := s.GetTicket("comment-cache-1")
	if err != nil || got == nil {
		t.Fatalf("GetTicket: %v %#v", err, got)
	}
	if len(got.Comments) != 1 || got.Comments[0].Text != "from db" {
		t.Fatalf("comments not persisted: %+v", got.Comments)
	}
}
