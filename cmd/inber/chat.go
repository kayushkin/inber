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
	"github.com/google/uuid"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
	"github.com/spf13/cobra"
)

var (
	chatModel          string
	chatThinking       int64
	chatAgent          string
	chatStep           bool
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
	chatCmd.Flags().BoolVarP(&chatStep, "step", "s", false, "Enable step mode (pause after each model turn)")
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
	
	sess, err := sessionMod.New("logs", chatModel, agentName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging disabled: %v\n", err)
	} else {
		defer sess.Close()
		fmt.Printf("%slogging to %s%s\n", dim, sess.FilePath(), reset)
		// Register active session
		if _, err := sessionMod.RegisterActive(repoRoot, sess, "chat"); err == nil {
			defer sessionMod.UnregisterActive(repoRoot, sess.SessionID())
		}
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

	// Prompt breakdown tracking
	turnCounter := 0

	// Build hooks helper
	buildHooks := func() *agent.Hooks {
		hooks := &agent.Hooks{
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
			hooks = &agent.Hooks{
				OnRequest: func(params *anthropic.MessageNewParams) {
					logHooks.OnRequest(params)
					// Prompt breakdown
					turnCounter++
					sessionMod.WritePromptBreakdown(sess.FilePath(), sess.SessionID(), turnCounter, params)
				},
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
			}
		}
		return hooks
	}

	a.SetHooks(buildHooks())

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
	if chatStep {
		header += " — step mode"
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
		
		a.SetHooks(buildHooks())

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

		// Step mode
		if chatStep {
			if !runStepMode(scanner, contextStore, &messages, buildSystemPrompt) {
				break
			}
		}
	}

	// Auto-save session summary to memory
	if memStore != nil && len(messages) > 0 {
		saveSessionSummary(memStore, messages, agentName)
	}

	fmt.Println()
}

// saveSessionSummary generates a brief session summary and saves it to memory.
func saveSessionSummary(store *memory.Store, messages []anthropic.MessageParam, agentName string) {
	var parts []string
	for _, msg := range messages {
		role := string(msg.Role)
		for _, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				parts = append(parts, fmt.Sprintf("%s: %s", role, text))
			}
		}
	}

	if len(parts) == 0 {
		return
	}

	summary := fmt.Sprintf("Session summary (%s):\n%s", agentName, strings.Join(parts, "\n"))
	if len(summary) > 2000 {
		summary = summary[:2000]
	}

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    summary,
		Tags:       []string{"session-summary", agentName},
		Importance: 0.4,
		Source:     "system",
	}

	if err := store.Save(m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save session summary: %v\n", err)
	}
}

// runStepMode runs the step-mode REPL. Returns false if user wants to quit.
func runStepMode(scanner *bufio.Scanner, store *inbercontext.Store, messages *[]anthropic.MessageParam, buildSysPrompt func(*inbercontext.Store, string) string) bool {
	for {
		fmt.Printf("\n%s[step]%s > ", cyan+bold, reset)
		if !scanner.Scan() {
			return false
		}
		input := strings.TrimSpace(scanner.Text())

		switch {
		case input == "" || input == "c" || input == "continue":
			return true

		case input == "q" || input == "quit":
			return false

		case input == "context":
			chunks := store.ListAll()
			if len(chunks) == 0 {
				fmt.Println("  No context chunks.")
				continue
			}
			fmt.Printf("  %-12s %-20s %-8s %s\n", "ID", "Tags", "Tokens", "Source")
			fmt.Printf("  %-12s %-20s %-8s %s\n", "---", "---", "---", "---")
			for _, c := range chunks {
				id := c.ID
				if len(id) > 12 {
					id = id[:12]
				}
				tags := strings.Join(c.Tags, ",")
				if len(tags) > 20 {
					tags = tags[:20]
				}
				fmt.Printf("  %-12s %-20s %-8d %s\n", id, tags, c.Tokens, c.Source)
			}

		case strings.HasPrefix(input, "context add "):
			parts := strings.SplitN(input, " ", 4)
			if len(parts) < 4 {
				fmt.Println("  Usage: context add <tag> <text>")
				continue
			}
			tag := parts[2]
			text := parts[3]
			id := fmt.Sprintf("step-%d", len(store.ListAll()))
			err := store.Add(inbercontext.Chunk{
				ID:     id,
				Text:   text,
				Tags:   []string{tag},
				Source: "user",
			})
			if err != nil {
				fmt.Printf("  %serror: %v%s\n", red, err, reset)
			} else {
				fmt.Printf("  Added chunk %s\n", id)
			}

		case strings.HasPrefix(input, "context remove "):
			id := strings.TrimPrefix(input, "context remove ")
			if store.Delete(id) {
				fmt.Printf("  Removed %s\n", id)
			} else {
				fmt.Printf("  %snot found: %s%s\n", red, id, reset)
			}

		case strings.HasPrefix(input, "context edit "):
			parts := strings.SplitN(input, " ", 4)
			if len(parts) < 4 {
				fmt.Println("  Usage: context edit <id> <new-text>")
				continue
			}
			id := parts[2]
			newText := parts[3]
			chunk, ok := store.Get(id)
			if !ok {
				fmt.Printf("  %snot found: %s%s\n", red, id, reset)
				continue
			}
			chunk.Text = newText
			chunk.Tokens = inbercontext.EstimateTokens(newText)
			store.Add(chunk)
			fmt.Printf("  Updated %s\n", id)

		case input == "messages":
			msgs := *messages
			if len(msgs) == 0 {
				fmt.Println("  No messages.")
				continue
			}
			for i, msg := range msgs {
				role := string(msg.Role)
				content := "(no text)"
				for _, block := range msg.Content {
					if block.OfText != nil {
						content = block.OfText.Text
						break
					}
					if block.OfToolResult != nil {
						content = "[tool_result]"
						break
					}
				}
				if len(content) > 80 {
					content = content[:80] + "..."
				}
				fmt.Printf("  %d. %s%-9s%s %s\n", i+1, bold, role, reset, content)
			}

		case strings.HasPrefix(input, "messages drop "):
			nStr := strings.TrimPrefix(input, "messages drop ")
			n := 0
			fmt.Sscanf(nStr, "%d", &n)
			if n <= 0 {
				fmt.Println("  Usage: messages drop <n>")
				continue
			}
			msgs := *messages
			if n > len(msgs) {
				n = len(msgs)
			}
			*messages = msgs[:len(msgs)-n]
			fmt.Printf("  Dropped %d messages\n", n)

		case input == "system":
			prompt := buildSysPrompt(store, "")
			if prompt == "" {
				fmt.Println("  (empty system prompt)")
			} else {
				if len(prompt) > 2000 {
					fmt.Println(prompt[:2000] + "...")
				} else {
					fmt.Println(prompt)
				}
			}

		case input == "tokens":
			sysPrompt := buildSysPrompt(store, "")
			sysTok := inbercontext.EstimateTokens(sysPrompt)
			msgTok := 0
			for _, msg := range *messages {
				msgTok += 4
				for _, block := range msg.Content {
					if block.OfText != nil {
						msgTok += inbercontext.EstimateTokens(block.OfText.Text)
					}
				}
			}
			ctxTok := 0
			for _, c := range store.ListAll() {
				ctxTok += c.Tokens
			}
			fmt.Printf("  System: ~%d tokens\n", sysTok)
			fmt.Printf("  Messages: ~%d tokens\n", msgTok)
			fmt.Printf("  Context chunks: ~%d tokens\n", ctxTok)
			fmt.Printf("  Total: ~%d tokens\n", sysTok+msgTok)

		default:
			fmt.Printf("  %sUnknown command. Available: continue, context, messages, system, tokens, quit%s\n", dim, reset)
		}
	}
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
