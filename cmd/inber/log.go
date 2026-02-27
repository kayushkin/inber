package main

import (
	"fmt"
	"os"
)

// Logger provides structured stderr logging with consistent formatting.
// All output goes to stderr so stdout stays clean for piping.
type Logger struct{}

// Log is the package-level logger.
var Log Logger

// Info prints a dim/muted message (context loading, session paths, auth info).
func (Logger) Info(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s"+format+"%s\n", append([]any{dim}, append(args, reset)...)...)
}

// Infof prints a dim message without a trailing newline.
func (Logger) Infof(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s"+format+"%s", append([]any{dim}, append(args, reset)...)...)
}

// Warn prints a yellow warning message.
func (Logger) Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%sWARNING: "+format+"%s\n", append([]any{yellow}, append(args, reset)...)...)
}

// Error prints a red error message.
func (Logger) Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%serror: "+format+"%s\n", append([]any{red}, append(args, reset)...)...)
}

// Errorf prints a red error message without the "error: " prefix.
func (Logger) Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s"+format+"%s\n", append([]any{red}, append(args, reset)...)...)
}

// Plain prints a message to stderr with no formatting.
func (Logger) Plain(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
