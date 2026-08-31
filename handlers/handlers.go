// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"sync"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/config"
	"goban/models"
	"goban/services"
	"goban/sse"
	"goban/store"
)

// TicketStore is a local alias for store.TicketStore.
type TicketStore = store.TicketStore

// Pagination holds pagination parameters for queries (alias for store.Pagination).
type Pagination = store.Pagination

var (
	boardStates   map[string]*models.BoardState
	knownBoardIDs map[string]struct{}
	dbStore       store.TicketStore
	mu            sync.RWMutex
)

// RegisterRoutes sets up all API endpoints using modular register functions.
func RegisterRoutes(app *fiber.App, db store.TicketStore, boards []config.Board) {
	dbStore = db

	if db != nil {
		auth.SetStore(db)
		claimService = services.NewClaimService(db)
		InitMoveService(db)
		InitReleaseService(db)
		userService = services.NewUserService(db)
		adminUserService = userService
	}

	// Initialize SSE subsystem with 100 event buffer
	sse.Init(100)

	InitBoards(boards, db)

	// Register all route groups (order matters: more specific routes first)
	RegisterAuthRoutes(app, db)     // Auth endpoints (login/logout/check)
	RegisterArchiveRoutes(app)      // Archive routes - must come before generic patterns to avoid prefix conflicts
	RegisterClaimRoutes(app)        // Claim endpoint needs to be registered before generic ticket routes
	RegisterActivityRoutes(app)     // Activity log retrieval - must come before generic :id routes
	RegisterMoveRoutesV1(app)       // Move v1 endpoint with permissions
	RegisterReleaseRoutes(app)      // Release endpoint for unassigning tickets
	RegisterRegistrationRoutes(app) // Self-registration endpoint (no auth required)
	RegisterAdminRoutes(app)        // Admin endpoints (require HUMAN_ADMIN role)
	RegisterSSERoutes(app)
	RegisterBoardRoutes(app)
	RegisterTicketRoutes(app, dbStore)
	RegisterCommentRoutes(app)
	RegisterSubtaskRoutes(app)
	RegisterLinkRoutes(app)
	RegisterRunRoutes(app)

	// Initialize Go game store and register routes
	InitGameStore(store.NewMemoryGameStore())
	RegisterGoGameRoutes(app)
}

// ClaimStore interface for claim service operations - now just an alias to store.TicketStore.
type ClaimStore = store.TicketStore
