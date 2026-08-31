package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"goban/config"
	"goban/models"
	"goban/testutil"
)

func TestEncodeTicketList_NilStore(t *testing.T) {
	got, err := encodeTicketList(nil, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "[]" {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeTicketList_FromStore(t *testing.T) {
	db := testutil.NewMockStore()
	if err := db.CreateTicket(&models.Ticket{ID: "t-1", Title: "One", BoardID: "human-to-ai", Column: "todo-0"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateTicket(&models.Ticket{ID: "t-2", Title: "Two", BoardID: "ai-to-human", Column: "todo-0"}); err != nil {
		t.Fatal(err)
	}

	all, err := encodeTicketList(db, "")
	if err != nil {
		t.Fatal(err)
	}
	var tickets []*models.Ticket
	if err := json.Unmarshal([]byte(all), &tickets); err != nil {
		t.Fatalf("json: %v %s", err, all)
	}
	if len(tickets) != 2 {
		t.Fatalf("want 2 tickets, got %d", len(tickets))
	}

	filtered, err := encodeTicketList(db, "human-to-ai")
	if err != nil {
		t.Fatal(err)
	}
	tickets = nil
	if err := json.Unmarshal([]byte(filtered), &tickets); err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 1 || tickets[0].ID != "t-1" {
		t.Fatalf("board filter: %+v", tickets)
	}
}

func TestStart_HTTPTransportNotImplemented(t *testing.T) {
	cfg := config.Config{MCPEnabled: true, MCPTransport: "http", Version: "test"}
	err := Start(cfg, nil)
	if err == nil {
		t.Fatal("expected error for unimplemented HTTP transport")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("unexpected error: %v", err)
	}
}
