// Package store provides database abstraction for Goban persistence.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"goban/models"
)

// MemoryGameStore implements GameStore using an in-memory map with mutex protection.
type MemoryGameStore struct {
	mu    sync.RWMutex
	games map[string]*models.Game
}

// NewMemoryGameStore creates and returns a new MemoryGameStore instance.
func NewMemoryGameStore() *MemoryGameStore {
	return &MemoryGameStore{
		games: make(map[string]*models.Game),
	}
}

// generateID creates a short, unique hex identifier using crypto/rand.
func generateID() string {
	b := make([]byte, 8) // 64 bits = 16 hex chars — sufficient for in-memory game IDs
	rand.Read(b)
	return "go-" + hex.EncodeToString(b)
}

// CreateGame stores a new game with an auto-generated ID and initialized board.
func (s *MemoryGameStore) CreateGame(game *models.Game) *models.Game {
	s.mu.Lock()
	defer s.mu.Unlock()

	newGame := models.NewGame(game.BoardSize)
	newGame.ID = generateID()
	newGame.InitBoard()

	if game.CurrentTurn != "" {
		newGame.CurrentTurn = game.CurrentTurn
	}

	s.games[newGame.ID] = newGame
	return newGame
}

// GetGame retrieves a game by its unique identifier.
func (s *MemoryGameStore) GetGame(id string) (*models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	game, ok := s.games[id]
	if !ok {
		return nil, fmt.Errorf("game not found: %s", id)
	}
	return game, nil
}

// UpdateGame persists changes to an existing game (board state, turn, prisoners, etc.).
func (s *MemoryGameStore) UpdateGame(game *models.Game) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.games[game.ID]; !ok {
		return fmt.Errorf("game not found: %s", game.ID)
	}
	s.games[game.ID] = game
	return nil
}

// ListGames returns all games currently stored.
func (s *MemoryGameStore) ListGames() ([]*models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	games := make([]*models.Game, 0, len(s.games))
	for _, game := range s.games {
		games = append(games, game)
	}
	return games, nil
}
