package mcp

import (
	"strings"
	"testing"
	"time"

	"goban/auth"
	"goban/config"
	"goban/models"
	"goban/sse"
	"goban/testutil"
)

func setupMCPUser(t *testing.T, db *testutil.MockStore, name, role, rawToken string) {
	t.Helper()
	id, err := db.CreateUser(name, role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateTokenWithUser(id, name, auth.HashToken(rawToken)); err != nil {
		t.Fatal(err)
	}
}

func TestUserFromToken_Required(t *testing.T) {
	db := testutil.NewMockStore()
	if _, err := userFromToken(db, ""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestUserFromToken_Unknown(t *testing.T) {
	db := testutil.NewMockStore()
	if _, err := userFromToken(db, "nope"); err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestCreateTicket_RequiresAuthAndTitle(t *testing.T) {
	db := testutil.NewMockStore()
	if _, err := createTicket(db, createInput{Title: "x"}); err == nil {
		t.Fatal("expected auth error")
	}
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	if _, err := createTicket(db, createInput{AuthToken: "tok-1"}); err == nil {
		t.Fatal("expected title error")
	}
}

func TestCreateTicket_Persists(t *testing.T) {
	db := testutil.NewMockStore()
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	got, err := createTicket(db, createInput{
		AuthToken: "tok-1", Title: "From MCP", BoardID: "human-to-ai", Column: "todo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "From MCP" || got.BoardID != "human-to-ai" || got.ID == "" {
		t.Fatalf("%+v", got)
	}
	stored, err := db.GetTicket(got.ID)
	if err != nil || stored == nil || stored.Title != "From MCP" {
		t.Fatalf("not persisted: %v %#v", err, stored)
	}
}

func TestCreateTicket_RejectsUnknownBoard(t *testing.T) {
	db := testutil.NewMockStore()
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	SetAllowedBoards([]config.Board{{ID: "human-to-ai"}})
	t.Cleanup(func() { SetAllowedBoards(nil) })
	_, err := createTicket(db, createInput{
		AuthToken: "tok-1", Title: "X", BoardID: "no-such-board",
	})
	if err == nil {
		t.Fatal("expected unknown board error")
	}
}

func TestClaimAndMoveTicket(t *testing.T) {
	db := testutil.NewMockStore()
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	tk, err := createTicket(db, createInput{AuthToken: "tok-1", Title: "Work", BoardID: "human-to-ai"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := claimTicket(db, claimInput{AuthToken: "tok-1", TicketID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Ticket.Assignee != "agent" {
		t.Fatalf("assignee: %+v", claimed.Ticket)
	}
	moved, err := moveTicket(db, moveInput{AuthToken: "tok-1", TicketID: tk.ID, TargetStatus: "REVIEW"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(moved.Ticket.Column, "review") {
		t.Fatalf("column: %s", moved.Ticket.Column)
	}
}

func TestAddComment_Persists(t *testing.T) {
	db := testutil.NewMockStore()
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	tk, err := createTicket(db, createInput{AuthToken: "tok-1", Title: "C"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := addComment(db, commentInput{AuthToken: "tok-1", TicketID: tk.ID, Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Text != "hello" || c.Who != "agent" {
		t.Fatalf("%+v", c)
	}
	got, _ := db.GetTicket(tk.ID)
	if len(got.Comments) != 1 {
		t.Fatalf("comments: %+v", got.Comments)
	}
}

func TestListBoards_FromConfig(t *testing.T) {
	cfg := config.Config{Boards: []config.Board{{ID: "human-to-ai", Title: "H"}}}
	out, err := listBoards(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "human-to-ai") {
		t.Fatalf("got %s", out)
	}
}

func waitSSE(t *testing.T, sub *models.Subscriber) models.SSEEvent {
	t.Helper()
	select {
	case ev := <-sub.Events:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE event")
		return models.SSEEvent{}
	}
}

func TestCreateTicket_EmitsSSE(t *testing.T) {
	sse.Init(16)
	sub, err := sse.Subscribe("", 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sse.Unsubscribe(sub) })

	db := testutil.NewMockStore()
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	got, err := createTicket(db, createInput{
		AuthToken: "tok-1", Title: "From MCP", BoardID: "human-to-ai", Column: "todo",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := waitSSE(t, sub)
	if ev.Type != "create" || ev.TicketID != got.ID || ev.BoardID != "human-to-ai" {
		t.Fatalf("sse: %+v want create %s human-to-ai", ev, got.ID)
	}
}

func TestClaimTicket_EmitsSSEAndCacheHook(t *testing.T) {
	sse.Init(16)
	sub, err := sse.Subscribe("", 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sse.Unsubscribe(sub) })

	var hooked *models.Ticket
	SetAfterTicketWrite(func(t *models.Ticket) { hooked = t })
	t.Cleanup(func() { SetAfterTicketWrite(nil) })

	db := testutil.NewMockStore()
	setupMCPUser(t, db, "agent", models.RoleNormalAI, "tok-1")
	tk, err := createTicket(db, createInput{AuthToken: "tok-1", Title: "Work", BoardID: "human-to-ai"})
	if err != nil {
		t.Fatal(err)
	}
	_ = waitSSE(t, sub) // create

	claimed, err := claimTicket(db, claimInput{AuthToken: "tok-1", TicketID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	ev := waitSSE(t, sub)
	if ev.Type != "claim" || ev.TicketID != tk.ID {
		t.Fatalf("sse: %+v", ev)
	}
	if hooked == nil || hooked.ID != claimed.Ticket.ID {
		t.Fatalf("cache hook not called: %+v", hooked)
	}
}
