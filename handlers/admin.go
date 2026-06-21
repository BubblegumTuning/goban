// Package handlers contains all HTTP request handlers for Goban API.
package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/auth"
	"goban/models"
	"goban/services"
	"goban/validation"
)

// AdminCreateUserRequest represents the request body for creating a user.
type AdminCreateUserRequest struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// AdminUpdateUserRoleRequest represents the request body for updating a user's role.
type AdminUpdateUserRoleRequest struct {
	Role string `json:"role"`
}

// AdminRegenerateTokenRequest represents the request body for regenerating a token.
type AdminRegenerateTokenRequest struct{}

// AdminDeleteUserRequest represents the optional force flag for deleting users.
type AdminDeleteUserRequest struct {
	Force bool `json:"force,omitempty"`
}

// AdminCreateUserResponse contains user and token info after creation.
type AdminCreateUserResponse struct {
	User  *models.User                `json:"user"`
	Token *AdminRegisterTokenResponse `json:"token"` // Includes full token (only shown once)
}

// AdminRegisterTokenResponse contains token info WITH the actual token value.
type AdminRegisterTokenResponse struct {
	ID        int64  `json:"id"`
	AgentName string `json:"agent_name"`
	TokenName string `json:"token_name"`
	UserID    int64  `json:"user_id"`
	Token     string `json:"token"` // Full token returned ONCE at creation
	CreatedAt string `json:"created_at"`
}

// AdminUserListResponse contains a list of users (without tokens).
type AdminUserListResponse struct {
	Users []models.User `json:"users"`
}

var adminUserService *services.UserService // Set via RegisterRoutes()

// AuthMiddlewareAdmin validates Bearer token and requires HUMAN_ADMIN role.
func AuthMiddlewareAdmin(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")

	if authHeader == "" {
		return auth.SendAuthError(c, "missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return auth.SendAuthError(c, "invalid authorization format (expected 'Bearer <token>')")
	}

	token := parts[1]
	if token == "" {
		return auth.SendAuthError(c, "empty token")
	}

	user, err := auth.ValidateTokenWithRole(token)
	if err != nil {
		return auth.SendAuthError(c, fmt.Sprintf("authentication failed: %v", err))
	}

	// Check for HUMAN_ADMIN role
	if !user.IsAdmin() {
		log.Printf("[ADMIN.AUDIT] Unauthorized admin access attempt - user=%s role=%s", user.Name, user.Role)
		return c.Status(403).JSON(fiber.Map{
			"error":   "forbidden",
			"message": "HUMAN_ADMIN role required for admin operations",
		})
	}

	// Store user in context for handlers to retrieve
	c.Locals("user", user)
	return c.Next()
}

// HandleAdminCreateUser handles POST /api/admin/users
func HandleAdminCreateUser(c *fiber.Ctx) error {
	if adminUserService == nil {
		return c.Status(503).JSON(fiber.Map{"error": "service unavailable"})
	}

	var req AdminCreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request format"})
	}

	if req.Username == "" {
		return c.Status(400).JSON(fiber.Map{"error": "username is required"})
	}

	if err := validation.ValidateAgentName(req.Username); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if req.Role == "" {
		req.Role = models.RoleNormalAI // Default role if not specified
	}

	// Validate role
	if req.Role != models.RoleHumanAdmin && req.Role != models.RoleOverseerAI && req.Role != models.RoleNormalAI {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid role",
			"message": fmt.Sprintf("role must be one of: %s, %s, %s",
				models.RoleHumanAdmin, models.RoleOverseerAI, models.RoleNormalAI),
		})
	}

	// Check if user already exists
	existingUser, _ := adminUserService.GetUserByName(req.Username)
	if existingUser != nil {
		return c.Status(409).JSON(fiber.Map{
			"error":   "user_already_exists",
			"message": fmt.Sprintf("User '%s' already exists with ID %d", req.Username, existingUser.ID),
		})
	}

	// Create user
	user, err := adminUserService.CreateUser(req.Username, req.Role)
	if err != nil {
		log.Printf("[ADMIN.ERROR] Failed to create user: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
	}

	// Generate API token linked to user
	token, err := auth.RegisterTokenForUser(user)
	if err != nil {
		// Rollback: delete user on token creation failure
		if delErr := adminUserService.DeleteUser(user.ID); delErr != nil {
			log.Printf("[ADMIN.ERROR] Failed to rollback-delete user %s after token error: %v", req.Username, delErr)
		}
		log.Printf("[ADMIN.ERROR] Failed to generate token for user %s: %v", req.Username, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Return user and full token (only shown once!)
	tokenResp := &AdminRegisterTokenResponse{
		ID:        token.ID,
		AgentName: token.AgentName,
		TokenName: token.TokenName,
		UserID:    token.UserID,
		Token:     token.Token, // Full token returned ONCE at creation
		CreatedAt: token.CreatedAt.Format(time.RFC3339),
	}

	log.Printf("[ADMIN.AUDIT] Created user: %s role=%s by admin", req.Username, req.Role)
	resp := AdminCreateUserResponse{User: user, Token: tokenResp}
	return c.Status(201).JSON(resp)
}

// HandleAdminListUsers handles GET /api/admin/users
func HandleAdminListUsers(c *fiber.Ctx) error {
	if adminUserService == nil {
		return c.Status(503).JSON(fiber.Map{"error": "service unavailable"})
	}

	users, err := adminUserService.ListUsers()
	if err != nil {
		log.Printf("[ADMIN.ERROR] Failed to list users: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list users"})
	}

	return c.JSON(AdminUserListResponse{Users: users})
}

// HandleAdminUpdateUserRole handles PATCH /api/admin/users/:id/role
func HandleAdminUpdateUserRole(c *fiber.Ctx) error {
	if adminUserService == nil {
		return c.Status(503).JSON(fiber.Map{"error": "service unavailable"})
	}

	userID, err := c.ParamsInt("id")
	if err != nil || userID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var req AdminUpdateUserRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request format"})
	}

	if req.Role == "" {
		return c.Status(400).JSON(fiber.Map{"error": "role is required"})
	}

	// Validate role
	if req.Role != models.RoleHumanAdmin && req.Role != models.RoleOverseerAI && req.Role != models.RoleNormalAI {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid role",
			"message": fmt.Sprintf("role must be one of: %s, %s, %s",
				models.RoleHumanAdmin, models.RoleOverseerAI, models.RoleNormalAI),
		})
	}

	// Get existing user
	existingUser, err := adminUserService.GetUserByID(int64(userID))
	if err != nil || existingUser == nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// Update role
	err = adminUserService.UpdateUserRole(int64(userID), req.Role)
	if err != nil {
		log.Printf("[ADMIN.ERROR] Failed to update user role: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update user role"})
	}

	adminUser := c.Locals("user").(*models.User)
	log.Printf("[ADMIN.AUDIT] Updated user role: %s -> %s by admin=%s", existingUser.Name, req.Role, adminUser.Name)
	return c.JSON(fiber.Map{
		"message":  "Role updated successfully",
		"user_id":  userID,
		"old_role": existingUser.Role,
		"new_role": req.Role,
	})
}

// HandleAdminDeleteUser handles DELETE /api/admin/users/:id
func HandleAdminDeleteUser(c *fiber.Ctx) error {
	if adminUserService == nil {
		return c.Status(503).JSON(fiber.Map{"error": "service unavailable"})
	}

	userID, err := c.ParamsInt("id")
	if err != nil || userID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var req AdminDeleteUserRequest
	if err := c.BodyParser(&req); err != nil {
		// Body is optional for DELETE, just use defaults
		req.Force = false
	}

	// Get existing user
	existingUser, err := adminUserService.GetUserByID(int64(userID))
	if err != nil || existingUser == nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// Check for active tickets (unless force is true)
	if !req.Force {
		tickets, _ := adminUserService.GetTicketsByAssignee(existingUser.Name)
		if len(tickets) > 0 {
			return c.Status(409).JSON(fiber.Map{
				"error":        "user_has_active_tickets",
				"message":      fmt.Sprintf("User '%s' has %d active tickets. Use force=true to delete anyway.", existingUser.Name, len(tickets)),
				"ticket_count": len(tickets),
			})
		}
	}

	// Delete user (cascades token deletion via DB constraints)
	err = adminUserService.DeleteUser(int64(userID))
	if err != nil {
		log.Printf("[ADMIN.ERROR] Failed to delete user: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete user"})
	}

	adminUser := c.Locals("user").(*models.User)
	log.Printf("[ADMIN.AUDIT] Deleted user: %s (force=%v) by admin=%s", existingUser.Name, req.Force, adminUser.Name)
	return c.JSON(fiber.Map{
		"message":  "User deleted successfully",
		"user_id":  userID,
		"username": existingUser.Name,
	})
}

// HandleAdminRegenerateToken handles POST /api/admin/users/:id/token-regenerate
func HandleAdminRegenerateToken(c *fiber.Ctx) error {
	if adminUserService == nil {
		return c.Status(503).JSON(fiber.Map{"error": "service unavailable"})
	}

	userID, err := c.ParamsInt("id")
	if err != nil || userID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	// Get existing user
	existingUser, err := adminUserService.GetUserByID(int64(userID))
	if err != nil || existingUser == nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// Revoke old token(s) for this user first (by agent name)
	oldTokens, _ := auth.ListTokens()
	for _, t := range oldTokens {
		if t.UserID == int64(userID) && t.AgentName == existingUser.Name {
			log.Printf("[ADMIN.AUDIT] Revoking old token for user %s", existingUser.Name)
			if revErr := auth.RevokeToken(t.AgentName); revErr != nil {
				log.Printf("[ADMIN.ERROR] Failed to revoke old token for user %s: %v", existingUser.Name, revErr)
			}
		}
	}

	// Generate new token
	newToken, err := auth.RegisterTokenForUser(existingUser)
	if err != nil {
		log.Printf("[ADMIN.ERROR] Failed to regenerate token for user %s: %v", existingUser.Name, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to regenerate token"})
	}

	adminUser := c.Locals("user").(*models.User)
	log.Printf("[ADMIN.AUDIT] Regenerated token for user: %s by admin=%s", existingUser.Name, adminUser.Name)

	tokenResp := &AdminRegisterTokenResponse{
		ID:        newToken.ID,
		AgentName: newToken.AgentName,
		TokenName: newToken.TokenName,
		UserID:    newToken.UserID,
		Token:     newToken.Token, // Full token returned ONCE at creation
		CreatedAt: newToken.CreatedAt.Format(time.RFC3339),
	}

	return c.JSON(fiber.Map{
		"message": "Token regenerated successfully",
		"user_id": userID,
		"token":   tokenResp, // Includes full token (only shown once!)
	})
}

// RegisterAdminRoutes registers admin-only routes under /api/admin/
func RegisterAdminRoutes(app *fiber.App) {
	adminGroup := app.Group("/api/admin")

	// All admin routes require HUMAN_ADMIN role
	adminGroup.Use(AuthMiddlewareAdmin)

	// User management endpoints
	adminGroup.Post("/users", HandleAdminCreateUser)                           // Create user + token
	adminGroup.Get("/users", HandleAdminListUsers)                             // List all users (no tokens)
	adminGroup.Patch("/users/:id/role", HandleAdminUpdateUserRole)             // Update user role
	adminGroup.Delete("/users/:id", HandleAdminDeleteUser)                     // Delete user (+ cascade tokens)
	adminGroup.Post("/users/:id/token-regenerate", HandleAdminRegenerateToken) // Force new token

}
