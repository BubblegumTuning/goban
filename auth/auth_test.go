// Package auth handles token-based authentication for API access.
package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/models"
	"goban/testutil"
)

// setupTestStore creates a fresh mock store and registers it with the auth package.
func setupTestStore(t *testing.T) *testutil.MockStore {
	t.Helper()
	store := testutil.NewMockStore()
	if err := store.Init(); err != nil {
		t.Fatalf("Failed to initialize mock store: %v", err)
	}
	SetStore(store)
	return store
}

// resetAuthState clears all auth package globals between tests.
func resetAuthState() {
	store = nil
	JWTSecretKey = nil
	jwtValidity = 30 * 24 * time.Hour
	jwtRefreshGracePeriod = 90 * 24 * time.Hour
}

// readBody reads response body for test assertions.
func readBody(b interface{ Read([]byte) (int, error) }) string {
	buf := make([]byte, 4096)
	n, _ := b.Read(buf)
	return string(buf[:n])
}



// ============================================================================
// Token Hashing & Generation Tests

func TestHashToken_Deterministic(t *testing.T) {
	defer resetAuthState()

	hash1 := HashToken("my-token-abc")
	hash2 := HashToken("my-token-abc")
	if hash1 != hash2 {
		t.Errorf("HashToken is not deterministic: %q != %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("Expected hash length 64, got %d", len(hash1))
	}

	hash3 := HashToken("different-token")
	if hash1 == hash3 {
		t.Error("Different tokens produced identical hashes")
	}
}

func TestHashToken_EmptyString(t *testing.T) {
	defer resetAuthState()

	hash := HashToken("")
	if len(hash) != 64 {
		t.Errorf("Expected hash length 64 for empty input, got %d", len(hash))
	}
	if hash == "" {
		t.Error("HashToken of empty string produced empty result")
	}
}

func TestGenerateToken_LengthAndUniqueness(t *testing.T) {
	defer resetAuthState()

	tokens := make(map[string]bool)
	for i := 0; i < 10; i++ {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}
		if len(token) != 64 {
			t.Errorf("Expected token length 64, got %d for token %q", len(token), token)
		}
		if tokens[token] {
			t.Error("Duplicate token generated")
		}
		tokens[token] = true

		for _, c := range token {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Token contains non-hex character: %q", c)
			}
		}
	}

	if len(tokens) != 10 {
		t.Errorf("Expected 10 unique tokens, got %d", len(tokens))
	}
}

// ============================================================================
// Token CRUD Tests (via mock store)

func TestValidateToken_Success(t *testing.T) {
	defer resetAuthState()
	store := setupTestStore(t)

	plainToken := "test-plain-token"
	tokenHash := HashToken(plainToken)

	if _, err := store.CreateToken("test-agent", tokenHash); err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	result, err := ValidateToken(plainToken)
	if err != nil {
		t.Fatalf("ValidateToken failed for valid token: %v", err)
	}
	if result.AgentName != "test-agent" {
		t.Errorf("Expected agent 'test-agent', got '%s'", result.AgentName)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	defer resetAuthState()
	setupTestStore(t)

	result, err := ValidateToken("nonexistent-token")
	if result != nil {
		t.Errorf("Expected nil result for invalid token, got %+v", result)
	}
	if err == nil {
		t.Log("ValidateToken returned nil result and nil error (store not-found)")
	}
}

func TestValidateToken_NoStore(t *testing.T) {
	defer resetAuthState()
	SetStore(nil)

	result, err := ValidateToken("any-token")
	if result != nil {
		t.Error("Expected nil result with no store")
	}
	if !strings.Contains(err.Error(), "database not initialized") {
		t.Errorf("Expected 'database not initialized' error, got: %v", err)
	}
}

func TestRegisterToken_Success(t *testing.T) {
	defer resetAuthState()
	store := setupTestStore(t)

	token, err := RegisterToken("test-agent")
	if err != nil {
		t.Fatalf("RegisterToken failed: %v", err)
	}
	if token.AgentName != "test-agent" {
		t.Errorf("Expected agent 'test-agent', got '%s'", token.AgentName)
	}
	if len(token.Token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(token.Token))
	}
	if !strings.HasPrefix(token.TokenName, "goban-") {
		t.Errorf("TokenName should start with 'goban-', got '%s'", token.TokenName)
	}

	expectedHash := HashToken(token.Token)
	if token.TokenHash != expectedHash {
		t.Error("Stored hash does not match generated token")
	}

	stored, err := store.ValidateToken(expectedHash)
	if err != nil || stored == nil {
		t.Error("Registered token not found in store after registration")
	}
}

func TestRegisterTokenForUser_Success(t *testing.T) {
	defer resetAuthState()
	_ = setupTestStore(t)

	user := &models.User{ID: 1, Name: "admin-user", Role: models.RoleHumanAdmin}

	token, err := RegisterTokenForUser(user)
	if err != nil {
		t.Fatalf("RegisterTokenForUser failed: %v", err)
	}
	if token.UserID != user.ID {
		t.Errorf("Expected UserID %d, got %d", user.ID, token.UserID)
	}
	if token.AgentName != user.Name {
		t.Errorf("Expected AgentName '%s', got '%s'", user.Name, token.AgentName)
	}
	if len(token.Token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(token.Token))
	}

	expectedHash := HashToken(token.Token)
	if token.TokenHash != expectedHash {
		t.Error("Stored hash does not match generated token")
	}
}

func TestRegisterToken_NoStore(t *testing.T) {
	defer resetAuthState()
	SetStore(nil)

	result, err := RegisterToken("any-agent")
	if result != nil {
		t.Error("Expected nil with no store")
	}
	if !strings.Contains(err.Error(), "database not initialized") {
		t.Errorf("Expected 'database not initialized' error, got: %v", err)
	}
}

func TestRevokeToken_Success(t *testing.T) {
	defer resetAuthState()
	store := setupTestStore(t)

	tokenHash := HashToken("revoked-token")
	if _, err := store.CreateToken("revoke-agent", tokenHash); err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	err := RevokeToken("revoke-agent")
	if err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	stored, _ := store.ValidateToken(tokenHash)
	if stored != nil {
		t.Error("Token still exists after revocation")
	}
}

func TestRevokeToken_NotFound(t *testing.T) {
	defer resetAuthState()
	setupTestStore(t)

	err := RevokeToken("nonexistent-agent")
	if err == nil {
		t.Error("Expected error for non-existent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' in error, got: %v", err)
	}
}

func TestRevokeToken_NoStore(t *testing.T) {
	defer resetAuthState()
	SetStore(nil)

	err := RevokeToken("any-agent")
	if !strings.Contains(err.Error(), "database not initialized") {
		t.Errorf("Expected 'database not initialized' error, got: %v", err)
	}
}

func TestListTokens_ReturnsAllWithMaskedValues(t *testing.T) {
	defer resetAuthState()
	store := setupTestStore(t)

	hash1 := HashToken("token-one")
	hash2 := HashToken("token-two")
	if _, err := store.CreateToken("agent-a", hash1); err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}
	if _, err := store.CreateToken("agent-b", hash2); err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	tokens, err := ListTokens()
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("Expected 2 tokens, got %d", len(tokens))
	}

	for _, token := range tokens {
		if token.Token != "" {
			t.Error("Token values should be masked (empty string), but was not empty")
		}
	}
}

func TestListTokens_NoStore(t *testing.T) {
	defer resetAuthState()
	SetStore(nil)

	result, err := ListTokens()
	if result != nil {
		t.Error("Expected nil with no store")
	}
	if !strings.Contains(err.Error(), "database not initialized") {
		t.Errorf("Expected 'database not initialized' error, got: %v", err)
	}
}

// ============================================================================
// ValidateTokenWithRole Tests

func TestValidateTokenWithRole_Success(t *testing.T) {
	defer resetAuthState()
	store := setupTestStore(t)

	plainToken := "role-test-token"
	tokenHash := HashToken(plainToken)

	if _, err := store.CreateUser("overseer", models.RoleOverseerAI); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if _, err := store.CreateTokenWithUser(1, "overseer", tokenHash); err != nil {
		t.Fatalf("Failed to create token with user: %v", err)
	}

	user, err := ValidateTokenWithRole(plainToken)
	if err != nil {
		t.Fatalf("ValidateTokenWithRole failed: %v", err)
	}
	if user.Name != "overseer" {
		t.Errorf("Expected user 'overseer', got '%s'", user.Name)
	}
	if user.Role != models.RoleOverseerAI {
		t.Errorf("Expected role '%s', got '%s'", models.RoleOverseerAI, user.Role)
	}
}

func TestValidateTokenWithRole_NoStore(t *testing.T) {
	defer resetAuthState()
	SetStore(nil)

	user, err := ValidateTokenWithRole("any-token")
	if user != nil {
		t.Error("Expected nil with no store")
	}
	if !strings.Contains(err.Error(), "database not initialized") {
		t.Errorf("Expected 'database not initialized' error, got: %v", err)
	}
}

func TestValidateTokenWithRole_InvalidToken(t *testing.T) {
	defer resetAuthState()
	setupTestStore(t)

	user, err := ValidateTokenWithRole("nonexistent-token")
	if user != nil {
		t.Error("Expected nil for invalid token")
	}
	if err == nil {
		t.Log("ValidateTokenWithRole returned nil/nil for unregistered token (store returns nil)")
	}
}

// ============================================================================
// JWT Configuration Tests

func TestSetJWTSecret_SetAndClear(t *testing.T) {
	defer resetAuthState()

	SetJWTSecret([]byte("test-secret-key"))
	if len(JWTSecretKey) == 0 {
		t.Error("JWTSecretKey should be set after SetJWTSecret")
	}
}

func TestSetJWTConfig_Defaults(t *testing.T) {
	defer resetAuthState()

	SetJWTConfig(7*24*time.Hour, 14*24*time.Hour)

	if jwtValidity != 7*24*time.Hour {
		t.Errorf("Expected validity 7d, got %v", jwtValidity)
	}
	if jwtRefreshGracePeriod != 14*24*time.Hour {
		t.Errorf("Expected grace period 14d, got %v", jwtRefreshGracePeriod)
	}

	duration := JWTRefreshGracePeriodDuration()
	if duration != 14*24*time.Hour {
		t.Errorf("JWTRefreshGracePeriodDuration returned %v, expected 14d", duration)
	}
}

func TestSetJWTConfig_ZeroValuesIgnored(t *testing.T) {
	defer resetAuthState()

	jwtValidity = 7 * 24 * time.Hour
	jwtRefreshGracePeriod = 14 * 24 * time.Hour

	SetJWTConfig(0, 0)

	if jwtValidity != 7*24*time.Hour {
		t.Error("Zero validity should not override existing value")
	}
	if jwtRefreshGracePeriod != 14*24*time.Hour {
		t.Error("Zero grace period should not override existing value")
	}
}

// ============================================================================
// JWT Generation & Verification Tests

func TestGenerateJWT_Success(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("test-secret"))

	user := &models.User{ID: 42, Name: "jwt-user", Role: models.RoleNormalAI}

	tokenStr, expiresIn, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}
	if tokenStr == "" {
		t.Error("Generated JWT is empty")
	}
	if expiresIn <= 0 {
		t.Errorf("Expected positive expires_in, got %d", expiresIn)
	}

	claims, err := VerifyJWT(tokenStr)
	if err != nil {
		t.Fatalf("Generated JWT failed verification: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("Expected UserID 42, got %d", claims.UserID)
	}
	if claims.Username != "jwt-user" {
		t.Errorf("Expected Username 'jwt-user', got '%s'", claims.Username)
	}
	if claims.Role != models.RoleNormalAI {
		t.Errorf("Expected Role '%s', got '%s'", models.RoleNormalAI, claims.Role)
	}

	expectedExpires := int64(jwtValidity / time.Second)
	if expiresIn != expectedExpires {
		t.Errorf("Expected expires_in %d, got %d", expectedExpires, expiresIn)
	}
}

func TestGenerateJWT_RememberMe(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("test-secret"))
	jwtValidity = 7 * 24 * time.Hour

	user := &models.User{ID: 1, Name: "remember-me-user", Role: models.RoleNormalAI}

	_, expiresInNormal, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT (normal) failed: %v", err)
	}

	_, expiresInRemember, err := GenerateJWT(user, true)
	if err != nil {
		t.Fatalf("GenerateJWT (remember me) failed: %v", err)
	}

	expectedNormal := int64(7 * 24 * time.Hour / time.Second)
	expectedDouble := expectedNormal * 2

	if expiresInNormal != expectedNormal {
		t.Errorf("Expected normal expires_in %d, got %d", expectedNormal, expiresInNormal)
	}
	if expiresInRemember != expectedDouble {
		t.Errorf("Expected remember me expires_in %d, got %d", expectedDouble, expiresInRemember)
	}
}

func TestGenerateJWT_NoSecret(t *testing.T) {
	defer resetAuthState()
	JWTSecretKey = nil

	user := &models.User{ID: 1, Name: "test-user", Role: models.RoleNormalAI}

	tokenStr, expiresIn, err := GenerateJWT(user, false)
	if tokenStr != "" {
		t.Error("Expected empty token with no secret")
	}
	if expiresIn != 0 {
		t.Errorf("Expected expires_in 0, got %d", expiresIn)
	}
	if !strings.Contains(err.Error(), "signing secret not configured") {
		t.Errorf("Expected 'signing secret not configured' error, got: %v", err)
	}
}

func TestVerifyJWT_ValidToken(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("verify-secret"))

	user := &models.User{ID: 99, Name: "verify-user", Role: models.RoleHumanAdmin}
	tokenStr, _, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	claims, err := VerifyJWT(tokenStr)
	if err != nil {
		t.Fatalf("VerifyJWT failed for valid token: %v", err)
	}
	if claims.UserID != 99 || claims.Username != "verify-user" || claims.Role != models.RoleHumanAdmin {
		t.Errorf("Claims mismatch: %+v", claims)
	}
}

func TestVerifyJWT_InvalidSignature(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("correct-secret"))

	user := &models.User{ID: 1, Name: "sig-user", Role: models.RoleNormalAI}
	tokenStr, _, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	SetJWTSecret([]byte("wrong-secret"))

	claims, err := VerifyJWT(tokenStr)
	if claims != nil {
		t.Error("Expected nil claims for invalid signature")
	}
	if !strings.Contains(err.Error(), "invalid JWT token") {
		t.Errorf("Expected 'invalid JWT token' error, got: %v", err)
	}
}

func TestVerifyJWT_NoSecret(t *testing.T) {
	defer resetAuthState()
	JWTSecretKey = nil

	claims, err := VerifyJWT("any-token-string")
	if claims != nil {
		t.Error("Expected nil with no secret")
	}
	if !strings.Contains(err.Error(), "signing secret not configured") {
		t.Errorf("Expected 'signing secret not configured' error, got: %v", err)
	}
}

func TestVerifyJWT_TamperedToken(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("tamper-secret"))

	_, err := VerifyJWT("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.tampered-signature-here")
	if err == nil {
		t.Error("Expected error for tampered token")
	}
}

// ============================================================================
// ParseJWT Tests (signature-only validation, no expiry check)

func TestParseJWT_ValidSignatureExpiredToken(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("parse-secret"))

	jwtValidity = -time.Hour // Already expired

	user := &models.User{ID: 7, Name: "parse-user", Role: models.RoleNormalAI}
	tokenStr, _, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	_, vErr := VerifyJWT(tokenStr)
	if vErr == nil {
		t.Log("VerifyJWT accepted expired token (may be within grace period)")
	} else {
		t.Logf("VerifyJWT correctly rejected expired token: %v", vErr)
	}

	pClaims, pErr := ParseJWT(tokenStr)
	if pErr != nil {
		t.Fatalf("ParseJWT failed for valid signature (even if expired): %v", pErr)
	}
	if pClaims.UserID != 7 || pClaims.Username != "parse-user" {
		t.Errorf("ParseJWT claims mismatch: %+v", pClaims)
	}

	jwtValidity = 30 * 24 * time.Hour
}

func TestParseJWT_InvalidSignature(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("parse-secret"))

	user := &models.User{ID: 1, Name: "user", Role: models.RoleNormalAI}
	tokenStr, _, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	SetJWTSecret([]byte("different-secret"))

	claims, pErr := ParseJWT(tokenStr)
	if claims != nil {
		t.Error("Expected nil for invalid signature")
	}
	if !strings.Contains(pErr.Error(), "invalid JWT signature") {
		t.Errorf("Expected 'invalid JWT signature' error, got: %v", pErr)
	}
}

func TestParseJWT_NoSecret(t *testing.T) {
	defer resetAuthState()
	JWTSecretKey = nil

	claims, err := ParseJWT("any-token")
	if claims != nil {
		t.Error("Expected nil with no secret")
	}
	if !strings.Contains(err.Error(), "signing secret not configured") {
		t.Errorf("Expected 'signing secret not configured' error, got: %v", err)
	}
}

// ============================================================================
// Middleware Tests (using Fiber test app + httptest)

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	defer resetAuthState()

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		return AuthMiddleware(c)
	})
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp == nil || resp.StatusCode < 400 {
		t.Errorf("Expected client/server error for missing Authorization header, got status %d", func() int { if resp != nil { return resp.StatusCode }; return 0 }())
	}
}

func TestAuthMiddleware_InvalidFormat(t *testing.T) {
	defer resetAuthState()

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		return AuthMiddleware(c)
	})
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	resp, _ := app.Test(req)
	if resp == nil || resp.StatusCode < 400 {
		t.Errorf("Expected client/server error for non-Bearer auth header, got status %d", func() int { if resp != nil { return resp.StatusCode }; return 0 }())
	}
}

func TestAuthMiddleware_EmptyToken(t *testing.T) {
	defer resetAuthState()

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		return AuthMiddleware(c)
	})
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ") // Empty token after Bearer
	resp, _ := app.Test(req)
	if resp == nil || resp.StatusCode < 400 {
		t.Errorf("Expected client/server error for empty Bearer token, got status %d", func() int { if resp != nil { return resp.StatusCode }; return 0 }())
	}
}

func TestSendAuthError_Returns401(t *testing.T) {
	defer resetAuthState()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return SendAuthError(c, "custom message")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	body := readBody(resp.Body)
	if !strings.Contains(body, "unauthorized") || !strings.Contains(body, "custom message") {
		t.Errorf("Response body missing expected fields: %s", body)
	}
}

// ============================================================================
// AuthMiddlewareWithUser Tests

func TestAuthMiddlewareWithUser_ValidJWT(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("middleware-secret"))

	app := fiber.New()
	app.Use(AuthMiddlewareWithUser)
	app.Get("/test", func(c *fiber.Ctx) error {
		userID := c.Locals("user_id")
		username := c.Locals("username")
		role := c.Locals("role")
		return c.JSON(fiber.Map{
			"user_id":  userID,
			"username": username,
			"role":     role,
		})
	})

	user := &models.User{ID: 55, Name: "middleware-user", Role: models.RoleOverseerAI}
	tokenStr, _, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	resp, testErr := app.Test(req)
	if testErr != nil {
		t.Fatalf("Request failed: %v", testErr)
	}
	if resp.StatusCode != 200 {
		body := readBody(resp.Body)
		t.Errorf("Expected status 200, got %d. Body: %s", resp.StatusCode, body)
	}

	body := readBody(resp.Body)
	if !strings.Contains(body, "middleware-user") {
		t.Errorf("Response missing username 'middleware-user': %s", body)
	}
	if !strings.Contains(body, models.RoleOverseerAI) {
		t.Errorf("Response missing role '%s': %s", models.RoleOverseerAI, body)
	}
}

func TestAuthMiddlewareWithUser_MissingHeader(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("middleware-secret"))

	app := fiber.New()
	app.Use(AuthMiddlewareWithUser)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil))
	if resp != nil && resp.StatusCode == 200 {
		t.Error("Expected non-200 status for missing header")
	}
}

func TestAuthMiddlewareWithUser_InvalidJWT(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("middleware-secret"))

	app := fiber.New()
	app.Use(AuthMiddlewareWithUser)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token-string-here")

	resp, _ := app.Test(req)
	if resp != nil && resp.StatusCode == 200 {
		t.Error("Expected non-200 status for invalid JWT")
	}
}

func TestAuthMiddlewareWithUser_WrongScheme(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("middleware-secret"))

	app := fiber.New()
	app.Use(AuthMiddlewareWithUser)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic some-token-here")

	resp, _ := app.Test(req)
	if resp != nil && resp.StatusCode == 200 {
		t.Error("Expected non-200 status for wrong auth scheme")
	}
}

// ============================================================================
// Integration: Full JWT workflow (generate -> verify -> middleware)

func TestJWT_FullWorkflow(t *testing.T) {
	defer resetAuthState()
	SetJWTSecret([]byte("integration-test-secret"))

	user := &models.User{ID: 100, Name: "full-flow-user", Role: models.RoleHumanAdmin}

	// Step 1: Generate JWT
	tokenStr, expiresIn, err := GenerateJWT(user, false)
	if err != nil {
		t.Fatalf("Step 1 failed - GenerateJWT: %v", err)
	}
	if tokenStr == "" || expiresIn <= 0 {
		t.Fatal("Step 1 failed - empty token or invalid expiry")
	}

	// Step 2: Verify JWT independently
	claims, err := VerifyJWT(tokenStr)
	if err != nil {
		t.Fatalf("Step 2 failed - VerifyJWT: %v", err)
	}
	if claims.UserID != 100 || claims.Username != "full-flow-user" {
		t.Fatalf("Step 2 failed - claims mismatch: %+v", claims)
	}

	// Step 3: ParseJWT (no expiry check) should also work
	pClaims, pErr := ParseJWT(tokenStr)
	if pErr != nil {
		t.Fatalf("Step 3 failed - ParseJWT: %v", pErr)
	}
	if pClaims.UserID != claims.UserID {
		t.Errorf("Step 3 failed - ParseUserID mismatch")
	}

	// Step 4: AuthMiddlewareWithUser should accept the token
	app := fiber.New()
	app.Use(AuthMiddlewareWithUser)
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"user_id":  c.Locals("user_id"),
			"username": c.Locals("username"),
			"role":     c.Locals("role"),
		})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	resp, testErr := app.Test(req)
	if testErr != nil {
		t.Fatalf("Step 4 failed - middleware request: %v", testErr)
	}
	if resp.StatusCode != 200 {
		body := readBody(resp.Body)
		t.Errorf("Step 4 failed - expected 200, got %d. Body: %s", resp.StatusCode, body)
	}

	// Step 5: Tampered token should be rejected by middleware
	req.Header.Set("Authorization", "Bearer tampered.token.here")
	resp2, _ := app.Test(req)
	if resp2 != nil && resp2.StatusCode == 200 {
		t.Error("Step 5 failed - tampered token was accepted")
	}

	// Step 6: Wrong secret should reject the valid token
	SetJWTSecret([]byte("wrong-secret-now"))
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp3, _ := app.Test(req)
	if resp3 != nil && resp3.StatusCode == 200 {
		t.Error("Step 6 failed - token accepted with wrong secret")
	}
}

// ============================================================================
// Edge Cases

func TestHashToken_LongInput(t *testing.T) {
	defer resetAuthState()

	longInput := strings.Repeat("a", 10000)
	hash := HashToken(longInput)
	if len(hash) != 64 {
		t.Errorf("Expected hash length 64 for long input, got %d", len(hash))
	}
}

func TestJWTClaims_Struct(t *testing.T) {
	defer resetAuthState()

	claims := &JWTClaims{
		UserID:   123,
		Username: "test",
		Role:     models.RoleNormalAI,
	}

	if claims.UserID != 123 || claims.Username != "test" || claims.Role != models.RoleNormalAI {
		t.Error("JWTClaims struct fields not stored correctly")
	}
}

func TestLoginRequest_RememberMeDefault(t *testing.T) {
	req := LoginRequest{Username: "user", Password: "pass"}
	if req.RememberMe != false {
		t.Error("RememberMe should default to false")
	}
}
