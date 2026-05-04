// Package store provides database abstraction for Goban persistence.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"goban/config"
	"goban/models"
)

// PostgresStore implements TicketStore using PostgreSQL backend.
type PostgresStore struct {
	db     *sql.DB
	config config.Config
}

// Init initializes the PostgreSQL database connection and creates tables if needed.
func (s *PostgresStore) Init() error {
	// Connection string with SSL mode disabled for local connections
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		s.config.DBHost, s.config.DBPort, s.config.DBUser, s.config.DBPassword, s.config.DBName)

	var err error
	s.db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Test the connection with a timeout
	err = s.db.Ping()
	if err != nil {
		return fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	log.Printf("Using PostgreSQL backend at %s:%d/%s", s.config.DBHost, s.config.DBPort, s.config.DBName)

	// Create tables if they don't exist - note: using 'column' for SQLite compatibility
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tickets (
			id VARCHAR(64) PRIMARY KEY,
			board_id VARCHAR(64),
			title VARCHAR(256) NOT NULL,
			description TEXT,
			"column" VARCHAR(32),
			assignee VARCHAR(128),
			priority VARCHAR(16),
			labels JSONB DEFAULT '[]'::jsonb,
			due_date TIMESTAMP WITHOUT TIME ZONE,
			subtasks JSONB DEFAULT '[]'::jsonb,
			comments JSONB DEFAULT '[]'::jsonb,
			created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
			archived BOOLEAN DEFAULT FALSE,
			archived_at TIMESTAMP WITHOUT TIME ZONE,
			archived_by INTEGER REFERENCES users(id), -- Admin who force-archived the ticket
			CHECK (priority IS NULL OR priority = '' OR priority IN ('low', 'medium', 'high', 'critical'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create tickets table: %w", err)
	}

	// Migrate existing priority values from capitalized to lowercase canonical form.
	_, _ = s.db.Exec(`UPDATE tickets SET priority = LOWER(priority) WHERE priority IN ('Low', 'Medium', 'High', 'Critical', 'URGENT', 'Urgent')`)

	// Drop old constraint if it exists (from previous schema with capitalized values), then recreate.
	_, _ = s.db.Exec(`ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_priority_check`)
	_, err = s.db.Exec(`ALTER TABLE tickets ADD CONSTRAINT tickets_priority_check CHECK (priority IS NULL OR priority = '' OR priority IN ('low', 'medium', 'high', 'critical'))`)
	if err != nil {
		log.Printf("Warning: failed to update priority constraint: %v", err)
	}

	// Create indexes for better query performance
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_tickets_board_id ON tickets(board_id);
		CREATE INDEX IF NOT EXISTS idx_tickets_column ON tickets("column");
		CREATE INDEX IF NOT EXISTS idx_tickets_archived ON tickets(archived) WHERE archived = true;
	`)
	if err != nil {
		log.Printf("Warning: could not create indexes: %v", err)
	}

	// Create tokens table for AI agent authentication with user reference
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(128) UNIQUE NOT NULL,
			role VARCHAR(32) NOT NULL DEFAULT 'NORMAL_AI',
			password_hash TEXT,
			created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Create tokens table for AI agent authentication
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS agent_tokens (
		id SERIAL PRIMARY KEY,
		agent_name VARCHAR(128) UNIQUE NOT NULL,
		token_name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		user_id INTEGER REFERENCES users(id),
		created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
		last_used TIMESTAMP WITHOUT TIME ZONE
	)
`)
	if err != nil {
		return fmt.Errorf("failed to create tokens table: %w", err)
	}

	// Create activity_logs table for audit trail
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS activity_logs (
			id SERIAL PRIMARY KEY,
			ticket_id VARCHAR(64) NOT NULL,
			event_type VARCHAR(32) NOT NULL,
			actor VARCHAR(128) NOT NULL,
			prev_state VARCHAR(32),
			new_state VARCHAR(32),
			metadata TEXT,
			created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create activity_logs table: %w", err)
	}

	// Create indexes for activity_logs
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_activity_ticket ON activity_logs(ticket_id);
		CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_logs(created_at);
	`)
	if err != nil {
		log.Printf("Warning: could not create activity_logs indexes: %v", err)
	}

	log.Printf("PostgreSQL database initialized at %s:%d/%s", s.config.DBHost, s.config.DBPort, s.config.DBName)
	return nil
}

// Close closes the PostgreSQL database connection.
func (s *PostgresStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// BeginTx starts a new transaction and returns a Tx interface for scoped operations.
func (s *PostgresStore) BeginTx() (Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &postgresTx{tx: tx, store: s}, nil
}

// postgresTx implements Tx interface for PostgreSQL transactions.
type postgresTx struct {
	tx    *sql.Tx
	store *PostgresStore // Reference to parent store for helper methods
}

// Commit commits the transaction.
func (t *postgresTx) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction.
func (t *postgresTx) Rollback() error {
	return t.tx.Rollback()
}

// GetTicket retrieves a ticket within the transaction context.
func (t *postgresTx) GetTicket(id string) (*models.Ticket, error) {
	row := t.tx.QueryRow(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE id = $1
	`, id)

	var tk models.Ticket
	var labelsJSON, subtasksJSON, commentsJSON []byte
	var archived bool
	var dueDate sql.NullTime
	var archivedAt sql.NullTime

	var archivedBy sql.NullInt64
	err := row.Scan(&tk.ID, &tk.Title, &tk.Description, &tk.Priority,
		&tk.Assignee, &tk.Column, &tk.BoardID,
		&labelsJSON, &dueDate,
		&subtasksJSON, &commentsJSON, &archived, &archivedAt, &archivedBy,
		&tk.CreatedAt, &tk.UpdatedAt)
	if err != nil {
		return nil, err
	}

	tk.Archived = archived
	if dueDate.Valid {
		dueDateString := dueDate.Time.Format(time.RFC3339)
		tk.DueDate = &dueDateString
	}
	if archivedAt.Valid {
		archivedTimeString := archivedAt.Time.Format(time.RFC3339)
		tk.ArchivedAt = &archivedTimeString
	}
	if archivedBy.Valid {
		archivedByID := int(archivedBy.Int64)
		tk.ArchivedBy = &archivedByID
	}

	if err := json.Unmarshal(labelsJSON, &tk.Labels); err != nil {
		log.Printf("Warning: Failed to unmarshal labels for ticket %s: %v", tk.ID, err)
	}
	if err := json.Unmarshal(subtasksJSON, &tk.Subtasks); err != nil {
		log.Printf("Warning: Failed to unmarshal subtasks for ticket %s: %v", tk.ID, err)
	}
	if err := json.Unmarshal(commentsJSON, &tk.Comments); err != nil {
		log.Printf("Warning: Failed to unmarshal comments for ticket %s: %v", tk.ID, err)
	}

	return &tk, nil
}

// UpdateTicket updates a ticket within the transaction context.
func (t *postgresTx) UpdateTicket(tk *models.Ticket) error {
	labelsJSON, err := safeMarshal(tk.Labels, "labels")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	subtasksJSON, err := safeMarshal(tk.Subtasks, "subtasks")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	commentsJSON, err := safeMarshal(tk.Comments, "comments")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}

	var archived bool = tk.Archived
	var dueDate sql.NullTime
	if tk.DueDate != nil {
		parsed, err := time.Parse(time.RFC3339, *tk.DueDate)
		if err != nil {
			return fmt.Errorf("UpdateTicket parse due_date: %w", err)
		}
		dueDate = sql.NullTime{Time: parsed, Valid: true}
	}

	var archivedAt sql.NullTime
	if tk.ArchivedAt != nil {
		parsed, err := time.Parse(time.RFC3339, *tk.ArchivedAt)
		if err != nil {
			return fmt.Errorf("UpdateTicket parse archived_at: %w", err)
		}
		archivedAt = sql.NullTime{Time: parsed, Valid: true}
	}

	_, err = t.tx.Exec(`
		UPDATE tickets SET 
			title=$1, description=$2, priority=$3, assignee=$4, "column"=$5, board_id=$6,
			labels=$7, due_date=$8, subtasks=$9, comments=$10, archived=$11,
			archived_at=$12, updated_at=$13
		WHERE id=$14
	`, tk.Title, tk.Description, tk.Priority, tk.Assignee, tk.Column, tk.BoardID,
		labelsJSON, dueDate, subtasksJSON, commentsJSON, archived,
		archivedAt, tk.UpdatedAt, tk.ID)
	if err != nil {
		return fmt.Errorf("UpdateTicket exec: %w", err)
	}

	return nil
}

// GetTicketsByColumnAndAssignee retrieves tickets matching column prefix and assignee within transaction.
func (t *postgresTx) GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error) {
	rows, err := t.tx.Query(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets 
		WHERE "column" LIKE $1 AND assignee = $2 AND archived = false
		ORDER BY created_at DESC
	`, columnPrefix+"%", assignee)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		tk := t.store.scanTicket(rows)
		if tk != nil {
			tickets = append(tickets, tk)
		}
	}

	return tickets, rows.Err()
}

// CreateActivityLog inserts an activity log entry within the transaction.
func (t *postgresTx) CreateActivityLog(logEntry *models.ActivityLog) (int64, error) {
	var metadataJSON string = ""
	if logEntry.Metadata != "" {
		metadataJSON = logEntry.Metadata
	}

	err := t.tx.QueryRow(`
		INSERT INTO activity_logs (ticket_id, event_type, actor, prev_state, new_state, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, logEntry.TicketID, logEntry.EventType, logEntry.Actor,
		logEntry.PrevState, logEntry.NewState, metadataJSON).Scan(&logEntry.ID)
	if err != nil {
		return 0, fmt.Errorf("CreateActivityLog: %w", err)
	}

	return logEntry.ID, nil
}

// CreateTicket inserts a new ticket within a transaction.
func (s *PostgresStore) CreateTicket(t *models.Ticket) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("CreateTicket begin tx: %w", err)
	}

	labelsJSON, err := safeMarshal(t.Labels, "labels")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("CreateTicket marshal labels: %w", err)
	}
	subtasksJSON, err := safeMarshal(t.Subtasks, "subtasks")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("CreateTicket marshal subtasks: %w", err)
	}
	commentsJSON, err := safeMarshal(t.Comments, "comments")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("CreateTicket marshal comments: %w", err)
	}

	var archived bool = t.Archived
	var dueDate sql.NullTime
	if t.DueDate != nil {
		parsed, err := time.Parse(time.RFC3339, *t.DueDate)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("CreateTicket parse due_date: %w", err)
		}
		dueDate = sql.NullTime{Time: parsed, Valid: true}
	}

	_, err = tx.Exec(`
		INSERT INTO tickets (id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, t.ID, t.Title, t.Description, t.Priority, t.Assignee, t.Column, t.BoardID,
		labelsJSON, dueDate, subtasksJSON, commentsJSON, archived, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("CreateTicket exec: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("CreateTicket commit: %w", err)
	}

	return nil
}

// GetTicket retrieves a single ticket by ID.
func (s *PostgresStore) GetTicket(id string) (*models.Ticket, error) {
	row := s.db.QueryRow(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE id = $1
	`, id)

	var t models.Ticket
	var labelsJSON, subtasksJSON, commentsJSON []byte
	var archived bool
	var dueDate sql.NullTime
	var archivedAt sql.NullTime

	var archivedBy sql.NullInt64
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Priority,
		&t.Assignee, &t.Column, &t.BoardID,
		&labelsJSON, &dueDate,
		&subtasksJSON, &commentsJSON, &archived, &archivedAt, &archivedBy,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	t.Archived = archived
	if dueDate.Valid {
		dueDateString := dueDate.Time.Format(time.RFC3339)
		t.DueDate = &dueDateString
	}
	if archivedAt.Valid {
		archivedTimeString := archivedAt.Time.Format(time.RFC3339)
		t.ArchivedAt = &archivedTimeString
	}
	if archivedBy.Valid {
		archivedByID := int(archivedBy.Int64)
		t.ArchivedBy = &archivedByID
	}

	if err := json.Unmarshal(labelsJSON, &t.Labels); err != nil {
		log.Printf("Warning: Failed to unmarshal labels for ticket %s: %v", t.ID, err)
	}
	if err := json.Unmarshal(subtasksJSON, &t.Subtasks); err != nil {
		log.Printf("Warning: Failed to unmarshal subtasks for ticket %s: %v", t.ID, err)
	}
	if err := json.Unmarshal(commentsJSON, &t.Comments); err != nil {
		log.Printf("Warning: Failed to unmarshal comments for ticket %s: %v", t.ID, err)
	}

	return &t, nil
}

// GetAllTickets retrieves all active (non-archived) tickets.
func (s *PostgresStore) GetAllTickets() ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = false
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// GetPaginatedTickets retrieves paginated active tickets with total count.
func (s *PostgresStore) GetPaginatedTickets(p Pagination) ([]*models.Ticket, int64, error) {
	// Apply defaults
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50 // default 50, max 1000
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	// Get total count first
	var totalCount int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM tickets WHERE archived = false").Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = false
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, totalCount, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, totalCount, rows.Err()
}

// UpdateTicket updates an existing ticket within a transaction.
func (s *PostgresStore) UpdateTicket(t *models.Ticket) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("UpdateTicket begin tx: %w", err)
	}

	labelsJSON, err := safeMarshal(t.Labels, "labels")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	subtasksJSON, err := safeMarshal(t.Subtasks, "subtasks")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	commentsJSON, err := safeMarshal(t.Comments, "comments")
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("UpdateTicket: %w", err)
	}

	var archived bool = t.Archived
	var dueDate sql.NullTime
	if t.DueDate != nil {
		parsed, err := time.Parse(time.RFC3339, *t.DueDate)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("UpdateTicket parse due_date: %w", err)
		}
		dueDate = sql.NullTime{Time: parsed, Valid: true}
	}

	var archivedAt sql.NullTime
	if t.ArchivedAt != nil {
		parsed, err := time.Parse(time.RFC3339, *t.ArchivedAt)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("UpdateTicket parse archived_at: %w", err)
		}
		archivedAt = sql.NullTime{Time: parsed, Valid: true}
	}

	_, err = tx.Exec(`
		UPDATE tickets SET 
			title=$1, description=$2, priority=$3, assignee=$4, "column"=$5, board_id=$6,
			labels=$7, due_date=$8, subtasks=$9, comments=$10, archived=$11,
			archived_at=$12, updated_at=$13
		WHERE id=$14
	`, t.Title, t.Description, t.Priority, t.Assignee, t.Column, t.BoardID,
		labelsJSON, dueDate, subtasksJSON, commentsJSON, archived,
		archivedAt, t.UpdatedAt, t.ID)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("UpdateTicket exec: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("UpdateTicket commit: %w", err)
	}

	return nil
}

// DeleteTicket removes a ticket by ID within a transaction.
func (s *PostgresStore) DeleteTicket(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("DeleteTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec("DELETE FROM tickets WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("DeleteTicket exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

// GetArchivedTickets retrieves archived tickets for a specific board.
func (s *PostgresStore) GetArchivedTickets(boardID string) ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = true AND board_id = $1
		ORDER BY archived_at DESC
	`, boardID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// ArchiveTicket force-archives a single ticket (admin operation).
func (s *PostgresStore) ArchiveTicket(ticketID string, adminID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("ArchiveTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`
		UPDATE tickets SET archived = true, archived_at = NOW(), 
		archived_by = $1, updated_at = NOW()
		WHERE id = $2`, adminID, ticketID)
	if err != nil {
		return fmt.Errorf("ArchiveTicket exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	return tx.Commit()
}

// ArchiveTicketsBulk force-archives multiple tickets (admin operation).
func (s *PostgresStore) ArchiveTicketsBulk(ticketIDs []string, adminID int64) error {
	if len(ticketIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("ArchiveTicketsBulk begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Build placeholder string for IN clause ($1, $2, etc.)
	placeholders := make([]string, len(ticketIDs))
	for i := range ticketIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2) // Start from $2 since $1 is adminID
	}
	inClause := strings.Join(placeholders, ", ")

	args := make([]interface{}, len(ticketIDs)+1)
	args[0] = adminID
	for i, id := range ticketIDs {
		args[i+1] = id
	}

	query := fmt.Sprintf(`
		UPDATE tickets SET archived = true, archived_at = NOW(),
		archived_by = $1, updated_at = NOW()
		WHERE id IN (%s)`, inClause)

	result, err := tx.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("ArchiveTicketsBulk exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[STORE] Archived %d tickets by admin %d", rowsAffected, adminID)

	return tx.Commit()
}

// GetArchivedByAdmin retrieves all tickets archived by a specific admin.
func (s *PostgresStore) GetArchivedByAdmin(adminID int64) ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, 
		 created_at, updated_at
		FROM tickets WHERE archived = true AND archived_by = $1
		ORDER BY archived_at DESC`, adminID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// GetAllArchivedTickets retrieves all archived tickets regardless of board.
func (s *PostgresStore) GetAllArchivedTickets() ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, "column", board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = true
		ORDER BY archived_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// UnarchiveTicket restores a previously archived ticket back to active status.
func (s *PostgresStore) UnarchiveTicket(ticketID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("UnarchiveTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`
		UPDATE tickets SET archived = false, archived_at = NULL, 
		archived_by = NULL, updated_at = NOW()
		WHERE id = $1`, ticketID)
	if err != nil {
		return fmt.Errorf("UnarchiveTicket exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	return tx.Commit()
}

// CreateToken inserts a new API token.
func (s *PostgresStore) CreateToken(agentName, tokenHash string) (int64, error) {
	tokenName := fmt.Sprintf("goban-%s", tokenHash[:8])

	result, err := s.db.Exec(`
		INSERT INTO agent_tokens (agent_name, token_name, token_hash) VALUES ($1, $2, $3)
	`, agentName, tokenName, tokenHash)
	if err != nil {
		return 0, err
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// CreateTokenWithUser creates a new API token linked to an existing user.
func (s *PostgresStore) CreateTokenWithUser(userID int64, agentName, tokenHash string) (int64, error) {
	tokenName := fmt.Sprintf("goban-%s", tokenHash[:8])

	result, err := s.db.Exec(`
		INSERT INTO agent_tokens (agent_name, token_name, token_hash, user_id) VALUES ($1, $2, $3, $4)
	`, agentName, tokenName, tokenHash, userID)
	if err != nil {
		return 0, fmt.Errorf("CreateTokenWithUser exec: %w", err)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// ValidateToken retrieves a token by its hash.
func (s *PostgresStore) ValidateToken(tokenHash string) (*models.AgentToken, error) {
	row := s.db.QueryRow(`
		SELECT id, agent_name, token_name, token_hash, user_id, created_at, last_used
		FROM agent_tokens WHERE token_hash = $1
	`, tokenHash)

	var t models.AgentToken
	var createdAt time.Time
	var lastUsed sql.NullTime

	err := row.Scan(&t.ID, &t.AgentName, &t.TokenName, &t.TokenHash, &t.UserID, &createdAt, &lastUsed)
	if err != nil {
		return nil, err
	}

	t.CreatedAt = createdAt
	if lastUsed.Valid {
		t.LastUsed = &lastUsed.Time
	}

	return &t, nil
}

// UpdateTokenLastUsed updates the last_used timestamp.
func (s *PostgresStore) UpdateTokenLastUsed(tokenHash string) error {
	_, err := s.db.Exec(`
		UPDATE agent_tokens SET last_used = CURRENT_TIMESTAMP WHERE token_hash = $1
	`, tokenHash)
	return err
}

// DeleteToken removes a token by agent name.
func (s *PostgresStore) DeleteToken(agentName string) (int64, error) {
	result, err := s.db.Exec("DELETE FROM agent_tokens WHERE agent_name = $1", agentName)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListTokens returns all tokens.
func (s *PostgresStore) ListTokens() ([]models.AgentToken, error) {
	rows, err := s.db.Query(`
		SELECT id, agent_name, token_name, token_hash, created_at, last_used
		FROM agent_tokens ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.AgentToken
	for rows.Next() {
		var t models.AgentToken
		var createdAt time.Time
		var lastUsed sql.NullTime

		err := rows.Scan(&t.ID, &t.AgentName, &t.TokenName, &t.TokenHash, &createdAt, &lastUsed)
		if err != nil {
			return nil, err
		}

		t.CreatedAt = createdAt
		if lastUsed.Valid {
			t.LastUsed = &lastUsed.Time
		}

		tokens = append(tokens, t)
	}

	return tokens, rows.Err()
}

// scanTicket scans a row into a Ticket struct.
func (s *PostgresStore) scanTicket(rows *sql.Rows) *models.Ticket {
	var t models.Ticket
	var labelsJSON, subtasksJSON, commentsJSON []byte
	var archived bool
	var dueDate sql.NullTime
	var archivedAt sql.NullTime

	var archivedBy sql.NullInt64
	err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Priority,
		&t.Assignee, &t.Column, &t.BoardID,
		&labelsJSON, &dueDate,
		&subtasksJSON, &commentsJSON, &archived, &archivedAt, &archivedBy,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		log.Printf("Warning: Failed to scan ticket row: %v", err)
		return nil
	}

	t.Archived = archived
	if dueDate.Valid {
		dueDateString := dueDate.Time.Format(time.RFC3339)
		t.DueDate = &dueDateString
	}
	if archivedAt.Valid {
		archivedTimeString := archivedAt.Time.Format(time.RFC3339)
		t.ArchivedAt = &archivedTimeString
	}
	if archivedBy.Valid {
		archivedByID := int(archivedBy.Int64)
		t.ArchivedBy = &archivedByID
	}

	if err := json.Unmarshal(labelsJSON, &t.Labels); err != nil {
		log.Printf("Warning: Failed to unmarshal labels for ticket %s: %v", t.ID, err)
	}
	if err := json.Unmarshal(subtasksJSON, &t.Subtasks); err != nil {
		log.Printf("Warning: Failed to unmarshal subtasks for ticket %s: %v", t.ID, err)
	}
	if err := json.Unmarshal(commentsJSON, &t.Comments); err != nil {
		log.Printf("Warning: Failed to unmarshal comments for ticket %s: %v", t.ID, err)
	}

	return &t
}

// ========== USER CRUD OPERATIONS ==========

// CreateUser inserts a new user.
func (s *PostgresStore) CreateUser(name string, role string) (int64, error) {
	// Validate role
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI && role != models.RoleNormalAI {
		return 0, fmt.Errorf("invalid role: %s (must be HUMAN_ADMIN, OVERSEER_AI, or NORMAL_AI)", role)
	}

	// Use RETURNING clause instead of LastInsertId() which is not supported by PostgreSQL driver
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO users (name, role, created_at, updated_at) 
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id`, name, role).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreateUser: %w", err)
	}

	return id, nil
}

// CreateUserWithPassword creates a new user with password hashing.
func (s *PostgresStore) CreateUserWithPassword(name, password, role string) (int64, error) {
	// Validate role
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI && role != models.RoleNormalAI {
		return 0, fmt.Errorf("invalid role: %s", role)
	}

	// Hash password with bcrypt
	passwordHash, err := HashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	var id int64
	err = s.db.QueryRow(`
		INSERT INTO users (name, role, password_hash) VALUES ($1, $2, $3) RETURNING id
	`, name, role, passwordHash).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreateUserWithPassword exec: %w", err)
	}

	return id, nil
}

// UpdateUserRole updates a user's role.
func (s *PostgresStore) UpdateUserRole(id int64, role string) error {
	// Validate role
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI && role != models.RoleNormalAI {
		return fmt.Errorf("invalid role: %s", role)
	}

	result, err := s.db.Exec(`
		UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2
	`, role, id)
	if err != nil {
		return fmt.Errorf("UpdateUserRole exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// DeleteUser removes a user by ID.
func (s *PostgresStore) DeleteUser(id int64) error {
	result, err := s.db.Exec("DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("DeleteUser exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// UpdateUserPassword updates a user's password with bcrypt hashing.
func (s *PostgresStore) UpdateUserPassword(id int64, newPassword string) error {
	// Hash the new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(`
		UPDATE users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("UpdateUserPassword exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetUserByID retrieves a user by ID.
func (s *PostgresStore) GetUserByID(id int64) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, name, role, created_at, updated_at 
		FROM users WHERE id = $1
	`, id)

	var u models.User
	var createdAt, updatedAt time.Time

	err := row.Scan(&u.ID, &u.Name, &u.Role, &createdAt, &updatedAt)
	if err != nil {
		return nil, err // caller should handle sql.ErrNoRows
	}

	u.CreatedAt = createdAt
	u.UpdatedAt = updatedAt

	return &u, nil
}

// GetUser retrieves a user by ID (alias for backward compatibility).
func (s *PostgresStore) GetUser(id int64) (*models.User, error) {
	return s.GetUserByID(id)
}

// GetUserByName retrieves a user by name.
func (s *PostgresStore) GetUserByName(name string) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, name, role, password_hash, created_at, updated_at 
		FROM users WHERE name = $1
	`, name)

	var u models.User
	var createdAt, updatedAt time.Time
	var passwordHash sql.NullString

	err := row.Scan(&u.ID, &u.Name, &u.Role, &passwordHash, &createdAt, &updatedAt)
	if err != nil {
		return nil, err // caller should handle sql.ErrNoRows
	}

	u.PasswordHash = passwordHash.String
	u.CreatedAt = createdAt
	u.UpdatedAt = updatedAt

	return &u, nil
}

// ListUsers retrieves all users ordered by name.
func (s *PostgresStore) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`
		SELECT id, name, role, created_at, updated_at 
		FROM users ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("ListUsers query: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var createdAt, updatedAt time.Time

		err := rows.Scan(&u.ID, &u.Name, &u.Role, &createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("ListUsers scan: %w", err)
		}

		u.CreatedAt = createdAt
		u.UpdatedAt = updatedAt
		users = append(users, u)
	}

	return users, rows.Err()
}

// GetAllUsers retrieves all users (alias for backward compatibility).
func (s *PostgresStore) GetAllUsers() ([]models.User, error) {
	return s.ListUsers()
}

// GetUserByToken retrieves a user by agent token hash.
func (s *PostgresStore) GetUserByToken(tokenHash string) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT u.id, u.name, u.role, u.created_at, u.updated_at 
		FROM users u
		INNER JOIN agent_tokens at ON u.id = at.user_id
		WHERE at.token_hash = $1
	`, tokenHash)

	var u models.User
	var createdAt, updatedAt time.Time

	err := row.Scan(&u.ID, &u.Name, &u.Role, &createdAt, &updatedAt)
	if err != nil {
		return nil, err // caller should handle sql.ErrNoRows
	}

	u.CreatedAt = createdAt
	u.UpdatedAt = updatedAt

	return &u, nil
}

// UpdateTokenUserID links a token to a user.
func (s *PostgresStore) UpdateTokenUserID(tokenHash string, userID int64) error {
	_, err := s.db.Exec(`
		UPDATE agent_tokens SET user_id = $1 WHERE token_hash = $2
	`, userID, tokenHash)
	if err != nil {
		return fmt.Errorf("UpdateTokenUserID exec: %w", err)
	}

	return nil
}

// GetTicketsByColumnAndAssignee returns tickets matching the column prefix and assignee.
func (s *PostgresStore) GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error) {
	var query string
	var args []interface{}
	argPos := 1

	if columnPrefix == "" && assignee == "" {
		return s.GetAllTickets()
	}

	query = "SELECT id, title, description, priority, assignee, \"column\", board_id, labels, due_date, subtasks, comments, archived, archived_at, archived_by, created_at, updated_at FROM tickets WHERE archived = false"

	if columnPrefix != "" {
		query += fmt.Sprintf(" AND \"column\" LIKE $%d", argPos)
		args = append(args, columnPrefix+"%")
		argPos++
	}
	if assignee != "" {
		query += fmt.Sprintf(" AND assignee = $%d", argPos)
		args = append(args, assignee)
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Query error: %v", err)
		}
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// GetTicketsWithFilter returns tickets filtered by allowed column prefixes with pagination.
func (s *PostgresStore) GetTicketsWithFilter(allowedColumns []string, p Pagination) ([]*models.Ticket, int64, error) {
	// Apply defaults
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	var whereClause string
	var args []interface{}
	argPos := 1

	if len(allowedColumns) > 0 {
		conditions := make([]string, 0, len(allowedColumns))
		for range allowedColumns {
			conditions = append(conditions, fmt.Sprintf("\"column\" LIKE $%d", argPos))
			args = append(args, allowedColumns[len(conditions)-1]+"%")
			argPos++
		}
		whereClause = " WHERE archived = false AND (" + strings.Join(conditions, " OR ") + ")"
	} else {
		whereClause = " WHERE archived = false"
	}

	var totalCount int64
	countQuery := "SELECT COUNT(*) FROM tickets" + whereClause
	err := s.db.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Count query error: %v", err)
		}
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}

	query := "SELECT id, title, description, priority, assignee, \"column\", board_id, " +
		"labels, due_date, subtasks, comments, archived, archived_at, archived_by," +
		"created_at, updated_at FROM tickets" + whereClause +
		" ORDER BY created_at DESC LIMIT $" + strconv.Itoa(argPos) + " OFFSET $" + strconv.Itoa(argPos+1)

	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Query error: %v", err)
		}
		return nil, totalCount, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, totalCount, rows.Err()
}

// GetTicketsByAssignee returns all non-archived tickets assigned to a specific user.
func (s *PostgresStore) GetTicketsByAssignee(assigneeName string) ([]*models.Ticket, error) {
	query := `
		SELECT id, title, description, priority, assignee, column, board_id, 
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets 
		WHERE assignee = $1 AND archived = false
		ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query, assigneeName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	tickets := make([]*models.Ticket, 0)
	for rows.Next() {
		t := s.scanTicket(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// CreateActivityLog inserts a new activity log entry.
func (s *PostgresStore) CreateActivityLog(logEntry *models.ActivityLog) (int64, error) {
	var metadataJSON string = ""
	if logEntry.Metadata != "" {
		metadataJSON = logEntry.Metadata
	}

	err := s.db.QueryRow(`
		INSERT INTO activity_logs (ticket_id, event_type, actor, prev_state, new_state, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, logEntry.TicketID, logEntry.EventType, logEntry.Actor, logEntry.PrevState,
		logEntry.NewState, metadataJSON).Scan(&logEntry.ID)
	if err != nil {
		return 0, fmt.Errorf("CreateActivityLog: %w", err)
	}

	return logEntry.ID, nil
}

// GetActivityLogs retrieves activity logs for a ticket, ordered by created_at DESC.
func (s *PostgresStore) GetActivityLogs(ticketID string, limit int) ([]*models.ActivityLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50 // default 50, max 100
	}

	query := `
		SELECT id, ticket_id, event_type, actor, prev_state, new_state, metadata, created_at
		FROM activity_logs
		WHERE ticket_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(query, ticketID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetActivityLogs query: %w", err)
	}
	defer rows.Close()

	var logs []*models.ActivityLog
	for rows.Next() {
		var logEntry models.ActivityLog
		err := rows.Scan(&logEntry.ID, &logEntry.TicketID, &logEntry.EventType,
			&logEntry.Actor, &logEntry.PrevState, &logEntry.NewState,
			&logEntry.Metadata, &logEntry.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("GetActivityLogs scan: %w", err)
		}
		logs = append(logs, &logEntry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetActivityLogs rows error: %w", err)
	}

	return logs, nil
}
