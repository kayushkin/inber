package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// The read cache decides whether a file is fully in context by parsing a
// footer that tool-store's read tool writes — extractCompleteFileLines reads
// "[complete file — N lines]". That is a contract between two repos, and
// nothing pinned it, so tool-store could call a truncated read complete and
// this package would believe it. It did: a read past the byte cap was
// labelled complete, the cache recorded it, and Check then answered every
// later read of that path with a stub — including the offset/limit read the
// truncation notice itself tells the model to make. The rest of the file
// became unreachable for the remainder of the session.
//
// These tests run the real read tool rather than a hand-written footer, so
// they fail if either side of the contract moves.

func readViaToolStore(t *testing.T, in map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := toolstoretools.ReadFile().Run(context.Background(), string(raw))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return out
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTruncatedReadDoesNotEnterTheReadCache(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		// One line, past the byte cap: a minified bundle. The line window
		// never fires, so only the byte cap cut this.
		{"past the byte cap", strings.Repeat("x", 500_000)},
		// Past the line window: the read keeps the first 500 and last 50.
		{"past the line window", strings.Repeat("line\n", 3000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFile(t, "big.txt", tc.body)
			out := readViaToolStore(t, map[string]any{"path": path})

			if lines := extractCompleteFileLines(out); lines != 0 {
				t.Fatalf("a truncated read was read as complete (%d lines); footer was %q",
					lines, out[len(out)-120:])
			}

			// And so the cache stays empty, and a later ranged read runs.
			cache := NewReadCache()
			if lines := extractCompleteFileLines(out); lines > 0 {
				cache.RecordFullRead(path, 1, lines)
			}
			if stub, cached := cache.Check(path); cached {
				t.Errorf("later reads of a truncated file are stubbed with %q", stub)
			}
		})
	}
}

func TestCompleteReadStillEntersTheReadCache(t *testing.T) {
	// The complement: blocking re-reads of a file that genuinely is in
	// context is the whole point of the cache, so the tests above must not be
	// passing because nothing is cached any more.
	path := writeFile(t, "small.go", "package main\n\nfunc main() {}\n")
	out := readViaToolStore(t, map[string]any{"path": path})

	lines := extractCompleteFileLines(out)
	if lines != 3 {
		t.Fatalf("a complete 3-line read reported %d lines; footer was %q", lines, out)
	}

	cache := NewReadCache()
	cache.RecordFullRead(path, 1, lines)
	stub, cached := cache.Check(path)
	if !cached {
		t.Fatalf("a complete read was not cached")
	}
	if !strings.Contains(stub, "3 lines") {
		t.Errorf("stub does not carry the line count: %q", stub)
	}
}

func TestRangedReadIsNeverTakenAsComplete(t *testing.T) {
	// A read that asked for a range says so in a different footer, and that
	// one must not parse as complete either.
	path := writeFile(t, "f.txt", "a\nb\nc\n")
	out := readViaToolStore(t, map[string]any{"path": path, "offset": 2})
	if lines := extractCompleteFileLines(out); lines != 0 {
		t.Errorf("a ranged read reported %d complete lines; footer was %q", lines, out)
	}
}
