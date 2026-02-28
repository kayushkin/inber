package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/internal"
)

// ListFiles returns a tool that lists directory contents.
func ListFiles() agent.Tool {
	type input struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	return agent.Tool{
		Name:        "list_files",
		Description: "List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).",
		InputSchema: internal.Props([]string{"path"}, map[string]any{
			"path":      internal.Str("Directory path to list"),
			"recursive": map[string]any{"type": "boolean", "description": "List recursively (default: false)"},
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := internal.Parse[input](raw)
			if err != nil {
				return "", err
			}
			if in.Path == "" {
				in.Path = "."
			}

			if !in.Recursive {
				entries, err := os.ReadDir(in.Path)
				if err != nil {
					return fmt.Sprintf("error: %s", err), nil
				}
				var lines []string
				for _, e := range entries {
					name := e.Name()
					if e.IsDir() {
						name += "/"
					}
					lines = append(lines, name)
				}
				// Apply smart truncation at 50 entries
				return internal.TruncateList(lines, 50), nil
			}

			var lines []string
			const maxEntries = 1000
			filepath.WalkDir(in.Path, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if len(lines) >= maxEntries {
					return filepath.SkipAll
				}
				name := d.Name()
				if d.IsDir() && strings.HasPrefix(name, ".") && path != in.Path {
					return filepath.SkipDir
				}
				rel, _ := filepath.Rel(in.Path, path)
				if rel == "." {
					return nil
				}
				if d.IsDir() {
					rel += "/"
				}
				lines = append(lines, rel)
				return nil
			})
			if len(lines) >= maxEntries {
				lines = append(lines, fmt.Sprintf("... (truncated at %d entries)", maxEntries))
			}
			// Apply smart truncation at 50 entries
			return internal.TruncateList(lines, 50), nil
		},
	}
}
