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
	"github.com/kayushkin/inber/agent/registry"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
	"github.com/spf13/cobra"
)

var (
	chatModel          string
	chatThinking       int64
	chatAgent          string
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	Long:  `Start an interactive REPL chat session with the agent.`,
	Run:   runChat,
}

func init() {
	chatCmd.Flags().StringVarP(&chatModel, "model", "m", agent.DefaultModel, "Claude model to use")
	chatCmd.Flags().Int64VarP(&chatThinking, "thinking", "t", 0, "Enable extended thinking with token budget (0=disabled)")
	chatCmd.Flags().StringVarP(&chatAgent, "agent", "a", "", "Agent name to load from registry")
}

func runChat(cmd *cobra.Command, args []string) {
	// Detect repository root
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not find repo root: %v\n", err)
		repoRoot, _ = os.Getwd()
	}

	// Load agent from registry if specified
	var agentConfig *registry.AgentConfig
	var identityText string
	
	if chatAgent != "" {
		configs, err := registry.LoadConfig(
			filepath.Join(repoRoot, "agents.json"),
			filepath.Join(repoRoot, "agents"),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading agents: %v\n", err)
			os.Exit(1)
		}
		
		var ok bool
		agentConfig, ok = configs[chatAgent]
		if !ok {
			fmt.Fprintf(os.Stderr, "agent not found: %s\n", chatAgent)
			os.Exit(1)
		}
		
		// Override model if agent has one configured
		if agentConfig.Model != "" {
			chatModel = agentConfig.Model
		}
		
		// Load identity
		identityText = agentConfig.System
		
		fmt.Printf("%sloaded agent: %s%s\n", dim, chatAgent, reset)
	} else {
		identityText = "You are a helpful coding assistant. You have access to shell, file reading/writing/editing, and directory listing tools. Use them to help the user."
	}

	// Auto-load context
	fmt.Printf("%sloading context...%s", dim, reset)
	contextCfg := inbercontext.DefaultAutoLoadConfig(repoRoot)
	contextCfg.IdentityText = identityText
	
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

	// Load memory
	memStore, err := memory.OpenOrCreate(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: memory disabled: %v\n", err)
	} else {
		defer memStore.Close()
		// Load high-importance memories into context
		if err := memory.LoadIntoContext(memStore, contextStore, 10, 0.6); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load memories: %v\n", err)
		}
	}

	// Session logging
	agentName := chatAgent
	if agentName == "" {
		agentName = "default"
	}
	
	sess, err := session.New("logs", chatModel, agentName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging disabled: %v\n", err)
	} else {
		defer sess.Close()
		fmt.Printf("%slogging to %s%s\n", dim, sess.FilePath(), reset)
	}

	// Agent setup
	key := agent.APIKey()
	if key == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}

	client := anthropic.NewClient(option.WithAPIKey(key))
	a := agent.New(&client, "")

	// Add tools (from agent config or default)
	var agentTools []agent.Tool
	if agentConfig != nil && len(agentConfig.Tools) > 0 {
		// Load scoped tools
		for _, toolName := range agentConfig.Tools {
			for _, t := range tools.All() {
				if t.Name == toolName {
					agentTools = append(agentTools, t)
					break
				}
			}
		}
		// Add memory tools if agent has them
		if memStore != nil {
			for _, toolName := range agentConfig.Tools {
				if strings.HasPrefix(toolName, "memory_") {
					for _, t := range memory.AllMemoryTools(memStore) {
						if t.Name == toolName {
							agentTools = append(agentTools, t)
							break
						}
					}
				}
			}
		}
	} else {
		// Default: all tools
		agentTools = tools.All()
		if memStore != nil {
			agentTools = append(agentTools, memory.AllMemoryTools(memStore)...)
		}
	}

	for _, t := range agentTools {
		a.AddTool(t)
	}

	if chatThinking > 0 {
		a.SetThinking(chatThinking)
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

	header := fmt.Sprintf("inber — model: %s", chatModel)
	if chatThinking > 0 {
		header += fmt.Sprintf(" — thinking: %d tokens", chatThinking)
	}
	if chatAgent != "" {
		header += fmt.Sprintf(" — agent: %s", chatAgent)
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
		for _, t := range agentTools {
			a.AddTool(t)
		}
		
		if chatThinking > 0 {
			a.SetThinking(chatThinking)
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

		result, err := a.Run(context.Background(), chatModel, &messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%serror: %v%s\n", red, err, reset)
			continue
		}

		if sess != nil {
			sess.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
		}

		DisplayResponse(result.Text)
		DisplayStats(result, chatModel)
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
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}

// buildSystemPrompt builds a context-aware system prompt
func buildSystemPrompt(store *inbercontext.Store, userMessage string) string {
	messageTags := inbercontext.AutoTag(userMessage, "user")
	builder := inbercontext.NewBuilder(store, 50000)
	chunks := builder.Build(messageTags)
	
	var parts []string
	for _, chunk := range chunks {
		parts = append(parts, chunk.Text)
	}
	
	return strings.Join(parts, "\n\n---\n\n")
}
