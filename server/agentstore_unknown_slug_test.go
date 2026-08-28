package server

// Characterisation tests for card 819102d3-5234-4361-a5ee-460f088aea96, the
// residual of 072a0e76-7f08-4bf5-ba06-4e578d0770ea.
//
// inber calls agent-store's GetAgentBySlug and GetStatus at four sites and pins
// none of them. Measured 2026-08-24: inber/server stayed GREEN under all three
// candidate repairs to GetAgentBySlug, including the one that returns (nil, nil)
// and makes GetStatus dereference a nil agent. agent-store's own suite caught
// every one; inber's caught none.
//
// These tests assert what each site does TODAY, so they pass against unmodified
// code. They exist so that whichever shape the parent card picks, inber's own
// suite says what changed instead of staying quiet.
//
// ⛔ They do not fix any of the four sites and must not be read as endorsing the
// current behaviour. Site A hands a raw driver string to the model and site B
// cannot tell "no such agent" from "the store is broken"; both are defects the
// parent card owns. Pinning them is what makes the repair visible, not approval.
//
// When one of these reddens, the fix is to update the expectation to the new
// behaviour and check the site still does something sensible -- not to revert
// the change that reddened it.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentstore "github.com/kayushkin/agent-store"
)

// emptyAgentStore is a real agent-store on a throwaway file. Every slug asked of
// it is unknown, which is the case all four sites are unpinned for.
func emptyAgentStore(t *testing.T) *agentstore.Store {
	t.Helper()
	st, err := agentstore.Open(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("open agent-store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// Site A -- server/status_tools.go:40, the agents_status tool.
//
// The guard is fmt.Errorf("get status: %w", err), so whatever agent-store puts
// in that error is what the MODEL reads. Today that is the raw database driver
// sentinel: a model asking after an agent that does not exist is told
// "sql: no rows in result set", which names neither the agent nor the problem.
//
// This is the assertion that catches all three candidate shapes:
//   - (nil, nil)                 -> GetStatus dereferences nil and panics
//   - typed sentinel (replace)   -> "get status: agent not found: ..."
//   - typed sentinel (wrap)      -> "...: sql: no rows in result set" appended
func TestUnknownSlugReachesTheModelAsTheRawDriverSentinel(t *testing.T) {
	g := &Server{agentStore: emptyAgentStore(t)}

	out, err := g.AgentsStatusTool().Run(context.Background(), `{"agent_slug":"no-such-agent"}`)
	if err == nil {
		t.Fatalf("agents_status returned no error for an unknown slug; got output %q.\n"+
			"If GetAgentBySlug now reports a missing agent without an error, site\n"+
			"server/status_tools.go:40 needs to say so itself rather than relying on one.", out)
	}

	const want = "get status: sql: no rows in result set"
	if err.Error() != want {
		t.Errorf("the string an unknown slug puts in front of the model changed.\n"+
			"  was: %q\n  now: %q\n"+
			"That is card 072a0e76's decision landing. Update this expectation to the new\n"+
			"text, and check server/status_tools.go:40 still tells the model which agent\n"+
			"was missing -- a bare driver sentinel was the defect being repaired.", want, err.Error())
	}
}

// Site A, second half: a KNOWN agent with no status rows is not an error. The
// site distinguishes the two by len(statuses), so a repair that turned "no rows"
// into an error at either level would collapse them.
func TestKnownAgentWithNoStatusIsNotAnErrorAtTheStatusTool(t *testing.T) {
	st := emptyAgentStore(t)
	if err := st.UpsertAgent(&agentstore.Agent{Slug: "known", DisplayName: "Known", Enabled: true}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	g := &Server{agentStore: st}

	out, err := g.AgentsStatusTool().Run(context.Background(), `{"agent_slug":"known"}`)
	if err != nil {
		t.Fatalf("a known agent with no status rows must not be an error, got %v.\n"+
			"server/status_tools.go reports that case with a sentence, via len(statuses)==0.", err)
	}
	if !strings.Contains(out, "No status found for agent") {
		t.Errorf("expected the empty-status sentence, got %q", out)
	}
}

// Site B -- server/spawn.go:153, the "is this agent already busy" check.
//
// The guard is `if statuses, err := ...; err == nil`, so an error is swallowed
// whole: "no such agent" and "the store is broken" both skip the block in
// silence and Spawn carries on. This pins that Spawn REACHES that call with an
// unknown slug and survives it, and that the error it eventually returns comes
// from further down (GetAgentConfig), not from the status check.
//
// Under the (nil, nil) shape this test is what turns the panic into a failure:
// GetStatus dereferences the nil agent inside the swallowing if-statement.
func TestUnknownSlugIsSwallowedByTheSpawnBusyCheck(t *testing.T) {
	g := &Server{
		agentStore: emptyAgentStore(t),
		config:     Config{MaxSpawnDepth: 4, MaxChildrenPerAgent: 4},
	}
	g.sessions.Store("parent-key", &Session{
		Key:        "parent-key",
		AgentName:  "parent",
		SpawnDepth: 0,
		CreatedAt:  time.Now(),
	})

	_, err := g.Spawn(context.Background(), SpawnRequest{
		ParentKey: "parent-key",
		Agent:     "no-such-agent",
		Task:      "anything",
	})
	if err == nil {
		t.Fatal("expected Spawn to fail for an unknown agent")
	}

	// The status check must not be what failed it -- it swallows.
	if strings.Contains(err.Error(), "no rows") || strings.Contains(err.Error(), "get status") {
		t.Errorf("the spawn busy-check stopped swallowing its error: %v\n"+
			"server/spawn.go:153 uses `err == nil`, so an unknown agent reached the\n"+
			"config lookup below it. If that is now deliberate, this test should assert\n"+
			"the new behaviour -- but check that a BROKEN store is still distinguishable\n"+
			"from a missing agent, which the old guard could not do.", err)
	}
	const want = "unknown agent: no-such-agent"
	if err.Error() != want {
		t.Errorf("Spawn's unknown-agent error changed.\n  was: %q\n  now: %q\n"+
			"It is produced by GetAgentConfig below the status check, not by agent-store.",
			want, err.Error())
	}
}

// Site D -- server/api_agent_config.go:119, the PATCH handler.
//
// The guard turns ANY error into 404 "agent not found", so a missing agent and a
// broken store are one response. Pinned as-is.
func TestUnknownSlugIsA404FromTheAgentConfigPatchHandler(t *testing.T) {
	g := &Server{agentStore: emptyAgentStore(t)}

	body, err := json.Marshal(agentConfigPatch{Slug: "no-such-agent"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	g.handleAgentConfigPatch(rec, httptest.NewRequest(http.MethodPatch, "/api/agent-config", strings.NewReader(string(body))))

	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown slug on PATCH /api/agent-config: want %d, got %d (body %q).\n"+
			"server/api_agent_config.go:119 maps every GetAgentBySlug error to 404. If a\n"+
			"repair now separates 'missing' from 'store broken', this should assert the\n"+
			"new split -- a broken store answering 404 was the defect.",
			http.StatusNotFound, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent not found: no-such-agent") {
		t.Errorf("the 404 body changed: %q", rec.Body.String())
	}
}

// Site C -- server/api_agent_config.go:77 -- is deliberately NOT pinned here,
// and this test records why, so the next reader does not read its absence as an
// oversight.
//
// That site's `if err != nil { continue }` guards a GetAgentBySlug call whose
// slug came out of ListAgentsExpanded three lines above, and that query JOINs
// the agents table. So the slug always names a row that exists: the error path
// is reachable only if the agent is deleted BETWEEN the two calls, which is a
// race no in-process test can stage without a seam that does not exist.
//
// The consequence worth writing down: site C does not change under any of the
// three candidate shapes, because all three differ only for an UNKNOWN slug and
// this site never passes one. It is unpinned because it is unreachable, not
// because it is safe -- if a future change lets ListAgentsExpanded return a slug
// GetAgentBySlug cannot resolve, this floor is void and the site needs a pin.
//
// The test below pins what the listing actually does on a freshly-migrated
// store, which is not what it was written to do -- see the ⛔ below.
func TestAgentConfigListingIsBrokenOnAFreshlyMigratedStore(t *testing.T) {
	st := emptyAgentStore(t)
	if err := st.UpsertHarness(&agentstore.Harness{ID: "inber", DisplayName: "inber"}); err != nil {
		t.Fatalf("seed harness row: %v", err)
	}
	a := &agentstore.Agent{Slug: "known", DisplayName: "Known", Enabled: true}
	if err := st.UpsertAgent(a); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if err := st.UpsertAgentHarness(&agentstore.AgentHarness{
		AgentID: a.ID, HarnessID: "inber", HarnessAgentID: "known", Enabled: true,
	}); err != nil {
		t.Fatalf("seed harness binding: %v", err)
	}
	g := &Server{agentStore: st}

	rec := httptest.NewRecorder()
	g.handleAgentConfigGet(rec)

	// ⛔ This pins a DEFECT, found while writing this file and measured against a
	// fresh store rather than inferred: agent-store's migrate() never creates
	// harness.emoji, but ListAgentsExpanded selects COALESCE(o.emoji, ''). So on
	// any database built by Open() alone the listing is a 500, and site C at
	// server/api_agent_config.go:77 is never even reached.
	//
	// It does not reproduce against the live ~/.config/agent-store/agents.db,
	// which HAS the column -- somebody added it by hand. That is the whole
	// finding: the schema on disk and the schema the code migrates to have
	// drifted, and only a fresh deployment can tell.
	//
	// When the missing ALTER lands in agent-store, this test reddens. That is
	// correct and expected: replace it with the listing assertion this test was
	// originally written to make -- one entry, slug "known" -- which is in the
	// card's write-up.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected the o.emoji schema drift to make this a 500, got %d (%q).\n"+
			"If agent-store now migrates harness.emoji, this test has served its purpose:\n"+
			"assert the real listing instead -- 200 with one entry, slug \"known\".",
			rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "o.emoji") {
		t.Errorf("the 500 is no longer the emoji column drift: %q", rec.Body.String())
	}
}

// Guard against the fixture rotting into one that cannot fail: every test above
// depends on the store genuinely having no such agent. If Open ever returned a
// pre-seeded store, the four tests would pass by accident.
func TestTheUnknownSlugFixtureIsGenuinelyEmpty(t *testing.T) {
	st := emptyAgentStore(t)
	if _, err := st.GetAgentBySlug("no-such-agent"); err == nil {
		t.Fatal("the fixture resolved a slug it was never given; every test in this file " +
			"would then be asserting against a store that is not empty")
	}
}
