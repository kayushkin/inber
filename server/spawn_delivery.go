package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/bus/messages"
	"github.com/kayushkin/inber/internal/textutil"
	"github.com/kayushkin/inber/memory"
)

// describeSpawnTokens renders a child's usage for the completion message its
// parent reads.
//
// It names the whole prompt, not the fresh part. TokenUsage's four counts are
// disjoint — Input is the portion of the prompt that was neither read from the
// cache nor written to it — and inber caches deliberately, so on the server's
// own traffic Input is about 4% of what a child actually sent. This line used
// to report that 4% under the bare word "in", so a sub-agent that pushed 1.5M
// prompt tokens announced itself to its parent as 39,915. The parent is a model
// deciding whether to spawn again, and it was reading the cost right next to a
// token figure twenty-five times too small to explain it.
func describeSpawnTokens(tokens TokenUsage) string {
	return fmt.Sprintf("prompt=%d (fresh=%d cache_read=%d cache_write=%d) out=%d ($%.3f)",
		tokens.Input+tokens.CacheRead+tokens.CacheWrite,
		tokens.Input, tokens.CacheRead, tokens.CacheWrite,
		tokens.Output, tokens.Cost)
}

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
		parent.inject(message)
	} else {
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

	msg := fmt.Sprintf("[Sub-agent completed]\n"+
		"Agent: %s (%s)\n"+
		"Task: %s\n"+
		"Status: %s\n"+
		"Duration: %s\n"+
		"Tokens: %s\n"+
		"\nResult:\n%s",
		result.Agent, result.ChildKey,
		result.Task,
		result.Status,
		result.Duration.Round(time.Second),
		describeSpawnTokens(result.Tokens),
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
		log.Printf("[server] delivering spawn result to idle parent %s", parentKey)

		// Publish completed spawn result to chat.outbound for dashboard
		if g.events != nil {
			summary := fmt.Sprintf("🔔 **Sub-agent %s completed** (%s)\n%s", result.Agent, result.Status, result.Summary)
			g.events.PublishOutbound(parent.AgentName, "main", summary)
		}

		// Trigger a turn on the parent session with the spawn result
		go func() {
			ac, ok := g.GetAgentConfig(parent.AgentName)
			if !ok {
				log.Printf("[server] cannot deliver spawn result: unknown agent %s", parent.AgentName)
				return
			}

			sessionID := "main"
			var fullText strings.Builder
			var onEvent func(StreamEvent)
			if g.bus != nil {
				onEvent = func(ev StreamEvent) {
					// This path relays prose only. Tool traffic from a
					// spawn-result turn has never reached the bus, and
					// narrowing here rather than in busDeltaFor keeps that
					// difference from the chat.inbound path visible.
					switch ev.Kind {
					case "delta", "thinking":
					default:
						return
					}
					delta, ok := busDeltaFor(parent.AgentName, sessionID, ev)
					if !ok {
						return
					}
					if ev.Kind == "delta" {
						fullText.WriteString(ev.Text)
					}
					g.bus.PublishDelta(delta)
				}
			}

			req := RunRequest{
				Agent:      parent.AgentName,
				Message:    msg,
				Channel:    "webchat",
				SessionKey: parentKey,
			}
			_, err := g.run(context.Background(), req, onEvent)
			if err != nil {
				log.Printf("[server] failed to deliver spawn result to %s: %v", parentKey, err)
			}

			// Publish done + outbound
			done := messages.NewDoneDelta(parent.AgentName, "inber", sessionID, nil)
			g.bus.PublishDelta(done)
			if fullText.Len() > 0 {
				g.bus.PublishOutbound(messages.ChatOutbound{
					Agent:        parent.AgentName,
					Orchestrator: "inber",
					SessionID:    sessionID,
					Text:         fullText.String(),
					Timestamp:    time.Now(),
				})
			}

			_ = ac
		}()
	}
}

// saveSpawnToMemory persists a summary of the spawn's work into the agent's memory DB.
func (g *Server) saveSpawnToMemory(child *Session, agentName, task, status, summary string) {
	if child.Engine == nil || child.Engine.MemStore == nil {
		return
	}

	content := fmt.Sprintf("Spawn task: %s\nStatus: %s\n\n%s", task, status, summary)

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

// updateMainSession injects a short context update into the agent's main session.
func (g *Server) updateMainSession(agentName, task, status, summary string) {
	mainKey := fmt.Sprintf("agent:%s:main", agentName)
	val, ok := g.sessions.Load(mainKey)
	if !ok {
		return
	}
	main := val.(*Session)

	summaryTrunc := summary
	if len(summaryTrunc) > 500 {
		summaryTrunc = textutil.Truncate(summaryTrunc, 497) + "..."
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
