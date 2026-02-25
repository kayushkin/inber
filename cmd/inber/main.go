package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/agent"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
)

func main() {
	cfg := ParseConfig()
	if cfg == nil {
		return
	}

	// Detect repository root
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not find repo root: %v\n", err)
		repoRoot, _ = os.Getwd()
	}

	// Auto-load context
	fmt.Printf("%sloading context...%s", dim, reset)
	contextCfg := inbercontext.DefaultAutoLoadConfig(repoRoot)
	contextCfg.IdentityText = "You are a helpful coding assistant. You have access to shell, file reading/writing/editing, and directory listing tools. Use them to help the user."
	
	contextStore, err := inbercontext.AutoLoad(contextCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nwarning: context auto-load failed: %v\n", err)
		contextStore = inbercontext.NewStore()
	}
	
	// Load project-specific context files
	if err := inbercontext.LoadProjectContext(contextStore, repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load project context: %v\n", err)
	}
	
	fmt.Printf(" done (%d chunks)\n", contextStore.Count())

	// Session logging
	sess, err := session.New("logs", cfg.Model, "", "") // empty agent name and parent for single-agent mode
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging disabled: %v\n", err)
	} else {
		defer sess.Close()
		fmt.Printf("%slogging to %s%s\n", dim, sess.FilePath(), reset)
	}

	// Agent setup
	client := anthropic.NewClient(option.WithAPIKey(cfg.Key))
	a := agent.New(&client, "")

	for _, t := range tools.All() {
		a.AddTool(t)
	}

	if cfg.ThinkingBudget > 0 {
		a.SetThinking(cfg.ThinkingBudget)
	}

	// Wire up hooks: logging + display
	displayHooks := &agent.Hooks{
		OnThinking: func(text string) {
			DisplayThinking(text)
		},
		OnToolCall: func(toolID, name string, input []byte) {
			DisplayToolCall(name, string(input))
		},
		OnToolResult: func(toolID, name, output string, isError bool) {
			DisplayToolResult(name, output, isError)
		},
	}

	if sess != nil {
		logHooks := sess.Hooks()
		// Merge: both log and display
		a.SetHooks(&agent.Hooks{
			OnRequest: logHooks.OnRequest,
			OnThinking: func(text string) {
				logHooks.OnThinking(text)
				displayHooks.OnThinking(text)
			},
			OnToolCall: func(toolID, name string, input []byte) {
				logHooks.OnToolCall(toolID, name, input)
				displayHooks.OnToolCall(toolID, name, input)
			},
			OnToolResult: func(toolID, name, output string, isError bool) {
				logHooks.OnToolResult(toolID, name, output, isError)
				displayHooks.OnToolResult(toolID, name, output, isError)
			},
		})
	} else {
		a.SetHooks(displayHooks)
	}

	// REPL
	var messages []anthropic.MessageParam
	scanner := bufio.NewScanner(os.Stdin)

	header := fmt.Sprintf("inber — model: %s", cfg.Model)
	if cfg.ThinkingBudget > 0 {
		header += fmt.Sprintf(" — thinking: %d tokens", cfg.ThinkingBudget)
	}
	fmt.Printf("%s — ctrl+d to quit\n", header)

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if sess != nil {
			sess.LogUser(input)
		}

		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(input)))

		// Build context-aware system prompt
		systemPrompt := buildSystemPrompt(contextStore, input)
		a = agent.New(&client, systemPrompt)
		
		// Re-attach tools
		for _, t := range tools.All() {
			a.AddTool(t)
		}
		
		if cfg.ThinkingBudget > 0 {
			a.SetThinking(cfg.ThinkingBudget)
		}
		
		// Re-attach hooks
		if sess != nil {
			logHooks := sess.Hooks()
			a.SetHooks(&agent.Hooks{
				OnRequest: logHooks.OnRequest,
				OnThinking: func(text string) {
					logHooks.OnThinking(text)
					DisplayThinking(text)
				},
				OnToolCall: func(toolID, name string, input []byte) {
					logHooks.OnToolCall(toolID, name, input)
					DisplayToolCall(name, string(input))
				},
				OnToolResult: func(toolID, name, output string, isError bool) {
					logHooks.OnToolResult(toolID, name, output, isError)
					DisplayToolResult(name, output, isError)
				},
			})
		} else {
			a.SetHooks(&agent.Hooks{
				OnThinking: func(text string) {
					DisplayThinking(text)
				},
				OnToolCall: func(toolID, name string, input []byte) {
					DisplayToolCall(name, string(input))
				},
				OnToolResult: func(toolID, name, output string, isError bool) {
					DisplayToolResult(name, output, isError)
				},
			})
		}

		result, err := a.Run(context.Background(), cfg.Model, &messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%serror: %v%s\n", red, err, reset)
			continue
		}

		if sess != nil {
			sess.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
		}

		DisplayResponse(result.Text)
		DisplayStats(result, cfg.Model)
	}
	fmt.Println()
}

// findRepoRoot finds the repository root by looking for .git directory
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	
	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}

// buildSystemPrompt builds a context-aware system prompt
func buildSystemPrompt(store *inbercontext.Store, userMessage string) string {
	// Auto-tag the user message
	messageTags := inbercontext.AutoTag(userMessage, "user")
	
	// Build context with 50k token budget (reasonable for system prompt)
	builder := inbercontext.NewBuilder(store, 50000)
	chunks := builder.Build(messageTags)
	
	// Assemble system prompt
	var parts []string
	for _, chunk := range chunks {
		parts = append(parts, chunk.Text)
	}
	
	return strings.Join(parts, "\n\n---\n\n")
}
