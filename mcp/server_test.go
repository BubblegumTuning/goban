package mcp

import (
	"testing"

	"goban/config"
)

func TestStart_DefaultConfig(t *testing.T) {
	cfg := config.GetDefaultConfig()
	// Default is now enabled + stdio; Start will block on stdio so test http path only
	cfg.MCPEnabled = true
	cfg.MCPTransport = "http"
	if err := Start(cfg); err != nil {
		t.Fatalf("Start http path failed: %v", err)
	}
}
