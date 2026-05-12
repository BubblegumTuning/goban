// Package store provides database abstraction for Goban persistence.
package store

import (
	"goban/models"
)

// Pagination holds pagination parameters for queries.
type Pagination struct {
	Limit  int // Max results per page (0 = use default of 50)
	Offset int // Number of results to skip
}

// TicketStore interface defines all database operations.
type TicketStore interface {
	// Initialize the database connection and create tables if needed
	Init() error

	// Close the database connection
	Close() error

	// Transaction support for atomic multi-step operations
	BeginTx() (Tx, error) // Begin a new transaction

	// CRUD operations for tickets
	CreateTicket(t *models.Ticket) error
	CreateOrGetTicket(t *models.Ticket) (*models.Ticket, error) // Idempotent: returns existing ticket if idempotency_key matches
	GetAllTickets() ([]*models.Ticket, error)
	GetPaginatedTickets(p Pagination) ([]*models.Ticket, int64, error) // (tickets, totalCount, err)
	GetTicket(id string) (*models.Ticket, error)
	UpdateTicket(t *models.Ticket) error
	DeleteTicket(id string) error

	// Query operations for ticket filtering
	GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error)

	// GetTicketsWithFilter returns tickets filtered by allowed column prefixes with pagination.
	// Example: allowedColumns=["todo"] returns only tickets in todo-* columns.
	GetTicketsWithFilter(allowedColumns []string, p Pagination) ([]*models.Ticket, int64, error)

	// Operations for archived tickets
	GetArchivedTickets(boardID string) ([]*models.Ticket, error)
	GetAllArchivedTickets() ([]*models.Ticket, error) // All archived regardless of board

	// Force archive operations (admin only)
	UnarchiveTicket(ticketID string) error
	ArchiveTicket(ticketID string, adminID int64) error
	ArchiveTicketsBulk(ticketIDs []string, adminID int64) error
	GetArchivedByAdmin(adminID int64) ([]*models.Ticket, error)

	// Token operations for AI agent authentication
	CreateToken(agentName, tokenHash string) (int64, error)
	CreateTokenWithUser(userID int64, agentName, tokenHash string) (int64, error)
	ValidateToken(tokenHash string) (*models.AgentToken, error)
	UpdateTokenLastUsed(tokenHash string) error
	DeleteToken(agentName string) (int64, error)
	ListTokens() ([]models.AgentToken, error)

	// User operations for role-based access control
	CreateUser(name, role string) (int64, error)
	CreateUserWithPassword(name, password, role string) (int64, error) // With bcrypt hashing
	GetUserByID(id int64) (*models.User, error)
	GetUserByName(name string) (*models.User, error) // Returns user with password hash for auth
	UpdateUserRole(id int64, role string) error
	UpdateUserPassword(id int64, newPassword string) error // With bcrypt hashing
	DeleteUser(id int64) error
	ListUsers() ([]models.User, error)

	// Ticket query for user operations (to check active tickets before deletion)
	GetTicketsByAssignee(assigneeName string) ([]*models.Ticket, error)

	// Token-User relationship operations
	UpdateTokenUserID(tokenHash string, userID int64) error
	GetUserByToken(tokenHash string) (*models.User, error)

	// TaskLink operations for parent/child dependencies
	AddTaskLink(parentID, childID string) error
	RemoveTaskLink(parentID, childID string) error
	GetTaskLinks(ticketID string) (parents []string, children []string, err error)

	// TicketRun operations for per-attempt tracking (ticket-2b0f57e014)
	CreateRun(r *models.TicketRun) (*models.TicketRun, error)
	GetRuns(ticketID string) ([]*models.TicketRun, error)
	UpdateRun(runID int64, outcome string, summary string, metadata string) error
	GetActiveRun(ticketID string) (*models.TicketRun, error)

	// Activity log operations for audit trail
	CreateActivityLog(log *models.ActivityLog) (int64, error)
	GetActivityLogs(ticketID string, limit int) ([]*models.ActivityLog, error)
}

// Tx represents an active database transaction with scoped operations.
type Tx interface {
	Commit() error   // Commit the transaction
	Rollback() error // Rollback the transaction

	// Transaction-scoped CRUD operations (same as TicketStore but within tx context)
	GetTicket(id string) (*models.Ticket, error)
	UpdateTicket(t *models.Ticket) error
	GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error)

	// Activity log operations for transaction-scoped audit trail
	CreateActivityLog(logEntry *models.ActivityLog) (int64, error)
}
