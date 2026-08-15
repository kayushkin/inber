package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	agentstore "github.com/kayushkin/agent-store"
)

// statusToolWithStore builds the agents_status tool over a real, empty
// agent-store. Empty matters: ListStatuses on an empty store answers with a
// recognisable sentence, so a test can tell the roster path from an error
// without needing to seed rows.
func statusToolWithStore(t *testing.T) (func(string) (string, error), *agentstore.Store) {
	t.Helper()
	store, err := agentstore.Open(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("open agent-store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	run := (&Server{agentStore: store}).AgentsStatusTool().Run
	return func(raw string) (string, error) { return run(context.Background(), raw) }, store
}

// The defect this pins: agents_status discarded its json.Unmarshal error, so an
// input that did not parse left AgentSlug at "" — the same value that means
// "the caller named no agent" — and the tool answered with the whole roster.
// The model asked about one agent and got all of them, with nothing on the
// result saying its argument was unreadable.
//
// If this reddens with the roster sentence, the parse error is being swallowed
// again; return it instead. Bare `json.Unmarshal(...)` with no `if err` is the
// shape to look for.
func TestAnUnreadableAgentSlugIsAnErrorNotTheWholeRoster(t *testing.T) {
	run, _ := statusToolWithStore(t)

	// Truncated mid-argument, which is the live trigger: a max_tokens-cut turn
	// carries a half-written tool_use through as a success.
	for _, raw := range []string{
		`{"agent_slug": "brigi`,
		`{"agent_slug":`,
		`not json at all`,
		`{"agent_slug": 42}`,
	} {
		out, err := run(raw)
		if err == nil {
			t.Errorf("input %q: want an error naming the unreadable input, got nil error and output %q", raw, out)
		}
		if strings.Contains(out, "No agent statuses tracked yet") {
			t.Errorf("input %q: answered from the roster path; the parse failure was read as \"no agent named\"", raw)
		}
	}
}

// The deliberate no-argument case must keep working — it is the reason the
// empty-slug fallthrough exists, and separating it from "unreadable" is the
// whole point of the fix above. Claude Code sends "{}" for a no-argument call
// and inber's own callers send "".
func TestCallingAgentsStatusWithNoArgumentStillListsTheRoster(t *testing.T) {
	run, _ := statusToolWithStore(t)

	for _, raw := range []string{"", "{}", `{"agent_slug": ""}`} {
		out, err := run(raw)
		if err != nil {
			t.Errorf("input %q: no-argument call must not error, got %v", raw, err)
		}
		if !strings.Contains(out, "No agent statuses tracked yet") {
			t.Errorf("input %q: want the roster listing, got %q", raw, out)
		}
	}
}

// A well-formed slug still reaches GetStatus rather than falling through to the
// roster. Without this, returning an error for every input would satisfy the
// first test and nothing would notice.
func TestANamedAgentSlugIsLookedUpRatherThanListed(t *testing.T) {
	run, store := statusToolWithStore(t)

	// agent_status.harness_id is a foreign key, so the harness row has to exist
	// before any status can be written against it.
	if err := store.UpsertHarness(&agentstore.Harness{ID: "claude_code", DisplayName: "Claude Code"}); err != nil {
		t.Fatalf("seed harness: %v", err)
	}

	// Two agents, one status apiece, so "looked up" and "listed" give visibly
	// different answers: the lookup names one, the roster names both.
	for _, slug := range []string{"brigid", "lugh"} {
		if err := store.UpsertAgent(&agentstore.Agent{Slug: slug, DisplayName: slug, Enabled: true}); err != nil {
			t.Fatalf("seed agent %q: %v", slug, err)
		}
		if err := store.SetStatus(slug, "claude_code", "idle", "", ""); err != nil {
			t.Fatalf("seed status for %q: %v", slug, err)
		}
	}

	out, err := run(`{"agent_slug": "brigid"}`)
	if err != nil {
		t.Fatalf("named slug must not error: %v", err)
	}
	if !strings.Contains(out, "brigid") {
		t.Errorf("want the answer to name the agent asked about, got %q", out)
	}
	if strings.Contains(out, "lugh") {
		t.Errorf("a named slug returned the whole roster: %q", out)
	}
}
