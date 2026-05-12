// Package store provides database abstraction for Goban persistence.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"goban/config"
	"goban/models"
)

// SQLiteStore implements TicketStore using SQLite backend.
type SQLiteStore struct {
	db     *sql.DB
	config config.Config
}

// Init opens the database connection and creates tables.
func (s *SQLiteStore) Init() error {
	db, err := sql.Open("sqlite3", s.config.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	s.db = db

	// Enable foreign keys and WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Printf("Warning: Failed to enable foreign keys: %v", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		log.Printf("Warning: Failed to enable WAL mode: %v", err)
	}

	// Create task_links table for parent/child dependencies (ticket-144555d9c3)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS task_links (
			parent_id TEXT NOT NULL,
			child_id TEXT NOT NULL,
			PRIMARY KEY (parent_id, child_id),
			FOREIGN KEY (parent_id) REFERENCES tickets(id) ON DELETE CASCADE,
			FOREIGN KEY (child_id) REFERENCES tickets(id) ON DELETE CASCADE,
			CHECK (parent_id != child_id)
		)
	`)

	if err := s.createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Migration: add idempotency_key column and unique index if not present (safe no-op on existing columns/indexes)
	if _, err := db.Exec("ALTER TABLE tickets ADD COLUMN idempotency_key TEXT DEFAULT NULL"); err != nil {
		log.Printf("Warning: idempotency_key migration skipped (%v) — likely already applied", err)
	}
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_tickets_idempotency_key ON tickets(idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key != ''")

	log.Printf("Database initialized: %s", s.config.DBPath)
	return nil
}

// Close closes the SQLite database connection.
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ============================================================
// TaskLink operations - Parent/Child dependencies (ticket-144555d9c3)
// ============================================================

// AddTaskLink creates a parent-child dependency between two tickets.
func (s *SQLiteStore) AddTaskLink(parentID, childID string) error {
	if parentID == childID {
		return fmt.Errorf("self-link: ticket cannot be its own parent")
	}

	err := s.detectCycle(parentID, childID)
	if err != nil {
		return err
	}

	tx, txErr := s.db.Begin()
	if txErr != nil {
		return fmt.Errorf("AddTaskLink begin: %w", txErr)
	}

	_, err = tx.Exec(`INSERT OR IGNORE INTO task_links (parent_id, child_id) VALUES (?, ?)`, parentID, childID)
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}

	return fmt.Errorf("AddTaskLink exec: %w", err)
}

// RemoveTaskLink deletes a parent-child dependency.
func (s *SQLiteStore) RemoveTaskLink(parentID, childID string) error {
	result, err := s.db.Exec(`DELETE FROM task_links WHERE parent_id = ? AND child_id = ?`, parentID, childID)
	if err != nil {
		return fmt.Errorf("RemoveTaskLink exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("task link not found")
	}
	return nil
}

// GetTaskLinks returns all parents and children for a given ticket.
func (s *SQLiteStore) GetTaskLinks(ticketID string) ([]string, []string, error) {
	var parents []string
	var children []string

	pRows, err := s.db.Query(`SELECT parent_id FROM task_links WHERE child_id = ?`, ticketID)
	if err != nil {
		return nil, nil, fmt.Errorf("GetTaskLinks parents query: %w", err)
	}
	defer pRows.Close()
	for pRows.Next() {
		var pid string
		if err := pRows.Scan(&pid); err == nil {
			parents = append(parents, pid)
		}
	}

	cRows, err := s.db.Query(`SELECT child_id FROM task_links WHERE parent_id = ?`, ticketID)
	if err != nil {
		return nil, nil, fmt.Errorf("GetTaskLinks children query: %w", err)
	}
	defer cRows.Close()
	for cRows.Next() {
		var cid string
		if err := cRows.Scan(&cid); err == nil {
			children = append(children, cid)
		}
	}

	return parents, children, nil
}

// detectCycle uses BFS from child following parent edges to check if parent is reachable.
func (s *SQLiteStore) detectCycle(parentID, childID string) error {
	visited := make(map[string]bool)
	queue := []string{childID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == parentID {
			return fmt.Errorf("cycle detected: adding link %s -> %s would create a dependency cycle", parentID, childID)
		}

		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := s.db.Query(`SELECT parent_id FROM task_links WHERE child_id = ?`, current)
		if err != nil {
			return fmt.Errorf("detectCycle query: %w", err)
		}

		for rows.Next() {
			var pid string
			if err := rows.Scan(&pid); err == nil {
				queue = append(queue, pid)
			}
		}
		rows.Close()
	}

	return nil // No cycle found
}

// ============================================================
// TicketRun operations - Per-attempt tracking (ticket-2b0f57e014)
// ============================================================

// CreateRun inserts a new run record for the given ticket.
func (s *SQLiteStore) CreateRun(r *models.TicketRun) (*models.TicketRun, error) {
	now := time.Now()
	var startedAt string = now.Format(time.RFC3339)
	if !r.StartedAt.IsZero() {
		startedAt = r.StartedAt.Format(time.RFC3339)
	}

	result, err := s.db.Exec(`
		INSERT INTO ticket_runs (ticket_id, outcome, started_at, summary, metadata, actor)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.TicketID, r.Outcome, startedAt, r.Summary, r.Metadata, r.Actor)
	if err != nil {
		return nil, fmt.Errorf("CreateRun insert: %w", err)
	}

	runID, _ := result.LastInsertId()
	r.ID = runID
	r.StartedAt = now

	return r, nil
}

// GetRuns returns all runs for a ticket, newest first.
func (s *SQLiteStore) GetRuns(ticketID string) ([]*models.TicketRun, error) {
	rows, err := s.db.Query(`
		SELECT id, ticket_id, outcome, started_at, ended_at, summary, metadata, actor
		FROM ticket_runs WHERE ticket_id = ? ORDER BY started_at DESC
	`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("GetRuns query: %w", err)
	}
	defer rows.Close()

	var runs []*models.TicketRun
	for rows.Next() {
		run := &models.TicketRun{}
		var endedAt sql.NullString
		err := rows.Scan(&run.ID, &run.TicketID, &run.Outcome, &run.StartedAt, &endedAt, &run.Summary, &run.Metadata, &run.Actor)
		if err != nil {
			continue
		}
		if endedAt.Valid && endedAt.String != "" {
			parsed, parseErr := time.Parse(time.RFC3339, endedAt.String)
			if parseErr == nil {
				run.EndedAt = &parsed
			}
		}
		runs = append(runs, run)
	}

	return runs, rows.Err()
}

// UpdateRun updates outcome/ended_at/summary/metadata for a run.
func (s *SQLiteStore) UpdateRun(runID int64, outcome string, summary string, metadata string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE ticket_runs SET outcome = ?, ended_at = ?, summary = COALESCE(?, summary), metadata = COALESCE(?, metadata) WHERE id = ?
	`, outcome, now, summary, metadata, runID)
	return fmt.Errorf("UpdateRun exec: %w", err)
}

// GetActiveRun returns the current active (non-terminal) run for a ticket.
func (s *SQLiteStore) GetActiveRun(ticketID string) (*models.TicketRun, error) {
	row := s.db.QueryRow(`
		SELECT id, ticket_id, outcome, started_at, ended_at, summary, metadata, actor
		FROM ticket_runs WHERE ticket_id = ? AND outcome = 'active' ORDER BY started_at DESC LIMIT 1
	`, ticketID)

	run := &models.TicketRun{}
	var endedAt sql.NullString
	err := row.Scan(&run.ID, &run.TicketID, &run.Outcome, &run.StartedAt, &endedAt, &run.Summary, &run.Metadata, &run.Actor)
	if err != nil {
		return nil, err
	}

	if endedAt.Valid && endedAt.String != "" {
		parsed, parseErr := time.Parse(time.RFC3339, endedAt.String)
		if parseErr == nil {
			run.EndedAt = &parsed
		}
	}

	return run, nil
}

// BeginTx starts a new transaction and returns a Tx interface for scoped operations.
func (s *SQLiteStore) BeginTx() (Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &sqliteTx{tx: tx, store: s}, nil
}

// sqliteTx implements Tx interface for SQLite transactions.
type sqliteTx struct {
	tx    *sql.Tx
	store *SQLiteStore // Reference to parent store for helper methods
}

// Commit commits the transaction.
func (t *sqliteTx) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction.
func (t *sqliteTx) Rollback() error {
	return t.tx.Rollback()
}

// GetTicket retrieves a ticket within the transaction context.
func (t *sqliteTx) GetTicket(id string) (*models.Ticket, error) {
	row := t.tx.QueryRow(`
		SELECT id, title, description, priority, assignee, column, board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE id = ?
	`, id)

	tickets, err := t.store.scanTicketsSingle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	if len(tickets) == 0 {
		return nil, sql.ErrNoRows
	}

	return tickets[0], nil
}

// UpdateTicket updates a ticket within the transaction context.
func (t *sqliteTx) UpdateTicket(tk *models.Ticket) error {
	subtasksJSON, err := safeMarshal(tk.Subtasks, "subtasks")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	commentsJSON, err := safeMarshal(tk.Comments, "comments")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	labelsJSON, err := safeMarshal(tk.Labels, "labels")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}

	var archivedInt int
	if tk.Archived {
		archivedInt = 1
	}

	// Handle nullable string fields for SQLite
	var dueDate interface{} = nil
	if tk.DueDate != nil && *tk.DueDate != "" {
		dueDate = *tk.DueDate
	}
	var archivedAt interface{} = nil
	if tk.ArchivedAt != nil && *tk.ArchivedAt != "" {
		archivedAt = *tk.ArchivedAt
	}

	_, err = t.tx.Exec(`
	UPDATE tickets SET 
		title=?, description=?, priority=?, assignee=?, column=?, board_id=?,
		labels=?, due_date=?, subtasks=?, comments=?, archived=?,
		archived_at=?, updated_at=?
	WHERE id=?
`, tk.Title, tk.Description, tk.Priority, tk.Assignee, tk.Column, tk.BoardID,
		labelsJSON, dueDate, subtasksJSON, commentsJSON, archivedInt,
		archivedAt, tk.UpdatedAt, tk.ID)
	if err != nil {
		return fmt.Errorf("UpdateTicket exec: %w", err)
	}

	return nil
}

// GetTicketsByColumnAndAssignee retrieves tickets matching column prefix and assignee within transaction.
func (t *sqliteTx) GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error) {
	rows, err := t.tx.Query(`
		SELECT id, title, description, priority, assignee, column, board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets 
		WHERE column LIKE ? AND assignee = ? AND archived = 0
		ORDER BY created_at DESC
	`, columnPrefix+"%", assignee)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		tk := t.store.scanTicketFromRows(rows)
		if tk != nil {
			tickets = append(tickets, tk)
		}
	}

	return tickets, rows.Err()
}

// CreateActivityLog inserts an activity log entry within the transaction.
func (t *sqliteTx) CreateActivityLog(logEntry *models.ActivityLog) (int64, error) {
	var metadataJSON string = ""
	if logEntry.Metadata != "" {
		metadataJSON = logEntry.Metadata
	}

	result, err := t.tx.Exec(`
		INSERT INTO activity_logs (ticket_id, event_type, actor, prev_state, new_state, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
	`, logEntry.TicketID, logEntry.EventType, logEntry.Actor,
		logEntry.PrevState, logEntry.NewState, metadataJSON)
	if err != nil {
		return 0, fmt.Errorf("CreateActivityLog: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("CreateActivityLog LastInsertId: %w", err)
	}

	logEntry.ID = id
	return id, nil
}

// createTables creates the database schema.
func (s *SQLiteStore) createTables() error {
	schema := `
	-- Users table for role-based access control
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		role TEXT NOT NULL DEFAULT 'NORMAL_AI',
		password_hash TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tickets (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		priority TEXT DEFAULT 'medium',
		assignee TEXT DEFAULT '',
		column TEXT NOT NULL,
		board_id TEXT NOT NULL,
		idempotency_key TEXT,
		labels TEXT DEFAULT '[]', -- JSON array of strings
		due_date TEXT DEFAULT '',
		subtasks TEXT DEFAULT '[]', -- JSON array of Subtask objects
		comments TEXT DEFAULT '[]', -- JSON array of Comment objects
		archived INTEGER DEFAULT 0,
		archived_at TEXT DEFAULT '',
		archived_by INTEGER REFERENCES users(id), -- Admin who force-archived the ticket
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_name TEXT NOT NULL UNIQUE,
		token_name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		user_id INTEGER REFERENCES users(id),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_used TIMESTAMP
	);


	CREATE INDEX IF NOT EXISTS idx_tickets_archived_by ON tickets(archived_by);
	CREATE INDEX IF NOT EXISTS idx_tickets_board ON tickets(board_id);
	CREATE INDEX IF NOT EXISTS idx_tickets_column ON tickets(column);
	CREATE INDEX IF NOT EXISTS idx_tickets_archived ON tickets(archived);
	CREATE INDEX IF NOT EXISTS idx_tokens_hash ON agent_tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_users_name ON users(name);

	-- Activity logs for audit trail of ticket state changes
	CREATE TABLE IF NOT EXISTS activity_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticket_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		actor TEXT NOT NULL,
		prev_state TEXT,
		new_state TEXT,
		metadata TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_activity_ticket ON activity_logs(ticket_id);
	CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_logs(created_at);
	`

	_, err := s.db.Exec(schema)
	return err
}

// GetAllTickets retrieves all active (non-archived) tickets.
func (s *SQLiteStore) GetAllTickets() ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, column, board_id, 
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = 0
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return s.scanTickets(rows)
}

// GetPaginatedTickets retrieves paginated active tickets with total count.
func (s *SQLiteStore) GetPaginatedTickets(p Pagination) ([]*models.Ticket, int64, error) {
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
	err := s.db.QueryRow("SELECT COUNT(*) FROM tickets WHERE archived = 0").Scan(&totalCount)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Count query error: %v", err)
		}
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}
	if config.Debug {
		log.Printf("[STORE.DEBUG] GetPaginatedTickets: totalCount=%d, limit=%d, offset=%d", totalCount, limit, offset)
	}

	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, column, board_id, 
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = 0
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Query error: %v", err)
		}
		return nil, totalCount, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	tickets, err := s.scanTickets(rows)
	if err != nil {
		return nil, totalCount, err
	}

	return tickets, totalCount, nil
}

// GetTicket retrieves a single ticket by ID.
func (s *SQLiteStore) GetTicket(id string) (*models.Ticket, error) {
	row := s.db.QueryRow(`
		SELECT id, title, description, priority, assignee, column, board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE id = ?
	`, id)

	tickets, err := s.scanTicketsSingle(row)
	if err != nil {
		if config.Debug {
			log.Printf("[DEBUG] GetTicket(%s) scan error: %v", id, err)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	if len(tickets) == 0 {
		if config.Debug {
			log.Printf("[DEBUG] GetTicket(%s) returned empty slice", id)
		}
		return nil, sql.ErrNoRows
	}

	return tickets[0], nil
}

// CreateTicket inserts a new ticket within a transaction.
func (s *SQLiteStore) CreateTicket(t *models.Ticket) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("CreateTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	subtasksJSON, err := safeMarshal(t.Subtasks, "subtasks")
	if err != nil {
		return fmt.Errorf("CreateTicket: %w", err)
	}
	commentsJSON, err := safeMarshal(t.Comments, "comments")
	if err != nil {
		return fmt.Errorf("CreateTicket: %w", err)
	}
	labelsJSON, err := safeMarshal(t.Labels, "labels")
	if err != nil {
		return fmt.Errorf("CreateTicket: %w", err)
	}

	// Handle nullable string fields for SQLite
	var dueDate interface{} = nil
	if t.DueDate != nil && *t.DueDate != "" {
		dueDate = *t.DueDate
	}

	_, err = tx.Exec(`
	INSERT INTO tickets (id, title, description, priority, assignee, column, board_id,
	 labels, due_date, subtasks, comments, archived, idempotency_key, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, t.ID, t.Title, t.Description, t.Priority, t.Assignee, t.Column, t.BoardID,
		labelsJSON, dueDate, subtasksJSON, commentsJSON, 0, t.IdempotencyKey, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("CreateTicket exec: %w", err)
	}

	return tx.Commit()
}

// CreateOrGetTicket creates a new ticket or returns the existing one if an idempotency key matches.
// If t.IdempotencyKey is non-empty, checks for an existing non-archived ticket with that key first.
func (s *SQLiteStore) CreateOrGetTicket(t *models.Ticket) (*models.Ticket, error) {
	if t.IdempotencyKey != "" {
		row := s.db.QueryRow(`
			SELECT id, title, description, priority, assignee, column, board_id,
			 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
			 created_at, updated_at
			FROM tickets WHERE idempotency_key = ? AND archived = 0
		`, t.IdempotencyKey)

		var (
			labelsJSON     []byte
			dueDateStr     sql.NullString
			subtasksJSON   []byte
			commentsJSON   []byte
			archivedInt    int
			archivedAtStr  sql.NullString
			archivedByVal  sql.NullInt64
		)

		result := &models.Ticket{}
		err := row.Scan(
			&result.ID, &result.Title, &result.Description, &result.Priority,
			&result.Assignee, &result.Column, &result.BoardID,
			&labelsJSON, &dueDateStr, &subtasksJSON, &commentsJSON,
			&archivedInt, &archivedAtStr, &archivedByVal,
			&result.CreatedAt, &result.UpdatedAt,
		)
		if err == nil {
			// Parse JSON fields from the existing ticket
			if len(labelsJSON) > 0 {
				if unmarshalErr := json.Unmarshal(labelsJSON, &result.Labels); unmarshalErr != nil {
					log.Printf("Warning: Failed to unmarshal labels for ticket %s: %v", result.ID, unmarshalErr)
				}
			}
			if dueDateStr.Valid && dueDateStr.String != "" {
				dueDate := dueDateStr.String
				result.DueDate = &dueDate
			}
			if len(subtasksJSON) > 0 {
				if unmarshalErr := json.Unmarshal(subtasksJSON, &result.Subtasks); unmarshalErr != nil {
					log.Printf("Warning: Failed to unmarshal subtasks for ticket %s: %v", result.ID, unmarshalErr)
				}
			}
			if len(commentsJSON) > 0 {
				if unmarshalErr := json.Unmarshal(commentsJSON, &result.Comments); unmarshalErr != nil {
					log.Printf("Warning: Failed to unmarshal comments for ticket %s: %v", result.ID, unmarshalErr)
				}
			}
			if archivedInt == 1 {
				result.Archived = true
			}
			if archivedAtStr.Valid && archivedAtStr.String != "" {
				archivedAt := archivedAtStr.String
				result.ArchivedAt = &archivedAt
			}
			if archivedByVal.Valid {
				aid := int(archivedByVal.Int64)
				result.ArchivedBy = &aid
			}

			// Set idempotency key on returned ticket for consistency
			result.IdempotencyKey = t.IdempotencyKey
			return result, nil
		} else if err != sql.ErrNoRows {
			return nil, fmt.Errorf("CreateOrGetTicket lookup: %w", err)
		}
	}

	// No existing match (or no key provided) — create the ticket normally
	if err := s.CreateTicket(t); err != nil {
		return nil, fmt.Errorf("CreateOrGetTicket: %w", err)
	}
	return t, nil
}

// UpdateTicket updates an existing ticket within a transaction.
func (s *SQLiteStore) UpdateTicket(t *models.Ticket) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("UpdateTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	subtasksJSON, err := safeMarshal(t.Subtasks, "subtasks")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	commentsJSON, err := safeMarshal(t.Comments, "comments")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}
	labelsJSON, err := safeMarshal(t.Labels, "labels")
	if err != nil {
		return fmt.Errorf("UpdateTicket: %w", err)
	}

	var archivedInt int
	if t.Archived {
		archivedInt = 1
	}

	// Handle nullable string fields for SQLite
	var dueDate interface{} = nil
	if t.DueDate != nil && *t.DueDate != "" {
		dueDate = *t.DueDate
	}
	var archivedAt interface{} = nil
	if t.ArchivedAt != nil && *t.ArchivedAt != "" {
		archivedAt = *t.ArchivedAt
	}

	_, err = tx.Exec(`
	UPDATE tickets SET 
		title=?, description=?, priority=?, assignee=?, column=?, board_id=?,
		labels=?, due_date=?, subtasks=?, comments=?, archived=?,
		archived_at=?, updated_at=?
	WHERE id=?
`, t.Title, t.Description, t.Priority, t.Assignee, t.Column, t.BoardID,
		labelsJSON, dueDate, subtasksJSON, commentsJSON, archivedInt,
		archivedAt, t.UpdatedAt, t.ID)
	if err != nil {
		return fmt.Errorf("UpdateTicket exec: %w", err)
	}

	return tx.Commit()
}

// DeleteTicket removes a ticket by ID within a transaction.
func (s *SQLiteStore) DeleteTicket(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("DeleteTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec("DELETE FROM tickets WHERE id = ?", id)
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
func (s *SQLiteStore) GetArchivedTickets(boardID string) ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, column, board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = 1 AND board_id = ?
		ORDER BY archived_at DESC
	`, boardID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return s.scanTickets(rows)
}

// GetAllArchivedTickets retrieves all archived tickets regardless of board.
func (s *SQLiteStore) GetAllArchivedTickets() ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, column, board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = 1
		ORDER BY archived_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return s.scanTickets(rows)
}

// ArchiveTicket force-archives a single ticket (admin operation).
func (s *SQLiteStore) ArchiveTicket(ticketID string, adminID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("ArchiveTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`
		UPDATE tickets SET archived = 1, archived_at = CURRENT_TIMESTAMP, 
		archived_by = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, adminID, ticketID)
	if err != nil {
		return fmt.Errorf("ArchiveTicket exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	return tx.Commit()
}

// UnarchiveTicket restores a previously archived ticket back to active status.
func (s *SQLiteStore) UnarchiveTicket(ticketID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("UnarchiveTicket begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`UPDATE tickets SET archived=0, archived_at=NULL, archived_by=NULL WHERE id=?`, ticketID)
	if err != nil {
		return fmt.Errorf("UnarchiveTicket exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	return tx.Commit()
}

// ArchiveTicketsBulk force-archives multiple tickets (admin operation).
func (s *SQLiteStore) ArchiveTicketsBulk(ticketIDs []string, adminID int64) error {
	if len(ticketIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("ArchiveTicketsBulk begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Build placeholder string for IN clause.
	// IMPORTANT: SQLite uses positional ? placeholders filled left-to-right.
	// The args array is [adminID, ticketID_1, ticketID_2, ...].
	// Therefore the SET clause MUST place archived_by=? BEFORE the WHERE id IN (...)
	// so that adminID binds to archived_by and subsequent ticket IDs bind correctly.
	// PostgreSQL uses named $N placeholders ($1 for adminID, $2+ for tickets) which is
	// not subject to this ordering constraint — see PostgresStore.ArchiveTicketsBulk.
	placeholders := make([]string, len(ticketIDs))
	for i := range ticketIDs {
		placeholders[i] = "?"
	}
	inClause := strings.Join(placeholders, ", ")

	args := make([]interface{}, len(ticketIDs)+1)
	args[0] = adminID // Maps to first ? in archived_by=? (SET clause)
	for i, id := range ticketIDs {
		args[i+1] = id // Maps to subsequent ?s in WHERE id IN (?, ?, ...)
	}

	query := fmt.Sprintf(`
		UPDATE tickets SET archived = 1, archived_at = CURRENT_TIMESTAMP,
		archived_by = ?, updated_at = CURRENT_TIMESTAMP
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
func (s *SQLiteStore) GetArchivedByAdmin(adminID int64) ([]*models.Ticket, error) {
	rows, err := s.db.Query(`
		SELECT id, title, description, priority, assignee, column, board_id,
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets WHERE archived = 1 AND archived_by = ?
		ORDER BY archived_at DESC`, adminID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return s.scanTickets(rows)
}

// CreateToken inserts a new API token.
func (s *SQLiteStore) CreateToken(agentName, tokenHash string) (int64, error) {
	tokenPrefix := "goban-"
	if len(tokenHash) >= 8 {
		tokenPrefix += tokenHash[:8]
	} else if len(tokenHash) > 0 {
		tokenPrefix += tokenHash
	}

	result, err := s.db.Exec(`
		INSERT INTO agent_tokens (agent_name, token_name, token_hash) VALUES (?, ?, ?)
	`, agentName, tokenPrefix, tokenHash)
	if err != nil {
		return 0, err
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// CreateTokenWithUser creates a new API token linked to an existing user.
func (s *SQLiteStore) CreateTokenWithUser(userID int64, agentName, tokenHash string) (int64, error) {
	tokenPrefix := "goban-"
	if len(tokenHash) >= 8 {
		tokenPrefix += tokenHash[:8]
	} else if len(tokenHash) > 0 {
		tokenPrefix += tokenHash
	}

	result, err := s.db.Exec(`
		INSERT INTO agent_tokens (agent_name, token_name, token_hash, user_id) VALUES (?, ?, ?, ?)
	`, agentName, tokenPrefix, tokenHash, userID)
	if err != nil {
		return 0, fmt.Errorf("CreateTokenWithUser exec: %w", err)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// ValidateToken retrieves a token by its hash.
func (s *SQLiteStore) ValidateToken(tokenHash string) (*models.AgentToken, error) {
	row := s.db.QueryRow(`
		SELECT id, agent_name, token_name, token_hash, user_id, created_at, last_used
		FROM agent_tokens WHERE token_hash = ?
	`, tokenHash)

	var t models.AgentToken
	var userID sql.NullInt64
	var createdAtStr, lastUsedStr sql.NullString

	err := row.Scan(&t.ID, &t.AgentName, &t.TokenName, &t.TokenHash, &userID,
		&createdAtStr, &lastUsedStr)
	if err != nil {
		return nil, err
	}

	if userID.Valid {
		t.UserID = userID.Int64
	}

	t.CreatedAt = parseTimeFromRFC3339(createdAtStr.String)
	if lastUsedStr.Valid {
		ts := parseTimeFromRFC3339(lastUsedStr.String)
		t.LastUsed = &ts
	}

	return &t, nil
}

// UpdateTokenLastUsed updates the last_used timestamp.
func (s *SQLiteStore) UpdateTokenLastUsed(tokenHash string) error {
	_, err := s.db.Exec(`
		UPDATE agent_tokens SET last_used = CURRENT_TIMESTAMP WHERE token_hash = ?
	`, tokenHash)
	return err
}

// DeleteToken removes a token by agent name.
func (s *SQLiteStore) DeleteToken(agentName string) (int64, error) {
	result, err := s.db.Exec("DELETE FROM agent_tokens WHERE agent_name = ?", agentName)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListTokens returns all tokens.
func (s *SQLiteStore) ListTokens() ([]models.AgentToken, error) {
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
		var createdAtStr, lastUsedStr sql.NullString

		err := rows.Scan(&t.ID, &t.AgentName, &t.TokenName, &t.TokenHash,
			&createdAtStr, &lastUsedStr)
		if err != nil {
			return nil, err
		}

		t.CreatedAt = parseTimeFromRFC3339(createdAtStr.String)
		if lastUsedStr.Valid {
			ts := parseTimeFromRFC3339(lastUsedStr.String)
			t.LastUsed = &ts
		}

		tokens = append(tokens, t)
	}

	return tokens, rows.Err()
}

// scanTickets scans multiple rows into Ticket pointers.
func (s *SQLiteStore) scanTickets(rows *sql.Rows) ([]*models.Ticket, error) {
	var tickets []*models.Ticket

	for rows.Next() {
		t := s.scanTicketFromRows(rows)
		if t != nil {
			tickets = append(tickets, t)
		}
	}

	return tickets, rows.Err()
}

// scanTicketRaw builds a *Ticket from raw scanned values. Shared by both
// scanTicketsSingle and scanTicketFromRows to eliminate duplicated JSON unmarshaling.
func scanTicketRaw(t models.Ticket, labelsJSON, subtasksJSON, commentsJSON []byte, dueDateStr, archivedAtStr sql.NullString, archivedBy sql.NullInt64) *models.Ticket {
	if dueDateStr.Valid && dueDateStr.String != "" {
		t.DueDate = &dueDateStr.String
	}
	if archivedAtStr.Valid && archivedAtStr.String != "" {
		t.ArchivedAt = &archivedAtStr.String
	}
	if archivedBy.Valid {
		archivedByID := int(archivedBy.Int64)
		t.ArchivedBy = &archivedByID
	}

	if err := json.Unmarshal(labelsJSON, &t.Labels); err != nil {
		log.Printf("Warning: Failed to unmarshal labels for ticket %s: %v", t.ID, err)
		t.Labels = []string{}
	}
	if err := json.Unmarshal(subtasksJSON, &t.Subtasks); err != nil {
		log.Printf("Warning: Failed to unmarshal subtasks for ticket %s: %v", t.ID, err)
		t.Subtasks = []models.Subtask{}
	}
	if err := json.Unmarshal(commentsJSON, &t.Comments); err != nil {
		log.Printf("Warning: Failed to unmarshal comments for ticket %s: %v", t.ID, err)
		t.Comments = []models.Comment{}
	}

	return &t
}

// scanTicketsSingle scans a single row into a Ticket (returns as slice for consistency).
func (s *SQLiteStore) scanTicketsSingle(row *sql.Row) ([]*models.Ticket, error) {
	var t models.Ticket
	var labelsJSON, subtasksJSON, commentsJSON []byte
	var dueDateStr, archivedAtStr sql.NullString

	var archivedBy sql.NullInt64
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Priority,
		&t.Assignee, &t.Column, &t.BoardID,
		&labelsJSON, &dueDateStr,
		&subtasksJSON, &commentsJSON, &t.Archived,
		&archivedAtStr, &archivedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return []*models.Ticket{scanTicketRaw(t, labelsJSON, subtasksJSON, commentsJSON, dueDateStr, archivedAtStr, archivedBy)}, nil
}

// scanTicketFromRows scans a single row (helper for GetAllTickets).
func (s *SQLiteStore) scanTicketFromRows(rows *sql.Rows) *models.Ticket {
	var t models.Ticket
	var labelsJSON, subtasksJSON, commentsJSON []byte
	var dueDateStr, archivedAtStr sql.NullString

	var archivedBy sql.NullInt64
	err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Priority,
		&t.Assignee, &t.Column, &t.BoardID,
		&labelsJSON, &dueDateStr,
		&subtasksJSON, &commentsJSON, &t.Archived,
		&archivedAtStr, &archivedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		log.Printf("Warning: Failed to scan ticket row: %v", err)
		return nil
	}

	return scanTicketRaw(t, labelsJSON, subtasksJSON, commentsJSON, dueDateStr, archivedAtStr, archivedBy)
}

// ==================== USER CRUD OPERATIONS ====================

// CreateUser creates a new user with the specified name and role.
func (s *SQLiteStore) CreateUser(name, role string) (int64, error) {
	// Validate role
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI && role != models.RoleNormalAI {
		return 0, fmt.Errorf("invalid role: %s (must be HUMAN_ADMIN, OVERSEER_AI, or NORMAL_AI)", role)
	}

	result, err := s.db.Exec(`
		INSERT INTO users (name, role) VALUES (?, ?)
	`, name, role)
	if err != nil {
		return 0, fmt.Errorf("CreateUser exec: %w", err)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// CreateUserWithPassword creates a new user with password hashing.
func (s *SQLiteStore) CreateUserWithPassword(name, password, role string) (int64, error) {
	// Validate role
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI && role != models.RoleNormalAI {
		return 0, fmt.Errorf("invalid role: %s", role)
	}

	// Hash password with bcrypt
	passwordHash, err := HashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(`
		INSERT INTO users (name, role, password_hash) VALUES (?, ?, ?)
	`, name, role, passwordHash)
	if err != nil {
		return 0, fmt.Errorf("CreateUserWithPassword exec: %w", err)
	}

	id, _ := result.LastInsertId()
	log.Printf("[STORE] Created user '%s' with ID %d (role: %s)", name, id, role)
	return id, nil
}

// GetUserByID retrieves a user by their ID.
func (s *SQLiteStore) GetUserByID(id int64) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, name, role, created_at, updated_at
		FROM users WHERE id = ?
	`, id)

	var u models.User
	var createdAtStr, updatedAtStr sql.NullString

	err := row.Scan(&u.ID, &u.Name, &u.Role, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}

	u.CreatedAt = parseTimeFromRFC3339(createdAtStr.String)
	u.UpdatedAt = parseTimeFromRFC3339(updatedAtStr.String)

	return &u, nil
}

// GetUserByName retrieves a user by their name.
func (s *SQLiteStore) GetUserByName(name string) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, name, role, password_hash, created_at, updated_at
		FROM users WHERE name = ?
	`, name)

	var u models.User
	var createdAtStr, updatedAtStr sql.NullString
	var passwordHash sql.NullString

	err := row.Scan(&u.ID, &u.Name, &u.Role, &passwordHash, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}

	u.PasswordHash = passwordHash.String
	u.CreatedAt = parseTimeFromRFC3339(createdAtStr.String)
	u.UpdatedAt = parseTimeFromRFC3339(updatedAtStr.String)

	return &u, nil
}

// UpdateUserRole updates a user's role.
func (s *SQLiteStore) UpdateUserRole(id int64, role string) error {
	// Validate role
	if role != models.RoleHumanAdmin && role != models.RoleOverseerAI && role != models.RoleNormalAI {
		return fmt.Errorf("invalid role: %s", role)
	}

	result, err := s.db.Exec(`
		UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
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
func (s *SQLiteStore) DeleteUser(id int64) error {
	result, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
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
func (s *SQLiteStore) UpdateUserPassword(id int64, newPassword string) error {
	// Hash the new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(`
		UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
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

// ListUsers returns all users.
func (s *SQLiteStore) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`
		SELECT id, name, role, created_at, updated_at
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("ListUsers query: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&u.ID, &u.Name, &u.Role, &createdAtStr, &updatedAtStr)
		if err != nil {
			return nil, err
		}

		u.CreatedAt = parseTimeFromRFC3339(createdAtStr.String)
		u.UpdatedAt = parseTimeFromRFC3339(updatedAtStr.String)

		users = append(users, u)
	}

	return users, rows.Err()
}

// ==================== TOKEN-USER RELATIONSHIP OPERATIONS ====================

// UpdateTokenUserID links a token to a user.
func (s *SQLiteStore) UpdateTokenUserID(tokenHash string, userID int64) error {
	_, err := s.db.Exec(`
		UPDATE agent_tokens SET user_id = ? WHERE token_hash = ?
	`, userID, tokenHash)
	if err != nil {
		return fmt.Errorf("UpdateTokenUserID exec: %w", err)
	}

	return nil
}

// GetUserByToken retrieves the user associated with a token.
func (s *SQLiteStore) GetUserByToken(tokenHash string) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT u.id, u.name, u.role, u.created_at, u.updated_at
		FROM users u
		JOIN agent_tokens t ON u.id = t.user_id
		WHERE t.token_hash = ?
	`, tokenHash)

	var u models.User
	var createdAtStr, updatedAtStr sql.NullString

	err := row.Scan(&u.ID, &u.Name, &u.Role, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}

	u.CreatedAt = parseTimeFromRFC3339(createdAtStr.String)
	u.UpdatedAt = parseTimeFromRFC3339(updatedAtStr.String)

	return &u, nil
}

// GetTicketsByColumnAndAssignee returns tickets matching the column prefix and assignee.
func (s *SQLiteStore) GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error) {
	var query string
	var args []interface{}

	if columnPrefix == "" && assignee == "" {
		return s.GetAllTickets()
	}

	query = "SELECT id, title, description, priority, assignee, column, board_id, labels, due_date, subtasks, comments, archived, archived_at, archived_by, created_at, updated_at FROM tickets WHERE archived = 0"

	if columnPrefix != "" {
		query += " AND column LIKE ?"
		args = append(args, columnPrefix+"%")
	}
	if assignee != "" {
		query += " AND assignee = ?"
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

	tickets, err := s.scanTickets(rows)
	if err != nil {
		return nil, err
	}

	return tickets, nil
}

// GetTicketsWithFilter returns tickets filtered by allowed column prefixes with pagination.
func (s *SQLiteStore) GetTicketsWithFilter(allowedColumns []string, p Pagination) ([]*models.Ticket, int64, error) {
	// Apply defaults
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	// Build column filter conditions
	var whereClause string
	var args []interface{}

	if len(allowedColumns) > 0 {
		conditions := make([]string, 0, len(allowedColumns))
		for _, colPrefix := range allowedColumns {
			conditions = append(conditions, "column LIKE ?")
			args = append(args, colPrefix+"%")
		}
		whereClause = " WHERE archived = 0 AND (" + strings.Join(conditions, " OR ") + ")"
	} else {
		whereClause = " WHERE archived = 0"
	}

	// Get total count first
	var totalCount int64
	countQuery := "SELECT COUNT(*) FROM tickets" + whereClause
	err := s.db.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Count query error: %v", err)
		}
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}

	// Get filtered tickets
	query := "SELECT id, title, description, priority, assignee, column, board_id," +
		"labels, due_date, subtasks, comments, archived, archived_at, archived_by," +
		"created_at, updated_at FROM tickets" + whereClause +
		" ORDER BY created_at DESC LIMIT ? OFFSET ?" 

	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		if config.Debug {
			log.Printf("[STORE.DEBUG] Query error: %v", err)
		}
		return nil, totalCount, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	tickets, err := s.scanTickets(rows)
	if err != nil {
		return nil, totalCount, err
	}

	return tickets, totalCount, nil
}

// GetTicketsByAssignee returns all non-archived tickets assigned to a specific user.
func (s *SQLiteStore) GetTicketsByAssignee(assigneeName string) ([]*models.Ticket, error) {
	query := `
		SELECT id, title, description, priority, assignee, column, board_id, 
		 labels, due_date, subtasks, comments, archived, archived_at, archived_by,
		 created_at, updated_at
		FROM tickets 
		WHERE assignee = ? AND archived = 0
		ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query, assigneeName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return s.scanTickets(rows)
}

// CreateActivityLog inserts a new activity log entry.
func (s *SQLiteStore) CreateActivityLog(log *models.ActivityLog) (int64, error) {
	var metadataJSON string = ""
	if log.Metadata != "" {
		metadataJSON = log.Metadata
	}

	result, err := s.db.Exec(`
		INSERT INTO activity_logs (ticket_id, event_type, actor, prev_state, new_state, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
	`, log.TicketID, log.EventType, log.Actor, log.PrevState, log.NewState, metadataJSON)
	if err != nil {
		return 0, fmt.Errorf("CreateActivityLog: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("CreateActivityLog LastInsertId: %w", err)
	}

	return id, nil
}

// GetActivityLogs retrieves activity logs for a ticket, ordered by created_at DESC.
func (s *SQLiteStore) GetActivityLogs(ticketID string, limit int) ([]*models.ActivityLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50 // default 50, max 100
	}

	query := `
		SELECT id, ticket_id, event_type, actor, prev_state, new_state, metadata, created_at
		FROM activity_logs
		WHERE ticket_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, ticketID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetActivityLogs query: %w", err)
	}
	defer rows.Close()

	var logs []*models.ActivityLog
	for rows.Next() {
		var log models.ActivityLog
		err := rows.Scan(&log.ID, &log.TicketID, &log.EventType, &log.Actor,
			&log.PrevState, &log.NewState, &log.Metadata, &log.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("GetActivityLogs scan: %w", err)
		}
		logs = append(logs, &log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetActivityLogs rows error: %w", err)
	}

	return logs, nil
}
