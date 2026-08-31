package handlers

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"goban/auth"
)

func TestAuthMiddlewareWithRole_JWTMissingIdentityFields(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	secret := []byte("test-jwt-secret-for-identity")
	auth.SetJWTSecret(secret)
	t.Cleanup(func() { auth.SetJWTSecret(nil) })

	claims := auth.JWTClaims{
		UserID:   0,
		Username: "",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "legacy",
			Subject:   "42",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}

	resp, err := makeRequestWithAuth(app, tokenStr, "POST", "/api/tickets/", map[string]interface{}{
		"title":    "x",
		"board_id": "test-board",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Code != 401 {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
	body, _ := io.ReadAll(resp.Body)
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	want := "JWT missing required identity fields (user_id and username)"
	if payload["message"] != want {
		t.Fatalf("message=%q, want %q", payload["message"], want)
	}
	if payload["error"] != "unauthorized" {
		t.Fatalf("error=%q, want unauthorized", payload["error"])
	}
}
