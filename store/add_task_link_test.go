package store

import (
	"testing"

	"goban/models"
)

func TestSQLiteStore_AddTaskLink_SuccessReturnsNil(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateTicket(&models.Ticket{ID: "p-1", Title: "P", Column: "todo-0", BoardID: "b1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTicket(&models.Ticket{ID: "c-1", Title: "C", Column: "todo-0", BoardID: "b1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTaskLink("p-1", "c-1"); err != nil {
		t.Fatalf("AddTaskLink success should return nil, got %v", err)
	}
}
