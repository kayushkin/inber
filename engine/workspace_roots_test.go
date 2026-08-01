package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

func threeRoots() []WorkspaceRoot {
	return []WorkspaceRoot{
		{Name: "inber", Path: "/forge/work/brigid-1/inber", Primary: true},
		{Name: "forge", Path: "/forge/work/brigid-1/forge"},
		{Name: "agent-store", Path: "/forge/work/brigid-1/agent-store"},
	}
}

// The defect this file closes: forge checks out every repository in the
// workspace, commits every one of them and pushes every one of them, and the
// model used to be told about exactly one.
func TestEveryRepositoryInTheWorkspaceIsNamed(t *testing.T) {
	rendered := renderWorkspaceRoots(threeRoots())
	for _, root := range threeRoots() {
		if !strings.Contains(rendered, root.Path) {
			t.Errorf("%s (%s) is checked out and is not named in:\n%s", root.Name, root.Path, rendered)
		}
	}
}

// Naming the repositories is only half of it. Relative paths resolve against
// the primary root, so which root is primary decides what a bare "server/x.go"
// means, and the model cannot know it without being told.
func TestThePrimaryRepositoryIsMarkedAndComesFirst(t *testing.T) {
	lines := strings.Split(renderWorkspaceRoots(threeRoots()), "\n")

	var listed []string
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			listed = append(listed, line)
		}
	}
	if len(listed) != 3 {
		t.Fatalf("want 3 repositories listed, got %d:\n%s", len(listed), strings.Join(lines, "\n"))
	}
	if !strings.Contains(listed[0], "inber") || !strings.Contains(listed[0], "(primary)") {
		t.Errorf("the primary repository should be marked and listed first, got %q", listed[0])
	}
	for _, line := range listed[1:] {
		if strings.Contains(line, "(primary)") {
			t.Errorf("only one repository is primary, and %q is also marked", line)
		}
	}
}

// forge.Workspace.Repos is a map, so the caller reads the roots in whatever
// order the runtime hands them over. Rendering them in that order would give
// the model a different prompt on every session with no change in meaning.
func TestTheRenderingDoesNotDependOnTheOrderTheRootsArrivedIn(t *testing.T) {
	forwards := renderWorkspaceRoots(threeRoots())

	reversed := threeRoots()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if backwards := renderWorkspaceRoots(reversed); backwards != forwards {
		t.Errorf("the roots rendered differently when they arrived in a different order:\n%s\n---\n%s", forwards, backwards)
	}
}

// A single-repository session has never been told where it is working and does
// not need to be — its tools already resolve relative paths there. Saying it
// anyway would add bytes to the last user message of every such session, and
// those bytes persist into the conversation and into the prefix a later turn
// caches.
func TestASingleRepositorySessionsPromptBytesDoNotMove(t *testing.T) {
	for _, roots := range [][]WorkspaceRoot{
		nil,
		{},
		{{Name: "inber", Path: "/forge/work/brigid-1/inber", Primary: true}},
	} {
		if rendered := renderWorkspaceRoots(roots); rendered != "" {
			t.Errorf("%d root(s) rendered %q, want nothing at all", len(roots), rendered)
		}
	}
}

// Naming a repository in the prompt is only useful if the tools accept the path
// that was named. tools.ScopeToRoot resolves a relative path against the
// primary root and passes an absolute path through as written, so the roots are
// rendered absolute — and this is the test that the two halves are one
// contract. Rendering a path the tools would have re-rooted would hand the
// model a location it cannot reach, which is the shipped defect one layer up.
func TestAPathTakenFromTheRenderingReachesTheToolUnchanged(t *testing.T) {
	roots := threeRoots()
	rendered := renderWorkspaceRoots(roots)

	var secondaryRoot string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, "- **forge**") {
			_, path, _ := strings.Cut(line, "— ")
			secondaryRoot = path
		}
	}
	if secondaryRoot == "" {
		t.Fatalf("no path for the secondary repository in:\n%s", rendered)
	}

	var recorded string
	e := &Engine{repoRoot: PrimaryWorkspaceRoot(roots), workspaceRoots: roots}
	e.setToolSet([]agent.Tool{toolRecordingItsArguments("write_files", &recorded)})

	wanted := secondaryRoot + "/workspace.go"
	arguments, err := json.Marshal(map[string]string{"path": wanted, "content": "package forge"})
	if err != nil {
		t.Fatal(err)
	}
	runInstalledTool(t, e, "write_files", string(arguments))

	var got struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(recorded), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != wanted {
		t.Errorf("the model was told to write %s and the tool was run against %s", wanted, got.Path)
	}
}

// The delivery channel: the roots have to reach the turn the model actually
// runs, not just render correctly on their own. They go in the volatile
// context, which is injected after the cache boundary, and they are queued
// because BuildSystemPrompt assigns that field wholesale.
func TestTheTurnTellsTheModelAboutEveryRoot(t *testing.T) {
	e := &Engine{repoRoot: "/forge/work/brigid-1/inber", workspaceRoots: threeRoots()}

	if _, err := e.buildTurnContext("change both repositories"); err != nil {
		t.Fatal(err)
	}

	for _, root := range threeRoots() {
		if !strings.Contains(e.Turn.VolatileContext, root.Path) {
			t.Errorf("the turn does not mention %s:\n%s", root.Path, e.Turn.VolatileContext)
		}
	}
}

// A note queued during context preparation must survive the roots being queued
// too — both end up in one turn's volatile context, and neither overwrites the
// other.
func TestTheRootsDoNotDisplaceTheNotesQueuedBeforeThem(t *testing.T) {
	e := &Engine{repoRoot: "/forge/work/brigid-1/inber", workspaceRoots: threeRoots()}
	e.queueVolatileNote("[Note: these files were re-read since last context snapshot]")

	if _, err := e.buildTurnContext("change both repositories"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(e.Turn.VolatileContext, "re-read since last context snapshot") {
		t.Errorf("the queued note is gone:\n%s", e.Turn.VolatileContext)
	}
	if !strings.Contains(e.Turn.VolatileContext, "/forge/work/brigid-1/forge") {
		t.Errorf("the roots are gone:\n%s", e.Turn.VolatileContext)
	}
}

// A session outside a forge workspace has one repository and passes no roots.
// That is the ordinary case, not a broken one.
func TestNoRootsAtAllIsNotAnError(t *testing.T) {
	if err := validateWorkspaceRoots(nil, "/repos/inber"); err != nil {
		t.Errorf("a session with no workspace was refused: %v", err)
	}
}

// The primary path and the session's repo root are one fact arriving by two
// routes: the prompt is built from the first and every filesystem tool is
// rooted at the second. Letting them disagree would tell the model it is
// working somewhere its tools do not write.
func TestASessionIsRefusedWhenItsPromptAndItsToolsDisagree(t *testing.T) {
	roots := threeRoots()
	err := validateWorkspaceRoots(roots, "/repos/inber")
	if err == nil {
		t.Fatal("a session rooted somewhere other than its primary repository was allowed")
	}
	if !strings.Contains(err.Error(), "/forge/work/brigid-1/inber") || !strings.Contains(err.Error(), "/repos/inber") {
		t.Errorf("the error names neither of the two paths that disagree: %v", err)
	}
}

func TestAWorkspaceMustHaveExactlyOnePrimaryRepository(t *testing.T) {
	twoPrimaries := threeRoots()
	twoPrimaries[1].Primary = true
	if err := validateWorkspaceRoots(twoPrimaries, "/forge/work/brigid-1/inber"); err == nil {
		t.Error("two primary repositories were allowed")
	}

	noPrimary := threeRoots()
	noPrimary[0].Primary = false
	if err := validateWorkspaceRoots(noPrimary, "/forge/work/brigid-1/inber"); err == nil {
		t.Error("a workspace with no primary repository was allowed")
	}
}

func TestARootMustHaveBothANameAndAPath(t *testing.T) {
	unnamed := threeRoots()
	unnamed[1].Name = ""
	if err := validateWorkspaceRoots(unnamed, "/forge/work/brigid-1/inber"); err == nil {
		t.Error("a repository with no name was allowed")
	}

	pathless := threeRoots()
	pathless[1].Path = ""
	if err := validateWorkspaceRoots(pathless, "/forge/work/brigid-1/inber"); err == nil {
		t.Error("a repository with no worktree path was allowed")
	}
}

// The check has to run where a session is actually built, not only when it is
// called directly — NewEngine is where a mismatch would otherwise become a
// running session.
func TestNewEngineRefusesAWorkspaceItsRootDisagreesWith(t *testing.T) {
	_, err := NewEngine(context.Background(), EngineConfig{
		RepoRoot:       "/repos/inber",
		WorkspaceRoots: threeRoots(),
		Raw:            true,
	})
	if err == nil {
		t.Fatal("NewEngine built a session whose prompt and tools point at different repositories")
	}
	if !strings.Contains(err.Error(), "workspace roots") {
		t.Errorf("NewEngine failed for some other reason: %v", err)
	}
}
