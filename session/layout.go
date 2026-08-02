package session

import "path/filepath"

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
