// Package config provides configuration loading and management for Goban.
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Debug is a global flag that gates all DEBUG-level log.Printf calls across the application.
// Set this from main() after loading configuration: config.Debug = cfg.Debug
var Debug bool

// Config holds all runtime configuration for the application.
type Config struct {
	Port        string  `toml:"port"`
	Debug       bool    `toml:"debug"`        // Enable debug logging (gates DEBUG log.Printf calls)
	LogLevel    string  `toml:"log_level"`    // "debug", "info", "warn", "error"
	DBPath      string  `toml:"db_path"`      // For SQLite backend
	DBType      string  `toml:"db_type"`      // "sqlite" (default) or "postgres"
	DBHost      string  `toml:"db_host"`      // PostgreSQL host
	DBPort      int     `toml:"db_port"`      // PostgreSQL port
	DBUser      string  `toml:"db_user"`      // PostgreSQL user
	DBPassword  string  `toml:"db_password"`  // PostgreSQL password (masked in logs)
	DBName      string  `toml:"db_name"`      // PostgreSQL database name
	StaticPath  string  `toml:"static_path"`  // Path to static files (HTML/CSS/JS)
	JWTSecret         string        `toml:"jwt_secret"`         // JWT signing secret for web UI auth
	JWTValidity       time.Duration `toml:"jwt_validity"`       // JWT token validity period (e.g. "30d", "72h")
	RefreshGracePeriod time.Duration `toml:"refresh_grace_period"` // Window to refresh expired tokens without re-auth
	CorsOrigins string  `toml:"cors_origins"` // Comma-separated allowed CORS origins (empty = same-origin only)
	MCPEnabled   bool   `toml:"mcp_enabled"`
	MCPTransport string `toml:"mcp_transport"` // "stdio" (default) or "http"
	Version     string  `toml:"version"`      // Default or from build tag
	Boards      []Board `toml:"boards"`
}

// Board represents a Kanban board configuration.
type Board struct {
	ID      string   `toml:"id"`
	Title   string   `toml:"title"`
	Desc    string   `toml:"desc,omitempty"`
	Columns []string `toml:"columns"`
}

// ResolveConfigPath determines which config file to use.
// Priority: 1) $GOBAN_CONFIG env var, 2) ~/.goban/goban.toml, 3) ./goban.toml
func ResolveConfigPath() string {
	// Highest priority: explicit env var
	if p := os.Getenv("GOBAN_CONFIG"); p != "" {
		return p
	}

	// Second: persistent user config in home directory
	if home, err := os.UserHomeDir(); err == nil {
		userCfg := filepath.Join(home, ".goban", "goban.toml")
		if _, err := os.Stat(userCfg); err == nil {
			return userCfg
		}
	}

	// Fallback: local file in working directory
	return "./goban.toml"
}

// LoadConfig loads configuration from a TOML file with sensible defaults.
// Priority: Environment variables > TOML config file > Hardcoded defaults
func LoadConfig(path string) Config {
	if path == "" {
		path = ResolveConfigPath()
	}
	// Step 1: Start with hardcoded defaults
	cfg := Config{
		Port:       "8080",
		LogLevel:   "info", // Default to info (no debug logs in production)
		DBPath:     "./goban.db",
		DBType:     "sqlite", // Default to SQLite for backwards compatibility
		DBHost:     "localhost",
		DBPort:     5432,
		DBUser:     "goban",
		DBPassword: "",
		DBName:     "goban",
		JWTSecret:  "", // MUST be set via config file or GOBAN_JWT_SECRET env var
		JWTValidity:       30 * 24 * time.Hour, // Default token validity: 30 days
		RefreshGracePeriod: 90 * 24 * time.Hour, // Refresh window: 90 days
		MCPEnabled:   true,
		MCPTransport: "stdio",
	}

	// Step 2: Load from TOML file if it exists (overrides defaults)
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			log.Printf("[INFO] Loading config from %s", path)
			parsedCfg := parseTOML(string(data))
			// Merge parsed values (only override if non-empty)
			if parsedCfg.Port != "" {
				cfg.Port = parsedCfg.Port
			}
			if parsedCfg.LogLevel != "" {
				cfg.LogLevel = parsedCfg.LogLevel
			}
			if parsedCfg.DBPath != "" {
				cfg.DBPath = parsedCfg.DBPath
			}
			if parsedCfg.DBType != "" {
				cfg.DBType = parsedCfg.DBType
			}
			if parsedCfg.DBHost != "" {
				cfg.DBHost = parsedCfg.DBHost
			}
			if parsedCfg.DBPort != 0 {
				cfg.DBPort = parsedCfg.DBPort
			}
			if parsedCfg.DBUser != "" {
				cfg.DBUser = parsedCfg.DBUser
			}
			if parsedCfg.DBPassword != "" {
				cfg.DBPassword = parsedCfg.DBPassword
			}
			if parsedCfg.DBName != "" {
				cfg.DBName = parsedCfg.DBName
			}
			if parsedCfg.StaticPath != "" {
				cfg.StaticPath = parsedCfg.StaticPath
			}
	if parsedCfg.JWTSecret != "" {
			cfg.JWTSecret = parsedCfg.JWTSecret
		}
		if parsedCfg.JWTValidity != 0 {
			cfg.JWTValidity = parsedCfg.JWTValidity
		}
		if parsedCfg.RefreshGracePeriod != 0 {
			cfg.RefreshGracePeriod = parsedCfg.RefreshGracePeriod
		}
		if parsedCfg.CorsOrigins != "" {
			cfg.CorsOrigins = parsedCfg.CorsOrigins
		}
		if parsedCfg.MCPEnabled {
			cfg.MCPEnabled = parsedCfg.MCPEnabled
		}
		if parsedCfg.MCPTransport != "" {
			cfg.MCPTransport = parsedCfg.MCPTransport
		}
			if parsedCfg.Debug {
				cfg.Debug = parsedCfg.Debug
			}
			if len(parsedCfg.Boards) > 0 {
				cfg.Boards = parsedCfg.Boards
			}
		} else if err != nil && !os.IsNotExist(err) {
			log.Printf("[WARN] Error reading config file: %v", err)
		}
	}

	// Step 3: Override with environment variables (highest priority)
	cfg = mergeWithEnvVars(cfg)

	// Step 4: Validate configuration
	if err := validateConfig(cfg); err != nil {
		log.Printf("[ERROR] Config validation failed: %v", err)
	}


	// Step 5: Default board configuration matching production
	if len(cfg.Boards) == 0 {
		cfg.Boards = []Board{
			{
				ID:      "human-to-ai",
				Title:   "Human → AI",
				Desc:    "Track tasks assigned from human users to AI agents",
				Columns: []string{"Backlog", "To Do", "In Progress", "Review", "Done", "Cancelled"},
			},
			{
				ID:      "ai-to-human",
				Title:   "AI → Human",
				Desc:    "Track tasks assigned from AI agents to human users",
				Columns: []string{"Backlog", "To Do", "In Progress", "Review", "Done", "Cancelled"},
			},
		}
	}

	return cfg
}

// GetDefaultConfig returns a config with production-ready defaults.
func GetDefaultConfig() Config {
	return LoadConfig("") // Empty path triggers defaults only
}

// expandEnvVars replaces ${VAR_NAME} patterns in strings with environment variable values.
func expandEnvVars(s string) string {
	// Simple regex-like expansion for ${VAR} syntax
	result := s
	for len(result) > 0 {
		start := -1
		end := -1
		for i, c := range result {
			if start == -1 && i+2 < len(result) && result[i:i+2] == "${" {
				start = i + 2
			} else if start != -1 && c == '}' {
				end = i
				break
			}
		}
		if start == -1 || end == -1 {
			break // No more ${...} patterns
		}
		varName := result[start:end]
		envVal := os.Getenv(varName)
		result = result[:start-2] + envVal + result[end+1:]
	}
	return result
}

// parseTOML parses a TOML configuration file and returns the config.
// Values from the TOML file override defaults but environment variables take precedence (called after).
func parseTOML(content string) Config {
	var cfg Config

	// Expand ${VAR_NAME} patterns in the content before parsing
	content = expandEnvVars(content)

	// Parse TOML content
	err := toml.Unmarshal([]byte(content), &cfg)
	if err != nil {
		log.Printf("[WARN] Failed to parse TOML: %v", err)
		return Config{}
	}

	return cfg
}

// mergeWithEnvVars merges parsed config with environment variable overrides.
// Environment variables always take precedence over config file values.
func mergeWithEnvVars(cfg Config) Config {
	// Override with environment variables if set
	if os.Getenv("GOBAN_PORT") != "" {
		cfg.Port = os.Getenv("GOBAN_PORT")
	}
	if os.Getenv("LOG_LEVEL") != "" {
		cfg.LogLevel = os.Getenv("LOG_LEVEL")
	}
	if os.Getenv("DB_TYPE") != "" {
		cfg.DBType = os.Getenv("DB_TYPE")
	}
	if os.Getenv("DB_PATH") != "" {
		cfg.DBPath = os.Getenv("DB_PATH")
	}
	if os.Getenv("DB_HOST") != "" {
		cfg.DBHost = os.Getenv("DB_HOST")
	}
	if os.Getenv("DB_USER") != "" {
		cfg.DBUser = os.Getenv("DB_USER")
	}
	if os.Getenv("DB_PASSWORD") != "" {
		cfg.DBPassword = os.Getenv("DB_PASSWORD")
	}
	if os.Getenv("DB_NAME") != "" {
		cfg.DBName = os.Getenv("DB_NAME")
	}
	if os.Getenv("GOBAN_STATIC_PATH") != "" {
		cfg.StaticPath = os.Getenv("GOBAN_STATIC_PATH")
	}
	if os.Getenv("GOBAN_JWT_SECRET") != "" {
		cfg.JWTSecret = os.Getenv("GOBAN_JWT_SECRET")
	}
	if v := os.Getenv("GOBAN_JWT_VALIDITY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.JWTValidity = d
		}
	}
	if v := os.Getenv("GOBAN_REFRESH_GRACE_PERIOD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.RefreshGracePeriod = d
		}
	}
	if os.Getenv("GOBAN_CORS_ORIGINS") != "" {
		cfg.CorsOrigins = os.Getenv("GOBAN_CORS_ORIGINS")
	}
	if v := os.Getenv("GOBAN_DEBUG"); strings.ToLower(v) == "true" || v == "1" {
		cfg.Debug = true
	}

	return cfg
}

// validateConfig checks that required configuration values are set.
func validateConfig(cfg Config) error {
	if cfg.Port == "" {
		return fmt.Errorf("server port is required")
	}
	if cfg.DBType != "sqlite" && cfg.DBType != "postgres" && cfg.DBType != "" {
		return fmt.Errorf("invalid DB_TYPE '%s', must be 'sqlite' or 'postgres'", cfg.DBType)
	}

	// PostgreSQL requires additional fields
	if cfg.DBType == "postgres" {
		if cfg.DBHost == "" {
			return fmt.Errorf("DB_HOST is required for PostgreSQL")
		}
		if cfg.DBName == "" {
			return fmt.Errorf("DB_NAME is required for PostgreSQL")
		}
	}

	// Warn if wildcard CORS is used outside debug mode
	if cfg.CorsOrigins == "*" && !cfg.Debug {
		log.Printf("[WARN] CORS AllowOrigins set to '*' (wildcard) in non-debug mode — restrict in production")
	}

	return nil
}

// LogFilter provides log level filtering based on configuration.
type LogFilter struct {
	level string
}

// NewLogFilter creates a new LogFilter with the specified log level.
func NewLogFilter(level string) *LogFilter {
	// Normalize to lowercase
	lower := strings.ToLower(level)
	if lower == "" {
		lower = "info" // Default
	}
	return &LogFilter{level: lower}
}

// ShouldLog returns true if the given log level should be logged.
// Level hierarchy (lowest to highest): debug < info < warn < error
func (lf *LogFilter) ShouldLog(level string) bool {
	levelWeights := map[string]int{
		"debug": 0,
		"info":  1,
		"warn":  2,
		"error": 3,
	}

	filterWeight := levelWeights[lf.level]
	if filterWeight == 0 && lf.level != "debug" {
		filterWeight = 1 // Default to info if unknown
	}

	msgWeight := levelWeights[strings.ToLower(level)]
	if msgWeight == 0 {
		msgWeight = 1 // Default to info if unknown
	}

	return msgWeight >= filterWeight
}
