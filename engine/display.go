// Display utilities for the inber engine.
//
// Organized into focused modules:
//   - display_colors.go: ANSI color constants
//   - display_logger.go: Logger struct and logging methods
//   - display_tools.go: Tool call and result formatting
//   - display_content.go: Content display (thinking, responses)
//   - display_stats.go: Token usage and cost statistics
package engine

// DisplayHooks configures how engine events are shown to the user.
type DisplayHooks struct {
	OnThinking   func(text string)
	OnTextDelta  func(text string)                          // streaming text chunks from API
	OnToolCall   func(name string, input string)
	OnToolResult func(name string, output string, isError bool)
	OnStatus     func(text string)                          // progress/status updates
}

// SetDisplayHooks updates the display hooks (thread-safe).
// Used by the server to point hooks at the current HTTP request's writer.
func (e *Engine) SetDisplayHooks(hooks *DisplayHooks) {
	e.displayMu.Lock()
	e.display = hooks
	e.displayMu.Unlock()
}

// GetDisplayHooks returns the current display hooks (thread-safe).
func (e *Engine) GetDisplayHooks() *DisplayHooks {
	e.displayMu.Lock()
	defer e.displayMu.Unlock()
	return e.display
}

// emitStatus emits a status update if the OnStatus hook is configured.
func (e *Engine) emitStatus(text string) {
	d := e.GetDisplayHooks()
	if d != nil && d.OnStatus != nil {
		d.OnStatus(text)
	}
}

