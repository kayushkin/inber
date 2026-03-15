package server

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreSessionCRUD(t *testing.T) {
	s := tempStore(t)

	// Create session.
	if err := s.UpsertSession("agent:claxon:main", "claxon", "main"); err != nil {
		t.Fatal(err)
	}

	// List sessions.
	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Agent != "claxon" {
		t.Errorf("expected agent=claxon, got %s", sessions[0].Agent)
	}
	if sessions[0].Kind != "main" {
		t.Errorf("expected kind=main, got %s", sessions[0].Kind)
	}

	// Upsert again (should not create duplicate).
	s.UpsertSession("agent:claxon:main", "claxon", "main")
	sessions, _ = s.ListSessions("")
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after upsert, got %d", len(sessions))
	}

	// Filter by kind.
	s.UpsertSession("agent:ogma:spawn-123", "ogma", "spawn")
	mains, _ := s.ListSessions("main")
	spawns, _ := s.ListSessions("spawn")
	if len(mains) != 1 || len(spawns) != 1 {
		t.Fatalf("expected 1 main + 1 spawn, got %d + %d", len(mains), len(spawns))
	}
}

func TestStoreRequestLifecycle(t *testing.T) {
	s := tempStore(t)
	s.UpsertSession("agent:claxon:main", "claxon", "main")

	// Create request.
	reqID, err := s.CreateRequest("agent:claxon:main", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reqID == 0 {
		t.Fatal("expected non-zero request ID")
	}

	// Active request.
	active, err := s.ActiveRequest("agent:claxon:main")
	if err != nil {
		t.Fatal(err)
	}
	if active == nil {
		t.Fatal("expected active request")
	}
	if active.Status != "running" {
		t.Errorf("expected status=running, got %s", active.Status)
	}

	// Complete request.
	err = s.CompleteRequest(reqID, "completed", "world", "", 3, 1000, 500, 800, 200, 0.05)
	if err != nil {
		t.Fatal(err)
	}

	// No longer active.
	active, _ = s.ActiveRequest("agent:claxon:main")
	if active != nil {
		t.Error("expected no active request after completion")
	}

	// Recent requests.
	recent, err := s.RecentRequests("agent:claxon:main", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent request, got %d", len(recent))
	}
	if recent[0].Status != "completed" {
		t.Errorf("expected completed, got %s", recent[0].Status)
	}
	if recent[0].InputTokens != 1000 {
		t.Errorf("expected 1000 input tokens, got %d", recent[0].InputTokens)
	}
	if recent[0].OutputText == nil || *recent[0].OutputText != "world" {
		t.Errorf("expected output_text='world', got %v", recent[0].OutputText)
	}
}

func TestStoreSpawnChain(t *testing.T) {
	s := tempStore(t)
	s.UpsertSession("agent:claxon:main", "claxon", "main")
	s.UpsertSession("agent:ogma:spawn-1", "ogma", "spawn")

	// Parent request.
	parentID, _ := s.CreateRequest("agent:claxon:main", "deploy everything", nil)

	// Child request linked to parent.
	childID, _ := s.CreateRequest("agent:ogma:spawn-1", "fix logstack", &parentID)

	// Complete child.
	s.CompleteRequest(childID, "completed", "done", "", 5, 2000, 800, 1500, 300, 0.08)

	// Query children.
	children, err := s.SpawnChildren(parentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].SessionKey != "agent:ogma:spawn-1" {
		t.Errorf("expected ogma spawn session, got %s", children[0].SessionKey)
	}
}

func TestStoreInterruptRunning(t *testing.T) {
	s := tempStore(t)
	s.UpsertSession("agent:claxon:main", "claxon", "main")

	// Create running requests.
	s.CreateRequest("agent:claxon:main", "task1", nil)
	s.CreateRequest("agent:claxon:main", "task2", nil)

	// Interrupt all.
	n, err := s.InterruptRunning()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 interrupted, got %d", n)
	}

	// No active.
	active, _ := s.ActiveRequest("agent:claxon:main")
	if active != nil {
		t.Error("expected no active after interrupt")
	}

	// Check status.
	recent, _ := s.RecentRequests("agent:claxon:main", 10)
	for _, r := range recent {
		if r.Status != "interrupted" {
			t.Errorf("expected interrupted, got %s", r.Status)
		}
	}
}

func TestStoreTouchSession(t *testing.T) {
	s := tempStore(t)
	s.UpsertSession("agent:claxon:main", "claxon", "main")

	s.TouchSession("agent:claxon:main", 42)

	sessions, _ := s.ListSessions("")
	if sessions[0].MessageCount != 42 {
		t.Errorf("expected 42 messages, got %d", sessions[0].MessageCount)
	}
}

// Ensure DB file is actually created.
func TestStoreCreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "server.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected DB file to exist")
	}
}
