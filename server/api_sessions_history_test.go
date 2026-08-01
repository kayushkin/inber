package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
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

	got := server.configuredLogsRoots()
	want := []string{filepath.Join("/repos/dash", "logs"), filepath.Join("/repos/inber", "logs")}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
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
