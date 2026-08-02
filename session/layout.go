package session

import (
	"path/filepath"
	"strconv"
	"strings"
)

// ServerErrorsFile is the one file directly under a logs root that is not a
// session transcript. It is named here rather than at the places that skip it
// because every reader of the root has to know to skip it, and two of them
// already disagreed about whether they did.
const ServerErrorsFile = "server-errors.jsonl"

// LogsRoot returns the directory a session working in repoRoot writes its
// transcripts under. New owns everything below it — the agent segment, the
// session directory and the file name — so this is the whole of the join a
// caller ever has to do.
//
// It is one exported function because the same join is made from both ends and
// the two ends have already disagreed. engine's setupSession builds it to write
// a session; the server's history endpoint builds it to find one. When a writer
// and a reader each spell out a layout, a change to either is a silent change
// to what the other can see: sessions written to a root the reader does not
// derive are not reported missing, they are reported as not existing.
func LogsRoot(repoRoot string) string {
	return filepath.Join(repoRoot, "logs")
}

// SessionIDOfTranscript names the session a transcript file belongs to, and
// returns "" for a path that is not a transcript.
//
// It is the inverse of the join New makes, and it exists because four separate
// searches used to answer this question by asking whether the session id
// appeared ANYWHERE in the path. That is a different question, and it is
// answered wrong three ways. A session id is <timestamp>_<4 hex>, so a
// truncated or hand-typed id from a URL is a prefix of a longer one and the
// shorter query returns the longer session's transcript. An agent directory —
// or any directory above it — whose name holds an id matches every session
// under it. And each of those searches took the first hit in walk order, so the
// wrong transcript was served as the right one with no error anywhere.
//
// Deriving the id from the path instead means the readers spell the layout the
// way the listing already did: the session directory's name IS the id.
func SessionIDOfTranscript(path string) string {
	name := filepath.Base(path)
	if name == "session.jsonl" {
		// <logs root>/<agent>/<session>/session.jsonl — the layout New writes.
		// The agent segment is absent for a session with no agent name, which
		// changes the depth but never the parent.
		return filepath.Base(filepath.Dir(path))
	}
	if name == ServerErrorsFile || !strings.HasSuffix(name, ".jsonl") {
		return ""
	}
	// Legacy flat layout: <logs root>/<agent>/<session>.jsonl.
	return strings.TrimSuffix(name, ".jsonl")
}

// PromptBreakdown names the session and turn a prompt-breakdown file belongs
// to, and reports false for any other file.
//
// Both layouts put the breakdowns in a prompts/ directory, and which session
// they belong to is spelled differently in each: the current one names the
// session directory above prompts/ and the file only turn-N.md, the legacy one
// shares one prompts/ directory per agent and puts the id in the file name.
// Requiring the turn to be a number is what keeps this from claiming the other
// files prompts/ holds — system.md, tools.md and the system-NN-*.md blocks,
// which are named after their content and so can contain anything at all,
// including something that reads like a turn.
func PromptBreakdown(path string) (sessionID string, turn int, ok bool) {
	name := filepath.Base(path)
	dir := filepath.Dir(path)
	if filepath.Base(dir) != "prompts" {
		return "", 0, false
	}

	if rest, isCurrent := strings.CutPrefix(name, "turn-"); isCurrent {
		n, numbered := turnNumber(rest)
		if !numbered {
			return "", 0, false
		}
		// <session>/prompts/turn-N.md
		return filepath.Base(filepath.Dir(dir)), n, true
	}

	// Legacy: <agent>/prompts/<session>-turn-N.md. The split cannot be at the
	// first hyphen, because the id holds hyphens of its own — the date, and the
	// "-sub" a sub-agent session ends in. It is at the LAST turn marker rather
	// than the first only as a matter of which end owns the marker: this layout
	// appends it, so the trailing one is always the one it wrote. No session id
	// contains a turn marker today, so the two readings cannot be told apart on
	// any real name, and no test here distinguishes them.
	marker := strings.LastIndex(name, "-turn-")
	if marker <= 0 {
		return "", 0, false
	}
	n, numbered := turnNumber(name[marker+len("-turn-"):])
	if !numbered {
		return "", 0, false
	}
	return name[:marker], n, true
}

// turnNumber reads the "N.md" tail of a breakdown file name.
func turnNumber(tail string) (int, bool) {
	digits, isMarkdown := strings.CutSuffix(tail, ".md")
	if !isMarkdown || digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
