package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/memory"
	_ "modernc.org/sqlite"
)

// saveSpawnToMemory used to save with no ID. memory-store upserts on the id and
// defaults every field of a row except that one, so every spawn result an agent
// ever produced was written onto the single key "" and only the last survived.
// Both live memory stores on this host held exactly one such row.
func TestEachSpawnResultKeepsItsOwnMemory(t *testing.T) {
	store := openSpawnTestMemoryStore(t)
	server := &Server{}

	for _, key := range []string{"agent:brigid:child:1", "agent:brigid:child:2"} {
		child := &Session{Key: key, Engine: &engine.Engine{MemStore: store}}
		server.saveSpawnToMemory(child, "brigid", "task for "+key, "success", "it is done")
	}

	rows, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("two spawns left %d memories, want 2", len(rows))
	}
	for _, m := range rows {
		if m.ID == "" {
			t.Fatalf("a spawn result was saved under the empty id: %q", m.Content)
		}
	}
}

// The memory has to be reachable by name — memory_expand takes an id, and an
// unaddressable row is one no agent can ever ask for.
func TestASpawnMemoryIsNamedByTheChildSessionKey(t *testing.T) {
	store := openSpawnTestMemoryStore(t)
	child := &Session{Key: "agent:brigid:child:7", Engine: &engine.Engine{MemStore: store}}

	(&Server{}).saveSpawnToMemory(child, "brigid", "count the files", "success", "there are four")

	saved, err := store.Get("spawn:" + child.Key)
	if err != nil {
		t.Fatalf("a spawn memory could not be fetched by the child's key: %v", err)
	}
	if saved.ID != "spawn:"+child.Key {
		t.Fatalf("spawn memory id is %q, want %q", saved.ID, "spawn:"+child.Key)
	}
}

// Delivering the same spawn's result twice is the same spawn, so it updates one
// memory rather than leaving two.
func TestRedeliveringASpawnUpdatesItsMemory(t *testing.T) {
	store := openSpawnTestMemoryStore(t)
	child := &Session{Key: "agent:brigid:child:9", Engine: &engine.Engine{MemStore: store}}
	server := &Server{}

	server.saveSpawnToMemory(child, "brigid", "count the files", "success", "there are four")
	server.saveSpawnToMemory(child, "brigid", "count the files", "success", "there are five")

	rows, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("one spawn delivered twice left %d memories, want 1", len(rows))
	}
	saved, err := store.Get("spawn:" + child.Key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "there are five"; !strings.Contains(saved.Content, want) {
		t.Fatalf("the second delivery did not replace the first: %q", saved.Content)
	}
}

func openSpawnTestMemoryStore(t *testing.T) memory.MemoryStore {
	t.Helper()
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
