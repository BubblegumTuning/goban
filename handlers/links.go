// Package handlers provides HTTP request handlers for Goban API.
package handlers

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"goban/validation"
)

type LinkRequest struct {
	ParentID string `json:"parent_id"`
}

type LinksResponse struct {
	Parents  []string `json:"parents,omitempty"`
	Children []string `json:"children,omitempty"`
}

// handleAddLink handles POST /api/tickets/:id/links (add a parent dependency)
func handleAddLink(c *fiber.Ctx) error {
	ticketID := c.Params("id")

	// Validate ticket ID format at handler boundary
	if err := validation.ValidateTicketID(ticketID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var req LinkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.ParentID == "" || ticketID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "parent_id is required"})
	}

	// Validate parent ID format at handler boundary
	if err := validation.ValidateTicketID(req.ParentID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid parent_id: " + err.Error()})
	}

	err := dbStore.AddTaskLink(req.ParentID, ticketID)
	if err != nil {
		status := 500
		if containsStr(err.Error(), "self-link") || containsStr(err.Error(), "cycle detected") {
			status = 409 // Conflict
		} else if containsStr(err.Error(), "not found") {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("Added link: parent=%s -> child=%s", req.ParentID, ticketID)
	return c.JSON(fiber.Map{"status": "linked"})
}

// handleRemoveLink handles DELETE /api/tickets/:id/links?parent=<id> (remove a dependency)
func handleRemoveLink(c *fiber.Ctx) error {
	ticketID := c.Params("id") // child ID from path
	parentID := c.Query("parent")

	// Validate ticket ID format at handler boundary
	if err := validation.ValidateTicketID(ticketID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid ticket id: " + err.Error()})
	}

	if parentID == "" || ticketID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "parent query parameter is required"})
	}

	// Validate parent ID format at handler boundary
	if err := validation.ValidateTicketID(parentID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid parent id: " + err.Error()})
	}

	err := dbStore.RemoveTaskLink(parentID, ticketID)
	if err != nil {
		status := 500
		if containsStr(err.Error(), "not found") {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("Removed link: parent=%s -> child=%s", parentID, ticketID)
	return c.JSON(fiber.Map{"status": "unlinked"})
}

// handleGetLinks handles GET /api/tickets/:id/links (get all links for a ticket)
func handleGetLinks(c *fiber.Ctx) error {
	ticketID := c.Params("id")

	parents, children, err := dbStore.GetTaskLinks(ticketID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch links: %v", err)})
	}

	resp := LinksResponse{Parents: parents, Children: children}
	// Ensure empty arrays instead of null in JSON
	if resp.Parents == nil {
		resp.Parents = []string{}
	}
	if resp.Children == nil {
		resp.Children = []string{}
	}

	return c.JSON(resp)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RegisterLinkRoutes registers all task link endpoints under /api/v1/tickets/:id/links.
func RegisterLinkRoutes(app *fiber.App) {
	linkGroup := app.Group("/api/v1/tickets/:id/links", AuthMiddlewareWithRole)

	linkGroup.Get("", handleGetLinks)
	linkGroup.Post("", handleAddLink)
	linkGroup.Delete("?parent=:parentID", handleRemoveLink)

}
