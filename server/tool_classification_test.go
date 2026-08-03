package server

import (
	"testing"

	"github.com/kayushkin/inber/guard"
)

// serverSuppliedToolNames is every name a tool that toolsForAgent injects
// actually answers to, derived from the constructors rather than restated here.
//
// Both branches are walked. toolsForAgent splits on whether the session's agent
// is the configured default, and the two halves offer different tools, so
// asking for one agent name would leave the other half of the set unexamined.
func serverSuppliedToolNames(t *testing.T) map[string]bool {
	t.Helper()

	server := &Server{config: Config{DefaultAgent: "the-orchestrator"}}

	names := make(map[string]bool)
	for _, agentName := range []string{"the-orchestrator", "some-other-agent"} {
		for _, tool := range server.toolsForAgent("agent:whoever:main", agentName) {
			names[tool.Name] = true
		}
	}
	return names
}

// classifyThroughVerdicts reports how the guard classifies a tool name, using
// only the package's exported surface.
//
// guard.isReadOnly and guard.isDangerous are unexported and this package cannot
// import them, but it does not need to: the two modes answer different
// questions about the same switch, and together they separate all three cases.
// Observe answers Allowed for exactly the read-only names and Denied for
// everything else. Assist with no approver answers NeedsApproval for exactly
// the dangerous names. A name that is neither read-only nor dangerous is
// therefore Denied in Observe and Allowed in Assist — which is the case this
// file exists for.
func classifyThroughVerdicts(name string) (readOnly, dangerous bool) {
	readOnly = guard.New(guard.Config{Mode: guard.Observe}).CheckTool(name, "{}") == guard.Allowed
	dangerous = guard.New(guard.Config{Mode: guard.Assist}).CheckTool(name, "{}") == guard.NeedsApproval
	return readOnly, dangerous
}

// TestEveryServerSuppliedToolIsClassifiedOrNamedHere closes a hole that
// guard/classification_test.go documents in prose and cannot assert.
//
// That file's TestEveryKnownToolIsClassifiedOrNamedHere is the completeness
// check over inber's tools: a tool the guard classifies as neither read-only
// nor dangerous is not "not yet reviewed", it is approved, because CheckTool's
// Assist branch diverts only the names isDangerous knows and lets everything
// else through. It claims that registering one more tool reddens it at the
// moment the tool is added.
//
// That claim holds only for tools its knownToolNames can reach, which is
// tool-store's registry plus the constructors it names. The tools this server
// injects into every session it creates reach the model by a different door —
// toolsForAgent to EngineConfig.ExtraTools to mergeExtraTools — and are
// invisible to it, because they are built by methods on *Server and package
// guard cannot import this one. There are seven, and they are listed below.
//
// So the check is written here instead, in the package that owns those tools,
// where the set can be derived rather than remembered.
//
// The set is not asserted to be empty. Classifying these is a policy call and
// not this test's to make; see the todo named in the exception list. What this
// test does is make the set closed, so an eighth server tool cannot join it
// silently.
func TestEveryServerSuppliedToolIsClassifiedOrNamedHere(t *testing.T) {
	// Every one of these is unclassified today, so Assist runs all of them with
	// no approval. The two sharpest are recorded so the next reader does not
	// have to re-derive them:
	//
	//   - "merge_workspace" rebases a sub-agent's spawn branch onto main and
	//     merges it, for every repo in the workspace. It is a write to the
	//     shared branch, and it is worth reading beside engine/workflow_git.go's
	//     refuseDefaultBranchPush, which exists precisely to stop a session
	//     pushing to the default branch on its own.
	//   - "spawn_agent" creates a child session, and a child is created from a
	//     zero RunRequest — so it inherits no mode and runs with every tool
	//     allowed. That makes this one not merely unclassified but an exit from
	//     the gate itself; TestAssistModeApprovalGateIsEscapableBySpawning below
	//     pins the whole path.
	unclassifiedToday := map[string]string{
		"agents_status":    "reads which agents are running; a read, but Observe denies it",
		"fix_workspace":    "sends a workspace back for rework; writes",
		"list_workspaces":  "reads the workspace list; a read, but Observe denies it",
		"merge_workspace":  "rebases and merges a spawn branch into main, per repo",
		"reject_workspace": "discards a workspace and the work in it",
		"spawn_agent":      "creates a child session that inherits no mode at all",
		"steer_agent":      "injects a message into another running session",
	}

	for name := range serverSuppliedToolNames(t) {
		readOnly, dangerous := classifyThroughVerdicts(name)

		if readOnly && dangerous {
			t.Errorf("%q is classified both read-only and dangerous — Observe reads isReadOnly first and Assist reads isDangerous first, so the two modes would disagree about the same tool", name)
			continue
		}

		_, named := unclassifiedToday[name]
		switch {
		case !readOnly && !dangerous && !named:
			t.Errorf("%q is a tool this server injects into every matching session, and the guard classifies it as neither read-only nor dangerous, so Assist mode runs it with no approval and nobody chose that. Classify it in guard, or add it here with the reason it is safe to leave unclassified", name)
		case (readOnly || dangerous) && named:
			t.Errorf("%q is now classified, so remove it from this test's list — a stale entry here silences the check for a name that no longer needs it", name)
		}
	}

	// The list must not outlive the tools it describes, or it goes on excusing
	// names nothing answers to. This is the rot that TestClassifiedToolsExist
	// caught in the classifiers themselves, arriving here by the same route.
	supplied := serverSuppliedToolNames(t)
	for name := range unclassifiedToday {
		if !supplied[name] {
			t.Errorf("this test names %q as a server-supplied tool, but toolsForAgent no longer produces it", name)
		}
	}
}

// TestAssistModeApprovalGateIsEscapableBySpawning states the whole path in one
// place, at the level of verdicts rather than classifiers.
//
// It asserts the CURRENT answers, so it pins the gap as present rather than
// pretending it is closed. Whoever decides that a spawned child should inherit
// its parent's mode, or that spawn_agent should be classified dangerous, will
// have to change this test — and that is the point: the change should be
// deliberate and visible, not a green diff nobody read.
func TestAssistModeApprovalGateIsEscapableBySpawning(t *testing.T) {
	approvalsAsked := 0
	parent := guard.New(guard.Config{Mode: guard.Assist, ApprovalFunc: func(tool, input string) bool {
		approvalsAsked++
		return false
	}})

	// The gate works, for the names it knows.
	if verdict := parent.CheckTool("shell_commands", "{}"); verdict != guard.NeedsApproval {
		t.Fatalf("CheckTool(\"shell_commands\") in Assist = %v, want NeedsApproval — the gate this test is about is not up", verdict)
	}
	if approvalsAsked != 1 {
		t.Fatalf("the approver was asked %d times about a classified dangerous tool, want 1", approvalsAsked)
	}

	// Step one of the escape: spawning is not a name isDangerous knows, so it
	// is allowed and the approver is never consulted.
	if verdict := parent.CheckTool("spawn_agent", `{"agent":"brigid","task":"anything"}`); verdict != guard.Allowed {
		t.Errorf("CheckTool(\"spawn_agent\") in Assist = %v, want Allowed — if this now diverts, spawn_agent was classified and TestEveryServerSuppliedToolIsClassifiedOrNamedHere should have said so first", verdict)
	}
	if approvalsAsked != 1 {
		t.Errorf("the approver was asked %d times in total, want 1 — an unclassified tool must not reach it, so a second call here means spawn_agent is now classified", approvalsAsked)
	}

	// Step two: the child. server.createSession builds engine.EngineConfig
	// without a Mode field, and applyRequestOverrides copies one only when the
	// request carries it. Both spawn and fork call createSession with a zero
	// RunRequest, so the child's configured mode is the empty string, which
	// guard.ParseMode resolves to Unset and CheckTool's default branch allows
	// everything through.
	childMode, err := guard.ParseMode("")
	if err != nil {
		t.Fatalf("ParseMode(\"\") returned an error: %v", err)
	}
	if childMode != guard.Unset {
		t.Fatalf("ParseMode(\"\") = %v, want Unset", childMode)
	}

	child := guard.New(guard.Config{Mode: childMode})
	if verdict := child.CheckTool("shell_commands", `{"command":"echo escaped"}`); verdict != guard.Allowed {
		t.Errorf("the spawned child's guard answered %v for shell_commands, want Allowed — this test pins the escape as present, so if the child is now gated the fix landed and this test should record it", verdict)
	}
}
