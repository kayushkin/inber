package server

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/logger"
)

// A parent's model moves at runtime and the spawn path never looks at it: the
// child's model comes from the agent's stored config, or from an explicit
// req.Model. So a user who escalates a parent to a stronger model and then
// delegates gets a child on the configured default — and, until these tests,
// nothing anywhere recorded that the two differed.
//
// ⚠️ These tests hold the SILENCE, not the precedence. Which of req.Model, the
// parent's live model and the stored config should win is noteboard todo
// 2dcdb9a6 and is open. TestASpawnStillTakesTheChildsModelFromItsConfig below
// is the pin that keeps this repair on its own side of that line: it fails if a
// later change makes the child inherit its parent's model.

// spawnModelTestAgent returns an agent config that names a model and cannot
// build an engine.
//
// The second half is load-bearing and is not tidiness. Spawn's model report
// sits before the child session is created, and a Spawn allowed to run past it
// on this host does not stop: it loads the real agent-store config, opens an
// Anthropic client against the live credential and starts a session log. So the
// config carries two workspace roots and no primary — the cheapest input
// validateWorkspaceRoots refuses, two statements into NewEngine and before
// anything reaches outside the process. The spawn is followed as far as
// production follows it and no further.
func spawnModelTestAgent(t *testing.T, name, model string) AgentConfig {
	t.Helper()
	dir := t.TempDir()
	return AgentConfig{
		Name:      name,
		Model:     model,
		Workspace: dir,
		WorkspaceRoots: []engine.WorkspaceRoot{
			{Name: "one", Path: dir},
			{Name: "two", Path: dir},
		},
	}
}

// spawnLogLines drives one Spawn with the package logger redirected, and
// returns the entries it wrote.
//
// Spawn is driven rather than spawnModelDivergence called directly: a test that
// only drove the helper would leave the call site unheld, which is how a repair
// on this box has more than once been correct and unreachable at the same time.
func spawnLogLines(t *testing.T, parentModel string, agentConfig AgentConfig, req SpawnRequest) []logger.LogEntry {
	t.Helper()

	server := &Server{
		store:  tempStore(t),
		config: Config{MaxSpawnDepth: 2, MaxChildrenPerAgent: 4, Agents: map[string]AgentConfig{req.Agent: agentConfig}},
	}
	server.sessions.Store(req.ParentKey, &Session{
		Key:       req.ParentKey,
		AgentName: "claxon",
		Engine:    &engine.Engine{Model: parentModel},
	})

	var buf bytes.Buffer
	restore := logger.SetDefaultLogger(logger.NewWithWriter(&buf, logger.InfoLevel))
	_, _ = server.Spawn(context.Background(), req)
	restore()

	var entries []logger.LogEntry
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry logger.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("a log line was not a log entry: %v (%q)", err, line)
		}
		entries = append(entries, entry)
	}
	return entries
}

// divergenceReport returns the one entry naming a model divergence, or nil.
func divergenceReport(entries []logger.LogEntry) *logger.LogEntry {
	for i := range entries {
		if strings.Contains(entries[i].Message, "differs from its parent's live model") {
			return &entries[i]
		}
	}
	return nil
}

func TestASpawnSaysSoWhenTheChildIgnoresTheParentsLiveModel(t *testing.T) {
	entries := spawnLogLines(t,
		"claude-opus-4-6",
		spawnModelTestAgent(t, "fionn", "claude-haiku-4-6"),
		SpawnRequest{ParentKey: parentKey, Agent: "fionn", Task: "delegate"})

	report := divergenceReport(entries)
	if report == nil {
		t.Fatalf("a parent on claude-opus-4-6 spawned a child on claude-haiku-4-6 and nothing said so; %d lines logged", len(entries))
	}
	if got := report.Fields["parent_model"]; got != "claude-opus-4-6" {
		t.Errorf("parent_model reported as %v, want claude-opus-4-6", got)
	}
	if got := report.Fields["child_model"]; got != "claude-haiku-4-6" {
		t.Errorf("child_model reported as %v, want claude-haiku-4-6", got)
	}
	if got := report.Fields["child_model_source"]; got != "agent config" {
		t.Errorf("child_model_source reported as %v; the child's model came from the agent config, and a reader who cannot tell that cannot tell whether anyone asked for it", got)
	}
}

// The other side of the same claim. Without this, "a divergence is reported" is
// satisfied by a line that is printed on every spawn, which tells a reader
// nothing.
func TestASpawnSaysNothingWhenTheChildRunsOnTheParentsModel(t *testing.T) {
	entries := spawnLogLines(t,
		"claude-opus-4-6",
		spawnModelTestAgent(t, "fionn", "claude-opus-4-6"),
		SpawnRequest{ParentKey: parentKey, Agent: "fionn", Task: "delegate"})

	if report := divergenceReport(entries); report != nil {
		t.Errorf("a child on its parent's own model was reported as a divergence: %+v", report.Fields)
	}
}

// An explicit override is a divergence the caller asked for, and it is a
// different fact about the world from one nobody asked for. Reporting them
// identically makes the line useless for the case it exists to catch.
func TestTheReportNamesTheSpawnRequestWhenTheModelWasOverridden(t *testing.T) {
	entries := spawnLogLines(t,
		"claude-opus-4-6",
		spawnModelTestAgent(t, "fionn", "claude-opus-4-6"),
		SpawnRequest{ParentKey: parentKey, Agent: "fionn", Task: "delegate", Model: "claude-haiku-4-6"})

	report := divergenceReport(entries)
	if report == nil {
		t.Fatalf("an explicit model override away from the parent's model was not reported; %d lines logged", len(entries))
	}
	if got := report.Fields["child_model_source"]; got != "spawn request" {
		t.Errorf("child_model_source reported as %v, want spawn request", got)
	}
	if got := report.Fields["child_model"]; got != "claude-haiku-4-6" {
		t.Errorf("child_model reported as %v, want the overridden claude-haiku-4-6", got)
	}
}

// ⭐ The reserved-behaviour pin. This repair is a report and nothing else; the
// precedence question is open on todo 2dcdb9a6. A change that answers it by
// making the child inherit its parent's live model reddens here, because the
// child's model would be reported as the parent's rather than the config's.
func TestASpawnStillTakesTheChildsModelFromItsConfigNotItsParent(t *testing.T) {
	entries := spawnLogLines(t,
		"claude-opus-4-6",
		spawnModelTestAgent(t, "fionn", "claude-haiku-4-6"),
		SpawnRequest{ParentKey: parentKey, Agent: "fionn", Task: "delegate"})

	report := divergenceReport(entries)
	if report == nil {
		t.Fatalf("no report, so this pin cannot see which model the child got; %d lines logged", len(entries))
	}
	if got := report.Fields["child_model"]; got != "claude-haiku-4-6" {
		t.Errorf("the child's model is %v where its config says claude-haiku-4-6 — the precedence question this repair reserved has been answered", got)
	}
}

// A parent that has not taken a turn has no live model, and an agent config
// that names none leaves the choice to the engine's default. Neither is a
// divergence: there is no second name to differ from, and reporting a
// difference between a name and a blank is a claim about a model nobody has
// selected yet.
func TestAnUnknownModelOnEitherSideIsNotADivergence(t *testing.T) {
	if fields := spawnModelDivergence("", "claude-haiku-4-6", ""); fields != nil {
		t.Errorf("a parent that has not taken a turn was reported as diverging: %+v", fields)
	}
	if fields := spawnModelDivergence("claude-opus-4-6", "", ""); fields != nil {
		t.Errorf("an agent config naming no model was reported as diverging: %+v", fields)
	}
	if fields := spawnModelDivergence("", "", ""); fields != nil {
		t.Errorf("two unknown models were reported as diverging: %+v", fields)
	}
}
