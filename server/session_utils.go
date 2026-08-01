package server

import (
	"strings"
	"time"

	"github.com/kayushkin/inber/internal/textutil"
	"github.com/kayushkin/inber/internal/timeutil"
)

// formatDuration is a convenience wrapper around timeutil.FormatDuration.
func formatDuration(d time.Duration) string {
	return timeutil.FormatDuration(d)
}

// truncate truncates a string for display.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	// n is the whole budget here, ellipsis included.
	return textutil.Truncate(s, n-3) + "..."
}