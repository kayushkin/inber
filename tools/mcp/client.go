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

// defaultResponseTimeout bounds a single call when its context carries no
// deadline of its own.
const defaultResponseTimeout = 30 * time.Second

// jsonrpcMessage is a line read from the server before we know what it is. A
// reply to one of our requests carries an id and no method; a request or
// notification the server sent us carries a method.
type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   *MCPError       `json:"error"`
}

// Client manages communication with an MCP tool server.
//
// One goroutine owns the server's output: it reads every line and hands each
// reply to the call that is waiting for that request id. A call therefore never
// reads the stream itself, so it cannot consume a reply belonging to another
// call in flight, and its deadline bounds the whole wait rather than one read
// off a stream that may never produce another line.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu        sync.Mutex
	requestID int
	// waiters holds one channel per request that has been sent and not yet
	// answered, keyed by request id. A waiter is registered before the request
	// is written, so no reply can arrive with nowhere to go.
	waiters map[string]chan *MCPResponse
	tools   map[string]ToolInfo
	readErr error

	// readerDone closes when the reading goroutine stops, which is the only
	// thing that can free a waiter the server never answers.
	readerDone chan struct{}
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

	client := newClientOverStreams(stdin, stdout, stderr)
	client.cmd = cmd

	// Initialize the MCP connection
	if err := client.initialize(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize MCP connection: %w", err)
	}

	return client, nil
}

// newClientOverStreams wires a client to an already-open pair of streams and
// starts the goroutine that reads the server's replies. It exists so the
// transport can be exercised without spawning a process.
func newClientOverStreams(stdin io.WriteCloser, stdout io.ReadCloser, stderr io.ReadCloser) *Client {
	client := &Client{
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		waiters:    make(map[string]chan *MCPResponse),
		tools:      make(map[string]ToolInfo),
		readerDone: make(chan struct{}),
	}
	go client.readResponses()
	return client
}

// readResponses reads the server's output for the life of the client and
// delivers each reply to the call waiting for it. It returns only when the
// output ends, which frees every call still waiting.
func (c *Client) readResponses() {
	scanner := bufio.NewScanner(c.stdout)
	for scanner.Scan() {
		c.deliver(scanner.Bytes())
	}

	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}

	c.mu.Lock()
	c.readErr = err
	c.mu.Unlock()
	close(c.readerDone)
}

// deliver routes one line of server output to the call that asked for it.
func (c *Client) deliver(line []byte) {
	var message jsonrpcMessage
	if err := json.Unmarshal(line, &message); err != nil {
		// Not JSON: some servers log to stdout alongside the protocol.
		return
	}
	if message.Method != "" {
		// A request or notification of the server's own. We advertise no
		// capabilities that would prompt one and do not answer them.
		return
	}
	if message.ID == "" {
		return
	}

	c.mu.Lock()
	waiter, waiting := c.waiters[message.ID]
	if waiting {
		delete(c.waiters, message.ID)
	}
	c.mu.Unlock()

	if !waiting {
		// The only reply that can arrive with no waiter is one for a call that
		// already gave up and reported its own timeout. Request ids are never
		// reused, so nothing else will ever claim it.
		return
	}

	waiter <- &MCPResponse{
		JSONRPC: message.JSONRPC,
		ID:      message.ID,
		Result:  message.Result,
		Error:   message.Error,
	}
}

// abandonWaiter drops a request's waiter, so a reply that arrives after the
// call has given up is discarded rather than left in the map forever.
func (c *Client) abandonWaiter(id string) {
	c.mu.Lock()
	delete(c.waiters, id)
	c.mu.Unlock()
}

// readError reports why the server's output ended. Only meaningful once
// readerDone is closed.
func (c *Client) readError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
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
	// Register the waiter before the request goes out, so a reply that comes
	// back immediately still has somewhere to land.
	waiter := make(chan *MCPResponse, 1)
	c.mu.Lock()
	c.requestID++
	id := fmt.Sprintf("%d", c.requestID)
	c.waiters[id] = waiter
	c.mu.Unlock()

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Send request
	if err := c.sendRequest(request); err != nil {
		c.abandonWaiter(id)
		return nil, fmt.Errorf("failed to send MCP request: %w", err)
	}

	// Wait for response
	response, err := c.waitForResponse(ctx, id, waiter)
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

// waitForResponse waits for the reading goroutine to hand over the reply to the
// given request. The wait ends on the reply, on the caller's context, on the
// deadline, or when the server's output ends — never on another call's traffic.
func (c *Client) waitForResponse(ctx context.Context, id string, waiter <-chan *MCPResponse) (*MCPResponse, error) {
	// One clock, so there is never a question of which bound ended the wait: the
	// caller's deadline if it brought one, ours otherwise.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultResponseTimeout)
		defer cancel()
	}

	select {
	case response := <-waiter:
		return response, nil

	case <-ctx.Done():
		c.abandonWaiter(id)
		return nil, fmt.Errorf("gave up waiting for response to request %s: %w", id, ctx.Err())

	case <-c.readerDone:
		// The reply may have been delivered in the instant before the output
		// ended; prefer it over the end-of-output error.
		select {
		case response := <-waiter:
			return response, nil
		default:
		}
		c.abandonWaiter(id)
		return nil, fmt.Errorf("MCP server output ended while waiting for response to request %s: %w", id, c.readError())
	}
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

	// Parse the result.
	//
	// isError is how MCP reports a tool that ran and failed, as distinct from
	// the JSON-RPC error the call above already handles: the protocol carries a
	// successful response whose result says the tool itself did not succeed, and
	// the reason is in the content blocks. Reading it is what separates "the
	// tool failed" from "the tool returned this text".
	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
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

	// A failed tool reported as ordinary output is a failure the model reads as
	// a result and builds on. Carry the server's own text into the error rather
	// than replacing it: it is the only description of what went wrong.
	if toolResult.IsError {
		if output == "" {
			return "", fmt.Errorf("MCP tool %q failed, and the server sent no text explaining why", name)
		}
		return "", fmt.Errorf("MCP tool %q failed: %s", name, output)
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

	// Closing the output ends the reading goroutine, which releases every call
	// still waiting for a reply. Wait for it so no goroutine outlives Close.
	if c.readerDone != nil {
		<-c.readerDone
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