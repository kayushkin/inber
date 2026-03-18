package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// deliverProgress sends progress messages from child to parent session.
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