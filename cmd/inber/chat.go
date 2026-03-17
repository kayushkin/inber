package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var (
	chatModel    string
	chatThinking int64
	chatAgent    string
	chatRaw      bool
	chatNoTools  bool
	chatNoHooks  bool
	chatSystem   string
	chatNew      bool
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive REPL chat session",
	Long: `Start an interactive chat session with the agent.

Special commands:
  /btw <note>   Inject context without triggering a full turn
  /quit         Exit the chat
  /exit         Exit the chat

Examples:
  inber chat
  inber chat -a myagent
  inber chat --model claude-sonnet-4-20250514`,
	Run: runChat,
}

func init() {
	chatCmd.Flags().StringVarP(&chatModel, "model", "m", agent.DefaultModel, "Claude model to use")
	chatCmd.Flags().Int64VarP(&chatThinking, "thinking", "t", 0, "Enable extended thinking with token budget (0=disabled)")
	chatCmd.Flags().StringVarP(&chatAgent, "agent", "a", "", "Agent name to load from registry")
	chatCmd.Flags().BoolVar(&chatRaw, "raw", false, "Skip context and memory loading")
	chatCmd.Flags().BoolVar(&chatNoTools, "no-tools", false, "Disable all tools")
	chatCmd.Flags().BoolVar(&chatNoHooks, "no-hooks", false, "Skip post-request hooks")
	chatCmd.Flags().StringVar(&chatSystem, "system", "", "Override system prompt")
	chatCmd.Flags().BoolVarP(&chatNew, "new", "n", false, "Start a new session")
}

func runChat(cmd *cobra.Command, args []string) {
	cfg := engine.EngineConfig{
		Model:              chatModel,
		ModelExplicitlySet: cmd.Flags().Changed("model"),
		Thinking:           chatThinking,
		AgentName:          chatAgent,
		Raw:                chatRaw,
		NoTools:            chatNoTools,
		NoHooks:            chatNoHooks,
		SystemOverride:     chatSystem,
		CommandName:        "chat",
		NewSession:         chatNew,
		Display: &engine.DisplayHooks{
			OnThinking: func(text string) {
				engine.DisplayThinking(text)
			},
			OnToolCall: func(name string, input string) {
				engine.DisplayToolCall(name, input)
			},
			OnToolResult: func(name string, output string, isError bool) {
				engine.DisplayToolResult(name, output, isError)
			},
			OnTextDelta: func(text string) {
				fmt.Print(text)
			},
		},
		AutoWorkflow: engine.AutoWorkflowConfig{
			AutoCommit: true,
			AutoFormat: true,
		},
	}

	eng, err := engine.NewEngine(cfg)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}
	defer eng.Close()

	scanner := bufio.NewScanner(os.Stdin)
	// Allow long inputs (1MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fmt.Fprintf(os.Stderr, "inber chat (%s) — type /quit to exit, /btw <note> for context injection\n\n", eng.Model)

	for {
		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Slash commands
		switch {
		case input == "/quit" || input == "/exit":
			fmt.Fprintln(os.Stderr, "bye")
			return

		case strings.HasPrefix(input, "/btw "):
			note := strings.TrimPrefix(input, "/btw ")
			note = strings.TrimSpace(note)
			if note == "" {
				continue
			}
			// Inject as a user message with a marker — no agent turn triggered
			eng.Messages = append(eng.Messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock("[btw] "+note),
			))
			// Add a synthetic assistant ack so the conversation stays well-formed
			// (Anthropic API requires alternating user/assistant messages)
			eng.Messages = append(eng.Messages, anthropic.NewAssistantMessage(
				anthropic.NewTextBlock("(noted)"),
			))
			fmt.Fprintf(os.Stderr, "  noted.\n")
			continue

		case input == "/btw":
			fmt.Fprintf(os.Stderr, "  usage: /btw <note>\n")
			continue
		}

		startTime := time.Now()
		result, err := eng.RunTurn(input)
		durationMs := time.Since(startTime).Milliseconds()
		if err != nil {
			engine.Log.Error("%v", err)
			continue
		}

		// Response already streamed via OnTextDelta
		fmt.Println() // newline after streamed response

		// Brief stats
		cost := session.CalcCostWithCache(eng.Model, result.InputTokens, result.OutputTokens,
			result.CacheReadTokens, result.CacheCreationTokens)
		fmt.Fprintf(os.Stderr, "  [%dms | in=%d out=%d | $%.4f]\n\n",
			durationMs, result.InputTokens, result.OutputTokens, cost)
	}
}
