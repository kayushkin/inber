package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// recordArguments returns a tool named name whose Run records the arguments it
// was handed, so a test can assert on what reached the real tool rather than on
// what the wrapper was given.
func recordArguments(name string, recorded *string) agent.Tool {
	return agent.Tool{
		Name: name,
		Run: func(ctx context.Context, raw string) (string, error) {
			*recorded = raw
			return "", nil
		},
	}
}

// runScoped runs the named tool with the given arguments through ScopeToRoot
// and returns what the underlying tool received, decoded.
func runScoped(t *testing.T, name, root, arguments string) map[string]any {
	t.Helper()
	var recorded string
	scoped := ScopeToRoot([]agent.Tool{recordArguments(name, &recorded)}, root)
	if _, err := scoped[0].Run(context.Background(), arguments); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(recorded), &got); err != nil {
		t.Fatalf("%s passed on unparseable arguments %q: %v", name, recorded, err)
	}
	return got
}

func TestRelativeSinglePathIsResolvedAgainstTheRoot(t *testing.T) {
	for _, name := range []string{"read_files", "write_files", "edit_files", "list_files"} {
		got := runScoped(t, name, "/work/tree", `{"path":"server/spawn.go"}`)
		if got["path"] != "/work/tree/server/spawn.go" {
			t.Errorf("%s: path = %v, want /work/tree/server/spawn.go", name, got["path"])
		}
	}
}

func TestReadFilesResolvesEveryPathInTheBatch(t *testing.T) {
	got := runScoped(t, "read_files", "/work/tree",
		`{"paths":["a.go","sub/b.go","/etc/hosts"]}`)
	want := []any{"/work/tree/a.go", "/work/tree/sub/b.go", "/etc/hosts"}
	paths, _ := got["paths"].([]any)
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %v, want %v", i, paths[i], want[i])
		}
	}
}

func TestWriteFilesResolvesThePathInsideEveryEntry(t *testing.T) {
	got := runScoped(t, "write_files", "/work/tree",
		`{"files":[{"path":"a.go","content":"package a"},{"path":"/tmp/b.go","content":"package b"}]}`)
	files, _ := got["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("files = %v, want 2 entries", files)
	}
	first, _ := files[0].(map[string]any)
	if first["path"] != "/work/tree/a.go" {
		t.Errorf("files[0].path = %v, want /work/tree/a.go", first["path"])
	}
	if first["content"] != "package a" {
		t.Errorf("files[0].content = %v — the wrapper must not touch anything but paths", first["content"])
	}
	second, _ := files[1].(map[string]any)
	if second["path"] != "/tmp/b.go" {
		t.Errorf("files[1].path = %v, want the absolute path unchanged", second["path"])
	}
}

func TestEditFilesResolvesThePathInsideEveryEdit(t *testing.T) {
	got := runScoped(t, "edit_files", "/work/tree",
		`{"edits":[{"path":"a.go","old_text":"x","new_text":"y"}]}`)
	edits, _ := got["edits"].([]any)
	if len(edits) != 1 {
		t.Fatalf("edits = %v, want 1 entry", edits)
	}
	edit, _ := edits[0].(map[string]any)
	if edit["path"] != "/work/tree/a.go" {
		t.Errorf("edits[0].path = %v, want /work/tree/a.go", edit["path"])
	}
	if edit["old_text"] != "x" || edit["new_text"] != "y" {
		t.Errorf("edit text was altered: %v", edit)
	}
}

// An absent workdir is what every shell call the model writes actually looks
// like, and defaulting it to the root is the one piece of rooting inber already
// had. It must survive the move.
func TestShellWithoutAWorkdirRunsInTheRoot(t *testing.T) {
	got := runScoped(t, "shell_commands", "/work/tree", `{"command":"go build ./..."}`)
	if got["workdir"] != "/work/tree" {
		t.Errorf("workdir = %v, want /work/tree", got["workdir"])
	}
	if got["command"] != "go build ./..." {
		t.Errorf("command = %v, want it untouched", got["command"])
	}
}

func TestShellWithARelativeWorkdirRunsUnderTheRoot(t *testing.T) {
	got := runScoped(t, "shell_commands", "/work/tree", `{"command":"ls","workdir":"server"}`)
	if got["workdir"] != "/work/tree/server" {
		t.Errorf("workdir = %v, want /work/tree/server", got["workdir"])
	}
}

func TestListFilesWithNoPathListsTheRoot(t *testing.T) {
	got := runScoped(t, "list_files", "/work/tree", `{"recursive":true}`)
	if got["path"] != "/work/tree" {
		t.Errorf("path = %v, want /work/tree", got["path"])
	}
	if got["recursive"] != true {
		t.Errorf("recursive = %v, want it untouched", got["recursive"])
	}
}

func TestAnAbsolutePathIsLeftAsTheModelWroteIt(t *testing.T) {
	got := runScoped(t, "read_files", "/work/tree", `{"path":"/etc/hosts"}`)
	if got["path"] != "/etc/hosts" {
		t.Errorf("path = %v, want /etc/hosts", got["path"])
	}
}

func TestAHomeAnchoredPathIsLeftAsTheModelWroteIt(t *testing.T) {
	for _, given := range []string{"~", "~/.config/inber/config.yaml"} {
		got := runScoped(t, "read_files", "/work/tree", `{"path":"`+given+`"}`)
		if got["path"] != given {
			t.Errorf("path = %v, want %q — \"~\" names somewhere by itself", got["path"], given)
		}
	}
}

func TestFieldsTheToolOwnsSurviveUntouched(t *testing.T) {
	got := runScoped(t, "read_files", "/work/tree", `{"path":"a.go","offset":10,"limit":40}`)
	if got["offset"] != float64(10) || got["limit"] != float64(40) {
		t.Errorf("offset/limit = %v/%v, want 10/40", got["offset"], got["limit"])
	}
}

func TestAToolThatNamesNoPathIsReturnedUnchanged(t *testing.T) {
	var recorded string
	original := recordArguments("web_search", &recorded)
	scoped := ScopeToRoot([]agent.Tool{original}, "/work/tree")
	if _, err := scoped[0].Run(context.Background(), `{"query":"path"}`); err != nil {
		t.Fatal(err)
	}
	if recorded != `{"query":"path"}` {
		t.Errorf("arguments = %q, want them untouched", recorded)
	}
}

// No root means inber does not know where the session is working. Substituting
// a guess would move files rather than fix them.
func TestNoRootLeavesEveryToolAlone(t *testing.T) {
	var recorded string
	scoped := ScopeToRoot([]agent.Tool{recordArguments("write_files", &recorded)}, "")
	if _, err := scoped[0].Run(context.Background(), `{"path":"a.go","content":"x"}`); err != nil {
		t.Fatal(err)
	}
	if recorded != `{"path":"a.go","content":"x"}` {
		t.Errorf("arguments = %q, want them untouched", recorded)
	}
}

// Arguments the tool itself will reject are its own to report on. A wrapper the
// model was never told about must not answer for them.
func TestUnparseableArgumentsReachTheToolUnchanged(t *testing.T) {
	var recorded string
	scoped := ScopeToRoot([]agent.Tool{recordArguments("read_files", &recorded)}, "/work/tree")
	if _, err := scoped[0].Run(context.Background(), `{"path": `); err != nil {
		t.Fatal(err)
	}
	if recorded != `{"path": ` {
		t.Errorf("arguments = %q, want them passed through for the tool to reject", recorded)
	}
}

func TestAToolWithNoRunIsNotWrapped(t *testing.T) {
	scoped := ScopeToRoot([]agent.Tool{{Name: "read_files"}}, "/work/tree")
	if scoped[0].Run != nil {
		t.Error("a tool with no Run was given one")
	}
}

// The guard on the table: every filesystem tool inber offers must say where its
// paths are, or it silently keeps resolving them against the server's own
// working directory.
func TestEveryFilesystemToolDeclaresItsPathArguments(t *testing.T) {
	var offered []string
	for _, tool := range All() {
		offered = append(offered, tool.Name)
	}
	var declared []string
	for name := range filesystemToolPathArguments {
		declared = append(declared, name)
	}
	sort.Strings(offered)
	sort.Strings(declared)
	if strings.Join(offered, ",") != strings.Join(declared, ",") {
		t.Errorf("tools.All() offers [%s] but filesystemToolPathArguments declares [%s]\n"+
			"a filesystem tool missing from the table resolves relative paths against "+
			"the inber-server process working directory, not the session's root",
			strings.Join(offered, ", "), strings.Join(declared, ", "))
	}
}

// The defect this file was written for, end to end: the real write_files, a
// relative path, and a root that is not the process working directory.
func TestTheRealWriteFilesLandsInsideTheRootAndNotTheProcessDirectory(t *testing.T) {
	root := t.TempDir()
	processDirectory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(processDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(previous) })

	scoped := ScopeToRoot([]agent.Tool{WriteFiles()}, root)
	if _, err := scoped[0].Run(context.Background(),
		`{"path":"server/spawn.go","content":"package server"}`); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(filepath.Join(root, "server", "spawn.go"))
	if err != nil {
		t.Fatalf("the file did not land in the root: %v", err)
	}
	if string(written) != "package server" {
		t.Errorf("content = %q", written)
	}
	if _, err := os.Stat(filepath.Join(processDirectory, "server", "spawn.go")); err == nil {
		t.Error("the file also landed in the process working directory")
	}
}
