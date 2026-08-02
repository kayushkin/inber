package session

import (
	"path/filepath"
	"testing"
)

// SessionIDOfTranscript and PromptBreakdown replace four hand-written searches
// that each asked whether the session id appeared SOMEWHERE in the path. These
// tests pin the question they ask instead: which session does this path name.
//
// The cases that matter are the ones the substring test got wrong, so each is
// written as an id that a substring search would have answered and an exact one
// refuses.

func TestSessionIDOfTranscriptNamesTheSessionDirectory(t *testing.T) {
	root := filepath.Join("/repos/inber", "logs")
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			"current layout",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana", "session.jsonl"),
			"2026-08-02_010000_cana",
		},
		{
			"no agent segment",
			filepath.Join(root, "2026-08-02_010000_cana", "session.jsonl"),
			"2026-08-02_010000_cana",
		},
		{
			"sub-agent session keeps its suffix",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana-sub", "session.jsonl"),
			"2026-08-02_010000_cana-sub",
		},
		{
			"legacy flat layout",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana.jsonl"),
			"2026-08-02_010000_cana",
		},
		{
			"the server's own error log is not a session",
			filepath.Join(root, ServerErrorsFile),
			"",
		},
		{
			"a file that is not a transcript names no session",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana", "messages.json"),
			"",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SessionIDOfTranscript(c.path); got != c.want {
				t.Errorf("SessionIDOfTranscript(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}

// The defect, stated as the property it broke. A session id is a timestamp and
// four hex characters, so a truncated one — from a URL, from a hand-typed id —
// is a prefix of a longer session's id, and the old search served that longer
// session's transcript under the shorter id with no error.
func TestAPrefixOfASessionIDNamesNoSession(t *testing.T) {
	path := filepath.Join("/repos/inber", "logs", "claxon", "2026-08-02_010000_cana", "session.jsonl")

	if got := SessionIDOfTranscript(path); got == "2026-08-02_010000_ca" {
		t.Fatalf("a prefix id resolved to the longer session: %q", got)
	}
	if got := SessionIDOfTranscript(path); got != "2026-08-02_010000_cana" {
		t.Fatalf("SessionIDOfTranscript = %q, want the full id", got)
	}
}

// The second way the substring test was wrong: it matched the id against every
// directory above the session too, so an agent — or a workspace path — whose
// name held an id claimed every session under it.
func TestASessionIsNotNamedByTheDirectoriesAboveIt(t *testing.T) {
	// An agent directory named exactly like a session id.
	path := filepath.Join("/repos/inber", "logs", "2026-08-02_010000_cana", "2026-08-02_020000_beef", "session.jsonl")

	if got := SessionIDOfTranscript(path); got != "2026-08-02_020000_beef" {
		t.Errorf("SessionIDOfTranscript = %q, want the session directory's name, not an ancestor's", got)
	}
}

func TestPromptBreakdownNamesItsSessionAndTurn(t *testing.T) {
	root := filepath.Join("/repos/inber", "logs")
	cases := []struct {
		name        string
		path        string
		wantSession string
		wantTurn    int
		wantOK      bool
	}{
		{
			"current layout",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana", "prompts", "turn-3.md"),
			"2026-08-02_010000_cana", 3, true,
		},
		{
			"legacy layout shares one prompts directory per agent",
			filepath.Join(root, "claxon", "prompts", "2026-08-02_010000_cana-turn-3.md"),
			"2026-08-02_010000_cana", 3, true,
		},
		{
			"a sub-agent id holds a hyphen, so the split is at the last turn marker",
			filepath.Join(root, "claxon", "prompts", "2026-08-02_010000_cana-sub-turn-12.md"),
			"2026-08-02_010000_cana-sub", 12, true,
		},
		{
			"the shared system prompt is not a turn",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana", "prompts", "system.md"),
			"", 0, false,
		},
		{
			"a system block named after its content is not a turn either",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana", "prompts", "system-44-tags-prompts-turn-1mdextmd.md"),
			"", 0, false,
		},
		{
			"a turn file outside a prompts directory belongs to nothing",
			filepath.Join(root, "claxon", "2026-08-02_010000_cana", "turn-3.md"),
			"", 0, false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotSession, gotTurn, gotOK := PromptBreakdown(c.path)
			if gotOK != c.wantOK || gotSession != c.wantSession || gotTurn != c.wantTurn {
				t.Errorf("PromptBreakdown(%q) = (%q, %d, %v), want (%q, %d, %v)",
					c.path, gotSession, gotTurn, gotOK, c.wantSession, c.wantTurn, c.wantOK)
			}
		})
	}
}
