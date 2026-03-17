// Package tools provides a clean, self-contained tool interface for inber agents.
// Tools implementing this interface can be registered without any knowledge of
// inber internals, making them swappable and modular.
package tools

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
)

// Tool defines the interface for agent tools.
// Tools are completely self-contained: they know their name, description,
// schema, and how to execute. No knowledge of agent internals required.
type Tool interface {
	// Name returns the tool's unique identifier
	Name() string
	
	// Description returns a human-readable description for the LLM
	Description() string
	
	// Schema returns the JSON schema for the tool's input parameters
	Schema() anthropic.ToolInputSchemaParam
	
	// Execute runs the tool with the given JSON input and returns the result
	Execute(ctx context.Context, input string) (string, error)
}

// Registry manages tool registration and discovery.
// This allows tools to be added dynamically without touching agent code.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
// If a tool with the same name already exists, it will be replaced.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// List returns all registered tools as a slice.
func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered tools.
func (r *Registry) Count() int {
	return len(r.tools)
}

// Clear removes all tools from the registry.
func (r *Registry) Clear() {
	r.tools = make(map[string]Tool)
}