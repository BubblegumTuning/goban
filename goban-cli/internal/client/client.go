package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"goban-cli/internal/config"
	"goban-cli/internal/types"
)

// Column represents a Kanban column with tickets
type Column struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Tickets []types.Ticket `json:"tickets"`
}

// BoardFull represents a full board with columns and tickets
type BoardFull struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// ClaimRequest is the request body for claiming a ticket (v1.1 API)
type ClaimRequest struct{}

// ClaimResponse is the response from claiming a ticket (v1.1 API)
type ClaimResponse struct {
	Ticket       *types.Ticket `json:"ticket"`
	AutoReleased []string      `json:"auto_released,omitempty"` // IDs of auto-released tickets (matches server)
}

// MoveRequest is the request body for moving a ticket (v1.1 API)
type MoveRequest struct {
	TargetStatus string `json:"target_status"`
	Force        bool   `json:"force,omitempty"`
}

// MoveResponse is the response from moving a ticket (v1.1 API)
// Server returns ticket object directly, not wrapped in {"ticket": {...}}
type MoveResponse types.Ticket

// ReleaseRequest is the request body for releasing a ticket (v1.1 API)
type ReleaseRequest struct{}

// ReleaseResponse is the response from releasing a ticket (v1.1 API)
// Server returns ticket object directly, not wrapped in {"ticket": {...}}
type ReleaseResponse types.Ticket

// ListTicketsRequestParams holds query parameters for list tickets endpoint
type ListTicketsRequestParams struct {
	BoardID string // Filter by board ID (optional)
	View    string // "full" = all columns including DONE/CANCELLED
	Include string // "backlog" = include BACKLOG column
}

// PaginatedTicketsResponse matches the server's paginated response format.
type PaginatedTicketsResponse struct {
	Tickets []*types.Ticket `json:"tickets"`
	Columns []string        `json:"columns"`
	Total   int64           `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
	HasMore bool            `json:"has_more"`
}

// Client wraps HTTP calls to the Goban API
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
	retry      RetryPolicy
}

// New creates a new Goban API client with retry configuration from config.
func New(cfg *config.Config) *Client {
	return &Client{
		baseURL:  cfg.API.BaseURL,
		apiToken: cfg.API.APIToken, // Bearer token for v1.1 API auth
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.API.Timeout) * time.Second,
		},
		retry: NewRetryPolicy(cfg.Retry),
	}
}

// newRequest creates an HTTP request with authentication header
func (c *Client) newRequest(ctx context.Context, method, url string, body interface{}) (*http.Request, error) {
	var buf io.Reader

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		buf = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, buf)
	if err != nil {
		return nil, err
	}

	// Add authentication header for v1.1 API
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// executeWithRetry executes an HTTP request with exponential backoff retry on transient failures.
func (c *Client) executeWithRetry(ctx context.Context, req *http.Request) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < c.retry.maxAttempts; attempt++ {
		if attempt > 0 {
			delay := c.retry.delayForAttempt(attempt - 1)
			select {
			case <-time.After(delay):
				// continue to next attempt
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed (attempt %d/%d): %w", attempt+1, c.retry.maxAttempts, err)
			if IsNetworkError(err) && ctx.Err() == nil {
				continue // Retry network errors
			}
			return nil, lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response: %w", readErr)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			apiErr := NewAPIError(resp.StatusCode, respBody)
			if IsRetryable(resp.StatusCode) && ctx.Err() == nil {
				lastErr = apiErr
				continue // Retry transient server errors and rate limits
			}
			return respBody, apiErr
		}

		return respBody, nil
	}

	// All retries exhausted — return the last error with a retry wrapper
	if lastErr != nil {
		return nil, fmt.Errorf("operation failed after %d attempts: %w", c.retry.maxAttempts, lastErr)
	}
	return nil, fmt.Errorf("unexpected: all retry attempts consumed without error")
}

// ListBoards returns all boards with their tickets
func (c *Client) ListBoards(ctx context.Context) ([]types.Board, error) {
	url := c.baseURL + "/api/boards"

	req, err := c.newRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var boardsFull []BoardFull
	if err := json.Unmarshal(body, &boardsFull); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to simple Board type for display
	var boards []types.Board
	for _, b := range boardsFull {
		boards = append(boards, types.Board{ID: b.ID, Name: b.Name})
	}

	return boards, nil
}

// ListTicketsWithParams returns tickets with v1.1 API query parameters
func (c *Client) ListTicketsWithParams(ctx context.Context, params ListTicketsRequestParams) ([]types.Ticket, error) {
	url := c.baseURL + "/api/v1/tickets"

	// Build query string based on params
	queryParts := []string{}
	if params.BoardID != "" {
		queryParts = append(queryParts, "board_id="+params.BoardID)
	}
	if params.View == "full" {
		queryParts = append(queryParts, "view=full")
	} else if params.Include == "backlog" {
		queryParts = append(queryParts, "include=backlog")
	}

	if len(queryParts) > 0 {
		url += "?" + joinQueryParams(queryParts)
	}

	req, err := c.newRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	// Unmarshal into paginated response structure (server returns wrapper object, not raw array)
	var resp PaginatedTicketsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert []*types.Ticket to []types.Ticket for compatibility with existing callers
	tickets := make([]types.Ticket, 0, len(resp.Tickets))
	for _, t := range resp.Tickets {
		if t != nil {
			tickets = append(tickets, *t)
		}
	}

	return tickets, nil
}

// ListTickets (legacy) returns all tickets for a board by ID using old API
func (c *Client) ListTickets(ctx context.Context, boardID string) ([]types.Ticket, error) {
	url := c.baseURL + "/api/boards"

	req, err := c.newRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var boardsFull []BoardFull
	if err := json.Unmarshal(body, &boardsFull); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Find the requested board and extract all tickets from its columns
	for _, b := range boardsFull {
		if b.ID == boardID {
			var tickets []types.Ticket
			for _, col := range b.Columns {
				// Preserve column ID with -0 suffix for consistency (e.g., "todo-0", "done-0")
				colName := col.ID
				for _, t := range col.Tickets {
					t.BoardID = boardID // Ensure board_id is set
					t.Column = colName  // Extract column from parent Column struct (CompactTicket lacks this field)
					tickets = append(tickets, t)
				}
			}
			return tickets, nil
		}
	}

	return nil, fmt.Errorf("board %s not found", boardID)
}

// GetTicketByID returns a single ticket by ID using the v1 API endpoint (efficient direct lookup)
func (c *Client) GetTicketByID(ctx context.Context, ticketID string) (*types.Ticket, error) {
	url := c.baseURL + "/api/v1/tickets/" + ticketID

	req, err := c.newRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var ticket types.Ticket
	if err := json.Unmarshal(body, &ticket); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ticket, nil
}

// GetTicket (legacy) returns a single ticket by ID from all boards using old /api/boards endpoint
func (c *Client) GetTicket(ctx context.Context, boardID, ticketID string) (*types.Ticket, error) {
	tickets, err := c.ListTickets(ctx, boardID)
	if err != nil {
		return nil, err
	}

	for _, t := range tickets {
		if t.ID == ticketID {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("ticket %s not found on board %s", ticketID, boardID)
}

// CreateTicket creates a new ticket on the specified board
func (c *Client) CreateTicket(ctx context.Context, boardID string, req types.CreateTicketRequest) (*types.Ticket, error) {
	url := c.baseURL + "/api/tickets" // Server uses /api/tickets endpoint

	httpReq, err := c.newRequest(ctx, "POST", url, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	respBody, err := c.executeWithRetry(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var ticket types.Ticket
	if err := json.Unmarshal(respBody, &ticket); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ticket, nil
}

// UpdateTicket updates an existing ticket (PATCH request for partial update)
func (c *Client) UpdateTicket(ctx context.Context, boardID, ticketID string, req types.UpdateTicketRequest) (*types.Ticket, error) {
	url := c.baseURL + "/api/tickets/" + ticketID

	httpReq, err := c.newRequest(ctx, "PATCH", url, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	respBody, err := c.executeWithRetry(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var ticket types.Ticket
	if err := json.Unmarshal(respBody, &ticket); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ticket, nil
}

// MoveTicket moves a ticket using v1.1 API (POST /api/v1/tickets/{id}/move)
func (c *Client) MoveTicket(ctx context.Context, ticketID string, targetStatus string, force bool) (*MoveResponse, error) {
	url := c.baseURL + "/api/v1/tickets/" + ticketID + "/move"

	reqBody := MoveRequest{
		TargetStatus: targetStatus,
		Force:        force,
	}

	req, err := c.newRequest(ctx, "POST", url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var response MoveResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ClaimTicket claims a ticket using v1.1 API (POST /api/v1/tickets/{id}/claim)
func (c *Client) ClaimTicket(ctx context.Context, ticketID string) (*ClaimResponse, error) {
	url := c.baseURL + "/api/v1/tickets/" + ticketID + "/claim"

	reqBody := ClaimRequest{}

	req, err := c.newRequest(ctx, "POST", url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var response ClaimResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ReleaseTicket releases a ticket using v1.1 API (POST /api/v1/tickets/{id}/release)
func (c *Client) ReleaseTicket(ctx context.Context, ticketID string) (*ReleaseResponse, error) {
	url := c.baseURL + "/api/v1/tickets/" + ticketID + "/release"

	reqBody := ReleaseRequest{}

	req, err := c.newRequest(ctx, "POST", url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var response ReleaseResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// AddComment adds a comment to a ticket via description append (simplest approach for this API)
func (c *Client) AddComment(ctx context.Context, boardID, ticketID string, req types.AddCommentRequest) (*types.Comment, error) {
	url := c.baseURL + "/api/tickets/" + ticketID + "/comments"

	httpReq, err := c.newRequest(ctx, "POST", url, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	respBody, err := c.executeWithRetry(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	// Server wraps comment in {"status": "added", "comment": {...}}
	var resp struct {
		Status  string        `json:"status"`
		Comment types.Comment `json:"comment"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Comment, nil
}

// ListComments returns all comments for a ticket by fetching the full ticket object
// (Server does not have a dedicated GET /comments endpoint; comments are embedded in ticket)
func (c *Client) ListComments(ctx context.Context, boardID, ticketID string) ([]types.Comment, error) {
	ticket, err := c.GetTicketByID(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticket: %w", err)
	}

	// Comments are embedded in the ticket object
	return ticket.Comments, nil
}

// DeleteTicket deletes a ticket (DELETE /api/tickets/:id)
func (c *Client) DeleteTicket(ctx context.Context, boardID, ticketID string) error {
	url := c.baseURL + "/api/tickets/" + ticketID

	req, err := c.newRequest(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	_, err = c.executeWithRetry(ctx, req)
	if err != nil {
		return err
	}

	return nil
}

// joinQueryParams joins query parameters with &
func joinQueryParams(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "&"
		}
		result += p
	}
	return result
}

