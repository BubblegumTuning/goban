// Package models defines all data structures used throughout Goban.
package models

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// SSEConfig holds SSE-related configuration.
type SSEConfig struct {
	BufferSize int `yaml:"buffer_size"`
	Retention  int `yaml:"retention_days"`
}

// Board defines a Kanban board configuration.
type Board struct {
	ID      string   `json:"id" yaml:"id"`
	Title   string   `json:"title" yaml:"title"`
	Columns []string `json:"columns" yaml:"columns"` // Column titles (e.g., "todo", "doing")
	Desc    string   `json:"desc,omitempty" yaml:"desc,omitempty"`
}

// Ticket represents a work item on the board.
type Ticket struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priority"` // low, medium, high, critical
	Assignee    string    `json:"assignee"`
	Column      string    `json:"column"` // Column ID (e.g., "todo-0")
	BoardID     string    `json:"board_id"`
	Labels      []string  `json:"labels,omitempty"`
	DueDate     *string   `json:"due_date,omitempty"` // RFC3339 timestamp (nullable)
	CreatedAt   string    `json:"created_at" db_type:"TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP"`
	UpdatedAt   string    `json:"updated_at" db_type:"TIMESTAMP DEFAULT CURRENT_TIMESTAMP"`
	Subtasks    []Subtask `json:"subtasks,omitempty"`
	Comments    []Comment `json:"comments,omitempty"`
	Archived    bool      `json:"archived"`
	ArchivedAt  *string   `json:"archived_at,omitempty"` // RFC3339 timestamp when archived (nullable)
	ArchivedBy  *int      `json:"archived_by,omitempty"` // ID of admin who force-archived (nullable)
}

// Subtask represents a checklist item within a ticket.
type Subtask struct {
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

// Comment represents a discussion comment on a ticket.
type Comment struct {
	ID        string `json:"id"`
	Who       string `json:"who"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"` // RFC3339 format
}

// CompactTicket is a reduced Ticket representation for list views.
type CompactTicket struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Priority    string   `json:"priority,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	DueDate     *string  `json:"due_date,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Subtasks    int      `json:"subtasks,omitempty"`    // Count of subtasks
	Completed   int      `json:"completed,omitempty"`   // Count of completed subtasks
	Description string   `json:"description,omitempty"` // Only if not truncated
}

// CompactColumn is a reduced Column representation for list views.
type CompactColumn struct {
	ID      string           `json:"id"`
	Title   string           `json:"title"`
	Tickets []*CompactTicket `json:"tickets"`
}

// CompactBoard is a reduced Board representation for list views.
type CompactBoard struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Desc      string           `json:"desc,omitempty"`
	Columns   []*CompactColumn `json:"columns"`
	TicketIDs []string         `json:"ticket_ids,omitempty"` // For diffing
}

// toCompact converts a Ticket to CompactTicket format.
func (t *Ticket) ToCompact(truncateDesc bool) *CompactTicket {
	ct := &CompactTicket{
		ID:       t.ID,
		Title:    t.Title,
		Priority: t.Priority,
		Assignee: t.Assignee,
		DueDate:  t.DueDate,
		Labels:   t.Labels,
	}

	if len(t.Subtasks) > 0 {
		ct.Subtasks = len(t.Subtasks)
		for _, st := range t.Subtasks {
			if st.Completed {
				ct.Completed++
			}
		}
	}

	if truncateDesc {
		desc := t.Description
		if len(desc) > 100 {
			desc = desc[:97] + "..."
		}
		ct.Description = strings.TrimSpace(desc)
	}

	return ct
}

// getCompactLevel determines if response should be compact based on query params.
func GetCompactLevel(c *fiber.Ctx) (bool, bool) {
	compact := c.Query("compact") == "true" || c.Query("c") == "1"
	if !compact && c.Get("X-Compact") == "true" {
		compact = true
	}

	truncateDesc := c.Query("truncate_desc") == "true" || c.Query("td") == "1"
	return compact, truncateDesc
}

// Column represents a runtime column with its tickets.
type Column struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Tickets []*Ticket `json:"tickets,omitempty"` // Pointer to avoid copy on update
}

// BoardState represents a board's current state in memory.
type BoardState struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Desc    string    `json:"desc,omitempty"`
	Columns []*Column `json:"columns"` // Pointer to avoid copy on update
}

// TicketLocation tracks where a ticket is located for O(1) lookups.
type TicketLocation struct {
	Ticket   *Ticket
	BoardID  string
	ColumnID string
}

// SSEEvent represents an event to broadcast via Server-Sent Events.
type SSEEvent struct {
	Type      string    `json:"type"` // create, update, move, delete, archive, unarchive
	TicketID  string    `json:"ticket_id"`
	BoardID   string    `json:"board_id"`
	Payload   fiber.Map `json:"payload"` // Extra context (column, title, etc.)
	Timestamp time.Time `json:"timestamp"`
}

// Subscriber represents a client subscribed to SSE events.
type Subscriber struct {
	ID        int64         `json:"id"`
	BoardID   string        `json:"board_id,omitempty"` // Empty = all boards
	Events    chan SSEEvent `json:"-"`                  // Events for this subscriber
	Done      chan struct{} `json:"-"`                  // Close signal
	Connected time.Time     `json:"connected_at"`
}

// AgentToken represents an API authentication token.
type AgentToken struct {
	ID        int64      `json:"id" db_type:"INTEGER PRIMARY KEY AUTOINCREMENT"`
	AgentName string     `json:"agent_name" db_type:"TEXT NOT NULL UNIQUE"`
	TokenName string     `json:"token_name" db_type:"TEXT NOT NULL"`             // Display name (e.g., "goban-abc123")
	Token     string     `json:"-" db_type:"TEXT NOT NULL"`                      // Full token (not returned in API)
	TokenHash string     `json:"-" db_type:"TEXT NOT NULL UNIQUE"`               // SHA256 hash for validation
	UserID    int64      `json:"user_id" db_type:"INTEGER REFERENCES users(id)"` // Foreign key to users table
	CreatedAt time.Time  `json:"created_at" db_type:"TIMESTAMP DEFAULT CURRENT_TIMESTAMP"`
	LastUsed  *time.Time `json:"last_used,omitempty" db_type:"TIMESTAMP"`
}

// User represents a user/agent with role-based access control.
type User struct {
	ID           int64     `json:"id" db_type:"INTEGER PRIMARY KEY AUTOINCREMENT"`
	Name         string    `json:"name" db_type:"TEXT NOT NULL UNIQUE"`              // e.g., "hermes", "overseer-01"
	Role         string    `json:"role" db_type:"TEXT NOT NULL DEFAULT 'NORMAL_AI'"` // HUMAN_ADMIN, OVERSEER_AI, NORMAL_AI
	PasswordHash string    `json:"-" db_type:"TEXT"`                                 // bcrypt hash (not returned in API)
	CreatedAt    time.Time `json:"created_at" db_type:"TIMESTAMP DEFAULT CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `json:"updated_at" db_type:"TIMESTAMP DEFAULT CURRENT_TIMESTAMP"`
}

// Role constants for RBAC.
const (
	RoleHumanAdmin = "HUMAN_ADMIN" // Full control: create users, set privileges, force anything
	RoleOverseerAI = "OVERSEER_AI" // Can claim/move/release any ticket
	RoleNormalAI   = "NORMAL_AI"   // Can only act on their own tickets
)

// IsAdmin returns true if user has HUMAN_ADMIN role.
func (u *User) IsAdmin() bool {
	return u.Role == RoleHumanAdmin
}

// IsOverseer returns true if user is OVERSEER_AI or HUMAN_ADMIN.
func (u *User) IsOverseer() bool {
	return u.Role == RoleOverseerAI || u.Role == RoleHumanAdmin
}

// CanManageTicket checks if user can manage a ticket based on ownership and role.
func (u *User) CanManageTicket(ticketAssignee string, tokenAgentName string) bool {
	// HUMAN_ADMIN can do anything
	if u.IsAdmin() {
		return true
	}
	// OVERSEER_AI can manage any ticket
	if u.Role == RoleOverseerAI {
		return true
	}
	// NORMAL_AI can only manage their own tickets
	return ticketAssignee == tokenAgentName || ticketAssignee == ""
}

// CanClaim checks if user can claim a ticket.
// HUMAN_ADMIN and OVERSEER_AI can claim any ticket.
// NORMAL_AI can only claim unassigned tickets or their own.
func (u *User) CanClaim(ticketAssignee string, tokenAgentName string) bool {
	if u.IsAdmin() || u.Role == RoleOverseerAI {
		return true
	}
	// NORMAL_AI: can only claim if unassigned or already theirs
	return ticketAssignee == "" || ticketAssignee == tokenAgentName
}

// CanRelease checks if user can release a ticket.
// HUMAN_ADMIN and OVERSEER_AI can release any ticket.
// NORMAL_AI can only release tickets they are assigned to.
func (u *User) CanRelease(ticketAssignee string, tokenAgentName string) bool {
	if u.IsAdmin() || u.Role == RoleOverseerAI {
		return true
	}
	// NORMAL_AI: can only release their own tickets
	return ticketAssignee == tokenAgentName
}

// ActivityLog represents an audit trail entry for ticket state changes.
type ActivityLog struct {
	ID        int64     `json:"id" db_type:"INTEGER PRIMARY KEY AUTOINCREMENT"`
	TicketID  string    `json:"ticket_id" db_type:"TEXT NOT NULL"`
	EventType string    `json:"event_type" db_type:"TEXT NOT NULL"`  // created, claimed, moved, reset, reviewed, completed, archived, cancelled, commented
	Actor     string    `json:"actor" db_type:"TEXT NOT NULL"`       // Username who performed the action
	PrevState *string   `json:"prev_state,omitempty" db_type:"TEXT"` // Previous column/status (nullable)
	NewState  *string   `json:"new_state,omitempty" db_type:"TEXT"`  // New column/status (nullable)
	Metadata  string    `json:"metadata,omitempty" db_type:"TEXT"`   // JSON blob for extra context
	CreatedAt time.Time `json:"created_at" db_type:"TIMESTAMP DEFAULT CURRENT_TIMESTAMP"`
}

// ActivityEventTypes are the standard event types for activity logging.
const (
	ActivityCreated   = "created"
	ActivityClaimed   = "claimed"
	ActivityMoved     = "moved"
	ActivityReset     = "reset"
	ActivityReviewed  = "reviewed"
	ActivityCompleted = "completed"
	ActivityArchived  = "archived"
	ActivityRestored  = "restored"
	ActivityCancelled = "cancelled"
	ActivityCommented = "commented"
)
