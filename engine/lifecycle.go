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
	"github.com/kayushkin/inber/internal/textutil"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// RestoreSession installs a transcript this engine did not produce — a resumed
// session's persisted history, or a fork of a live parent's context — together
// with the turn count that produced it, and marks the transcript frozen.
//
// Freezing is why assigning to e.Messages directly is wrong. A restored
// transcript is history: it is replayed verbatim at the head of the next request
// and this process will not rewrite it. Left in the staging zone it claims the
// opposite — that every message in it is recent and free to mutate — and the
// first turn after the restore then runs DeduplicateFileRefs and age-based
// tool-result pruning across the entire transcript, dropping results without the
// memory write a real prune performs. It also leaves FrozenIdx at 0, which sends
// the BP3 cache breakpoint down its legacy fallback, so a prefix that was
// already paid for is paid for again.
//
// The turn count travels with the transcript for the same reason the freeze
// does: a fresh engine starts at 0, and every reader of e.Turn.Counter then
// treats a long resumed session as a brand new one. contextBudget hands it the
// 4,000-token first-turn memory budget instead of the 6,000 or 8,000 a
// conversation of that age earns, and cannot reach its Counter > 15 branch for
// another fifteen turns. ShouldCheckpoint gates on turn%Interval, so a session
// resumed more often than the interval never checkpoints at all. Unlike
// FrozenIdx the count is not derivable from the messages, so it is persisted —
// see PersistedTurnCounter.
//
// An oversized restored transcript is still pruned: the flush path (and its
// emergency threshold) runs over the whole conversation regardless of the
// frozen boundary.
func (e *Engine) RestoreSession(messages []anthropic.MessageParam, turnCounter int) {
	e.Messages = messages
	e.Turn.Counter = turnCounter
	if e.staged == nil {
		e.staged = conversation.NewStagedConversation(e.pruneConfig().ManageInterval)
	}
	e.staged.Flush(conversation.FreezePoint(messages))
}

// summarizeIfNeeded checks if the conversation is long enough to warrant summarization.
// A summarization that fails returns the error with the conversation untouched — the
// caller decides whether a compaction it did not get is worth interrupting for.
//
// ctx bounds the summarization API call. Each caller passes the context that
// actually owns the work: a turn passes the turn's context, so an interrupt
// stops summarizing, and the explicit compact endpoint passes its request's
// context, so a client that hangs up stops paying for a summary nobody will
// read. There is no single policy to pick here because the two callers are not
// the same piece of work.
func (e *Engine) summarizeIfNeeded(ctx context.Context) error {
	role := conversation.RoleDefault
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		role = conversation.AgentRole(strings.ToLower(e.AgentConfig.Role))
	}
	cfg := conversation.DefaultSummarizeConfig(role)

	// Whether the summary block may point at the archived transcript. Read off
	// the tools this session actually puts on the wire, so a name the agent was
	// never configured with — or one SetDisabledTools has since taken away —
	// never gets advertised to the model.
	cfg.ArchiveIsRecallable = e.hasEnabledTool(memoryExpandToolName)

	if !conversation.ShouldSummarize(e.Messages, cfg) {
		return nil
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
		ctx,
		e.Client,
		e.Messages,
		e.MemStore,
		sessionID,
		cfg,
		model,
	)

	if err != nil {
		Log.Warn("summarization failed, conversation left uncompacted: %v", err)
		return err
	}

	if result.Summarized {
		e.Messages = summarized
		Log.Info("summarized %d turns → %d token summary (kept %d recent messages, memory: %s)",
			result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		// The line above reads the same whether the model finished or ran out of
		// output tokens mid-sentence, and the assignment above it is destructive:
		// what is left is the session's transcript from here on. Say so loudly,
		// and name the archive, which is the only remaining copy of the turns.
		if result.SummaryWasCutOffAtTokenLimit {
			Log.Warn("the summary those %d turns were exchanged for was cut off at the %d-token limit — the transcript now continues from a fragment (full text archived as %q)",
				result.SummarizedTurns, cfg.MaxSummaryTokens, result.MemoryID)
		}
		if e.Session != nil {
			e.Session.LogSummarize(result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		}
	}
	return nil
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
//
// ctx bounds the emergency flush, which is the one branch that leaves this
// function's own memory. See summarizeIfNeeded for why the caller supplies it.
func (e *Engine) pruneIfNeeded(ctx context.Context) {
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
			// Queue a note so the model knows its frozen-zone reads are stale.
			// It cannot be written onto e.Turn.VolatileContext here: the prompt
			// build that runs next assigns that field wholesale.
			note := "[Note: these files were re-read since last context snapshot — ignore earlier versions: "
			for i, p := range superseded {
				if i > 0 {
					note += ", "
				}
				note += p
			}
			note += "]"
			e.queueVolatileNote(note)
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
			ctx,
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
		// Freeze everything except the new user input, which is still staging.
		freezePoint := conversation.FreezePoint(e.Messages)
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

	err := e.Session.SaveCheckpoint(e.Turn.Counter, e.Messages, summary, keyFacts)
	if err != nil {
		Log.Warn("checkpoint failed: %v", err)
	} else {
		Log.Info("checkpoint saved (turn %d)", e.Turn.Counter)
	}
}

// saveResumableState writes the current messages to the workspace and session
// log dir, and the turn count to the workspace beside them.
//
// Only the workspace copy is resumable, which is why only it gets the turn
// count: the session log dir is a new directory per invocation, so nothing ever
// reads its transcript back into an engine. The log-dir copy is kept because it
// is the only lossless record in that directory — session.jsonl is an
// append-only log, and reconstructing messages from it drops any tool call the
// process died in the middle of (session.LoadMessagesFromDir prefers the
// snapshot for exactly that reason).
//
// Every write here used to discard its error, so a full disk lost the
// transcript in silence. The counter is written only after its transcript is,
// because a count without the messages it describes tells the next session it
// is fifty turns into a conversation that is not there.
func (e *Engine) saveResumableState() {
	data, err := json.Marshal(e.Messages)
	if err != nil {
		Log.Warn("transcript not persisted, this turn will be missing from the next resume: %v", err)
		return
	}
	if e.workspace != nil {
		if err := e.workspace.SaveMessages(data); err != nil {
			Log.Warn("transcript not persisted to the workspace, the next resume will replay an older conversation: %v", err)
		} else if err := sessionMod.SaveTurnCounter(e.workspace.Dir, e.Turn.Counter); err != nil {
			Log.Warn("turn counter not persisted, next resume will start from turn 0: %v", err)
		}
	}
	if e.Session != nil {
		sessDir := filepath.Dir(e.Session.FilePath())
		if err := os.WriteFile(filepath.Join(sessDir, "messages.json"), data, 0644); err != nil {
			Log.Warn("transcript snapshot not written to the session log dir, that session is now reconstruct-only: %v", err)
		}
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
		e.Session.LogAssistant(result.Text, turnTokens(result), result.ToolCalls)
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
				text = textutil.TruncateWith(text, 200, "...")
				parts = append(parts, fmt.Sprintf("%s: %s", role, text))
			}
		}
	}

	if len(parts) == 0 {
		return
	}

	summary := fmt.Sprintf("Session summary (%s):\n%s", agentName, strings.Join(parts, "\n"))
	summary = textutil.Truncate(summary, 2000)

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    summary,
		Tags:       []string{memory.TagSessionSummary, agentName},
		Importance: 0.4,
		Source:     "system",
	}

	if err := store.Save(m); err != nil {
		Log.Warn("failed to save session summary: %v", err)
	}
}

