package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var (
	runModel    string
	runThinking int64
	runAgent    string
	runRaw      bool
	runNoTools  bool
	runSystem   string
	runNew    bool
	runDetach bool
)

var runCmd = &cobra.Command{
	Use:   "run [message]",
	Short: "Send a single message and print the response",
	Long: `Send a message and print the response. Equivalent to a one-message chat.

Use -c/--continue to resume the most recent session instead of starting fresh.

Examples:
  inber run "explain this error"
  echo "summarize this" | inber run
  inber run -n "start fresh task"         # new session (becomes default)
  inber run -d "one-off question"        # detached, doesn't affect main session
  inber run -a myagent "refactor this"
  inber run --raw --system "You are a translator" "translate to French: hello"
  inber run --no-tools "what time is it?"`,
	Run: runRun,
}

func init() {
	runCmd.Flags().StringVarP(&runModel, "model", "m", agent.DefaultModel, "Claude model to use")
	runCmd.Flags().Int64VarP(&runThinking, "thinking", "t", 0, "Enable extended thinking with token budget (0=disabled)")
	runCmd.Flags().StringVarP(&runAgent, "agent", "a", "", "Agent name to load from registry")
	runCmd.Flags().BoolVar(&runRaw, "raw", false, "Skip context and memory loading")
	runCmd.Flags().BoolVar(&runNoTools, "no-tools", false, "Disable all tools")
	runCmd.Flags().StringVar(&runSystem, "system", "", "Override system prompt")
	runCmd.Flags().BoolVarP(&runNew, "new", "n", false, "Start a new session instead of continuing the default")
	runCmd.Flags().BoolVarP(&runDetach, "detach", "d", false, "Run in a one-off session without affecting the main session")
}

func runRun(cmd *cobra.Command, args []string) {
	// Get message from args or stdin
	var input string
	if len(args) > 0 {
		input = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			Log.Error("reading stdin: %v", err)
			os.Exit(1)
		}
		input = strings.TrimSpace(string(data))
	}

	if input == "" {
		Log.Error("no message provided")
		Log.Plain("usage: inber run \"your message\" or echo \"message\" | inber run")
		os.Exit(1)
	}

	eng, err := NewEngine(EngineConfig{
		Model:          runModel,
		Thinking:       runThinking,
		AgentName:      runAgent,
		Raw:            runRaw,
		NoTools:        runNoTools,
		SystemOverride: runSystem,
		CommandName:    "run",
		NewSession:     runNew,
		Detach:         runDetach,
		Display: &DisplayHooks{
			OnToolCall:   DisplayToolCall,
			OnToolResult: DisplayToolResult,
		},
	})
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}
	defer eng.Close()

	result, err := eng.RunTurn(input)
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	// Print response to stdout — clean, no ANSI
	fmt.Print(result.Text)

	// Stats to stderr - more prominent token logging
	cost := session.CalcCost(eng.Model, result.InputTokens, result.OutputTokens)
	total := result.InputTokens + result.OutputTokens
	fmt.Fprintf(os.Stderr, "\n┌─ Tokens ──────────────────────\n")
	fmt.Fprintf(os.Stderr, "│ in=%d  out=%d  total=%d  tools=%d\n", 
		result.InputTokens, result.OutputTokens, total, result.ToolCalls)
	fmt.Fprintf(os.Stderr, "│ cost=$%.4f\n", cost)
	fmt.Fprintf(os.Stderr, "└───────────────────────────────\n")
}
