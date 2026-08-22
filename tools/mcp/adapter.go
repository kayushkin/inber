package mcp

import (
	"context"
	"fmt"
	"sort"

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

// clientNamesInOrder returns the registered client names sorted by name.
//
// Every method below walks the clients and folds their tools into one answer.
// Ranging the map directly would make that answer depend on Go's per-range map
// order: the tool list would come back in a different order on each call, and a
// name offered by two clients would resolve to a different one each time. Tool
// order is not cosmetic here — it is the order the tools reach the model, and
// the last definition in the array is what anchors the prompt cache breakpoint.
// Sorting by client name makes all of that reproducible.
func (r *MCPToolRegistry) clientNamesInOrder() []string {
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetAllTools returns all tools from all registered MCP clients as agent.Tool
// slice, walking the clients in name order and keeping each client's own tool
// order within its own block.
func (r *MCPToolRegistry) GetAllTools() []agent.Tool {
	var tools []agent.Tool

	for _, clientName := range r.clientNamesInOrder() {
		client := r.clients[clientName]
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

// GetTool returns a specific tool by name from any registered client. When more
// than one client offers the name, the first client in name order answers.
//
// Whether a duplicate name should be an error at all is an open question on
// noteboard card e2d0b07b and is deliberately not decided here; this only makes
// the answer the same on every call.
func (r *MCPToolRegistry) GetTool(name string) *agent.Tool {
	for _, clientName := range r.clientNamesInOrder() {
		client := r.clients[clientName]
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

// Close shuts down all MCP clients and reports the first error, walking the
// clients in name order so the same broken state names the same cause.
func (r *MCPToolRegistry) Close() error {
	var firstErr error

	for _, clientName := range r.clientNamesInOrder() {
		if err := r.clients[clientName].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
