package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kayushkin/inber/internal/textutil"
)

// DisplayToolCall prints a tool call to the terminal with inline payload.
func DisplayToolCall(name string, input string) {
	fmt.Fprintf(os.Stderr, "\n%s⚡ %s%s", magenta+bold, name, reset)
	
	// Show payload inline if small enough
	payload := formatToolPayload(name, input)
	if payload != "" {
		fmt.Fprintf(os.Stderr, " %s%s%s", dim, payload, reset)
	}
	
	fmt.Fprintln(os.Stderr)
}

// DisplayToolResult prints a tool result to the terminal with inline summary.
func DisplayToolResult(name string, output string, isError bool) {
	if isError {
		// Show error inline, truncated
		errMsg := strings.ReplaceAll(output, "\n", " ")
		errMsg = textutil.TruncateWith(errMsg, 100, "…")
		fmt.Fprintf(os.Stderr, "%s  ✗ %s%s\n", red, errMsg, reset)
		return
	}
	
	// Show result summary
	summary := formatToolResult(name, output)
	fmt.Fprintf(os.Stderr, "%s  → %s%s\n", dim, summary, reset)
}

// formatToolPayload formats the tool input payload for inline display.
func formatToolPayload(name, rawInput string) string {
	// Parse the JSON input
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
		// If parsing fails, just show truncated raw
		return textutil.TruncateWith(rawInput, 120, "…")
	}

	// Tool-specific formatting
	switch name {
	case "shell", "shell_commands", "bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 100 {
				return fmt.Sprintf("$ %s…", textutil.Truncate(cmd, 100))
			}
			return fmt.Sprintf("$ %s", cmd)
		}

	case "read_file", "read_files":
		if path, ok := input["path"].(string); ok {
			return path
		}
		if path, ok := input["file_path"].(string); ok {
			return path
		}

	case "write_file", "write_files":
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

	case "edit_file", "edit_files":
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
			content = textutil.TruncateWith(content, 60, "…")
			return fmt.Sprintf("{\"content\":\"%s\"}", content)
		}

	case "memory_search":
		if query, ok := input["query"].(string); ok {
			if len(query) > 60 {
				return fmt.Sprintf("{\"query\":\"%s…\"}", textutil.Truncate(query, 60))
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
	return textutil.TruncateWith(s, 120, "…")
}

// formatToolResult formats the tool result for inline display.
func formatToolResult(name, output string) string {
	bytes := len(output)
	lines := strings.Count(output, "\n") + 1

	// Tool-specific formatting
	switch name {
	case "shell", "shell_commands", "bash":
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

	case "read_file", "read_files":
		if lines > 1 {
			return fmt.Sprintf("%d lines", lines)
		}
		return fmt.Sprintf("%d bytes", bytes)

	case "write_file", "write_files":
		if lines > 1 {
			return fmt.Sprintf("wrote %d lines", lines)
		}
		return fmt.Sprintf("wrote %d bytes", bytes)

	case "edit_file", "edit_files":
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
			// The id comes from the tool's own JSON result, so its length is
			// whatever memory_save returned. A raw result.ID[:8] panics on
			// anything shorter than eight bytes, in the display path of an
			// interactive run.
			return fmt.Sprintf("saved %s", textutil.Truncate(result.ID, 8))
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