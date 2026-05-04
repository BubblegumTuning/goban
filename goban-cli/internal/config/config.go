package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all configuration for the Goban CLI
type Config struct {
	API    APIConfig    `mapstructure:"api"`
	Output OutputConfig `mapstructure:"output"`
	Retry  RetryConfig  `mapstructure:"retry"`
}

// APIConfig holds API server configuration
type APIConfig struct {
	BaseURL  string `mapstructure:"base_url"`
	APIToken string `mapstructure:"api_token,omitempty"`
	Timeout  int    `mapstructure:"timeout"`
	BoardID  string `mapstructure:"board_id,omitempty"` // Default board ID
	User     string `mapstructure:"user,omitempty"`     // Default username
}

// OutputConfig holds output formatting configuration
type OutputConfig struct {
	Format   string `mapstructure:"format"`
	Colorize bool   `mapstructure:"colorize"`
}

// RetryConfig holds retry behavior configuration
type RetryConfig struct {
	MaxAttempts       int `mapstructure:"max_attempts"`
	InitialDelay      int `mapstructure:"initial_delay"`
	BackoffMultiplier int `mapstructure:"backoff_multiplier"`
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		API: APIConfig{
			BaseURL: "http://localhost:8080",
			Timeout: 30,
		},
		Output: OutputConfig{
			Format:   "line",
			Colorize: true,
		},
		Retry: RetryConfig{
			MaxAttempts:       3,
			InitialDelay:      1,
			BackoffMultiplier: 2,
		},
	}
}

// Load reads configuration from disk and merges with defaults.
// Priority: $GOBAN_CLI_CONFIG > ~/.goban/goban-cli/config.yaml > ~/.goban/config.yaml > ./config.yaml
func Load() (*Config, error) {
	cfg := DefaultConfig()

	viper.SetDefault("api.base_url", cfg.API.BaseURL)
	viper.SetDefault("api.timeout", cfg.API.Timeout)
	viper.SetDefault("output.format", cfg.Output.Format)
	viper.SetDefault("output.colorize", cfg.Output.Colorize)
	viper.SetDefault("retry.max_attempts", cfg.Retry.MaxAttempts)
	viper.SetDefault("retry.initial_delay", cfg.Retry.InitialDelay)
	viper.SetDefault("retry.backoff_multiplier", cfg.Retry.BackoffMultiplier)

	// Highest priority: explicit env var pointing to a config file
	if customPath := os.Getenv("GOBAN_CLI_CONFIG"); customPath != "" {
		viper.SetConfigFile(customPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		homeDir, err := os.UserHomeDir()
		if err == nil {
			// Try ~/.goban/goban-cli/ first (dedicated CLI config directory)
			cliConfigDir := filepath.Join(homeDir, ".goban", "goban-cli")
			viper.AddConfigPath(cliConfigDir)

			// Then ~/.goban/ for backwards compatibility
			configDir := filepath.Join(homeDir, ".goban")
			viper.AddConfigPath(configDir)
		}

		// Also try current directory for testing
		viper.AddConfigPath(".")
	}

	if err := viper.ReadInConfig(); err != nil {
		// Config file not found is okay, we'll use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	} else {
		// Successfully read config, unmarshal into our struct
		if err := viper.Unmarshal(cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// ConfigFile returns the path to the loaded config file
func ConfigFile() string {
	return viper.ConfigFileUsed()
}
