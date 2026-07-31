package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// Checking off the last task in the plan fires the project's build command. A
// build that was stopped has judged nothing about the code, so reading it as a
// failure writes a "Fix build error" task describing a build that never
// finished — and the agent then spends its next turn hunting a build error that
// does not exist.
func TestStoppedBuildLeavesNoBuildErrorTask(t *testing.T) {
	repoRoot := repoWithOneRemainingTask(t)
	defer useBuildCommand(t, "sleep 60")()

	turnContext, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	callbacks := (&Engine{repoRoot: repoRoot, AgentName: "test"}).buildSidebandCallbacks()
	if err := callbacks.CompleteTasks(turnContext, []int{0}); err != nil {
		t.Fatalf("CompleteTasks: %v", err)
	}

	if plan := toolstoretools.LoadPlanContext(repoRoot); strings.Contains(plan, "Fix build error") {
		t.Errorf("a stopped build left a build-error task behind:\n%s", plan)
	}
}

// A build that genuinely fails must still file the fix task, so the assertion
// above cannot pass by never filing one at all.
func TestFailedBuildStillLeavesABuildErrorTask(t *testing.T) {
	repoRoot := repoWithOneRemainingTask(t)
	defer useBuildCommand(t, "echo 'undefined: Foo' >&2; exit 2")()

	callbacks := (&Engine{repoRoot: repoRoot, AgentName: "test"}).buildSidebandCallbacks()
	if err := callbacks.CompleteTasks(context.Background(), []int{0}); err != nil {
		t.Fatalf("CompleteTasks: %v", err)
	}

	if plan := toolstoretools.LoadPlanContext(repoRoot); !strings.Contains(plan, "Fix build error") {
		t.Errorf("a failing build filed no fix task:\n%s", plan)
	}
}

// Completing the last task must not take longer than the interrupt it is
// supposed to honour.
func TestCompletingTheLastTaskStopsWhenTheTurnIsCancelled(t *testing.T) {
	repoRoot := repoWithOneRemainingTask(t)
	defer useBuildCommand(t, "sleep 120")()

	turnContext, cancel := context.WithCancel(context.Background())
	callbacks := (&Engine{repoRoot: repoRoot, AgentName: "test"}).buildSidebandCallbacks()

	returned := make(chan struct{})
	go func() {
		_ = callbacks.CompleteTasks(turnContext, []int{0})
		close(returned)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-returned:
	case <-time.After(15 * time.Second):
		t.Fatal("completing the last task ignored the cancelled turn and sat on the build")
	}
}

func repoWithOneRemainingTask(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	plan := &toolstoretools.TaskPlan{
		Tasks: []toolstoretools.TaskItem{{Task: "the only task", Status: toolstoretools.TaskPending}},
	}
	if err := toolstoretools.SavePlanMD(repoRoot, plan); err != nil {
		t.Fatalf("could not seed the plan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".task.md")); err != nil {
		t.Fatalf("the seeded plan is not on disk: %v", err)
	}
	return repoRoot
}

func useBuildCommand(t *testing.T, command string) (restore func()) {
	t.Helper()
	previous := toolstoretools.TaskPlanBuildCommand
	toolstoretools.TaskPlanBuildCommand = command
	return func() { toolstoretools.TaskPlanBuildCommand = previous }
}
