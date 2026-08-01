package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A spawned child's session key is its parent's key plus a suffix
// (sessionKeyForChild), so the key spells the *parent's* agent name in the one
// position anything reading the key looks at. The bridge endpoints used to read
// it from there. On the live server that made every one of the 29 recorded
// spawn sessions — brigid's, fionn's and manannan's, 2,103 persisted messages
// between them — resolve to claxon, their parent: rebuilding one loaded the
// child's transcript into claxon's system prompt, model, workspace root and
// tools, and the next turn ran as claxon, with nothing logged and nothing to see.
//
// The child's own agent is recorded in the sessions table when it is spawned.
// These tests pin that the recorded name is what answers, and that the key is
// never allowed to answer in its place.

const (
	parentKey = "agent:claxon:main"
	childKey  = "agent:claxon:main:sub:41287"
)

func TestARevivedChildResolvesToItsOwnAgentNotTheOneItsKeyNames(t *testing.T) {
	server := &Server{store: tempStore(t)}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	agentName, err := server.agentForSession(childKey)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agentName != "brigid" {
		t.Fatalf("a child spawned as brigid resolved to %q; its key names claxon, its record names brigid", agentName)
	}
}

func TestAChildKeyOnItsOwnNamesNoAgentAtAll(t *testing.T) {
	if agentName := agentFromTopLevelSessionKey(childKey); agentName != "" {
		t.Fatalf("a child key answered %q; it carries its parent's name and knows nothing about the child", agentName)
	}
	if agentName := agentFromTopLevelSessionKey(parentKey); agentName != "claxon" {
		t.Fatalf("a top-level key should still read as its agent, got %q", agentName)
	}
}

// The last resort must not invent a name. With no record and a key that cannot
// be read, every caller reports the session as unknown; the old code would have
// handed back "claxon" and run the turn under the default agent's neighbour.
func TestAnUnrecordedChildSessionResolvesToNothing(t *testing.T) {
	server := &Server{store: tempStore(t)}

	agentName, err := server.agentForSession(childKey)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agentName != "" {
		t.Fatalf("an unrecorded child resolved to %q; nothing knows which agent it was", agentName)
	}
}

func TestALoadedSessionAnswersForItselfBeforeTheStoreIsAsked(t *testing.T) {
	server := &Server{store: tempStore(t)}
	// The store disagrees on purpose: a live session is the fresher record.
	if err := server.store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	server.sessions.Store(childKey, &Session{Key: childKey, AgentName: "fionn"})

	agentName, err := server.agentForSession(childKey)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agentName != "fionn" {
		t.Fatalf("the loaded session runs fionn; resolution said %q", agentName)
	}
}

// POST /api/run takes agent and session_key as independent fields, and
// Server.run records the pair as given. So a top-level key can name one agent
// in its text and another in its row, and the row is the one that ran. This
// pins the order — the record is consulted before the key, not merely as a
// fallback when the key happens to be unreadable.
func TestTheRecordedAgentBeatsTheOneTheKeySpells(t *testing.T) {
	server := &Server{store: tempStore(t)}
	// What POST /api/run {"agent":"brigid","session_key":"agent:claxon:main"} writes.
	if err := server.store.UpsertSession(parentKey, "brigid", "main", SessionLineage{}); err != nil {
		t.Fatalf("record the run: %v", err)
	}

	agentName, err := server.agentForSession(parentKey)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agentName != "brigid" {
		t.Fatalf("the session ran brigid and resolved to %q; the key's text is not the record", agentName)
	}
}

// createSession always names the session it builds, so a loaded session with no
// agent means something built one another way. It is not an answer, and the
// record behind it still is — a nameless session must not shadow the row.
func TestANamelessLiveSessionDoesNotShadowTheRecord(t *testing.T) {
	server := &Server{store: tempStore(t)}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	server.sessions.Store(childKey, &Session{Key: childKey})

	agentName, err := server.agentForSession(childKey)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agentName != "brigid" {
		t.Fatalf("a session that cannot name itself answered %q instead of deferring to the record", agentName)
	}
}

func TestATopLevelSessionWithNoRecordStillResolvesFromItsKey(t *testing.T) {
	server := &Server{store: tempStore(t)}

	agentName, err := server.agentForSession(parentKey)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agentName != "claxon" {
		t.Fatalf("a top-level key names its own agent and should still resolve, got %q", agentName)
	}
}

// Server.run upserts the session on every turn, passing the agent it resolved.
// If that write overwrote the agent column, one turn sent to a child through
// the bridge would destroy the only record of which agent the child is — and
// the fix above would then be reading back the wrong answer it just wrote.
func TestALaterUpsertCannotRewriteTheAgentASessionWasCreatedWith(t *testing.T) {
	store := tempStore(t)
	if err := store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	if err := store.UpsertSession(childKey, "claxon", "main", SessionLineage{}); err != nil {
		t.Fatalf("touch the session: %v", err)
	}

	agentName, err := store.SessionAgent(childKey)
	if err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if agentName != "brigid" {
		t.Fatalf("the recorded agent became %q; the name written at creation is the durable one", agentName)
	}
}

// Server.run routes a RunRequest with no agent to config.DefaultAgent, so an
// unresolvable session reaching Run does not fail — it quietly runs the message
// as somebody else. The send endpoint has to stop before that, and the test
// asserts the refusal rather than the resolution, because the refusal is what
// the default-agent fallback would swallow.
func TestSendRefusesASessionNoRecordCanNameRatherThanRunningItAsTheDefaultAgent(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DefaultAgent: "claxon"}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+childKey+"/send",
		strings.NewReader(`{"message":"carry on"}`))
	server.handleBridgeSend(recorder, request, childKey)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("an unnameable session was accepted with %d; it would have run as %q",
			recorder.Code, server.config.DefaultAgent)
	}
	if !strings.Contains(recorder.Body.String(), "no agent recorded") {
		t.Fatalf("the refusal should say what is missing, got %q", recorder.Body.String())
	}
}

func TestSendAcceptsAChildSessionOnceItsAgentIsOnRecord(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DefaultAgent: "claxon"}}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+childKey+"/send",
		strings.NewReader(`{"message":"carry on"}`))
	server.handleBridgeSend(recorder, request, childKey)

	// The turn itself needs a queue and an engine this Server has not got, so it
	// fails further in. What matters is that it got past resolution: a recorded
	// child is never refused, and never answered with the default agent's name.
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("a recorded child was refused: %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "no agent recorded") {
		t.Fatalf("a recorded child was reported as unnameable: %q", recorder.Body.String())
	}
}

// Resume rebuilds a session that is no longer in memory, and the agent it looks
// up decides the system prompt, model, workspace root and tools the rebuilt
// session gets. Neither name is a registered agent here, so the handler stops at
// the config lookup and reports the name it asked for — which is the only thing
// this test is about, and it needs no engine to see it.
func TestResumeLooksUpTheAgentTheSessionRanAsNotTheOneItsKeySpells(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+childKey+"/resume", nil)
	server.handleBridgeResume(recorder, request, childKey)

	body := recorder.Body.String()
	if strings.Contains(body, "claxon") {
		t.Fatalf("resume went looking for the parent agent: %q", body)
	}
	if !strings.Contains(body, "brigid") {
		t.Fatalf("resume should have gone looking for brigid, the agent the child ran as: %q", body)
	}
}

// A store that cannot be read is not a session with no agent. Returning an empty
// name for a failed read would put every caller on the "no record" path, and
// resume would answer "no agent recorded" for a session whose record is right
// there — a database fault reported as a missing session.
func TestAStoreThatCannotBeReadIsReportedRatherThanReadAsNoAgent(t *testing.T) {
	server := &Server{store: tempStore(t)}
	if err := server.store.UpsertSession(childKey, "brigid", "spawn", SessionLineage{ParentKey: parentKey, SpawnDepth: 1}); err != nil {
		t.Fatalf("record the spawn: %v", err)
	}
	if err := server.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}

	agentName, err := server.agentForSession(childKey)
	if err == nil {
		t.Fatalf("an unreadable store resolved to %q with no error", agentName)
	}
	if agentName != "" {
		t.Fatalf("a failed read returned a name anyway: %q", agentName)
	}
}

// Discover answers "what other sessions belong to whoever this is". Asked from
// inside a spawned child it used to answer with the parent's sessions, so a
// child looking for its own history was shown somebody else's and none of its own.
func TestDiscoverListsTheSessionsOfTheAgentTheChildActuallyIs(t *testing.T) {
	server := &Server{store: tempStore(t)}
	for _, session := range []struct{ key, agent, kind string }{
		{childKey, "brigid", "spawn"},
		{"agent:brigid:main", "brigid", "main"},
		{parentKey, "claxon", "main"},
	} {
		if err := server.store.UpsertSession(session.key, session.agent, session.kind, SessionLineage{}); err != nil {
			t.Fatalf("record %s: %v", session.key, err)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sessions/"+childKey+"/discover", nil)
	server.handleBridgeDiscover(recorder, request, childKey)

	body := recorder.Body.String()
	if !strings.Contains(body, "agent:brigid:main") {
		t.Fatalf("brigid's own session is missing from her child's discovery: %q", body)
	}
	if strings.Contains(body, `"Agent":"claxon"`) {
		t.Fatalf("discovery returned the parent's sessions: %q", body)
	}
}

func TestSessionAgentOfAKeyTheStoreHasNeverSeenIsEmptyAndNotAnError(t *testing.T) {
	store := tempStore(t)

	agentName, err := store.SessionAgent("agent:nobody:main")
	if err != nil {
		t.Fatalf("an absent session is an ordinary answer, not a failure: %v", err)
	}
	if agentName != "" {
		t.Fatalf("got %q for a key that was never written", agentName)
	}
}
