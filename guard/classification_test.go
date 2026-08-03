package guard

import (
	"testing"

	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// knownToolNames is every name a tool in this process actually answers to,
// derived from the tools themselves rather than restated here.
func knownToolNames(t *testing.T) map[string]bool {
	t.Helper()

	names := make(map[string]bool)

	// tool-store auto-registers its zero-argument tools.
	for _, impl := range toolstoretools.All() {
		names[impl.Name] = true
	}

	// The rest are constructed on demand: tool-store's context-taking tools and
	// inber's own memory/deploy tools. Only Name is read, so the throwaway
	// arguments are never used and the nil store is never dereferenced. Naming
	// them via their constructors rather than restating the strings is the point
	// — a renamed tool must move this set, not slip past it.
	//
	// tool-store's register.go deliberately does not auto-register the
	// constructors that take arguments, so All() above cannot return any of
	// them and every one has to be named here. This list picked up repo_map and
	// recent_files from that carve-out and missed task_plan and scratchpad,
	// which are in it for the same reason — so those two reached the model, via
	// Engine.buildSpecialTool, entirely outside this test's view.
	for _, name := range []string{
		tools.RepoMap("", nil).Name,
		tools.RecentFiles("").Name,
		tools.TaskPlan("").Name,
		tools.Scratchpad("", "").Name,
		tools.Deploy().Name,
		memory.SearchTool(nil).Name,
		memory.SaveTool(nil).Name,
		memory.ExpandTool(nil).Name,
		memory.ForgetTool(nil).Name,
	} {
		names[name] = true
	}
	return names
}

// TestClassifiedToolsExist asserts that every tool name the guard classifies is
// a name some tool actually has.
//
// This is the check that was missing. isReadOnly and isDangerous were written
// against the pre-tool-store names, and when tool-store renamed the tools
// (read_file -> read_files, write_file -> write_files, edit_file -> edit_files)
// the classifiers kept matching on names nothing answered to. A name that
// matches no tool fails open in isDangerous: Assist mode routes dangerous calls
// through ApprovalFunc, so write_files and edit_files would have been treated as
// safe and executed with no approval gate at all.
//
// This comment used to end "the classifiers are unreachable today — CheckTool
// has no caller and the engine hardcodes Autonomous mode". Both halves are now
// false: Engine.buildToolRefusal (engine/build_hooks.go) is CheckTool's caller
// on every tool call, and engine.NewEngine parses the mode the create request
// asked for. A session created with mode "assist" is gated for real, so a
// misclassification is live rather than latent.
func TestClassifiedToolsExist(t *testing.T) {
	known := knownToolNames(t)

	classified := map[string][]string{
		"isReadOnly": {
			"read_files", "list_files", "ripgrep", "memory_expand", "memory_search",
			"repo_map", "recent_files", "web_search",
		},
		"isDangerous": {
			"shell_commands", "write_files", "edit_files", "deploy",
		},
	}

	for fn, toolNames := range classified {
		for _, name := range toolNames {
			if !known[name] {
				t.Errorf("%s classifies %q, but no tool has that name — the case can never match, so the tool it was meant to cover is unclassified", fn, name)
			}
		}
	}

	// The classifier lists above must stay in step with the switches themselves.
	for _, name := range classified["isReadOnly"] {
		if !isReadOnly(name) {
			t.Errorf("isReadOnly(%q) = false, but this test lists it as read-only", name)
		}
	}
	for _, name := range classified["isDangerous"] {
		if !isDangerous(name) {
			t.Errorf("isDangerous(%q) = false, but this test lists it as dangerous", name)
		}
	}
}

// TestWriteToolsNeedApprovalInAssistMode drives the actual verdict, not the
// classifier, because that is the property that matters: a write must not reach
// the model unapproved. With the pre-rename names this passed Allowed straight
// back for every write tool.
func TestWriteToolsNeedApprovalInAssistMode(t *testing.T) {
	g := New(Config{Mode: Assist}) // no ApprovalFunc — nothing can auto-approve

	for _, tool := range []string{"shell_commands", "write_files", "edit_files", "deploy"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != NeedsApproval {
			t.Errorf("CheckTool(%q) in Assist = %v, want NeedsApproval — a write tool that is not classified dangerous executes with no approval gate", tool, verdict)
		}
	}

	// Reads still pass straight through; the gate is for writes.
	for _, tool := range []string{"read_files", "list_files"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Allowed {
			t.Errorf("CheckTool(%q) in Assist = %v, want Allowed", tool, verdict)
		}
	}
}

// TestObserveModeAllowsTheRealReadTools pins the other side: in Observe mode a
// read tool whose name the classifier no longer recognises is Denied, which
// would have made Observe mode useless rather than unsafe.
func TestObserveModeAllowsTheRealReadTools(t *testing.T) {
	g := New(Config{Mode: Observe})

	for _, tool := range []string{"read_files", "list_files", "memory_search"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Allowed {
			t.Errorf("CheckTool(%q) in Observe = %v, want Allowed", tool, verdict)
		}
	}
	for _, tool := range []string{"write_files", "shell_commands"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Denied {
			t.Errorf("CheckTool(%q) in Observe = %v, want Denied", tool, verdict)
		}
	}
}

// TestEveryKnownToolIsClassifiedOrNamedHere is the converse of
// TestClassifiedToolsExist, and it is the direction that fails open.
//
// TestClassifiedToolsExist asks whether every name the guard classifies belongs
// to a real tool. It cannot see the opposite mistake: a real tool the guard
// classifies as neither read-only nor dangerous. Observe refuses such a tool,
// which is safe. Assist runs it with no approval, because CheckTool's Assist
// branch only diverts the names isDangerous knows and lets everything else
// through — so an unclassified tool is not "not yet reviewed", it is approved.
//
// Eight tools are in that position today and this test names all eight, so the
// set can only change deliberately. Registering a ninth tool reddens this test
// at the moment the tool is added, which is the moment somebody can still answer
// "should assist mode run this unattended?" while the answer is cheap.
//
// The set is not asserted to be empty, because emptying it is a policy call and
// not this test's to make. Three of the eight are the sharp ones and are
// recorded here so the next reader does not have to re-derive them:
//
//   - "scheduler" creates and deletes cron jobs, and this host's scheduler runs
//     shell commands. An assist session can therefore schedule shell work it is
//     not allowed to run directly, and nothing asks an approver.
//   - "memory_forget" deletes memories. Memory is where an agent's identity and
//     instructions live (see engine.BuildSystemPrompt), so this is a write in
//     the strongest sense while being classified as neither kind.
//   - "scratchpad" and "task_plan" write files under the repo root AND their
//     contents are injected into the system context every turn
//     (engine.registerToolContextInjectors). That is the hazard already noted
//     for memory_save — a write later turns read back as instructions — by a
//     second route.
//
// This test can only see the tools knownToolNames can construct, which is
// tool-store's registry plus the constructors named there. It cannot see the
// tools the server injects through EngineConfig.ExtraTools — spawn_agent,
// steer_agent, agents_status and the four workspace tools — because those are
// built by methods on *Server, which this package cannot import. That set is
// checked by TestEveryServerSuppliedToolIsClassifiedOrNamedHere in package
// server, and the sharpest of them, spawn_agent, is an exit from this gate
// rather than merely a hole in it: a spawned child is created from a zero
// RunRequest, so it inherits no mode at all.
func TestEveryKnownToolIsClassifiedOrNamedHere(t *testing.T) {
	unclassifiedToday := map[string]string{
		"browser":       "drives a real browser; reads pages but also clicks and types",
		"end_turn":      "control flow, not an effect — reaches no model today either",
		"memory_forget": "deletes memories, including the ones carrying identity",
		"memory_save":   "writes a memory that later turns read back as instructions",
		"scheduler":     "creates cron jobs, and cron jobs on this host run shell",
		"scratchpad":    "writes a file under the repo root that is injected into every later turn",
		"task_plan":     "writes a plan file under the repo root that is injected into every later turn",
		"web_fetch":     "network read; note web_search IS classified read-only",
	}

	for name := range knownToolNames(t) {
		readOnly, dangerous := isReadOnly(name), isDangerous(name)

		if readOnly && dangerous {
			t.Errorf("%q is classified both read-only and dangerous — CheckTool reads isReadOnly first in Observe and isDangerous first in Assist, so the two modes would disagree about the same tool", name)
			continue
		}

		_, named := unclassifiedToday[name]
		switch {
		case !readOnly && !dangerous && !named:
			t.Errorf("%q is classified as neither read-only nor dangerous and is not named in this test, so assist mode runs it with no approval and nobody chose that. Classify it in isReadOnly or isDangerous, or add it here with the reason it is safe to leave unclassified", name)
		case (readOnly || dangerous) && named:
			t.Errorf("%q is now classified, so remove it from this test's list — a stale entry here silences the check for a name that no longer needs it", name)
		}
	}

	// The list must not outlive the tools it describes, or it goes on excusing
	// names nothing answers to — the same rot TestClassifiedToolsExist caught in
	// the classifiers themselves.
	known := knownToolNames(t)
	for name := range unclassifiedToday {
		if !known[name] {
			t.Errorf("this test names %q as a known-but-unclassified tool, but no tool has that name", name)
		}
	}
}

// TestAssistRunsAnUnclassifiedToolWithNoApproval pins the behaviour the list
// above describes, at the level that matters — the verdict, not the classifier.
//
// It asserts the current answer, which is Allowed, so it pins the gap as
// present rather than pretending it is closed. Whoever classifies these tools
// will have to change this test, and that is the point: the change should be
// deliberate and visible, not a green diff nobody read.
func TestAssistRunsAnUnclassifiedToolWithNoApproval(t *testing.T) {
	approvalsAsked := 0
	g := New(Config{Mode: Assist, ApprovalFunc: func(tool, input string) bool {
		approvalsAsked++
		return false
	}})

	if verdict := g.CheckTool("scheduler", `{"action":"create"}`); verdict != Allowed {
		t.Errorf("CheckTool(\"scheduler\") in Assist = %v, want Allowed — if this now diverts, the tool was classified and TestEveryKnownToolIsClassifiedOrNamedHere should have said so first", verdict)
	}
	if approvalsAsked != 0 {
		t.Errorf("the approver was asked %d times about an unclassified tool; Assist only consults it for names isDangerous knows", approvalsAsked)
	}

	// The contrast, in one place: same guard, same approver, a classified tool.
	if verdict := g.CheckTool("shell_commands", "{}"); verdict != NeedsApproval {
		t.Errorf("CheckTool(\"shell_commands\") in Assist = %v, want NeedsApproval", verdict)
	}
	if approvalsAsked != 1 {
		t.Errorf("the approver was asked %d times about a classified dangerous tool, want 1", approvalsAsked)
	}
}
