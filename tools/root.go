package tools

import (
	"context"
	"encoding/json"

	"github.com/kayushkin/inber/agent"
)

// A filesystem tool's arguments name paths in one of three shapes, and which
// shape a given tool uses is fixed by that tool's schema.
type filesystemPathArguments struct {
	// singlePathFields are top-level string fields holding one path.
	singlePathFields []string
	// pathListFields are top-level array-of-string fields, every element a path.
	pathListFields []string
	// pathsInsideObjectLists maps an array-of-object field to the key inside
	// each element that holds a path — write_files' files[].path, for example.
	pathsInsideObjectLists map[string]string
	// fieldsThatDefaultToRoot are the single-path fields whose absence means
	// "the root", rather than "no path was given". Only two fields qualify:
	// shell_commands' workdir and list_files' path both fall back to the
	// process working directory inside the tool itself, which is exactly the
	// leak this file exists to close.
	fieldsThatDefaultToRoot []string
}

// filesystemToolPathArguments is the closed set of tools whose arguments name a
// path on this host's disk. A tool absent from this table is handed to the
// model unchanged, so a new filesystem tool must be added here — the test
// TestEveryFilesystemToolDeclaresItsPathArguments fails if one is not.
var filesystemToolPathArguments = map[string]filesystemPathArguments{
	"shell_commands": {
		singlePathFields:        []string{"workdir"},
		fieldsThatDefaultToRoot: []string{"workdir"},
	},
	"read_files": {
		singlePathFields: []string{"path"},
		pathListFields:   []string{"paths"},
	},
	"write_files": {
		singlePathFields:       []string{"path"},
		pathsInsideObjectLists: map[string]string{"files": "path"},
	},
	"edit_files": {
		singlePathFields:       []string{"path"},
		pathsInsideObjectLists: map[string]string{"edits": "path"},
	},
	"list_files": {
		singlePathFields:        []string{"path"},
		fieldsThatDefaultToRoot: []string{"path"},
	},
}

// ScopeToRoot resolves every relative filesystem path in these tools' arguments
// against root, and returns the tools with that resolution applied.
//
// This is the one place a tool acquires a working directory, and every tool the
// engine offers the model passes through it. Before it existed, only
// shell_commands was given a root: it took a default workdir, so a spawned
// agent's commands ran inside its forge worktree, while read_files/write_files/
// edit_files/list_files took a bare path with nothing to join it against and so
// resolved relative paths against the inber-server process's own working
// directory. One agent, one workspace, and two different meanings for
// "server/spawn.go" — commands read and edited a file that writes never
// touched, with no error on either side saying so.
//
// An absolute path is left exactly as written, as is a path starting with "~":
// both are the model naming somewhere by itself rather than relative to where
// it is working, and shell_commands has always let a command leave its workdir.
// Confining a tool call to the root is a separate and stricter promise, and
// this function does not make it.
//
// A root of "" means no root is known — inber run from a directory that is not
// a repository, for one — and the tools are returned untouched. There is no
// better default to invent: the process working directory is what they already
// use, and substituting a guess for it would move files rather than fix them.
func ScopeToRoot(toolSet []agent.Tool, root string) []agent.Tool {
	if root == "" {
		return toolSet
	}
	scoped := make([]agent.Tool, len(toolSet))
	for i, t := range toolSet {
		scoped[i] = scopeOneToolToRoot(t, root)
	}
	return scoped
}

// scopeOneToolToRoot returns t with its path arguments resolved against root,
// or t unchanged if it names no paths.
func scopeOneToolToRoot(t agent.Tool, root string) agent.Tool {
	pathArguments, isFilesystemTool := filesystemToolPathArguments[t.Name]
	if !isFilesystemTool || t.Run == nil {
		return t
	}
	runWithBarePaths := t.Run
	t.Run = func(ctx context.Context, raw string) (string, error) {
		return runWithBarePaths(ctx, resolveArgumentPaths(raw, root, pathArguments))
	}
	return t
}

// resolveArgumentPaths rewrites raw — a tool call's JSON arguments — so that
// every path it names is resolved against root, leaving every other field
// exactly as it arrived.
//
// Arguments that are not the JSON object the tool's schema describes are
// returned unchanged, so the tool itself reports the parse failure. Reporting
// it from here would replace the tool's own message about its own arguments
// with a worse one from a wrapper the model was never told about.
func resolveArgumentPaths(raw, root string, pathArguments filesystemPathArguments) string {
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return raw
	}

	changed := false

	for _, field := range pathArguments.singlePathFields {
		given, _ := arguments[field].(string)
		if given == "" {
			continue
		}
		if resolved := resolvePathAgainstRoot(given, root); resolved != given {
			arguments[field] = resolved
			changed = true
		}
	}

	for _, field := range pathArguments.fieldsThatDefaultToRoot {
		value, present := arguments[field]
		if present {
			// A field of the wrong type is left for the tool to reject. Only
			// "absent" and "empty" mean the root.
			if given, isString := value.(string); !isString || given != "" {
				continue
			}
		}
		arguments[field] = root
		changed = true
	}

	for _, field := range pathArguments.pathListFields {
		elements, ok := arguments[field].([]any)
		if !ok {
			continue
		}
		for i, element := range elements {
			given, ok := element.(string)
			if !ok || given == "" {
				continue
			}
			if resolved := resolvePathAgainstRoot(given, root); resolved != given {
				elements[i] = resolved
				changed = true
			}
		}
	}

	for field, keyHoldingPath := range pathArguments.pathsInsideObjectLists {
		elements, ok := arguments[field].([]any)
		if !ok {
			continue
		}
		for _, element := range elements {
			object, ok := element.(map[string]any)
			if !ok {
				continue
			}
			given, _ := object[keyHoldingPath].(string)
			if given == "" {
				continue
			}
			if resolved := resolvePathAgainstRoot(given, root); resolved != given {
				object[keyHoldingPath] = resolved
				changed = true
			}
		}
	}

	if !changed {
		return raw
	}
	rewritten, err := json.Marshal(arguments)
	if err != nil {
		return raw
	}
	return string(rewritten)
}

// resolvePathAgainstRoot joins a relative path onto root. An absolute path, and
// a path the model anchored at "~", are returned as written — see ScopeToRoot
// for why those two are the model's to choose and not this function's to move.
//
// The rule itself lives in agent.ResolvePathAgainstRoot because the read cache
// identifies files by it too, and a cache that resolves paths differently from
// the tools would file one file under two keys.
func resolvePathAgainstRoot(given, root string) string {
	return agent.ResolvePathAgainstRoot(given, root)
}
