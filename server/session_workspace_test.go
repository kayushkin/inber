package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/internal/sqlitewal"
	_ "modernc.org/sqlite"
)

// A session spawned into a forge workspace has its agent's Workspace overwritten
// with the worktree, and that overwrite lives in the request that spawned it and
// in Server.workspaces, an in-memory map. So every later rebuild of the session
// reached for the agent's *stored* config — this host's live checkout — and came
// back holding a conversation entirely about ~/forge/work/<id>/<repo>.
//
// Since inber 9f363dc every filesystem tool is rooted at the repo root, so the
// rebuilt session did not write to the process directory the way it once would
// have. It wrote confidently into the live checkout instead, which is worse:
// it looks right.
//
// These tests pin that a session's workspace outlives the process, that both
// rebuild paths read it back, and that a workspace which has been cleaned up
// refuses the rebuild rather than silently falling back to the live checkout.

// worktreeRoots builds a workspace that exists on disk, so that the
// still-on-disk check is answering about real directories.
func worktreeRoots(t *testing.T) []engine.WorkspaceRoot {
	t.Helper()
	base := t.TempDir()
	primary := filepath.Join(base, "inber")
	secondary := filepath.Join(base, "forge")
	for _, path := range []string{primary, secondary} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create worktree %s: %v", path, err)
		}
	}
	return []engine.WorkspaceRoot{
		{Name: "inber", Path: primary, Primary: true},
		{Name: "forge", Path: secondary},
	}
}

func TestASpawnedChildsWorkspaceOutlivesTheProcess(t *testing.T) {
	store := tempStore(t)
	roots := worktreeRoots(t)
	if err := store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	recorded, err := store.SessionWorkspaceRoots(childKey)
	if err != nil {
		t.Fatalf("read the workspace: %v", err)
	}
	if len(recorded) != len(roots) {
		t.Fatalf("the workspace came back with %d repositories, want %d", len(recorded), len(roots))
	}
	for i, root := range roots {
		if recorded[i] != root {
			t.Errorf("repository %d came back %+v, want %+v", i, recorded[i], root)
		}
	}
}

// A session that never had a forge workspace must come back with none, not with
// an empty-but-present one: the whole resolver turns on "did this session record
// a workspace", and a zero-length slice that reads as an answer would root every
// ordinary session at "".
func TestASessionWithNoWorkspaceRecordsNone(t *testing.T) {
	store := tempStore(t)
	if err := store.UpsertSession(parentKey, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatalf("record the session: %v", err)
	}

	recorded, err := store.SessionWorkspaceRoots(parentKey)
	if err != nil {
		t.Fatalf("read the workspace: %v", err)
	}
	if len(recorded) != 0 {
		t.Errorf("a session outside any workspace came back with %d repositories: %+v", len(recorded), recorded)
	}
}

// Where a session works is settled when it is spawned. Every later turn upserts
// the same row to bump last_active, and none of them may quietly move the
// session to the agent's ordinary repository.
func TestATouchOfAnExistingSessionDoesNotRewriteItsWorkspace(t *testing.T) {
	store := tempStore(t)
	roots := worktreeRoots(t)
	if err := store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	// The shape of an ordinary turn on that same key: no lineage, no workspace.
	if err := store.UpsertSession(childKey, "brigid", "main", SessionLineage{}, nil); err != nil {
		t.Fatalf("touch the session: %v", err)
	}

	recorded, err := store.SessionWorkspaceRoots(childKey)
	if err != nil {
		t.Fatalf("read the workspace: %v", err)
	}
	if len(recorded) != len(roots) {
		t.Fatalf("after a touch the session works in %d repositories, want %d — it has been moved to the live checkout",
			len(recorded), len(roots))
	}
}

// A spawn is the one moment the config is right and the record does not exist
// yet: forge has just built the workspace and useWorkspace has just written it
// over the agent's config, and the child's row is written after the session is
// built. So the config has to win while it has an answer.
func TestTheConfigWinsAtTheMomentAWorkspaceIsCreated(t *testing.T) {
	roots := worktreeRoots(t)
	server := &Server{store: tempStore(t)}
	config := AgentConfig{
		Workspace:      engine.PrimaryWorkspaceRoot(roots),
		WorkspaceRoots: roots,
	}

	repoRoot, resolved, err := server.workspaceRootsForSession(childKey, config)
	if err != nil {
		t.Fatalf("resolve the workspace: %v", err)
	}
	if repoRoot != config.Workspace {
		t.Errorf("a freshly spawned session was rooted at %q, want its new worktree %q", repoRoot, config.Workspace)
	}
	if len(resolved) != len(roots) {
		t.Errorf("a freshly spawned session got %d repositories, want %d", len(resolved), len(roots))
	}
}

// Every rebuild afterwards has the opposite problem: the config it is handed is
// the agent's stored one. This is the defect itself, at the resolver.
func TestTheRecordAnswersWhenTheConfigNamesTheLiveCheckout(t *testing.T) {
	roots := worktreeRoots(t)
	liveCheckout := t.TempDir()
	server := &Server{store: tempStore(t)}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	repoRoot, resolved, err := server.workspaceRootsForSession(childKey, AgentConfig{Workspace: liveCheckout})
	if err != nil {
		t.Fatalf("resolve the workspace: %v", err)
	}
	if repoRoot == liveCheckout {
		t.Fatalf("the rebuilt session was rooted at the agent's live checkout %q — every tool call edits this host's repository", liveCheckout)
	}
	if repoRoot != engine.PrimaryWorkspaceRoot(roots) {
		t.Errorf("the rebuilt session was rooted at %q, want its worktree %q", repoRoot, engine.PrimaryWorkspaceRoot(roots))
	}
	if len(resolved) != len(roots) {
		t.Errorf("the rebuilt session got %d repositories, want %d — forge checked out more than it can see", len(resolved), len(roots))
	}
}

// A session that never had a workspace is left exactly as it was: the resolver
// must not start answering for the ordinary case it has no business in.
func TestASessionWithNoRecordedWorkspaceKeepsItsAgentsRepository(t *testing.T) {
	liveCheckout := t.TempDir()
	server := &Server{store: tempStore(t)}

	repoRoot, resolved, err := server.workspaceRootsForSession(parentKey, AgentConfig{Workspace: liveCheckout})
	if err != nil {
		t.Fatalf("resolve the workspace: %v", err)
	}
	if repoRoot != liveCheckout {
		t.Errorf("an ordinary session was rooted at %q, want its agent's repository %q", repoRoot, liveCheckout)
	}
	if len(resolved) != 0 {
		t.Errorf("an ordinary session was given %d workspace repositories", len(resolved))
	}
}

// The refusal. forge.Cleanup removes a workspace, and a merged or expired one is
// meant to be gone; falling back to the agent's repository at that point is the
// original defect with an extra step.
func TestARebuildIsRefusedWhenTheWorktreeIsGone(t *testing.T) {
	roots := worktreeRoots(t)
	liveCheckout := t.TempDir()
	server := &Server{store: tempStore(t)}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	if err := os.RemoveAll(roots[0].Path); err != nil {
		t.Fatalf("clean up the worktree: %v", err)
	}

	repoRoot, _, err := server.workspaceRootsForSession(childKey, AgentConfig{Workspace: liveCheckout})
	if err == nil {
		t.Fatalf("a session whose worktree was deleted was rebuilt at %q instead of being refused", repoRoot)
	}
	if !errors.Is(err, ErrWorkspaceGone) {
		t.Errorf("the refusal came back as %v, which the HTTP layer cannot tell from a broken server", err)
	}
}

// The refusal covers every repository, not just the one relative paths resolve
// against. forge.Cleanup removes worktrees one at a time and collects the
// failures per repository, so a workspace can lose one and keep the rest — and
// a session resumed into that has a primary that works, a secondary the model
// has been told about that is not there, and no error anywhere.
//
// This is the one a sabotage caught: narrowing the check to the primary root
// passed a green suite, because every other test here deletes the primary.
func TestARebuildIsRefusedWhenAnySecondaryWorktreeIsGone(t *testing.T) {
	roots := worktreeRoots(t)
	liveCheckout := t.TempDir()
	server := &Server{store: tempStore(t)}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	// The primary survives; a repository the model was told about does not.
	if err := os.RemoveAll(roots[1].Path); err != nil {
		t.Fatalf("clean up the worktree: %v", err)
	}

	repoRoot, _, err := server.workspaceRootsForSession(childKey, AgentConfig{Workspace: liveCheckout})
	if err == nil {
		t.Fatalf("a session missing its %s worktree was rebuilt at %q; the model will be told about a repository that is not there",
			roots[1].Name, repoRoot)
	}
	if !errors.Is(err, ErrWorkspaceGone) {
		t.Errorf("the refusal came back as %v, which the HTTP layer cannot tell from a broken server", err)
	}
}

// The caller, not the resolver. workspaceRootsForSession answering correctly
// says nothing about whether createSession — the one place a session is rebuilt
// from its transcript — asks it. That gap is how the agent half of this same
// class of bug survived a green suite three separate times.
//
// Building an engine needs a workspace and the host's agent configuration, so
// this cannot simply fail where it cannot run. It must not simply skip either:
// a blanket skip on any construction error is how a sabotage that ROOTED THE
// ENGINE AT THE LIVE CHECKOUT passed a green suite. The engine refuses a
// session whose roots and repo root disagree, so that sabotage failed
// construction — and "construction failed" was the skip condition.
//
// So the environment is probed first with a session that has no workspace at
// all. Once that has succeeded, every later failure here is the change under
// test rather than the host, and is fatal.
func TestARebuiltSessionComesBackInItsOwnWorktree(t *testing.T) {
	roots := worktreeRoots(t)
	liveCheckout := t.TempDir()
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}

	if _, err := server.createSession(context.Background(), "agent:brigid:probe", "brigid",
		AgentConfig{Workspace: liveCheckout}, RunRequest{}, nil); err != nil {
		t.Skipf("no engine can be built here at all (%v); the recorded workspace is pinned by the tests above", err)
	}

	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	rebuilt, err := server.createSession(context.Background(), childKey, "brigid",
		AgentConfig{Workspace: liveCheckout}, RunRequest{}, nil)
	if err != nil {
		t.Fatalf("an engine builds here, but rebuilding the session in its worktree failed: %v", err)
	}

	// What the tools resolve against, not what the session says about itself.
	// The two are separate values reaching the engine by separate routes, and
	// asserting only on the session leaves the one that edits files unpinned.
	if rebuilt.Engine.RepoRoot() == liveCheckout {
		t.Fatalf("the rebuilt session's tools resolve against this host's live checkout %q", liveCheckout)
	}
	if rebuilt.Engine.RepoRoot() != roots[0].Path {
		t.Errorf("the rebuilt session's tools resolve against %q, want its worktree %q", rebuilt.Engine.RepoRoot(), roots[0].Path)
	}
	if len(rebuilt.WorkspaceRoots) != len(roots) {
		t.Fatalf("the rebuilt session works in %d repositories, want %d", len(rebuilt.WorkspaceRoots), len(roots))
	}
	if primary := engine.PrimaryWorkspaceRoot(rebuilt.WorkspaceRoots); primary != roots[0].Path {
		t.Errorf("the rebuilt session is rooted at %q, want its worktree %q", primary, roots[0].Path)
	}
}

// The same caller for a session that was never in a workspace: it must come back
// in its agent's repository, with nothing added.
func TestARebuiltOrdinarySessionStaysInItsAgentsRepository(t *testing.T) {
	liveCheckout := t.TempDir()
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	if err := server.store.UpsertSession(parentKey, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatalf("record the session: %v", err)
	}

	rebuilt, err := server.createSession(context.Background(), parentKey, "claxon",
		AgentConfig{Workspace: liveCheckout}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}

	if len(rebuilt.WorkspaceRoots) != 0 {
		t.Errorf("an ordinary session was rebuilt into a workspace of %d repositories: %+v",
			len(rebuilt.WorkspaceRoots), rebuilt.WorkspaceRoots)
	}
}

// The second rebuild path, and the one that does not need a restart to go wrong.
// A fork copies its parent's messages — a conversation entirely about the
// parent's worktree — and both callers hand forkSession the agent's stored
// config. So before this, forking a spawned session produced a child rooted at
// ~/repos/<repo> holding a transcript about ~/forge/work/<id>/<repo>,
// immediately, in a live process.
func TestAForkStaysInTheWorktreeItsParentWorksIn(t *testing.T) {
	roots := worktreeRoots(t)
	liveCheckout := t.TempDir()
	server := &Server{
		store: tempStore(t),
		config: Config{
			DataDir: t.TempDir(),
			Agents:  map[string]AgentConfig{"claxon": {Workspace: liveCheckout}},
		},
	}
	server.sessions.Store(childKey, &Session{
		Key:            childKey,
		AgentName:      "claxon",
		SpawnDepth:     1,
		ParentKey:      parentKey,
		WorkspaceRoots: roots,
		Engine:         &engine.Engine{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+childKey+"/fork", strings.NewReader("{}"))
	server.handleBridgeFork(recorder, request, childKey)
	if recorder.Code != http.StatusCreated {
		t.Skipf("the fork endpoint did not run here (%d: %s)", recorder.Code, recorder.Body.String())
	}

	var forked struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &forked); err != nil {
		t.Fatalf("read the fork response: %v", err)
	}

	recorded, err := server.store.SessionWorkspaceRoots(forked.SessionID)
	if err != nil {
		t.Fatalf("read the fork's workspace: %v", err)
	}
	if len(recorded) != len(roots) {
		t.Fatalf("the fork works in %d repositories, want its parent's %d — it has been forked into the live checkout",
			len(recorded), len(roots))
	}
	if primary := engine.PrimaryWorkspaceRoot(recorded); primary != roots[0].Path {
		t.Errorf("the fork is rooted at %q, want its parent's worktree %q", primary, roots[0].Path)
	}
}

// A fork of an ordinary session must not acquire a workspace it never had.
func TestAForkOfAnOrdinarySessionRecordsNoWorkspace(t *testing.T) {
	liveCheckout := t.TempDir()
	server := &Server{
		store: tempStore(t),
		config: Config{
			DataDir: t.TempDir(),
			Agents:  map[string]AgentConfig{"claxon": {Workspace: liveCheckout}},
		},
	}
	server.sessions.Store(parentKey, &Session{
		Key:       parentKey,
		AgentName: "claxon",
		Engine:    &engine.Engine{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+parentKey+"/fork", strings.NewReader("{}"))
	server.handleBridgeFork(recorder, request, parentKey)
	if recorder.Code != http.StatusCreated {
		t.Skipf("the fork endpoint did not run here (%d: %s)", recorder.Code, recorder.Body.String())
	}

	var forked struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &forked); err != nil {
		t.Fatalf("read the fork response: %v", err)
	}

	recorded, err := server.store.SessionWorkspaceRoots(forked.SessionID)
	if err != nil {
		t.Fatalf("read the fork's workspace: %v", err)
	}
	if len(recorded) != 0 {
		t.Errorf("a fork of an ordinary session acquired %d workspace repositories: %+v", len(recorded), recorded)
	}
}

// The HTTP answer for a workspace that has been cleaned up. A 500 invites a
// retry that can never succeed; the caller has to be told its request is the
// thing that cannot be satisfied.
func TestResumingASessionWhoseWorktreeIsGoneIsRefusedLoudly(t *testing.T) {
	roots := worktreeRoots(t)
	liveCheckout := t.TempDir()
	server := &Server{
		store: tempStore(t),
		config: Config{
			DataDir: t.TempDir(),
			Agents:  map[string]AgentConfig{"brigid": {Workspace: liveCheckout}},
		},
	}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn",
		SessionLineage{ParentKey: parentKey, SpawnDepth: 1}, roots); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	if err := os.RemoveAll(roots[0].Path); err != nil {
		t.Fatalf("clean up the worktree: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+childKey+"/resume", nil)
	server.handleBridgeResume(recorder, request, childKey)

	if recorder.Code == http.StatusOK {
		t.Fatalf("a session whose worktree was deleted resumed anyway — it is now rooted at the live checkout")
	}
	if recorder.Code != http.StatusConflict {
		t.Errorf("the refusal came back as %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

// The migration. CREATE TABLE IF NOT EXISTS leaves an existing table exactly as
// it found it, so on every host that ran an earlier build the column is missing
// and every read of it is an error — which is the failure mode that would take
// the whole server down at startup rather than degrade quietly.
func TestAnOlderSessionsTableGainsTheWorkspaceColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.db")
	older, err := sql.Open("sqlite", sqlitewal.ConnectionDSN(path))
	if err != nil {
		t.Fatalf("open the older database: %v", err)
	}
	if _, err := older.Exec(`
		CREATE TABLE sessions (
			key TEXT PRIMARY KEY,
			agent TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'main',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_active TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			message_count INTEGER DEFAULT 0
		)`); err != nil {
		t.Fatalf("create the older table: %v", err)
	}
	if _, err := older.Exec(`INSERT INTO sessions (key, agent, kind) VALUES (?, ?, ?)`,
		childKey, "brigid", "spawn"); err != nil {
		t.Fatalf("seed the older table: %v", err)
	}
	older.Close()

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("migrate the older database: %v", err)
	}
	defer store.db.Close()

	// A session recorded before the column existed has no workspace to read,
	// and must not be guessed at: nothing on disk says which worktree it used.
	recorded, err := store.SessionWorkspaceRoots(childKey)
	if err != nil {
		t.Fatalf("read the workspace of a pre-existing session: %v", err)
	}
	if len(recorded) != 0 {
		t.Errorf("a session recorded before the column existed came back with a workspace: %+v", recorded)
	}

	roots := worktreeRoots(t)
	if err := store.UpsertSession(parentKey, "claxon", "spawn", SessionLineage{}, roots); err != nil {
		t.Fatalf("record a workspace against the migrated table: %v", err)
	}
}
