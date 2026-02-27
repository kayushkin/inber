package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// SpawnAgentTool creates a tool that spawns sub-agents for task delegation.
// This is how orchestrator agents (like Bran) delegate work to specialists.
func (r *Registry) SpawnAgentTool() agent.Tool {
	type input struct {
		Agent   string `json:"agent"`
		Task    string `json:"task"`
		Timeout int    `json:"timeout"` // seconds, 0 = no timeout
	}

	return agent.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a sub-agent to complete a task. Use this to delegate work to specialists: fionn (coding), scathach (testing), oisin (deployment), etc. Returns the agent's response.",
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

			// Create context with timeout
			taskCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Spawn and run the agent
			result, err := r.SpawnAndRun(taskCtx, in.Agent, in.Task)
			if err != nil {
				return fmt.Sprintf("❌ Agent %s failed: %s", in.Agent, err), nil
			}

			// Format result with metadata
			response := fmt.Sprintf("✅ Agent: %s\n\n%s\n\n[Tokens: in=%d out=%d | Tools: %d]",
				in.Agent, result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)

			return response, nil
		},
	}
}

// SpawnAndRun creates a new agent instance, runs the task, and returns the result.
// This is isolated from the caller's session—each spawn gets its own context.
func (r *Registry) SpawnAndRun(ctx context.Context, agentName string, task string) (*agent.TurnResult, error) {
	// Get agent config
	cfg, err := r.GetConfig(agentName)
	if err != nil {
		return nil, fmt.Errorf("agent config: %w", err)
	}

	// Create fresh agent instance
	a, err := r.createAgent(cfg)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	// Get or create context for this agent
	contextStore, err := r.GetContext(agentName)
	if err != nil {
		return nil, fmt.Errorf("get context: %w", err)
	}

	// Build system prompt from context
	// For sub-agents, we use a simpler context (just identity + project context)
	systemBlocks := r.buildSubAgentSystem(contextStore, cfg)
	a = agent.NewWithSystemBlocks(r.client, systemBlocks)

	// Re-add tools (they don't carry over from createAgent's returned instance)
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

	// Create message history with the task
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}

	// Run the agent
	model := cfg.Model
	if model == "" {
		model = agent.DefaultModel
	}

	result, err := a.Run(ctx, model, &messages)
	if err != nil {
		return nil, fmt.Errorf("agent run failed: %w", err)
	}

	return result, nil
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
