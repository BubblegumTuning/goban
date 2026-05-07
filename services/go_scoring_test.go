package services

import (
	"testing"
)

// --- CalculateScore tests ---

func TestCalculateScore(t *testing.T) {
	t.Run("empty board gives all territory to neither - draw with komi", func(t *testing.T) {
		board := newEmptyBoard(9)
		result := CalculateScore(board, 0, 0)
		if result.BlackTerritory != 0 {
			t.Errorf("expected 0 black territory on empty board, got %d", result.BlackTerritory)
		}
		if result.WhiteTerritory != 0 {
			t.Errorf("expected 0 white territory on empty board, got %d", result.WhiteTerritory)
		}
		// Empty board has no boundary colors -> no territory assigned to either side
		// With komi = 6.5, White wins by default
		if result.Winner != "white" {
			t.Errorf("expected white to win on empty board (komi), got %s", result.Winner)
		}
	})

	t.Run("simple black territory surrounded completely", func(t *testing.T) {
		board := newEmptyBoard(5)
		// Surround center (2,2) with Black stones using diamond pattern
		board[1][2] = Black // above center
		board[3][2] = Black // below center
		board[2][1] = Black // left of center
		board[2][3] = Black // right of center
		// Fill the rest with White so only (2,2) is black territory
		for r := 0; r < 5; r++ {
			for c := 0; c < 5; c++ {
				if board[r][c] == Empty && !(r == 2 && c == 2) {
					board[r][c] = White // fill all other empties with white, preserve center
				}
			}
		}
		result := CalculateScore(board, 0, 0)

		if result.BlackTerritory != 1 {
			t.Errorf("expected 1 black territory point, got %d", result.BlackTerritory)
		}
	})

	t.Run("white territory surrounded completely", func(t *testing.T) {
		board := newEmptyBoard(5)
		// Surround center (2,2) with White stones using diamond pattern
		board[1][2] = White // above center
		board[3][2] = White // below center
		board[2][1] = White // left of center
		board[2][3] = White // right of center
		// Fill the rest with Black so only (2,2) is white territory
		for r := 0; r < 5; r++ {
			for c := 0; c < 5; c++ {
				if board[r][c] == Empty && !(r == 2 && c == 2) {
					board[r][c] = Black // fill all other empties with black, preserve center
				}
			}
		}
		result := CalculateScore(board, 0, 0)

		if result.WhiteTerritory != 1 {
			t.Errorf("expected 1 white territory point, got %d", result.WhiteTerritory)
		}
	})

	t.Run("neutral territory when both colors border region", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[1][2] = Black // above center empty region
		board[3][2] = White // below center empty region
		result := CalculateScore(board, 0, 0)

		if result.BlackTerritory != 0 {
			t.Errorf("expected 0 black territory for neutral region, got %d", result.BlackTerritory)
		}
		if result.WhiteTerritory != 0 {
			t.Errorf("expected 0 white territory for neutral region, got %d", result.WhiteTerritory)
		}
	})

	t.Run("territory counts are correct with prisoners and komi", func(t *testing.T) {
		board := newEmptyBoard(5)
		// Surround center (2,2) with Black stones using diamond pattern
		board[1][2] = Black; board[3][2] = Black
		board[2][1] = Black; board[2][3] = Black
		for r := 0; r < 5; r++ {
			for c := 0; c < 5; c++ {
				if board[r][c] == Empty && !(r == 2 && c == 2) {
					board[r][c] = White // fill rest with white, preserve center
				}
			}
		}

		result := CalculateScore(board, 0, 1) // black has 0 prisoners, white has 1 (captured stone)
		if result.BlackTerritory != 1 {
			t.Errorf("expected 1 black territory, got %d", result.BlackTerritory)
		}
		if result.WhitePrisoners != 1 {
			t.Errorf("expected 1 white prisoner, got %d", result.WhitePrisoners)
		}
		if result.BlackTotal != float64(2) { // black territory (1) + white prisoners (1) = 2
			t.Errorf("expected black total of 2, got %.1f", result.BlackTotal)
		}
	})

	t.Run("black wins when score exceeds white with komi", func(t *testing.T) {
		board := newEmptyBoard(9)
		// Black ring from (2,2) to (6,6) surrounding 3x3 territory at (3-5,3-5) = 9 points
		for i := 2; i <= 6; i++ {
			board[2][i] = Black // top row of black ring
			board[6][i] = Black // bottom row
		}
		for i := 3; i <= 5; i++ {
			board[i][2] = Black // left column
			board[i][6] = Black // right column
		}
		// Fill outer area (outside ring) with White so it doesn't count as territory
		for r := 0; r < 9; r++ {
			for c := 0; c < 9; c++ {
				if board[r][c] == Empty && ((r < 2 || r > 6) || (c < 2 || c > 6)) {
					board[r][c] = White // outside ring -> white, not territory
				}
			}
		}
		result := CalculateScore(board, 0, 0)

		if result.BlackTerritory != 9 {
			t.Errorf("expected 9 black territory points, got %d", result.BlackTerritory)
		}
		if result.Winner != "black" {
			t.Errorf("expected black to win with 9 territory vs komi 6.5, got winner=%s blackTotal=%.1f whiteTotal=%.1f",
				result.Winner, result.BlackTotal, result.WhiteTotal)
		}
	})

	t.Run("white wins with komi when scores are close", func(t *testing.T) {
		board := newEmptyBoard(5)
		// White territory = 1 point; komi = 6.5 -> white total = 7.5
		// Black territory = 0; no prisoners -> black total = 0
		board[1][1] = White; board[1][2] = White; board[1][3] = White
		board[2][1] = White;                       board[2][3] = White
		board[3][1] = White; board[3][2] = White; board[3][3] = White

		result := CalculateScore(board, 0, 0)
		if result.Winner != "white" {
			t.Errorf("expected white to win, got %s (blackTotal=%.1f whiteTotal=%.1f)",
				result.Winner, result.BlackTotal, result.WhiteTotal)
		}
	})

	t.Run("komi value is always 6.5", func(t *testing.T) {
		board := newEmptyBoard(9)
		result := CalculateScore(board, 0, 0)
		if result.Komi != 6.5 {
			t.Errorf("expected komi of 6.5, got %.1f", result.Komi)
		}
	})

	t.Run("margin is correct positive difference", func(t *testing.T) {
		board := newEmptyBoard(9)
		// Black territory = 20 points -> black total = 20
		for i := 1; i <= 7; i++ {
			board[1][i] = Black
			board[7][i] = Black
		}
		for i := 2; i <= 6; i++ {
			board[i][1] = Black
			board[i][7] = Black
		}
		result := CalculateScore(board, 0, 0)

		if result.Winner != "black" {
			t.Errorf("expected black winner, got %s (blackTotal=%.1f whiteTotal=%.1f)",
				result.Winner, result.BlackTotal, result.WhiteTotal)
		}
		expectedMargin := result.BlackTotal - result.WhiteTotal
		if result.Margin != expectedMargin {
			t.Errorf("expected margin %.1f, got %.1f", expectedMargin, result.Margin)
		}
	})

	t.Run("prisoners affect totals correctly", func(t *testing.T) {
		board := newEmptyBoard(5)
		// Surround center (2,2) with Black stones using diamond pattern
		board[1][2] = Black; board[3][2] = Black
		board[2][1] = Black; board[2][3] = Black
		for r := 0; r < 5; r++ {
			for c := 0; c < 5; c++ {
				if board[r][c] == Empty && !(r == 2 && c == 2) {
					board[r][c] = White // fill rest with white, preserve center
				}
			}
		}

		result := CalculateScore(board, 0, 2) // black prisoners=0, white prisoners=2
		if result.BlackTotal != float64(3) { // territory(1) + white_prisoners(2) = 3
			t.Errorf("expected black total 3, got %.1f", result.BlackTotal)
		}
		if result.WhiteTotal != float64(6.5) { // territory(0) + komi(6.5) + black_prisoners(0) = 6.5
			t.Errorf("expected white total 6.5, got %.1f", result.WhiteTotal)
		}
	})

	t.Run("margin is always positive for winner", func(t *testing.T) {
		board := newEmptyBoard(9)
		result := CalculateScore(board, 0, 0)
		if result.Margin < 0 {
			t.Errorf("expected non-negative margin, got %.1f", result.Margin)
		}
	})
}

// --- floodFillTerritory tests ---

func TestFloodFillTerritory(t *testing.T) {
	t.Run("empty region bordered only by black returns black boundary", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[1][1] = Black; board[1][2] = Black; board[1][3] = Black
		board[2][1] = Black;                       board[2][3] = Black
		board[3][1] = Black; board[3][2] = Black; board[3][3] = Black

		visited := make(map[string]bool)
		boundaryColors := floodFillTerritory(board, 2, 2, visited)

		if !boundaryColors[Black] {
			t.Error("expected black in boundary colors")
		}
		if boundaryColors[White] {
			t.Error("did not expect white in boundary colors")
		}
	})

	t.Run("empty region bordered by both colors returns both", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[1][2] = Black // above center
		board[3][2] = White // below center
		// Center (2,2) and surrounding empty points are the region

		visited := make(map[string]bool)
		boundaryColors := floodFillTerritory(board, 2, 2, visited)

		if !boundaryColors[Black] {
			t.Error("expected black in boundary colors")
		}
		if !boundaryColors[White] {
			t.Error("expected white in boundary colors")
		}
	})

	t.Run("floodFillTerritory marks all connected empty points as visited", func(t *testing.T) {
		board := newEmptyBoard(5)
		// Border with Black stones, leaving a 3x3 empty center
		for i := 0; i < 5; i++ {
			if board[0][i] == Empty {
				board[0][i] = Black // top border - but (0,i) is edge
			}
		}
		// Actually just create a simple surrounded region
		board = newEmptyBoard(5)
		board[1][1] = Black; board[1][2] = Black; board[1][3] = Black
		board[2][1] = Black;                       board[2][3] = Black
		board[3][1] = Black; board[3][2] = Black; board[3][3] = Black

		visited := make(map[string]bool)
		floodFillTerritory(board, 2, 2, visited)

		if !visited["2:2"] {
			t.Error("expected (2,2) to be marked as visited")
		}
	})

	t.Run("region on board edge with only one boundary color", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[1][0] = Black // left of corner region
		board[0][1] = Black // above corner region

		visited := make(map[string]bool)
		boundaryColors := floodFillTerritory(board, 0, 0, visited)

		if !boundaryColors[Black] {
			t.Error("expected black in boundary colors for corner region")
		}
	})

	t.Run("completely empty board - no boundary colors", func(t *testing.T) {
		board := newEmptyBoard(5) // all zeros (empty)

		visited := make(map[string]bool)
		boundaryColors := floodFillTerritory(board, 2, 2, visited)

		if len(boundaryColors) > 0 {
			t.Errorf("expected no boundary colors on empty board, got %v", boundaryColors)
		}
	})
}

// --- countRegion tests ---

func TestCountRegion(t *testing.T) {
	t.Run("single empty point counts as 1", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[1][2] = Black; board[3][2] = Black
		board[2][1] = Black; board[2][3] = Black
		count := countRegion(board, 2, 2)
		if count != 1 {
			t.Errorf("expected count 1 for single enclosed point, got %d", count)
		}
	})

	t.Run("connected empty region counts all points", func(t *testing.T) {
		board := newEmptyBoard(7)
		// Black ring around a 3x3 area (but one side open to test connectivity)
		for i := 1; i <= 5; i++ {
			board[1][i] = Black // top of ring
			board[5][i] = Black // bottom of ring
		}
		for i := 2; i <= 4; i++ {
			board[i][1] = Black // left column
			board[i][5] = Black // right column
		}
		count := countRegion(board, 3, 3)
		if count != 9 {
			t.Errorf("expected count 9 for 3x3 region, got %d", count)
		}
	})

	t.Run("countRegion counts starting position plus all reachable empties", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[2][2] = Black // start from stone - countRegion counts it + 24 adjacent empties = 25
		count := countRegion(board, 2, 2)
		if count != 25 {
			t.Errorf("expected 25 (1 starting + 24 reachable empties), got %d", count)
		}
	})

	t.Run("region that spans entire board edge", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[4][0] = Black; board[4][1] = Black; board[4][2] = Black
		board[4][3] = Black; board[4][4] = Black // bottom row all black

		count := countRegion(board, 2, 2)
		// This counts the large empty region above the black wall
		if count < 1 {
			t.Errorf("expected at least 1 point in open region, got %d", count)
		}
	})
}
