// Package testutil provides testing utilities for Goban services.
package testutil

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"goban/models"
	"goban/store"
)

// MockStore implements store.TicketStore interface using in-memory storage.
// Designed for unit tests - NOT thread-safe for concurrent access beyond basic mutex protection.
type MockStore struct {
	mu       sync.RWMutex
	tickets  map[string]*models.Ticket
	tokens   map[int64]models.AgentToken
	users    map[int64]models.User
	nextID   int64
	tokenSeq int64

	// Activity log storage
	activityLogs []models.ActivityLog
	logSeq       int64

	// Task link storage for parent/child dependencies
	taskLinks []mockLink
	// Run history storage for per-attempt tracking
	runs []*models.TicketRun

	// Transaction support
	txTickets     map[string]*models.Ticket
	inTransaction bool
}

// NewMockStore creates a new in-memory mock store for testing.
func NewMockStore() *MockStore {
	return &MockStore{
		tickets:       make(map[string]*models.Ticket),
		tokens:        make(map[int64]models.AgentToken),
		users:         make(map[int64]models.User),
		nextID:        1,
		tokenSeq:      1,
		txTickets:     nil,
		inTransaction: false,
		taskLinks:     []mockLink{},
	}
}

// Init is a no-op for mock store.
func (m *MockStore) Init() error {
	return nil
}

// Close is a no-op for mock store.
func (m *MockStore) Close() error {
	return nil
}

// BeginTx starts a new transaction with a copy of current data.
func (m *MockStore) BeginTx() (store.Tx, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Deep copy tickets for transaction isolation
	txTickets := make(map[string]*models.Ticket)
	for id, ticket := range m.tickets {
		ticketCopy := *ticket
		txTickets[id] = &ticketCopy
	}

	return &MockTx{
		mockStore:     m,
		tickets:       txTickets,
		inTransaction: true,
	}, nil
}

// MockTx implements store.Tx interface for mock transactions.
type MockTx struct {
	mu            sync.RWMutex
	mockStore     *MockStore
	tickets       map[string]*models.Ticket
	inTransaction bool
}

// Commit applies transaction changes to the main store.
func (tx *MockTx) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if !tx.inTransaction {
		return nil // Already committed or rolled back
	}

	tx.mockStore.mu.Lock()
	for id, ticket := range tx.tickets {
		// Deep copy to avoid reference issues
		ticketCopy := *ticket
		tx.mockStore.tickets[id] = &ticketCopy
	}
	tx.inTransaction = false
	tx.mockStore.mu.Unlock()

	return nil
}

// Rollback discards transaction changes.
func (tx *MockTx) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	// Simply discard the transaction copy - main store unchanged
	tx.tickets = nil
	tx.inTransaction = false
	return nil
}

// GetTicket retrieves a ticket within the transaction context.
func (tx *MockTx) GetTicket(id string) (*models.Ticket, error) {
	tx.mu.RLock()
	defer tx.mu.RUnlock()

	ticket, exists := tx.tickets[id]
	if !exists {
		return nil, nil // Not found - caller should handle as sql.ErrNoRows equivalent
	}
	return ticket, nil
}

// UpdateTicket updates a ticket within the transaction context.
func (tx *MockTx) UpdateTicket(t *models.Ticket) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	ticketCopy := *t
	tx.tickets[t.ID] = &ticketCopy
	return nil
}

// GetTicketsByColumnAndAssignee fetches tickets within transaction context.
func (tx *MockTx) GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error) {
	tx.mu.RLock()
	defer tx.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range tx.tickets {
		if ticket.Assignee == assignee && len(ticket.Column) >= len(columnPrefix) &&
			ticket.Column[:len(columnPrefix)] == columnPrefix {
			result = append(result, ticket)
		}
	}
	return result, nil
}

// CreateActivityLog records an activity log entry within the transaction context.
func (tx *MockTx) CreateActivityLog(logEntry *models.ActivityLog) (int64, error) {
	// No-op in mock transactions - activity logs are persisted by the main store
	return 0, nil
}

// CreateTicket adds a new ticket to the mock store.
func (m *MockStore) CreateTicket(t *models.Ticket) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticketCopy := *t
	if t.CreatedAt == "" {
		ticketCopy.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if t.UpdatedAt == "" {
		ticketCopy.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	m.tickets[t.ID] = &ticketCopy
	return nil
}

// CreateOrGetTicket creates a ticket or returns existing one with matching idempotency key.
func (m *MockStore) CreateOrGetTicket(t *models.Ticket) (*models.Ticket, error) {
	if t.IdempotencyKey != "" {
		for _, existing := range m.tickets {
			if existing.IdempotencyKey == t.IdempotencyKey && !existing.Archived {
				return existing, nil
			}
		}
	}

	// No match — create new ticket
	if err := m.CreateTicket(t); err != nil {
		return nil, fmt.Errorf("CreateOrGetTicket: %w", err)
	}
	return t, nil
}

// GetAllTickets returns all tickets in the mock store.
func (m *MockStore) GetAllTickets() ([]*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range m.tickets {
		result = append(result, ticket)
	}
	return result, nil
}

// GetPaginatedTickets returns tickets with pagination.
func (m *MockStore) GetPaginatedTickets(p store.Pagination) ([]*models.Ticket, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := int64(len(m.tickets))

	// Simple pagination - convert map to slice and slice it
	var all []*models.Ticket
	for _, ticket := range m.tickets {
		all = append(all, ticket)
	}

	limit := p.Limit
	if limit == 0 || limit > 100 {
		limit = 50 // Default page size
	}

	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// GetTicket retrieves a ticket by ID.
func (m *MockStore) GetTicket(id string) (*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ticket, exists := m.tickets[id]
	if !exists {
		return nil, sql.ErrNoRows // Match real store behavior
	}
	return ticket, nil
}

// UpdateTicket updates an existing ticket.
func (m *MockStore) UpdateTicket(t *models.Ticket) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticketCopy := *t
	if t.UpdatedAt == "" {
		ticketCopy.UpdatedAt = time.Now().Format(time.RFC3339)
	} else {
		ticketCopy.UpdatedAt = t.UpdatedAt
	}
	m.tickets[t.ID] = &ticketCopy
	return nil
}

// DeleteTicket removes a ticket by ID.
func (m *MockStore) DeleteTicket(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tickets, id)
	return nil
}

// GetTicketsByColumnAndAssignee fetches tickets matching column prefix and assignee.
// If assignee is empty, returns all tickets in the matching columns regardless of assignment.
func (m *MockStore) GetTicketsByColumnAndAssignee(columnPrefix, assignee string) ([]*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range m.tickets {
		columnMatch := len(ticket.Column) >= len(columnPrefix) &&
			ticket.Column[:len(columnPrefix)] == columnPrefix

		if assignee == "" {
			// Empty assignee means "any assignee" - just match by column
			if columnMatch {
				result = append(result, ticket)
			}
		} else {
			// Specific assignee - must match both column and assignee
			if ticket.Assignee == assignee && columnMatch {
				result = append(result, ticket)
			}
		}
	}
	return result, nil
}

// GetTicketsWithFilter returns tickets filtered by allowed column prefixes.
func (m *MockStore) GetTicketsWithFilter(allowedColumns []string, p store.Pagination) ([]*models.Ticket, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range m.tickets {
		for _, colPrefix := range allowedColumns {
			if len(ticket.Column) >= len(colPrefix) && ticket.Column[:len(colPrefix)] == colPrefix {
				result = append(result, ticket)
				break
			}
		}
	}

	total := int64(len(result))

	limit := p.Limit
	if limit == 0 || limit > 100 {
		limit = 50
	}

	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	end := offset + limit
	if end > len(result) {
		end = len(result)
	}

	return result[offset:end], total, nil
}

// GetArchivedTickets returns archived tickets for a board.
func (m *MockStore) GetArchivedTickets(boardID string) ([]*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range m.tickets {
		if ticket.Archived && ticket.BoardID == boardID {
			result = append(result, ticket)
		}
	}
	return result, nil
}

// Token operations (simplified for testing)

func (m *MockStore) CreateToken(agentName, tokenHash string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.tokenSeq
	m.tokenSeq++

	token := models.AgentToken{
		ID:        id,
		AgentName: agentName,
		TokenName: "test-token-" + agentName,
		Token:     "mock-token-value",
		TokenHash: tokenHash,
		UserID:    0,
		CreatedAt: time.Now(),
	}

	m.tokens[id] = token
	return id, nil
}

func (m *MockStore) CreateTokenWithUser(userID int64, agentName, tokenHash string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.tokenSeq
	m.tokenSeq++

	token := models.AgentToken{
		ID:        id,
		AgentName: agentName,
		TokenName: "test-token-" + agentName,
		Token:     "mock-token-value",
		TokenHash: tokenHash,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	m.tokens[id] = token
	return id, nil
}

func (m *MockStore) ValidateToken(tokenHash string) (*models.AgentToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, token := range m.tokens {
		if token.TokenHash == tokenHash {
			tokenCopy := token
			return &tokenCopy, nil
		}
	}
	return nil, sql.ErrNoRows // Not found - match real store behavior
}

func (m *MockStore) UpdateTokenLastUsed(tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, token := range m.tokens {
		if token.TokenHash == tokenHash {
			token.LastUsed = &now
			m.tokens[id] = token
			return nil
		}
	}
	return nil // Not an error if not found in tests
}

func (m *MockStore) DeleteToken(agentName string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var deletedID int64 = 0
	for id, token := range m.tokens {
		if token.AgentName == agentName {
			deletedID = id
			delete(m.tokens, id)
			break
		}
	}
	return deletedID, nil
}

func (m *MockStore) ListTokens() ([]models.AgentToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []models.AgentToken
	for _, token := range m.tokens {
		result = append(result, token)
	}
	return result, nil
}

// User operations (for RBAC testing)

func (m *MockStore) CreateUser(name, role string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	user := models.User{
		ID:        id,
		Name:      name,
		Role:      role,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.users[id] = user
	return id, nil
}

func (m *MockStore) GetUserByID(id int64) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, exists := m.users[id]
	if !exists {
		return nil, nil
	}
	userCopy := user
	return &userCopy, nil
}

func (m *MockStore) GetUserByName(name string) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, user := range m.users {
		if user.Name == name {
			userCopy := user
			return &userCopy, nil
		}
	}
	return nil, nil
}

func (m *MockStore) UpdateUserRole(id int64, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[id]
	if !exists {
		return nil // Not found - not an error in tests
	}
	user.Role = role
	user.UpdatedAt = time.Now()
	m.users[id] = user
	return nil
}

func (m *MockStore) DeleteUser(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.users, id)
	return nil
}

func (m *MockStore) ListUsers() ([]models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []models.User
	for _, user := range m.users {
		result = append(result, user)
	}
	return result, nil
}

func (m *MockStore) UpdateTokenUserID(tokenHash string, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, token := range m.tokens {
		if token.TokenHash == tokenHash {
			token.UserID = userID
			m.tokens[id] = token
			return nil
		}
	}
	return nil // Not found - not an error in tests
}

func (m *MockStore) GetUserByToken(tokenHash string) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, token := range m.tokens {
		if token.TokenHash == tokenHash {
			user, exists := m.users[token.UserID]
			if !exists {
				return nil, nil
			}
			userCopy := user
			return &userCopy, nil
		}
	}
	return nil, nil
}

// GetAllArchivedTickets retrieves all archived tickets regardless of board.
func (m *MockStore) GetAllArchivedTickets() ([]*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range m.tickets {
		if ticket.Archived {
			result = append(result, ticket)
		}
	}
	return result, nil
}

// ArchiveTicket force-archives a single ticket (admin operation).
func (m *MockStore) ArchiveTicket(ticketID string, adminID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket, exists := m.tickets[ticketID]
	if !exists {
		return nil // Not found - not an error in tests
	}
	now := time.Now().Format(time.RFC3339)
	adminInt := int(adminID)
	ticket.Archived = true
	ticket.ArchivedAt = &now
	ticket.ArchivedBy = &adminInt
	return nil
}

// UnarchiveTicket restores a previously archived ticket back to active status.
func (m *MockStore) UnarchiveTicket(ticketID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket, exists := m.tickets[ticketID]
	if !exists {
		return nil // Not found - not an error in tests
	}
	ticket.Archived = false
	ticket.ArchivedAt = nil
	ticket.ArchivedBy = nil
	return nil
}

// ArchiveTicketsBulk force-archives multiple tickets (admin operation).
func (m *MockStore) ArchiveTicketsBulk(ticketIDs []string, adminID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	adminInt := int(adminID)
	for _, id := range ticketIDs {
		ticket, exists := m.tickets[id]
		if !exists {
			continue
		}
		ticket.Archived = true
		ticket.ArchivedAt = &now
		ticket.ArchivedBy = &adminInt
	}
	return nil
}

// GetArchivedByAdmin retrieves all tickets archived by a specific admin.
func (m *MockStore) GetArchivedByAdmin(adminID int64) ([]*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	adminInt := int(adminID)
	var result []*models.Ticket
	for _, ticket := range m.tickets {
		if ticket.Archived && ticket.ArchivedBy != nil && *ticket.ArchivedBy == adminInt {
			result = append(result, ticket)
		}
	}
	return result, nil
}

// CreateUserWithPassword creates a user with hashed password.

func (m *MockStore) CreateUserWithPassword(name, password, role string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	user := models.User{
		ID:           id,
		Name:         name,
		Role:         role,
		PasswordHash: "$2a$10$" + password, // Mock hash prefix for testing
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	m.users[id] = user
	return id, nil
}

func (m *MockStore) UpdateUserPassword(id int64, newPassword string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[id]
	if !exists {
		return nil // Not found - not an error in tests
	}
	user.PasswordHash = "$2a$10$" + newPassword // Mock hash prefix for testing
	user.UpdatedAt = time.Now()
	m.users[id] = user
	return nil
}

func (m *MockStore) CreateActivityLog(logEntry *models.ActivityLog) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logSeq++
	entry := *logEntry
	if entry.ID == 0 {
		entry.ID = m.logSeq
	}
	m.activityLogs = append(m.activityLogs, entry)
	return entry.ID, nil
}

func (m *MockStore) GetActivityLogs(ticketID string, limit int) ([]*models.ActivityLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.ActivityLog
	for i := len(m.activityLogs) - 1; i >= 0 && len(result) < limit; i-- {
		if m.activityLogs[i].TicketID == ticketID {
			entry := m.activityLogs[i]
			result = append(result, &entry)
		}
	}
	return result, nil
}

func (m *MockStore) GetTicketsByAssignee(assigneeName string) ([]*models.Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*models.Ticket
	for _, ticket := range m.tickets {
		if ticket.Assignee == assigneeName {
			result = append(result, ticket)
		}
	}
	return result, nil
}

// taskLinks stores parent-child dependencies for testing.
type mockLink struct {
	parentID string
	childID  string
}

func (m *MockStore) AddTaskLink(parentID, childID string) error {
	if parentID == childID {
		return fmt.Errorf("self-link: ticket cannot be its own parent")
	}
	// Simple cycle check - look for direct reverse link
	for _, link := range m.taskLinks {
		if link.parentID == childID && link.childID == parentID {
			return fmt.Errorf("cycle detected: adding link %s -> %s would create a dependency cycle", parentID, childID)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Check for duplicate
	for _, link := range m.taskLinks {
		if link.parentID == parentID && link.childID == childID {
			return nil // Already exists
		}
	}
	m.taskLinks = append(m.taskLinks, mockLink{parentID: parentID, childID: childID})
	return nil
}

func (m *MockStore) RemoveTaskLink(parentID, childID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, link := range m.taskLinks {
		if link.parentID == parentID && link.childID == childID {
			m.taskLinks = append(m.taskLinks[:i], m.taskLinks[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("task link not found")
}

func (m *MockStore) GetTaskLinks(ticketID string) ([]string, []string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var parents []string
	var children []string
	for _, link := range m.taskLinks {
		if link.childID == ticketID {
			parents = append(parents, link.parentID)
		}
		if link.parentID == ticketID {
			children = append(children, link.childID)
		}
	}
	return parents, children, nil
}

// ============================================================
// TicketRun operations - Per-attempt tracking (ticket-2b0f57e014)
// ============================================================

func (m *MockStore) CreateRun(r *models.TicketRun) (*models.TicketRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r.ID == 0 {
		runID := int64(1)
		for _, existing := range m.runs {
			if existing.ID >= runID {
				runID = existing.ID + 1
			}
		}
		r.ID = runID
	}

	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	if r.Outcome == "" {
		r.Outcome = "active"
	}

	m.runs = append(m.runs, r)
	return r, nil
}

func (m *MockStore) GetRuns(ticketID string) ([]*models.TicketRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var runs []*models.TicketRun
	for _, run := range m.runs {
		if run.TicketID == ticketID {
			runs = append(runs, run)
		}
	}

	// Sort by started_at descending (newest first)
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j].StartedAt.After(runs[i].StartedAt) {
				runs[i], runs[j] = runs[j], runs[i]
			}
		}
	}

	return runs, nil
}

func (m *MockStore) UpdateRun(runID int64, outcome string, summary string, metadata string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, run := range m.runs {
		if run.ID == runID {
			run.Outcome = outcome
			run.EndedAt = &now
			if summary != "" {
				run.Summary = summary
			}
			if metadata != "" {
				run.Metadata = metadata
			}
			return nil
		}
	}

	return fmt.Errorf("run not found: %d", runID)
}

func (m *MockStore) GetActiveRun(ticketID string) (*models.TicketRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, run := range m.runs {
		if run.TicketID == ticketID && run.Outcome == "active" {
			return run, nil
		}
	}

	return nil, sql.ErrNoRows
}
