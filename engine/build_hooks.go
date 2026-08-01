package engine

import (
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/guard"
	"github.com/kayushkin/inber/internal/textutil"
)

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
	return textutil.TruncateWith(s, n, "...")
}

// buildLimitCheck creates a closure that checks turn/token/time limits.
func (e *Engine) buildLimitCheck() func(result *agent.TurnResult) (bool, string) {
	return func(result *agent.TurnResult) (bool, string) {
		// Check cumulative input tokens (session-level + current turn)
		totalInput := e.Tokens.Input + result.InputTokens
		if e.Limits.MaxInputTokens > 0 && totalInput > e.Limits.MaxInputTokens {
			return true, fmt.Sprintf(
				"Input token budget exceeded: %dk / %dk used (%d tool calls so far).",
				totalInput/1000, e.Limits.MaxInputTokens/1000, result.ToolCalls,
			)
		}

		// Check API round-trips within this turn
		if e.Limits.MaxTurns > 0 && result.ToolCalls >= e.Limits.MaxTurns {
			return true, fmt.Sprintf(
				"Turn limit exceeded: %d tool calls made (limit: %d). Used %dk input tokens.",
				result.ToolCalls, e.Limits.MaxTurns, totalInput/1000,
			)
		}

		// Check response time limit (orchestrator speed enforcement)
		if e.Limits.MaxResponseTime > 0 && !e.Turn.StartTime.IsZero() {
			elapsed := time.Since(e.Turn.StartTime)
			limit := time.Duration(e.Limits.MaxResponseTime) * time.Second
			if elapsed > limit {
				return true, fmt.Sprintf(
					"Response time exceeded: %.0fs elapsed (limit: %ds). "+
						"You are an orchestrator — respond or spawn immediately. "+
						"Do NOT continue reading files or running commands. "+
						"Either give your answer now or spawn_agent for the work.",
					elapsed.Seconds(), e.Limits.MaxResponseTime,
				)
			}
		}

		return false, ""
	}
}

// buildToolRefusal renders the guard's verdict on a tool call as the reason to
// refuse the call, or "" to let it run. Nil means this session has no guard and
// therefore no gate.
//
// This is the only caller of guard.CheckTool. Before it existed, the modes were
// unreachable: the engine built every session Autonomous, nothing asked the
// guard about a tool, and a caller who asked for observe over the API got a
// session that could run `bash -c` with the full environment.
//
// NeedsApproval is refused rather than queued. Approval needs somewhere to ask,
// and there is nowhere yet — no session here sets ApprovalFunc and inber emits
// no approval event for llm-bridge to answer. Refusing says so to the model,
// which is a true answer; running the tool would be a false one.
func (e *Engine) buildToolRefusal() func(tool, input string) string {
	if e.Guard == nil {
		return nil
	}
	return func(tool, input string) string {
		switch e.Guard.CheckTool(tool, input) {
		case guard.Denied:
			return fmt.Sprintf("%s mode allows read-only tools only", e.Guard.Mode())
		case guard.NeedsApproval:
			return fmt.Sprintf("%s mode holds this tool for approval, and this session has no approver to ask", e.Guard.Mode())
		}
		return ""
	}
}

// buildHooks creates hooks that combine logging and display.
func (e *Engine) buildHooks() *agent.Hooks {
	hooks := &agent.Hooks{}

	// Display hooks are resolved dynamically via GetDisplayHooks() so the
	// gateway can swap them per-request without rebuilding the engine.

	if e.Session != nil {
		logHooks := e.Session.Hooks()

		hooks.OnRequest = func(params *anthropic.MessageNewParams) {
			if logHooks.OnRequest != nil {
				logHooks.OnRequest(params)
			}
			e.Session.WritePromptBreakdown(e.Turn.Counter, params, e.Cache.LastNamedBlocks)
		}
		hooks.OnThinking = func(text string) {
			if logHooks.OnThinking != nil {
				logHooks.OnThinking(text)
			}
			if d := e.GetDisplayHooks(); d != nil && d.OnThinking != nil {
				d.OnThinking(text)
			}
		}
		hooks.OnTextDelta = func(text string) {
			if d := e.GetDisplayHooks(); d != nil && d.OnTextDelta != nil {
				d.OnTextDelta(text)
			}
		}
		hooks.OnToolCall = func(toolID, name string, input []byte) {
			if e.toolInputsCache != nil {
				e.toolInputsCache[toolID] = string(input)
			}
			if logHooks.OnToolCall != nil {
				logHooks.OnToolCall(toolID, name, input)
			}
			if d := e.GetDisplayHooks(); d != nil && d.OnToolCall != nil {
				d.OnToolCall(name, string(input))
			}
		}
		hooks.OnToolResult = func(toolID, name, output string, isError bool) {
			if logHooks.OnToolResult != nil {
				logHooks.OnToolResult(toolID, name, output, isError)
			}
			if d := e.GetDisplayHooks(); d != nil && d.OnToolResult != nil {
				d.OnToolResult(name, output, isError)
			}
			if isError {
				e.Turn.ConsecutiveErrors++
				e.Turn.LastHadError = true
			}
		}
		hooks.ModifyToolResult = func(toolID, name, output string, isError bool) string {
			if e.Session != nil {
				return e.Session.TruncateToolResult(name, output, isError)
			}
			return ""
		}
		hooks.PostToolResult = func(toolID, name, output string, isError bool) string {
			if isError {
				return ""
			}
			toolInput := e.toolInputsCache[toolID]
			
			// Workflow hooks (auto-commit, auto-format, build/test)
			var result string
			if e.workflowHooks != nil {
				result = e.workflowHooks.OnToolResult(name, toolInput, output, isError)
			}
			
			// Forge hooks (project detection, preview tracking)
			if e.forgeHook != nil {
				action := e.forgeHook.Evaluate(name, toolInput, output, isError)
				if action.Kind != "none" {
					Log.Info("forge: %s — %s", action.Kind, action.Reason)
				}
			}
			
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
			if !e.Turn.LastHadError {
				e.Turn.ConsecutiveErrors = 0
			}
			e.Turn.LastHadError = false
			if logHooks.OnResponse != nil {
				logHooks.OnResponse(resp)
			}
		}
	} else {
		// No session logging, but still wire display hooks dynamically.
		hooks.OnThinking = func(text string) {
			if d := e.GetDisplayHooks(); d != nil && d.OnThinking != nil {
				d.OnThinking(text)
			}
		}
		hooks.OnTextDelta = func(text string) {
			if d := e.GetDisplayHooks(); d != nil && d.OnTextDelta != nil {
				d.OnTextDelta(text)
			}
		}
		hooks.OnToolCall = func(toolID, name string, input []byte) {
			if d := e.GetDisplayHooks(); d != nil && d.OnToolCall != nil {
				d.OnToolCall(name, string(input))
			}
		}
		hooks.OnToolResult = func(toolID, name, output string, isError bool) {
			if d := e.GetDisplayHooks(); d != nil && d.OnToolResult != nil {
				d.OnToolResult(name, output, isError)
			}
		}
	}

	return hooks
}