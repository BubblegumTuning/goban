// Package services provides business logic for Goban game operations.
package services

import (
	"goban/models"
)

const komiPoints float64 = 6.5 // Standard komi for White under Japanese rules

// ScoreResult holds the complete scoring breakdown.
type ScoreResult struct {
	BlackTerritory int     `json:"black_territory"` // Empty points surrounded only by Black stones
	WhiteTerritory int     `json:"white_territory"` // Empty points surrounded only by White stones
	BlackPrisoners int     `json:"black_prisoners"` // Stones captured by Black (passed to caller)
	WhitePrisoners int     `json:"white_prisoners"` // Stones captured by White (passed to caller)
	Komi           float64 `json:"komi"`            // Points awarded to White (standard 6.5)
	BlackTotal     float64 `json:"black_total"`     // Black territory + White prisoners
	WhiteTotal     float64 `json:"white_total"`     // White territory + komi + Black prisoners
	Winner         string  `json:"winner"`          // "black", "white", or "draw"
	Margin         float64 `json:"margin"`          // Point difference (positive = winner's lead)
}

// CalculateScore performs Japanese territory scoring on the final board.
// Flood-fills empty regions to determine which color surrounds each territory.
// Prisoner counts are passed in from game state; komi is fixed at 6.5 for White.
func CalculateScore(board [][]int, blackPrisoners int, whitePrisoners int) ScoreResult {
	visited := make(map[string]bool)
	var blackTerritory, whiteTerritory int

	for r := range board {
		for c := range board[r] {
			if board[r][c] == Empty && !visited[coordKey(r, c)] {
				boundaryColors := floodFillTerritory(board, r, c, visited)

				if boundaryColors[Black] && !boundaryColors[White] {
					count := countRegion(board, r, c)
					blackTerritory += count
				} else if boundaryColors[White] && !boundaryColors[Black] {
					count := countRegion(board, r, c)
					whiteTerritory += count
				}
			}
		}
	}

	blackTotal := float64(blackTerritory + whitePrisoners)
	whiteTotal := float64(whiteTerritory+blackPrisoners) + komiPoints

	var winner string
	var margin float64
	if blackTotal > whiteTotal {
		winner = "black"
		margin = blackTotal - whiteTotal
	} else if whiteTotal > blackTotal {
		winner = "white"
		margin = whiteTotal - blackTotal
	} else {
		winner = "draw"
		margin = 0
	}

	return ScoreResult{
		BlackTerritory: blackTerritory,
		WhiteTerritory: whiteTerritory,
		BlackPrisoners: blackPrisoners,
		WhitePrisoners: whitePrisoners,
		Komi:           komiPoints,
		BlackTotal:     blackTotal,
		WhiteTotal:     whiteTotal,
		Winner:         winner,
		Margin:         margin,
	}
}

// floodFillTerritory finds all boundary colors surrounding an empty region starting at (row, col).
// Returns a map indicating which colors border the region.
func floodFillTerritory(board [][]int, row, col int, visited map[string]bool) map[int]bool {
	boardSize := len(board)
	boundaryColors := make(map[int]bool)
	queue := []models.Point{{Row: row, Col: col}}
	visited[coordKey(row, col)] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, neighbor := range neighbors(current.Row, current.Col, boardSize) {
			if board[neighbor.Row][neighbor.Col] == Empty && !visited[coordKey(neighbor.Row, neighbor.Col)] {
				visited[coordKey(neighbor.Row, neighbor.Col)] = true
				queue = append(queue, neighbor)
			} else if board[neighbor.Row][neighbor.Col] != Empty {
				boundaryColors[board[neighbor.Row][neighbor.Col]] = true
			}
		}
	}

	return boundaryColors
}

// countRegion counts the number of empty points in a connected region starting at (row, col).
func countRegion(board [][]int, row, col int) int {
	boardSize := len(board)
	count := 0
	regionVisited := make(map[string]bool)
	queue := []models.Point{{Row: row, Col: col}}
	regionVisited[coordKey(row, col)] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		count++

		for _, neighbor := range neighbors(current.Row, current.Col, boardSize) {
			if board[neighbor.Row][neighbor.Col] == Empty && !regionVisited[coordKey(neighbor.Row, neighbor.Col)] {
				regionVisited[coordKey(neighbor.Row, neighbor.Col)] = true
				queue = append(queue, neighbor)
			}
		}
	}

	return count
}
