package server

import (
	"testing"

	"github.com/kayushkin/inber/agent"
)

// TestEveryServerSuppliedToolIsNamedByTheReadCacheRules is the read-cache twin
// of TestEveryServerSuppliedToolIsClassifiedOrNamedHere.
//
// agent.ReadCacheEffect answers ReadCacheUnaffected for any name it does not
// know, so a tool nobody classified is treated as writing nothing, and after it
// runs the model can be told it already has a file the tool just rewrote.
// tools/read_cache_classification_test.go holds that default to account for
// every tool in tool-store's registry and the tools package, but the tools this
// server injects reach the model by another door — toolsForAgent to
// EngineConfig.ExtraTools to mergeExtraTools — and that test cannot see them,
// because package tools cannot import this one.
//
// So the check lives here, and it does not pick a bucket for the tools that
// write. Which bucket is a decision with a real cost — see the reason on each
// entry below — and it belongs to todo 36f6c3e3-e59b-4936-8477-4dff72fa69db.
// What this test does is make the set closed: an eighth server tool cannot
// inherit "writes nothing" by being forgotten.
func TestEveryServerSuppliedToolIsNamedByTheReadCacheRules(t *testing.T) {
	// writesNothing are the server tools that write no file the read cache can
	// hold. Checked against the implementations 2026-09-26: agents_status reads
	// agent-store's status rows and list_workspaces reads forge's workspace
	// list; neither touches the working tree.
	writesNothing := map[string]string{
		"agents_status":   "reads agent-store's status rows",
		"list_workspaces": "reads forge's workspace list",
	}

	// awaitingReadCacheDecision are the server tools that can change files the
	// calling session has read, which the read cache does not know about. Each
	// answers ReadCacheUnaffected today, and that answer is wrong for all of
	// them; which answer is right is todo 36f6c3e3's question. The obvious
	// bucket, ReadCacheEverything, costs an orchestrator a full re-read after
	// every spawn, and the other answer is that a child should never write the
	// parent's tree at all.
	awaitingReadCacheDecision := map[string]string{
		"spawn_agent":      "the child's writes run concurrently with the caller's turn, in the caller's tree whenever useWorkspace does not run",
		"steer_agent":      "sets a running child to work, which writes wherever that child writes",
		"merge_workspace":  "rebases and merges a spawn branch into main, per repo",
		"reject_workspace": "discards a workspace and the work in it",
		"fix_workspace":    "re-spawns an agent into an existing workspace, which then writes there",
	}

	supplied := serverSuppliedToolNames(t)
	for name := range supplied {
		_, safe := writesNothing[name]
		_, awaiting := awaitingReadCacheDecision[name]
		effect := agent.ReadCacheEffect(name)

		switch {
		case safe && awaiting:
			t.Errorf("%q is in both lists in this test; it belongs in one", name)
		case safe && effect != agent.ReadCacheUnaffected:
			t.Errorf("%q: the read cache says %q, and this test says it writes nothing", name, effect)
		case awaiting && effect != agent.ReadCacheUnaffected:
			t.Errorf("%q is now classified %q by the read cache, so remove it from awaitingReadCacheDecision — a stale entry here excuses a name that no longer needs it", name, effect)
		case !safe && !awaiting && effect == agent.ReadCacheUnaffected:
			t.Errorf("%q is a tool this server injects, and the read cache treats it as writing nothing because it has never heard of it. If it writes files the caller may have read, classify it in agent/read_cache.go; if it writes nothing, add it to writesNothing here with the reason", name)
		}
	}

	// A list that outlives its tools goes on excusing names nothing answers to.
	for _, list := range []map[string]string{writesNothing, awaitingReadCacheDecision} {
		for name := range list {
			if !supplied[name] {
				t.Errorf("this test names %q as a server-supplied tool, but toolsForAgent no longer produces it", name)
			}
		}
	}
}
