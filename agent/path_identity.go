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
// is one file.
//
// A file that does not exist cannot be resolved, and that case is not exotic:
// it is every create, and it is every invalidation of a file something has just
// deleted. Falling straight back to the lexical path there would give one file
// two identities across its own lifetime — recorded under the resolved path
// while it existed, invalidated under the unresolved one after — which is the
// stale hit this whole file exists to stop. So the directory is resolved
// instead, which needs no file, and the name is joined back on.
//
// It returns "" only when the directory cannot be resolved either. Callers then
// fall back to the lexical identity rather than admitting a key they cannot
// trust; a read cannot have succeeded under a directory that is not there, so
// nothing is filed under such a key in the first place.
func canonicalPathIdentity(lexical string) string {
	if canonical, err := filepath.EvalSymlinks(lexical); err == nil {
		return canonical
	}
	directory, name := filepath.Split(lexical)
	canonicalDirectory, err := filepath.EvalSymlinks(filepath.Clean(directory))
	if err != nil {
		return ""
	}
	return filepath.Join(canonicalDirectory, name)
}
