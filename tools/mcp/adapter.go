package mcp

import (
	"context"
	"fmt"

	"github.com/kayushkin/inber/agent"
)

// MCPClient defines the interface that MCP clients must implement
type MCPClient interface {
	ListTools() []ToolInfo
	HasTool(name string) bool
	CallTool(ctx context.Context, name string, input string) (string, error)
	Close() error
}

// MCPToolAdapter bridges an MCP tool to the agent.Tool interface
type MCPToolAdapter struct {
	name   string
	client MCPClient
	info   ToolInfo
}

// NewMCPToolAdapter creates a new adapter for an MCP tool
func NewMCPToolAdapter(client MCPClient, toolName string) (*MCPToolAdapter, error) {
	tools := client.ListTools()
	for _, tool := range tools {
		if tool.Name == toolName {
			return &MCPToolAdapter{
				name:   toolName,
				client: client,
				info:   tool,
			}, nil
		}
	}

	return nil, fmt.Errorf("tool %q not found in MCP client", toolName)
}

// ToAgentTool converts the MCP tool to an agent.Tool
func (a *MCPToolAdapter) ToAgentTool() agent.Tool {
	return agent.Tool{
		Name:        a.info.Name,
		Description: a.info.Description,
		InputSchema: a.info.InputSchema,
		Run:         a.run,
	}
}

// run executes the MCP tool
func (a *MCPToolAdapter) run(ctx context.Context, input string) (string, error) {
	return a.client.CallTool(ctx, a.name, input)
}

// MCPToolRegistry manages multiple MCP clients and their tools
type MCPToolRegistry struct {
	clients map[string]MCPClient
}

// NewMCPToolRegistry creates a new registry for MCP tools
func NewMCPToolRegistry() *MCPToolRegistry {
	return &MCPToolRegistry{
		clients: make(map[string]MCPClient),
	}
}

// AddClient adds an MCP client to the registry
func (r *MCPToolRegistry) AddClient(name string, client MCPClient) {
	r.clients[name] = client
}

// GetAllTools returns all tools from all registered MCP clients as agent.Tool slice
func (r *MCPToolRegistry) GetAllTools() []agent.Tool {
	var tools []agent.Tool
	
	for _, client := range r.clients {
		for _, toolInfo := range client.ListTools() {
			adapter := &MCPToolAdapter{
				name:   toolInfo.Name,
				client: client,
				info:   toolInfo,
			}
			tools = append(tools, adapter.ToAgentTool())
		}
	}
	
	return tools
}

// GetTool returns a specific tool by name from any registered client
func (r *MCPToolRegistry) GetTool(name string) *agent.Tool {
	for _, client := range r.clients {
		if client.HasTool(name) {
			adapter, err := NewMCPToolAdapter(client, name)
			if err == nil {
				tool := adapter.ToAgentTool()
				return &tool
			}
		}
	}
	return nil
}

// Close shuts down all MCP clients
func (r *MCPToolRegistry) Close() error {
	var firstErr error
	
	for _, client := range r.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	
	return firstErr
}