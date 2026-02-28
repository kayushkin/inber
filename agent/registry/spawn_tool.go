package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
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
