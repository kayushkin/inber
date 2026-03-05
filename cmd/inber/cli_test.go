package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/session"
)

// executeCommand runs a cobra command and captures stdout
func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// setupTestRepo creates a temporary repo structure for testing
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create .git dir so FindRepoRoot works
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Create agents dir with a test agent
	agentsDir := filepath.Join(dir, "agents")
	os.MkdirAll(agentsDir, 0755)

	os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte("# Test Agent\nYou are a test agent."), 0644)

	// Create agents.json
	agentsJSON := `{
		"agents": {
			"test-agent": {
				"name": "test-agent",
				"model": "claude-sonnet-4-20250514",
				"tools": ["shell", "read_file"]
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "agents.json"), []byte(agentsJSON), 0644)

	return dir
}

func TestFindRepoRoot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Create a subdirectory
	subdir := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(subdir, 0755)

	// FindRepoRoot from subdirectory should find root
	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	os.Chdir(subdir)
	root, err := engine.FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot failed: %v", err)
	}
	if root != dir {
		t.Errorf("expected %s, got %s", dir, root)
	}
}

func TestFindRepoRoot_NotInRepo(t *testing.T) {
	dir := t.TempDir() // no .git

	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	os.Chdir(dir)
	_, err := engine.FindRepoRoot()
	if err == nil {
		t.Error("expected error when not in a git repository")
	}
}

func TestAgentsList(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Call the command function directly
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runAgentsList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "test-agent") {
		t.Errorf("expected output to contain 'test-agent', got: %s", output)
	}
	if !strings.Contains(output, "Configured agents") {
		t.Errorf("expected output to contain 'Configured agents', got: %s", output)
	}
}

func TestAgentsShow(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runAgentsShow(nil, []string{"test-agent"})

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "test-agent") {
		t.Errorf("expected agent name in output, got: %s", output)
	}
	if !strings.Contains(output, "You are a test agent") {
		t.Errorf("expected identity text in output, got: %s", output)
	}
}

func TestModelsList(t *testing.T) {
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runModelsList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "Claude") || !strings.Contains(output, "tokens") {
		t.Errorf("expected model listing with token info, got: %s", output)
	}
}

func TestSessionsList(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Create a fake session log
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(logsDir, 0755)

	entry := session.Entry{
		Timestamp: time.Now(),
		Role:      "user",
		Content:   "hello",
	}
	data, _ := json.Marshal(entry)
	sessDir := filepath.Join(logsDir, "2026-02-24_120000_abc1")
	os.MkdirAll(sessDir, 0755)
	os.WriteFile(filepath.Join(sessDir, "session.jsonl"), data, 0644)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sessionsLimit = 10
	runSessionsList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "2026-02-24_120000_abc1") {
		t.Errorf("expected session ID in output, got: %s", output)
	}
}

func TestSessionsShow(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(logsDir, 0755)

	entries := []session.Entry{
		{Timestamp: time.Now(), Role: "user", Content: "hello world"},
		{Timestamp: time.Now(), Role: "assistant", Content: "hi there"},
	}

	var lines []string
	for _, e := range entries {
		data, _ := json.Marshal(e)
		lines = append(lines, string(data))
	}
	os.WriteFile(filepath.Join(logsDir, "20260224-120000-sess42.jsonl"),
		[]byte(strings.Join(lines, "\n")), 0644)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runSessionsShow(nil, []string{"sess42"})

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "hello world") {
		t.Errorf("expected user message in output, got: %s", output)
	}
	if !strings.Contains(output, "hi there") {
		t.Errorf("expected assistant message in output, got: %s", output)
	}
}

func TestMemorySaveSearchForget(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Save a memory directly via the store
	store, err := memory.OpenOrCreate(dir)
	if err != nil {
		t.Fatalf("failed to open memory store: %v", err)
	}

	m := memory.Memory{
		ID:         "test-mem-001",
		Content:    "The quick brown fox jumps over the lazy dog",
		Tags:       []string{"test", "animals"},
		Importance: 0.8,
		Source:     "user",
	}
	if err := store.Save(m); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}
	store.Close()

	// Test search
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	memorySearchLimit = 10
	runMemorySearch(nil, []string{"fox", "dog"})

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "quick brown fox") {
		t.Errorf("expected memory content in search results, got: %s", output)
	}

	// Test list
	r, w, _ = os.Pipe()
	os.Stdout = w
	buf.Reset()

	memoryListLimit = 10
	memoryListMin = 0.0
	runMemoryList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output = buf.String()
	if !strings.Contains(output, "quick brown fox") {
		t.Errorf("expected memory in list, got: %s", output)
	}

	// Test stats
	r, w, _ = os.Pipe()
	os.Stdout = w
	buf.Reset()

	runMemoryStats(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output = buf.String()
	if !strings.Contains(output, "Total memories: 1") {
		t.Errorf("expected 1 memory in stats, got: %s", output)
	}
}

func TestConfigShow(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfigShow(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "Configuration") {
		t.Errorf("expected 'Configuration' in output, got: %s", output)
	}
	if !strings.Contains(output, "Repo root:") {
		t.Errorf("expected 'Repo root:' in output, got: %s", output)
	}
}

func TestConfigInit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfigInit(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	// Verify files were created
	if _, err := os.Stat(filepath.Join(dir, ".inber")); os.IsNotExist(err) {
		t.Error("expected .inber directory to be created")
	}
	if _, err := os.Stat(filepath.Join(dir, "agents.json")); os.IsNotExist(err) {
		t.Error("expected agents.json to be created")
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "default.md")); os.IsNotExist(err) {
		t.Error("expected agents/default.md to be created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); os.IsNotExist(err) {
		t.Error("expected .env to be created")
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncateText(tt.input, tt.max)
		if result != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
		}
	}
}

func TestSessionsListSubdirectories(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	logsDir := filepath.Join(dir, "logs")

	// Create top-level session (new dir format)
	topSessDir := filepath.Join(logsDir, "2026-02-24_120000_ab12")
	os.MkdirAll(topSessDir, 0755)
	entry := session.Entry{Timestamp: time.Now(), Role: "user", Content: "top level"}
	data, _ := json.Marshal(entry)
	os.WriteFile(filepath.Join(topSessDir, "session.jsonl"), data, 0644)

	// Create session in agent subdirectory
	agentSessDir := filepath.Join(logsDir, "myagent", "2026-02-25_190214_cd34")
	os.MkdirAll(agentSessDir, 0755)
	entry2 := session.Entry{Timestamp: time.Now(), Role: "user", Content: "from agent"}
	data2, _ := json.Marshal(entry2)
	os.WriteFile(filepath.Join(agentSessDir, "session.jsonl"), data2, 0644)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sessionsLimit = 10
	runSessionsList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "2026-02-24_120000_ab12") {
		t.Errorf("expected top-level session in output, got: %s", output)
	}
	if !strings.Contains(output, "2026-02-25_190214_cd34") {
		t.Errorf("expected subdirectory session in output, got: %s", output)
	}
	if !strings.Contains(output, "[myagent]") {
		t.Errorf("expected agent name in output, got: %s", output)
	}
}

func TestSessionsShowSubdirectory(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	agentDir := filepath.Join(dir, "logs", "myagent")
	os.MkdirAll(agentDir, 0755)

	entries := []session.Entry{
		{Timestamp: time.Now(), Role: "user", Content: "sub hello"},
		{Timestamp: time.Now(), Role: "assistant", Content: "sub response"},
	}
	var lines []string
	for _, e := range entries {
		data, _ := json.Marshal(e)
		lines = append(lines, string(data))
	}
	os.WriteFile(filepath.Join(agentDir, "2026-02-25_190214.jsonl"),
		[]byte(strings.Join(lines, "\n")), 0644)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runSessionsShow(nil, []string{"2026-02-25_190214"})

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "sub hello") {
		t.Errorf("expected user message, got: %s", output)
	}
	if !strings.Contains(output, "sub response") {
		t.Errorf("expected assistant message, got: %s", output)
	}
}

func TestSessionsActive(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Create a session and register it as active
	sess, err := session.New(filepath.Join(dir, "logs"), "test-model", "test-agent", "")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer sess.Close()

	_, err = session.RegisterActive(dir, sess, "chat")
	if err != nil {
		t.Fatalf("failed to register active: %v", err)
	}

	// List active sessions
	active, err := session.ListActive(dir)
	if err != nil {
		t.Fatalf("failed to list active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active session, got %d", len(active))
	}
	if active[0].Agent != "test-agent" {
		t.Errorf("expected agent test-agent, got %s", active[0].Agent)
	}
	if active[0].Command != "chat" {
		t.Errorf("expected command chat, got %s", active[0].Command)
	}

	// Unregister
	session.UnregisterActive(dir, sess.SessionID())
	active, _ = session.ListActive(dir)
	if len(active) != 0 {
		t.Errorf("expected 0 active sessions after unregister, got %d", len(active))
	}
}

func TestSessionsActiveStaleCleanup(t *testing.T) {
	dir := setupTestRepo(t)

	// Write a fake active session with a dead PID
	activeDir := filepath.Join(dir, ".inber", "active")
	os.MkdirAll(activeDir, 0755)

	fakeActive := session.ActiveSession{
		PID:       999999999, // almost certainly not running
		Agent:     "stale",
		SessionID: "stale-session",
		Command:   "chat",
	}
	data, _ := json.Marshal(fakeActive)
	os.WriteFile(filepath.Join(activeDir, "stale-session.json"), data, 0644)

	active, err := session.ListActive(dir)
	if err != nil {
		t.Fatalf("failed to list active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected stale session to be cleaned up, got %d active", len(active))
	}

	// Verify the file was removed
	if _, err := os.Stat(filepath.Join(activeDir, "stale-session.json")); !os.IsNotExist(err) {
		t.Error("expected stale file to be removed")
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Create a minimal context store
	store := inbercontext.NewStore()
	store.Add(inbercontext.Chunk{
		ID:     "test-identity",
		Text:   "You are a test agent.",
		Tags:   []string{"identity"},
		Tokens: 20,
	})

	eng := &engine.Engine{ContextStore: store}
	blocks := eng.BuildSystemPrompt("hello")
	found := false
	for _, b := range blocks {
		if strings.Contains(b.Text, "test agent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected identity in system prompt blocks")
	}
}
