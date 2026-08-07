package engine

import (
	"testing"

	"github.com/kayushkin/inber/internal/fsutil"
)

func TestLogger_Functions(t *testing.T) {
	// Test that all logger functions work without panicking
	Log.Info("test info: %s", "working")
	Log.Infof("test infof: %s", "working")
	Log.Warn("test warning: %s", "working") 
	Log.Error("test error: %s", "working")
	Log.Errorf("test errorf: %s", "working")
	Log.Plain("test plain: %s", "working")
}

func TestAutoWorkflowConfig_Defaults(t *testing.T) {
	// Test that we can create a config without panicking
	cfg := AutoWorkflowConfig{
		AutoCommit:     false,
		AutoFormat:     true,
		SmartTests:     true,
		VerifyDeployed: false,
	}

	if cfg.AutoCommit != false {
		t.Error("Expected AutoCommit to be false")
	}
	if cfg.AutoFormat != true {
		t.Error("Expected AutoFormat to be true")
	}
	if cfg.SmartTests != true {
		t.Error("Expected SmartTests to be true")
	}
	if cfg.VerifyDeployed != false {
		t.Error("Expected VerifyDeployed to be false")
	}
}

func TestEngineConfig_Basic(t *testing.T) {
	// Test that we can create an EngineConfig
	cfg := EngineConfig{
		Model:     "claude-3-5-sonnet-20241022",
		Thinking:  1000,
		AgentName: "test-agent",
		Raw:       false,
		NoTools:   false,
		NoHooks:   false,
	}

	if cfg.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Expected model to be claude-3-5-sonnet-20241022, got %s", cfg.Model)
	}
	if cfg.Thinking != 1000 {
		t.Errorf("Expected thinking to be 1000, got %d", cfg.Thinking)
	}
	if cfg.AgentName != "test-agent" {
		t.Errorf("Expected agent name to be test-agent, got %s", cfg.AgentName)
	}
}

func TestDisplayHooks_Basic(t *testing.T) {
	// Test that we can create DisplayHooks
	var thinkingCalled bool
	var textDeltaCalled bool
	var toolCallCalled bool
	var toolResultCalled bool

	hooks := &DisplayHooks{
		OnThinking: func(text string) {
			thinkingCalled = true
		},
		OnTextDelta: func(text string) {
			textDeltaCalled = true
		},
		OnToolCall: func(toolID, name, input string) {
			toolCallCalled = true
		},
		OnToolResult: func(toolID, name, output string, isError bool) {
			toolResultCalled = true
		},
	}

	// Test the hooks
	if hooks.OnThinking != nil {
		hooks.OnThinking("test thinking")
	}
	if hooks.OnTextDelta != nil {
		hooks.OnTextDelta("test delta")
	}
	if hooks.OnToolCall != nil {
		hooks.OnToolCall("toolu_test", "test_tool", "test input")
	}
	if hooks.OnToolResult != nil {
		hooks.OnToolResult("toolu_test", "test_tool", "test output", false)
	}

	if !thinkingCalled {
		t.Error("Expected OnThinking to be called")
	}
	if !textDeltaCalled {
		t.Error("Expected OnTextDelta to be called")
	}
	if !toolCallCalled {
		t.Error("Expected OnToolCall to be called")
	}
	if !toolResultCalled {
		t.Error("Expected OnToolResult to be called")
	}
}

func TestFindRepoRoot_ErrorHandling(t *testing.T) {
	// Test that FindRepoRoot doesn't panic when called in non-git directory
	_, err := fsutil.FindRepoRoot()
	// We don't assert success/failure since it depends on the test environment,
	// just that it doesn't panic
	_ = err
}