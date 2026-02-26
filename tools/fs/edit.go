package fs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/internal"
)

// EditFile returns a tool that does exact string replacement in a file.
func EditFile() agent.Tool {
	type input struct {
		Path    string `json:"path"`
		OldText string `json:"old_text"`
		NewText string `json:"new_text"`
	}
	return agent.Tool{
		Name:        "edit_file",
		Description: "Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.",
		InputSchema: internal.Props([]string{"path", "old_text", "new_text"}, map[string]any{
			"path":     internal.Str("Path to the file to edit"),
			"old_text": internal.Str("Exact text to find and replace"),
			"new_text": internal.Str("New text to replace the old text with"),
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
