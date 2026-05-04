// Comment management handlers for Goban API.
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

type CommentRequest struct {
	Who       string `json:"who"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

func handleAddComment(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	var req CommentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := validation.ValidateComment(req.Text); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	mu.Lock()
	defer mu.Unlock()

	for _, board := range boardStates {
		boardID := board.ID
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if t.ID == ticketID {
					timestamp := req.Timestamp
					if timestamp == "" {
						timestamp = time.Now().Format(time.RFC3339)
					}

					commentID := fmt.Sprintf("comment-%s-%d", ticketID[len(ticketID)-8:], len(t.Comments))
					comment := models.Comment{
						ID:        commentID,
						Who:       req.Who,
						Text:      req.Text,
						Timestamp: timestamp,
					}

					t.Comments = append(t.Comments, comment)
					t.UpdatedAt = time.Now().Format(time.RFC3339)

					if err := saveTicketToDB(t); err != nil {
						log.Printf("ERROR: Failed to persist comment on ticket %s: %v", ticketID, err)
						return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save comment: %v", err)})
					}

					log.Printf("Added comment to ticket %s by %s", ticketID, req.Who)

					sse.Emit("comment_add", ticketID, boardID, fiber.Map{
						"ticket_id": ticketID,
						"comment":   comment,
					})

					return c.JSON(fiber.Map{"status": "added", "comment": comment})
				}
			}
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
}

func handleDeleteComment(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")
	commentIndex := c.QueryInt("index", -1)

	if commentIndex < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid comment index"})
	}

	mu.Lock()
	defer mu.Unlock()

	for _, board := range boardStates {
		boardID := board.ID
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if t.ID == ticketID {
					if commentIndex >= len(t.Comments) {
						return c.Status(404).JSON(fiber.Map{"error": "Comment not found"})
					}

					t.Comments = append(t.Comments[:commentIndex], t.Comments[commentIndex+1:]...)
					t.UpdatedAt = time.Now().Format(time.RFC3339)

					if err := saveTicketToDB(t); err != nil {
						log.Printf("ERROR: Failed to persist comment deletion on ticket %s: %v", ticketID, err)
						return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete comment: %v", err)})
					}

					log.Printf("Deleted comment %d from ticket %s", commentIndex, ticketID)

					sse.Emit("comment_delete", ticketID, boardID, fiber.Map{
						"ticket_id":     ticketID,
						"comment_index": commentIndex,
					})

					return c.JSON(fiber.Map{"status": "deleted"})
				}
			}
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
}

// RegisterCommentRoutes registers all comment-related endpoints.
func RegisterCommentRoutes(app *fiber.App) {
	commentGroup := app.Group("/api/tickets/:ticketId/comments", AuthMiddlewareWithRole)

	commentGroup.Get("", handleListComments)
	commentGroup.Post("", handleAddComment)
	commentGroup.Delete("?index=:index", handleDeleteComment)
}

// handleListComments returns all comments for a ticket as a list
func handleListComments(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	mu.RLock()
	defer mu.RUnlock()

	for _, board := range boardStates {
		for _, col := range board.Columns {
			for _, t := range col.Tickets {
				if t.ID == ticketID {
					// Return comments as a list, or empty array if none
					if len(t.Comments) == 0 {
						return c.JSON(fiber.Map{"comments": []models.Comment{}})
					}
					return c.JSON(fiber.Map{"comments": t.Comments})
				}
			}
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
}
