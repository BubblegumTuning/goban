package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCompactLevel(t *testing.T) {
	app := fiber.New()
	var compact, truncate bool
	app.Get("/x", func(c *fiber.Ctx) error {
		compact, truncate = compactLevel(c)
		return c.SendStatus(200)
	})

	cases := []struct {
		url    string
		header string
		wantC  bool
		wantTD bool
	}{
		{"/x", "", false, false},
		{"/x?compact=true", "", true, false},
		{"/x?c=1&td=1", "", true, true},
		{"/x?truncate_desc=true", "", false, true},
	}
	for _, tc := range cases {
		compact, truncate = false, false
		req := httptest.NewRequest("GET", tc.url, nil)
		if tc.header != "" {
			req.Header.Set("X-Compact", tc.header)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", tc.url, err)
		}
		resp.Body.Close()
		if compact != tc.wantC || truncate != tc.wantTD {
			t.Errorf("%s: compact=%v truncate=%v, want %v %v", tc.url, compact, truncate, tc.wantC, tc.wantTD)
		}
	}

	compact, truncate = false, false
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Compact", "true")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !compact {
		t.Error("X-Compact: true should enable compact")
	}
}
