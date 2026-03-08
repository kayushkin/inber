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

// pruneIfNeeded checks if conversation should be pruned and does so if necessary.
func (e *Engine) pruneIfNeeded() {
	cfg := e.pruneConfig()

	if !conversation.ShouldPrune(e.Messages, cfg) {
		return
	}

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
		Log.Warn("pruning failed: %v", err)
		return
	}

	if result.PrunedMessages > 0 {
		e.Messages = pruned
		Log.Info("pruned %d messages (%d tokens freed, %d memories saved)",
			result.PrunedMessages, result.TokensFreed, result.MemoriesSaved)
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
	if !sessionMod.ShouldCheckpoint(e.TurnCounter, cfg) {
		return
	}

	summary := sessionMod.GenerateConversationSummary(e.Messages)
	keyFacts := sessionMod.ExtractKeyFacts(e.Messages, 10)

	err := e.Session.SaveCheckpoint(e.Messages, summary, keyFacts)
	if err != nil {
		Log.Warn("checkpoint failed: %v", err)
	} else {
		Log.Info("checkpoint saved (turn %d)", e.TurnCounter)
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

// Close saves session summary, closes memory store, and unregisters the active session.
func (e *Engine) Close() {
	if e.workflowHooks != nil {
		if summary := e.workflowHooks.FinishSession(); summary != "" {
			fmt.Fprintln(os.Stderr, "\n"+summary)
		}
	}

	// Post-request verification: check if changes are pushed and deployed
	if !e.noHooks && e.Client != nil && e.repoRoot != "" && len(e.Messages) > 0 {
		verifier := NewPostRequestVerifier(e.repoRoot, e.Client, nil)
		if result, err := verifier.Verify(context.Background()); err == nil {
			if output := result.Format(); output != "" {
				fmt.Fprint(os.Stderr, output)
			}
		}
	}

	if e.MemStore != nil && len(e.Messages) > 0 {
		SaveSessionSummary(e.MemStore, e.Messages, e.AgentName)
	}

	if e.MemStore != nil {
		e.MemStore.Close()
	}
	if e.Session != nil {
		e.Session.Close()
	}
	if e.SessionDB != nil {
		e.SessionDB.Close()
	}
	if e.modelStore != nil {
		e.modelStore.Close()
	}
}

// SaveSessionSummary generates a brief session summary and saves it to memory.
func SaveSessionSummary(store *memory.Store, messages []anthropic.MessageParam, agentName string) {
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

// FindRepoRoot finds the repository root by looking for .git directory.
func FindRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}
