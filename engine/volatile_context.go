package engine

import "strings"

// The turn's volatile context is the text injected into the last user message,
// after the cache breakpoints, so it can change every turn without busting the
// prompt cache. Two things write to it and they run in a fixed order:
// BuildSystemPrompt assembles it from memories, fleet status and injectors, and
// context preparation queues notes about the conversation itself before that.
// Everything in this file exists so the second kind cannot be overwritten by the
// first, and so the value is handed to the agent exactly once.

// queueVolatileNote records a note to fold into this turn's volatile context.
//
// Callers run during context preparation, which happens before the prompt build
// that assigns e.Turn.VolatileContext — a note written straight onto that field
// would be overwritten and never reach the model. A note already queued is not
// queued twice, so a compaction that repeats a finding does not repeat the text.
func (e *Engine) queueVolatileNote(note string) {
	if note == "" {
		return
	}
	for _, queued := range e.Turn.PendingVolatileNotes {
		if queued == note {
			return
		}
	}
	e.Turn.PendingVolatileNotes = append(e.Turn.PendingVolatileNotes, note)
}

// applyPendingVolatileNotes folds the queued notes into this turn's volatile
// context and clears the queue.
//
// It runs from buildTurnContext, after the prompt build: on every path,
// including the ones where BuildSystemPrompt returns early without touching the
// field. Clearing is what keeps a note from being repeated on later turns —
// each turn's context preparation re-derives the notes that still apply.
func (e *Engine) applyPendingVolatileNotes() {
	if len(e.Turn.PendingVolatileNotes) == 0 {
		return
	}
	notes := strings.Join(e.Turn.PendingVolatileNotes, "\n")
	if e.Turn.VolatileContext == "" {
		e.Turn.VolatileContext = "[Context]\n" + notes
	} else {
		e.Turn.VolatileContext += "\n" + notes
	}
	e.Turn.PendingVolatileNotes = nil
}

// takeVolatileContext returns this turn's volatile context and clears it, so the
// agent built next holds the only copy.
//
// Agent.Run injects that text into the last user message of the engine's own
// message slice, which persists the injection. The thinking-signature repair
// path then rebuilds the agent and runs again over those same messages, so an
// engine that still held the text would put a second copy in one message.
func (e *Engine) takeVolatileContext() string {
	volatileContext := e.Turn.VolatileContext
	e.Turn.VolatileContext = ""
	return volatileContext
}
