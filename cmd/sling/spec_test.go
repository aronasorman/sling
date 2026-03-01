package main

import "testing"

// TestSpecCmdRegistered verifies that the spec command is wired into the root
// command with the correct usage string and argument requirement.
func TestSpecCmdRegistered(t *testing.T) {
	if specCmd == nil {
		t.Fatal("specCmd is nil")
	}

	if got := specCmd.Use; got != "spec <bead-id>" {
		t.Errorf("specCmd.Use = %q; want %q", got, "spec <bead-id>")
	}

	// The command must require exactly one argument (the bead ID).
	// Cobra returns an error when no args are provided.
	cmd := specCmd
	// REVIEW: cmd.SetArgs([]string{}) has no effect here — the test calls
	// cmd.Args(...) directly with an explicit slice, so the SetArgs call is
	// dead code and is misleading.  Remove it.
	cmd.SetArgs([]string{}) // no args
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("specCmd.Args should reject zero arguments")
	}
	if err := cmd.Args(cmd, []string{"bead-123"}); err != nil {
		t.Errorf("specCmd.Args should accept one argument, got error: %v", err)
	}
	if err := cmd.Args(cmd, []string{"bead-1", "bead-2"}); err == nil {
		t.Error("specCmd.Args should reject two arguments")
	}
}
