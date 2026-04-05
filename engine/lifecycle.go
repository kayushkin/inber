package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// summarizeIfNeeded checks if the conversation is long enough to warrant summarization.
func (e *Engine) summarizeIfNeeded() {
	role := conversation.RoleDefault
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		role = conversation.AgentRole(strings.ToLower(e.AgentConfig.Role))
	}
	cfg := conversation.DefaultSummarizeConfig(role)

	if !conversation.ShouldSummarize(e.Messages, cfg) {
		return
	}

	e.emitStatus("Summarizing context...")

	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
	}

	model := e.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}

	summarized, result, err := conversation.SummarizeConversation(
		context.Background(),
		e.Client,
		e.Messages,
		e.MemStore,
		sessionID,
		cfg,
		model,
	)

	if err != nil {
		Log.Warn("summarization failed: %v", err)
		return
	}

	if result.Summarized {
		e.Messages = summarized
		Log.Info("summarized %d turns → %d token summary (kept %d recent messages, memory: %s)",
			result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		if e.Session != nil {
			e.Session.LogSummarize(result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		}
	}
}

// pruneConfig returns the appropriate PruneConfig for this engine's agent role.
func (e *Engine) pruneConfig() conversation.PruneConfig {
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		return conversation.PruneConfigForRole(e.AgentConfig.Role)
	}
	return conversation.DefaultPruneConfig()
}

// pruneIfNeeded uses staged conversation management:
//   - Between flushes: only dedup/prune the STAGING zone (uncached, free to mutate)
//   - On flush: freeze staging into cached prefix, advance FrozenIdx
//   - Emergency: full management if tokens > 2x budget
//
// Layout: [tools BP1] [system BP2] [frozen msgs BP3] [staging - uncached] [new input]
func (e *Engine) pruneIfNeeded() {
	cfg := e.pruneConfig()

	if e.staged == nil {
		e.staged = conversation.NewStagedConversation(cfg.ManageInterval)
	}
	e.staged.Tick()

	// Always dedup/prune the staging zone (it's uncached — zero cache cost)
	deduped := conversation.ManageStaging(e.Messages, e.staged.FrozenIdx, cfg)
	if deduped > 0 {
		Log.Info("staging dedup: %d file refs deduplicated", deduped)
	}

	// Check for cross-zone superseded files (frozen has old read, staging has newer)
	if e.staged.FrozenIdx > 0 && e.staged.FrozenIdx < len(e.Messages) {
		frozen := e.Messages[:e.staged.FrozenIdx]
		staging := e.Messages[e.staged.FrozenIdx:]
		superseded := conversation.CrossZoneDedup(frozen, staging)
		if len(superseded) > 0 {
			// Add note to volatile context so model knows frozen reads are stale
			note := "[Note: these files were re-read since last context snapshot — ignore earlier versions: "
			for i, p := range superseded {
				if i > 0 {
					note += ", "
				}
				note += p
			}
			note += "]"
			e.Turn.VolatileContext += "\n" + note
		}
	}

	// Check if it's time for a full flush (freeze staging)
	emergency := conversation.EstimateTokens(e.Messages) > cfg.TokenBudget*2
	if e.staged.ShouldFlush() || emergency {
		if emergency {
			Log.Warn("emergency flush: token estimate %d > 2x budget %d",
				conversation.EstimateTokens(e.Messages), cfg.TokenBudget)
		}

		// Full management on the entire conversation (happens rarely)
		sessionID := ""
		if e.Session != nil {
			sessionID = e.Session.SessionID()
		}

		pruned, result, err := conversation.PruneConversation(
			context.Background(),
			e.Messages,
			e.MemStore,
			sessionID,
			cfg,
		)

		if err != nil {
			Log.Warn("flush/prune failed: %v", err)
			return
		}

		e.Messages = pruned
		// Freeze everything: advance FrozenIdx to cover all current messages
		// (minus the last one which is the new user input, still staging)
		freezePoint := len(e.Messages)
		if freezePoint > 1 {
			freezePoint = freezePoint - 1 // keep latest user message in staging
		}
		e.staged.Flush(freezePoint)

		Log.Info("flush: froze %d messages (pruned %d, %d tokens freed, %d memories saved, %d files deduped)",
			freezePoint, result.PrunedMessages, result.TokensFreed, result.MemoriesSaved, result.DeduplicatedFiles)
		if e.Session != nil {
			e.Session.LogPrune(result.PrunedMessages, result.TokensFreed, result.Strategy)
		}
	}
}

// checkpointIfNeeded creates a checkpoint if we've reached the checkpoint interval.
func (e *Engine) checkpointIfNeeded() {
	if e.Session == nil {
		return
	}

	cfg := sessionMod.DefaultCheckpointConfig()
	if !sessionMod.ShouldCheckpoint(e.Turn.Counter, cfg) {
		return
	}

	summary := sessionMod.GenerateConversationSummary(e.Messages)
	keyFacts := sessionMod.ExtractKeyFacts(e.Messages, 10)

	err := e.Session.SaveCheckpoint(e.Messages, summary, keyFacts)
	if err != nil {
		Log.Warn("checkpoint failed: %v", err)
	} else {
		Log.Info("checkpoint saved (turn %d)", e.Turn.Counter)
	}
}

// saveMessages writes the current messages to the workspace and session log dir.
func (e *Engine) saveMessages() {
	data, err := json.Marshal(e.Messages)
	if err != nil {
		return
	}
	if e.workspace != nil {
		e.workspace.SaveMessages(data)
	}
	if e.Session != nil {
		sessDir := filepath.Dir(e.Session.FilePath())
		os.WriteFile(filepath.Join(sessDir, "messages.json"), data, 0644)
	}
}

// LogUser logs a user message to the session (for external callers that need pre-logging).
func (e *Engine) LogUser(input string) {
	if e.Session != nil {
		e.Session.LogUser(input)
	}
}

// LogAssistant logs an assistant response to the session.
func (e *Engine) LogAssistant(result *agent.TurnResult) {
	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}
}

// SaveSessionSummary generates a brief session summary and saves it to memory.
func SaveSessionSummary(store memory.MemoryStore, messages []anthropic.MessageParam, agentName string) {
	var parts []string
	for _, msg := range messages {
		role := string(msg.Role)
		for _, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				parts = append(parts, fmt.Sprintf("%s: %s", role, text))
			}
		}
	}

	if len(parts) == 0 {
		return
	}

	summary := fmt.Sprintf("Session summary (%s):\n%s", agentName, strings.Join(parts, "\n"))
	if len(summary) > 2000 {
		summary = summary[:2000]
	}

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    summary,
		Tags:       []string{"session-summary", agentName},
		Importance: 0.4,
		Source:     "system",
	}

	if err := store.Save(m); err != nil {
		Log.Warn("failed to save session summary: %v", err)
	}
}

