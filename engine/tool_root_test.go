package engine

import (
	"context"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// toolRecordingItsArguments returns a tool that records the arguments it is run with.
func toolRecordingItsArguments(name string, recorded *string) agent.Tool {
	return agent.Tool{
		Name: name,
		Run: func(ctx context.Context, raw string) (string, error) {
			*recorded = raw
			return "", nil
		},
	}
}

func runInstalledTool(t *testing.T, e *Engine, name, arguments string) {
	t.Helper()
	for _, installed := range e.agentTools {
		if installed.Name == name {
			if _, err := installed.Run(context.Background(), arguments); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("%s was not installed", name)
}

// The rooting has to survive the extras merge. The server injects tools through
// EngineConfig.ExtraTools after buildTools has run, and an injected tool
// replaces a built-in one of the same name — so a root applied inside
// buildTools would be dropped by an injected write_files without a word.
func TestAnInjectedToolIsRootedLikeABuiltInOne(t *testing.T) {
	var recorded string
	e := &Engine{repoRoot: "/work/tree"}
	e.setToolSet(mergeExtraTools(
		namedTools("read_files", "shell_commands"),
		[]agent.Tool{toolRecordingItsArguments("write_files", &recorded)},
	))

	runInstalledTool(t, e, "write_files", `{"path":"server/spawn.go","content":"package server"}`)

	const want = `{"content":"package server","path":"/work/tree/server/spawn.go"}`
	if recorded != want {
		t.Errorf("the injected write_files was run with %s, want %s", recorded, want)
	}
}

// An injected tool that replaces a built-in one by name must be rooted too,
// not just an injected tool with a new name.
func TestAnInjectedReplacementOfABuiltInToolIsRooted(t *testing.T) {
	var recorded string
	e := &Engine{repoRoot: "/work/tree"}
	e.setToolSet(mergeExtraTools(
		[]agent.Tool{toolRecordingItsArguments("read_files", &recorded)},
		[]agent.Tool{toolRecordingItsArguments("read_files", &recorded)},
	))

	runInstalledTool(t, e, "read_files", `{"path":"a.go"}`)

	const want = `{"path":"/work/tree/a.go"}`
	if recorded != want {
		t.Errorf("the replacing read_files was run with %s, want %s", recorded, want)
	}
}

// A session with no root — inber started somewhere that is not a repository —
// keeps the behaviour it has always had rather than being given a guess.
func TestASessionWithNoRootLeavesToolArgumentsAlone(t *testing.T) {
	var recorded string
	e := &Engine{}
	e.setToolSet([]agent.Tool{toolRecordingItsArguments("write_files", &recorded)})

	runInstalledTool(t, e, "write_files", `{"path":"a.go","content":"x"}`)

	if recorded != `{"path":"a.go","content":"x"}` {
		t.Errorf("arguments = %s, want them untouched", recorded)
	}
}
