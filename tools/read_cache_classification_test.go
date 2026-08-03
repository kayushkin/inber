package tools

import (
	"sort"
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// The read cache invalidates on a tool name. A name it does not recognise is
// treated as writing nothing, so a tool that writes files and is not named here
// leaves stale entries behind and the model is told not to re-read them. That
// is a fail-open default, and the only thing that stops it is a test which
// notices a tool nobody classified.
//
// This is guard's TestClassifiedToolsExist rule applied to the other classifier
// in the tree. It lives in this package rather than in agent because agent
// cannot import tools — tools imports agent — the same reason
// server/tool_classification_test.go lives in server.
//
// Adding a tool to the registry without adding it below turns this red. That is
// the whole point: pick a bucket deliberately, do not inherit "none" by
// forgetting.

// everyToolName is every name a tool in this process answers to, taken from the
// tools themselves rather than restated. Kept deliberately parallel to
// guard.knownToolNames — if that list grows, this one has the same gap.
func everyToolName(t *testing.T) []string {
	t.Helper()

	seen := make(map[string]bool)

	// tool-store auto-registers its zero-argument tools.
	for _, impl := range toolstoretools.All() {
		seen[impl.Name] = true
	}

	// The argument-taking constructors are not auto-registered, so each has to
	// be named. Only Name is read; the throwaway arguments are never used and
	// the nil store is never dereferenced.
	for _, name := range []string{
		RepoMap("", nil).Name,
		RecentFiles("").Name,
		TaskPlan("").Name,
		Scratchpad("", "").Name,
		Deploy().Name,
		memory.SearchTool(nil).Name,
		memory.SaveTool(nil).Name,
		memory.ExpandTool(nil).Name,
		memory.ForgetTool(nil).Name,
	} {
		seen[name] = true
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// writesNothing are the tools that create and modify no file the read cache can
// hold. Verified against the implementations 2026-08-03: none of them calls
// os.WriteFile, os.Create, os.Rename, os.Remove or exec.Command. The memory
// tools write to SQLite, not to the working tree.
var writesNothing = map[string]bool{
	"read_files":    true,
	"list_files":    true,
	"ripgrep":       true,
	"repo_map":      true,
	"recent_files":  true,
	"web_search":    true,
	"web_fetch":     true,
	"browser":       true,
	"scheduler":     true,
	"end_turn":      true,
	"memory_search": true,
	"memory_save":   true,
	"memory_expand": true,
	"memory_forget": true,
}

// writesNamedPaths are the tools whose input names every path they write, so
// the cache can invalidate precisely.
var writesNamedPaths = map[string]bool{
	"write_files": true,
	"edit_files":  true,
}

// writesUnnamedPaths are the tools that can write files their input does not
// name. Each is here for a measured reason, not a guess:
//
//   - shell_commands runs an arbitrary command (tool-store tools/shell.go).
//   - deploy posts to /api/forge/deploy, which its own description says
//     "Commits any uncommitted changes first" and which builds the slot in
//     another process against this working tree (tools/deploy.go).
//   - task_plan os.WriteFiles planPath(repoRoot) and reaches RunBuildCheck ->
//     a subprocess when the plan empties (tool-store tools/task_plan.go:205,
//     :329).
//   - scratchpad os.WriteFiles scratchpadPath(repoRoot, agentName)
//     (tool-store tools/scratchpad.go:129).
//
// task_plan and scratchpad each write one derivable path, so a precise rule is
// imaginable for them. They are here anyway: the path comes from the engine's
// repoRoot rather than from the call, so the cache would have to be taught a
// second way of knowing what a call touched to gain back one re-read.
var writesUnnamedPaths = map[string]bool{
	"shell_commands": true,
	"deploy":         true,
	"task_plan":      true,
	"scratchpad":     true,
}

func TestEveryToolIsNamedByTheReadCacheInvalidationRules(t *testing.T) {
	for _, name := range everyToolName(t) {
		classified := writesNothing[name] || writesNamedPaths[name] || writesUnnamedPaths[name]
		if !classified {
			t.Errorf("tool %q is in the registry and in none of the three buckets in this file. "+
				"It defaults to %q, which means the read cache will keep answering reads of any "+
				"file it writes with a stale stub. Decide which bucket it belongs in.",
				name, agent.ReadCacheEffect(name))
		}
	}
}

// TestTheReadCacheAgreesWithThisFile checks the buckets against the classifier
// itself, so the two cannot drift apart silently.
func TestTheReadCacheAgreesWithThisFile(t *testing.T) {
	for name := range writesNothing {
		if got := agent.ReadCacheEffect(name); got != agent.ReadCacheUnaffected {
			t.Errorf("%q: read cache says %q, this file says it writes nothing", name, got)
		}
	}
	for name := range writesNamedPaths {
		if got := agent.ReadCacheEffect(name); got != agent.ReadCacheNamedPaths {
			t.Errorf("%q: read cache says %q, this file says it writes the paths it names", name, got)
		}
	}
	for name := range writesUnnamedPaths {
		if got := agent.ReadCacheEffect(name); got != agent.ReadCacheEverything {
			t.Errorf("%q: read cache says %q, this file says it can write anything", name, got)
		}
	}
}

// TestTheBucketsNameOnlyRealTools is the other direction, and it is the one
// guard learned the hard way: a name no tool answers to is not a harmless
// spare, it is a rule that silently stopped matching. guard.isDangerous lost
// write_files that way when tool-store renamed write_file.
func TestTheBucketsNameOnlyRealTools(t *testing.T) {
	real := make(map[string]bool)
	for _, name := range everyToolName(t) {
		real[name] = true
	}
	for _, bucket := range []map[string]bool{writesNothing, writesNamedPaths, writesUnnamedPaths} {
		for name := range bucket {
			if !real[name] {
				t.Errorf("bucket names %q, which no tool answers to — either it was renamed "+
					"and this rule is now dead, or it never existed", name)
			}
		}
	}
}
