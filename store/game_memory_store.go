// Package store provides database abstraction for Goban persistence.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

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
	if _, err := rand.Read(b); err != nil {
		log.Printf("[WARN] crypto/rand failed during game ID generation: %v", err)
		return fmt.Sprintf("go-%d", time.Now().UnixNano())
	}
	return "go-" + hex.EncodeToString(b)
}

// cloneGame creates a deep copy of a Game to prevent race conditions when handlers
// read-modify-write game state. Each handler gets its own independent copy.
func cloneGame(src *models.Game) *models.Game {
	dst := *src // Shallow copy of scalar fields

	// Deep copy Board ([][]int)
	dst.Board = make([][]int, len(src.Board))
	for i := range dst.Board {
		dst.Board[i] = make([]int, len(src.Board[i]))
		copy(dst.Board[i], src.Board[i])
	}

	// Deep copy MoveHistory ([]MoveRecord with nested Captures slices)
	if src.MoveHistory != nil {
		dst.MoveHistory = make([]models.MoveRecord, len(src.MoveHistory))
		for i := range src.MoveHistory {
			dst.MoveHistory[i] = src.MoveHistory[i]
			if src.MoveHistory[i].Captures != nil {
				capCopy := slices.Clone(src.MoveHistory[i].Captures)
				dst.MoveHistory[i].Captures = capCopy
			}
		}
	}

	// Deep copy KoPoint pointer
	if src.KoPoint != nil {
		kp := *src.KoPoint
		dst.KoPoint = &kp
	}

	return &dst
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
	return cloneGame(newGame)
}

// GetGame retrieves a game by its unique identifier.
// Returns a deep copy to prevent concurrent modification races between handlers.
func (s *MemoryGameStore) GetGame(id string) (*models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	game, ok := s.games[id]
	if !ok {
		return nil, fmt.Errorf("game not found: %s", id)
	}
	return cloneGame(game), nil
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
		games = append(games, cloneGame(game))
	}
	return games, nil
}
