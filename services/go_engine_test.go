package services

import (
	"testing"

	"goban/models"
)

// newEmptyBoard creates an empty NxN board.
func newEmptyBoard(size int) [][]int {
	board := make([][]int, size)
	for i := range board {
		board[i] = make([]int, size)
	}
	return board
}

// --- inBounds tests ---

func TestInBounds(t *testing.T) {
	tests := []struct {
		name string
		row  int
		col  int
		size int
		want bool
	}{
		{"center of 9x9", 4, 4, 9, true},
		{"top-left corner", 0, 0, 9, true},
		{"bottom-right corner", 8, 8, 9, true},
		{"row out of bounds negative", -1, 0, 9, false},
		{"col out of bounds negative", 0, -1, 9, false},
		{"row out of bounds high", 9, 0, 9, false},
		{"col out of bounds high", 0, 9, 9, false},
		{"both out of bounds", 10, 10, 9, false},
		{"1x1 board valid", 0, 0, 1, true},
		{"1x1 board invalid", 0, 1, 1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inBounds(tc.row, tc.col, tc.size)
			if got != tc.want {
				t.Errorf("inBounds(%d,%d,%d) = %v, want %v", tc.row, tc.col, tc.size, got, tc.want)
			}
		})
	}
}

// --- neighbors tests ---

func TestNeighbors(t *testing.T) {
	boardSize := 5

	t.Run("center has four neighbors", func(t *testing.T) {
		result := neighbors(2, 2, boardSize)
		if len(result) != 4 {
			t.Errorf("expected 4 neighbors for center, got %d", len(result))
		}
		expected := map[string]bool{
			"1:2": true, "3:2": true, "2:1": true, "2:3": true,
		}
		for _, p := range result {
			key := coordKey(p.Row, p.Col)
			if !expected[key] {
				t.Errorf("unexpected neighbor %v", p)
			}
		}
	})

	t.Run("corner has two neighbors", func(t *testing.T) {
		result := neighbors(0, 0, boardSize)
		if len(result) != 2 {
			t.Errorf("expected 2 neighbors for corner (0,0), got %d", len(result))
		}
	})

	t.Run("edge has three neighbors", func(t *testing.T) {
		result := neighbors(0, 2, boardSize)
		if len(result) != 3 {
			t.Errorf("expected 3 neighbors for edge (0,2), got %d", len(result))
		}
	})

	t.Run("1x1 board has no neighbors", func(t *testing.T) {
		result := neighbors(0, 0, 1)
		if len(result) != 0 {
			t.Errorf("expected 0 neighbors for 1x1 board, got %d", len(result))
		}
	})

	t.Run("2x2 board center (0,0) has two neighbors", func(t *testing.T) {
		result := neighbors(0, 0, 2)
		if len(result) != 2 {
			t.Errorf("expected 2 neighbors for (0,0) on 2x2, got %d", len(result))
		}
	})
}

// --- deepCopyBoard tests ---

func TestDeepCopyBoard(t *testing.T) {
	board := newEmptyBoard(5)
	board[2][2] = Black
	board[3][3] = White

	copy := deepCopyBoard(board)

	t.Run("copy has same values", func(t *testing.T) {
		if copy[2][2] != Black {
			t.Errorf("expected Black at [2][2], got %d", copy[2][2])
		}
		if copy[3][3] != White {
			t.Errorf("expected White at [3][3], got %d", copy[3][3])
		}
	})

	t.Run("copy is independent from original", func(t *testing.T) {
		copy[2][2] = Empty
		if board[2][2] != Black {
			t.Errorf("modifying copy affected original: board[2][2] changed to %d", board[2][2])
		}
	})

	t.Run("copy of empty board is all zeros", func(t *testing.T) {
		empty := newEmptyBoard(3)
		copied := deepCopyBoard(empty)
		for r := range copied {
			for c := range copied[r] {
				if copied[r][c] != Empty {
					t.Errorf("expected Empty at [%d][%d], got %d", r, c, copied[r][c])
				}
			}
		}
	})
}

// --- ValidateMove tests ---

func TestValidateMove(t *testing.T) {
	board := newEmptyBoard(9)

	t.Run("valid move on empty point", func(t *testing.T) {
		err := ValidateMove(board, Black, 4, 4, nil)
		if err != nil {
			t.Errorf("expected no error for valid move, got %v", err)
		}
	})

	t.Run("out of bounds returns error", func(t *testing.T) {
		err := ValidateMove(board, Black, -1, 0, nil)
		if err == nil {
			t.Error("expected error for out-of-bounds move")
		}
	})

	t.Run("occupied point returns ErrPointOccupied", func(t *testing.T) {
		board[4][4] = Black
		err := ValidateMove(board, White, 4, 4, nil)
		if err != ErrPointOccupied {
			t.Errorf("expected ErrPointOccupied, got %v", err)
		}
	})

	t.Run("ko violation returns ErrKoViolation", func(t *testing.T) {
		testBoard := newEmptyBoard(9)
		testBoard[3][3] = Black // stone elsewhere; (4,4) is the ko point and empty
		ko := &models.Point{Row: 4, Col: 4}
		err := ValidateMove(testBoard, White, 4, 4, ko)
		if err != ErrKoViolation {
			t.Errorf("expected ErrKoViolation, got %v", err)
		}
	})

	t.Run("suicide move returns ErrSuicide", func(t *testing.T) {
		// Create a position where placing Black at (2,2) would have no liberties
		// and capture nothing: surround center with White stones on 5x5 board
		small := newEmptyBoard(5)
		small[1][2] = White // above center
		small[3][2] = White // below center
		small[2][1] = White // left of center
		small[2][3] = White // right of center
		// Each white stone has liberties on the outside edges, so Black at (2,2) captures nothing
		err := ValidateMove(small, Black, 2, 2, nil)
		if err != ErrSuicide {
			t.Errorf("expected ErrSuicide for surrounded point with no captures, got %v", err)
		}
	})

	t.Run("move that captures opponent is valid despite filling last liberty", func(t *testing.T) {
		small := newEmptyBoard(5)
		small[3][2] = White // single white stone
		small[2][2] = Black // above - blocks one side
		small[3][1] = Black // left - blocks one side
		small[3][3] = Black // right - blocks one side
		// White at (3,2) has only liberty at (4,2). Black playing there captures it.
		err := ValidateMove(small, Black, 4, 2, nil)
		if err != nil {
			t.Errorf("expected no error for capturing move, got %v", err)
		}
	})
}

// --- CountLiberties tests ---

func TestCountLiberties(t *testing.T) {
	t.Run("single stone in center of empty board has 4 liberties", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		count := CountLiberties(board, 4, 4)
		if count != 4 {
			t.Errorf("expected 4 liberties for center stone, got %d", count)
		}
	})

	t.Run("single stone in corner has 2 liberties", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[0][0] = Black
		count := CountLiberties(board, 0, 0)
		if count != 2 {
			t.Errorf("expected 2 liberties for corner stone, got %d", count)
		}
	})

	t.Run("single stone on edge has 3 liberties", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[0][4] = Black
		count := CountLiberties(board, 0, 4)
		if count != 3 {
			t.Errorf("expected 3 liberties for edge stone, got %d", count)
		}
	})

	t.Run("group of two connected stones counts shared liberties once", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		board[4][5] = Black
		count := CountLiberties(board, 4, 4)
		if count != 6 {
			t.Errorf("expected 6 liberties for two connected stones, got %d", count)
		}
	})

	t.Run("empty point returns 0 liberties", func(t *testing.T) {
		board := newEmptyBoard(9)
		count := CountLiberties(board, 4, 4)
		if count != 0 {
			t.Errorf("expected 0 liberties for empty point, got %d", count)
		}
	})

	t.Run("out of bounds returns 0 liberties", func(t *testing.T) {
		board := newEmptyBoard(9)
		count := CountLiberties(board, -1, 0)
		if count != 0 {
			t.Errorf("expected 0 liberties for out-of-bounds, got %d", count)
		}
	})

	t.Run("stone with no liberties returns 0", func(t *testing.T) {
		board := newEmptyBoard(3)
		board[1][1] = Black
		board[0][1] = White // above
		board[2][1] = White // below
		board[1][0] = White // left
		board[1][2] = White // right
		count := CountLiberties(board, 1, 1)
		if count != 0 {
			t.Errorf("expected 0 liberties for completely surrounded stone, got %d", count)
		}
	})

	t.Run("group shares liberties correctly on 3x3", func(t *testing.T) {
		board := newEmptyBoard(3)
		board[1][1] = Black
		board[0][0] = Black
		count := CountLiberties(board, 0, 0)
		if count != 2 {
			t.Errorf("expected 2 liberties for group at (0,0)+(1,1) on 3x3, got %d", count)
		}
	})
}

// --- GetGroup tests ---

func TestGetGroup(t *testing.T) {
	t.Run("single stone returns group of one", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		group := GetGroup(board, 4, 4)
		if len(group) != 1 {
			t.Errorf("expected group size 1, got %d", len(group))
		}
	})

	t.Run("connected stones form single group", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		board[4][5] = Black
		board[5][4] = Black
		group := GetGroup(board, 4, 4)
		if len(group) != 3 {
			t.Errorf("expected group size 3 for connected stones, got %d", len(group))
		}
	})

	t.Run("diagonal stones are separate groups", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		board[5][5] = Black // diagonal, not connected orthogonally
		group := GetGroup(board, 4, 4)
		if len(group) != 1 {
			t.Errorf("expected group size 1 for diagonally separated stone, got %d", len(group))
		}
	})

	t.Run("empty point returns nil", func(t *testing.T) {
		board := newEmptyBoard(9)
		group := GetGroup(board, 4, 4)
		if group != nil {
			t.Errorf("expected nil for empty point, got %v", group)
		}
	})

	t.Run("different colors are separate groups", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		board[4][5] = White
		group := GetGroup(board, 4, 4)
		if len(group) != 1 {
			t.Errorf("expected group size 1 (only same color), got %d", len(group))
		}
	})

	t.Run("2x2 block of Black stones", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][4] = Black
		board[4][5] = Black
		board[5][4] = Black
		board[5][5] = Black
		group := GetGroup(board, 4, 4)
		if len(group) != 4 {
			t.Errorf("expected group size 4 for 2x2 block, got %d", len(group))
		}
	})

	t.Run("out of bounds returns nil", func(t *testing.T) {
		board := newEmptyBoard(9)
		group := GetGroup(board, -1, 0)
		if group != nil {
			t.Errorf("expected nil for out-of-bounds, got %v", group)
		}
	})
}

// --- PlaceStone tests ---

func TestPlaceStone(t *testing.T) {
	t.Run("places stone on empty board", func(t *testing.T) {
		board := newEmptyBoard(9)
		newBoard, captures := PlaceStone(board, Black, 4, 4)
		if newBoard[4][4] != Black {
			t.Errorf("expected Black at [4][4], got %d", newBoard[4][4])
		}
		if len(captures) != 0 {
			t.Errorf("expected no captures, got %d", len(captures))
		}
	})

	t.Run("original board is not modified", func(t *testing.T) {
		board := newEmptyBoard(9)
		_, _ = PlaceStone(board, Black, 4, 4)
		if board[4][4] != Empty {
			t.Errorf("original board was modified: [4][4] = %d", board[4][4])
		}
	})

	t.Run("captures opponent group with zero liberties", func(t *testing.T) {
		board := newEmptyBoard(5)
		board[2][1] = White // white stone to capture - single stone at center-left
		board[1][1] = Black // above white
		board[3][1] = Black // below white
		board[2][0] = Black // left of white (edge)
		// White at (2,1) has only liberty at (2,2). Playing Black there captures it.
		newBoard, captures := PlaceStone(board, Black, 2, 2)
		if len(captures) != 1 {
			t.Errorf("expected 1 capture, got %d", len(captures))
		}
		if newBoard[2][1] != Empty {
			t.Errorf("captured stone should be removed from board")
		}
	})

	t.Run("captures multiple stones in one group", func(t *testing.T) {
		// Surround a 2-stone white group with black, leaving only (1,3) as liberty
		board := newEmptyBoard(5)
		board[1][1] = White
		board[1][2] = White // connected pair
		board[0][1] = Black // above left
		board[2][1] = Black // below left
		board[0][2] = Black // above right
		board[2][2] = Black // below right
		board[1][0] = Black // left of left white - only liberty is (1,3)
		// Place at (1,3) to capture both whites
		newBoard, captures := PlaceStone(board, Black, 1, 3)
		if len(captures) != 2 {
			t.Errorf("expected 2 captures for connected group, got %d", len(captures))
		}
		if newBoard[1][1] != Empty || newBoard[1][2] != Empty {
			t.Error("captured stones should be removed from board")
		}
	})

	t.Run("no capture when opponent has liberties", func(t *testing.T) {
		board := newEmptyBoard(9)
		board[4][3] = White // adjacent to placement but has many liberties
		newBoard, captures := PlaceStone(board, Black, 4, 4)
		if len(captures) != 0 {
			t.Errorf("expected no captures when opponent has liberties, got %d", len(captures))
		}
		if newBoard[4][3] != White {
			t.Errorf("opponent stone should remain on board")
		}
	})

	t.Run("corner placement works correctly", func(t *testing.T) {
		board := newEmptyBoard(5)
		newBoard, _ := PlaceStone(board, Black, 0, 0)
		if newBoard[0][0] != Black {
			t.Errorf("expected Black at corner [0][0]")
		}
	})
}

// --- GetKoPoint tests ---

func TestGetKoPoint(t *testing.T) {
	t.Run("returns nil when no stone captured", func(t *testing.T) {
		prev := newEmptyBoard(9)
		newB := deepCopyBoard(prev)
		newB[4][4] = Black
		result := GetKoPoint(prev, newB, models.Point{Row: 4, Col: 4})
		if result != nil {
			t.Errorf("expected nil ko point when no capture, got %v", *result)
		}
	})

	t.Run("returns nil when multiple stones captured (not single-stone ko)", func(t *testing.T) {
		prev := newEmptyBoard(5)
		prev[2][1] = White
		prev[2][0] = White // two white stones removed
		newB := deepCopyBoard(prev)
		newB[2][1] = Empty
		newB[2][0] = Empty
		newB[3][2] = Black
		result := GetKoPoint(prev, newB, models.Point{Row: 3, Col: 2})
		if result != nil {
			t.Errorf("expected nil when multiple stones captured, got %v", *result)
		}
	})

	t.Run("returns ko point for single stone capture creating ko shape", func(t *testing.T) {
		prev := newEmptyBoard(5)
		prev[2][1] = White // single white stone - only liberty is (2,0)
		prev[1][1] = Black // above white
		prev[3][1] = Black // below white
		prev[2][2] = Black // right of white

		newB := deepCopyBoard(prev)
		newB[2][0] = Black // black plays at (2,0), adjacent to white
		newB[2][1] = Empty // captures the single white stone at (2,1)

		result := GetKoPoint(prev, newB, models.Point{Row: 2, Col: 0})
		if result == nil {
			t.Error("expected ko point for single stone capture")
		} else if result.Row != 2 || result.Col != 0 {
			t.Errorf("expected ko point at (2,0) [the placement], got (%d,%d)", result.Row, result.Col)
		}
	})

	t.Run("returns nil when captured stone had same-color neighbors", func(t *testing.T) {
		prev := newEmptyBoard(5)
		prev[2][1] = White
		prev[2][0] = White // connected white - not a single-stone capture
		newB := deepCopyBoard(prev)
		newB[2][1] = Empty
		newB[3][2] = Black
		result := GetKoPoint(prev, newB, models.Point{Row: 3, Col: 2})
		if result != nil {
			t.Errorf("expected nil when captured stone had same-color neighbors")
		}
	})
}
