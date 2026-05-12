package types

// Board represents a Kanban board
type Board struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TicketStatus represents the status of a ticket
type TicketStatus string

const (
	StatusToDo       TicketStatus = "To Do"
	StatusInProgress TicketStatus = "In Progress"
	StatusDone       TicketStatus = "Done"
)

// Ticket represents a Kanban ticket
type Ticket struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Column      string    `json:"column"` // API uses canonical format: backlog-0/todo-0/inprogress-0/review-0/done-0/cancelled-0
	BoardID     string    `json:"board_id"`
	Assignee    string    `json:"assignee,omitempty"` // API uses assignee for claimed_by
	Priority    string    `json:"priority,omitempty"`
	CreatedAt   string    `json:"created_at,omitempty"`
	UpdatedAt   string    `json:"updated_at,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	Subtasks    []Subtask `json:"subtasks,omitempty"`
	Comments    []Comment `json:"comments,omitempty"`
}

// Subtask represents a checklist item within a ticket
type Subtask struct {
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

// GetStatus returns the TicketStatus based on column (canonical -0 suffix only)
func (t *Ticket) GetStatus() TicketStatus {
	// Server uses canonical suffixed IDs: backlog-0, todo-0, inprogress-0, review-0, done-0, cancelled-0
	switch t.Column {
	case "backlog-0":
		return StatusToDo // Treat backlog as To Do
	case "todo-0":
		return StatusToDo
	case "inprogress-0":
		return StatusInProgress
	case "review-0":
		return StatusInProgress // Treat review as in-progress state
	case "done-0":
		return StatusDone
	case "cancelled-0":
		return StatusDone // Cancelled is a terminal state like Done
	default:
		return StatusToDo
	}
}

// IsClaimed returns true if the ticket has an assignee
func (t *Ticket) IsClaimed() bool {
	return t.Assignee != ""
}

// MatchesColumn returns true if this ticket's Column matches the given base name
// (canonical server format with -0 suffix only, e.g., "todo-0", "inprogress-0")
func (t *Ticket) MatchesColumn(base string) bool {
	col := t.Column
	// Strip canonical -0 suffix for comparison if present
	if len(col) > 2 && col[len(col)-2:] == "-0" {
		col = col[:len(col)-2]
	}
	return col == base
}

// Comment represents a comment on a ticket (matches server model)
type Comment struct {
	ID        string `json:"id"`
	Who       string `json:"who"`       // Server uses "who" for author
	Text      string `json:"text"`      // Server uses "text" for content
	Timestamp string `json:"timestamp"` // Server uses "timestamp" in RFC3339 format
}

// CreateTicketRequest represents the request body for creating a ticket
type CreateTicketRequest struct {
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	Column         string   `json:"column,omitempty"` // API uses "column" field
	BoardID        string   `json:"board_id,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	Parents        []string `json:"parents,omitempty"`
}

// UpdateTicketRequest represents the request body for updating a ticket
type UpdateTicketRequest struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Column      string `json:"column,omitempty"`   // API uses "column" to move tickets
	ClaimedBy   string `json:"assignee,omitempty"` // API uses assignee for claimed_by
	Priority    string `json:"priority,omitempty"`
	Archived    *bool  `json:"archived,omitempty"`
}

// ClaimTicketRequest represents the request body for claiming a ticket
type ClaimTicketRequest struct {
	ClaimedBy string `json:"claimed_by"`
}

// AddCommentRequest represents the request body for adding a comment
type AddCommentRequest struct {
	Who  string `json:"who"`
	Text string `json:"text"`
}
