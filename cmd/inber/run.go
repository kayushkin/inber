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
)

var runCmd = &cobra.Command{
	Use:   "run [message]",
	Short: "Send a single message and print the response to stdout",
	Long: `Run a single turn against Claude and print the response to stdout.
Reads from stdin if no message argument is provided.

Examples:
  inber run "explain this error"
  echo "summarize this" | inber run
  inber run -a myagent "refactor this function"
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
}

func runRun(cmd *cobra.Command, args []string) {
	// Get message from args or stdin
	var input string
	if len(args) > 0 {
		input = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		input = strings.TrimSpace(string(data))
	}

	if input == "" {
		fmt.Fprintln(os.Stderr, "error: no message provided")
		fmt.Fprintln(os.Stderr, "usage: inber run \"your message\" or echo \"message\" | inber run")
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
		Display: &DisplayHooks{
			OnToolCall: func(name string, input string) {
				fmt.Fprintf(os.Stderr, "⚡ %s\n", name)
			},
			OnToolResult: func(name string, output string, isError bool) {
				if isError {
					fmt.Fprintf(os.Stderr, "  ✗ %s\n", truncate(output, 200))
				}
			},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer eng.Close()

	result, err := eng.RunTurn(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Print response to stdout — clean, no ANSI
	fmt.Print(result.Text)

	// Stats to stderr
	cost := session.CalcCost(eng.Model, result.InputTokens, result.OutputTokens)
	fmt.Fprintf(os.Stderr, "\n[in=%d | out=%d | tools=%d | $%.4f]\n",
		result.InputTokens, result.OutputTokens, result.ToolCalls, cost)
}
