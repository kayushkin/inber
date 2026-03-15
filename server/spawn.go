package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// SpawnRequest is the input for creating a sub-agent.
type SpawnRequest struct {
	ParentKey string        `json:"parent_key"`
	Agent     string        `json:"agent"`
	Task      string        `json:"task"`
	Model     string        `json:"model,omitempty"`   // override
	Fork      bool          `json:"fork,omitempty"`    // inherit parent messages
	Timeout   time.Duration `json:"timeout,omitempty"` // max runtime (0 = use default)
}

const defaultSpawnTimeout = 5 * time.Minute

// SpawnResponse is returned immediately when a sub-agent is accepted.
type SpawnResponse struct {
	Status   string `json:"status"`
	ChildKey string `json:"child_key"`
}

// SpawnResult is delivered to the parent when a child completes.
type SpawnResult struct {
	ChildKey string        `json:"child_key"`
	Agent    string        `json:"agent"`
	Task     string        `json:"task"`
	Status   string        `json:"status"` // "success", "error", "timeout"
	Summary  string        `json:"summary"`
	Tokens   TokenUsage    `json:"tokens"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
	WorkspaceID string        `json:"workspace_id,omitempty"`
	Branch      string        `json:"branch,omitempty"`
	Commits     map[string]string `json:"commits,omitempty"` // repo → hash
}

// Spawn creates a child session and enqueues its work.
// Returns immediately. Result delivered to parent async.
func (g *Server) Spawn(ctx context.Context, req SpawnRequest) (*SpawnResponse, error) {
	// Validate parent.
	val, ok := g.sessions.Load(req.ParentKey)
	if !ok {
		return nil, fmt.Errorf("parent session not found: %s", req.ParentKey)
	}
	parent := val.(*Session)

	// Check depth limit.
	if parent.SpawnDepth >= g.config.MaxSpawnDepth {
		return nil, fmt.Errorf("max spawn depth reached (%d)", g.config.MaxSpawnDepth)
	}

	// Check children limit.
	parent.mu.Lock()
	childCount := len(parent.Children)
	parent.mu.Unlock()
	if childCount >= g.config.MaxChildrenPerAgent {
		return nil, fmt.Errorf("max children reached (%d)", g.config.MaxChildrenPerAgent)
	}

	// Resolve agent config.
	ac, ok := g.GetAgentConfig(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", req.Agent)
	}
	if req.Model != "" {
		ac.Model = req.Model
	}

	// Generate child session key.
	childKey := sessionKeyForChild(req.ParentKey)

	// Create ephemeral workspace if agent has projects configured.
	var ws *forge.Workspace
	if len(ac.Projects) > 0 && g.forgeDB != nil {
		w, err := g.forgeDB.CreateWorkspace(req.Agent, ac.Projects)
		if err != nil {
			log.Printf("[server] workspace creation failed for %s: %v (using source repo)", req.Agent, err)
		} else {
			ws = w
			// Set agent's working directory to the primary repo worktree.
			ac.Workspace = w.Repos[w.Primary]
			log.Printf("[server] workspace %s created → %s", w.ID, w.BaseDir)
		}
	}

	// Create child session.
	var child *Session
	var err error

	if req.Fork {
		child, err = g.forkSession(parent, childKey, req.Agent, ac, nil)
	} else {
		child, err = g.createSession(childKey, req.Agent, ac, nil)
		if err == nil {
			child.SpawnDepth = parent.SpawnDepth + 1
			child.ParentKey = req.ParentKey
		}
	}
	if err != nil {
		if ws != nil {
			g.forgeDB.Cleanup(ws)
		}
		return nil, fmt.Errorf("create child session: %w", err)
	}

	g.sessions.Store(childKey, child)

	// Register child with parent.
	parent.mu.Lock()
	parent.Children = append(parent.Children, childKey)
	parent.mu.Unlock()

	log.Printf("[server] spawn %s → %s/%s: %s",
		req.ParentKey, req.Agent, childKey, truncate(req.Task, 80))

	// Apply timeout.
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultSpawnTimeout
	}

	// Ensure spawn session exists in DB.
	g.store.UpsertSession(childKey, req.Agent, "spawn")

	// Look up the parent's active request ID for linking.
	var parentReqID *int
	if pr, _ := g.store.ActiveRequest(req.ParentKey); pr != nil {
		parentReqID = &pr.ID
	}

	// Enqueue the work asynchronously.
	go func() {
		start := time.Now()

		// Track request in DB.
		reqID, _ := g.store.CreateRequest(childKey, truncate(req.Task, 500), parentReqID)

		// Notify parent that child has started.
		g.deliverProgress(req.ParentKey, childKey, req.Agent,
			fmt.Sprintf("⏳ Sub-agent %s started working on: %s", req.Agent, truncate(req.Task, 100)))

		// Publish to bus for dashboard.
		g.events.SpawnStarted(childKey, req.Agent, req.ParentKey, truncate(req.Task, 200))

		err := g.queue.Enqueue(ctx, "subagent", childKey, func(ctx context.Context) error {
			// Wrap with timeout.
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			result, err := child.turn(ctx, req.Task)

			status := "success"
			summary := ""
			var tokens TokenUsage
			errMsg := ""

			if ctx.Err() == context.DeadlineExceeded {
				status = "timeout"
				errMsg = fmt.Sprintf("timed out after %s", timeout)
			} else if err != nil {
				status = "error"
				errMsg = err.Error()
			}

			if result != nil {
				summary = result.Text
				tokens = TokenUsage{
					Input:      result.InputTokens,
					Output:     result.OutputTokens,
					CacheRead:  result.CacheReadTokens,
					CacheWrite: result.CacheCreationTokens,
				}
			}

			// Complete request in DB.
			cost := sessionMod.CalcCostWithCache("", tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite)
			turns := 0
			if result != nil {
				turns = result.ToolCalls
			}
			g.store.CompleteRequest(reqID, status, truncate(summary, 1000), errMsg,
				turns, tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite, cost)
			g.store.TouchSession(childKey, len(child.Engine.Messages))

			// Save spawn transcript to the agent's memory for continuity.
			g.saveSpawnToMemory(child, req.Agent, req.Task, status, summary)

			// Inject short update into the agent's main session.
			g.updateMainSession(req.Agent, req.Task, status, summary)

			// Commit workspace changes (stay on spawn branch — no merge).
			var workspaceID, branch string
			var commits map[string]string
			if ws != nil {
				commitMsg := fmt.Sprintf("%s: %s", req.Agent, truncate(req.Task, 100))
				results, cerr := g.forgeDB.CommitAll(ws, commitMsg)
				if cerr != nil {
					log.Printf("[server] workspace commit error: %v", cerr)
				} else {
					commits = make(map[string]string)
					for repo, cr := range results {
						if cr.Dirty && cr.Hash != "" {
							commits[repo] = cr.Hash
							log.Printf("[server] committed %s: %s", repo, cr.Hash)
						} else if cr.Error != "" {
							log.Printf("[server] commit failed for %s: %s", repo, cr.Error)
						}
					}
				}
				workspaceID = ws.ID
				branch = ws.Branch
			}

			// Deliver full result to the parent orchestrator + publish to bus.
			spawnResult := SpawnResult{
				ChildKey:    childKey,
				Agent:       req.Agent,
				Task:        req.Task,
				Status:      status,
				Summary:     summary,
				Tokens:      tokens,
				Duration:    time.Since(start),
				Error:       errMsg,
				WorkspaceID: workspaceID,
				Branch:      branch,
				Commits:     commits,
			}
			g.events.SpawnCompleted(spawnResult)
			g.deliverResult(req.ParentKey, spawnResult)

			// Persist child's messages.
			g.persistMessages(child)

			return err
		})
		if err != nil {
			log.Printf("[server] spawn %s failed: %v", childKey, err)
			if ws != nil {
				g.forgeDB.Cleanup(ws)
			}
			g.store.CompleteRequest(reqID, "error", "", fmt.Sprintf("enqueue failed: %v", err), 0, 0, 0, 0, 0, 0)
			// If enqueue itself failed (not the work), still notify parent.
			g.deliverResult(req.ParentKey, SpawnResult{
				ChildKey: childKey,
				Agent:    req.Agent,
				Task:     req.Task,
				Status:   "error",
				Error:    fmt.Sprintf("enqueue failed: %v", err),
				Duration: time.Since(start),
			})
		}
	}()

	return &SpawnResponse{
		Status:   "accepted",
		ChildKey: childKey,
	}, nil
}

// ForkAndSpawn forks the parent session N times, one per task.
// All children start with the same conversation history.
func (g *Server) ForkAndSpawn(ctx context.Context, parentKey string, tasks []SpawnRequest) ([]*SpawnResponse, error) {
	var responses []*SpawnResponse

	for _, task := range tasks {
		task.ParentKey = parentKey
		task.Fork = true

		resp, err := g.Spawn(ctx, task)
		if err != nil {
			log.Printf("[server] fork-spawn failed for %s: %v", task.Agent, err)
			continue
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// deliverProgress sends a lightweight status update to the parent session.
func (g *Server) deliverProgress(parentKey, childKey, agentName, message string) {
	val, ok := g.sessions.Load(parentKey)
	if !ok {
		return
	}
	parent := val.(*Session)

	parent.mu.Lock()
	isRunning := parent.Status == Running
	parent.mu.Unlock()

	if isRunning {
		// Parent is mid-turn — inject directly.
		parent.inject(message)
	} else {
		// Parent is idle — queue for next turn.
		parent.queuePending(message)
	}
}

// deliverResult injects the child's result into the parent session.
func (g *Server) deliverResult(parentKey string, result SpawnResult) {
	val, ok := g.sessions.Load(parentKey)
	if !ok {
		log.Printf("[server] parent %s gone, dropping result from %s", parentKey, result.ChildKey)
		return
	}
	parent := val.(*Session)

	cost := sessionMod.CalcCostWithCache("", result.Tokens.Input, result.Tokens.Output,
		result.Tokens.CacheRead, result.Tokens.CacheWrite)

	msg := fmt.Sprintf("[Sub-agent completed]\n"+
		"Agent: %s (%s)\n"+
		"Task: %s\n"+
		"Status: %s\n"+
		"Duration: %s\n"+
		"Tokens: %d in / %d out ($%.3f)\n"+
		"\nResult:\n%s",
		result.Agent, result.ChildKey,
		result.Task,
		result.Status,
		result.Duration.Round(time.Second),
		result.Tokens.Input, result.Tokens.Output, cost,
		result.Summary,
	)

	if result.WorkspaceID != "" {
		msg += fmt.Sprintf("\n\nWorkspace: %s (branch: %s)", result.WorkspaceID, result.Branch)
		if len(result.Commits) > 0 {
			for repo, hash := range result.Commits {
				msg += fmt.Sprintf("\n  %s: %s", repo, hash)
			}
		}
		msg += "\n\nActions: merge(workspace_id) | reject(workspace_id) | fix(workspace_id, instructions)"
	}

	if result.Error != "" {
		msg += fmt.Sprintf("\n\nError: %s", result.Error)
	}

	log.Printf("[server] result %s → %s: %s (%s, %s)",
		result.ChildKey, parentKey, result.Status,
		result.Duration.Round(time.Second), truncate(result.Summary, 60))

	parent.mu.Lock()
	isRunning := parent.Status == Running
	parent.mu.Unlock()

	if isRunning {
		parent.inject(msg)
	} else {
		// Parent finished its turn. Queue for delivery on next turn.
		parent.queuePending(msg)
		log.Printf("[server] result queued (parent %s idle), will deliver on next turn", parentKey)

		// Publish the result directly to the bus outbound topic
		// so the dashboard shows it immediately.
		if g.events != nil {
			g.events.PublishOutbound(parent.AgentName, result)
		}
	}
}

// saveSpawnToMemory persists a summary of the spawn's work into the agent's memory DB.
// This gives the agent's main session access to the full context via memory_search.
func (g *Server) saveSpawnToMemory(child *Session, agentName, task, status, summary string) {
	if child.Engine == nil || child.Engine.MemStore == nil {
		return
	}

	// Build a detailed memory entry from the spawn.
	content := fmt.Sprintf("Spawn task: %s\nStatus: %s\n\n%s", task, status, summary)

	// Extract key tool calls/decisions from the transcript for richer context.
	if transcript := formatTranscriptHighlights(child.Engine.Messages); transcript != "" {
		content += "\n\nKey actions:\n" + transcript
	}

	err := child.Engine.MemStore.Save(memory.Memory{
		Content:    content,
		Tags:       []string{"spawn", "task-result", agentName},
		Importance: 0.7,
		Source:     "system",
	})
	if err != nil {
		log.Printf("[server] failed to save spawn memory for %s: %v", agentName, err)
	} else {
		log.Printf("[server] saved spawn result to %s memory", agentName)
	}
}

// updateMainSession injects a short context update into the agent's main session
// so it knows what happened in the spawn without loading the full transcript.
func (g *Server) updateMainSession(agentName, task, status, summary string) {
	mainKey := fmt.Sprintf("agent:%s:main", agentName)
	val, ok := g.sessions.Load(mainKey)
	if !ok {
		return
	}
	main := val.(*Session)

	// Keep it brief — full context is in memory_search.
	summaryTrunc := summary
	if len(summaryTrunc) > 500 {
		summaryTrunc = summaryTrunc[:497] + "..."
	}

	msg := fmt.Sprintf("[Context update] Completed spawned task.\n"+
		"Task: %s\nStatus: %s\nSummary: %s\n"+
		"Full details available via memory_search.",
		truncate(task, 200), status, summaryTrunc)

	main.queuePending(msg)
}

// formatTranscriptHighlights extracts key tool calls from a message history.
func formatTranscriptHighlights(msgs []anthropic.MessageParam) string {
	var highlights []string
	for _, msg := range msgs {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				highlights = append(highlights, fmt.Sprintf("- %s", block.OfToolUse.Name))
			}
		}
	}
	if len(highlights) > 10 {
		highlights = highlights[:10]
		highlights = append(highlights, fmt.Sprintf("- ... and %d more", len(highlights)-10))
	}
	return strings.Join(highlights, "\n")
}

// SessionsListTool creates a tool for checking sub-agent status.
func (g *Server) SessionsListTool(parentSessionKey string) agent.Tool {
	return agent.Tool{
		Name:        "sessions_list",
		Description: "List active sessions and their status. Shows your spawned sub-agents and whether they're running, idle, completed, or errored.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			sessions := g.ListSessions()
			if len(sessions) == 0 {
				return "No active sessions.", nil
			}

			var sb strings.Builder
			for _, s := range sessions {
				marker := "  "
				if s.Key == parentSessionKey {
					marker = "→ "
				}
				sb.WriteString(fmt.Sprintf("%s%s (%s) [%s] %d msgs, active %s ago",
					marker, s.Key, s.Agent, s.Status,
					s.Messages, time.Since(s.LastActive).Round(time.Second)))
				if s.ParentKey != "" {
					sb.WriteString(fmt.Sprintf(" parent=%s", s.ParentKey))
				}
				if len(s.Children) > 0 {
					sb.WriteString(fmt.Sprintf(" children=%v", s.Children))
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	}
}

// SteerAgentTool creates a tool for sending messages to running sub-agents.
func (g *Server) SteerAgentTool() agent.Tool {
	return agent.Tool{
		Name: "steer_agent",
		Description: "Send a message to a running sub-agent session. " +
			"If the agent is mid-turn, the message is injected immediately (seen between tool calls). " +
			"If idle, it's queued for the next turn.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"session_key", "message"},
			Properties: map[string]any{
				"session_key": map[string]any{
					"type":        "string",
					"description": "Session key of the target agent (from spawn response or sessions_list)",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Message to send to the agent",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				SessionKey string `json:"session_key"`
				Message    string `json:"message"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			err := g.Inject(in.SessionKey, in.Message)
			if err != nil {
				return "", err
			}

			// Check if it was injected live or queued.
			val, ok := g.sessions.Load(in.SessionKey)
			if !ok {
				return "Message sent (session not found in memory, may have been queued to DB).", nil
			}
			sess := val.(*Session)
			sess.mu.Lock()
			status := sess.Status
			sess.mu.Unlock()

			if status == Running {
				return fmt.Sprintf("Message injected into %s (currently running). Agent will see it between tool calls.", in.SessionKey), nil
			}
			return fmt.Sprintf("Message queued for %s (currently %s). Will be delivered on next turn.", in.SessionKey, status), nil
		},
	}
}

// SpawnAgentTool creates the spawn_agent tool that calls server.Spawn directly.
// This replaces the old stderr-based INBER_SPAWN protocol.
func (g *Server) SpawnAgentTool(parentSessionKey string) agent.Tool {
	// Build available agents description.
	agentNames := make([]string, 0, len(g.config.Agents))
	for name := range g.config.Agents {
		agentNames = append(agentNames, name)
	}
	agentDesc := fmt.Sprintf("Agent name to spawn. Available: %s", fmt.Sprintf("%v", agentNames))

	return agent.Tool{
		Name: "spawn_agent",
		Description: "Spawn a sub-agent to work on a task. Returns immediately. " +
			"Results are delivered when the agent completes. " +
			"Use fork=true to give the child your current conversation context.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{
					"type":        "string",
					"description": agentDesc,
				},
				"task": map[string]any{
					"type":        "string",
					"description": "Task for the agent to complete",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model override (optional)",
				},
				"fork": map[string]any{
					"type":        "boolean",
					"description": "If true, child inherits this session's conversation history",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Max runtime in seconds (default 300)",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				Agent          string `json:"agent"`
				Task           string `json:"task"`
				Model          string `json:"model,omitempty"`
				Fork           bool   `json:"fork,omitempty"`
				TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}
			if in.Agent == "" {
				return "", fmt.Errorf("agent name required")
			}
			if in.Task == "" {
				return "", fmt.Errorf("task required")
			}

			var timeout time.Duration
			if in.TimeoutSeconds > 0 {
				timeout = time.Duration(in.TimeoutSeconds) * time.Second
			}

			resp, err := g.Spawn(ctx, SpawnRequest{
				ParentKey: parentSessionKey,
				Agent:     in.Agent,
				Task:      in.Task,
				Model:     in.Model,
				Fork:      in.Fork,
				Timeout:   timeout,
			})
			if err != nil {
				return "", err
			}

			taskPreview := in.Task
			if len(taskPreview) > 100 {
				taskPreview = taskPreview[:97] + "..."
			}

			return fmt.Sprintf("🚀 Spawned %s (%s)\nTask: %s\nFork: %v\n\nResult will be delivered when complete.",
				in.Agent, resp.ChildKey, taskPreview, in.Fork), nil
		},
	}
}
