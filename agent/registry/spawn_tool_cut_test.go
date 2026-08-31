package registry

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// SpawnAgentTool cuts the task it was handed down to a preview and puts that
// preview in the string it returns to the model as the tool result:
//
//	agent/registry/spawn_tool.go:163   textutil.Truncate(taskPreview, 97) + "..."
//
// Commit 9ce1666 ("Cut text on a rune boundary, everywhere it is cut") calls
// the cuts that reach the model the ones that matter. This is one of them and
// until now nothing asked about it: measured 2026-08-31 on branch
// test/every-textutil-cut-is-asked (54ce4ec), reverting this call site to the
// pre-9ce1666 byte cut, and deleting the truncation outright, each left the
// whole repository's suite green. Card 3388f950, residual 0751b490.
//
// # Why these cases needed a seam first
//
// The tool's body sits behind a validation branch fed by the bus-agent
// registry, which fetchRegistryAgents reads with a hardcoded GET against a
// fixed port. A test written without a seam reaches line 163 only because that
// endpoint currently answers with a page that fails to unmarshal, so the agent
// list comes back empty and the validation is skipped — and it would stop
// reaching it the day the endpoint answers JSON, for a reason having nothing to
// do with the cut. Registry.listRegisteredAgents is that seam, and every case
// below supplies its own list.

// spawnedToolResult runs spawn_agent against a registry that lists exactly one
// enabled agent and returns the tool result the model would receive.
func spawnedToolResult(t *testing.T, task string) string {
	t.Helper()

	r := &Registry{listRegisteredAgents: func() []RegistryAgent {
		return []RegistryAgent{{Name: "claxon", Orchestrator: "inber", Enabled: true}}
	}}

	raw, err := json.Marshal(map[string]string{"agent": "claxon", "task": task})
	if err != nil {
		t.Fatalf("marshal the tool input: %v", err)
	}
	out, err := r.SpawnAgentTool().Run(context.Background(), string(raw))
	if err != nil {
		t.Fatalf("spawn_agent refused a task it should have accepted: %v", err)
	}
	return out
}

// taskFieldOf pulls the preview back out of the tool result. It reads the
// result the model is handed rather than calling any helper the production code
// calls, so a repair that moves the cut into an extracted function is still
// scored at the call site.
func taskFieldOf(t *testing.T, result string) string {
	t.Helper()
	const opens, closes = "\n\nTask: ", "\n\nThe result will be delivered"
	start := strings.Index(result, opens)
	end := strings.LastIndex(result, closes)
	if start < 0 || end < start {
		t.Fatalf("no task preview in the spawn tool result:\n%s", result)
	}
	return result[start+len(opens) : end]
}

// cutInsideTheMultibyteRun reports whether the preview was cut inside a run of
// multibyte runes, which is the only state in which a case can tell a byte cut
// from a rune-safe one. If the byte before the marker is ASCII the cut landed
// in the fixture's prefix and the case proves nothing.
func cutInsideTheMultibyteRun(preview string) bool {
	cut := strings.TrimSuffix(preview, "...")
	if cut == preview || cut == "" {
		return false
	}
	return cut[len(cut)-1] >= 0x80
}

// multibyteTask is an ASCII prefix of `prefix` bytes followed by `runes` copies
// of a two-byte rune. Shifting `prefix` by one byte moves the cut's offset
// within the multibyte run by one, so sweeping consecutive values covers both
// residues modulo the rune width. That is deliberate: 97 is a literal in the
// source, and a fixture engineered to straddle today's value goes quiet in the
// flattering direction the day it is changed.
func multibyteTask(prefix, runes int) string {
	return strings.Repeat("a", prefix) + strings.Repeat("é", runes)
}

func TestTheSpawnedTaskPreviewReturnedToTheModelLandsOnARuneBoundary(t *testing.T) {
	for pad := 0; pad < 4; pad++ {
		result := spawnedToolResult(t, multibyteTask(90+pad, 100))
		preview := taskFieldOf(t, result)

		if !cutInsideTheMultibyteRun(preview) {
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

func TestTheSpawnedTaskIsCutBeforeItReachesTheModel(t *testing.T) {
	short := strings.Repeat("s", 100)
	if got := taskFieldOf(t, spawnedToolResult(t, short)); got != short {
		t.Errorf("a %d-byte task is inside the budget and should reach the model whole; "+
			"it did not.\nPreview %q", len(short), got)
	}

	long := strings.Repeat("L", 5000)
	got := taskFieldOf(t, spawnedToolResult(t, long))

	// Contains, not equality. The call site is `Truncate(...) + "..."`, so a
	// mutation that stops the cut but keeps the marker yields `long + "..."` —
	// unequal to `long`, and an equality check calls that a pass. Measured on
	// this same scorer at server/spawn_delivery.go:214, where the equality form
	// was watched going red and still reported the site SURVIVED.
	if strings.Contains(got, long) {
		t.Errorf("the whole %d-byte task reached the model, so the cut at "+
			"spawn_tool.go:163 bounds nothing.\nPreview is %d bytes", len(long), len(got))
	}
}

// The seam the two cases above stand on. Without this, deleting
// Registry.listRegisteredAgents and going back to the package-level HTTP call
// would leave them green on this box — passing for the same accidental reason
// they were written to stop relying on.
func TestTheSpawnToolValidatesAgainstTheInjectedRegistry(t *testing.T) {
	asked := 0
	r := &Registry{listRegisteredAgents: func() []RegistryAgent {
		asked++
		return []RegistryAgent{{Name: "claxon", Orchestrator: "inber", Enabled: true}}
	}}

	raw, err := json.Marshal(map[string]string{"agent": "brigid", "task": "some task"})
	if err != nil {
		t.Fatalf("marshal the tool input: %v", err)
	}
	if _, err := r.SpawnAgentTool().Run(context.Background(), string(raw)); err == nil {
		t.Fatal("spawn_agent accepted an agent the injected registry does not list, " +
			"so it is not validating against the injected registry")
	} else if !strings.Contains(err.Error(), "claxon") {
		t.Errorf("the refusal does not name the agent the injected registry does list, "+
			"so the list the tool consulted is not the one supplied: %v", err)
	}
	if asked == 0 {
		t.Error("the spawn tool never asked the injected registry for its agent list")
	}
}
