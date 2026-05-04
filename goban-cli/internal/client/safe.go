package client

import (
	"context"

	gerr "goban-cli/internal/errors"
	"goban-cli/internal/types"
)

// SafeGet fetches a ticket by ID and returns a classified error on failure.
func (c *Client) SafeGet(ctx context.Context, ticketID string) (*types.Ticket, *gerr.ClassifiedError) {
	ticket, err := c.GetTicketByID(ctx, ticketID)
	if err != nil {
		return nil, Classify(err, "fetching ticket "+ticketID)
	}
	return ticket, nil
}

// SafeMove moves a ticket and returns a classified error on failure.
func (c *Client) SafeMove(ctx context.Context, ticketID, targetStatus string, force bool) (*MoveResponse, *gerr.ClassifiedError) {
	resp, err := c.MoveTicket(ctx, ticketID, targetStatus, force)
	if err != nil {
		return nil, Classify(err, "moving ticket "+ticketID+" to "+targetStatus)
	}
	return resp, nil
}

// SafeClaim claims a ticket and returns a classified error on failure.
func (c *Client) SafeClaim(ctx context.Context, ticketID string) (*ClaimResponse, *gerr.ClassifiedError) {
	resp, err := c.ClaimTicket(ctx, ticketID)
	if err != nil {
		return nil, Classify(err, "claiming ticket "+ticketID)
	}
	return resp, nil
}

// SafeCreate creates a ticket and returns a classified error on failure.
func (c *Client) SafeCreate(ctx context.Context, boardID string, req types.CreateTicketRequest) (*types.Ticket, *gerr.ClassifiedError) {
	ticket, err := c.CreateTicket(ctx, boardID, req)
	if err != nil {
		return nil, Classify(err, "creating ticket on board "+boardID)
	}
	return ticket, nil
}

// SafeDelete deletes a ticket and returns a classified error on failure.
func (c *Client) SafeDelete(ctx context.Context, boardID, ticketID string) *gerr.ClassifiedError {
	err := c.DeleteTicket(ctx, boardID, ticketID)
	if err != nil {
		return Classify(err, "deleting ticket "+ticketID+" from board "+boardID)
	}
	return nil
}

// SafeUpdate updates a ticket and returns a classified error on failure.
func (c *Client) SafeUpdate(ctx context.Context, boardID, ticketID string, req types.UpdateTicketRequest) (*types.Ticket, *gerr.ClassifiedError) {
	ticket, err := c.UpdateTicket(ctx, boardID, ticketID, req)
	if err != nil {
		return nil, Classify(err, "updating ticket "+ticketID+" on board "+boardID)
	}
	return ticket, nil
}

// SafeAddComment adds a comment and returns a classified error on failure.
func (c *Client) SafeAddComment(ctx context.Context, boardID, ticketID string, req types.AddCommentRequest) (*types.Comment, *gerr.ClassifiedError) {
	comment, err := c.AddComment(ctx, boardID, ticketID, req)
	if err != nil {
		return nil, Classify(err, "adding comment to ticket "+ticketID)
	}
	return comment, nil
}

// SafeListTickets lists tickets with params and returns a classified error on failure.
func (c *Client) SafeListTickets(ctx context.Context, params ListTicketsRequestParams) ([]types.Ticket, *gerr.ClassifiedError) {
	tickets, err := c.ListTicketsWithParams(ctx, params)
	if err != nil {
		return nil, Classify(err, "listing tickets")
	}
	return tickets, nil
}

// SafeListBoards lists boards and returns a classified error on failure.
func (c *Client) SafeListBoards(ctx context.Context) ([]types.Board, *gerr.ClassifiedError) {
	boards, err := c.ListBoards(ctx)
	if err != nil {
		return nil, Classify(err, "listing boards")
	}
	return boards, nil
}

// SafeListComments lists comments for a ticket and returns a classified error on failure.
func (c *Client) SafeListComments(ctx context.Context, boardID, ticketID string) ([]types.Comment, *gerr.ClassifiedError) {
	comments, err := c.ListComments(ctx, boardID, ticketID)
	if err != nil {
		return nil, Classify(err, "listing comments for ticket "+ticketID)
	}
	return comments, nil
}

// SafeRelease releases a ticket and returns a classified error on failure.
func (c *Client) SafeRelease(ctx context.Context, ticketID string) (*ReleaseResponse, *gerr.ClassifiedError) {
	resp, err := c.ReleaseTicket(ctx, ticketID)
	if err != nil {
		return nil, Classify(err, "releasing ticket "+ticketID)
	}
	return resp, nil
}
