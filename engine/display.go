package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/session"
)

// ANSI color helpers
const (
	Reset   = "\033[0m"
	Dim     = "\033[2m"
	Bold    = "\033[1m"
	Italic  = "\033[3m"
	Cyan    = "\033[36m"
	Magenta = "\033[35m"
	Yellow  = "\033[33m"
	Green   = "\033[32m"
	Red     = "\033[31m"
	Blue    = "\033[34m"
)

// Keep lowercase aliases for internal use
const (
	reset   = Reset
	dim     = Dim
	bold    = Bold
	italic  = Italic
	cyan    = Cyan
	magenta = Magenta
	yellow  = Yellow
	green   = Green
	red     = Red
	blue    = Blue
)

// DisplayToolCall prints a tool call to the terminal with inline payload.
func DisplayToolCall(name string, input string) {
	fmt.Printf("\n%s⚡ %s%s", magenta+bold, name, reset)
	
	// Show payload inline if small enough
	payload := formatToolPayload(name, input)
	if payload != "" {
		fmt.Printf(" %s%s%s", dim, payload, reset)
	}
	
	fmt.Println()
}

// DisplayToolResult prints a tool result to the terminal with inline summary.
func DisplayToolResult(name string, output string, isError bool) {
	if isError {
		// Show error inline, truncated
		errMsg := strings.ReplaceAll(output, "\n", " ")
		if len(errMsg) > 100 {
			errMsg = errMsg[:100] + "…"
		}
		fmt.Printf("%s  ✗ %s%s\n", red, errMsg, reset)
		return
	}
	
	// Show result summary
	summary := formatToolResult(name, output)
	fmt.Printf("%s  → %s%s\n", dim, summary, reset)
}

// formatToolPayload formats the tool input payload for inline display.
func formatToolPayload(name, rawInput string) string {
	// Parse the JSON input
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
		// If parsing fails, just show truncated raw
		if len(rawInput) <= 120 {
			return rawInput
		}
		return rawInput[:120] + "…"
	}

	// Tool-specific formatting
	switch name {
	case "shell", "bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 100 {
				return fmt.Sprintf("$ %s…", cmd[:100])
			}
			return fmt.Sprintf("$ %s", cmd)
		}

	case "read_file":
		if path, ok := input["path"].(string); ok {
			return path
		}
		if path, ok := input["file_path"].(string); ok {
			return path
		}

	case "write_file":
		var path string
		if p, ok := input["path"].(string); ok {
			path = p
		} else if p, ok := input["file_path"].(string); ok {
			path = p
		}
		
		var size string
		if content, ok := input["content"].(string); ok {
			lines := strings.Count(content, "\n") + 1
			size = fmt.Sprintf("%d lines", lines)
		}
		
		if path != "" && size != "" {
			return fmt.Sprintf("%s (%s)", path, size)
		} else if path != "" {
			return path
		}

	case "edit_file":
		if path, ok := input["path"].(string); ok {
			return path
		}
		if path, ok := input["file_path"].(string); ok {
			return path
		}

	case "list_files":
		if path, ok := input["path"].(string); ok {
			return path
		}
		if path, ok := input["directory"].(string); ok {
			return path
		}

	case "memory_save":
		if content, ok := input["content"].(string); ok {
			if len(content) > 60 {
				content = content[:60] + "…"
			}
			return fmt.Sprintf("{\"content\":\"%s\"}", content)
		}

	case "memory_search":
		if query, ok := input["query"].(string); ok {
			if len(query) > 60 {
				return fmt.Sprintf("{\"query\":\"%s…\"}", query[:60])
			}
			return fmt.Sprintf("{\"query\":\"%s\"}", query)
		}

	case "memory_forget":
		if id, ok := input["id"].(string); ok {
			return fmt.Sprintf("{\"id\":\"%s\"}", id)
		}

	case "memory_expand":
		if id, ok := input["id"].(string); ok {
			return fmt.Sprintf("{\"id\":\"%s\"}", id)
		}
	}

	// Generic JSON formatting
	// Serialize compact and truncate if needed
	compact, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	
	s := string(compact)
	if len(s) <= 120 {
		return s
	}
	return s[:120] + "…"
}

// formatToolResult formats the tool result for inline display.
func formatToolResult(name, output string) string {
	bytes := len(output)
	lines := strings.Count(output, "\n") + 1

	// Tool-specific formatting
	switch name {
	case "shell", "bash":
		// Show first line if it's short, otherwise just "OK" or line count
		if bytes == 0 {
			return "OK (no output)"
		}
		firstLine := strings.Split(output, "\n")[0]
		if len(firstLine) <= 80 && lines == 1 {
			return firstLine
		}
		if bytes < 100 && lines <= 3 {
			return strings.ReplaceAll(output, "\n", " ")
		}
		return fmt.Sprintf("%d lines", lines)

	case "read_file":
		if lines > 1 {
			return fmt.Sprintf("%d lines", lines)
		}
		return fmt.Sprintf("%d bytes", bytes)

	case "write_file":
		if lines > 1 {
			return fmt.Sprintf("wrote %d lines", lines)
		}
		return fmt.Sprintf("wrote %d bytes", bytes)

	case "edit_file":
		// Try to extract edit summary from output
		if strings.Contains(output, "replaced") || strings.Contains(output, "edited") {
			firstLine := strings.Split(output, "\n")[0]
			if len(firstLine) <= 100 {
				return firstLine
			}
		}
		return "edited"

	case "list_files":
		// Count files listed
		fileCount := 0
		for _, line := range strings.Split(output, "\n") {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "total") {
				fileCount++
			}
		}
		if fileCount > 0 {
			return fmt.Sprintf("%d items", fileCount)
		}
		return "OK"

	case "memory_save":
		// Extract memory ID if present
		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(output), &result); err == nil && result.ID != "" {
			return fmt.Sprintf("saved %s", result.ID[:8])
		}
		return fmt.Sprintf("saved (%d bytes)", bytes)

	case "memory_search":
		// Count results
		var result struct {
			Memories []interface{} `json:"memories"`
		}
		if err := json.Unmarshal([]byte(output), &result); err == nil {
			return fmt.Sprintf("%d results", len(result.Memories))
		}
		return "OK"

	case "memory_forget":
		return "deleted"

	case "memory_expand":
		// Show how many related memories were expanded
		var result struct {
			Related []interface{} `json:"related"`
		}
		if err := json.Unmarshal([]byte(output), &result); err == nil && len(result.Related) > 0 {
			return fmt.Sprintf("expanded (%d related)", len(result.Related))
		}
		return "expanded"
	}

	// Generic: show byte/line count
	if bytes == 0 {
		return "OK"
	}
	if lines > 1 {
		return fmt.Sprintf("%d lines", lines)
	}
	// Single line: show it if short enough
	if bytes <= 80 {
		return output
	}
	return fmt.Sprintf("%d bytes", bytes)
}

// DisplayThinking prints thinking/reasoning text.
func DisplayThinking(text string) {
	if text == "" {
		return
	}
	fmt.Printf("\n%s%s💭 thinking...%s\n", dim, italic, reset)
	// Show truncated thinking — full text goes to logs
	lines := strings.Split(text, "\n")
	max := 8
	if len(lines) <= max {
		for _, l := range lines {
			fmt.Printf("%s%s  %s%s\n", dim, italic, l, reset)
		}
	} else {
		for _, l := range lines[:3] {
			fmt.Printf("%s%s  %s%s\n", dim, italic, l, reset)
		}
		fmt.Printf("%s%s  ... (%d more lines)%s\n", dim, italic, len(lines)-6, reset)
		for _, l := range lines[len(lines)-3:] {
			fmt.Printf("%s%s  %s%s\n", dim, italic, l, reset)
		}
	}
}

// DisplayResponse prints the final assistant response.
func DisplayResponse(text string) {
	fmt.Printf("\n%s\n", text)
}

// DisplayStats prints token usage and cost using the shared CalcCost from session package.
// Now also displays cumulative session stats if engine is provided.
func DisplayStats(result *agent.TurnResult, model string) {
	cost := session.CalcCost(model, result.InputTokens, result.OutputTokens)
	total := result.InputTokens + result.OutputTokens
	
	// Show prominent token summary
	fmt.Printf("\n%s┌─ Turn Tokens ─────────────────%s\n", dim, reset)
	fmt.Printf("%s│%s in=%s%d%s  out=%s%d%s  total=%s%d%s", 
		dim, reset,
		cyan, result.InputTokens, reset,
		cyan, result.OutputTokens, reset,
		bold+cyan, total, reset)
	
	if result.ToolCalls > 0 {
		fmt.Printf("  tools=%s%d%s", yellow, result.ToolCalls, reset)
	}
	
	if cost > 0 {
		fmt.Printf("  cost=%s$%.4f%s", green, cost, reset)
	}
	
	fmt.Printf("\n%s└───────────────────────────────%s\n", dim, reset)
}

// DisplaySessionStats prints cumulative session token usage and cost.
func DisplaySessionStats(turnNum, inputTokens, outputTokens int, cost float64) {
	total := inputTokens + outputTokens
	
	fmt.Printf("%s│ Session (turn %d): in=%d out=%d total=%s%d%s cost=%s$%.4f%s%s\n", 
		dim,
		turnNum,
		inputTokens,
		outputTokens,
		cyan, total, dim,
		green, cost, dim,
		reset)
}
