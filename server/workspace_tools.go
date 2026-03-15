package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// toolsForAgent returns the set of server-provided tools for an agent session.
// Orchestrators get workspace management tools; all agents get spawn/steer/list.
func (g *Server) toolsForAgent(sessionKey, agentName string) []agent.Tool {
	tools := []agent.Tool{
		g.SpawnAgentTool(sessionKey),
		g.SessionsListTool(sessionKey),
		g.SteerAgentTool(),
	}

	// Orchestrator agents get workspace management tools.
	if agentName == g.config.DefaultAgent {
		tools = append(tools,
			g.MergeWorkspaceTool(),
			g.RejectWorkspaceTool(),
			g.FixWorkspaceTool(),
			g.ListWorkspacesTool(),
		)
	}

	return tools
}

// MergeWorkspaceTool lets the orchestrator merge a workspace's spawn branch into main.
func (g *Server) MergeWorkspaceTool() agent.Tool {
	return agent.Tool{
		Name: "merge_workspace",
		Description: "Merge a completed workspace's spawn branch into main for each repo. " +
			"This rebases onto main and fast-forward merges. Use after reviewing a sub-agent's work. " +
			"Returns per-repo merge status (ok, conflict, error).",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"workspace_id"},
			Properties: map[string]any{
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Workspace ID from the spawn result (e.g. brigid-1710512345)",
				},
				"push": map[string]any{
					"type":        "boolean",
					"description": "Push main to origin after merge (default true)",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				WorkspaceID string `json:"workspace_id"`
				Push        *bool  `json:"push,omitempty"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			g.mu.RLock()
			ws, ok := g.workspaces[in.WorkspaceID]
			g.mu.RUnlock()
			if !ok {
				return "", fmt.Errorf("workspace not found: %s", in.WorkspaceID)
			}

			if g.forgeDB == nil {
				return "", fmt.Errorf("forge not available")
			}

			// Merge each repo.
			results := g.forgeDB.MergeToMain(ws)

			var sb strings.Builder
			allOk := true
			for repo, mr := range results {
				sb.WriteString(fmt.Sprintf("%s: %s", repo, mr.Status))
				if mr.Status != "ok" {
					allOk = false
					if len(mr.Conflicts) > 0 {
						sb.WriteString(fmt.Sprintf(" (conflicts: %s)", strings.Join(mr.Conflicts, ", ")))
					}
					if mr.Error != "" {
						sb.WriteString(fmt.Sprintf(" — %s", mr.Error))
					}
				}
				sb.WriteString("\n")
			}

			// Push if requested (default true).
			shouldPush := in.Push == nil || *in.Push
			if allOk && shouldPush {
				pushErrs := g.forgeDB.PushAll(ws)
				for repo, err := range pushErrs {
					if err != nil {
						sb.WriteString(fmt.Sprintf("push %s: FAILED — %v\n", repo, err))
					} else {
						sb.WriteString(fmt.Sprintf("push %s: ok\n", repo))
					}
				}
			}

			// Cleanup workspace after successful merge.
			if allOk {
				if err := g.forgeDB.Cleanup(ws); err != nil {
					log.Printf("[server] workspace cleanup failed: %v", err)
				}
				g.mu.Lock()
				delete(g.workspaces, in.WorkspaceID)
				g.mu.Unlock()
				sb.WriteString("\n✅ Workspace merged and cleaned up.")
			} else {
				sb.WriteString("\n⚠️ Merge had issues. Workspace preserved for manual resolution.")
			}

			return sb.String(), nil
		},
	}
}

// RejectWorkspaceTool lets the orchestrator discard a workspace.
func (g *Server) RejectWorkspaceTool() agent.Tool {
	return agent.Tool{
		Name:        "reject_workspace",
		Description: "Discard a workspace and all its changes. Removes worktrees, deletes spawn branches, releases concurrency slots.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"workspace_id"},
			Properties: map[string]any{
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Workspace ID to discard",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Why the workspace is being rejected (logged)",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				WorkspaceID string `json:"workspace_id"`
				Reason      string `json:"reason,omitempty"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			g.mu.RLock()
			ws, ok := g.workspaces[in.WorkspaceID]
			g.mu.RUnlock()
			if !ok {
				return "", fmt.Errorf("workspace not found: %s", in.WorkspaceID)
			}

			if g.forgeDB == nil {
				return "", fmt.Errorf("forge not available")
			}

			reason := in.Reason
			if reason == "" {
				reason = "rejected by orchestrator"
			}
			log.Printf("[server] workspace %s rejected: %s", in.WorkspaceID, reason)

			if err := g.forgeDB.Cleanup(ws); err != nil {
				return fmt.Sprintf("cleanup had errors: %v", err), nil
			}

			g.mu.Lock()
			delete(g.workspaces, in.WorkspaceID)
			g.mu.Unlock()

			return fmt.Sprintf("🗑️ Workspace %s rejected and cleaned up. Reason: %s", in.WorkspaceID, reason), nil
		},
	}
}

// FixWorkspaceTool lets the orchestrator re-spawn an agent in the same workspace.
func (g *Server) FixWorkspaceTool() agent.Tool {
	return agent.Tool{
		Name: "fix_workspace",
		Description: "Re-spawn an agent in an existing workspace to fix issues. " +
			"The agent gets the same worktree with their previous changes still present. " +
			"Use when a spawn completed but the result needs fixes.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"workspace_id", "agent", "instructions"},
			Properties: map[string]any{
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Workspace ID to reopen",
				},
				"agent": map[string]any{
					"type":        "string",
					"description": "Agent to spawn for the fix",
				},
				"instructions": map[string]any{
					"type":        "string",
					"description": "What to fix or change",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				WorkspaceID  string `json:"workspace_id"`
				Agent        string `json:"agent"`
				Instructions string `json:"instructions"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			g.mu.RLock()
			ws, ok := g.workspaces[in.WorkspaceID]
			g.mu.RUnlock()
			if !ok {
				return "", fmt.Errorf("workspace not found: %s", in.WorkspaceID)
			}

			if g.forgeDB == nil {
				return "", fmt.Errorf("forge not available")
			}

			// Reopen the workspace.
			if err := g.forgeDB.ReopenWorkspace(ws); err != nil {
				return "", fmt.Errorf("reopen workspace: %w", err)
			}

			// Find the parent session key (the orchestrator calling this tool).
			// We'll use a synthetic parent key since this is a tool call, not a spawn request.
			// The orchestrator session is whoever called this tool — we get that from context.
			// For now, we need the parent key passed or inferred.

			// Resolve agent config.
			ac, agentOk := g.GetAgentConfig(in.Agent)
			if !agentOk {
				return "", fmt.Errorf("unknown agent: %s", in.Agent)
			}

			// Override workspace to the existing worktree.
			ac.Workspace = ws.Repos[ws.Primary]

			task := fmt.Sprintf("[FIX REQUEST — workspace %s]\n"+
				"You are working in an existing workspace with previous changes.\n"+
				"Review what's already done, then fix the following:\n\n%s", ws.ID, in.Instructions)

			// Find parent session for this tool call.
			// The tool runs in the orchestrator's session context.
			var parentKey string
			g.sessions.Range(func(key, val any) bool {
				s := val.(*Session)
				s.mu.Lock()
				running := s.Status == Running
				s.mu.Unlock()
				if running {
					parentKey = key.(string)
					return false
				}
				return true
			})

			if parentKey == "" {
				return "", fmt.Errorf("no running parent session found")
			}

			// Spawn into the existing workspace.
			resp, err := g.Spawn(ctx, SpawnRequest{
				ParentKey: parentKey,
				Agent:     in.Agent,
				Task:      task,
			})
			if err != nil {
				return "", fmt.Errorf("spawn fix agent: %w", err)
			}

			return fmt.Sprintf("🔧 Fix spawned: %s (%s)\nWorkspace: %s\nAgent will see previous changes and apply fixes.\nResult will be delivered when complete.",
				in.Agent, resp.ChildKey, ws.ID), nil
		},
	}
}

// ListWorkspacesTool shows active workspaces.
func (g *Server) ListWorkspacesTool() agent.Tool {
	return agent.Tool{
		Name:        "list_workspaces",
		Description: "List all active workspaces (from spawned agents that haven't been merged or rejected yet).",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			g.mu.RLock()
			defer g.mu.RUnlock()

			if len(g.workspaces) == 0 {
				return "No active workspaces.", nil
			}

			var sb strings.Builder
			for id, ws := range g.workspaces {
				repos := make([]string, 0, len(ws.Repos))
				for name := range ws.Repos {
					repos = append(repos, name)
				}
				sb.WriteString(fmt.Sprintf("%s [%s] repos=%s branch=%s\n",
					id, ws.Status, strings.Join(repos, ","), ws.Branch))
			}
			return sb.String(), nil
		},
	}
}
