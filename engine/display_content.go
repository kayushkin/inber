package engine

import (
	"fmt"
	"os"
	"strings"
)

// DisplayThinking prints thinking/reasoning text.
func DisplayThinking(text string) {
	if text == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s%s💭 thinking...%s\n", dim, italic, reset)
	// Show truncated thinking — full text goes to logs
	lines := strings.Split(text, "\n")
	max := 8
	if len(lines) <= max {
		for _, l := range lines {
			fmt.Fprintf(os.Stderr, "%s%s  %s%s\n", dim, italic, l, reset)
		}
	} else {
		for _, l := range lines[:3] {
			fmt.Fprintf(os.Stderr, "%s%s  %s%s\n", dim, italic, l, reset)
		}
		fmt.Fprintf(os.Stderr, "%s%s  ... (%d more lines)%s\n", dim, italic, len(lines)-6, reset)
		for _, l := range lines[len(lines)-3:] {
			fmt.Fprintf(os.Stderr, "%s%s  %s%s\n", dim, italic, l, reset)
		}
	}
}

// DisplayResponse prints the final assistant response.
func DisplayResponse(text string) {
	fmt.Fprintf(os.Stderr, "\n%s\n", text)
}