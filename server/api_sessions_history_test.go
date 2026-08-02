package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/session"
)

// historyEntry mirrors the anonymous struct handleSessionHistory serialises.
type historyEntry struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	FilePath  string `json:"file_path"`
	StartTime string `json:"start_time"`
}

// writeSessionLog creates <workspace>/logs/<segments...>/session.jsonl.
func writeSessionLog(t *testing.T, workspace string, segments ...string) {
	t.Helper()
	dir := filepath.Join(append([]string{workspace, "logs"}, segments...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write session log in %s: %v", dir, err)
	}
}

func readHistory(t *testing.T, server *Server, query string) []historyEntry {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.handleSessionHistory(recorder, httptest.NewRequest(http.MethodGet, "/api/sessions/history"+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("history %q: status %d, body %s", query, recorder.Code, recorder.Body.String())
	}
	var entries []historyEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode history %q: %v (body %s)", query, err, recorder.Body.String())
	}
	return entries
}

func agentsOf(entries []historyEntry) map[string]string {
	byID := make(map[string]string, len(entries))
	for _, e := range entries {
		byID[e.ID] = e.Agent
	}
	return byID
}

// A session written before inber 6271cfa sits at logs/<agent>/<agent>/<session>
// because both the caller and session.New joined the agent name. The reader used
// to hand the whole relative path back as the agent, so every one of the 151
// such sessions on this box reported agent "claxon/claxon" — a name no agent
// has, over an API whose consumers join on it.
func TestANestedSessionReportsTheAgentNotTheDoubledPath(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "claxon", "2026-05-02_061800_1903")
	writeSessionLog(t, workspace, "claxon", "2026-06-02_061800_1903")

	server := &Server{config: Config{Agents: map[string]AgentConfig{"claxon": {Workspace: workspace}}}}

	got := agentsOf(readHistory(t, server, ""))
	if len(got) != 2 {
		t.Fatalf("want both layouts listed, got %v", got)
	}
	for id, agent := range got {
		if agent != "claxon" {
			t.Errorf("session %s: agent = %q, want %q", id, agent, "claxon")
		}
	}
}

// A logs root belongs to the workspace, not to an agent: session.New writes the
// <agent> segment under a root every agent in that workspace shares. Walking the
// root once per configured agent listed every session once per agent, and the
// name came from the loop, so claxon's sessions were also served as fionn's.
func TestAWorkspaceSharedByTwoAgentsListsEachSessionOnceUnderItsOwnAgent(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "2026-05-02_061800_1903")
	writeSessionLog(t, workspace, "fionn", "2026-05-03_061800_2f0a")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: workspace},
		"fionn":  {Workspace: workspace},
	}}}

	entries := readHistory(t, server, "")
	if len(entries) != 2 {
		t.Fatalf("want 2 sessions from a shared root, got %d: %+v", len(entries), entries)
	}
	want := map[string]string{
		"2026-05-02_061800_1903": "claxon",
		"2026-05-03_061800_2f0a": "fionn",
	}
	for id, agent := range agentsOf(entries) {
		if want[id] != agent {
			t.Errorf("session %s: agent = %q, want %q", id, agent, want[id])
		}
	}
}

// ?agent= names the agent a session was logged under. Matching the configured
// agent instead meant that in a shared workspace every agent's filter returned
// every agent's sessions.
func TestTheAgentFilterMatchesTheLoggedAgentNotTheConfiguredOne(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "2026-05-02_061800_1903")
	writeSessionLog(t, workspace, "fionn", "2026-05-03_061800_2f0a")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: workspace},
		"fionn":  {Workspace: workspace},
	}}}

	entries := readHistory(t, server, "?agent=fionn")
	if len(entries) != 1 {
		t.Fatalf("want only fionn's session, got %d: %+v", len(entries), entries)
	}
	if entries[0].Agent != "fionn" || entries[0].ID != "2026-05-03_061800_2f0a" {
		t.Errorf("got %+v, want fionn's 2026-05-03_061800_2f0a", entries[0])
	}
}

// The nested leftovers are the case the filter has to survive: the old reader
// derived "claxon/claxon", so ?agent=claxon matched none of them.
func TestTheAgentFilterFindsNestedSessions(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "claxon", "2026-05-02_061800_1903")

	server := &Server{config: Config{Agents: map[string]AgentConfig{"claxon": {Workspace: workspace}}}}

	entries := readHistory(t, server, "?agent=claxon")
	if len(entries) != 1 {
		t.Fatalf("want the nested session, got %d: %+v", len(entries), entries)
	}
}

// session.New leaves out the agent segment entirely when it is given no agent
// name, so the session sits directly under the root. There is no agent to
// report and the reader must not invent one out of the session id.
func TestASessionWithNoAgentSegmentReportsNoAgent(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "2026-05-02_061800_1903")

	server := &Server{config: Config{Agents: map[string]AgentConfig{"claxon": {Workspace: workspace}}}}

	entries := readHistory(t, server, "")
	if len(entries) != 1 {
		t.Fatalf("want 1 session, got %d: %+v", len(entries), entries)
	}
	if entries[0].Agent != "" {
		t.Errorf("agent = %q, want empty", entries[0].Agent)
	}
}

// A session id carries whole seconds, so sessions opened in the same second tie
// on start time, and sorting an unstable sort by start time alone leaves those
// ties in an arbitrary order. The contract is start time descending, then file
// path ascending — pinned here over a tie group large enough that the sort does
// not fall back to insertion sort and accidentally preserve the input order.
func TestSessionsAreOrderedByStartTimeThenPath(t *testing.T) {
	const tied = 40
	workspace := t.TempDir()
	var wantTied []string
	for i := 0; i < tied; i++ {
		agent := "aaa"
		if i%2 == 1 {
			agent = "zzz"
		}
		id := fmt.Sprintf("2026-05-02_061800_%04d", i)
		writeSessionLog(t, workspace, agent, id)
		wantTied = append(wantTied, filepath.Join(workspace, "logs", agent, id, "session.jsonl"))
	}
	sort.Strings(wantTied)
	newest := "2026-06-02_061800_ffff"
	writeSessionLog(t, workspace, "aaa", newest)

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"aaa": {Workspace: workspace},
		"zzz": {Workspace: workspace},
	}}}

	got := readHistory(t, server, "?limit=100")
	if len(got) != tied+1 {
		t.Fatalf("want %d sessions, got %d", tied+1, len(got))
	}
	if got[0].ID != newest {
		t.Errorf("newest first: got %q, want %q", got[0].ID, newest)
	}
	for i, wantPath := range wantTied {
		if got[i+1].FilePath != wantPath {
			t.Fatalf("tied position %d: got %q, want %q", i, got[i+1].FilePath, wantPath)
		}
	}
}

// The roots are collected through a map, so the walk order — and with it the
// order equal-ranked entries reach the sort — must not depend on map iteration.
func TestTheListingIsStableAcrossIdenticalCalls(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeSessionLog(t, first, "claxon", "2026-05-02_061800_1903")
	writeSessionLog(t, first, "claxon", "2026-05-02_061800_2b71")
	writeSessionLog(t, second, "fionn", "2026-05-02_061800_3c82")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: first},
		"fionn":  {Workspace: second},
	}}}

	var want []historyEntry
	for call := 0; call < 20; call++ {
		got := readHistory(t, server, "")
		if len(got) != 3 {
			t.Fatalf("call %d: want 3 sessions, got %d", call, len(got))
		}
		if want == nil {
			want = got
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("call %d position %d: got %+v, want %+v", call, i, got[i], want[i])
			}
		}
	}
}

// Two agents in one workspace must not make the reader walk that workspace
// twice; an agent with no workspace contributes no root at all.
func TestConfiguredLogsRootsAreDistinctAndOrdered(t *testing.T) {
	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon":  {Workspace: "/repos/inber"},
		"fionn":   {Workspace: "/repos/inber"},
		"brigid":  {Workspace: "/repos/dash"},
		"unhomed": {Workspace: ""},
	}}}

	assertLogsRoots(t, server, filepath.Join("/repos/dash", "logs"), filepath.Join("/repos/inber", "logs"))
}

func assertLogsRoots(t *testing.T, server *Server, want ...string) {
	t.Helper()
	got, err := server.sessionLogsRoots()
	if err != nil {
		t.Fatalf("sessionLogsRoots: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// recordForgeSession writes the store row a spawn into a forge workspace
// leaves behind: the only durable record that a session ever worked there.
func recordForgeSession(t *testing.T, store *Store, key, agent, workspace string) {
	t.Helper()
	roots := []engine.WorkspaceRoot{{Path: workspace, Primary: true}}
	if err := store.UpsertSession(key, agent, "spawn", SessionLineage{}, roots); err != nil {
		t.Fatalf("record session %s: %v", key, err)
	}
}

// A session spawned into a forge workspace writes its transcript under the
// worktree slot, which no agent's configuration names. The reader used to build
// its search set from configuration alone, so the endpoint answered that those
// sessions did not exist — not that they were elsewhere.
func TestAForgeWorkspaceSessionIsListed(t *testing.T) {
	liveCheckout := t.TempDir()
	worktree := t.TempDir()
	store := tempStore(t)
	recordForgeSession(t, store, "agent:claxon:spawn-1", "claxon", worktree)

	writeSessionLog(t, liveCheckout, "claxon", "2026-06-02_061800_1903")
	writeSessionLog(t, worktree, "claxon", "2026-06-03_061800_2201")

	server := &Server{store: store, config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: liveCheckout},
	}}}

	got := agentsOf(readHistory(t, server, ""))
	if _, listed := got["2026-06-03_061800_2201"]; !listed {
		t.Fatalf("the forge-workspace session is missing from the listing: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("want the live checkout's session and the worktree's, got %v", got)
	}
	if agent := got["2026-06-03_061800_2201"]; agent != "claxon" {
		t.Errorf("forge-workspace session: agent = %q, want %q", agent, "claxon")
	}
}

// The recorded roots widen the scan, they do not replace it. A session whose
// store row is gone — the store is not the record of where an ordinary session
// works — is still found on disk under the agent's configured workspace.
func TestRecordedWorkspacesAddToTheConfiguredOnes(t *testing.T) {
	liveCheckout := t.TempDir()
	worktree := t.TempDir()
	store := tempStore(t)
	recordForgeSession(t, store, "agent:claxon:spawn-1", "claxon", worktree)

	server := &Server{store: store, config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: liveCheckout},
	}}}

	want := []string{session.LogsRoot(liveCheckout), session.LogsRoot(worktree)}
	sort.Strings(want)
	assertLogsRoots(t, server, want...)
}

// A session outside a workspace records an empty column, and its logs are under
// the agent's configured workspace the caller already walks. Reading one as a
// root would walk "/logs" — an absolute path in nobody's workspace.
func TestASessionWithNoWorkspaceAddsNoRoot(t *testing.T) {
	liveCheckout := t.TempDir()
	store := tempStore(t)
	if err := store.UpsertSession("agent:claxon:main-1", "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatalf("record session: %v", err)
	}

	server := &Server{store: store, config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: liveCheckout},
	}}}

	assertLogsRoots(t, server, session.LogsRoot(liveCheckout))
}

// Two sessions spawned into one workspace name it once, and a workspace that is
// also an agent's configured one is not walked twice.
func TestRecordedWorkspacesAreDistinct(t *testing.T) {
	shared := t.TempDir()
	store := tempStore(t)
	recordForgeSession(t, store, "agent:claxon:spawn-1", "claxon", shared)
	recordForgeSession(t, store, "agent:fionn:spawn-2", "fionn", shared)

	server := &Server{store: store, config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: shared},
	}}}

	assertLogsRoots(t, server, session.LogsRoot(shared))
}

// A forge workspace holds one worktree per repository the session works in, and
// the transcript goes under the primary one — that is the root the engine is
// handed. Reading any other root would point the search at a sibling repository
// that has no logs directory at all, and only the primary flag says which is
// which: the roots arrive in no meaningful order.
func TestOnlyThePrimaryRootOfAWorkspaceHoldsTheLogs(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	store := tempStore(t)
	roots := []engine.WorkspaceRoot{
		{Path: secondary, Primary: false},
		{Path: primary, Primary: true},
		{Path: t.TempDir(), Primary: false},
	}
	if err := store.UpsertSession("agent:claxon:spawn-1", "claxon", "spawn", SessionLineage{}, roots); err != nil {
		t.Fatalf("record session: %v", err)
	}

	got, err := store.RecordedPrimaryWorkspaces()
	if err != nil {
		t.Fatalf("RecordedPrimaryWorkspaces: %v", err)
	}
	if len(got) != 1 || got[0] != primary {
		t.Fatalf("got %v, want [%s]", got, primary)
	}
}

// A roots set with no primary is a workspace the engine refuses to build a
// session in, so a row holding one is corrupt. Skipping it would take every
// session in that workspace out of a listing that still said 200 — this defect
// again, with the cause hidden.
func TestAWorkspaceWithNoPrimaryRootIsReported(t *testing.T) {
	store := tempStore(t)
	roots := []engine.WorkspaceRoot{
		{Path: t.TempDir(), Primary: false},
		{Path: t.TempDir(), Primary: false},
	}
	if err := store.UpsertSession("agent:claxon:spawn-1", "claxon", "spawn", SessionLineage{}, roots); err != nil {
		t.Fatalf("record session: %v", err)
	}

	got, err := store.RecordedPrimaryWorkspaces()
	if err == nil {
		t.Fatalf("a workspace with no primary root was accepted, returning %v", got)
	}
	if !strings.Contains(err.Error(), "agent:claxon:spawn-1") {
		t.Errorf("the error does not name the session that holds the bad row: %v", err)
	}
}

// findLogsDir and findSessionLogFile answer the per-session endpoints — context,
// timeline, prompts — and they derived the search set separately from the
// listing. A forge-workspace session the listing can now see must not still be
// "session not found" everywhere else.
func TestThePerSessionLookupsFindAForgeWorkspaceSession(t *testing.T) {
	liveCheckout := t.TempDir()
	worktree := t.TempDir()
	store := tempStore(t)
	recordForgeSession(t, store, "agent:claxon:spawn-1", "claxon", worktree)
	writeSessionLog(t, worktree, "claxon", "2026-06-03_061800_2201")

	server := &Server{store: store, config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: liveCheckout},
	}}}

	logsDir, err := server.findLogsDir("2026-06-03_061800_2201")
	if err != nil {
		t.Fatalf("findLogsDir: %v", err)
	}
	if want := session.LogsRoot(worktree); logsDir != want {
		t.Errorf("findLogsDir = %q, want %q", logsDir, want)
	}

	logFile, err := server.findSessionLogFile("2026-06-03_061800_2201")
	if err != nil {
		t.Fatalf("findSessionLogFile: %v", err)
	}
	if want := filepath.Join(worktree, "logs", "claxon", "2026-06-03_061800_2201", "session.jsonl"); logFile != want {
		t.Errorf("findSessionLogFile = %q, want %q", logFile, want)
	}
}

// A store that cannot be read is a hole in the answer, and a listing that
// quietly drops every workspace session is the defect this change closes,
// wearing a 200. The endpoint says so instead.
func TestAnUnreadableStoreFailsTheListingRatherThanShrinkingIt(t *testing.T) {
	store := tempStore(t)
	store.Close()

	server := &Server{store: store, config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: t.TempDir()},
	}}}

	recorder := httptest.NewRecorder()
	server.handleSessionHistory(recorder, httptest.NewRequest(http.MethodGet, "/api/sessions/history", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want %d (body %s)", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
}

// The helper is the whole derivation, so pin its edges directly: a session
// directly under the root, the fixed layout, the nested leftovers, and a path
// that does not live under the root at all.
func TestAgentFromSessionParent(t *testing.T) {
	root := filepath.Join("/workspace", "logs")
	cases := []struct {
		name          string
		sessionParent string
		want          string
	}{
		{"session directly under the root has no agent", root, ""},
		{"current layout", filepath.Join(root, "claxon"), "claxon"},
		{"pre-6271cfa doubled layout", filepath.Join(root, "claxon", "claxon"), "claxon"},
		{"deeper nesting still names the first segment", filepath.Join(root, "claxon", "a", "b"), "claxon"},
		{"outside the root", filepath.Join("/elsewhere", "claxon"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentFromSessionParent(root, tc.sessionParent); got != tc.want {
				t.Errorf("agentFromSessionParent(%q, %q) = %q, want %q", root, tc.sessionParent, got, tc.want)
			}
		})
	}
}
