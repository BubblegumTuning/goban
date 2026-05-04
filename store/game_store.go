// Package store provides database abstraction for Goban persistence.
package store

import "goban/models"

// GameStore defines the interface for game data persistence operations.
type GameStore interface {
	// CreateGame stores a new game and returns it with its generated ID and initialized board.
	CreateGame(game *models.Game) *models.Game

	// GetGame retrieves a game by its unique identifier.
	GetGame(id string) (*models.Game, error)

	// UpdateGame persists changes to an existing game (board state, turn, prisoners, etc.).
	UpdateGame(game *models.Game) error

	// ListGames returns all games currently stored.
	ListGames() ([]*models.Game, error)
}
