package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The read cache used to key on the path exactly as the model wrote it, with no
// resolution of any kind. The tools resolve a relative path against the session
// root before opening it, so one file the model named two ways was two entries —
// and the failure that made it a correctness bug rather than a cost one is the
// stale hit: read a file relatively, edit it absolutely, read it relatively
// again, and the cache answers "already in context" over content the edit has
// already replaced.
//
// These tests use a real directory and a real symlink. The upstream bug this
// mirrors (goose #10545) survived its own tests precisely because they compared
// hardcoded path strings and never touched a filesystem.

// cacheAtRoot returns a cache rooted at a real temporary directory holding one
// file, plus the root and the file's name within it.
func cacheAtRoot(t *testing.T) (cache *ReadCache, root, name string) {
	t.Helper()
	root = t.TempDir()
	// The temporary directory is itself reached through a symlink on macOS and
	// on any host with a symlinked /tmp, which is exactly the case under test —
	// resolve it so the test's own expectations are about one file, not two.
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", root, err)
	}
	root = resolvedRoot
	name = "lifecycle.go"
	if err := os.WriteFile(filepath.Join(root, name), []byte("package engine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache = NewReadCache()
	cache.SetRoot(root)
	return cache, root, name
}

func TestAWriteSpelledAbsolutelyInvalidatesAReadSpelledRelatively(t *testing.T) {
	cache, root, name := cacheAtRoot(t)

	cache.RecordFullRead(name, 12)
	cache.Invalidate(filepath.Join(root, name))

	if stub, cached := cache.Check(name); cached {
		t.Fatalf("relative read hit the cache after an absolute write invalidated it: %q\n"+
			"the model is told the file is in context; what is in context is the pre-edit content", stub)
	}
}

func TestAWriteSpelledRelativelyInvalidatesAReadSpelledAbsolutely(t *testing.T) {
	cache, root, name := cacheAtRoot(t)

	cache.RecordFullRead(filepath.Join(root, name), 12)
	cache.Invalidate(name)

	if stub, cached := cache.Check(filepath.Join(root, name)); cached {
		t.Fatalf("absolute read hit the cache after a relative write invalidated it: %q", stub)
	}
}

func TestDotSlashIsTheSameFileAsTheBareName(t *testing.T) {
	cache, _, name := cacheAtRoot(t)

	cache.RecordFullRead("./"+name, 12)

	if _, cached := cache.Check(name); !cached {
		t.Fatalf("%q and %q are the same file and must share one cache entry; "+
			"two entries means the file is re-read in full, which is the cost the cache exists to avoid", "./"+name, name)
	}
}

// Both halves matter and the negative one alone is not enough: a cache that
// records under one identity and looks up under another passes "the write
// invalidated the read" for the wrong reason — nothing was ever findable. So
// assert the hit first, then the invalidation.
func TestASymlinkedPathIsTheSameFileAsItsTarget(t *testing.T) {
	cache, root, name := cacheAtRoot(t)

	linkedDirectory := filepath.Join(root, "link")
	if err := os.Symlink(root, linkedDirectory); err != nil {
		t.Skipf("this host does not allow symlinks: %v", err)
	}
	throughLink := filepath.Join(linkedDirectory, name)

	cache.RecordFullRead(throughLink, 12)

	if _, cached := cache.Check(name); !cached {
		t.Fatalf("%q and %q are the same file and must share one entry", throughLink, name)
	}

	cache.Invalidate(name)

	if stub, cached := cache.Check(throughLink); cached {
		t.Fatalf("a read through a symlink survived a write to its target: %q", stub)
	}
}

// A file that has just been deleted cannot be resolved, so a fallback to the
// lexical path would give one file two identities across its own lifetime:
// filed under the resolved path while it existed, invalidated under the
// unresolved one afterwards. The directory is resolved instead. This only bites
// where the two differ, which is any session working inside a symlinked root —
// a forge worktree, for one — so the test builds exactly that.
func TestAFileDeletedMidTurnIsStillInvalidatedThroughASymlinkedRoot(t *testing.T) {
	const name = "lifecycle.go"

	target := t.TempDir()
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resolvedTarget, name), []byte("package engine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkedRoot := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(resolvedTarget, symlinkedRoot); err != nil {
		t.Skipf("this host does not allow symlinks: %v", err)
	}

	cache := NewReadCache()
	cache.SetRoot(symlinkedRoot)

	cache.RecordFullRead(name, 12)

	// The file is gone at the moment the write is announced — a shell step
	// removed it, or the write is a replace — so this is where an identity that
	// needs the file to exist stops matching the one the read was filed under.
	if err := os.Remove(filepath.Join(resolvedTarget, name)); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate(name)

	// And then the write puts it back, which is what makes a missed
	// invalidation visible: the path resolves again, the old entry is found
	// again, and the model is told the new file is the one it already read.
	if err := os.WriteFile(filepath.Join(resolvedTarget, name), []byte("package engine // rewritten\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if stub, cached := cache.Check(name); cached {
		t.Fatalf("a file deleted and rewritten mid-turn kept its cache entry: %q\n"+
			"the model is told the rewritten file is already in context", stub)
	}
}

// A cache with no root is what a sub-agent built through agent/registry gets,
// and its tools resolve relative paths against the process working directory.
// The cache has to model that same rule, or the two spellings part company
// again in exactly the place nobody is looking.
func TestWithNoRootThePathsStillMeetOnTheProcessWorkingDirectory(t *testing.T) {
	cache := NewReadCache()

	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cache.RecordFullRead("read_cache.go", 12)
	cache.Invalidate(filepath.Join(working, "read_cache.go"))

	if _, cached := cache.Check("read_cache.go"); cached {
		t.Fatal("with no root, an absolute write did not invalidate a relative read")
	}
}

// The stub is the only thing the model is told about the cache, so it must not
// state anything the cache cannot know. It used to report a turn number that
// was really the tool-call index within the current turn — "read on turn 2" for
// a file read as the second tool call of turn 40, forever, and biased low.
func TestTheStubClaimsNoTurnNumber(t *testing.T) {
	cache, _, name := cacheAtRoot(t)
	cache.RecordFullRead(name, 12)

	stub, cached := cache.Check(name)
	if !cached {
		t.Fatal("the file was just recorded and did not hit")
	}
	if strings.Contains(stub, "turn 1") || strings.Contains(stub, "on turn") {
		t.Fatalf("the stub states a turn number it cannot know: %q", stub)
	}
	if !strings.Contains(stub, "12 lines") {
		t.Fatalf("the stub dropped the line count, which is the part that is true: %q", stub)
	}
}
