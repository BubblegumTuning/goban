// Subtask management handlers for Goban API.
package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/models"
	"goban/sse"
	"goban/validation"
)

type SubtaskRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done,omitempty"`
}

func handleAddSubtask(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	var req SubtaskRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := validation.ValidateSubtaskTitle(req.Title); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	mu.Lock()
	defer mu.Unlock()

	for _, board := range boardStates {
		boardID := board.ID
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if t.ID == ticketID {
					subtask := models.Subtask{
						Title:     req.Title,
						Completed: req.Done,
					}

					t.Subtasks = append(t.Subtasks, subtask)
					t.UpdatedAt = time.Now().Format(time.RFC3339)

					if err := saveTicketToDB(t); err != nil {
						log.Printf("ERROR: Failed to persist subtask on ticket %s: %v", ticketID, err)
						return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save subtask: %v", err)})
					}

					log.Printf("Added subtask '%s' to ticket %s", req.Title, ticketID)

					sse.Emit("subtask_add", ticketID, boardID, fiber.Map{
						"ticket_id": ticketID,
						"title":     req.Title,
					})

					return c.JSON(fiber.Map{
						"status":   "added",
						"subtasks": t.Subtasks,
					})
				}
			}
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
}

func handleUpdateSubtask(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")
	subtaskIndex := c.QueryInt("index", -1)

	if subtaskIndex < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid subtask index"})
	}

	var req SubtaskRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := validation.ValidateSubtaskTitle(req.Title); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	mu.Lock()
	defer mu.Unlock()

	for _, board := range boardStates {
		boardID := board.ID
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if t.ID == ticketID {
					if subtaskIndex >= len(t.Subtasks) {
						return c.Status(404).JSON(fiber.Map{"error": "Subtask not found"})
					}

					t.Subtasks[subtaskIndex].Title = req.Title
					t.Subtasks[subtaskIndex].Completed = req.Done
					t.UpdatedAt = time.Now().Format(time.RFC3339)

					if err := saveTicketToDB(t); err != nil {
						log.Printf("ERROR: Failed to persist subtask update on ticket %s: %v", ticketID, err)
						return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update subtask: %v", err)})
					}

					log.Printf("Updated subtask %d on ticket %s", subtaskIndex, ticketID)

					sse.Emit("subtask_update", ticketID, boardID, fiber.Map{
						"ticket_id": ticketID,
						"index":     subtaskIndex,
						"title":     req.Title,
						"completed": req.Done,
					})

					return c.JSON(fiber.Map{
						"status":   "updated",
						"subtasks": t.Subtasks,
					})
				}
			}
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
}

func handleDeleteSubtask(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")
	subtaskIndex := c.QueryInt("index", -1)

	if subtaskIndex < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid subtask index"})
	}

	mu.Lock()
	defer mu.Unlock()

	for _, board := range boardStates {
		boardID := board.ID
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if t.ID == ticketID {
					if subtaskIndex >= len(t.Subtasks) {
						return c.Status(404).JSON(fiber.Map{"error": "Subtask not found"})
					}

					t.Subtasks = append(t.Subtasks[:subtaskIndex], t.Subtasks[subtaskIndex+1:]...)
					t.UpdatedAt = time.Now().Format(time.RFC3339)

					if err := saveTicketToDB(t); err != nil {
						log.Printf("ERROR: Failed to persist subtask deletion on ticket %s: %v", ticketID, err)
						return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete subtask: %v", err)})
					}

					log.Printf("Deleted subtask %d from ticket %s", subtaskIndex, ticketID)

					sse.Emit("subtask_delete", ticketID, boardID, fiber.Map{
						"ticket_id": ticketID,
						"index":     subtaskIndex,
					})

					return c.JSON(fiber.Map{
						"status":   "deleted",
						"subtasks": t.Subtasks,
					})
				}
			}
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
}

// RegisterSubtaskRoutes registers all subtask-related endpoints.
func RegisterSubtaskRoutes(app *fiber.App) {
	subtaskGroup := app.Group("/api/tickets/:ticketId/subtasks", AuthMiddlewareWithRole)

	subtaskGroup.Post("", handleAddSubtask)
	subtaskGroup.Patch("?index=:index", handleUpdateSubtask)
	subtaskGroup.Delete("?index=:index", handleDeleteSubtask)
}
