package main

import (
	"strings"
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

func TestRunCmd_MessageParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "single word",
			args:     []string{"run", "hello"},
			expected: "hello",
		},
		{
			name:     "multiple words joined",
			args:     []string{"run", "hello", "world", "test"},
			expected: "hello world test",
		},
		{
			name:     "quoted string preserved",
			args:     []string{"run", "hello world"},
			expected: "hello world",
		},
		{
			name:     "flags before message",
			args:     []string{"run", "--raw", "my prompt"},
			expected: "my prompt",
		},
		{
			name:     "flags after message",
			args:     []string{"run", "my prompt", "--raw"},
			expected: "my prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the command to clear flag values
			runCmd.Flags().Parse([]string{})
			
			// Parse the args
			rootCmd.SetArgs(tt.args)
			
			// We can't actually execute without API key, but we can verify args parsing
			cmd, _, err := rootCmd.Find(tt.args)
			if err != nil {
				t.Fatalf("error finding command: %v", err)
			}
			
			// Should find the run command
			if cmd.Name() != "run" {
				t.Errorf("expected run command, got %s", cmd.Name())
			}
			
			// Parse flags to separate them from positional args
			cmd.ParseFlags(tt.args[1:]) // skip "run"
			remaining := cmd.Flags().Args()
			
			// Join remaining args to get the message
			got := strings.Join(remaining, " ")
			
			if got != tt.expected {
				t.Errorf("expected message %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestRootCmd_AsRunShortcut(t *testing.T) {
	// Test that `inber <prompt>` works as a shortcut for `inber run <prompt>`
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "direct prompt",
			args:     []string{"hello"},
			expected: "hello",
		},
		{
			name:     "multiple words",
			args:     []string{"hello", "world"},
			expected: "hello world",
		},
		{
			name:     "with flags",
			args:     []string{"--raw", "my prompt"},
			expected: "my prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			rootCmd.Flags().Parse([]string{})
			
			rootCmd.SetArgs(tt.args)
			
			// Find which command will be executed
			cmd, _, err := rootCmd.Find(tt.args)
			if err != nil {
				t.Fatalf("error finding command: %v", err)
			}
			
			// When no subcommand is given, root command itself is returned
			if cmd.Name() != "inber" {
				t.Errorf("expected root command (inber), got %s", cmd.Name())
			}
			
			// Parse the flags
			cmd.ParseFlags(tt.args)
			remaining := cmd.Flags().Args()
			
			got := strings.Join(remaining, " ")
			if got != tt.expected {
				t.Errorf("expected message %q, got %q", tt.expected, got)
			}
		})
	}
}
