package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every finished turn used to leave a timeline.md beside the session log, and
// producing it meant syncing the JSONL, parsing the whole file and formatting
// the whole timeline just to slice out the turn that had finished — so the cost
// of ending a turn grew with the length of the session.
//
// Nothing read the file. The one consumer, GET /api/sessions/{key}/timeline,
// calls ReadTimelineFromJSONL and regenerates the markdown from session.jsonl,
// which is the source of truth timeline.md was derived from. The write is gone.
// These two tests pin both halves of that: the artifact is not written, and the
// answer it used to hold is still available from the log.

// endTwoTurns logs two complete turns to a fresh session and returns the logs
// root, the session id, and the directory holding session.jsonl.
func endTwoTurns(t *testing.T) (logsDir, sessionID, sessionDir string) {
	t.Helper()

	logsDir = t.TempDir()
	session, err := New(logsDir, "claude-sonnet-4-20250514", "test-agent", "",
		registryWithTwoDifferentlyPricedModels(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer session.Close()

	for _, prompt := range []string{"first prompt", "second prompt"} {
		request, err := json.Marshal(map[string]any{
			"messages": []map[string]any{{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": prompt}},
			}},
		})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		session.LogRequest(request)
		session.LogAssistant("reply to the "+prompt, TurnTokens{Input: 100, Output: 50}, 0)
		session.EndTurn(TurnTokens{Input: 100, Output: 50}, 0, "end_turn", "")
	}

	sessionDir = filepath.Dir(session.FilePath())
	return logsDir, session.SessionID(), sessionDir
}

func TestEndTurnWritesNoTimelineArtifact(t *testing.T) {
	_, _, sessionDir := endTwoTurns(t)

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("read session dir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
		if entry.Name() == "timeline.md" {
			t.Fatalf("timeline.md was written; the derived file has no reader and rebuilding it costs a full parse of the log every turn")
		}
	}
	if len(names) == 0 {
		t.Fatalf("the session directory is empty, so this test would pass without proving anything")
	}
}

func TestTheTimelineIsStillAnswerableFromTheLog(t *testing.T) {
	logsDir, sessionID, _ := endTwoTurns(t)

	timeline, err := ReadTimelineFromJSONL(logsDir, sessionID,
		registryWithTwoDifferentlyPricedModels(t))
	if err != nil {
		t.Fatalf("ReadTimelineFromJSONL: %v", err)
	}
	for _, want := range []string{"first prompt", "second prompt", "reply to the first prompt"} {
		if !strings.Contains(timeline, want) {
			t.Errorf("timeline is missing %q; it reads:\n%s", want, timeline)
		}
	}
	if !strings.Contains(timeline, "## Turn 1") || !strings.Contains(timeline, "## Turn 2") {
		t.Errorf("both turns should appear in the regenerated timeline, it reads:\n%s", timeline)
	}
}
