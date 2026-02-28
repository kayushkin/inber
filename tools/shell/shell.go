package shell

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/internal"
)

// Shell returns a tool that executes shell commands via bash.
func Shell() agent.Tool {
	type input struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	return agent.Tool{
		Name:        "shell",
		Description: "Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.",
		InputSchema: internal.Props([]string{"command"}, map[string]any{
			"command": internal.Str("Shell command to execute"),
			"workdir": internal.Str("Working directory (optional, defaults to cwd)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := internal.Parse[input](raw)
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
				result = fmt.Sprintf("%s\nexit: %s", result, err)
			}
			if result == "" {
				result = "(no output)"
			}
			
			// Apply smart truncation to keep context manageable
			result = internal.TruncateShellOutput(result)
			return result, nil
		},
	}
}
