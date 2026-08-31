package handlers

import (
	"testing"

	"goban/auth"
	"goban/models"
)

func TestAuthMiddlewareAdmin_AcceptsAdminJWT(t *testing.T) {
	app, s, cleanup := setupTestAppWithAdmin(t)
	defer cleanup()

	auth.SetJWTSecret([]byte("test-jwt-secret-for-admin"))
	t.Cleanup(func() { auth.SetJWTSecret(nil) })

	jwtToken, _ := createAdminJWT(t, s, "test-admin-jwt-mw", models.RoleHumanAdmin)
	resp, err := makeRequestWithAuth(app, jwtToken, "GET", "/api/admin/users", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Code != 200 {
		t.Fatalf("expected 200 for admin JWT on /api/admin/users, got %d", resp.Code)
	}
}

func TestAuthMiddlewareWithUser_AcceptsAPIToken(t *testing.T) {
	app, s, cleanup := setupTestAppWithArchive(t)
	defer cleanup()

	tokenStr, _ := createAdminToken(t, s, "test-archive-api-token")
	resp, err := makeRequestWithAuth(app, tokenStr, "GET", "/api/archived", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Code != 200 {
		t.Fatalf("expected 200 for API token on /api/archived, got %d", resp.Code)
	}
}
