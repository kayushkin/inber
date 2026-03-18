package mcp

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// MockClient implements a simple mock of the MCP Client for testing
type MockClient struct {
	tools map[string]ToolInfo
}

// Ensure MockClient implements MCPClient interface
var _ MCPClient = (*MockClient)(nil)

func NewMockClient() *MockClient {
	return &MockClient{
		tools: make(map[string]ToolInfo),
	}
}

func (m *MockClient) AddMockTool(name, description string) {
	m.tools[name] = ToolInfo{
		Name:        name,
		Description: description,
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "string",
					"description": "Input for the tool",
				},
			},
		},
	}
}

func (m *MockClient) ListTools() []ToolInfo {
	var tools []ToolInfo
	for _, tool := range m.tools {
		tools = append(tools, tool)
	}
	return tools
}

func (m *MockClient) HasTool(name string) bool {
	_, exists := m.tools[name]
	return exists
}

func (m *MockClient) CallTool(ctx context.Context, name string, input string) (string, error) {
	if !m.HasTool(name) {
		return "", nil
	}
	return "mock result for " + name, nil
}

func (m *MockClient) Close() error {
	return nil
}

func TestMCPToolAdapter_NewMCPToolAdapter(t *testing.T) {
	client := NewMockClient()
	client.AddMockTool("test-tool", "A test tool")

	// Test successful adapter creation
	adapter, err := NewMCPToolAdapter(client, "test-tool")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("expected adapter to be non-nil")
	}
	if adapter.name != "test-tool" {
		t.Errorf("expected name 'test-tool', got %q", adapter.name)
	}

	// Test adapter creation for non-existent tool
	_, err = NewMCPToolAdapter(client, "nonexistent-tool")
	if err == nil {
		t.Fatal("expected error for non-existent tool")
	}
}

func TestMCPToolAdapter_ToAgentTool(t *testing.T) {
	client := NewMockClient()
	client.AddMockTool("test-tool", "A test tool")

	adapter, err := NewMCPToolAdapter(client, "test-tool")
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	agentTool := adapter.ToAgentTool()
	
	// Verify that we get back an agent.Tool type
	var _ agent.Tool = agentTool
	
	if agentTool.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got %q", agentTool.Name)
	}
	if agentTool.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", agentTool.Description)
	}
	if agentTool.Run == nil {
		t.Error("expected Run function to be set")
	}
}

func TestMCPToolAdapter_Run(t *testing.T) {
	client := NewMockClient()
	client.AddMockTool("test-tool", "A test tool")

	adapter, err := NewMCPToolAdapter(client, "test-tool")
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	result, err := adapter.run(context.Background(), `{"input": "test"}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := "mock result for test-tool"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMCPToolRegistry_NewMCPToolRegistry(t *testing.T) {
	registry := NewMCPToolRegistry()
	if registry == nil {
		t.Fatal("expected registry to be non-nil")
	}
	if registry.clients == nil {
		t.Fatal("expected clients map to be initialized")
	}
}

func TestMCPToolRegistry_AddClient(t *testing.T) {
	registry := NewMCPToolRegistry()
	client := NewMockClient()

	registry.AddClient("test-client", client)

	if len(registry.clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(registry.clients))
	}
	if registry.clients["test-client"] != client {
		t.Error("client not properly registered")
	}
}

func TestMCPToolRegistry_GetAllTools(t *testing.T) {
	registry := NewMCPToolRegistry()
	
	// Add first client with one tool
	client1 := NewMockClient()
	client1.AddMockTool("tool1", "First tool")
	registry.AddClient("client1", client1)

	// Add second client with two tools
	client2 := NewMockClient()
	client2.AddMockTool("tool2", "Second tool")
	client2.AddMockTool("tool3", "Third tool")
	registry.AddClient("client2", client2)

	tools := registry.GetAllTools()
	
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}

	// Verify all tools are present
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{"tool1", "tool2", "tool3"}
	for _, expectedTool := range expectedTools {
		if !toolNames[expectedTool] {
			t.Errorf("expected tool %q not found", expectedTool)
		}
	}
}

func TestMCPToolRegistry_GetTool(t *testing.T) {
	registry := NewMCPToolRegistry()
	client := NewMockClient()
	client.AddMockTool("test-tool", "A test tool")
	registry.AddClient("test-client", client)

	// Test existing tool
	tool := registry.GetTool("test-tool")
	if tool == nil {
		t.Fatal("expected tool to be found")
	}
	if tool.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got %q", tool.Name)
	}

	// Test non-existent tool
	tool = registry.GetTool("nonexistent-tool")
	if tool != nil {
		t.Error("expected nil for non-existent tool")
	}
}

func TestMCPToolRegistry_Close(t *testing.T) {
	registry := NewMCPToolRegistry()
	client := NewMockClient()
	registry.AddClient("test-client", client)

	err := registry.Close()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestMCPError_String(t *testing.T) {
	err := &MCPError{
		Code:    123,
		Message: "test error",
	}
	
	// This tests that the error struct is properly defined
	if err.Code != 123 {
		t.Errorf("expected code 123, got %d", err.Code)
	}
	if err.Message != "test error" {
		t.Errorf("expected message 'test error', got %q", err.Message)
	}
}

func TestMockClient_Interface(t *testing.T) {
	// This test ensures our mock client implements the necessary interface
	client := NewMockClient()
	client.AddMockTool("test", "test tool")

	// Test ListTools
	tools := client.ListTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}

	// Test HasTool
	if !client.HasTool("test") {
		t.Error("expected tool 'test' to exist")
	}
	if client.HasTool("nonexistent") {
		t.Error("expected tool 'nonexistent' to not exist")
	}

	// Test CallTool
	result, err := client.CallTool(context.Background(), "test", `{}`)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "mock result for test" {
		t.Errorf("unexpected result: %q", result)
	}

	// Test Close
	err = client.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}