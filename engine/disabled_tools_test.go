package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
)

func namedTools(names ...string) []agent.Tool {
	tools := make([]agent.Tool, len(names))
	for i, n := range names {
		tools[i] = agent.Tool{Name: n}
	}
	return tools
}

// enabledNames reads through the exported accessor deliberately: the tests
// then measure what a caller of this package can actually observe, not a field
// only this package can reach.
func enabledNames(e *Engine) []string { return e.EnabledToolNames() }

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestDisablingASecondToolDoesNotKeepTheFirstOneHidden is the defect this file
// was written for. SetDisabledTools filtered the tool slice it had already
// filtered, so every call subtracted from the previous answer instead of
// answering the request it was given. A caller that disabled "shell" and then
// asked for only "write_files" to be disabled got neither, and nothing said so.
func TestDisablingASecondToolDoesNotKeepTheFirstOneHidden(t *testing.T) {
	e := &Engine{}
	e.setToolSet(namedTools("read_files", "write_files", "shell"))

	e.SetDisabledTools([]string{"shell"})
	if got, want := enabledNames(e), []string{"read_files", "write_files"}; !equal(got, want) {
		t.Fatalf("after disabling shell: got %v, want %v", got, want)
	}

	e.SetDisabledTools([]string{"write_files"})
	if got, want := enabledNames(e), []string{"read_files", "shell"}; !equal(got, want) {
		t.Errorf("after asking for write_files alone to be disabled: got %v, want %v — "+
			"the set is cumulative, so shell stayed hidden and the request was not honoured", got, want)
	}
}

// TestAnEmptyDisabledSetRestoresEveryTool is the half that makes disabling
// reversible at all. Without it there is no request a caller can send that
// turns a tool back on, and a session that ever disabled one carries that
// hole until it ends.
func TestAnEmptyDisabledSetRestoresEveryTool(t *testing.T) {
	all := []string{"read_files", "write_files", "shell"}

	for _, empty := range [][]string{nil, {}} {
		e := &Engine{}
		e.setToolSet(namedTools(all...))
		e.SetDisabledTools([]string{"shell", "write_files"})

		e.SetDisabledTools(empty)
		if got := enabledNames(e); !equal(got, all) {
			t.Errorf("SetDisabledTools(%v) left %v, want %v — nothing re-enables a tool", empty, got, all)
		}
	}
}

// TestDisablingLeavesTheOrderTheModelAlreadySaw. The last tool definition
// carries the cache_control breakpoint (agent/agent_run.go), so the order of
// the surviving tools is load-bearing: a filter that reordered them would move
// the breakpoint and invalidate the cached prefix on every config call.
func TestDisablingLeavesTheOrderTheModelAlreadySaw(t *testing.T) {
	e := &Engine{}
	e.setToolSet(namedTools("a", "b", "c", "d"))

	e.SetDisabledTools([]string{"b"})
	if got, want := enabledNames(e), []string{"a", "c", "d"}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestDisablingANameNoToolHasChangesNothing. The set is a filter, not a
// registry: an unknown name is not an error and must not empty the wire.
func TestDisablingANameNoToolHasChangesNothing(t *testing.T) {
	e := &Engine{}
	e.setToolSet(namedTools("read_files", "shell"))

	e.SetDisabledTools([]string{"no_such_tool"})
	if got, want := enabledNames(e), []string{"read_files", "shell"}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestDisabledToolNamesReportsWhatWasTakenAway. EnabledToolNames cannot answer
// this: a tool is off the wire either because the agent config never asked for
// it or because a caller took it away, and only the second is a decision this
// session made. Anything that has to carry that decision past this process —
// the server's session sidecar — has to be able to read it back.
func TestDisabledToolNamesReportsWhatWasTakenAway(t *testing.T) {
	e := &Engine{}
	e.setToolSet(namedTools("read_files", "write_files", "shell", "web_fetch", "ripgrep"))

	if got := e.DisabledToolNames(); len(got) != 0 {
		t.Fatalf("a session that has disabled nothing reports %v, want nothing", got)
	}

	// Given in an order that is neither sorted nor reverse-sorted, and four
	// names deep, so an unsorted answer cannot pass by luck on a repeated run:
	// the set is a Go map, whose iteration order is deliberately random.
	e.SetDisabledTools([]string{"web_fetch", "shell", "ripgrep", "write_files"})
	if got, want := e.DisabledToolNames(), []string{"ripgrep", "shell", "web_fetch", "write_files"}; !equal(got, want) {
		t.Errorf("got %v, want %v — sorted, so a caller writing this to a file gets the same bytes for the same set", got, want)
	}

	e.SetDisabledTools(nil)
	if got := e.DisabledToolNames(); len(got) != 0 {
		t.Errorf("after re-enabling everything: got %v, want nothing", got)
	}
}

// TestDisabledToolNamesKeepsANameNoToolHas. SetDisabledTools documents an
// unknown name as not an error — the set is a filter, not a registry — so it
// survives, and subtracting the wire set from the full set would therefore not
// recover the set. That is why this reader exists rather than a derivation.
func TestDisabledToolNamesKeepsANameNoToolHas(t *testing.T) {
	e := &Engine{}
	e.setToolSet(namedTools("read_files"))

	e.SetDisabledTools([]string{"no_such_tool"})

	if got, want := e.DisabledToolNames(), []string{"no_such_tool"}; !equal(got, want) {
		t.Errorf("got %v, want %v — a name disabled before its tool was added must survive a rebuild that adds it", got, want)
	}
}

// TestAnEngineHandedOnlyItsWireSetCanStillDisable covers the engines built by
// hand rather than through initTools — the test helpers in this package, and
// anything future that assigns the tool slice directly. Without the fallback
// the first SetDisabledTools call would filter an empty full set and send no
// tools at all.
func TestAnEngineHandedOnlyItsWireSetCanStillDisable(t *testing.T) {
	e := &Engine{agentTools: namedTools("read_files", "shell")}

	e.SetDisabledTools([]string{"shell"})
	if got, want := enabledNames(e), []string{"read_files"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	e.SetDisabledTools(nil)
	if got, want := enabledNames(e), []string{"read_files", "shell"}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
