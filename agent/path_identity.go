package agent

import (
	"path/filepath"
	"strings"
)

// ResolvePathAgainstRoot joins a relative path onto root. An absolute path, and
// a path the model anchored at "~", are returned as written: both are the model
// naming somewhere by itself rather than relative to where it is working.
//
// This is the rule by which a path the model wrote becomes the path a tool acts
// on, and it has two readers. tools.ScopeToRoot rewrites a tool call's
// arguments with it, so the tool opens the right file. ReadCache identifies a
// file with it, so two spellings of one file meet on one cache key. Those two
// have to agree — a cache that identifies files by a different rule than the
// tools resolve them by is not identifying files at all — so the rule lives
// here, once, rather than in each of them.
func ResolvePathAgainstRoot(given, root string) string {
	if root == "" {
		return given
	}
	if filepath.IsAbs(given) || given == "~" || strings.HasPrefix(given, "~/") {
		return given
	}
	return filepath.Join(root, given)
}

// lexicalPathIdentity is the absolute, cleaned path a tool call names, derived
// without touching the filesystem. It is what the tool itself will open:
// ResolvePathAgainstRoot for the root, then filepath.Abs for the process
// working directory the tools fall back to when no root is known.
//
// A "~" the model wrote is deliberately NOT expanded, because the file tools do
// not expand it either — they hand the string to os.ReadFile, which reads a
// directory literally named "~" under the working directory. Abs says exactly
// that, so the identity stays a claim about the file the tool opens rather than
// a guess about the one the model meant.
func lexicalPathIdentity(given, root string) string {
	resolved := ResolvePathAgainstRoot(given, root)
	if absolute, err := filepath.Abs(resolved); err == nil {
		return absolute
	}
	return filepath.Clean(resolved)
}

// canonicalPathIdentity follows symlinks so that a file reached by two routes
// is one file. It returns "" when the path cannot be canonicalized — most often
// because it does not exist yet, which is every create — and callers then fall
// back to the lexical identity rather than admitting a key they cannot trust.
func canonicalPathIdentity(lexical string) string {
	canonical, err := filepath.EvalSymlinks(lexical)
	if err != nil {
		return ""
	}
	return canonical
}
