package client

import (
	"context"
	"fmt"

	gerr "goban-cli/internal/errors"
)

// MoveVerificationResult holds the result of verifying a ticket move.
type MoveVerificationResult struct {
	TicketID string // The verified ticket ID
}

// VerifyMove fetches the ticket from server and confirms it is in the expected column.
func (c *Client) VerifyMove(ctx context.Context, ticketID string, expectedColumnPrefix string) (*MoveVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return nil, classErr
	}

	if !ticket.MatchesColumn(expectedColumnPrefix) {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Move failed — state mismatch: expected ticket in [%s] but found in [%s]", expectedColumnPrefix, ticket.Column),
			ticket.ID+" was not moved to the expected column")
	}

	return &MoveVerificationResult{TicketID: ticket.ID}, nil
}

// ClaimVerificationResult holds the result of verifying a ticket claim.
type ClaimVerificationResult struct {
	TicketID string // The verified ticket ID
}

// VerifyClaim fetches the ticket from server and confirms it has an assignee.
func (c *Client) VerifyClaim(ctx context.Context, ticketID string, expectedAssignee string) (*ClaimVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return nil, classErr
	}

	if !ticket.IsClaimed() || (expectedAssignee != "" && ticket.Assignee != expectedAssignee) {
		msg := fmt.Sprintf("Claim failed — state mismatch: ticket %s", ticketID)
		detail := fmt.Sprintf("Expected assignee '%s', got '%s'", expectedAssignee, ticket.Assignee)
		return nil, gerr.NewVerifyFailedError(msg, detail)
	}

	return &ClaimVerificationResult{TicketID: ticket.ID}, nil
}

// DeleteVerificationResult holds the result of verifying a ticket deletion.
type DeleteVerificationResult struct {
	TicketID string // The verified ticket ID that was confirmed deleted
}

// VerifyDelete attempts to fetch the ticket and confirms 404 = success (ticket gone).
func (c *Client) VerifyDelete(ctx context.Context, ticketID string) (*DeleteVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		// 404 means the ticket is gone — that's what we want for delete verification
		if classErr.Category == gerr.CatNotFound {
			return &DeleteVerificationResult{TicketID: ticketID}, nil
		}
		// Other errors (network, server) mean we couldn't verify
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Could not verify deletion of %s", ticketID),
			classErr.Message)
	}

	// Got the ticket back — deletion did NOT take effect
	if ticket != nil {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Delete failed — ticket still exists: %s in column [%s]", ticket.ID, ticket.Column),
			ticket.Title)
	}

	return &DeleteVerificationResult{TicketID: ticketID}, nil
}

// UpdateVerificationResult holds the result of verifying a ticket update.
type UpdateVerificationResult struct {
	TicketID string // The verified ticket ID
}

// VerifyUpdate fetches the ticket and confirms the specified field matches expected value.
func (c *Client) VerifyUpdate(ctx context.Context, ticketID string, field string, expected string) (*UpdateVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return nil, classErr
	}

	var actual string
	switch field {
	case "description":
		actual = ticket.Description
	default:
		actual = ""
	}

	if actual != expected {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Update failed — state mismatch on %s", field),
			fmt.Sprintf("Expected '%s', got '%s'", expected, actual))
	}

	return &UpdateVerificationResult{TicketID: ticket.ID}, nil
}

// CommentVerificationResult holds the result of verifying a comment was added.
type CommentVerificationResult struct {
	TicketID string // The verified ticket ID
}

// VerifyComment fetches the ticket and confirms the latest comment matches expected text/author.
func (c *Client) VerifyComment(ctx context.Context, ticketID string, author string, text string) (*CommentVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return nil, classErr
	}

	// Check if the latest comment matches (comments are ordered newest first on server)
	found := false
	for _, comment := range ticket.Comments {
		if comment.Who == author && comment.Text == text {
			found = true
			break
		}
	}

	if !found {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Comment verification failed — comment not found on ticket %s", ticketID),
			fmt.Sprintf("Expected author '%s' with text '%s'", author, text))
	}

	return &CommentVerificationResult{TicketID: ticket.ID}, nil
}

// CreateVerificationResult holds the result of verifying a ticket creation.
type CreateVerificationResult struct {
	TicketID string // The verified created ticket ID
}

// VerifyCreate fetches the newly created ticket and confirms it exists with expected title.
func (c *Client) VerifyCreate(ctx context.Context, ticketID string, expectedTitle string) (*CreateVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Could not verify creation of %s", ticketID),
			classErr.Message)
	}

	if ticket.Title != expectedTitle {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Create verification failed — title mismatch on %s", ticketID),
			fmt.Sprintf("Expected '%s', got '%s'", expectedTitle, ticket.Title))
	}

	return &CreateVerificationResult{TicketID: ticket.ID}, nil
}

// ReleaseVerificationResult holds the result of verifying a ticket release.
type ReleaseVerificationResult struct {
	TicketID string // The verified released ticket ID
}

// VerifyRelease confirms that a ticket has been released (assignee cleared).
func (c *Client) VerifyRelease(ctx context.Context, ticketID string) (*ReleaseVerificationResult, *gerr.ClassifiedError) {
	ticket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return nil, classErr
	}

	if ticket.Assignee != "" {
		return nil, gerr.NewVerifyFailedError(
			fmt.Sprintf("Release verification failed: assignee is still '%s' (expected empty)", ticket.Assignee), "")
	}

	return &ReleaseVerificationResult{TicketID: ticket.ID}, nil
}
