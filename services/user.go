// Package services provides business logic layer for Goban operations.
package services

import (
	"fmt"

	"goban/models"
	"goban/store"
)

// UserService handles user-related business logic.
type UserService struct {
	store store.TicketStore
}

// NewUserService creates a new UserService with the given store.
func NewUserService(store store.TicketStore) *UserService {
	return &UserService{store: store}
}

// CreateUser creates a new user with the specified name and role.
// Returns the created user or an error if creation fails.
func (s *UserService) CreateUser(name, role string) (*models.User, error) {
	id, err := s.store.CreateUser(name, role)
	if err != nil {
		return nil, fmt.Errorf("CreateUser store: %w", err)
	}

	user, getErr := s.store.GetUserByID(id)
	if getErr != nil {
		return nil, fmt.Errorf("GetUserByID after create (id=%d): %w", id, getErr)
	}
	return user, nil
}

// GetUserByID retrieves a user by ID.
func (s *UserService) GetUserByID(id int64) (*models.User, error) {
	return s.store.GetUserByID(id)
}

// GetUserByName retrieves a user by their name.
// Returns nil if the user is not found.
func (s *UserService) GetUserByName(name string) (*models.User, error) {
	return s.store.GetUserByName(name)
}

// DeleteUser deletes a user by ID.
// This also cascades to delete associated tokens automatically via database constraints.
func (s *UserService) DeleteUser(id int64) error {
	return s.store.DeleteUser(id)
}

// UpdateUserRole updates a user's role.
func (s *UserService) UpdateUserRole(id int64, role string) error {
	return s.store.UpdateUserRole(id, role)
}

// UpdateUserPassword bcrypt-hashes and stores a new password.
func (s *UserService) UpdateUserPassword(id int64, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("password is required")
	}
	return s.store.UpdateUserPassword(id, newPassword)
}

// ListUsers retrieves all users.
func (s *UserService) ListUsers() ([]models.User, error) {
	return s.store.ListUsers()
}

// GetTicketsByAssignee returns all active tickets assigned to a user by name.
func (s *UserService) GetTicketsByAssignee(assigneeName string) ([]*models.Ticket, error) {
	return s.store.GetTicketsByAssignee(assigneeName)
}
