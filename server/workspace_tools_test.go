package server

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Track calls for assertions.
	createCalls  []createCall
	commitCalls  []commitCall
	mergeCalls   int
	pushCalls    int
	cleanupCalls int
	reopenCalls  int
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
				"claxon":  {Name: "claxon", Model: "test-model"},
				"brigid":  {Name: "brigid", Model: "test-model", Projects: []string{"kayushkin"}},
				"oisin":   {Name: "oisin", Model: "test-model", Projects: []string{"si", "bus"}},
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

	// Add workspaces.
	srv.mu.Lock()
	srv.workspaces["brigid-111"] = &forge.Workspace{
		ID:     "brigid-111",
		Repos:  map[string]string{"kayushkin": "/tmp"},
		Branch: "spawn/brigid-111",
		Status: "done",
	}
	srv.workspaces["oisin-222"] = &forge.Workspace{
		ID:     "oisin-222",
		Repos:  map[string]string{"si": "/tmp", "bus": "/tmp"},
		Branch: "spawn/oisin-222",
		Status: "created",
	}
	srv.mu.Unlock()

	result, err = tool.Run(context.Background(), "{}")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if result == "No active workspaces." {
		t.Error("expected workspaces in list")
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

	for _, expected := range []string{"spawn_agent", "sessions_list", "steer_agent", "merge_workspace", "reject_workspace", "fix_workspace", "list_workspaces"} {
		if !toolNames[expected] {
			t.Errorf("orchestrator missing tool: %s", expected)
		}
	}

	// Non-orchestrator agent does NOT get workspace tools.
	agentTools := srv.toolsForAgent("session:2", "brigid")
	for _, tool := range agentTools {
		if tool.Name == "merge_workspace" || tool.Name == "reject_workspace" || tool.Name == "fix_workspace" {
			t.Errorf("non-orchestrator should not have tool: %s", tool.Name)
		}
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
