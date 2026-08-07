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

// The event kinds a spawn puts on its parent's stream. They are envelopes: each
// one already names the session it came from in its Data, so a forwarder that
// meets one is looking at a deeper descendant's event, not its own child's.
const (
	eventKindAgentUpdate  = "agent_update"
	eventKindAgentSpawned = "agent_spawned"
	eventKindAgentDone    = "agent_done"
)

// emitToParentStream puts one event on a parent session's stream, if that
// parent has a stream at this moment.
//
// The moment is the whole point. Session.onEvent belongs to the parent's
// *current* request — getOrCreateSession replaces it on every run, which is what
// "updated per-turn" on the field means — while a spawn deliberately outlives
// the turn that started it: the tool returns as soon as the work is queued, and
// withoutCallerCancellation drops the caller's cancellation so a browser tab
// closing cannot abort the child. Resolving the parent's writer once, at spawn
// time, therefore aims a sub-agent's entire event stream at a writer that has
// since been replaced — and on the HTTP path (api_run.go closes its sink over
// the http.ResponseWriter and its Flusher) at one whose handler has returned,
// which net/http says may not be used.
//
// A parent between turns has no writer and getOnEvent says so by returning nil.
// Dropping is then the only honest outcome: there is nobody to deliver to, and
// no queue that would hold the event until there is.
func emitToParentStream(parent *Session, ev StreamEvent) {
	if onEvent := parent.getOnEvent(); onEvent != nil {
		onEvent(ev)
	}
}

// newSubagentEventForwarder returns the event sink for a child session.
//
// A child's own progress (status, thinking) is wrapped in an agent_update
// envelope naming the child, and handed to the parent's stream. An envelope
// from a deeper descendant is passed through unchanged: it already carries its
// own origin, and re-wrapping it would replace that origin with this child's.
//
// Passing envelopes through is what makes the forwarder compose. A grandchild's
// sink is this same closure, so its events arrive here already labelled
// agent_update / agent_spawned / agent_done — and MaxSpawnDepth defaults to 2,
// so a grandchild is an ordinary case, not a corner one.
//
// resolveParentStream is called per event rather than once, for the reason
// emitToParentStream gives: the parent's writer is per-turn and the child
// outlives the turn. Pass the parent's getOnEvent method, not its result.
//
// The child's delta, tool_call and tool_result are not forwarded: a parent
// stream shows what its sub-agents are doing, not every token they emit.
// Whether that should change is the open half of noteboard todo c81c4b63.
func newSubagentEventForwarder(resolveParentStream func() func(StreamEvent), agentName, childKey, parentKey string, depth int) func(StreamEvent) {
	emit := func(ev StreamEvent) {
		if parentOnEvent := resolveParentStream(); parentOnEvent != nil {
			parentOnEvent(ev)
		}
	}
	return func(ev StreamEvent) {
		switch ev.Kind {
		case "status", "thinking":
			emit(StreamEvent{
				Kind: eventKindAgentUpdate,
				Text: ev.Text,
				Turn: ev.Turn,
				Data: map[string]any{
					"agent":       agentName,
					"session_key": childKey,
					"parent_key":  parentKey,
					"depth":       depth,
				},
			})
		case eventKindAgentUpdate, eventKindAgentSpawned, eventKindAgentDone:
			emit(ev)
		}
	}
}

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
	// Model is what the child's engine actually ran on. It is carried so that
	// Tokens.Cost can be checked against the prices it was worked out from —
	// and so a reader never has to guess the model in order to reprice, which
	// is how every reader ended up at the flat rate.
	Model       string        `json:"model,omitempty"`
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

	// Mint the child's session key, and hold it until the child is in the
	// session map. A key that is already a sibling's is a key that inherits that
	// sibling's recorded budget and agent — see mintChildSessionKey.
	childKey, err := g.mintChildSessionKey(req.ParentKey)
	if err != nil {
		return nil, err
	}
	defer g.releaseSessionKeyReservation(childKey)

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
			// Point the agent at every repository forge checked out, and at the
			// primary one as its working directory. A workspace that cannot
			// name its own primary repository fails the spawn: running anyway
			// would root the child at the server's process directory, and it
			// would edit this host's live checkouts believing they were its
			// worktree.
			if err := useWorkspace(&ac, w); err != nil {
				g.forgeDB.Cleanup(w)
				return nil, fmt.Errorf("workspace for %s: %w", req.Agent, err)
			}
			ws = w
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

	// Ensure spawn session exists in DB. The lineage goes down with it: it is
	// set on the Session above and nowhere else, so without this row the depth
	// cap and the parent's address are forgotten at the next restart.
	g.recordChildSession(child, req.Agent, "spawn")

	// Look up the parent's active request ID for linking.
	var parentReqID *int
	if pr, _ := g.store.ActiveRequest(req.ParentKey); pr != nil {
		parentReqID = &pr.ID
	}

	// Wire child hooks so the child's progress — and that of anything it spawns
	// in turn — reaches the parent's stream. The forwarder resolves that stream
	// per event (see emitToParentStream), so it is wired unconditionally: whether
	// the parent has a writer is a question about now, not about spawn time, and
	// a child spawned during a silent moment must still be visible once the
	// parent is being streamed again.
	child.setOnEvent(newSubagentEventForwarder(parent.getOnEvent, req.Agent, childKey, req.ParentKey, child.SpawnDepth))

	// Enqueue the work asynchronously.
	go func() {
		start := time.Now()

		// Track request in DB.
		reqID, _ := g.store.CreateRequest(childKey, truncate(req.Task, 500), parentReqID)

		// Notify parent that child has started — both via injection and live stream.
		g.deliverProgress(req.ParentKey, childKey, req.Agent,
			fmt.Sprintf("⏳ Sub-agent %s started working on: %s", req.Agent, truncate(req.Task, 100)))

		// Fire agent_spawned on the parent's live stream.
		emitToParentStream(parent, StreamEvent{
			Kind: eventKindAgentSpawned,
			Text: truncate(req.Task, 200),
			Data: map[string]any{
				"agent":       req.Agent,
				"session_key": childKey,
				"parent_key":  req.ParentKey,
				"depth":       child.SpawnDepth,
			},
		})

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

			// Complete request in DB. The model is the one the child's engine
			// last ran on, which executeAgent assigns after selection and
			// failover, so it is what was actually billed rather than what was
			// asked for.
			tokens.Cost = sessionMod.CalcCostWithCache(child.Engine.Model, tokens.Input, tokens.Output,
				tokens.CacheRead, tokens.CacheWrite, g.modelStore)
			turns := 0
			if result != nil {
				turns = result.ToolCalls
			}
			g.store.CompleteRequest(reqID, status, truncate(summary, 1000), errMsg,
				turns, tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite, tokens.Cost)
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
				Model:       child.Engine.Model,
				Duration:    time.Since(start),
				Error:       errMsg,
				WorkspaceID: workspaceID,
				Branch:      branch,
				Commits:     commits,
			}
			g.events.SpawnCompleted(spawnResult)

			// Fire agent_done on the parent's live stream. This one is the sharpest
			// case for resolving the writer now: a child reaches it after however
			// long its whole task took, by which point the turn that spawned it has
			// almost always ended.
			emitToParentStream(parent, StreamEvent{
				Kind: eventKindAgentDone,
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