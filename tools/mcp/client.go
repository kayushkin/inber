// Package mcp implements a Model Context Protocol (MCP) client for dynamic tool loading.
// This allows inber to communicate with external tool servers via JSON-RPC over stdin/stdout.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// MCPRequest represents a JSON-RPC request to an MCP server
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC response from an MCP server  
type MCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC error
type MCPError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ToolInfo represents metadata about an available tool
type ToolInfo struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	InputSchema anthropic.ToolInputSchemaParam `json:"inputSchema"`
}

// Client manages communication with an MCP tool server
type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	scanner   *bufio.Scanner
	mu        sync.Mutex
	requestID int
	tools     map[string]ToolInfo
}

// NewClient creates a new MCP client that spawns the given command
func NewClient(command []string) (*Client, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("command cannot be empty")
	}

	cmd := exec.Command(command[0], command[1:]...)
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe() 
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	client := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		scanner: bufio.NewScanner(stdout),
		tools:   make(map[string]ToolInfo),
	}

	// Initialize the MCP connection
	if err := client.initialize(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize MCP connection: %w", err)
	}

	return client, nil
}

// initialize performs the MCP handshake and discovers available tools
func (c *Client) initialize() error {
	// Send initialization request
	initResp, err := c.call(context.Background(), "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "inber",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	// Parse initialization response (we could validate server capabilities here)
	_ = initResp

	// Send initialized notification
	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("failed to send initialized notification: %w", err)
	}

	// Discover available tools
	toolsResp, err := c.call(context.Background(), "tools/list", nil)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	// Parse tools response
	var toolsList struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(toolsResp, &toolsList); err != nil {
		return fmt.Errorf("failed to parse tools list: %w", err)
	}

	// Register discovered tools
	for _, tool := range toolsList.Tools {
		c.tools[tool.Name] = tool
	}

	return nil
}

// call sends a JSON-RPC request and waits for a response
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	c.requestID++
	id := fmt.Sprintf("%d", c.requestID)
	c.mu.Unlock()

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Send request
	if err := c.sendRequest(request); err != nil {
		return nil, fmt.Errorf("failed to send MCP request: %w", err)
	}

	// Wait for response
	response, err := c.waitForResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for MCP response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// notify sends a JSON-RPC notification (no response expected)
func (c *Client) notify(method string, params interface{}) error {
	request := MCPRequest{
		JSONRPC: "2.0", 
		Method:  method,
		Params:  params,
	}
	return c.sendRequest(request)
}

// sendRequest writes a JSON-RPC request to the process stdin
func (c *Client) sendRequest(request MCPRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("failed to write request: %w", err)
	}
	if _, err := c.stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// waitForResponse reads from stdout until it finds a response with the given ID
func (c *Client) waitForResponse(ctx context.Context, id string) (*MCPResponse, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second) // Default timeout
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !c.scanner.Scan() {
			if err := c.scanner.Err(); err != nil {
				return nil, fmt.Errorf("failed to read response: %w", err)
			}
			return nil, fmt.Errorf("unexpected end of output")
		}

		line := c.scanner.Bytes()
		var response MCPResponse
		if err := json.Unmarshal(line, &response); err != nil {
			// Skip non-JSON lines (might be stderr output)
			continue
		}

		if response.ID == id {
			return &response, nil
		}
		// This response is for a different request, ignore it for now
		// (in a full implementation we'd buffer these)
	}

	return nil, fmt.Errorf("timeout waiting for response")
}

// CallTool executes a tool with the given name and input
func (c *Client) CallTool(ctx context.Context, name string, input string) (string, error) {
	if _, exists := c.tools[name]; !exists {
		return "", fmt.Errorf("tool %q not found", name)
	}

	// Parse input JSON to extract arguments
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("failed to parse tool input: %w", err)
	}

	// Call the tool via MCP
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", fmt.Errorf("failed to call MCP tool %q: %w", name, err)
	}

	// Parse the result
	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return "", fmt.Errorf("failed to parse tool result: %w", err)
	}

	// Extract text content
	var output string
	for _, content := range toolResult.Content {
		if content.Type == "text" {
			output += content.Text
		}
	}

	return output, nil
}

// ListTools returns information about all available tools
func (c *Client) ListTools() []ToolInfo {
	var tools []ToolInfo
	for _, tool := range c.tools {
		tools = append(tools, tool)
	}
	return tools
}

// HasTool checks if a tool with the given name is available
func (c *Client) HasTool(name string) bool {
	_, exists := c.tools[name]
	return exists
}

// Close shuts down the MCP client and kills the subprocess
func (c *Client) Close() error {
	var errs []error

	if c.stdin != nil {
		errs = append(errs, c.stdin.Close())
	}
	if c.stdout != nil {
		errs = append(errs, c.stdout.Close())
	}
	if c.stderr != nil {
		errs = append(errs, c.stderr.Close())
	}

	if c.cmd != nil {
		if c.cmd.Process != nil {
			c.cmd.Process.Kill()
		}
		errs = append(errs, c.cmd.Wait())
	}

	// Return first error encountered
	for _, err := range errs {
		if err != nil {
			return fmt.Errorf("failed to close MCP client resources: %w", err)
		}
	}
	return nil
}