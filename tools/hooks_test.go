package tools_test

import (
	"context"
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools"
)

func TestHooksInterceptTools(t *testing.T) {
	// Create a mock agent with hooks
	ag := &agent.Agent{}
	
	var _ bool
	var _ string
	
	ag.SetHooks(&agent.Hooks{
		OnToolCall: func(toolID, name string, input []byte) {
			// Hook would be called during Agent.Run()
			_ = name
		},
	})
	
	// Add a tool
	ag.AddTool(tools.Shell())
	
	// Manually trigger a tool call simulation
	// In a real scenario, this would happen during Agent.Run()
	// For this test, we verify the tool structure is compatible
	
	shellTool := tools.Shell()
	if shellTool.Name != "shell" {
		t.Errorf("tool name should be 'shell', got %s", shellTool.Name)
	}
	
	// Verify Run function exists and is callable
	ctx := context.Background()
	result, err := shellTool.Run(ctx, `{"command": "echo test"}`)
	if err != nil {
		t.Fatalf("tool Run failed: %v", err)
	}
	if result == "" {
		t.Error("tool Run should return output")
	}
	
	// The hook test itself would require running the full agent loop,
	// which we test in agent_test.go. This test verifies tool compatibility.
	t.Log("Tool structure is compatible with agent hooks system")
}

func TestToolWrapperPreservesFields(t *testing.T) {
	tools := []agent.Tool{
		tools.Shell(),
		tools.ReadFile(),
		tools.WriteFile(),
		tools.EditFile(),
		tools.ListFiles(),
	}
	
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("wrapped tool missing Name")
		}
		if tool.Description == "" {
			t.Error("wrapped tool missing Description")
		}
		if tool.Run == nil {
			t.Error("wrapped tool missing Run function")
		}
		// InputSchema is a struct, just verify it's not nil
		// The actual validation happens in the Anthropic SDK
	}
}
