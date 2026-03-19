package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kayushkin/inber/session"
)

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
	sess, err := session.New(filepath.Join(dir, "logs"), "test-model", "claxon", "", nil)
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
	if active[0].Agent != "claxon" {
		t.Errorf("expected agent claxon, got %s", active[0].Agent)
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