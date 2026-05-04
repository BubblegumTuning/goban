// Package models provides core data structures for the Goban application.
package models

import "time"

// Point represents a coordinate on the Go board (row, col).
type Point struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// MoveRecord records a single move played during a game.
type MoveRecord struct {
	Player   string  `json:"player"`   // "black" or "white"
	Point    Point   `json:"point"`    // Where the stone was placed
	Captures []Point `json:"captures"` // Opponent stones captured by this move
	IsPass   bool    `json:"is_pass"`  // Whether this was a pass instead of a placement
}

// Game represents an active Go board game.
type Game struct {
	ID            string      `json:"id"`             // Unique game identifier (join code)
	BoardSize     int         `json:"board_size"`     // Board dimension (9, 13, or 19). Default: 19.
	CurrentTurn   string      `json:"current_turn"`   // "black" or "white" — whose turn it is.
	BlackPrisoners int        `json:"black_prisoners"` // Stones captured by Black (counted toward Black's score)
	WhitePrisoners int        `json:"white_prisoners"` // Stones captured by White (counted toward White's score)
	KoPoint       *Point      `json:"ko_point,omitempty"` // Current ko restriction point, if any. Nil when no ko active.
	Board         [][]int     `json:"board"`          // 2D grid: 0=empty, 1=black, 2=white. Dimensions: BoardSize x BoardSize.
	MoveHistory   []MoveRecord `json:"move_history"`  // Chronological record of all moves played.
	Status        string      `json:"status"`         // "playing", "passed" (one pass), "resigned", or "completed" (double-pass).
	Passes        int         `json:"passes"`         // Consecutive pass count (resets to 0 on a real move). Two consecutive passes end the game.
	CreatedAt     time.Time   `json:"created_at"`     // Game creation timestamp.
	UpdatedAt     time.Time   `json:"updated_at"`     // Last modification timestamp.
}

// NewGame initializes a fresh Go game with the given board size and status.
func NewGame(boardSize int) *Game {
	if boardSize <= 0 || boardSize > 19 {
		boardSize = 19 // Default to standard 19x19
	}
	return &Game{
		BoardSize:     boardSize,
		CurrentTurn:   "black",
		Status:        "playing",
		Passes:        0,
		MoveHistory:   make([]MoveRecord, 0),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// InitBoard populates the Board field with an empty grid of the configured size.
func (g *Game) InitBoard() {
	g.Board = make([][]int, g.BoardSize)
	for i := range g.Board {
		g.Board[i] = make([]int, g.BoardSize) // All zeros (empty) by default
	}
}

// IsValidPoint checks if the given coordinates are within board bounds.
func (g *Game) IsValidPoint(row, col int) bool {
	return row >= 0 && row < g.BoardSize && col >= 0 && col < g.BoardSize
}
