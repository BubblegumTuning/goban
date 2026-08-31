package cmd

import "testing"

func TestRegenerateTokenHostFlagOverridesDefault(t *testing.T) {
	t.Cleanup(func() {
		_ = regenerateTokenCmd.Flags().Set("host", "kanban01")
	})

	if err := regenerateTokenCmd.ParseFlags([]string{"--host", "git02"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if got := sshHost(regenerateTokenCmd); got != "git02" {
		t.Errorf("sshHost = %q, want git02", got)
	}
}

func TestRegenerateTokenHostDefault(t *testing.T) {
	cmd := regenerateTokenCmd
	_ = cmd.Flags().Set("host", "kanban01")
	if got := sshHost(cmd); got != "kanban01" {
		t.Errorf("sshHost = %q, want kanban01", got)
	}
}
