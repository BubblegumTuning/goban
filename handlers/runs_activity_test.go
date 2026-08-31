package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"goban/models"
	"goban/testutil"
)

func TestCreateRun_LogsRunStartedNotClaimed(t *testing.T) {
	mock := testutil.NewMockStore()
	prev := dbStore
	dbStore = mock
	t.Cleanup(func() { dbStore = prev })

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/runs/:ticketId", func(c *fiber.Ctx) error {
		c.Locals("actor", "agent")
		return handleCreateRun(c)
	})

	req := httptest.NewRequest("POST", "/runs/t1", strings.NewReader(`{"summary":"started"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status %d", resp.StatusCode)
	}

	logs, err := mock.GetActivityLogs("t1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs: %+v", logs)
	}
	if logs[0].EventType == models.ActivityClaimed {
		t.Fatal("run create logged as claimed")
	}
	if logs[0].EventType != models.ActivityRunStarted {
		t.Fatalf("event %q want %q", logs[0].EventType, models.ActivityRunStarted)
	}
}
