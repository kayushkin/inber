package tools

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// AgentToolAdapter adapts an agent.Tool to implement the Tool interface.
// This provides backward compatibility while allowing migration to the interface.
type AgentToolAdapter struct {
	tool agent.Tool
}

// NewAgentToolAdapter wraps an agent.Tool to implement the Tool interface.
func NewAgentToolAdapter(tool agent.Tool) Tool {
	return &AgentToolAdapter{tool: tool}
}

// Name implements Tool.Name
func (a *AgentToolAdapter) Name() string {
	return a.tool.Name
}

// Description implements Tool.Description  
func (a *AgentToolAdapter) Description() string {
	return a.tool.Description
}

// Schema implements Tool.Schema
func (a *AgentToolAdapter) Schema() anthropic.ToolInputSchemaParam {
	return a.tool.InputSchema
}

// Execute implements Tool.Execute
func (a *AgentToolAdapter) Execute(ctx context.Context, input string) (string, error) {
	return a.tool.Run(ctx, input)
}

// ToAgentTool converts a Tool interface back to agent.Tool for compatibility.
// This allows the engine to continue using agent.Tool while tools can be
// developed against the clean interface.
func ToAgentTool(tool Tool) agent.Tool {
	// If it's already an adapted agent.Tool, unwrap it
	if adapter, ok := tool.(*AgentToolAdapter); ok {
		return adapter.tool
	}
	
	// Otherwise, create a new agent.Tool from the interface
	return agent.Tool{
		Name:        tool.Name(),
		Description: tool.Description(),
		InputSchema: tool.Schema(),
		Run:         tool.Execute,
	}
}