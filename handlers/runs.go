// Package handlers provides HTTP request handlers for Goban API.
package handlers

import (
	"fmt"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"goban/config"
	"goban/models"
	"goban/sse"
	"goban/validation"
)

type RunRequest struct {
	Summary  string `json:"summary,omitempty"`
	Metadata string `json:"metadata,omitempty"`
}

// handleCreateRun creates a new run for the given ticket.
func handleCreateRun(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")
	actor := extractActorFromContext(c)

	var req RunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Validate run fields at handler boundary
	if err := validation.ValidateRunSummary(req.Summary); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidateRunMetadata(req.Metadata); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	run := &models.TicketRun{
		TicketID: ticketID,
		Outcome:  "active",
		Summary:  req.Summary,
		Metadata: req.Metadata,
		Actor:    actor,
	}

	result, err := dbStore.CreateRun(run)
	if err != nil {
		log.Printf("CreateRun failed for ticket %s: %v", ticketID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create run"})
	}

	dbStore.CreateActivityLog(&models.ActivityLog{
		TicketID:  ticketID,
		EventType: models.ActivityClaimed,
		Actor:     actor,
		NewState:  strPtr("active"),
	})

	sse.Emit("ticket_update", ticketID, c.Params("boardId"), fiber.Map{"action": "run_created"})

	return c.Status(fiber.StatusCreated).JSON(result)
}

// handleGetRuns returns all runs for a ticket.
func handleGetRuns(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	runsWithError, err := dbStore.GetRuns(ticketID)
	if err != nil {
		log.Printf("GetRuns failed for ticket %s: %v", ticketID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to retrieve runs"})
	}

	if runsWithError == nil {
		runsWithError = []*models.TicketRun{}
	}

	return c.JSON(runsWithError)
}

// handleGetActiveRun returns the current active run for a ticket.
func handleGetActiveRun(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	run, err := dbStore.GetActiveRun(ticketID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no active run found"})
	}

	return c.JSON(run)
}

// handleUpdateRun updates the outcome of a run.
func handleUpdateRun(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")
	actor := extractActorFromContext(c)

	runIDStr := c.Query("run_id")
	if runIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing run_id query parameter"})
	}

	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid run_id"})
	}

	var req RunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	outcome := req.Summary
	if outcome == "" {
		outcome = "completed"
	}

	// Validate run fields and outcome at handler boundary
	if err := validation.ValidateRunSummary(req.Summary); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidateRunMetadata(req.Metadata); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := validation.ValidateRunOutcome(outcome); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	storeErr := dbStore.UpdateRun(runID, outcome, req.Summary, req.Metadata)
	if storeErr != nil {
		log.Printf("UpdateRun failed for run %d: %v", runID, storeErr)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update run"})
	}

	dbStore.CreateActivityLog(&models.ActivityLog{
		TicketID:  ticketID,
		EventType: models.ActivityCompleted,
		Actor:     actor,
		NewState:  strPtr(outcome),
	})

	sse.Emit("ticket_update", ticketID, c.Params("boardId"), fiber.Map{"action": "run_created"})

	return c.JSON(fiber.Map{"status": "updated", "run_id": runID})
}

// extractActorFromContext retrieves the actor (username/agent name) from request context.
func extractActorFromContext(c *fiber.Ctx) string {
	if actor, ok := c.Locals("actor").(string); ok && actor != "" {
		return actor
	}

	actor := c.Get("X-Actor")
	if actor == "" {
		actor = c.Get("X-Username")
	}
	if actor == "" {
		actor = "system"
	}
	return fmt.Sprintf("@%s", actor)
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// RegisterRunRoutes registers all ticket run endpoints under /api/v1/tickets/:ticketId/runs.
func RegisterRunRoutes(app *fiber.App) {
	runGroup := app.Group("/api/v1/tickets/:ticketId/runs", AuthMiddlewareWithRole)

	runGroup.Post("", handleCreateRun)
	runGroup.Get("", handleGetRuns)
	runGroup.Get("/active", handleGetActiveRun)
	runGroup.Patch("", handleUpdateRun)

	if config.Debug {
		log.Println("DEBUG: Registered ticket run routes /api/v1/tickets/:ticketId/runs")
	}
}
