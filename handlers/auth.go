// Package handlers provides HTTP request handlers for Goban API.
package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/middleware"
	"goban/models"
	"goban/store"
	"goban/validation"
)

// LoginRequest represents a login request from the web UI.
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me,omitempty"`
}

// LoginResponse represents the response after successful login.
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	User        *models.User `json:"user"`
	ExpiresIn   int64        `json:"expires_in"`
}

// LogoutResponse represents the response after logout.
type LogoutResponse struct {
	Message string `json:"message"`
}

// Login handles user authentication and returns JWT token.
func Login(db store.TicketStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "invalid request body",
				"message": err.Error(),
			})
		}

		// Validate input
		if req.Username == "" || req.Password == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "missing username or password",
			})
		}

		if err := validation.ValidateAgentName(req.Username); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid username format",
			})
		}

		if len(req.Password) > 256 {
			return c.Status(400).JSON(fiber.Map{
				"error": "password exceeds maximum length",
			})
		}

		// Look up user by name (includes password hash)
		user, err := db.GetUserByName(req.Username)
		if err != nil {
			log.Printf("[AUTH] Login failed for user '%s': %v", req.Username, err)
			// Don't reveal whether user exists or not
			return c.Status(401).JSON(fiber.Map{
				"error": "invalid credentials",
			})
		}

		// Verify password using bcrypt
		valid, err := store.VerifyPassword(user.PasswordHash, req.Password)
		if err != nil {
			log.Printf("[AUTH] Password verification error for user '%s': %v", req.Username, err)
			return c.Status(500).JSON(fiber.Map{
				"error": "internal server error",
			})
		}
		if !valid {
			log.Printf("[AUTH] Login failed for user '%s': incorrect password", req.Username)
			return c.Status(401).JSON(fiber.Map{
				"error": "invalid credentials",
			})
		}

		// Generate JWT token with appropriate expiration
		token, expiresIn, err := auth.GenerateJWT(user, req.RememberMe)
		if err != nil {
			log.Printf("[AUTH] Failed to generate JWT for user '%s': %v", req.Username, err)
			return c.Status(500).JSON(fiber.Map{
				"error": "failed to generate token",
			})
		}

		log.Printf("[AUTH] Login successful for user '%s' (role: %s), expires in %d seconds",
			req.Username, user.Role, expiresIn)

		return c.JSON(LoginResponse{
			AccessToken: token,
			User: &models.User{
				ID:   user.ID,
				Name: user.Name,
				Role: user.Role,
			},
			ExpiresIn: expiresIn,
		})
	}
}

// Logout handles user logout. Since we use stateless JWT, this mainly logs the event.
func Logout() fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.Locals("username")
		if username != nil {
			log.Printf("[AUTH] Logout for user '%s'", username)
		}

		return c.JSON(LogoutResponse{
			Message: "logged out successfully",
		})
	}
}

// Me returns the current authenticated user's information.
func Me() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id")
		username := c.Locals("username")
		role := c.Locals("role")

		if userID == nil || username == nil {
			return c.Status(401).JSON(fiber.Map{
				"error": "not authenticated",
			})
		}

		return c.JSON(fiber.Map{
			"id":            userID,
			"name":          username,
			"role":          role,
			"authenticated": true,
		})
	}
}

// CheckAuthStatus returns whether a user is authenticated based on JWT or API token in header.
func CheckAuthStatus() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")

		if authHeader == "" {
			return c.JSON(fiber.Map{
				"authenticated": false,
			})
		}

		// Check for "Bearer " prefix
		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			return c.JSON(fiber.Map{
				"authenticated": false,
			})
		}

		tokenString := authHeader[7:] // Remove "Bearer " prefix

		// Try JWT first (web UI / human login tokens)
		claims, err := auth.VerifyJWT(tokenString)
		if err == nil {
			return c.JSON(fiber.Map{
				"authenticated": true,
				"user_id":       claims.UserID,
				"username":      claims.Username,
				"role":          claims.Role,
			})
		}

		// Fallback: API token path (SHA256-hashed tokens)
		if user, err := auth.ValidateTokenWithRole(tokenString); err == nil {
			return c.JSON(fiber.Map{
				"authenticated": true,
				"user_id":       user.ID,
				"username":      user.Name,
				"role":          user.Role,
			})
		}

		return c.JSON(fiber.Map{
			"authenticated": false,
		})
	}
}

// Refresh handles POST /api/auth/refresh — issues a new JWT if the submitted token's signature is valid
// and its issued-at time falls within the configured refresh grace period window.
func Refresh(db store.TicketStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")

		if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			return c.Status(401).JSON(fiber.Map{
				"error": "missing or invalid authorization header",
			})
		}

		tokenString := authHeader[7:]

		// Parse the token without expiration validation (signature check only)
		claims, err := auth.ParseJWT(tokenString)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{
				"error":   "invalid or expired token",
				"message": err.Error(),
			})
		}

		// Check if the token is within the refresh grace period window
		var issuedAt time.Time
		if claims.IssuedAt != nil {
			issuedAt = claims.IssuedAt.Time
		} else {
			return c.Status(401).JSON(fiber.Map{
				"error":   "invalid token",
				"message": "Token is missing issued-at time",
			})
		}
		now := time.Now()
		gracePeriod := auth.JWTRefreshGracePeriodDuration()
		if now.Sub(issuedAt) > gracePeriod {
			return c.Status(401).JSON(fiber.Map{
				"error": "token expired beyond grace period",
				"message": fmt.Sprintf("Token was issued %.1f days ago; refresh window has closed (grace period: %s)",
					now.Sub(issuedAt).Hours()/24, gracePeriod),
			})
		}

		// Look up user from database to get current role
		user, err := db.GetUserByName(claims.Username)
		if err != nil || user == nil {
			return c.Status(401).JSON(fiber.Map{
				"error":   "user not found",
				"message": fmt.Sprintf("User '%s' could not be looked up in the database", claims.Username),
			})
		}

		// Generate a fresh token with full validity period
		newToken, expiresIn, err := auth.GenerateJWT(user, false)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "failed to generate token",
			})
		}

		log.Printf("[AUTH] Token refreshed for user '%s' (role: %s), expires in %d seconds",
			user.Name, user.Role, expiresIn)

		return c.JSON(LoginResponse{
			AccessToken: newToken,
			User: &models.User{
				ID:   user.ID,
				Name: user.Name,
				Role: user.Role,
			},
			ExpiresIn: expiresIn,
		})
	}
}

// =============================================================================
// Route Registration for Auth Endpoints
// =============================================================================

// RegisterAuthRoutes sets up authentication endpoints.
func RegisterAuthRoutes(app *fiber.App, db store.TicketStore) {
	api := app.Group("/api")
	authGroup := api.Group("/auth")

	// Public endpoints (no auth required) — apply rate limiting to login
	loginGroup := authGroup.Group("")
	loginGroup.Use(middleware.StrictLimiter())
	loginGroup.Post("/login", Login(db))

	authGroup.Get("/check", CheckAuthStatus())
	authGroup.Post("/refresh", Refresh(db))

	// Protected endpoint (requires JWT)
	authProtected := authGroup.Group("")
	authProtected.Use(auth.AuthMiddlewareWithUser)
	authProtected.Post("/logout", Logout())
	authProtected.Get("/me", Me())
}
