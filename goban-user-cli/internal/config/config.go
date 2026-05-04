package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the CLI configuration for direct DB access.
type Config struct {
	DBType     string `yaml:"db_type"`     // "sqlite" or "postgres"
	DBPath     string `yaml:"db_path"`     // SQLite database path
	DBHost     string `yaml:"db_host"`     // PostgreSQL host
	DBPort     int    `yaml:"db_port"`     // PostgreSQL port
	DBUser     string `yaml:"db_user"`     // PostgreSQL user
	DBPassword string `yaml:"db_password"` // PostgreSQL password
	DBName     string `yaml:"db_name"`     // PostgreSQL database name
}

// Load reads configuration from environment variables or defaults.
// Priority: Environment variables > Hardcoded defaults
func Load() (*Config, error) {
	cfg := &Config{
		DBType:     getEnv("DB_TYPE", "sqlite"),
		DBPath:     getEnv("DB_PATH", "./goban.db"),
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getIntEnv("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", "goban"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "goban"),
	}

	// Try to load from Goban's config file if env vars aren't set
	if cfg.DBType == "sqlite" && cfg.DBPath == "./goban.db" {
		if gobanConfig := findGobanConfig(); gobanConfig != "" {
			parsedCfg := parseTOML(gobanConfig)
			if parsedCfg.DBType != "" {
				cfg.DBType = parsedCfg.DBType
			}
			if parsedCfg.DBPath != "" && cfg.DBType == "sqlite" {
				cfg.DBPath = parsedCfg.DBPath
			}
			if parsedCfg.DBHost != "" && cfg.DBType == "postgres" {
				cfg.DBHost = parsedCfg.DBHost
				cfg.DBPort = parsedCfg.DBPort
				cfg.DBUser = parsedCfg.DBUser
				cfg.DBPassword = parsedCfg.DBPassword
				cfg.DBName = parsedCfg.DBName
			}
		}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		fmt.Sscanf(value, "%d", &defaultValue)
	}
	return defaultValue
}

// findGobanConfig searches for goban.toml in standard locations.
func findGobanConfig() string {
	paths := []string{
		"/opt/goban/config/goban.toml",
		"/etc/goban/goban.toml",
		"~/goban/goban.toml",
		"./goban.toml",
	}

	for _, p := range paths {
		var expanded string
		if filepath.IsAbs(p) || !filepath.HasPrefix(p, "~/") {
			expanded = p
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			expanded = filepath.Join(home, p[2:])
		}

		if _, err := os.Stat(expanded); err == nil {
			return expanded
		}
	}
	return ""
}

// parseTOML parses a TOML configuration file and extracts DB settings.
func parseTOML(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	cfg := &Config{}
	content := string(data)

	// Split into lines for proper UTF-8-safe parsing
	lines := strings.Split(content, "\n")
	for _, rawLine := range lines {
		line := skipWhitespaceStr(rawLine)
		if len(line) == 0 {
			continue
		}

		if strings.HasPrefix(line, "db_type") && len(line) > 8 {
			cfg.DBType = extractValue(line[7:])
		} else if strings.HasPrefix(line, "db_path") && len(line) > 9 {
			cfg.DBPath = extractValue(line[9:])
		} else if strings.HasPrefix(line, "db_host") && len(line) > 10 {
			cfg.DBHost = extractValue(line[10:])
		} else if strings.HasPrefix(line, "db_port") && len(line) > 10 {
			fmt.Sscanf(extractValue(line[10:]), "%d", &cfg.DBPort)
		} else if strings.HasPrefix(line, "db_user") && len(line) > 10 {
			cfg.DBUser = extractValue(line[10:])
		} else if strings.HasPrefix(line, "db_password") && len(line) > 15 {
			cfg.DBPassword = extractValue(line[15:])
		} else if strings.HasPrefix(line, "db_name") && len(line) > 10 {
			cfg.DBName = extractValue(line[10:])
		}
	}

	return cfg
}

func extractValue(s string) string {
	s = skipWhitespaceStr(s)
	if len(s) == 0 {
		return ""
	}

	// Handle quoted strings
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		end := -1
		for i := 1; i < len(s); i++ {
			if s[i] == quote && (i == len(s)-1 || s[i+1] != '\\') {
				end = i
				break
			}
		}
		if end > 0 {
			return s[1:end]
		}
	}

	// Unquoted value - take until whitespace or comment
	for i, c := range s {
		if c == ' ' || c == '\t' || c == '#' {
			return s[:i]
		}
	}
	return s
}

func skipWhitespaceStr(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}
