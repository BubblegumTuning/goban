package cmd

import "testing"

func TestClaimStatusFlagBindsToTargetStatus(t *testing.T) {
	t.Cleanup(func() {
		targetStatus = ""
		_ = claimCmd.Flags().Set("status", "")
	})

	if err := claimCmd.ParseFlags([]string{"--status", "review"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if targetStatus != "review" {
		t.Errorf("targetStatus = %q, want %q (flag is registered but not bound)", targetStatus, "review")
	}
}
