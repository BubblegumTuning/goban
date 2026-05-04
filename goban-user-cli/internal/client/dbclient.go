package client

import (
	"fmt"
	"goban/models"
	"goban/store"
	"time"

	gobanconfig "goban/config"

	"goban-user-cli/internal/config"
)

// Client provides direct database access for user management.
type Client struct {
	store store.TicketStore
}

// User represents a Goban user.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateTokenResponse represents a newly created token.
type CreateTokenResponse struct {
	ID        int64  `json:"id"`
	AgentName string `json:"agent_name"`
	TokenName string `json:"token_name"`
	UserID    int64  `json:"user_id"`
	Token     string `json:"token"`
	CreatedAt string `json:"created_at"`
}

// CreateUserResponse includes both the created user and their token.
type CreateUserResponse struct {
	User  *User                `json:"user"`
	Token *CreateTokenResponse `json:"token"`
}

// RegenerateTokenResponse represents a regenerated token response.
type RegenerateTokenResponse struct {
	Message string               `json:"message"`
	UserID  int64                `json:"user_id"`
	Token   *CreateTokenResponse `json:"token"`
}

// New creates a new database client using Goban's store layer.
func New(cfg *config.Config) (*Client, error) {
	// Build Goban config from our simplified config
	gobanCfg := gobanconfig.Config{
		DBType:     cfg.DBType,
		DBPath:     cfg.DBPath,
		DBHost:     cfg.DBHost,
		DBPort:     cfg.DBPort,
		DBUser:     cfg.DBUser,
		DBPassword: cfg.DBPassword,
		DBName:     cfg.DBName,
	}

	db, err := store.New(gobanCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &Client{store: db}, nil
}

// CreateUser creates a new user with the specified name and role.
func (c *Client) CreateUser(username, role string) (*CreateUserResponse, error) {
	userID, err := c.store.CreateUser(username, role)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user, err := c.store.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created user: %w", err)
	}

	// Generate token for the new user
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	tokenHash := hashToken(token)

	tokenID, err := c.store.CreateTokenWithUser(userID, username, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	// Get the created token details
	tokens, err := c.store.ListTokens()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	var createdToken *models.AgentToken
	for _, t := range tokens {
		if t.ID == tokenID {
			createdToken = &t
			break
		}
	}

	respUser := &User{
		ID:        user.ID,
		Name:      user.Name,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}

	respToken := &CreateTokenResponse{
		ID:        createdToken.ID,
		AgentName: createdToken.AgentName,
		TokenName: createdToken.TokenName,
		UserID:    createdToken.UserID,
		Token:     token, // Return the actual token (only shown at creation)
		CreatedAt: createdToken.CreatedAt.Format(time.RFC3339),
	}

	return &CreateUserResponse{User: respUser, Token: respToken}, nil
}

// ListUsers returns all users.
func (c *Client) ListUsers() ([]User, error) {
	users, err := c.store.ListUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	result := make([]User, len(users))
	for i, u := range users {
		result[i] = User{
			ID:        u.ID,
			Name:      u.Name,
			Role:      u.Role,
			CreatedAt: u.CreatedAt,
			UpdatedAt: u.UpdatedAt,
		}
	}

	return result, nil
}

// UpdateUserRole updates a user's role.
func (c *Client) UpdateUserRole(userID int64, role string) error {
	err := c.store.UpdateUserRole(userID, role)
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}
	return nil
}

// UpdateUserPassword updates a user's password with bcrypt hashing.
func (c *Client) UpdateUserPassword(userID int64, newPassword string) error {
	err := c.store.UpdateUserPassword(userID, newPassword)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}
	return nil
}

// DeleteUser removes a user by ID.
func (c *Client) DeleteUser(userID int64) error {
	err := c.store.DeleteUser(userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// RegenerateToken creates a new token for an existing user.
func (c *Client) RegenerateToken(userID int64) (*RegenerateTokenResponse, error) {
	user, err := c.store.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve user: %w", err)
	}

	// Delete existing token first (tokens are keyed by agent_name which equals username)
	tokens, err := c.store.ListTokens()
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	tokenDeleted := false
	for _, t := range tokens {
		// Match by agent_name (which is set to username when creating token)
		if t.AgentName == user.Name {
			count, delErr := c.store.DeleteToken(t.AgentName)
			if delErr != nil {
				return nil, fmt.Errorf("failed to delete old token: %w", delErr)
			}
			if count > 0 {
				tokenDeleted = true
			}
			break
		}
	}

	if !tokenDeleted {
		fmt.Printf("Note: No existing token found for user %s, creating new one\n", user.Name)
	}

	// Create new token
	newToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
	}
	tokenHash := hashToken(newToken)

	tokenID, err := c.store.CreateTokenWithUser(userID, user.Name, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create new token: %w", err)
	}

	// Retrieve the created token
	tokens, err = c.store.ListTokens()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve new token: %w", err)
	}

	var createdToken *models.AgentToken
	for _, t := range tokens {
		if t.ID == tokenID {
			createdToken = &t
			break
		}
	}

	if createdToken == nil {
		return nil, fmt.Errorf("failed to find newly created token (id=%d)", tokenID)
	}

	respToken := &CreateTokenResponse{
		ID:        createdToken.ID,
		AgentName: createdToken.AgentName,
		TokenName: createdToken.TokenName,
		UserID:    createdToken.UserID,
		Token:     newToken,
		CreatedAt: createdToken.CreatedAt.Format(time.RFC3339),
	}

	return &RegenerateTokenResponse{
		Message: "Token regenerated successfully",
		UserID:  userID,
		Token:   respToken,
	}, nil
}
