package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kayushkin/inber/agent"
)

// RunTurn sends a user message, rebuilds the system prompt, runs the agent, and returns the result.
// This is the main turn execution pipeline, split into clear phases:
// 1. Prepare input (stash, repair alternation, summarize/prune)
// 2. Build context (system prompt)
// 3. Execute agent (model selection, API call, retry logic)
// 4. Post-process (memory extraction, stashing, save, checkpoint, tracking)
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
	// Increment and log turn number
	e.Turn.Counter++
	e.Turn.StartTime = time.Now()
	fmt.Fprintf(os.Stderr, "\n%s━━━ Turn %d ━━━%s\n", cyan+bold, e.Turn.Counter, reset)

	// Take memory snapshot at start of turn
	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.Turn.Counter)
	}

	// Get session ID for tagging
	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
		e.Session.LogUser(input)
	}

	// Phase 1: Prepare input (stash, repair, summarize, prune)
	processedInput := e.prepareInput(input, sessionID)

	// Phase 2: Build context (system prompt)
	systemBlocks := e.buildTurnContext(processedInput)

	// Phase 3: Execute agent (model selection, API call)
	result, err := e.executeAgent(context.Background(), systemBlocks)
	if err != nil {
		return nil, err
	}

	// Phase 4: Post-process (memory extraction, stashing, tracking)
	if err := e.postProcessResult(result, input, sessionID); err != nil {
		Log.Warn("post-processing failed: %v", err)
	}

	// Take memory snapshot at end of turn
	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.Turn.Counter)
	}

	return result, nil
}

// Close saves session summary, closes memory store, and unregisters the active session.
func (e *Engine) Close() {
	if e.workflowHooks != nil {
		if summary := e.workflowHooks.FinishSession(); summary != "" {
			fmt.Fprintln(os.Stderr, "\n"+summary)
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
	if e.modelStore != nil && e.ownsModelStore {
		e.modelStore.Close()
	}

	if e.forgeDB != nil {
		e.forgeDB.Close()
	}

	// Close memory profiler
	if e.memoryProfiler != nil {
		e.memoryProfiler.Close()
	}
}