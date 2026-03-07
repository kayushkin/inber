package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
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
	runNew      bool
	runDetach   bool
	
	// Auto-workflow flags (Phase 1)
	runAutoBranch bool
	runAutoCommit bool
	runAutoFormat bool

	// Safety limits
	runMaxTurns       int // max API round-trips per run
	runMaxInputTokens int // max cumulative input tokens per run
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
	
	// Auto-workflow flags (defaults to true for all Phase 1 features)
	runCmd.Flags().BoolVar(&runAutoBranch, "auto-branch", true, "Auto-create session branch")
	runCmd.Flags().BoolVar(&runAutoCommit, "auto-commit", true, "Auto-commit after successful writes")
	runCmd.Flags().BoolVar(&runAutoFormat, "auto-format", true, "Auto-format code after writes")
	runCmd.Flags().BoolVarP(&runDetach, "detach", "d", false, "Run in a one-off session without affecting the main session")

	// Safety limits
	runCmd.Flags().IntVar(&runMaxTurns, "max-turns", 0, "Max API round-trips per run (0=unlimited, default 25 for --detach)")
	runCmd.Flags().IntVar(&runMaxInputTokens, "max-input-tokens", 0, "Max cumulative input tokens per run (0=unlimited, default 500k for --detach)")
}

// stdinMessage is the JSON line format for bus-agent → inber communication.
type stdinMessage struct {
	Text   string `json:"text"`
	Author string `json:"author,omitempty"`
}

func runRun(cmd *cobra.Command, args []string) {
	var input string
	var injections chan string

	if len(args) > 0 {
		// Message from CLI args — no injection support
		input = strings.Join(args, " ")
	} else {
		// Read from stdin
		reader := bufio.NewReader(os.Stdin)
		firstLine, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			engine.Log.Error("reading stdin: %v", err)
			os.Exit(1)
		}
		firstLine = strings.TrimRight(firstLine, "\n\r")

		// Try JSON line protocol (bus-agent sends JSON lines)
		var msg stdinMessage
		if json.Unmarshal([]byte(firstLine), &msg) == nil && msg.Text != "" {
			input = msg.Text
			if msg.Author != "" {
				input = fmt.Sprintf("[%s] %s", msg.Author, input)
			}

			// Keep reading stdin for follow-up injections
			injections = make(chan string, 10)
			go func() {
				defer close(injections)
				scanner := bufio.NewScanner(reader)
				for scanner.Scan() {
					line := scanner.Text()
					if line == "" {
						continue
					}
					var followUp stdinMessage
					if json.Unmarshal([]byte(line), &followUp) == nil && followUp.Text != "" {
						text := followUp.Text
						if followUp.Author != "" {
							text = fmt.Sprintf("[%s] %s", followUp.Author, text)
						}
						injections <- text
					}
				}
			}()
		} else {
			// Plain text fallback — read rest of stdin, no injections
			rest, _ := io.ReadAll(reader)
			input = strings.TrimSpace(firstLine + "\n" + string(rest))
		}
	}

	if input == "" {
		engine.Log.Error("no message provided")
		engine.Log.Plain("usage: inber run \"your message\" or echo \"message\" | inber run")
		os.Exit(1)
	}

	cfg := engine.EngineConfig{
		Model:              runModel,
		ModelExplicitlySet: cmd.Flags().Changed("model"),
		Thinking:           runThinking,
		AgentName:          runAgent,
		Raw:                runRaw,
		NoTools:            runNoTools,
		SystemOverride:     runSystem,
		CommandName:        "run",
		NewSession:         runNew,
		Detach:             runDetach,
		Display: &engine.DisplayHooks{
			OnToolCall:   engine.DisplayToolCall,
			OnToolResult: engine.DisplayToolResult,
		},
		AutoWorkflow: engine.AutoWorkflowConfig{
			AutoBranch: runAutoBranch,
			AutoCommit: runAutoCommit,
			AutoFormat: runAutoFormat,
		},
		MaxTurns:       runMaxTurns,
		MaxInputTokens: runMaxInputTokens,
		Injections:     injections,
	}

	eng, err := engine.NewEngine(cfg)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}
	defer eng.Close()

	startTime := time.Now()
	result, err := eng.RunTurn(input)
	durationMs := time.Since(startTime).Milliseconds()
	if err != nil {
		engine.Log.Error("%v", err)
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
	// Show cache savings if any
	if result.CacheReadTokens > 0 || result.CacheCreationTokens > 0 {
		fmt.Fprintf(os.Stderr, "│ cache: %d read, %d created\n",
			result.CacheReadTokens, result.CacheCreationTokens)
	}
	fmt.Fprintf(os.Stderr, "│ cost=$%.4f\n", cost)
	fmt.Fprintf(os.Stderr, "└───────────────────────────────\n")

	// Machine-readable metadata for programmatic consumers (bus-agent, etc.)
	// Prefixed with INBER_META: for easy parsing
	meta := map[string]interface{}{
		"input_tokens":          result.InputTokens,
		"output_tokens":         result.OutputTokens,
		"cache_read_tokens":     result.CacheReadTokens,
		"cache_creation_tokens": result.CacheCreationTokens,
		"tool_calls":            result.ToolCalls,
		"cost":                  cost,
		"duration_ms":           durationMs,
		"model":                 eng.Model,
		"turn":                  eng.TurnCounter,
	}
	if metaJSON, err := json.Marshal(meta); err == nil {
		fmt.Fprintf(os.Stderr, "INBER_META:%s\n", metaJSON)
	}
}
