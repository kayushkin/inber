package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/kayushkin/forge"
)

// mockWorkspaceManager is a test double for WorkspaceManager.
type mockWorkspaceManager struct {
	createErr    error
	commitResult map[string]forge.CommitResult
	commitErr    error
	mergeResult  map[string]forge.MergeResult
	pushResult   map[string]error
	cleanupErr   error
	reopenErr    error

	// recorded stands in for what forge has on disk: the workspaces a restarted
	// server can still find, as opposed to the ones it happens to hold in memory.
	recorded    map[string]*forge.Workspace
	recordedErr error

	// Track calls for assertions.
	createCalls  []createCall
	commitCalls  []commitCall
	mergeCalls   int
	pushCalls    int
	cleanupCalls int
	reopenCalls  int
	getCalls     []string
}

type createCall struct {
	Agent    string
	Projects []string
}

type commitCall struct {
	WorkspaceID string
	Message     string
}

func (m *mockWorkspaceManager) CreateWorkspace(agent string, projects []string) (*forge.Workspace, error) {
	m.createCalls = append(m.createCalls, createCall{agent, projects})
	if m.createErr != nil {
		return nil, m.createErr
	}
	repos := make(map[string]string)
	for _, p := range projects {
		repos[p] = fmt.Sprintf("/tmp/test-workspace/%s-%s", agent, p)
	}
	return &forge.Workspace{
		ID:      fmt.Sprintf("%s-123456", agent),
		Repos:   repos,
		Primary: projects[0],
		BaseDir: fmt.Sprintf("/tmp/test-workspace/%s-123456", agent),
		Branch:  fmt.Sprintf("spawn/%s-123456", agent),
		Status:  "created",
	}, nil
}

func (m *mockWorkspaceManager) CommitAll(ws *forge.Workspace, message string) (map[string]forge.CommitResult, error) {
	m.commitCalls = append(m.commitCalls, commitCall{ws.ID, message})
	if m.commitErr != nil {
		return nil, m.commitErr
	}
	if m.commitResult != nil {
		return m.commitResult, nil
	}
	// Default: all repos dirty with a commit.
	result := make(map[string]forge.CommitResult)
	for name := range ws.Repos {
		result[name] = forge.CommitResult{Hash: "abc1234", Dirty: true}
	}
	return result, nil
}

func (m *mockWorkspaceManager) MergeToMain(ws *forge.Workspace) map[string]forge.MergeResult {
	m.mergeCalls++
	if m.mergeResult != nil {
		return m.mergeResult
	}
	result := make(map[string]forge.MergeResult)
	for name := range ws.Repos {
		result[name] = forge.MergeResult{Status: "ok"}
	}
	return result
}

func (m *mockWorkspaceManager) PushAll(ws *forge.Workspace) map[string]error {
	m.pushCalls++
	if m.pushResult != nil {
		return m.pushResult
	}
	return make(map[string]error)
}

func (m *mockWorkspaceManager) Cleanup(ws *forge.Workspace) error {
	m.cleanupCalls++
	return m.cleanupErr
}

func (m *mockWorkspaceManager) ReopenWorkspace(ws *forge.Workspace) error {
	m.reopenCalls++
	return m.reopenErr
}

func (m *mockWorkspaceManager) GetWorkspace(id string) (*forge.Workspace, error) {
	m.getCalls = append(m.getCalls, id)
	ws, recorded := m.recorded[id]
	if !recorded {
		return nil, fmt.Errorf("no workspace %s", id)
	}
	return ws, nil
}

func (m *mockWorkspaceManager) ListWorkspaces() ([]*forge.Workspace, error) {
	workspaces := make([]*forge.Workspace, 0, len(m.recorded))
	for _, ws := range m.recorded {
		workspaces = append(workspaces, ws)
	}
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].ID < workspaces[j].ID })
	return workspaces, m.recordedErr
}

func (m *mockWorkspaceManager) Close() error { return nil }

// newTestServer creates a Server with a mock workspace manager.
func newTestServer(t *testing.T, mock *mockWorkspaceManager) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	return &Server{
		config: Config{
			DefaultAgent:        "claxon",
			MaxSpawnDepth:       2,
			MaxChildrenPerAgent: 5,
			MainConcurrency:     4,
			SubagentConcurrency: 8,
			Agents: map[string]AgentConfig{
				"claxon": {Name: "claxon", Model: "test-model"},
				"brigid": {Name: "brigid", Model: "test-model", Projects: []string{"kayushkin"}},
				"oisin":  {Name: "oisin", Model: "test-model", Projects: []string{"si", "bus"}},
			},
		},
		store:      store,
		forgeDB:    mock,
		workspaces: make(map[string]*forge.Workspace),
		queue:      NewQueue(map[string]int{"main": 4, "subagent": 8}),
		events:     NewEventPublisher("", ""),
	}
}

func TestMergeWorkspaceTool(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	// Pre-populate a workspace.
	ws := &forge.Workspace{
		ID:     "brigid-123",
		Repos:  map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
		Branch: "spawn/brigid-123",
		Status: "done",
	}
	srv.mu.Lock()
	srv.workspaces["brigid-123"] = ws
	srv.mu.Unlock()

	tool := srv.MergeWorkspaceTool()
	input, _ := json.Marshal(map[string]any{"workspace_id": "brigid-123"})
	result, err := tool.Run(context.Background(), string(input))
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if mock.mergeCalls != 1 {
		t.Errorf("expected 1 merge call, got %d", mock.mergeCalls)
	}
	if mock.pushCalls != 1 {
		t.Errorf("expected 1 push call, got %d", mock.pushCalls)
	}
	if mock.cleanupCalls != 1 {
		t.Errorf("expected 1 cleanup call, got %d", mock.cleanupCalls)
	}

	// Workspace should be removed.
	srv.mu.RLock()
	_, exists := srv.workspaces["brigid-123"]
	srv.mu.RUnlock()
	if exists {
		t.Error("workspace should be removed after merge")
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestMergeWorkspaceConflict(t *testing.T) {
	mock := &mockWorkspaceManager{
		mergeResult: map[string]forge.MergeResult{
			"kayushkin": {Status: "conflict", Conflicts: []string{"src/App.tsx"}, Error: "CONFLICT in src/App.tsx"},
		},
	}
	srv := newTestServer(t, mock)

	ws := &forge.Workspace{
		ID:    "brigid-456",
		Repos: map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
	}
	srv.mu.Lock()
	srv.workspaces["brigid-456"] = ws
	srv.mu.Unlock()

	tool := srv.MergeWorkspaceTool()
	input, _ := json.Marshal(map[string]any{"workspace_id": "brigid-456"})
	result, err := tool.Run(context.Background(), string(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT push or cleanup on conflict.
	if mock.pushCalls != 0 {
		t.Errorf("should not push on conflict, got %d push calls", mock.pushCalls)
	}
	if mock.cleanupCalls != 0 {
		t.Errorf("should not cleanup on conflict, got %d cleanup calls", mock.cleanupCalls)
	}

	// Workspace should still exist.
	srv.mu.RLock()
	_, exists := srv.workspaces["brigid-456"]
	srv.mu.RUnlock()
	if !exists {
		t.Error("workspace should be preserved on conflict")
	}

	if result == "" {
		t.Error("expected conflict info in result")
	}
}

func TestRejectWorkspaceTool(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	ws := &forge.Workspace{
		ID:    "brigid-789",
		Repos: map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
	}
	srv.mu.Lock()
	srv.workspaces["brigid-789"] = ws
	srv.mu.Unlock()

	tool := srv.RejectWorkspaceTool()
	input, _ := json.Marshal(map[string]any{
		"workspace_id": "brigid-789",
		"reason":       "tests failing",
	})
	result, err := tool.Run(context.Background(), string(input))
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}

	if mock.cleanupCalls != 1 {
		t.Errorf("expected 1 cleanup call, got %d", mock.cleanupCalls)
	}

	srv.mu.RLock()
	_, exists := srv.workspaces["brigid-789"]
	srv.mu.RUnlock()
	if exists {
		t.Error("workspace should be removed after reject")
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestListWorkspacesTool(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	// Empty list.
	tool := srv.ListWorkspacesTool()
	result, err := tool.Run(context.Background(), "{}")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if result != "No active workspaces." {
		t.Errorf("expected empty message, got: %s", result)
	}

	// Add workspaces — to forge, which is where they live. The in-memory map
	// holds what THIS process spawned, and a server that has just restarted has
	// spawned nothing while the worktrees are still on disk holding the work.
	mock.recorded = map[string]*forge.Workspace{
		"brigid-111": {
			ID:     "brigid-111",
			Repos:  map[string]string{"kayushkin": "/tmp"},
			Branch: "spawn/brigid-111",
			Status: "done",
		},
		"oisin-222": {
			ID:     "oisin-222",
			Repos:  map[string]string{"si": "/tmp", "bus": "/tmp"},
			Branch: "spawn/oisin-222",
			Status: "created",
		},
	}

	result, err = tool.Run(context.Background(), "{}")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	for _, id := range []string{"brigid-111", "oisin-222"} {
		if !strings.Contains(result, id) {
			t.Errorf("workspace %s is on disk and the list does not mention it: %s", id, result)
		}
	}
}

func TestAWorkspaceDirectoryForgeCannotReadIsReportedInTheList(t *testing.T) {
	// One workspace that predates forge's records, or is half-deleted, must not
	// hide the ones that can still be merged — and must not go unmentioned
	// either, since nothing else will ever tell anyone it is there.
	mock := &mockWorkspaceManager{
		recorded: map[string]*forge.Workspace{
			"brigid-111": {
				ID:     "brigid-111",
				Repos:  map[string]string{"kayushkin": "/tmp"},
				Branch: "spawn/brigid-111",
				Status: "done",
			},
		},
		recordedErr: fmt.Errorf("1 workspace directories could not be read: the workspace at /home/x/forge/work/oisin-222 kept no workspace.json"),
	}
	srv := newTestServer(t, mock)

	result, err := srv.ListWorkspacesTool().Run(context.Background(), "{}")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "brigid-111") {
		t.Errorf("an unreadable workspace hid a readable one: %s", result)
	}
	if !strings.Contains(result, "oisin-222") {
		t.Errorf("the unreadable workspace went unreported: %s", result)
	}
}

func TestAWorkspaceOutlivesTheProcessThatSpawnedIt(t *testing.T) {
	// The defect: Server.workspaces is in memory, and it was the only thing the
	// workspace tools consulted. After a restart merge_workspace, reject_workspace
	// and fix_workspace all refused with "workspace not found" — so a revived
	// session's work sat in a worktree that nothing could merge or push.
	recorded := &forge.Workspace{
		ID:      "brigid-999",
		Repos:   map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
		Primary: "kayushkin",
		Branch:  "spawn/brigid-999",
		Status:  "done",
	}
	mock := &mockWorkspaceManager{recorded: map[string]*forge.Workspace{"brigid-999": recorded}}
	srv := newTestServer(t, mock)

	// Nothing in the map: this server did not spawn it.
	input, _ := json.Marshal(map[string]any{"workspace_id": "brigid-999"})
	if _, err := srv.MergeWorkspaceTool().Run(context.Background(), string(input)); err != nil {
		t.Fatalf("merge of a recorded workspace failed: %v", err)
	}
	if mock.mergeCalls != 1 {
		t.Errorf("expected the recorded workspace to be merged, got %d merge calls", mock.mergeCalls)
	}
	if len(mock.getCalls) != 1 || mock.getCalls[0] != "brigid-999" {
		t.Errorf("the workspace was not looked up in forge: %v", mock.getCalls)
	}
}

func TestAWorkspaceLookedUpOnceIsNotLookedUpAgain(t *testing.T) {
	// Two tools acting on one workspace have to hold one value, because status is
	// a field of it and forge moves it — a second copy would let a reopen read a
	// status that the merge has already changed.
	recorded := &forge.Workspace{
		ID:      "brigid-888",
		Repos:   map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
		Primary: "kayushkin",
		Branch:  "spawn/brigid-888",
		Status:  "done",
	}
	mock := &mockWorkspaceManager{recorded: map[string]*forge.Workspace{"brigid-888": recorded}}
	srv := newTestServer(t, mock)

	first, err := srv.workspaceByID("brigid-888")
	if err != nil {
		t.Fatal(err)
	}
	second, err := srv.workspaceByID("brigid-888")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("two lookups of one workspace produced two values")
	}
	if len(mock.getCalls) != 1 {
		t.Errorf("forge was asked %d times for a workspace already resolved", len(mock.getCalls))
	}
}

func TestWorkspaceNotFound(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	// Merge nonexistent workspace.
	tool := srv.MergeWorkspaceTool()
	input, _ := json.Marshal(map[string]any{"workspace_id": "nonexistent"})
	_, err := tool.Run(context.Background(), string(input))
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}

	// Reject nonexistent workspace.
	tool = srv.RejectWorkspaceTool()
	_, err = tool.Run(context.Background(), string(input))
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}
}

func TestToolsForAgent(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	// Orchestrator (default agent) gets workspace tools.
	orchestratorTools := srv.toolsForAgent("session:1", "claxon")
	toolNames := make(map[string]bool)
	for _, tool := range orchestratorTools {
		toolNames[tool.Name] = true
	}

	for _, expected := range []string{"spawn_agent", "steer_agent", "merge_workspace", "reject_workspace", "fix_workspace", "list_workspaces"} {
		if !toolNames[expected] {
			t.Errorf("orchestrator missing tool: %s", expected)
		}
	}

	// sessions_list is deliberately NOT a tool. The orchestrator reads the live
	// session list out of its system prompt instead — see TestOrchestratorGetsSessionStatusWithoutASessionsListTool.
	if toolNames["sessions_list"] {
		t.Error("sessions_list is no longer a tool; the orchestrator gets session status via a context injector")
	}

	// Non-orchestrator agent does NOT get workspace tools.
	agentTools := srv.toolsForAgent("session:2", "brigid")
	for _, tool := range agentTools {
		if tool.Name == "merge_workspace" || tool.Name == "reject_workspace" || tool.Name == "fix_workspace" {
			t.Errorf("non-orchestrator should not have tool: %s", tool.Name)
		}
	}
}

// TestOrchestratorGetsSessionStatusWithoutASessionsListTool pins the capability
// that replaced the sessions_list tool when it was deleted. The orchestrator no
// longer calls a tool to enumerate sessions; contextInjectorsFor hands it a
// live session block in its system prompt, and only the orchestrator gets one.
// Without this, dropping sessions_list from the tool list above would look like
// the orchestrator had simply lost the ability to see its sub-agents.
func TestOrchestratorGetsSessionStatusWithoutASessionsListTool(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	if got := srv.contextInjectorsFor("session:1", "claxon"); len(got) == 0 {
		t.Error("orchestrator got no context injectors — it can no longer see the session list at all")
	}

	if got := srv.contextInjectorsFor("session:2", "brigid"); got != nil {
		t.Errorf("non-orchestrator should get no session status, got %d injectors", len(got))
	}
}

func TestMergeNoPush(t *testing.T) {
	mock := &mockWorkspaceManager{}
	srv := newTestServer(t, mock)

	ws := &forge.Workspace{
		ID:    "brigid-nopush",
		Repos: map[string]string{"kayushkin": "/tmp"},
	}
	srv.mu.Lock()
	srv.workspaces["brigid-nopush"] = ws
	srv.mu.Unlock()

	tool := srv.MergeWorkspaceTool()
	pushFalse := false
	input, _ := json.Marshal(map[string]any{"workspace_id": "brigid-nopush", "push": pushFalse})
	_, err := tool.Run(context.Background(), string(input))
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if mock.pushCalls != 0 {
		t.Errorf("expected 0 push calls with push=false, got %d", mock.pushCalls)
	}
}
