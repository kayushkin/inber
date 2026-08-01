package engine

import "testing"

// The read cache identifies a file by the path the tools will open, which it
// can only do if it is given the same root setToolSet resolves the tools
// against. That handover is one line in buildAgent, and every test that pins
// the identity rule itself lives in package agent and passes with the line
// deleted — so the line needs its own test here, where the root is.
func TestBuildAgentGivesTheReadCacheTheRootTheToolsResolveAgainst(t *testing.T) {
	root := t.TempDir()
	e := &Engine{repoRoot: root}

	a := e.buildAgent(nil)

	if got := a.RepoRoot(); got != root {
		t.Fatalf("agent root = %q, want %q — the read cache resolves relative paths against a "+
			"different root than the tools do, so a relative read and an absolute write are two entries", got, root)
	}
}
