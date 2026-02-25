package main

import (
	"testing"
)

func TestRunCmd_Flags(t *testing.T) {
	// Verify the run command is registered and has expected flags
	cmd, _, err := rootCmd.Find([]string{"run"})
	if err != nil {
		t.Fatalf("run command not found: %v", err)
	}

	if cmd.Use != "run [message]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	// Check flags exist
	flags := []string{"model", "thinking", "agent", "raw", "no-tools", "system"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s to be registered", name)
		}
	}

	// Check short flags
	shortFlags := map[string]string{"model": "m", "thinking": "t", "agent": "a"}
	for long, short := range shortFlags {
		f := cmd.Flags().Lookup(long)
		if f == nil {
			t.Errorf("flag --%s not found", long)
			continue
		}
		if f.Shorthand != short {
			t.Errorf("expected shorthand -%s for --%s, got -%s", short, long, f.Shorthand)
		}
	}
}

func TestRunCmd_Defaults(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"run"})

	// raw defaults to false
	f := cmd.Flags().Lookup("raw")
	if f.DefValue != "false" {
		t.Errorf("expected raw default false, got %s", f.DefValue)
	}

	// no-tools defaults to false
	f = cmd.Flags().Lookup("no-tools")
	if f.DefValue != "false" {
		t.Errorf("expected no-tools default false, got %s", f.DefValue)
	}
}
