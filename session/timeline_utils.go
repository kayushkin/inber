package session

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kayushkin/inber/internal/textutil"
)

func truncateStr(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return textutil.TruncateWith(s, max, "...")
}

func summarizeToolInput(name, raw string) string {
	// Extract a concise representation
	raw = strings.TrimSpace(raw)
	
	// For shell/exec, try to extract the command
	if name == "shell" || name == "shell_commands" || name == "bash" {
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(raw), &input); err == nil && input.Command != "" {
			if len(input.Command) > 100 {
				return fmt.Sprintf("`%s...`", textutil.Truncate(input.Command, 100))
			}
			return fmt.Sprintf("`%s`", input.Command)
		}
	}
	
	// For file tools, try to extract the path
	if strings.Contains(name, "file") || strings.Contains(name, "write") || strings.Contains(name, "read") || strings.Contains(name, "edit") {
		var input struct {
			Path     string `json:"path"`
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			path := input.Path
			if path == "" {
				path = input.FilePath
			}
			if path != "" {
				return fmt.Sprintf("`%s`", path)
			}
		}
	}
	
	// Generic: truncate
	return textutil.TruncateWith(raw, 80, "...")
}

func summarizeToolOutput(output string) string {
	bytes := len(output)
	if bytes == 0 {
		return "0 bytes"
	}
	lines := strings.Count(output, "\n") + 1
	if lines > 1 {
		return fmt.Sprintf("%d lines, %s bytes", lines, formatNumber(bytes))
	}
	return fmt.Sprintf("%s bytes", formatNumber(bytes))
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}