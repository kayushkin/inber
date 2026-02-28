package fs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/internal"
)

// ReadFile returns a tool that reads file contents.
func ReadFile() agent.Tool {
	type input struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	return agent.Tool{
		Name:        "read_file",
		Description: "Read the contents of a file. For large files, use offset (1-indexed line number) and limit (max lines) to read a portion.",
		InputSchema: internal.Props([]string{"path"}, map[string]any{
			"path":   internal.Str("Path to the file to read"),
			"offset": internal.Integer("Line number to start from (1-indexed, optional)"),
			"limit":  internal.Integer("Maximum number of lines to return (optional)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := internal.Parse[input](raw)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(in.Path)
			if err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}
			content := string(data)
			truncatedByUser := false

			if in.Offset > 0 || in.Limit > 0 {
				lines := strings.Split(content, "\n")
				start := 0
				if in.Offset > 0 {
					start = in.Offset - 1
				}
				if start > len(lines) {
					return fmt.Sprintf("offset %d beyond file length (%d lines)", in.Offset, len(lines)), nil
				}
				end := len(lines)
				if in.Limit > 0 && start+in.Limit < end {
					end = start + in.Limit
				}
				content = strings.Join(lines[start:end], "\n")
				truncatedByUser = true
			}

			const maxBytes = 100_000
			if len(content) > maxBytes {
				content = content[:maxBytes] + "\n... (truncated)"
			}
			
			// Apply smart line truncation if user didn't specify offset/limit
			content = internal.TruncateFileRead(content, truncatedByUser)
			return content, nil
		},
	}
}
