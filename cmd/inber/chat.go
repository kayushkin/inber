package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	inbercontext "github.com/kayushkin/inber/context"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var (
	chatModel    string
	chatThinking int64
	chatAgent    string
	chatStep     bool
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
	eng, err := NewEngine(EngineConfig{
		Model:       chatModel,
		Thinking:    chatThinking,
		AgentName:   chatAgent,
		CommandName: "chat",
		Display: &DisplayHooks{
			OnThinking:   DisplayThinking,
			OnToolCall:   DisplayToolCall,
			OnToolResult: DisplayToolResult,
		},
	})
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}
	defer eng.Close()

	// REPL
	scanner := bufio.NewScanner(os.Stdin)

	header := fmt.Sprintf("inber — model: %s", eng.Model)
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

		result, err := eng.RunTurn(input)
		if err != nil {
			Log.Errorf("%v", err)
			continue
		}

		DisplayResponse(result.Text)
		DisplayStats(result, eng.Model)

		// Step mode
		if chatStep {
			if !runStepMode(scanner, eng.ContextStore, &eng.Messages, eng.BuildSystemPrompt) {
				break
			}
		}
	}

	fmt.Println()
}

// runStepMode runs the step-mode REPL. Returns false if user wants to quit.
func runStepMode(scanner *bufio.Scanner, store *inbercontext.Store, messages *[]anthropic.MessageParam, buildSysPrompt func(string) []sessionMod.NamedBlock) bool {
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
			blocks := buildSysPrompt("")
			if len(blocks) == 0 {
				fmt.Println("  (empty system prompt)")
			} else {
				for _, b := range blocks {
					fmt.Printf("  --- %s ---\n", b.ID)
					text := b.Text
					if len(text) > 2000 {
						text = text[:2000] + "..."
					}
					fmt.Println(text)
				}
			}

		case input == "tokens":
			blocks := buildSysPrompt("")
			sysTok := 0
			for _, b := range blocks {
				sysTok += inbercontext.EstimateTokens(b.Text)
			}
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
