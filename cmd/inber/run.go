package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"encoding/json"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
	"github.com/spf13/cobra"
)

var (
	runModel    string
	runThinking int64
	runAgent    string
	runRaw      bool // skip context/memory loading
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
	// Get the message from args or stdin
	var input string
	if len(args) > 0 {
		input = strings.Join(args, " ")
	} else {
		// Read from stdin
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

	// Find repo root
	repoRoot, err := findRepoRoot()
	if err != nil {
		repoRoot, _ = os.Getwd()
	}

	// Load agent config if specified
	var agentConfig *registry.AgentConfig
	var identityText string

	if runAgent != "" {
		configs, err := registry.LoadConfig(
			filepath.Join(repoRoot, "agents.json"),
			filepath.Join(repoRoot, "agents"),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading agents: %v\n", err)
			os.Exit(1)
		}

		var ok bool
		agentConfig, ok = configs[runAgent]
		if !ok {
			fmt.Fprintf(os.Stderr, "agent not found: %s\n", runAgent)
			os.Exit(1)
		}

		if agentConfig.Model != "" {
			runModel = agentConfig.Model
		}
		identityText = agentConfig.System
	} else {
		identityText = "You are a helpful assistant."
	}

	// Build system prompt
	var systemPrompt string
	if runSystem != "" {
		// Explicit system prompt overrides everything
		systemPrompt = runSystem
	} else if runRaw {
		systemPrompt = identityText
	} else {
		// Full context loading
		contextCfg := inbercontext.DefaultAutoLoadConfig(repoRoot)
		contextCfg.IdentityText = identityText

		contextStore, err := inbercontext.AutoLoad(contextCfg)
		if err != nil {
			contextStore = inbercontext.NewStore()
		}

		if err := inbercontext.LoadProjectContext(contextStore, repoRoot); err != nil {
			// Non-fatal
			fmt.Fprintf(os.Stderr, "warning: failed to load project context: %v\n", err)
		}

		// Load memory
		memStore, err := memory.OpenOrCreate(repoRoot)
		if err == nil {
			defer memStore.Close()
			memory.LoadIntoContext(memStore, contextStore, 10, 0.6)
		}

		systemPrompt = buildSystemPrompt(contextStore, input)
	}

	// Session logging
	agentName := runAgent
	if agentName == "" {
		agentName = "run"
	}

	sess, err := sessionMod.New("logs", runModel, agentName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging disabled: %v\n", err)
	} else {
		defer sess.Close()
		// Register active session
		if _, err := sessionMod.RegisterActive(repoRoot, sess, "run"); err == nil {
			defer sessionMod.UnregisterActive(repoRoot, sess.SessionID())
		}
	}

	// API setup
	key := agent.APIKey()
	if key == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}

	client := anthropic.NewClient(option.WithAPIKey(key))
	a := agent.New(&client, systemPrompt)

	// Add tools unless disabled
	if !runNoTools {
		var agentTools []agent.Tool
		if agentConfig != nil && len(agentConfig.Tools) > 0 {
			for _, toolName := range agentConfig.Tools {
				for _, t := range tools.All() {
					if t.Name == toolName {
						agentTools = append(agentTools, t)
						break
					}
				}
			}
		} else {
			agentTools = tools.All()
		}
		for _, t := range agentTools {
			a.AddTool(t)
		}
	}

	if runThinking > 0 {
		a.SetThinking(runThinking)
	}

	// Tool call output goes to stderr so stdout stays clean
	hooks := &agent.Hooks{
		OnToolCall: func(toolID, name string, input []byte) {
			fmt.Fprintf(os.Stderr, "⚡ %s\n", name)
			if sess != nil {
				sess.LogToolCall(toolID, name, json.RawMessage(input))
			}
		},
		OnToolResult: func(toolID, name, output string, isError bool) {
			if isError {
				fmt.Fprintf(os.Stderr, "  ✗ %s\n", truncate(output, 200))
			}
			if sess != nil {
				sess.LogToolResult(toolID, name, output, isError)
			}
		},
	}
	turnCounter := 0
	if sess != nil {
		logHooks := sess.Hooks()
		hooks.OnRequest = func(params *anthropic.MessageNewParams) {
			if logHooks.OnRequest != nil {
				logHooks.OnRequest(params)
			}
			turnCounter++
			sessionMod.WritePromptBreakdown(sess.FilePath(), sess.SessionID(), turnCounter, params)
		}
		hooks.OnThinking = logHooks.OnThinking
	}
	a.SetHooks(hooks)

	if sess != nil {
		sess.LogUser(input)
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(input)),
	}

	result, err := a.Run(context.Background(), runModel, &messages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if sess != nil {
		sess.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	// Print response to stdout — clean, no ANSI
	fmt.Print(result.Text)

	// Print stats to stderr
	cost := calcCost(runModel, result.InputTokens, result.OutputTokens)
	fmt.Fprintf(os.Stderr, "\n[in=%d | out=%d | tools=%d | $%.4f]\n",
		result.InputTokens, result.OutputTokens, result.ToolCalls, cost)
}
