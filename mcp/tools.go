package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/config"
	"goban/models"
	"goban/services"
	"goban/store"
	"goban/validation"
)

type createInput struct {
	AuthToken   string `json:"auth_token" jsonschema:"API bearer token"`
	Title       string `json:"title" jsonschema:"ticket title"`
	Description string `json:"description,omitempty"`
	BoardID     string `json:"board_id,omitempty"`
	Column      string `json:"column,omitempty"`
	Priority    string `json:"priority,omitempty"`
}

type claimInput struct {
	AuthToken string `json:"auth_token" jsonschema:"API bearer token"`
	TicketID  string `json:"ticket_id" jsonschema:"ticket to claim"`
}

type moveInput struct {
	AuthToken    string `json:"auth_token" jsonschema:"API bearer token"`
	TicketID     string `json:"ticket_id" jsonschema:"ticket to move"`
	TargetStatus string `json:"target_status" jsonschema:"BACKLOG, TODO, IN_PROGRESS, REVIEW, DONE, CANCELLED"`
	Force        bool   `json:"force,omitempty"`
}

type commentInput struct {
	AuthToken string `json:"auth_token" jsonschema:"API bearer token"`
	TicketID  string `json:"ticket_id"`
	Text      string `json:"text"`
}

type listInput struct {
	BoardID   string `json:"board_id" jsonschema:"board to list from"`
	AuthToken string `json:"auth_token,omitempty"`
}

type listOutput struct {
	Tickets string `json:"tickets" jsonschema:"JSON array"`
}

type ticketResult struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	BoardID string `json:"board_id"`
	Column  string `json:"column"`
}

func encodeTicketList(db store.TicketStore, boardID string) (string, error) {
	if db == nil {
		return "[]", nil
	}
	tickets, err := db.GetAllTickets()
	if err != nil {
		return "", err
	}
	if boardID != "" {
		filtered := make([]*models.Ticket, 0, len(tickets))
		for _, t := range tickets {
			if t != nil && t.BoardID == boardID {
				filtered = append(filtered, t)
			}
		}
		tickets = filtered
	}
	b, err := json.Marshal(tickets)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func userFromToken(db store.TicketStore, token string) (*models.User, error) {
	if token == "" {
		return nil, fmt.Errorf("auth_token is required")
	}
	if db == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	user, err := db.GetUserByToken(auth.HashToken(token))
	if err != nil {
		return nil, err
	}
	if user == nil || user.Name == "" {
		return nil, fmt.Errorf("invalid auth_token")
	}
	return user, nil
}

func createTicket(db store.TicketStore, in createInput) (*models.Ticket, error) {
	if _, err := userFromToken(db, in.AuthToken); err != nil {
		return nil, err
	}
	if err := validation.ValidateTitle(in.Title); err != nil {
		return nil, err
	}
	if err := validation.ValidateDescription(in.Description); err != nil {
		return nil, err
	}
	priority := in.Priority
	if priority != "" {
		if err := validation.ValidatePriority(priority); err != nil {
			return nil, err
		}
		if n, ok := validation.NormalizePriority(priority); ok {
			priority = n
		}
	}
	boardID := in.BoardID
	if boardID == "" {
		boardID = "human-to-ai"
	}
	if !boardAllowed(boardID) {
		return nil, fmt.Errorf("board not found")
	}
	column := in.Column
	if column == "" {
		column = "todo"
	}
	now := time.Now().Format(time.RFC3339)
	t := &models.Ticket{
		ID:          models.GenerateTicketID(),
		Title:       in.Title,
		Description: in.Description,
		Priority:    priority,
		Column:      models.GetColumnID(column),
		BoardID:     boardID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.CreateTicket(t); err != nil {
		return nil, err
	}
	announce("create", t, fiber.Map{
		"title":  t.Title,
		"column": t.Column,
	})
	return t, nil
}

func claimTicket(db store.TicketStore, in claimInput) (*services.ClaimResult, error) {
	user, err := userFromToken(db, in.AuthToken)
	if err != nil {
		return nil, err
	}
	if in.TicketID == "" {
		return nil, fmt.Errorf("ticket_id is required")
	}
	res, err := services.NewClaimService(db).Claim(in.TicketID, user)
	if err != nil {
		return nil, err
	}
	if res != nil && res.Ticket != nil {
		announce("claim", res.Ticket, fiber.Map{"assignee": res.Ticket.Assignee})
		for _, releasedID := range res.AutoReleased {
			rt, gerr := db.GetTicket(releasedID)
			if gerr == nil && rt != nil {
				announce("release", rt, fiber.Map{"assignee": rt.Assignee})
			}
		}
	}
	return res, nil
}

func moveTicket(db store.TicketStore, in moveInput) (*services.MoveResult, error) {
	user, err := userFromToken(db, in.AuthToken)
	if err != nil {
		return nil, err
	}
	if in.TicketID == "" {
		return nil, fmt.Errorf("ticket_id is required")
	}
	if in.TargetStatus == "" {
		return nil, fmt.Errorf("target_status is required")
	}
	res, err := services.NewMoveService(db).Move(in.TicketID, services.MoveRequest{
		TargetStatus: in.TargetStatus,
		Force:        in.Force,
	}, user)
	if err != nil {
		return nil, err
	}
	if res != nil && res.Ticket != nil {
		announce("move", res.Ticket, fiber.Map{"column": res.Ticket.Column})
	}
	return res, nil
}

func addComment(db store.TicketStore, in commentInput) (*models.Comment, error) {
	user, err := userFromToken(db, in.AuthToken)
	if err != nil {
		return nil, err
	}
	if in.TicketID == "" {
		return nil, fmt.Errorf("ticket_id is required")
	}
	if err := validation.ValidateComment(in.Text); err != nil {
		return nil, err
	}
	t, err := db.GetTicket(in.TicketID)
	if err != nil || t == nil {
		return nil, fmt.Errorf("ticket not found")
	}
	suffix := in.TicketID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	c := models.Comment{
		ID:        fmt.Sprintf("comment-%s-%d", suffix, len(t.Comments)),
		Who:       user.Name,
		Text:      in.Text,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	t.Comments = append(t.Comments, c)
	t.UpdatedAt = c.Timestamp
	if err := db.UpdateTicket(t); err != nil {
		return nil, err
	}
	announce("comment_add", t, fiber.Map{
		"ticket_id": in.TicketID,
		"comment":   c,
	})
	return &c, nil
}

func listBoards(cfg config.Config, db store.TicketStore) (string, error) {
	type boardOut struct {
		ID    string `json:"id"`
		Title string `json:"title,omitempty"`
		Desc  string `json:"desc,omitempty"`
	}
	out := make([]boardOut, 0, len(cfg.Boards))
	for _, b := range cfg.Boards {
		out = append(out, boardOut{ID: b.ID, Title: b.Title, Desc: b.Desc})
	}
	if len(out) == 0 && db != nil {
		tickets, err := db.GetAllTickets()
		if err != nil {
			return "", err
		}
		seen := map[string]bool{}
		for _, t := range tickets {
			if t == nil || t.BoardID == "" || seen[t.BoardID] {
				continue
			}
			seen[t.BoardID] = true
			out = append(out, boardOut{ID: t.BoardID})
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
