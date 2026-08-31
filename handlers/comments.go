// Comment management handlers for Goban API.
package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/models"
	"goban/sse"
	"goban/validation"
)

type CommentRequest struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

func handleAddComment(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")

	// Tie comment identity to authenticated user — prevent spoofing.
	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return auth.SendAuthError(c, "User not authenticated")
	}

	var req CommentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := validation.ValidateComment(req.Text); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	mu.Lock()
	defer mu.Unlock()

	t, err := loadTicketFromDB(ticketID)
	if err != nil || t == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
	}

	timestamp := req.Timestamp
	if timestamp == "" {
		timestamp = time.Now().Format(time.RFC3339)
	}

	suffix := ticketID
	if len(suffix) > 8 {
		suffix = ticketID[len(ticketID)-8:]
	}
	commentID := fmt.Sprintf("comment-%s-%d", suffix, len(t.Comments))
	comment := models.Comment{
		ID:        commentID,
		Who:       user.Name,
		Text:      req.Text,
		Timestamp: timestamp,
	}

	t.Comments = append(t.Comments, comment)
	t.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := saveTicketToDB(t); err != nil {
		log.Printf("ERROR: Failed to persist comment on ticket %s: %v", ticketID, err)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save comment: %v", err)})
	}
	replaceTicketInCache(t)

	log.Printf("Added comment to ticket %s by %s (authenticated)", ticketID, user.Name)

	sse.Emit("comment_add", ticketID, t.BoardID, fiber.Map{
		"ticket_id": ticketID,
		"comment":   comment,
	})

	return c.JSON(fiber.Map{"status": "added", "comment": comment})
}

func handleDeleteComment(c *fiber.Ctx) error {
	ticketID := c.Params("ticketId")
	commentIndex := c.QueryInt("index", -1)

	if commentIndex < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid comment index"})
	}

	mu.Lock()
	defer mu.Unlock()

	t, err := loadTicketFromDB(ticketID)
	if err != nil || t == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
	}

	if commentIndex >= len(t.Comments) {
		return c.Status(404).JSON(fiber.Map{"error": "Comment not found"})
	}

	t.Comments = append(t.Comments[:commentIndex], t.Comments[commentIndex+1:]...)
	t.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := saveTicketToDB(t); err != nil {
		log.Printf("ERROR: Failed to persist comment deletion on ticket %s: %v", ticketID, err)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete comment: %v", err)})
	}
	replaceTicketInCache(t)

	log.Printf("Deleted comment %d from ticket %s", commentIndex, ticketID)

	sse.Emit("comment_delete", ticketID, t.BoardID, fiber.Map{
		"ticket_id":     ticketID,
		"comment_index": commentIndex,
	})

	return c.JSON(fiber.Map{"status": "deleted"})
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

	t, err := loadTicketFromDB(ticketID)
	if err != nil || t == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Ticket not found"})
	}

	if len(t.Comments) == 0 {
		return c.JSON(fiber.Map{"comments": []models.Comment{}})
	}
	return c.JSON(fiber.Map{"comments": t.Comments})
}

func loadTicketFromDB(id string) (*models.Ticket, error) {
	if dbStore == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return dbStore.GetTicket(id)
}

func replaceTicketInCache(ticket *models.Ticket) {
	if ticket == nil {
		return
	}
	for _, board := range boardStates {
		for _, col := range board.Columns {
			for i, cached := range col.Tickets {
				if cached.ID == ticket.ID {
					col.Tickets[i] = ticket
					return
				}
			}
		}
	}
}
