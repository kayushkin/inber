package server

import (
	"context"
	"fmt"
	"time"

	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/logger"
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

	// Check if agent is already busy (informational).
	if g.agentStore != nil {
		if statuses, err := g.agentStore.GetStatus(req.Agent); err == nil {
			for _, s := range statuses {
				if s.Status == "working" {
					task := ""
					if s.Task != nil {
						task = *s.Task
					}
					logger.WithComponent("spawn").Info("spawning agent that is already working", map[string]interface{}{
						"agent":        req.Agent,
						"current_task": task,
					})
				}
			}
		}
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
			logger.WithComponent("spawn").Warn("workspace creation failed, using source repo", map[string]interface{}{
				"agent": req.Agent,
				"error": err,
			})
		} else {
			ws = w
			// Set agent's working directory to the primary repo worktree.
			ac.Workspace = w.Repos[w.Primary]
			logger.WithComponent("spawn").Info("workspace created", map[string]interface{}{
				"workspace_id": w.ID,
				"base_dir":     w.BaseDir,
			})
			g.mu.Lock()
			g.workspaces[w.ID] = w
			g.mu.Unlock()
		}
	}

	// Create child session.
	var child *Session
	var err error

	if req.Fork {
		child, err = g.forkSession(ctx, parent, childKey, req.Agent, ac, nil)
	} else {
		child, err = g.createSession(ctx, childKey, req.Agent, ac, RunRequest{}, nil)
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

	logger.WithComponent("spawn").Info("creating sub-agent", map[string]interface{}{
		"parent_key":  req.ParentKey,
		"agent":       req.Agent,
		"child_key":   childKey,
		"task":        truncate(req.Task, 80),
	})

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

	// Snapshot parent's onEvent so we can bubble subagent events to the parent's stream.
	parentOnEvent := parent.getOnEvent()

	// Wire child hooks to forward status updates as agent_update events to parent stream.
	if parentOnEvent != nil {
		child.setOnEvent(func(ev StreamEvent) {
			if ev.Kind == "status" || ev.Kind == "thinking" {
				parentOnEvent(StreamEvent{
					Kind: "agent_update",
					Text: ev.Text,
					Turn: ev.Turn,
					Data: map[string]any{
						"agent":       req.Agent,
						"session_key": childKey,
						"parent_key":  req.ParentKey,
						"depth":       child.SpawnDepth,
					},
				})
			}
		})
	}

	// Enqueue the work asynchronously.
	go func() {
		start := time.Now()

		// Track request in DB.
		reqID, _ := g.store.CreateRequest(childKey, truncate(req.Task, 500), parentReqID)

		// Notify parent that child has started — both via injection and live stream.
		g.deliverProgress(req.ParentKey, childKey, req.Agent,
			fmt.Sprintf("⏳ Sub-agent %s started working on: %s", req.Agent, truncate(req.Task, 100)))

		// Fire agent_spawned on the parent's live stream.
		if parentOnEvent != nil {
			parentOnEvent(StreamEvent{
				Kind: "agent_spawned",
				Text: truncate(req.Task, 200),
				Data: map[string]any{
					"agent":       req.Agent,
					"session_key": childKey,
					"parent_key":  req.ParentKey,
					"depth":       child.SpawnDepth,
				},
			})
		}

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
					logger.WithComponent("spawn").Error("workspace commit error", map[string]interface{}{
						"error": cerr,
					})
				} else {
					commits = make(map[string]string)
					for repo, cr := range results {
						if cr.Dirty && cr.Hash != "" {
							commits[repo] = cr.Hash
							logger.WithComponent("spawn").Debug("committed repository", map[string]interface{}{
								"repo": repo,
								"hash": cr.Hash,
							})
						} else if cr.Error != "" {
							logger.WithComponent("spawn").Error("commit failed for repository", map[string]interface{}{
								"repo":  repo,
								"error": cr.Error,
							})
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

			// Fire agent_done on the parent's live stream.
			if parentOnEvent != nil {
				parentOnEvent(StreamEvent{
					Kind: "agent_done",
					Text: truncate(summary, 300),
					Data: map[string]any{
						"agent":       req.Agent,
						"session_key": childKey,
						"parent_key":  req.ParentKey,
						"depth":       child.SpawnDepth,
						"status":      status,
						"duration":    time.Since(start).String(),
					},
				})
			}

			g.deliverResult(req.ParentKey, spawnResult)

			// Persist child's messages.
			g.persistSessionState(child)

			return err
		})
		if err != nil {
			logger.WithComponent("spawn").Error("spawn failed", map[string]interface{}{
				"child_key": childKey,
				"error":     err,
			})
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
			logger.WithComponent("spawn").Error("fork-spawn failed", map[string]interface{}{
				"agent": task.Agent,
				"error": err,
			})
			continue
		}
		responses = append(responses, resp)
	}

	return responses, nil
}