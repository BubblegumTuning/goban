package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setEnv temporarily sets an env var and returns a cleanup function.
func setEnv(t *testing.T, key, value string) func() {
	t.Helper()
	old, exists := os.LookupEnv(key)
	err := os.Setenv(key, value)
	if err != nil {
		t.Fatalf("failed to set %s: %v", key, err)
	}
	return func() {
		if exists {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	}
}

// unsetEnv temporarily unsets an env var and returns a cleanup function.
func unsetEnv(t *testing.T, key string) func() {
	t.Helper()
	old, exists := os.LookupEnv(key)
	os.Unsetenv(key)
	return func() {
		if exists {
			os.Setenv(key, old)
		}
	}
}

// clearConfigEnvs unsets all config-related env vars and returns cleanup.
func clearConfigEnvs(t *testing.T) func() {
	t.Helper()
	vars := []string{"GOBAN_CONFIG", "GOBAN_PORT", "LOG_LEVEL", "DB_TYPE", "DB_PATH",
		"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "GOBAN_STATIC_PATH",
		"GOBAN_JWT_SECRET", "GOBAN_JWT_VALIDITY", "GOBAN_REFRESH_GRACE_PERIOD",
		"GOBAN_CORS_ORIGINS", "GOBAN_DEBUG"}
	old := make(map[string]string)
	var existed []string

	for _, v := range vars {
		if val, ok := os.LookupEnv(v); ok {
			old[v] = val
			existed = append(existed, v)
		} else {
			os.Unsetenv(v)
		}
	}
	return func() {
		for _, v := range existed {
			if orig, ok := old[v]; ok {
				os.Setenv(v, orig)
			} else {
				os.Unsetenv(v)
			}
		}
	}
}

// --- ResolveConfigPath tests ---

func TestResolveConfigPath(t *testing.T) {
	t.Run("GOBAN_CONFIG env var takes highest priority", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_CONFIG", "/custom/path/goban.toml")
		defer cleanup()
		result := ResolveConfigPath()
		if result != "/custom/path/goban.toml" {
			t.Errorf("expected /custom/path/goban.toml, got %s", result)
		}
	})

	t.Run("falls back to ~/.goban/goban.toml when it exists", func(t *testing.T) {
		cleanup := unsetEnv(t, "GOBAN_CONFIG")
		defer cleanup()

		tmpDir := t.TempDir()
		gobanDir := filepath.Join(tmpDir, ".goban")
		os.MkdirAll(gobanDir, 0755)
		configFile := filepath.Join(gobanDir, "goban.toml")
		os.WriteFile(configFile, []byte("port = \"9090\""), 0644)

		homeCleanup := setEnv(t, "HOME", tmpDir)
		defer homeCleanup()

		result := ResolveConfigPath()
		if result != configFile {
			t.Errorf("expected %s, got %s", configFile, result)
		}
	})

	t.Run("falls back to ./goban.toml when no other config exists", func(t *testing.T) {
		cleanup := unsetEnv(t, "GOBAN_CONFIG")
		defer cleanup()

		tmpDir := t.TempDir()
		homeCleanup := setEnv(t, "HOME", tmpDir)
		defer homeCleanup()

		result := ResolveConfigPath()
		if result != "./goban.toml" {
			t.Errorf("expected ./goban.toml, got %s", result)
		}
	})

	t.Run("GOBAN_CONFIG overrides ~/.goban/goban.toml even when it exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		gobanDir := filepath.Join(tmpDir, ".goban")
		os.MkdirAll(gobanDir, 0755)
		configFile := filepath.Join(gobanDir, "goban.toml")
		os.WriteFile(configFile, []byte("port = \"9090\""), 0644)

		homeCleanup := setEnv(t, "HOME", tmpDir)
		defer homeCleanup()

		envCleanup := setEnv(t, "GOBAN_CONFIG", "/explicit/config.toml")
		defer envCleanup()

		result := ResolveConfigPath()
		if result != "/explicit/config.toml" {
			t.Errorf("expected /explicit/config.toml (from GOBAN_CONFIG), got %s", result)
		}
	})
}

// --- GetDefaultConfig tests ---

func TestGetDefaultConfig(t *testing.T) {
	// Reset global Debug flag that may have been set by other tests loading config files.
	prevDebug := Debug
	Debug = false
	defer func() { Debug = prevDebug }()

	// Use a non-existent path to get pure defaults without file loading interference.
	cleanup := clearConfigEnvs(t)
	defer cleanup()

	tmpDir := t.TempDir()
	homeCleanup := setEnv(t, "HOME", tmpDir)
	defer homeCleanup()

	cfg := GetDefaultConfig()

	t.Run("default port is 8080", func(t *testing.T) {
		if cfg.Port != "8080" {
			t.Errorf("expected default port 8080, got %s", cfg.Port)
		}
	})

	t.Run("default log level is info", func(t *testing.T) {
		if cfg.LogLevel != "info" {
			t.Errorf("expected default log level info, got %s", cfg.LogLevel)
		}
	})

	t.Run("default db path is ./goban.db", func(t *testing.T) {
		if cfg.DBPath != "./goban.db" {
			t.Errorf("expected default DB path ./goban.db, got %s", cfg.DBPath)
		}
	})

	t.Run("default db type is sqlite", func(t *testing.T) {
		if cfg.DBType != "sqlite" {
			t.Errorf("expected default DB type sqlite, got %s", cfg.DBType)
		}
	})

	t.Run("default jwt validity is 30 days", func(t *testing.T) {
		expected := 30 * 24 * time.Hour
		if cfg.JWTValidity != expected {
			t.Errorf("expected JWT validity %v, got %v", expected, cfg.JWTValidity)
		}
	})

	t.Run("default refresh grace period is 90 days", func(t *testing.T) {
		expected := 90 * 24 * time.Hour
		if cfg.RefreshGracePeriod != expected {
			t.Errorf("expected refresh grace %v, got %v", expected, cfg.RefreshGracePeriod)
		}
	})

	t.Run("default boards include human-to-ai and ai-to-human", func(t *testing.T) {
		if len(cfg.Boards) < 2 {
			t.Errorf("expected at least 2 default boards, got %d", len(cfg.Boards))
		}
		hasHTAI := false
		hasATHuman := false
		for _, b := range cfg.Boards {
			if b.ID == "human-to-ai" {
				hasHTAI = true
			}
			if b.ID == "ai-to-human" {
				hasATHuman = true
			}
		}
		if !hasHTAI {
			t.Error("expected human-to-ai board in defaults")
		}
		if !hasATHuman {
			t.Error("expected ai-to-human board in defaults")
		}
	})

	t.Run("default debug is false", func(t *testing.T) {
		if cfg.Debug != false {
			t.Errorf("expected default Debug to be false, got %v", cfg.Debug)
		}
	})
}

// --- expandEnvVars tests ---

func TestExpandEnvVars(t *testing.T) {
	cleanup := setEnv(t, "TEST_VAR", "hello")
	defer cleanup()
	setEnv2 := setEnv(t, "DB_PASS", "secret123")
	defer setEnv2()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple replacement", "${TEST_VAR}", "hello"},
		{"embedded in text", "prefix-${TEST_VAR}-suffix", "prefix-hello-suffix"},
		{"multiple replacements", "${TEST_VAR} and ${DB_PASS}", "hello and secret123"},
		{"no placeholder", "plain text", "plain text"},
		{"empty var", "before-${NONEXISTENT}-after", "before--after"},
		{"unclosed brace", "${incomplete", "${incomplete"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandEnvVars(tc.input)
			if got != tc.want {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- parseTOML tests ---

func TestParseTOML(t *testing.T) {
	t.Run("parses valid TOML correctly", func(t *testing.T) {
		input := `port = "9090"
log_level = "debug"
db_type = "postgres"`
		cfg := parseTOML(input)

		if cfg.Port != "9090" {
			t.Errorf("expected port 9090, got %s", cfg.Port)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("expected log_level debug, got %s", cfg.LogLevel)
		}
		if cfg.DBType != "postgres" {
			t.Errorf("expected db_type postgres, got %s", cfg.DBType)
		}
	})

	t.Run("returns empty Config for malformed TOML", func(t *testing.T) {
		input := `port = [INVALID TOML {{{`
		cfg := parseTOML(input)

		if cfg.Port != "" {
			t.Errorf("expected empty port for malformed TOML, got %s", cfg.Port)
		}
	})

	t.Run("empty string returns zero Config", func(t *testing.T) {
		cfg := parseTOML("")
		if cfg.Port != "" || cfg.DBType != "" {
			t.Errorf("expected all-zero config for empty input, got %+v", cfg)
		}
	})

	t.Run("expands env vars within TOML content", func(t *testing.T) {
		cleanup := setEnv(t, "MY_PORT", "7777")
		defer cleanup()

		input := `port = "${MY_PORT}"`
		cfg := parseTOML(input)

		if cfg.Port != "7777" {
			t.Errorf("expected port 7777 from env expansion, got %s", cfg.Port)
		}
	})
}

// --- mergeWithEnvVars tests ---

func TestMergeWithEnvVars(t *testing.T) {
	base := Config{Port: "8080", LogLevel: "info", DBType: "sqlite", Debug: false}

	t.Run("GOBAN_PORT overrides port", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_PORT", "3000")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.Port != "3000" {
			t.Errorf("expected port 3000 from env, got %s", cfg.Port)
		}
	})

	t.Run("LOG_LEVEL overrides log level", func(t *testing.T) {
		cleanup := setEnv(t, "LOG_LEVEL", "debug")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.LogLevel != "debug" {
			t.Errorf("expected log level debug from env, got %s", cfg.LogLevel)
		}
	})

	t.Run("DB_TYPE overrides db type", func(t *testing.T) {
		cleanup := setEnv(t, "DB_TYPE", "postgres")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.DBType != "postgres" {
			t.Errorf("expected db_type postgres from env, got %s", cfg.DBType)
		}
	})

	t.Run("GOBAN_JWT_SECRET overrides jwt secret", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_JWT_SECRET", "env-secret")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.JWTSecret != "env-secret" {
			t.Errorf("expected jwt_secret env-secret from env, got %s", cfg.JWTSecret)
		}
	})

	t.Run("GOBAN_JWT_VALIDITY overrides jwt validity with valid duration", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_JWT_VALIDITY", "72h")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.JWTValidity != 72*time.Hour {
			t.Errorf("expected jwt validity 72h, got %v", cfg.JWTValidity)
		}
	})

	t.Run("GOBAN_JWT_VALIDITY ignored for invalid duration string", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_JWT_VALIDITY", "not-a-duration")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.JWTValidity != 0 {
			t.Errorf("expected jwt validity unchanged (0), got %v", cfg.JWTValidity)
		}
	})

	t.Run("GOBAN_DEBUG=true enables debug mode", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_DEBUG", "true")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if !cfg.Debug {
			t.Error("expected Debug to be true from GOBAN_DEBUG=true env var")
		}
	})

	t.Run("GOBAN_DEBUG=1 enables debug mode", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_DEBUG", "1")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if !cfg.Debug {
			t.Error("expected Debug to be true from GOBAN_DEBUG=1 env var")
		}
	})

	t.Run("env vars not set leave config unchanged", func(t *testing.T) {
		cleanup := clearConfigEnvs(t)
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.Port != "8080" || cfg.LogLevel != "info" || cfg.DBType != "sqlite" {
			t.Errorf("expected unchanged config when no env vars set, got %+v", cfg)
		}
	})

	t.Run("DB_HOST overrides host", func(t *testing.T) {
		cleanup := setEnv(t, "DB_HOST", "db.example.com")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.DBHost != "db.example.com" {
			t.Errorf("expected DB_HOST from env, got %s", cfg.DBHost)
		}
	})

	t.Run("GOBAN_CORS_ORIGINS overrides cors origins", func(t *testing.T) {
		cleanup := setEnv(t, "GOBAN_CORS_ORIGINS", "https://example.com")
		defer cleanup()
		cfg := mergeWithEnvVars(base)
		if cfg.CorsOrigins != "https://example.com" {
			t.Errorf("expected CORS origins from env, got %s", cfg.CorsOrigins)
		}
	})
}

// --- validateConfig tests ---

func TestValidateConfig(t *testing.T) {
	validCfg := Config{Port: "8080", DBType: "sqlite"}

	t.Run("valid sqlite config passes", func(t *testing.T) {
		err := validateConfig(validCfg)
		if err != nil {
			t.Errorf("expected no error for valid sqlite config, got %v", err)
		}
	})

	t.Run("empty port returns error", func(t *testing.T) {
		cfg := Config{Port: "", DBType: "sqlite"}
		err := validateConfig(cfg)
		if err == nil {
			t.Error("expected error for empty port")
		} else if !strings.Contains(err.Error(), "port is required") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("invalid DB_TYPE returns error", func(t *testing.T) {
		cfg := Config{Port: "8080", DBType: "mysql"}
		err := validateConfig(cfg)
		if err == nil {
			t.Error("expected error for invalid DB_TYPE mysql")
		} else if !strings.Contains(err.Error(), "invalid DB_TYPE") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("postgres without DB_HOST returns error", func(t *testing.T) {
		cfg := Config{Port: "8080", DBType: "postgres", DBName: "goban"}
		err := validateConfig(cfg)
		if err == nil {
			t.Error("expected error for postgres without DB_HOST")
		} else if !strings.Contains(err.Error(), "DB_HOST is required") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("postgres without DB_NAME returns error", func(t *testing.T) {
		cfg := Config{Port: "8080", DBType: "postgres", DBHost: "localhost"}
		err := validateConfig(cfg)
		if err == nil {
			t.Error("expected error for postgres without DB_NAME")
		} else if !strings.Contains(err.Error(), "DB_NAME is required") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("valid postgres config passes", func(t *testing.T) {
		cfg := Config{Port: "8080", DBType: "postgres", DBHost: "localhost", DBName: "goban"}
		err := validateConfig(cfg)
		if err != nil {
			t.Errorf("expected no error for valid postgres config, got %v", err)
		}
	})

	t.Run("empty DB_TYPE is allowed (defaults to sqlite)", func(t *testing.T) {
		cfg := Config{Port: "8080", DBType: ""}
		err := validateConfig(cfg)
		if err != nil {
			t.Errorf("expected no error for empty DB_TYPE, got %v", err)
		}
	})

	t.Run("wildcard CORS in non-debug mode does not return error (only warns)", func(t *testing.T) {
		cfg := Config{Port: "8080", DBType: "sqlite", CorsOrigins: "*", Debug: false}
		err := validateConfig(cfg)
		if err != nil {
			t.Errorf("expected no error for wildcard CORS (should only warn), got %v", err)
		}
	})
}

// --- NewLogFilter tests ---

func TestNewLogFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"debug level", "debug", "debug"},
		{"info level", "info", "info"},
		{"warn level", "warn", "warn"},
		{"error level", "error", "error"},
		{"uppercase normalized to lowercase", "DEBUG", "debug"},
		{"mixed case normalized", "Info", "info"},
		{"empty string defaults to info", "", "info"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lf := NewLogFilter(tc.input)
			if lf.level != tc.want {
				t.Errorf("NewLogFilter(%q).level = %q, want %q", tc.input, lf.level, tc.want)
			}
		})
	}
}

// --- ShouldLog tests ---

func TestShouldLog(t *testing.T) {
	// Reset global Debug flag that may have been set by other tests.
	prevDebug := Debug
	Debug = false
	defer func() { Debug = prevDebug }()

	tests := []struct {
		name       string
		filter     string
		msgLevel   string
		wantResult bool
	}{
		{"debug filter logs debug", "debug", "debug", true},
		{"debug filter logs info", "debug", "info", true},
		{"debug filter logs warn", "debug", "warn", true},
		{"debug filter logs error", "debug", "error", true},

		{"info filter does not log debug", "info", "debug", true},
		{"info filter logs info", "info", "info", true},
		{"info filter logs warn", "info", "warn", true},
		{"info filter logs error", "info", "error", true},

		{"warn filter does not log debug", "warn", "debug", false},
		{"warn filter does not log info", "warn", "info", false},
		{"warn filter logs warn", "warn", "warn", true},
		{"warn filter logs error", "warn", "error", true},

		{"error filter only logs error", "error", "debug", false},
		{"error filter does not log info", "error", "info", false},
		{"error filter does not log warn", "error", "warn", false},
		{"error filter logs error", "error", "error", true},

		{"unknown filter level defaults to info behavior", "verbose", "debug", true},
		{"unknown msg level defaults to info weight", "info", "trace", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lf := NewLogFilter(tc.filter)
			got := lf.ShouldLog(tc.msgLevel)
			if got != tc.wantResult {
				t.Errorf("ShouldLog(%q) with filter %q = %v, want %v",
					tc.msgLevel, tc.filter, got, tc.wantResult)
			}
		})
	}
}

// --- LoadConfig integration tests ---

func TestLoadConfig(t *testing.T) {
	t.Run("loads from valid TOML file and merges with defaults", func(t *testing.T) {
		cleanup := clearConfigEnvs(t)
		defer cleanup()

		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "goban.toml")
		content := `port = "9090"
log_level = "debug"
db_type = "postgres"
db_host = "localhost"
db_name = "testdb"`
		os.WriteFile(configFile, []byte(content), 0644)

		cfg := LoadConfig(configFile)

		if cfg.Port != "9090" {
			t.Errorf("expected port from file 9090, got %s", cfg.Port)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("expected log_level debug from file, got %s", cfg.LogLevel)
		}
		if cfg.DBType != "postgres" {
			t.Errorf("expected db_type postgres from file, got %s", cfg.DBType)
		}
		if cfg.DBHost != "localhost" {
			t.Errorf("expected DB_HOST localhost from file, got %s", cfg.DBHost)
		}
	})

	t.Run("env var overrides take precedence over TOML file values", func(t *testing.T) {
		cleanup := clearConfigEnvs(t)
		defer cleanup()
		envCleanup := setEnv(t, "GOBAN_PORT", "3000")
		defer envCleanup()

		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "goban.toml")
		os.WriteFile(configFile, []byte(`port = "9090"`), 0644)

		cfg := LoadConfig(configFile)
		if cfg.Port != "3000" {
			t.Errorf("expected port from env var (3000), not file (9090), got %s", cfg.Port)
		}
	})

	t.Run("missing file falls back to defaults without panic", func(t *testing.T) {
		cleanup := clearConfigEnvs(t)
		defer cleanup()

		cfg := LoadConfig("/nonexistent/path/goban.toml")
		if cfg.Port != "8080" {
			t.Errorf("expected default port 8080 for missing file, got %s", cfg.Port)
		}
	})

	t.Run("empty path uses defaults only", func(t *testing.T) {
		cleanup := clearConfigEnvs(t)
		defer cleanup()

		cfg := LoadConfig("")
		if cfg.Port != "8080" {
			t.Errorf("expected default port 8080, got %s", cfg.Port)
		}
	})

	t.Run("default boards are set when file has no boards", func(t *testing.T) {
		cleanup := clearConfigEnvs(t)
		defer cleanup()

		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "goban.toml")
		os.WriteFile(configFile, []byte(`port = "9090"`), 0644)

		cfg := LoadConfig(configFile)
		if len(cfg.Boards) < 2 {
			t.Errorf("expected at least 2 default boards when file has none, got %d", len(cfg.Boards))
		}
	})
}
