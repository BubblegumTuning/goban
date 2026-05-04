// SSE (Server-Sent Events) handler for real-time updates - uses sse package.
package handlers

import (
	"github.com/gofiber/fiber/v2"
	"goban/sse"
)

func handleSSE(c *fiber.Ctx) error {
	return sse.HandleSSE(c)
}

// RegisterSSERoutes registers the SSE endpoint.
func RegisterSSERoutes(app *fiber.App) {
	app.Get("/events", handleSSE)
}
