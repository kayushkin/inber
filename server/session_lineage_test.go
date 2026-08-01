package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// A spawned child's parent key and spawn depth were set on the in-memory
// Session by Spawn and forkSession and written down nowhere else, so a restart
// rebuilt every child as a root. The cap in Spawn is checked against the
// parent's depth, which made MaxSpawnDepth a bound on a tree only for as long
// as the process stayed up: a revived depth-2 child read zero and could spawn
// two more levels, and each of those could do it again after the next restart.
//
// These tests pin that lineage survives the process, that a rebuild reads it
// back, and that the cap is enforced against what was read.

func TestASpawnedChildsLineageOutlivesTheProcess(t *testing.T) {
	store := tempStore(t)
	if err := store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 2}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	lineage, err := store.SessionLineage(childKey)
	if err != nil {
		t.Fatalf("read lineage: %v", err)
	}
	if lineage.ParentKey != parentKey {
		t.Errorf("parent key came back %q, want %q — the child's results have nowhere to go", lineage.ParentKey, parentKey)
	}
	if lineage.SpawnDepth != 2 {
		t.Errorf("spawn depth came back %d, want 2 — the depth cap is being checked against a lie", lineage.SpawnDepth)
	}
}

// Where a session came from is settled when it is spawned. Every later turn
// upserts the same row to bump last_active, and none of them may quietly
// promote a child to a root.
func TestATouchOfAnExistingSessionDoesNotRewriteItsLineage(t *testing.T) {
	store := tempStore(t)
	if err := store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 2}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	if err := store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{}); err != nil {
		t.Fatalf("touch the session: %v", err)
	}

	lineage, _ := store.SessionLineage(childKey)
	if lineage != (SessionLineage{ParentKey: parentKey, SpawnDepth: 2}) {
		t.Errorf("a second upsert rewrote the lineage to %+v", lineage)
	}
}

// A key nothing has recorded is a root, and stays one. The alternative — the
// key names ":sub:" so read the parent out of it — is the defect that handed a
// child's transcript to its parent's agent, and it must not arrive a second
// way through the lineage read.
func TestAnUnrecordedSessionIsARootAndItsKeyIsNotConsulted(t *testing.T) {
	server := &Server{store: tempStore(t)}

	if lineage := server.lineageForSession(childKey); lineage != (SessionLineage{}) {
		t.Errorf("an unrecorded child key answered %+v; nothing knows where it came from", lineage)
	}
}

func TestAServerWithNoStoreRebuildsRootsRatherThanPanicking(t *testing.T) {
	server := &Server{}

	if lineage := server.lineageForSession(childKey); lineage != (SessionLineage{}) {
		t.Errorf("lineage without a store answered %+v", lineage)
	}
}

// The consequence, at the level that matters: the cap is checked against
// parent.SpawnDepth, and the rebuild is where that number now comes from.
func TestARevivedChildStillCountsAgainstTheSpawnDepthCap(t *testing.T) {
	server := &Server{
		store:  tempStore(t),
		config: Config{MaxSpawnDepth: 2, MaxChildrenPerAgent: 4},
	}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 2}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	// Exactly what createSession builds from the recorded lineage.
	lineage := server.lineageForSession(childKey)
	server.sessions.Store(childKey, &Session{
		Key:        childKey,
		AgentName:  "brigid",
		SpawnDepth: lineage.SpawnDepth,
		ParentKey:  lineage.ParentKey,
	})

	_, err := server.Spawn(context.Background(), SpawnRequest{
		ParentKey: childKey, Agent: "fionn", Task: "go deeper",
	})
	if err == nil || !strings.Contains(err.Error(), "max spawn depth") {
		t.Fatalf("a revived depth-2 child was allowed to spawn (err %v); the cap bounds the tree only while the server stays up", err)
	}
}

// The other half of the same cap: a child under the limit must still get
// through it. Without this, "the cap refuses" is satisfied by a cap that
// refuses everything.
func TestAChildUnderTheCapIsNotStoppedByIt(t *testing.T) {
	server := &Server{
		store:  tempStore(t),
		config: Config{MaxSpawnDepth: 2, MaxChildrenPerAgent: 4},
	}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	lineage := server.lineageForSession(childKey)
	server.sessions.Store(childKey, &Session{
		Key:        childKey,
		AgentName:  "brigid",
		SpawnDepth: lineage.SpawnDepth,
		ParentKey:  lineage.ParentKey,
	})

	// It gets past the cap and stops at the next gate — resolving the agent —
	// which is as far as a test without an engine can follow it.
	_, err := server.Spawn(context.Background(), SpawnRequest{
		ParentKey: childKey, Agent: "fionn", Task: "one more level",
	})
	if err != nil && strings.Contains(err.Error(), "max spawn depth") {
		t.Fatalf("a depth-1 child was refused by a cap of 2: %v", err)
	}
}

// The caller, not the resolver. lineageForSession answering correctly says
// nothing about whether createSession — the one place a child is rebuilt from
// its transcript — asks it. That gap is how the agent half of this same bug
// survived a green suite three separate times, so the rebuild itself is pinned
// here.
//
// Building an engine needs a workspace and the host's agent configuration, so
// this skips where it cannot run rather than failing; an assertion below it
// never skips.
func TestARebuiltSessionComesBackKnowingWhoseChildItIs(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 2}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	rebuilt, err := server.createSession(context.Background(), childKey, "brigid",
		AgentConfig{Workspace: workspace}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v); the recorded lineage is pinned by the tests above", err)
	}

	if rebuilt.ParentKey != parentKey {
		t.Errorf("the rebuilt child's parent is %q, want %q — its results have nowhere to go", rebuilt.ParentKey, parentKey)
	}
	if rebuilt.SpawnDepth != 2 {
		t.Errorf("the rebuilt child came back at depth %d, want 2 — it can spawn two more levels", rebuilt.SpawnDepth)
	}
}

// The same rebuild for a session that is nobody's child: it must come back a
// root, not inherit anything.
func TestARebuiltTopLevelSessionComesBackARoot(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	if err := server.store.UpsertSession(parentKey, "claxon", "main", SessionLineage{}); err != nil {
		t.Fatalf("record the session: %v", err)
	}

	rebuilt, err := server.createSession(context.Background(), parentKey, "claxon",
		AgentConfig{Workspace: workspace}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}

	if rebuilt.ParentKey != "" || rebuilt.SpawnDepth != 0 {
		t.Errorf("a top-level session was rebuilt as a child of %q at depth %d", rebuilt.ParentKey, rebuilt.SpawnDepth)
	}
}

// Every child already on disk was recorded before there were columns to record
// it in. sessionKeyForChild builds a child's key as its parent's key with
// ":sub:<suffix>" appended, so the key carries the parent's *key* — an id this
// table assigned — and one ":sub:" per level. Without the backfill those
// children stay at depth zero forever and the cap goes on lying about exactly
// the sessions it was added for.
func TestChildrenRecordedBeforeTheColumnsExistedAreRepairedOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// A child written by an older build: its row has no lineage.
	const grandchildKey = childKey + ":sub:90210"
	for _, key := range []string{childKey, grandchildKey} {
		if _, err := store.db.Exec(
			`INSERT INTO sessions (key, agent, kind) VALUES (?, 'brigid', 'spawn')`, key); err != nil {
			t.Fatalf("write the pre-migration row: %v", err)
		}
	}
	// And a top-level session, which the repair must leave alone.
	if err := store.UpsertSession(parentKey, "claxon", "main", SessionLineage{}); err != nil {
		t.Fatalf("record the parent: %v", err)
	}
	store.Close()

	reopened, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { reopened.Close() })

	child, _ := reopened.SessionLineage(childKey)
	if child != (SessionLineage{ParentKey: parentKey, SpawnDepth: 1}) {
		t.Errorf("child repaired to %+v, want parent %q at depth 1", child, parentKey)
	}
	grandchild, _ := reopened.SessionLineage(grandchildKey)
	if grandchild != (SessionLineage{ParentKey: childKey, SpawnDepth: 2}) {
		t.Errorf("grandchild repaired to %+v, want parent %q at depth 2", grandchild, childKey)
	}
	root, _ := reopened.SessionLineage(parentKey)
	if root != (SessionLineage{}) {
		t.Errorf("a top-level session was given a lineage of %+v", root)
	}
}

// A database written by a build that predates the columns has to gain them,
// and the gain has to survive being opened again.
func TestOpeningADatabaseWithoutTheLineageColumnsAddsThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.db.Exec(`ALTER TABLE sessions DROP COLUMN parent_key`); err != nil {
		t.Fatalf("simulate the old schema: %v", err)
	}
	if _, err := store.db.Exec(`ALTER TABLE sessions DROP COLUMN spawn_depth`); err != nil {
		t.Fatalf("simulate the old schema: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO sessions (key, agent, kind) VALUES (?, 'brigid', 'spawn')`, childKey); err != nil {
		t.Fatalf("write the pre-migration row: %v", err)
	}
	store.Close()

	reopened, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen a database with the old schema: %v", err)
	}
	t.Cleanup(func() { reopened.Close() })

	lineage, err := reopened.SessionLineage(childKey)
	if err != nil {
		t.Fatalf("read lineage after the migration: %v", err)
	}
	if lineage != (SessionLineage{ParentKey: parentKey, SpawnDepth: 1}) {
		t.Errorf("lineage after the migration is %+v, want parent %q at depth 1", lineage, parentKey)
	}
}
