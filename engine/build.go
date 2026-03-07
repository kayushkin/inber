package engine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
)

// buildTools resolves tools from agent config or defaults.
func (e *Engine) buildTools() []agent.Tool {
	var result []agent.Tool

	if e.AgentConfig != nil && len(e.AgentConfig.Tools) > 0 {
		for _, toolName := range e.AgentConfig.Tools {
			if toolName == "repo_map" {
				ignorePatterns := []string{
					"*.log", "*.tmp", ".git/*", "vendor/*",
					"node_modules/*", ".openclaw/*", "logs/*",
				}
				result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
				continue
			}
			if toolName == "recent_files" {
				result = append(result, tools.RecentFiles(e.repoRoot))
				continue
			}
			if toolName == "spawn_agent" || toolName == "check_spawns" {
				if e.agentRegistry != nil {
					if toolName == "spawn_agent" {
						result = append(result, e.agentRegistry.SpawnAgentTool())
					} else {
						result = append(result, e.agentRegistry.CheckSpawnsTool())
					}
				}
				continue
			}
			for _, t := range tools.All() {
				if t.Name == toolName {
					result = append(result, t)
					break
				}
			}
		}
		if e.MemStore != nil {
			for _, toolName := range e.AgentConfig.Tools {
				if strings.HasPrefix(toolName, "memory_") {
					for _, t := range memory.AllMemoryTools(e.MemStore) {
						if t.Name == toolName {
							result = append(result, t)
							break
						}
					}
				}
			}
		}
	} else {
		result = tools.All()
		if e.MemStore != nil {
			result = append(result, memory.AllMemoryTools(e.MemStore)...)
		}
		ignorePatterns := []string{
			"*.log", "*.tmp", ".git/*", "vendor/*",
			"node_modules/*", ".openclaw/*", "logs/*",
		}
		result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
		result = append(result, tools.RecentFiles(e.repoRoot))
	}

	return result
}

// needsSpawnTools checks if the tool list includes spawn_agent or check_spawns.
func (e *Engine) needsSpawnTools(tools []string) bool {
	for _, t := range tools {
		if t == "spawn_agent" || t == "check_spawns" {
			return true
		}
	}
	return false
}

// contextBudget returns the token budget for memory context loading.
func (e *Engine) contextBudget(userMessage string) (minImportance float64, tokenBudget int) {
	msgTokens := inbercontext.EstimateTokens(userMessage)
	baseBudget := 4000

	switch {
	case e.TurnCounter == 0:
		return 0, baseBudget
	case e.consecutiveErrors >= 5:
		return 0, 50000
	case e.consecutiveErrors >= 3:
		return 0, 35000
	case e.consecutiveErrors >= 1 || e.lastTurnHadError:
		return 0, 20000
	case msgTokens > 1000:
		return 0, 15000
	case msgTokens > 300:
		return 0, 10000
	case e.TurnCounter > 15:
		return 0, 8000
	default:
		return 0, 6000
	}
}

// BuildSystemPrompt builds a context-aware system prompt as individual named blocks.
func (e *Engine) BuildSystemPrompt(userMessage string) []sessionMod.NamedBlock {
	if e.MemStore != nil {
		messageTags := inbercontext.AutoTag(userMessage, "user")
		minImportance, tokenBudget := e.contextBudget(userMessage)

		req := memory.BuildContextRequest{
			Tags:              messageTags,
			TokenBudget:       tokenBudget,
			MinImportance:     minImportance,
			IncludeAlwaysLoad: true,
			ExcludeTags:       []string{"session-summary", "repo-map", "code-introspection"},
			MaxChunkSize:      5000,
			TruncateThreshold: 500,
			TruncatePreview:   300,
		}

		memories, tokensUsed, err := e.MemStore.BuildContext(req)
		if err != nil {
			Log.Warn("failed to build context from memory: %v", err)
			return nil
		}

		Log.Info("context: %d memories, %d tokens (min_importance=%.1f, budget=%d)", len(memories), tokensUsed, minImportance, tokenBudget)

		var blocks []sessionMod.NamedBlock
		for _, m := range memories {
			text := m.Content
			if text == "" {
				text = m.Summary
			}
			if text == "" {
				continue
			}
			desc := fmt.Sprintf("%s (%.1f", m.ID[:8], m.Importance)
			if len(m.Tags) > 0 {
				desc += fmt.Sprintf(", tags: %s", strings.Join(m.Tags, ","))
			}
			desc += ")"
			blocks = append(blocks, sessionMod.NamedBlock{ID: desc, Text: text})
		}

		if e.workspace != nil {
			e.workspace.WriteSystem(blocks)
		}
		return blocks
	}
	
	if e.ContextStore == nil {
		return nil
	}
	messageTags := inbercontext.AutoTag(userMessage, "user")
	builder := inbercontext.NewBuilder(e.ContextStore, 50000)
	chunks := builder.Build(messageTags)

	blocks := make([]sessionMod.NamedBlock, len(chunks))
	for i, chunk := range chunks {
		blocks[i] = sessionMod.NamedBlock{ID: chunk.ID, Text: chunk.Text}
	}

	if e.workspace != nil {
		e.workspace.WriteSystem(blocks)
	}
	return blocks
}

// buildAgent creates a fresh Agent with current system prompt, tools, and hooks.
// Automatically adds cache_control to system blocks for prompt caching.
func (e *Engine) buildAgent(blocks []sessionMod.NamedBlock) *agent.Agent {
	systemBlocks := make([]anthropic.TextBlockParam, len(blocks))
	for i, b := range blocks {
		systemBlocks[i] = anthropic.TextBlockParam{Text: b.Text}
	}
	
	// Enable prompt caching: add cache_control to last system block
	// This caches the entire system prompt (all preceding blocks)
	if len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	a := agent.NewWithSystemBlocks(e.Client, systemBlocks)
	for _, t := range e.agentTools {
		a.AddTool(t)
	}
	if e.thinkingBud > 0 {
		a.SetThinking(e.thinkingBud)
	}
	a.SetHooks(e.buildHooks())

	// Wire up mid-run message injection
	if e.injections != nil {
		a.InjectCheck = e.buildInjectCheck()
	}

	// Wire up turn/token limit checks
	if e.maxTurns > 0 || e.maxInputTokens > 0 {
		a.SetLimitCheck(e.buildLimitCheck())
	}

	modelInfo, ok := agent.Models[e.Model]
	if ok {
		a.SetContextWindow(modelInfo.ContextWindow)
	} else {
		a.SetContextWindow(200000)
	}
	a.SetBeforeRequest(func(messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam {
		cfg := e.pruneConfig()
		cfg.TokenBudget = contextWindow / 2

		if conversation.ShouldPrune(messages, cfg) {
			Log.Warn("context approaching limit (%d messages), pruning", len(messages))
			pruned, result, err := conversation.PruneConversation(context.Background(), messages, e.MemStore, "", cfg)
			if err == nil {
				Log.Info("pruned: %d tokens freed", result.TokensFreed)
				messages = pruned
			}
		}

		maxMessages := cfg.KeepRecentTurns * 2
		if len(messages) > maxMessages {
			dropTo := len(messages) - maxMessages
			for dropTo < len(messages) {
				msg := messages[dropTo]
				if msg.Role == anthropic.MessageParamRoleUser && !hasToolResult(msg) {
					break
				}
				dropTo++
			}
			if dropTo < len(messages) && dropTo > 0 {
				Log.Warn("hard-dropping %d old messages (%d → %d)", dropTo, len(messages), len(messages)-dropTo)
				messages = messages[dropTo:]
			}
		}

		return messages
	})

	e.Agent = a
	return a
}

// hasToolResult checks if a message contains any tool_result content blocks.
func hasToolResult(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return true
		}
	}
	return false
}

// buildInjectCheck creates a closure that drains the injection channel.
func (e *Engine) buildInjectCheck() func() []string {
	return func() []string {
		var result []string
		for {
			select {
			case text, ok := <-e.injections:
				if !ok {
					return result // channel closed
				}
				result = append(result, text)
				Log.Info("injection received: %s", truncateLog(text, 80))
			default:
				return result // no more pending
			}
		}
	}
}

func truncateLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// buildLimitCheck creates a closure that checks turn/token limits.
func (e *Engine) buildLimitCheck() func(result *agent.TurnResult) (bool, string) {
	return func(result *agent.TurnResult) (bool, string) {
		// Check cumulative input tokens (session-level + current turn)
		totalInput := e.SessionInputTokens + result.InputTokens
		if e.maxInputTokens > 0 && totalInput > e.maxInputTokens {
			return true, fmt.Sprintf(
				"Input token budget exceeded: %dk / %dk used (%d tool calls so far).",
				totalInput/1000, e.maxInputTokens/1000, result.ToolCalls,
			)
		}

		// Check API round-trips within this turn
		// apiCalls is tracked in Agent.Run(); result.ToolCalls is a proxy
		// (one API call can have multiple tool calls, but each tool_use response = 1 API call)
		// We approximate: tool_calls + 1 >= maxTurns (the +1 is the initial request)
		if e.maxTurns > 0 && result.ToolCalls >= e.maxTurns {
			return true, fmt.Sprintf(
				"Turn limit exceeded: %d tool calls made (limit: %d). Used %dk input tokens.",
				result.ToolCalls, e.maxTurns, totalInput/1000,
			)
		}

		return false, ""
	}
}

// buildHooks creates hooks that combine logging and display.
func (e *Engine) buildHooks() *agent.Hooks {
	hooks := &agent.Hooks{}

	if e.display != nil && e.display.OnThinking != nil {
		hooks.OnThinking = e.display.OnThinking
	}
	if e.display != nil && e.display.OnToolCall != nil {
		hooks.OnToolCall = func(toolID, name string, input []byte) {
			e.display.OnToolCall(name, string(input))
		}
	}
	if e.display != nil && e.display.OnToolResult != nil {
		hooks.OnToolResult = func(toolID, name, output string, isError bool) {
			e.display.OnToolResult(name, output, isError)
		}
	}

	if e.Session != nil {
		logHooks := e.Session.Hooks()
		origThinking := hooks.OnThinking
		origToolCall := hooks.OnToolCall
		origToolResult := hooks.OnToolResult

		hooks.OnRequest = func(params *anthropic.MessageNewParams) {
			if logHooks.OnRequest != nil {
				logHooks.OnRequest(params)
			}
			sessionMod.WritePromptBreakdown(e.Session.FilePath(), e.Session.SessionID(), e.TurnCounter, params, e.lastNamedBlocks)
		}
		hooks.OnThinking = func(text string) {
			if logHooks.OnThinking != nil {
				logHooks.OnThinking(text)
			}
			if origThinking != nil {
				origThinking(text)
			}
		}
		hooks.OnToolCall = func(toolID, name string, input []byte) {
			if e.autoRefMgr != nil && e.toolInputsCache != nil {
				e.toolInputsCache[toolID] = string(input)
			}
			if logHooks.OnToolCall != nil {
				logHooks.OnToolCall(toolID, name, input)
			}
			if origToolCall != nil {
				origToolCall(toolID, name, input)
			}
		}
		hooks.OnToolResult = func(toolID, name, output string, isError bool) {
			if logHooks.OnToolResult != nil {
				logHooks.OnToolResult(toolID, name, output, isError)
			}
			if origToolResult != nil {
				origToolResult(toolID, name, output, isError)
			}
			if isError {
				e.consecutiveErrors++
				e.lastTurnHadError = true
			}
			if !isError && e.autoRefMgr != nil && e.toolInputsCache != nil {
				inputJSON := e.toolInputsCache[toolID]
				if err := e.autoRefMgr.OnToolResult(toolID, name, inputJSON, output); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to auto-create reference for %s: %v\n", name, err)
				}
			}
		}
		hooks.ModifyToolResult = func(toolID, name, output string, isError bool) string {
			if e.Session != nil {
				return e.Session.TruncateToolResult(name, output, isError)
			}
			return ""
		}
		hooks.PostToolResult = func(toolID, name, output string, isError bool) string {
			if isError || e.workflowHooks == nil {
				return ""
			}
			toolInput := e.toolInputsCache[toolID]
			result := e.workflowHooks.OnToolResult(name, toolInput, output, isError)
			if e.toolInputsCache != nil {
				delete(e.toolInputsCache, toolID)
			}
			return result
		}
		hooks.OnResponse = func(resp *anthropic.Message) {
			stopReason := string(resp.StopReason)
			toolCalls := 0
			for _, block := range resp.Content {
				if block.Type == "tool_use" {
					toolCalls++
				}
			}
			e.Session.EndTurn(
				int(resp.Usage.InputTokens),
				int(resp.Usage.OutputTokens),
				toolCalls,
				stopReason,
				"",
			)
			if !e.lastTurnHadError {
				e.consecutiveErrors = 0
			}
			e.lastTurnHadError = false
			if logHooks.OnResponse != nil {
				logHooks.OnResponse(resp)
			}
		}
	}

	return hooks
}
