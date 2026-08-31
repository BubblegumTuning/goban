package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"goban/models"
)

func TestHandleUpdateTicket_WorksWhenCacheEmpty(t *testing.T) {
	s := newHandlerSuite(t)

	createBody := map[string]interface{}{
		"title": "Original", "board_id": "test-board",
	}
	req1 := httptest.NewRequest("POST", "/api/tickets", bytes.NewReader(mustJSON(createBody)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp1, err := s.app.Test(req1)
	if err != nil || resp1.StatusCode != 201 {
		t.Fatalf("create ticket: status=%d err=%v", resp1.StatusCode, err)
	}
	var created map[string]interface{}
	if err := json.NewDecoder(resp1.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	ticketID, _ := created["id"].(string)
	if ticketID == "" {
		t.Fatal("create response missing id")
	}

	mu.Lock()
	boardStates = make(map[string]*models.BoardState)
	mu.Unlock()

	updateBody := map[string]interface{}{"title": "Updated from DB"}
	req2 := httptest.NewRequest("PUT", "/api/tickets/"+ticketID, bytes.NewReader(mustJSON(updateBody)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+s.adminToken)
	resp2, err := s.app.Test(req2)
	if err != nil {
		t.Fatalf("update request: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200 when ticket is only in DB, got %d", resp2.StatusCode)
	}

	got, err := s.db.GetTicket(ticketID)
	if err != nil || got == nil {
		t.Fatalf("GetTicket: %v %#v", err, got)
	}
	if got.Title != "Updated from DB" {
		t.Fatalf("title not persisted: %q", got.Title)
	}
}
