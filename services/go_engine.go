// Package services provides business logic for Goban game operations.
package services

import (
	"errors"
	"strconv"

	"goban/models"
)

const (
	Empty = iota
	Black
	White
)

var (
	ErrPointOccupied = errors.New("point is already occupied")
	ErrSuicide       = errors.New("suicide move: stone would have no liberties and captures nothing")
	ErrKoViolation   = errors.New("ko violation: cannot immediately recapture the ko point")
)

// ValidateMove checks if placing a stone of the given color at (row, col) is legal.
// Implements Japanese Rules Articles 4-6: empty point check, suicide prevention, and ko rule.
func ValidateMove(board [][]int, color int, row, col int, koPoint *models.Point) error {
	boardSize := len(board)

	if !inBounds(row, col, boardSize) {
		return errors.New("point is outside the board")
	}

	if board[row][col] != Empty {
		return ErrPointOccupied
	}

	// Ko rule: cannot play on the current ko point
	if koPoint != nil && row == koPoint.Row && col == koPoint.Col {
		return ErrKoViolation
	}

	// Suicide prevention: simulate placement, check if stone has liberties after captures
	newBoard := deepCopyBoard(board)
	newBoard[row][col] = color

	opponent := 3 - color // Black=1 -> White=2, White=2 -> Black=1

	// First, remove any captured opponent groups (this may create liberties for our stone)
	for _, neighbor := range neighbors(row, col, boardSize) {
		if newBoard[neighbor.Row][neighbor.Col] == opponent {
			liberties := CountLiberties(newBoard, neighbor.Row, neighbor.Col)
			if liberties == 0 {
				removeGroup(newBoard, neighbor.Row, neighbor.Col, opponent)
			}
		}
	}

	// After captures, check if our stone has at least one liberty
	liberties := CountLiberties(newBoard, row, col)
	if liberties == 0 {
		return ErrSuicide
	}

	return nil
}

// GetKoPoint detects if a move creates a ko shape by comparing boards before and after.
// Returns the ko point (the point that can be immediately recaptured) or nil.
func GetKoPoint(prevBoard [][]int, newBoard [][]int, playerMove models.Point) *models.Point {
	boardSize := len(newBoard)

	// A ko occurs when a single stone is captured and the resulting board position
	// allows the opponent to immediately recapture at the same point, recreating the previous board.
	// We detect this by checking if exactly one stone was removed AND removing it from newBoard
	// would recreate prevBoard.

	// Count differences between boards (excluding the placed stone)
	diffs := 0
	var capturedPoint models.Point
	for r := range newBoard {
		for c := range newBoard[r] {
			if r == playerMove.Row && c == playerMove.Col {
				continue // Skip the newly placed stone
			}
			if prevBoard[r][c] != newBoard[r][c] {
				diffs++
				capturedPoint = models.Point{Row: r, Col: c}
			}
		}
	}

	// Ko requires exactly one stone captured at the placement point's adjacency
	// and the captured stone must have been a single-stone group in prevBoard
	if diffs == 1 {
		// Verify it was a single stone capture (the captured point had no other same-color neighbors)
		capturedColor := prevBoard[capturedPoint.Row][capturedPoint.Col]
		for _, neighbor := range neighbors(capturedPoint.Row, capturedPoint.Col, boardSize) {
			if prevBoard[neighbor.Row][neighbor.Col] == capturedColor {
				return nil // Not a single-stone capture, so not ko
			}
		}

		// The ko point is the placement position — opponent cannot immediately recapture there
		return &playerMove
	}

	return nil
}

// CountLiberties returns the number of liberties for the group containing (row, col).
// Uses iterative flood-fill BFS to avoid stack overflow on 19x19 boards.
func CountLiberties(board [][]int, row, col int) int {
	boardSize := len(board)
	if !inBounds(row, col, boardSize) || board[row][col] == Empty {
		return 0
	}

	color := board[row][col]
	liberties := make(map[string]bool)
	visited := make(map[string]bool)
	queue := []models.Point{{Row: row, Col: col}}
	visited[coordKey(row, col)] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, neighbor := range neighbors(current.Row, current.Col, boardSize) {
			if board[neighbor.Row][neighbor.Col] == Empty {
				liberties[coordKey(neighbor.Row, neighbor.Col)] = true
			} else if board[neighbor.Row][neighbor.Col] == color && !visited[coordKey(neighbor.Row, neighbor.Col)] {
				visited[coordKey(neighbor.Row, neighbor.Col)] = true
				queue = append(queue, neighbor)
			}
		}
	}

	return len(liberties)
}

// PlaceStone places a stone of the given color on the board and detects captures.
// Returns the updated board (as a deep copy) and the list of captured opponent points.
// The caller is responsible for validating the move before calling this function.
func PlaceStone(board [][]int, color int, row, col int) ([][]int, []models.Point) {
	boardSize := len(board)

	// Deep copy the board so we can modify it without affecting the original
	newBoard := deepCopyBoard(board)
	newBoard[row][col] = color

	opponent := 3 - color // Black=1 -> White=2, White=2 -> Black=1
	allCaptures := make([]models.Point, 0)

	// Check each adjacent opponent group for zero liberties (capture detection)
	for _, neighbor := range neighbors(row, col, boardSize) {
		if newBoard[neighbor.Row][neighbor.Col] == opponent {
			liberties := CountLiberties(newBoard, neighbor.Row, neighbor.Col)
			if liberties == 0 {
				captured := removeGroup(newBoard, neighbor.Row, neighbor.Col, opponent)
				allCaptures = append(allCaptures, captured...)
			}
		}
	}

	return newBoard, allCaptures
}

// GetGroup returns all points belonging to the same color group as (row, col).
// Uses iterative flood-fill BFS.
func GetGroup(board [][]int, row, col int) []models.Point {
	boardSize := len(board)
	if !inBounds(row, col, boardSize) || board[row][col] == Empty {
		return nil
	}

	color := board[row][col]
	group := make([]models.Point, 0)
	visited := make(map[string]bool)
	queue := []models.Point{{Row: row, Col: col}}
	visited[coordKey(row, col)] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		group = append(group, current)

		for _, neighbor := range neighbors(current.Row, current.Col, boardSize) {
			if board[neighbor.Row][neighbor.Col] == color && !visited[coordKey(neighbor.Row, neighbor.Col)] {
				visited[coordKey(neighbor.Row, neighbor.Col)] = true
				queue = append(queue, neighbor)
			}
		}
	}

	return group
}

// removeGroup removes all stones of a given color connected to (row, col) from the board.
// Returns the list of points that were removed.
func removeGroup(board [][]int, row, col int, color int) []models.Point {
	boardSize := len(board)
	removed := make([]models.Point, 0)
	visited := make(map[string]bool)
	queue := []models.Point{{Row: row, Col: col}}
	visited[coordKey(row, col)] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		board[current.Row][current.Col] = Empty
		removed = append(removed, current)

		for _, neighbor := range neighbors(current.Row, current.Col, boardSize) {
			if board[neighbor.Row][neighbor.Col] == color && !visited[coordKey(neighbor.Row, neighbor.Col)] {
				visited[coordKey(neighbor.Row, neighbor.Col)] = true
				queue = append(queue, neighbor)
			}
		}
	}

	return removed
}

// neighbors returns the up to 4 orthogonal neighbors of (row, col) within board bounds.
func neighbors(row, col, boardSize int) []models.Point {
	var result []models.Point
	deltas := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for _, d := range deltas {
		r, c := row+d[0], col+d[1]
		if inBounds(r, c, boardSize) {
			result = append(result, models.Point{Row: r, Col: c})
		}
	}
	return result
}

// inBounds checks if coordinates are within the board.
func inBounds(row, col, size int) bool {
	return row >= 0 && row < size && col >= 0 && col < size
}

// deepCopyBoard creates a complete independent copy of a board grid.
func deepCopyBoard(board [][]int) [][]int {
	boardSize := len(board)
	newBoard := make([][]int, boardSize)
	for i := range newBoard {
		newBoard[i] = make([]int, boardSize)
		copy(newBoard[i], board[i])
	}
	return newBoard
}

// coordKey generates a string key for a board coordinate (used in maps).
func coordKey(row, col int) string {
	return strconv.Itoa(row) + ":" + strconv.Itoa(col)
}
