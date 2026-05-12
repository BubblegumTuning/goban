package handlers

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/config"
	"goban/middleware"
	"goban/models"
	"goban/services"
	"goban/validation"
)

// RegisterRequest represents the registration request body.
type RegisterRequest struct {
	AgentName string `json:"agent_name"`
}

// RegisterTokenResponse contains token info WITH the actual token value (for registration only).
type RegisterTokenResponse struct {
	ID        int64  `json:"id"`
	AgentName string `json:"agent_name"`
	TokenName string `json:"token_name"`
	UserID    int64  `json:"user_id"`
	Token     string `json:"token"` // Full token returned ONCE at creation
	CreatedAt string `json:"created_at"`
}

// RegisterResponse contains the user and token information returned after successful registration.
type RegisterResponse struct {
	User  *models.User           `json:"user"`
	Token *RegisterTokenResponse `json:"token"` // Includes full token (only shown once)
}

var userService *services.UserService // Set via InitHandlers()

// HandleRegister handles POST /api/v1/register endpoint.
// Design decision: intentionally unauthenticated — this is the bootstrap mechanism for AI agents.
// Security controls in place: ModerateLimiter rate limit (10 req/min), agent name validation,
// duplicate-check returns 409, NORMAL_AI role only (no admin privileges), and rollback on token failure.
func HandleRegister(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request format"})
	}

	if err := validation.ValidateAgentName(req.AgentName); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	existingUser, _ := userService.GetUserByName(req.AgentName)
	if existingUser != nil {
		return c.Status(409).JSON(fiber.Map{
			"error":   "Agent already registered. Use existing credentials.",
			"user_id": existingUser.ID,
		})
	}

	// Create user with NORMAL_AI role (default for self-registration)
	user, err := userService.CreateUser(req.AgentName, "NORMAL_AI")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
	}

	// Generate API token linked to user
	token, err := auth.RegisterTokenForUser(user)
	if err != nil {
		// Rollback: delete user on token creation failure
		if delErr := userService.DeleteUser(user.ID); delErr != nil {
			log.Printf("[REGISTER.ERROR] Failed to rollback-delete user %s after token error: %v", req.AgentName, delErr)
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Return user and full token (only shown once!)
	tokenResp := &RegisterTokenResponse{
		ID:        token.ID,
		AgentName: token.AgentName,
		TokenName: token.TokenName,
		UserID:    token.UserID,
		Token:     token.Token, // Full token returned ONCE at creation
		CreatedAt: token.CreatedAt.Format(time.RFC3339),
	}
	resp := RegisterResponse{User: user, Token: tokenResp}
	return c.Status(201).JSON(resp)
}

// RegisterRegistrationRoutes sets up the registration endpoint.
// This endpoint does NOT require authentication as it's the bootstrap mechanism for new agents.
func RegisterRegistrationRoutes(app *fiber.App) {
	if config.Debug {
		log.Printf("DEBUG: Registered POST /api/v1/register with rate limiting")
	}
	app.Post("/api/v1/register", middleware.ModerateLimiter(), HandleRegister)
}
