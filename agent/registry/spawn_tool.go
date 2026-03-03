package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kayushkin/inber/agent"
)

// SpawnAgentTool creates a tool that spawns sub-agents for task delegation.
// This is how orchestrator agents (like Bran) delegate work to specialists.
// By default, spawning is ASYNC (fire-and-forget). Use wait:true for synchronous behavior.
func (r *Registry) SpawnAgentTool() agent.Tool {
	type input struct {
		Agent   string `json:"agent"`
		Task    string `json:"task"`
		Timeout int    `json:"timeout"` // seconds, 0 = default (300s)
		Wait    bool   `json:"wait"`    // if true, block until completion
	}

	return agent.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a sub-agent to complete a task. BY DEFAULT this is ASYNC (returns immediately with task ID). Use wait:true to block until completion. Use this to delegate work to specialists: fionn (coding), scathach (testing), oisin (deployment), etc.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{
					"type":        "string",
					"description": "Agent name to spawn (fionn, scathach, oisin, etc)",
				},
				"task": map[string]any{
					"type":        "string",
					"description": "Task description for the agent to complete",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (optional, default 300)",
				},
				"wait": map[string]any{
					"type":        "boolean",
					"description": "If true, block until agent completes. If false (default), return immediately with task ID.",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			if in.Agent == "" {
				return "", fmt.Errorf("agent name required")
			}
			if in.Task == "" {
				return "", fmt.Errorf("task description required")
			}

			// Default timeout: 5 minutes
			timeout := time.Duration(in.Timeout) * time.Second
			if in.Timeout == 0 {
				timeout = 5 * time.Minute
			}

			// Async mode (default): spawn and return task ID immediately
			if !in.Wait {
				taskID, err := r.spawnManager.SpawnAsync(ctx, r, in.Agent, in.Task, timeout)
				if err != nil {
					return "", fmt.Errorf("spawn failed: %w", err)
				}

				return fmt.Sprintf("🚀 Spawned %s (task %s)\n\nTask: %s\n\nStatus: Running in background\n\nUse check_spawns to see results when ready.",
					in.Agent, taskID, truncate(in.Task, 100)), nil
			}

			// Sync mode (wait:true): block until completion
			taskID, err := r.spawnManager.SpawnAsync(ctx, r, in.Agent, in.Task, timeout)
			if err != nil {
				return "", fmt.Errorf("spawn failed: %w", err)
			}

			// Wait for completion
			result, err := r.spawnManager.WaitForCompletion(taskID, 500*time.Millisecond, timeout)
			if err != nil {
				return "", fmt.Errorf("wait failed: %w", err)
			}

			if result.Status == "failed" {
				return fmt.Sprintf("❌ Agent %s failed: %s", in.Agent, result.Error), nil
			}

			// Format result with metadata
			response := fmt.Sprintf("✅ Agent: %s (task %s)\n\n%s\n\n[Tokens: in=%d out=%d | Tools: %d]",
				in.Agent, taskID, result.Result, result.InputTokens, result.OutputTokens, result.ToolCalls)

			return response, nil
		},
	}
}

// CheckSpawnsTool creates a tool to check status of spawned agents
func (r *Registry) CheckSpawnsTool() agent.Tool {
	type input struct {
		TaskID string `json:"task_id"` // optional: check specific task
		All    bool   `json:"all"`     // show all tasks, including completed
	}

	return agent.Tool{
		Name:        "check_spawns",
		Description: "Check status of spawned sub-agents. Returns task results when ready.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "Specific task ID to check (optional)",
				},
				"all": map[string]any{
					"type":        "boolean",
					"description": "Show all tasks including completed ones (default: running only)",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			// Check specific task
			if in.TaskID != "" {
				spawn, err := r.spawnManager.GetStatus(in.TaskID)
				if err != nil {
					return "", err
				}
				return formatSpawn(spawn, true), nil
			}

			// List all spawns
			spawns := r.spawnManager.ListSpawns()
			if len(spawns) == 0 {
				return "No spawned agents.", nil
			}

			var parts []string
			for _, spawn := range spawns {
				if !in.All && spawn.Status != "running" {
					continue
				}
				parts = append(parts, formatSpawn(spawn, false))
			}

			if len(parts) == 0 {
				return "No running spawns. Use all:true to see completed tasks.", nil
			}

			return strings.Join(parts, "\n\n---\n\n"), nil
		},
	}
}

// formatSpawn formats a SpawnedAgent for display
func formatSpawn(spawn *SpawnedAgent, detailed bool) string {
	icon := "⏳"
	switch spawn.Status {
	case "completed":
		icon = "✅"
	case "failed":
		icon = "❌"
	}

	elapsed := time.Since(spawn.StartedAt)
	if spawn.Status != "running" {
		elapsed = spawn.CompletedAt.Sub(spawn.StartedAt)
	}

	header := fmt.Sprintf("%s Task %s — %s — %s (%.1fs)",
		icon, spawn.ID, spawn.Agent, spawn.Status, elapsed.Seconds())

	if !detailed {
		return header + "\n" + truncate(spawn.Task, 80)
	}

	// Detailed view
	var parts []string
	parts = append(parts, header)
	parts = append(parts, fmt.Sprintf("Task: %s", spawn.Task))

	if spawn.Status == "completed" {
		parts = append(parts, fmt.Sprintf("\nResult:\n%s", spawn.Result))
		parts = append(parts, fmt.Sprintf("\n[Tokens: in=%d out=%d | Tools: %d]",
			spawn.InputTokens, spawn.OutputTokens, spawn.ToolCalls))
	} else if spawn.Status == "failed" {
		parts = append(parts, fmt.Sprintf("\nError: %s", spawn.Error))
	}

	return strings.Join(parts, "\n")
}

// truncate shortens a string with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SpawnAndRun creates a new agent instance, runs the task, and returns the result.
// This is isolated from the caller's session—each spawn gets its own context.
func (r *Registry) SpawnAndRun(ctx context.Context, agentName string, task string) (*agent.TurnResult, error) {
	// Check if this agent should route to OpenClaw
	r.mu.RLock()
	openclawURL := r.openclawURL
	openclawToken := r.openclawToken
	isOpenClawAgent := false
	for _, name := range r.openclawAgents {
		if name == agentName {
			isOpenClawAgent = true
			break
		}
	}
	r.mu.RUnlock()

	// Route to OpenClaw if configured
	if isOpenClawAgent && openclawURL != "" && openclawToken != "" {
		return r.spawnViaOpenClaw(ctx, agentName, task, openclawURL, openclawToken)
	}

	// Get agent config
	cfg, err := r.GetConfig(agentName)
	if err != nil {
		return nil, fmt.Errorf("agent config: %w", err)
	}

	// Get or create context for this agent
	contextStore, err := r.GetContext(agentName)
	if err != nil {
		return nil, fmt.Errorf("get context: %w", err)
	}

	// Build system prompt from context
	// For sub-agents, we use a simpler context (just identity + project context)
	systemBlocks := r.buildSubAgentSystem(contextStore, cfg)

	// Determine model to use
	model := cfg.Model
	if model == "" {
		model = agent.DefaultModel
	}

	// Create message history with the task
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}

	// Get the model client for this agent
	// Always create a new client based on the spawned agent's model
	// (not the orchestrator's model) to handle multi-provider scenarios
	r.mu.RLock()
	modelStore := r.modelStore
	r.mu.RUnlock()

	var spawnModelClient *agent.ModelClient
	if modelStore != nil {
		// Create a new client based on the agent's model
		var err error
		spawnModelClient, err = agent.NewModelClient(model, modelStore)
		if err != nil {
			return nil, fmt.Errorf("create model client for %s: %w", model, err)
		}
	} else {
		// Fallback: no model store configured
		return nil, fmt.Errorf("no model store configured for spawning agents")
	}

	// Branch based on the spawned agent's model provider (not the orchestrator's)
	if spawnModelClient.IsOpenAI() {
		// Use OpenAI-compatible path
		return r.spawnAndRunOpenAIWithClient(ctx, cfg, systemBlocks, spawnModelClient, &messages)
	}

	// Use Anthropic path
	anthropicClient, _ := spawnModelClient.GetAnthropicClient()
	a := agent.NewWithSystemBlocks(anthropicClient, systemBlocks)

	// Add tools
	for _, toolName := range cfg.Tools {
		tool, err := r.tools.Get(toolName)
		if err != nil {
			// Skip unavailable tools (like spawn_agent for non-orchestrators)
			continue
		}
		a.AddTool(tool)
	}

	if cfg.Thinking > 0 {
		a.SetThinking(cfg.Thinking)
	}

	result, err := a.Run(ctx, model, &messages)
	if err != nil {
		return nil, fmt.Errorf("agent run failed: %w", err)
	}

	return result, nil
}

// spawnAndRunOpenAI runs a spawned agent using an OpenAI-compatible API.
// DEPRECATED: Use spawnAndRunOpenAIWithClient instead.
func (r *Registry) spawnAndRunOpenAI(ctx context.Context, cfg *AgentConfig, systemBlocks []anthropic.TextBlockParam, model string, messages *[]anthropic.MessageParam) (*agent.TurnResult, error) {
	result := &agent.TurnResult{}

	client, err := r.modelClient.GetOpenAIClient()
	if err != nil {
		return nil, err
	}

	return r.runOpenAIHelper(ctx, cfg, systemBlocks, client, messages, result)
}

// spawnAndRunOpenAIWithClient runs a spawned agent using the provided ModelClient.
// This is used when each spawned agent may have a different model provider.
func (r *Registry) spawnAndRunOpenAIWithClient(ctx context.Context, cfg *AgentConfig, systemBlocks []anthropic.TextBlockParam, modelClient *agent.ModelClient, messages *[]anthropic.MessageParam) (*agent.TurnResult, error) {
	result := &agent.TurnResult{}

	client, err := modelClient.GetOpenAIClient()
	if err != nil {
		return nil, fmt.Errorf("get openai client: %w", err)
	}

	return r.runOpenAIHelper(ctx, cfg, systemBlocks, client, messages, result)
}

// runOpenAIHelper contains the shared OpenAI agent loop logic.
func (r *Registry) runOpenAIHelper(ctx context.Context, cfg *AgentConfig, systemBlocks []anthropic.TextBlockParam, client *agent.OpenAIClient, messages *[]anthropic.MessageParam, result *agent.TurnResult) (*agent.TurnResult, error) {

	// Build tool map and convert tools to OpenAI format
	var agentTools []agent.Tool
	toolMap := make(map[string]agent.Tool)
	
	for _, toolName := range cfg.Tools {
		tool, err := r.tools.Get(toolName)
		if err != nil {
			continue
		}
		agentTools = append(agentTools, tool)
		toolMap[tool.Name] = tool
	}

	openAITools := agent.ConvertAnthropicToolsToOpenAI(agentTools)

	// Build system message from blocks
	var systemParts []string
	for _, block := range systemBlocks {
		systemParts = append(systemParts, block.Text)
	}
	systemMessage := strings.Join(systemParts, "\n\n")

	// Tool call loop
	for {
		// Convert messages to OpenAI format
		oaiMessages := agent.ConvertAnthropicMessagesToOpenAI(*messages)

		// Prepend system message
		if systemMessage != "" {
			oaiMessages = append([]agent.OpenAIMessage{
				{Role: "system", Content: systemMessage},
			}, oaiMessages...)
		}

		// Build request
		req := agent.OpenAIRequest{
			Model:     client.Model,
			Messages:  oaiMessages,
			MaxTokens: 16384,
		}

		if len(openAITools) > 0 {
			req.Tools = openAITools
		}

		// Make API call
		resp, err := client.ChatCompletion(ctx, req)
		if err != nil {
			return result, fmt.Errorf("OpenAI API call failed: %w", err)
		}

		// Convert response to Anthropic format for compatibility
		anthropicResp := agent.ConvertOpenAIResponseToAnthropic(resp)

		result.InputTokens += int(anthropicResp.Usage.InputTokens)
		result.OutputTokens += int(anthropicResp.Usage.OutputTokens)

		// Append assistant message
		*messages = append(*messages, anthropicResp.ToParam())

		// Check stop reason
		if anthropicResp.StopReason == anthropic.StopReasonEndTurn ||
			anthropicResp.StopReason == anthropic.StopReasonMaxTokens {
			// Extract text and return
			for _, block := range anthropicResp.Content {
				if block.Type == "text" {
					result.Text += block.Text
				}
			}
			return result, nil
		}

		// Handle tool calls
		if anthropicResp.StopReason == anthropic.StopReasonToolUse {
			var toolResults []anthropic.ContentBlockParamUnion

			for _, block := range anthropicResp.Content {
				if block.Type != "tool_use" {
					continue
				}

				result.ToolCalls++

				// Execute tool
				tool, ok := toolMap[block.Name]
				if !ok {
					errMsg := fmt.Sprintf("error: unknown tool %q", block.Name)
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, errMsg, true,
					))
					continue
				}

				output, err := tool.Run(ctx, string(block.Input))
				if err != nil {
					errMsg := fmt.Sprintf("error: %s", err)
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, errMsg, true,
					))
					continue
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(
					block.ID, output, false,
				))
			}

			*messages = append(*messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// Unexpected stop reason
		return result, fmt.Errorf("unexpected stop reason: %s", anthropicResp.StopReason)
	}
}

// buildSubAgentSystem creates a minimal system prompt for spawned agents.
// We don't want to duplicate all the orchestrator's context.
func (r *Registry) buildSubAgentSystem(store any, cfg *AgentConfig) []anthropic.TextBlockParam {
	// For now, just use the agent's identity
	// In the future, we could filter context chunks by tags
	return []anthropic.TextBlockParam{
		{Text: cfg.System},
	}
}

// spawnViaOpenClaw delegates a task to an OpenClaw agent via gateway WebSocket.
// This allows inber to use OpenClaw agents (kayushkin, downloadstack, etc.) as specialists.
func (r *Registry) spawnViaOpenClaw(ctx context.Context, agentName, task, url, token string) (*agent.TurnResult, error) {
	// Import the openclaw subagent from cmd/inber
	// We can't import it directly due to circular dependency, so we'll use a local implementation
	// For now, create the subagent inline
	timeout := 5 * time.Minute
	
	subagent := &openClawSubagentImpl{
		url:     url,
		token:   token,
		agentID: agentName,
		timeout: timeout,
	}
	
	result, err := subagent.Run(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("openclaw spawn failed: %w", err)
	}
	
	return result, nil
}

// openClawSubagentImpl is a local implementation to avoid circular import
type openClawSubagentImpl struct {
	url     string
	token   string
	agentID string
	timeout time.Duration
}

func (o *openClawSubagentImpl) Run(ctx context.Context, task string) (*agent.TurnResult, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	// Connect to gateway
	conn, err := o.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}
	defer conn.Close()

	// Send agent request
	requestID := uuid.New().String()
	req := map[string]interface{}{
		"type":   "req",
		"id":     requestID,
		"method": "agent",
		"params": map[string]interface{}{
			"message":        task,
			"agentId":        o.agentID,
			"channel":        "webchat",
			"idempotencyKey": uuid.New().String(),
		},
	}

	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	// Buffer streaming response
	var responseText strings.Builder
	var inputTokens, outputTokens int

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for response")
		default:
		}

		var msg gwMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("read response failed: %w", err)
		}

		// Handle agent events
		if msg.Type == "event" && msg.Event == "agent" {
			if msg.Payload == nil {
				continue
			}

			var payload agentPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}

			switch payload.Stream {
			case "assistant":
				if payload.Data != nil && payload.Data.Delta != "" {
					responseText.WriteString(payload.Data.Delta)
				}

			case "lifecycle":
				if payload.Data != nil {
					phase := payload.Data.Phase
					if phase == "end" {
						// Response complete
						result := &agent.TurnResult{
							Text:         strings.TrimSpace(responseText.String()),
							InputTokens:  inputTokens,
							OutputTokens: outputTokens,
						}
						return result, nil
					}
					if phase == "error" {
						errMsg := "agent error"
						if payload.Data.Error != "" {
							errMsg = payload.Data.Error
						}
						return nil, fmt.Errorf("agent error: %s", errMsg)
					}
				}

			case "usage":
				// Track token usage if provided
				if payload.Data != nil {
					if payload.Data.InputTokens > 0 {
						inputTokens = payload.Data.InputTokens
					}
					if payload.Data.OutputTokens > 0 {
						outputTokens = payload.Data.OutputTokens
					}
				}
			}
		}
	}
}

func (o *openClawSubagentImpl) connect(ctx context.Context) (*websocket.Conn, error) {
	// Derive Origin from WebSocket URL
	origin := strings.Replace(o.url, "ws://", "http://", 1)
	origin = strings.Replace(origin, "wss://", "https://", 1)
	origin = strings.TrimSuffix(origin, "/ws")
	origin = strings.TrimSuffix(origin, "/")

	header := http.Header{}
	header.Set("Origin", origin)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, o.url, header)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	// Wait for connect.challenge event
	var msg gwMessage
	if err := conn.ReadJSON(&msg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read challenge failed: %w", err)
	}

	if msg.Type != "event" || msg.Event != "connect.challenge" {
		conn.Close()
		return nil, fmt.Errorf("unexpected message, expected connect.challenge")
	}

	// Send connect request
	connectReq := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "connect",
		"params": map[string]interface{}{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]interface{}{
				"id":       "inber-orchestrator",
				"version":  "1.0.0",
				"platform": "go",
				"mode":     "webchat",
			},
			"role":   "operator",
			"scopes": []string{"operator.admin", "operator.read", "operator.write"},
			"caps":   []string{},
			"commands": []string{},
			"permissions": map[string]interface{}{},
			"auth": map[string]interface{}{
				"token": o.token,
			},
			"locale":    "en-US",
			"userAgent": "inber/1.0.0",
		},
	}

	if err := conn.WriteJSON(connectReq); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send connect failed: %w", err)
	}

	// Wait for hello-ok response
	if err := conn.ReadJSON(&msg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read hello-ok failed: %w", err)
	}

	if msg.Type != "res" || !msg.OK {
		errMsg := "connection rejected"
		if msg.Error != nil {
			var errData map[string]interface{}
			if err := json.Unmarshal(msg.Error, &errData); err == nil {
				if m, ok := errData["message"].(string); ok {
					errMsg = m
				}
			}
		}
		conn.Close()
		return nil, fmt.Errorf("connect failed: %s", errMsg)
	}

	// Verify hello-ok payload
	if msg.Payload != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			if payload["type"] != "hello-ok" {
				conn.Close()
				return nil, fmt.Errorf("unexpected payload type: %v", payload["type"])
			}
		}
	}

	return conn, nil
}

// gwMessage represents the base OpenClaw gateway message structure.
type gwMessage struct {
	Type    string          `json:"type"`
	Event   string          `json:"event,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// agentPayload represents agent event data.
type agentPayload struct {
	Stream string         `json:"stream"`
	Data   *agentDataMsg  `json:"data,omitempty"`
}

// agentDataMsg represents the data field in agent events.
type agentDataMsg struct {
	Delta        string `json:"delta,omitempty"`
	Phase        string `json:"phase,omitempty"`
	Error        string `json:"error,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
}
