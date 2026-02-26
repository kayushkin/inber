package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/internal"
)

// WriteFile returns a tool that creates or overwrites a file.
func WriteFile() agent.Tool {
	type input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	return agent.Tool{
		Name:        "write_file",
		Description: "Create or overwrite a file with the given content. Creates parent directories automatically.",
		InputSchema: internal.Props([]string{"path", "content"}, map[string]any{
			"path":    internal.Str("Path to the file to write"),
			"content": internal.Str("Content to write to the file"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := internal.Parse[input](raw)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(in.Path), 0755); err != nil {
				return fmt.Sprintf("error creating directory: %s", err), nil
			}
			if err := os.WriteFile(in.Path, []byte(in.Content), 0644); err != nil {
				return fmt.Sprintf("error writing file: %s", err), nil
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
		},
	}
}
