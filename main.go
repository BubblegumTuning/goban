// Goban - A Kanban API for seamless human-AI collaboration
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"goban/auth"
	"goban/config"
	"goban/handlers"
	"goban/middleware"
	"goban/store"
	"goban/version"
)

var publicPath string

// getConfigPath returns the path to goban.toml from env, config dir, or default
func getConfigPath() string {
	if path := os.Getenv("GOBAN_CONFIG_PATH"); path != "" {
		return path
	}
	// Try standard locations in order
	paths := []string{
		"/opt/goban/config/goban.toml",
		"/etc/goban/goban.toml",
		"~/goban/goban.toml",
		"./goban.toml",
	}
	for _, p := range paths {
		if expanded, err := filepath.Abs(p); err == nil {
			if home, err := os.UserHomeDir(); err == nil {
				expanded = filepath.Clean(filepath.Join(expanded))
				if strings.HasPrefix(expanded, "~/") {
					expanded = strings.Replace(expanded, "~", home, 1)
				}
			}
			if _, err := os.Stat(expanded); err == nil {
				return expanded
			}
		}
	}
	return "./goban.toml" // fallback
}

func main() {
	// Handle --version / -v before any config/DB initialization (fast cold start)
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("goban %s\n", version.Version)
			os.Exit(0)
		}
	}

	// Load config
	cfg := config.LoadConfig(getConfigPath())
	config.Debug = cfg.Debug // Gate debug logging globally

	// Get absolute path to static directory from config or env
	publicPath = os.Getenv("GOBAN_STATIC_PATH")
	if publicPath == "" {
		publicPath = cfg.StaticPath
	}
	if publicPath == "" {
		// Fallback: next to binary (dev mode)
		ex, _ := os.Executable()
		publicPath = filepath.Join(filepath.Dir(ex), "public")
	}

	// Create logger that respects log level configuration
	logFilter := config.NewLogFilter(cfg.LogLevel)

	log.Printf("Serving static files from: %s", publicPath)

	// Create Fiber app with middleware
	app := fiber.New(fiber.Config{AppName: "Goban/" + version.Version})

	// Panic recovery — MUST be first middleware to catch panics from all handlers.
	// Returns 500 responses instead of crashing the server process.
	app.Use(recover.New())

	// Debug middleware - only enabled when log_level is "debug"
	if logFilter.ShouldLog("debug") {
		app.Use(DebugLogger(logFilter))
	}

	// CORS: configurable via cors_origins in goban.toml or GOBAN_CORS_ORIGINS env var.
	// Defaults to same-origin only (no wildcard) for security — set explicitly if cross-origin needed.
	corsOrigins := cfg.CorsOrigins
	if corsOrigins == "" {
		// Default: restrict to the server's own origin
		corsOrigins = fmt.Sprintf("http://localhost:%s", cfg.Port)
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins,
		AllowMethods: "GET,POST,PUT,DELETE,PATCH",
	}))

	// Request ID middleware — generates unique ID for every request.
	// Available in handlers via c.Locals("request_id").
	app.Use(middleware.RequestID())

	// Initialize database store
	db, err := store.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Serve static assets BEFORE catch-all (critical fix for CSS Embed Bug)
	// Static middleware MUST precede catch-all; registers /styles/* with proper MIME types (text/css)
	// This prevents /styles/tailwind.min.css from falling through to index.html handler
	app.Static("/styles", filepath.Join(publicPath, "styles"), fiber.Static{
		Browse:   false,
		Compress: true,
		MaxAge:   3600,
	})

	// Serve root-level static files (index.html, login.html, etc.) - REQUIRED
	// MUST come BEFORE catch-all and handlers. Prevents path leakage and 302 redirects
	// to FS paths like /opt/goban/public/ that break relative asset links and SSE.
	app.Static("/", publicPath, fiber.Static{
		Browse:   false,
		Compress: true,
		MaxAge:   3600,
	})

	// Register API routes AFTER static middleware so they always take precedence for /api/* paths.
	// This ensures POST/PUT/PATCH requests reach handlers instead of getting 405 from static serving.
	auth.SetJWTSecret([]byte(cfg.JWTSecret))
	auth.SetJWTConfig(cfg.JWTValidity, cfg.RefreshGracePeriod)
	if cfg.JWTSecret == "" {
		log.Fatalf("GOBAN_JWT_SECRET must be set via config or GOBAN_JWT_SECRET env var")
	}
	handlers.RegisterRoutes(app, db, cfg.Boards)

	// Health check endpoint (must be BEFORE catch-all handler for route precedence)
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": version.Version})
	})

	// Catch-all for SPA: serve index.html ONLY for non-API, non-static paths.
	// Registered LAST after all specific routes to ensure proper route precedence.
	// Uses GET method only (not All) since we only need to handle browser navigation requests.
	app.Get("/*", func(c *fiber.Ctx) error {
		path := c.Path()
		if logFilter.ShouldLog("debug") {
			log.Printf("DEBUG: SPA catch-all called for %s %s", c.Method(), path)
		}
		// Skip API endpoints, static assets, and special paths - return 404 to let other handlers take over
		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/styles/") ||
			strings.HasPrefix(path, "/healthz") ||
			strings.HasPrefix(path, "/events") ||
			strings.HasSuffix(path, ".css") ||
			strings.HasSuffix(path, ".js") ||
			strings.HasSuffix(path, ".html") {
			return fiber.ErrNotFound
		}
		indexPath := filepath.Join(publicPath, "index.html")
		return c.SendFile(indexPath)
	})

	// Start server in background
	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Log startup info
	log.Printf("Goban API listening on :%s (version %s)", cfg.Port, version.Version)
	log.Printf("Database: %s | Boards: %d", cfg.DBPath, len(cfg.Boards))

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v - shutting down gracefully", sig)
		if err := app.Shutdown(); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
		os.Exit(0)
	}()

	select {} // Block until interrupted
}
