package server

import (
	"strings"
	"testing"

	"github.com/kayushkin/forge"
)

func threeRepoWorkspace() *forge.Workspace {
	return &forge.Workspace{
		ID:      "brigid-1",
		Primary: "inber",
		Repos: map[string]string{
			"inber":       "/forge/work/brigid-1/inber",
			"forge":       "/forge/work/brigid-1/forge",
			"agent-store": "/forge/work/brigid-1/agent-store",
		},
	}
}

// The defect: both callers kept ws.Repos[ws.Primary] and dropped the rest of
// the map, while forge went on committing, merging and pushing all of it.
func TestEveryRepositoryOfTheWorkspaceBecomesARoot(t *testing.T) {
	roots, err := workspaceRoots(threeRepoWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 3 {
		t.Fatalf("the workspace holds 3 repositories and produced %d roots: %+v", len(roots), roots)
	}
	if !roots[0].Primary || roots[0].Name != "inber" {
		t.Errorf("the primary repository should come first and be marked, got %+v", roots[0])
	}
	for _, root := range roots {
		if root.Path != threeRepoWorkspace().Repos[root.Name] {
			t.Errorf("root %q carries path %q, the workspace says %q", root.Name, root.Path, threeRepoWorkspace().Repos[root.Name])
		}
	}
}

// The map is walked in name order so that two sessions over the same workspace
// describe it identically.
func TestTheRootsComeBackInAFixedOrder(t *testing.T) {
	first, err := workspaceRoots(threeRepoWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 20; attempt++ {
		again, err := workspaceRoots(threeRepoWorkspace())
		if err != nil {
			t.Fatal(err)
		}
		for i := range again {
			if again[i] != first[i] {
				t.Fatalf("root %d came back as %+v, was %+v", i, again[i], first[i])
			}
		}
	}
}

// Indexing the map with a primary name it does not hold yields "", and "" is
// how the engine spells "no root is known" — which sends every relative path in
// every tool call to the inber-server process's own working directory. Loud
// failure is the only safe answer.
func TestAWorkspaceThatCannotNameItsPrimaryRepositoryIsRefused(t *testing.T) {
	ws := threeRepoWorkspace()
	ws.Primary = "llm-bridge"

	_, err := workspaceRoots(ws)
	if err == nil {
		t.Fatal("a workspace whose primary repository is not one of its repositories was accepted")
	}
	if !strings.Contains(err.Error(), "llm-bridge") || !strings.Contains(err.Error(), "inber") {
		t.Errorf("the error names neither the missing primary nor what the workspace does hold: %v", err)
	}
}

func TestAWorkspaceWithNoRepositoriesIsRefused(t *testing.T) {
	if _, err := workspaceRoots(&forge.Workspace{ID: "brigid-1"}); err == nil {
		t.Error("an empty workspace was accepted")
	}
	if _, err := workspaceRoots(nil); err == nil {
		t.Error("a missing workspace was accepted")
	}
}

func TestARepositoryWithNoWorktreePathIsRefused(t *testing.T) {
	ws := threeRepoWorkspace()
	ws.Repos["forge"] = ""
	if _, err := workspaceRoots(ws); err == nil {
		t.Error("a repository with no worktree path was accepted")
	}
}

// The working directory and the rendered roots are the same reading of the same
// workspace, set together, so that the engine's check that they agree can never
// fail on a workspace this function accepted.
func TestTheAgentsWorkingDirectoryIsThePrimaryRootItWasToldAbout(t *testing.T) {
	var ac AgentConfig
	if err := useWorkspace(&ac, threeRepoWorkspace()); err != nil {
		t.Fatal(err)
	}
	if ac.Workspace != "/forge/work/brigid-1/inber" {
		t.Errorf("working directory is %q, want the primary worktree", ac.Workspace)
	}
	if len(ac.WorkspaceRoots) != 3 {
		t.Errorf("the agent was told about %d repositories, the workspace holds 3", len(ac.WorkspaceRoots))
	}
	for _, root := range ac.WorkspaceRoots {
		if root.Primary && root.Path != ac.Workspace {
			t.Errorf("the primary root is %s and the working directory is %s", root.Path, ac.Workspace)
		}
	}
}

// A workspace it refuses must leave the config untouched rather than half
// pointed at it: a Workspace of "" is the process directory again.
func TestARefusedWorkspaceDoesNotMoveTheAgentAnywhere(t *testing.T) {
	ac := AgentConfig{Workspace: "/repos/inber"}
	ws := threeRepoWorkspace()
	ws.Primary = "llm-bridge"

	if err := useWorkspace(&ac, ws); err == nil {
		t.Fatal("the workspace was accepted")
	}
	if ac.Workspace != "/repos/inber" || ac.WorkspaceRoots != nil {
		t.Errorf("a refused workspace left the agent at %q with %d roots", ac.Workspace, len(ac.WorkspaceRoots))
	}
}
