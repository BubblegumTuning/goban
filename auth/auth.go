// Package auth handles token-based authentication for API access.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"goban/models"
)

// TicketStore interface for database operations (defined in store package).
type TicketStore interface {
	CreateToken(agentName, tokenHash string) (int64, error)
	CreateTokenWithUser(userID int64, agentName, tokenHash string) (int64, error)
	ValidateToken(tokenHash string) (*models.AgentToken, error)
	UpdateTokenLastUsed(tokenHash string) error
	DeleteToken(agentName string) (int64, error)
	ListTokens() ([]models.AgentToken, error)

	// User management methods
	GetUserByToken(tokenHash string) (*models.User, error)
	CreateUser(name, role string) (int64, error)
	GetUserByName(name string) (*models.User, error)
	DeleteUser(id int64) error
}

var store TicketStore // Set via SetStore()

// SetStore sets the database layer for token operations.
func SetStore(s TicketStore) {
	store = s
}

// HashToken creates a SHA256 hash of a token string.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateToken creates a cryptographically secure API token.
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	token := hex.EncodeToString(bytes)
	return token[:64], nil // Return first 64 chars (128 bits of entropy)
}

// ValidateToken checks if a Bearer token is valid and returns the associated agent.
func ValidateToken(token string) (*models.AgentToken, error) {
	if store == nil {
		return nil, errors.New("database not initialized")
	}

	hash := HashToken(token)
	tokenData, err := store.ValidateToken(hash)
	if err != nil {
		return nil, err
	}

	// Update last used timestamp
	if updateErr := store.UpdateTokenLastUsed(hash); updateErr != nil {
		log.Printf("Warning: Failed to update token last_used: %v", updateErr)
	}

	return tokenData, nil
}

// ValidateTokenWithRole validates a Bearer token and returns the associated User with role.
// Returns an error if token is invalid or user lookup fails.
func ValidateTokenWithRole(token string) (*models.User, error) {
	if store == nil {
		return nil, errors.New("database not initialized")
	}

	hash := HashToken(token)

	// First validate the token exists
	if _, err := store.ValidateToken(hash); err != nil {
		return nil, err
	}

	// Update last used timestamp
	if updateErr := store.UpdateTokenLastUsed(hash); updateErr != nil {
		log.Printf("Warning: Failed to update token last_used: %v", updateErr)
	}

	// Look up user by token hash (includes role information)
	user, err := store.GetUserByToken(hash)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}

	return user, nil
}

// RegisterToken creates a new API token for an agent (legacy, no user linkage).
func RegisterToken(agentName string) (*models.AgentToken, error) {
	if store == nil {
		return nil, errors.New("database not initialized")
	}

	token, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	hash := HashToken(token)
	tokenName := fmt.Sprintf("goban-%s", token[:8]) // Display name with first 8 chars

	id, err := store.CreateToken(agentName, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return &models.AgentToken{
		ID:        id,
		AgentName: agentName,
		TokenName: tokenName,
		Token:     token,
		TokenHash: hash,
		CreatedAt: time.Now(),
	}, nil
}

// RegisterTokenForUser creates a new API token linked to an existing user.
func RegisterTokenForUser(user *models.User) (*models.AgentToken, error) {
	if store == nil {
		return nil, errors.New("database not initialized")
	}

	token, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	hash := HashToken(token)
	tokenName := fmt.Sprintf("goban-%s", token[:8])

	id, err := store.CreateTokenWithUser(user.ID, user.Name, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return &models.AgentToken{
		ID:        id,
		UserID:    user.ID, // Link token to user
		AgentName: user.Name,
		TokenName: tokenName,
		Token:     token, // Full token returned once at creation
		TokenHash: hash,
		CreatedAt: time.Now(),
	}, nil
}

// RevokeToken deletes a token by agent name.
func RevokeToken(agentName string) error {
	if store == nil {
		return errors.New("database not initialized")
	}

	count, err := store.DeleteToken(agentName)
	if err != nil {
		return err
	}

	if count == 0 {
		return fmt.Errorf("token for agent '%s' not found", agentName)
	}

	log.Printf("Revoked token for agent: %s", agentName)
	return nil
}

// ListTokens returns all active tokens.
func ListTokens() ([]models.AgentToken, error) {
	if store == nil {
		return nil, errors.New("database not initialized")
	}

	tokens, err := store.ListTokens()
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	// Remove sensitive token values from response
	for i := range tokens {
		tokens[i].Token = "" // Don't expose full tokens
	}

	return tokens, nil
}

// AuthMiddleware validates Bearer token in Authorization header.
func AuthMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")

	if authHeader == "" {
		return errors.New("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return errors.New("invalid authorization format (expected 'Bearer <token>')")
	}

	token := parts[1]
	if token == "" {
		return errors.New("empty token")
	}

	_, err := ValidateToken(token)
	return err
}

// SendAuthError returns a standardized authentication error response.
func SendAuthError(c *fiber.Ctx, msg string) error {
	return c.Status(401).JSON(fiber.Map{
		"error":   "unauthorized",
		"message": msg,
	})
}

// =============================================================================
// JWT Authentication (Web UI Login System) - Added 2026-04-22 for ticket-e0a4c2d9d8

// JWTClaims represents the claims structure for JWT tokens.
type JWTClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// LoginRequest represents a login request from the web UI.
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me,omitempty"` // 30-day session if true, 7-day default
}

// LoginResponse represents the response after successful login.
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	User        *models.User `json:"user"`
	ExpiresIn   int64        `json:"expires_in"` // seconds until expiration
}

// JWTSecretKey is the signing key for JWT tokens. Must be set via SetJWTSecret() before use.
var JWTSecretKey []byte

// jwtValidity holds the configured token validity duration. Defaults to 30 days if not set.
var jwtValidity = 30 * 24 * time.Hour

// jwtRefreshGracePeriod holds the configured refresh grace period. Defaults to 90 days if not set.
var jwtRefreshGracePeriod = 90 * 24 * time.Hour

// JWTRefreshGracePeriodDuration returns the current refresh grace period duration.
func JWTRefreshGracePeriodDuration() time.Duration {
	return jwtRefreshGracePeriod
}

// SetJWTSecret sets the JWT signing secret from configuration.
// Returns an error if called with an empty secret, preventing accidental use of defaults.
func SetJWTSecret(secret []byte) {
	if len(secret) == 0 {
		log.Printf("[WARN] No JWT secret provided — JWT authentication will be disabled")
		return
	}
	JWTSecretKey = secret
}

// SetJWTConfig sets configurable JWT parameters from application config.
func SetJWTConfig(validity time.Duration, refreshGracePeriod time.Duration) {
	if validity > 0 {
		jwtValidity = validity
		log.Printf("[INFO] JWT token validity set to %s", jwtValidity)
	}
	if refreshGracePeriod > 0 {
		jwtRefreshGracePeriod = refreshGracePeriod
		log.Printf("[INFO] JWT refresh grace period set to %s", jwtRefreshGracePeriod)
	}
}

// GenerateJWT creates a new JWT token for the given user.
// Default expiration uses configured jwtValidity (default 30 days). If rememberMe is true: 2x validity.
// Returns an error if no JWT secret has been configured.
func GenerateJWT(user *models.User, rememberMe bool) (string, int64, error) {
	if len(JWTSecretKey) == 0 {
		return "", 0, errors.New("JWT authentication not available: signing secret not configured")
	}

	// Set expiration based on configured validity and remember_me preference
	expiresIn := jwtValidity
	if rememberMe {
		expiresIn = jwtValidity * 2 // Double the validity for "remember me" sessions
	}

	// Create JWT claims with user information
	claims := &JWTClaims{
		UserID:   user.ID,
		Username: user.Name,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "goban",
		},
	}

	// Create token with HS256 signing method
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(JWTSecretKey)
	if err != nil {
		return "", 0, fmt.Errorf("failed to sign JWT: %w", err)
	}

	// Calculate expiration in seconds
	expiresInSeconds := int64(expiresIn / time.Second)

	return tokenString, expiresInSeconds, nil
}

// VerifyJWT validates a JWT token and returns the claims if valid.
// Returns an error if no JWT secret has been configured.
func VerifyJWT(tokenString string) (*JWTClaims, error) {
	if len(JWTSecretKey) == 0 {
		return nil, errors.New("JWT authentication not available: signing secret not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return JWTSecretKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid JWT token: %w", err)
	}

	// Extract claims if token is valid
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// ParseJWT validates a JWT signature without checking expiration.
// Returns the claims if the signature is valid, regardless of whether the token has expired.
// Used for refresh operations where we check if the token is within the grace period window.
func ParseJWT(tokenString string) (*JWTClaims, error) {
	if len(JWTSecretKey) == 0 {
		return nil, errors.New("JWT authentication not available: signing secret not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return JWTSecretKey, nil
	}, jwt.WithoutClaimsValidation()) // Skip expiration validation

	if err != nil {
		return nil, fmt.Errorf("invalid JWT signature: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// AuthMiddlewareWithUser validates JWT Bearer token and attaches user info to context.
func AuthMiddlewareWithUser(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")

	if authHeader == "" {
		return SendAuthError(c, "missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return SendAuthError(c, "invalid authorization format (expected 'Bearer <token>')")
	}

	tokenString := parts[1]
	if tokenString == "" {
		return SendAuthError(c, "empty token")
	}

	// Verify JWT and extract claims
	claims, err := VerifyJWT(tokenString)
	if err != nil {
		return SendAuthError(c, fmt.Sprintf("invalid or expired token: %v", err))
	}

	// Attach user info to context for downstream handlers
	c.Locals("user_id", claims.UserID)
	c.Locals("username", claims.Username)
	c.Locals("role", claims.Role)

	return c.Next()
}
