// Package tools provides built-in tools for the inber agent.
// Each tool is a function returning an agent.Tool, so callers pick what to enable.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// helper to unmarshal tool input JSON
func parse[T any](input string) (T, error) {
	var v T
	err := json.Unmarshal([]byte(input), &v)
	return v, err
}

// props builds an InputSchema from a map of property definitions.
func props(required []string, properties map[string]any) anthropic.ToolInputSchemaParam {
	s := anthropic.ToolInputSchemaParam{
		Properties: properties,
	}
	if len(required) > 0 {
		s.Required = required
	}
	return s
}

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// Shell returns a tool that executes shell commands via bash.
func Shell() agent.Tool {
	type input struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	return agent.Tool{
		Name:        "shell",
		Description: "Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.",
		InputSchema: props([]string{"command"}, map[string]any{
			"command": str("Shell command to execute"),
			"workdir": str("Working directory (optional, defaults to cwd)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := parse[input](raw)
			if err != nil {
				return "", err
			}
			cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
			if in.Workdir != "" {
				cmd.Dir = in.Workdir
			}
			out, err := cmd.CombinedOutput()
			result := string(out)
			if err != nil {
				return fmt.Sprintf("%s\nexit: %s", result, err), nil // don't error — let the model see the failure
			}
			if result == "" {
				result = "(no output)"
			}
			return result, nil
		},
	}
}

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
		InputSchema: props([]string{"path"}, map[string]any{
			"path":   str("Path to the file to read"),
			"offset": integer("Line number to start from (1-indexed, optional)"),
			"limit":  integer("Maximum number of lines to return (optional)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := parse[input](raw)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(in.Path)
			if err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}
			content := string(data)

			// Apply offset/limit if specified
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
			}

			// Truncate very large outputs
			const maxBytes = 100_000
			if len(content) > maxBytes {
				content = content[:maxBytes] + "\n... (truncated)"
			}
			return content, nil
		},
	}
}

// WriteFile returns a tool that creates or overwrites a file.
func WriteFile() agent.Tool {
	type input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	return agent.Tool{
		Name:        "write_file",
		Description: "Create or overwrite a file with the given content. Creates parent directories automatically.",
		InputSchema: props([]string{"path", "content"}, map[string]any{
			"path":    str("Path to the file to write"),
			"content": str("Content to write to the file"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := parse[input](raw)
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

// EditFile returns a tool that does exact string replacement in a file.
func EditFile() agent.Tool {
	type input struct {
		Path      string `json:"path"`
		OldText   string `json:"old_text"`
		NewText   string `json:"new_text"`
	}
	return agent.Tool{
		Name:        "edit_file",
		Description: "Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.",
		InputSchema: props([]string{"path", "old_text", "new_text"}, map[string]any{
			"path":     str("Path to the file to edit"),
			"old_text": str("Exact text to find and replace"),
			"new_text": str("New text to replace the old text with"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := parse[input](raw)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(in.Path)
			if err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}
			content := string(data)

			count := strings.Count(content, in.OldText)
			if count == 0 {
				return "error: old_text not found in file", nil
			}
			if count > 1 {
				return fmt.Sprintf("error: old_text matches %d times — must be unique", count), nil
			}

			newContent := strings.Replace(content, in.OldText, in.NewText, 1)
			if err := os.WriteFile(in.Path, []byte(newContent), 0644); err != nil {
				return fmt.Sprintf("error writing file: %s", err), nil
			}
			return fmt.Sprintf("edited %s", in.Path), nil
		},
	}
}

// ListFiles returns a tool that lists directory contents.
func ListFiles() agent.Tool {
	type input struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	return agent.Tool{
		Name:        "list_files",
		Description: "List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).",
		InputSchema: props([]string{"path"}, map[string]any{
			"path":      str("Directory path to list"),
			"recursive": map[string]any{"type": "boolean", "description": "List recursively (default: false)"},
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := parse[input](raw)
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
				return strings.Join(lines, "\n"), nil
			}

			// Recursive: walk the tree, skip hidden dirs
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
				// Skip hidden directories
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
			return strings.Join(lines, "\n"), nil
		},
	}
}

// All returns every built-in tool.
func All() []agent.Tool {
	return []agent.Tool{
		Shell(),
		ReadFile(),
		WriteFile(),
		EditFile(),
		ListFiles(),
	}
}
