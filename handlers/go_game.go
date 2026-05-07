// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"goban/middleware"
	"goban/models"
	"goban/services"
	"goban/sse"
	"goban/store"
)

var gameStore store.GameStore

// InitGameStore initializes the Go game handler with a GameStore implementation.
func InitGameStore(gs store.GameStore) {
	gameStore = gs
}

// RegisterGoGameRoutes registers all /api/v1/go/ endpoints.
func RegisterGoGameRoutes(app *fiber.App) {
	goAPI := app.Group("/api/v1/go")
	goAPI.Use(middleware.GameLimiter())

	goAPI.Post("/games", createGame)
	goAPI.Get("/games/:id", getGame)
	goAPI.Post("/games/:id/move", playMove)
	goAPI.Post("/games/:id/pass", passMove)
	goAPI.Post("/games/:id/resign", resignGame)
	goAPI.Get("/games/:id/score", getScore)
	goAPI.Get("/games/:id/events", gameSSEEvents)
}

// emitGameEvent sends an SSE event for a Go game state change.
func emitGameEvent(eventType, gameId string, payload fiber.Map) {
	payload["game_id"] = gameId
	sse.Emit(eventType, gameId, "", payload)
}

// gameSSEEvents serves the SSE endpoint scoped to a specific game using streaming via SetBodyStreamWriter.
func gameSSEEvents(c *fiber.Ctx) error {
	gameId := c.Params("id")

	_, err := gameStore.GetGame(gameId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "game not found"})
	}

	sub, err := sse.Subscribe("", 10) // Subscribe to all boards (empty board_id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create subscription"})
	}

	fctx := c.Context()
	fctx.Response.Header.SetContentType("text/event-stream")
	fctx.Response.Header.Set("Cache-Control", "no-cache")
	fctx.Response.Header.Set("Connection", "keep-alive")
	fctx.Response.Header.Set("Access-Control-Allow-Origin", "*")

	// Use SetBodyStreamWriter for unbuffered streaming.
	// Return nil immediately so fasthttp sends headers and invokes the body writer.
	fctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			w.Flush()
			sse.Unsubscribe(sub)
		}()

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\",\"game_id\":\"%s\"}\n\n", gameId)
		w.Flush()

		for {
			select {
			case event, ok := <-sub.Events:
				if !ok {
					return // Channel closed
				}

				// Filter: only forward events for this specific game_id
				gameIdFromEvent, _ := event.Payload["game_id"].(string)
				if gameIdFromEvent != gameId {
					continue
				}

				data, _ := json.Marshal(event)
				fmt.Fprintf(w, "event: update\ndata: %s\n\n", data)
				w.Flush()

			case <-fctx.Done():
				return // Client disconnected or server shutting down
			}
		}
	})

	// Return immediately — do NOT block on a done channel here.
	// Blocking would prevent headers from being sent, causing a deadlock.
	return nil
}

// createGameRequest represents the optional request body for creating a game.
type createGameRequest struct {
	BoardSize int `json:"board_size"` // 9, 13, or 19 (default: 19)
}

func createGame(c *fiber.Ctx) error {
	var req createGameRequest
	if err := c.BodyParser(&req); err != nil {
		req.BoardSize = 19 // Default if no body provided
	}

	game := &models.Game{BoardSize: req.BoardSize}
	created := gameStore.CreateGame(game)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"game_id":    created.ID,
		"board_size": created.BoardSize,
		"status":     created.Status,
	})
}

func getGame(c *fiber.Ctx) error {
	id := c.Params("id")
	game, err := gameStore.GetGame(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "game not found"})
	}
	return c.JSON(game)
}

// playMoveRequest represents the request body for playing a stone.
type playMoveRequest struct {
	Row    int `json:"row"`
	Col    int `json:"col"`
	Player string `json:"player"` // "black" or "white"
}

func playMove(c *fiber.Ctx) error {
	id := c.Params("id")
	game, err := gameStore.GetGame(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "game not found"})
	}

	var req playMoveRequest
	if err = c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Validate turn
	expectedPlayer := game.CurrentTurn
	if req.Player != expectedPlayer {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error":        "wrong turn",
			"current_turn": expectedPlayer,
		})
	}

	// Validate point is within bounds
	if !game.IsValidPoint(req.Row, req.Col) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "point outside board"})
	}

	// Convert player name to color constant
	color := services.Black
	if req.Player == "white" {
		color = services.White
	}

	// Validate move using rules engine
	err = services.ValidateMove(game.Board, color, req.Row, req.Col, game.KoPoint)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Save board state before placement (for ko detection)
	prevBoard := make([][]int, len(game.Board))
	for i := range prevBoard {
		prevBoard[i] = make([]int, len(game.Board[i]))
		copy(prevBoard[i], game.Board[i])
	}

	// Place stone and detect captures
	newBoard, captures := services.PlaceStone(game.Board, color, req.Row, req.Col)
	game.Board = newBoard

	// Update prisoners based on captures
	captureCount := len(captures)
	if captureCount > 0 {
		if req.Player == "black" {
			game.BlackPrisoners += captureCount // Black captured White stones
		} else {
			game.WhitePrisoners += captureCount // White captured Black stones
		}
	}

	// Detect ko point from this move
	playerMove := models.Point{Row: req.Row, Col: req.Col}
	koPoint := services.GetKoPoint(prevBoard, newBoard, playerMove)
	game.KoPoint = koPoint

	// Record the move
	moveRecord := models.MoveRecord{
		Player:   req.Player,
		Point:    playerMove,
		Captures: captures,
		IsPass:   false,
	}
	game.MoveHistory = append(game.MoveHistory, moveRecord)

	// Reset pass counter on real moves
	game.Passes = 0

	// Switch turn
	if req.Player == "black" {
		game.CurrentTurn = "white"
	} else {
		game.CurrentTurn = "black"
	}

	// Persist the updated game state
	if err = gameStore.UpdateGame(game); err != nil {
		log.Printf("Error updating game %s: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save game"})
	}

	// Emit SSE event for real-time sync
	emitGameEvent("go_move", id, fiber.Map{
		"player":       req.Player,
		"row":          req.Row,
		"col":          req.Col,
		"captures":     captureCount,
		"current_turn": game.CurrentTurn,
	})

	return c.JSON(game)
}

func passMove(c *fiber.Ctx) error {
	id := c.Params("id")
	game, err := gameStore.GetGame(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "game not found"})
	}

	var req playMoveRequest
	if err := c.BodyParser(&req); err != nil {
		log.Printf("[GO_GAME] Invalid request body in passMove for game %s: %v", id, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	player := req.Player
	if player == "" {
		player = game.CurrentTurn
	}

	game.Passes++

	// Record pass in move history
	passRecord := models.MoveRecord{
		Player:   player,
		Point:    models.Point{},
		Captures: []models.Point{},
		IsPass:   true,
	}
	game.MoveHistory = append(game.MoveHistory, passRecord)

	// Two consecutive passes end the game
	if game.Passes >= 2 {
		game.Status = "completed"
	} else if game.Status == "playing" {
		game.Status = "passed"
	}

	// Clear ko when a player passes (ko restriction lifted)
	game.KoPoint = nil

	// Switch turn
	if player == "black" {
		game.CurrentTurn = "white"
	} else {
		game.CurrentTurn = "black"
	}

	if err = gameStore.UpdateGame(game); err != nil {
		log.Printf("Error updating game %s: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save game"})
	}

	// Emit SSE event for real-time sync
	emitGameEvent("go_pass", id, fiber.Map{
		"player":       player,
		"passes":       game.Passes,
		"status":       game.Status,
		"current_turn": game.CurrentTurn,
	})

	return c.JSON(fiber.Map{
		"game":   game,
		"passes": game.Passes,
	})
}

func resignGame(c *fiber.Ctx) error {
	id := c.Params("id")
	game, err := gameStore.GetGame(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "game not found"})
	}

	var req struct {
		Player string `json:"player"`
	}
	if err := c.BodyParser(&req); err != nil {
		log.Printf("[GO_GAME] Invalid request body in resignGame for game %s: %v", id, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	player := game.CurrentTurn
	if req.Player != "" {
		player = req.Player
	}

	game.Status = "resigned"

	resignRecord := models.MoveRecord{
		Player:   player,
		Point:    models.Point{},
		Captures: []models.Point{},
		IsPass:   false,
	}
	game.MoveHistory = append(game.MoveHistory, resignRecord)

	if err = gameStore.UpdateGame(game); err != nil {
		log.Printf("Error updating game %s: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save game"})
	}

	// Emit SSE event for real-time sync
	opponent := "white"
	if player == "black" {
		opponent = "black"
	}
	emitGameEvent("go_resign", id, fiber.Map{
		"resigned":  player,
		"winner":    opponent,
		"status":    game.Status,
	})

	return c.JSON(fiber.Map{
		"game":     game,
		"resigned": player,
		"winner":   opponent,
	})
}

func getScore(c *fiber.Ctx) error {
	id := c.Params("id")
	game, err := gameStore.GetGame(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "game not found"})
	}

	result := services.CalculateScore(game.Board, game.BlackPrisoners, game.WhitePrisoners)

	return c.JSON(fiber.Map{
		"game_id": id,
		"score":   result,
	})
}
