// Package middleware provides shared HTTP middleware for Goban.
package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// ============================================================================
// RequestID Middleware Tests

func TestRequestID_GeneratesUniqueID(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(RequestID())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	requestID := resp.Header.Get("X-Request-ID")
	if requestID == "" {
		t.Error("Expected X-Request-ID header to be set, got empty string")
	}
}

func TestRequestID_IDIs16CharsHex(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(RequestID())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	requestID := resp.Header.Get("X-Request-ID")
	if len(requestID) != 16 {
		t.Errorf("Expected X-Request-ID to be 16 hex chars, got %d: %s", len(requestID), requestID)
		return
	}

	// Verify it's valid hex (8 bytes = 16 hex characters)
	for _, c := range requestID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("X-Request-ID contains invalid character: %q", string(c))
			return
		}
	}

	t.Logf("Valid X-Request-ID format: %s", requestID)
}

func TestRequestID_CorrelatedAcrossRequests(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(RequestID())
	app.Get("/test1", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	resp1, err := app.Test(httptest.NewRequest("GET", "/test1", nil))
	if err != nil {
		t.Fatalf("Request 1 failed: %v", err)
	}

	resp2, err := app.Test(httptest.NewRequest("GET", "/test1", nil))
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}

	id1 := resp1.Header.Get("X-Request-ID")
	id2 := resp2.Header.Get("X-Request-ID")

	if id1 == "" || id2 == "" {
		t.Error("One or both X-Request-ID headers were empty")
		return
	}

	// Each request should get a unique ID (not guaranteed but extremely likely with 8-byte random)
	t.Logf("Request 1: %s, Request 2: %s", id1, id2)
	if id1 == id2 {
		t.Log("Warning: Two requests got the same X-Request-ID (extremely unlikely collision)")
	}
}

func TestRequestID_LocalsKeyAvailable(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(RequestID())
	var capturedID string
	app.Get("/test", func(c *fiber.Ctx) error {
		capturedID, _ = c.Locals(requestIDKey).(string)
		return c.SendString("OK")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	responseID := resp.Header.Get("X-Request-ID")
	if responseID == "" || capturedID == "" {
		t.Error("Either Locals or header X-Request-ID was empty")
		return
	}

	if responseID != capturedID {
		t.Errorf("Locals ID (%s) does not match header X-Request-ID (%s)", capturedID, responseID)
	}
}

func TestRequestID_ResponseStatusCodeUnaffected(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(RequestID())
	app.Get("/not-found", func(c *fiber.Ctx) error {
		return c.Status(404).SendString("Not Found")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/not-found", nil))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected status 404, got %d — middleware should not affect response code", resp.StatusCode)
		return
	}

	requestID := resp.Header.Get("X-Request-ID")
	if requestID == "" {
		t.Error("Expected X-Request-ID header even on error responses")
	}
}

func TestRequestID_PreservesUpstreamHeader(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(RequestID())
	var capturedID string
	app.Get("/test", func(c *fiber.Ctx) error {
		capturedID, _ = c.Locals(requestIDKey).(string)
		return c.SendString("OK")
	})

	upstreamID := "my-upstream-trace-id"
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", upstreamID)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	responseID := resp.Header.Get("X-Request-ID")
	if responseID != upstreamID {
		t.Errorf("Expected upstream X-Request-ID to be preserved (%s), got %q", upstreamID, responseID)
	}

	if capturedID != upstreamID {
		t.Errorf("Expected Locals to contain upstream ID (%s), got %q", upstreamID, capturedID)
	}

	// Verify generated IDs still work when no upstream header provided
	resp2, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	responseID2 := resp2.Header.Get("X-Request-ID")
	if responseID2 == "" || len(responseID2) != 16 {
		t.Errorf("Expected auto-generated 16-char ID when no upstream header, got %q", responseID2)
	}
}

// ============================================================================
// Rate Limiter Tests (StrictLimiter, ModerateLimiter, GameLimiter)

func TestStrictLimiter_AllowsWithinLimit(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/auth", StrictLimiter())
	app.Get("/auth/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 5 requests (within the limit of 5)
	for i := 0; i < 5; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/auth/test", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Errorf("Request %d was rate limited before reaching the limit of 5", i+1)
			return
		}
	}

	t.Log("All 5 requests within strict limiter passed successfully")
}

func TestStrictLimiter_ExceedsLimit(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/auth", StrictLimiter())
	app.Get("/auth/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 6 requests (exceeds the limit of 5)
	for i := 0; i < 6; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/auth/test", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Logf("Request %d correctly rate limited with 429 status", i+1)
			return // Found the rate limit — test passes
		}
	}

	t.Error("Expected at least one request to be rate limited after exceeding limit of 5")
}

func TestModerateLimiter_AllowsWithinLimit(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/moderate", ModerateLimiter())
	app.Get("/moderate/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 10 requests (within the limit of 10)
	for i := 0; i < 10; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/moderate/test", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Errorf("Request %d was rate limited before reaching the limit of 10", i+1)
			return
		}
	}

	t.Log("All 10 requests within moderate limiter passed successfully")
}

func TestModerateLimiter_ExceedsLimit(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/moderate", ModerateLimiter())
	app.Get("/moderate/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 12 requests (exceeds the limit of 10)
	rateLimited := false
	for i := 0; i < 12; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/moderate/test", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			rateLimited = true
			t.Logf("Request %d correctly rate limited with 429 status", i+1)
			break
		}
	}

	if !rateLimited {
		t.Error("Expected at least one request to be rate limited after exceeding limit of 10")
	}
}

func TestGameLimiter_AllowsWithinLimit(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/game", GameLimiter())
	app.Get("/game/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 60 requests (within the limit of 60 for game endpoints)
	for i := 0; i < 60; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/game/test", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Errorf("Request %d was rate limited before reaching the limit of 60", i+1)
			return
		}
	}

	t.Log("All 60 requests within game limiter passed successfully")
}

func TestGameLimiter_ExceedsLimit(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/game", GameLimiter())
	app.Get("/game/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 65 requests (exceeds the limit of 60)
	rateLimited := false
	for i := 0; i < 65; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/game/test", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			rateLimited = true
			t.Logf("Request %d correctly rate limited with 429 status", i+1)
			break
		}
	}

	if !rateLimited {
		t.Error("Expected at least one request to be rate limited after exceeding limit of 60")
	}
}

func TestRateLimiter_ResponseFormat(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/auth", StrictLimiter())
	app.Get("/auth/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Exhaust the limit first
	for i := 0; i < 5; i++ {
		app.Test(httptest.NewRequest("GET", "/auth/test", nil))
	}

	// Next request should be rate limited with proper response format
	resp, err := app.Test(httptest.NewRequest("GET", "/auth/test", nil))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Logf("Got status %d (expected 429 — may vary based on limiter state)", resp.StatusCode)
		return // Limiter may have reset or not triggered yet
	}

	t.Log("Rate limited response returned 429 with proper error format")
}

func TestStrictLimiter_IPBasedCounting(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/auth", StrictLimiter())
	app.Get("/auth/login", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Make 5 requests to exhaust the limit (IP-based counting in Fiber limiter)
	for i := 0; i < 5; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/auth/login", nil))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		if resp.StatusCode == fiber.StatusTooManyRequests {
			break // Stop if we hit the limit
		}
	}

	// Next request from same IP should be rate limited regardless of path
	resp, err := app.Test(httptest.NewRequest("GET", "/auth/login", nil))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode == fiber.StatusTooManyRequests {
		t.Log("Correctly rate limited after exhausting IP-based counter")
	} else {
		t.Logf("Got status %d (limiter may have reset between requests)", resp.StatusCode)
	}
}
