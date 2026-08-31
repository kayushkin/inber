package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// The server-side spawn_agent tool cuts the task it was handed down to a
// preview and puts that preview in the string it returns to the model as the
// tool result:
//
//	server/spawn_tools.go:141   textutil.Truncate(taskPreview, 97) + "..."
//
// It is one of the cuts commit 9ce1666 calls "the ones that matter", and the
// sibling of agent/registry/spawn_tool.go:163. Measured 2026-08-31 on
// test/every-textutil-cut-is-asked (54ce4ec): reverting this site to the
// pre-9ce1666 byte cut, and deleting the truncation outright, each left the
// whole repository green. Card 3388f950, residual 0751b490.
//
// # Why these cases needed a seam first
//
// The cut sits after g.Spawn must succeed. Spawn validates the parent session,
// checks the depth and children limits, consults the agent store, mints a
// session key, creates a real child session, writes a request row and starts a
// goroutine. A test that reaches the cut through the real Spawn is not testing
// the tool; it is standing up half a server, and it fails for a dozen reasons
// having nothing to do with a rune boundary. Server.spawnChildSession is that
// seam, and every case below supplies its own.
//
// The assertion is on the tool's returned string, never on an extracted helper
// — card 3388f950 is about repairs whose tests cover the helper and leave the
// call site unheld.

// spawnToolResultFor runs the server's spawn_agent against a spawn that
// succeeds without creating anything, and returns the tool result the model
// would receive.
func spawnToolResultFor(t *testing.T, task string) string {
	t.Helper()

	g := &Server{}
	g.spawnChildSession = func(_ context.Context, req SpawnRequest) (*SpawnResponse, error) {
		if req.Task != task {
			t.Errorf("the tool passed a different task to the spawn than it was given:\n"+
				" got %d bytes\nwant %d bytes", len(req.Task), len(task))
		}
		return &SpawnResponse{Status: "started", ChildKey: "agent:claxon:sub-1"}, nil
	}

	raw, err := json.Marshal(map[string]any{"agent": "claxon", "task": task})
	if err != nil {
		t.Fatalf("marshal the tool input: %v", err)
	}
	out, err := g.SpawnAgentTool("agent:claxon:main").Run(context.Background(), string(raw))
	if err != nil {
		t.Fatalf("spawn_agent refused a task it should have accepted: %v", err)
	}
	return out
}

// taskLineOf pulls the preview back out of the tool result. The result reads
//
//	🚀 Spawned <agent> (<childKey>)
//	Task: <preview>
//	Fork: <bool>
func taskLineOf(t *testing.T, result string) string {
	t.Helper()
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "Task: ") {
			return strings.TrimPrefix(line, "Task: ")
		}
	}
	t.Fatalf("no task line in the spawn tool result:\n%s", result)
	return ""
}

func TestTheServerSpawnedTaskPreviewLandsOnARuneBoundary(t *testing.T) {
	for pad := 0; pad < 4; pad++ {
		// An ASCII prefix, then two-byte runes. Shifting the prefix by one byte
		// moves the cut's offset inside the multibyte run by one, so four
		// consecutive values cover both residues modulo the rune width. 97 is a
		// literal in the source, and a fixture built to straddle today's value
		// goes quiet in the flattering direction the day it is changed.
		task := strings.Repeat("a", 90+pad) + strings.Repeat("é", 100)
		result := spawnToolResultFor(t, task)
		preview := taskLineOf(t, result)

		cut := strings.TrimSuffix(preview, "...")
		if cut == preview || cut == "" || cut[len(cut)-1] < 0x80 {
			t.Fatalf("pad=%d: the preview was not cut inside the multibyte run, so this "+
				"case cannot tell a byte cut from a rune-safe one. The fixture needs "+
				"rebuilding against the current budget.\nPreview %q", pad, preview)
		}
		if !utf8.ValidString(result) {
			t.Errorf("pad=%d: the tool result handed back to the model is not valid "+
				"UTF-8: the task preview was cut inside a rune.\nPreview %q", pad, preview)
		}
	}
}

func TestTheServerSpawnedTaskIsCutBeforeItReachesTheModel(t *testing.T) {
	short := strings.Repeat("s", 100)
	if got := taskLineOf(t, spawnToolResultFor(t, short)); got != short {
		t.Errorf("a %d-byte task is inside the budget and should reach the model whole; "+
			"it did not.\nPreview %q", len(short), got)
	}

	long := strings.Repeat("L", 5000)
	got := taskLineOf(t, spawnToolResultFor(t, long))

	// Contains, not equality. The site is `Truncate(...) + "..."`, so a mutation
	// that stops the cut but keeps the marker yields `long + "..."` — unequal to
	// `long`, and an equality check calls that a pass. Measured on this same
	// scorer at spawn_delivery.go:214, where the equality form was watched going
	// red and still let the site be reported SURVIVED.
	if strings.Contains(got, long) {
		t.Errorf("the whole %d-byte task reached the model, so the cut at "+
			"spawn_tools.go:141 bounds nothing.\nPreview is %d bytes", len(long), len(got))
	}
}

// The seam the two cases above stand on. Without it they would reach the cut
// only by way of the real Spawn, which on a zero-value Server refuses at its
// first line — so they would not reach it at all.
func TestTheServerSpawnToolReportsTheChildTheSpawnReturned(t *testing.T) {
	asked := 0
	g := &Server{}
	g.spawnChildSession = func(_ context.Context, _ SpawnRequest) (*SpawnResponse, error) {
		asked++
		return &SpawnResponse{Status: "started", ChildKey: "agent:claxon:sub-7"}, nil
	}

	raw, _ := json.Marshal(map[string]any{"agent": "claxon", "task": "a short task"})
	out, err := g.SpawnAgentTool("agent:claxon:main").Run(context.Background(), string(raw))
	if err != nil {
		t.Fatalf("spawn_agent refused a spawn that succeeded: %v", err)
	}
	if asked != 1 {
		t.Errorf("the spawn tool called the injected spawn %d times, want exactly 1", asked)
	}
	if !strings.Contains(out, "agent:claxon:sub-7") {
		t.Errorf("the tool result does not name the child key the spawn returned, so the "+
			"model is told about a session that is not the one that was started:\n%s", out)
	}
}
