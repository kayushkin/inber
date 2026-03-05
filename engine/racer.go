package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	sessionMod "github.com/kayushkin/inber/session"
)

// RaceResult is the outcome of a model race.
type RaceResult struct {
	*agent.TurnResult
	ModelUsed string        // which model produced the result
	Priority  int           // 0 = best model, higher = fallback
	Latency   time.Duration // time from race start to this result
	Upgraded  bool          // true if a lower-priority response was replaced
}

// raceEntry tracks a single model's goroutine.
type raceEntry struct {
	model    string
	priority int
	result   *agent.TurnResult
	messages []anthropic.MessageParam
	err      error
}

type raceSignal struct {
	index int
}

// raceModels runs multiple models with staggered starts, returning the best result.
// Called internally by RunTurn when multiple models are in the active tier.
//
// Strategy:
//  1. Fire models[0] immediately
//  2. After Delay, fire models[1] (if no result yet)
//  3. Continue staggering through the list
//  4. When any model finishes, start grace window for a better one
//  5. Return the best available result
func (e *Engine) raceModels(systemBlocks []sessionMod.NamedBlock, models []string) (*RaceResult, error) {
	delay := 4 * time.Second
	grace := 8 * time.Second
	if e.tiers != nil {
		delay = e.tiers.Delay
		grace = e.tiers.Grace
	}

	start := time.Now()

	// Prepare entries — each gets its own message copy
	entries := make([]*raceEntry, len(models))
	for i, model := range models {
		msgCopy := make([]anthropic.MessageParam, len(e.Messages))
		copy(msgCopy, e.Messages)
		entries[i] = &raceEntry{
			model:    model,
			priority: i,
			messages: msgCopy,
		}
	}

	resultCh := make(chan raceSignal, len(entries))
	ctx, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()

	// Launch a single model run
	launchModel := func(idx int) {
		entry := entries[idx]

		mc, err := agent.NewModelClient(entry.model, e.modelStore)
		if err != nil {
			entry.err = fmt.Errorf("client for %s: %w", entry.model, err)
			resultCh <- raceSignal{idx}
			return
		}

		var result *agent.TurnResult
		if mc.IsOpenAI() {
			result, err = raceOpenAITurn(ctx, mc, systemBlocks, &entry.messages, e.agentTools)
		} else {
			result, err = raceAnthropicTurn(ctx, mc, entry.model, systemBlocks, &entry.messages, e.agentTools, e.thinkingBud)
		}

		entry.result = result
		entry.err = err
		resultCh <- raceSignal{idx}
	}

	// Staggered launch
	var mu sync.Mutex
	launched := 0

	launchNext := func() {
		mu.Lock()
		defer mu.Unlock()
		if launched < len(entries) {
			idx := launched
			launched++
			Log.Info("race: launching %s (priority %d)", entries[idx].model, idx)
			go launchModel(idx)
		}
	}

	// Fire first model
	launchNext()

	stagger := time.NewTicker(delay)
	defer stagger.Stop()

	bestIdx := -1
	bestPriority := len(entries)
	finished := 0

	for {
		select {
		case <-stagger.C:
			if bestIdx < 0 { // only launch more if no result yet
				launchNext()
			}

		case sig := <-resultCh:
			finished++
			entry := entries[sig.index]

			if entry.err != nil {
				Log.Warn("race: %s failed (%v): %v", entry.model, time.Since(start), entry.err)
			} else {
				Log.Info("race: %s responded in %v", entry.model, time.Since(start))

				if entry.priority < bestPriority {
					bestIdx = sig.index
					bestPriority = entry.priority
				}

				// Best possible model responded — done
				if bestPriority == 0 {
					cancelAll()
					return e.finishRace(entries, bestIdx, start, bestIdx != sig.index)
				}

				// We have a result but not from the best model.
				// Launch everything remaining and wait for grace window.
				mu.Lock()
				rem := len(entries) - launched
				mu.Unlock()
				for i := 0; i < rem; i++ {
					launchNext()
				}

				betterIdx := e.waitForBetter(resultCh, entries, bestPriority, grace, &finished, start)
				if betterIdx >= 0 {
					cancelAll()
					return e.finishRace(entries, betterIdx, start, true)
				}
				cancelAll()
				return e.finishRace(entries, bestIdx, start, false)
			}

			// All failed?
			if finished >= len(entries) {
				if bestIdx < 0 {
					return nil, fmt.Errorf("all %d models failed", len(entries))
				}
				return e.finishRace(entries, bestIdx, start, false)
			}
		}
	}
}

// waitForBetter waits up to graceWindow for a model with better priority.
func (e *Engine) waitForBetter(
	resultCh chan raceSignal,
	entries []*raceEntry,
	currentBest int,
	graceWindow time.Duration,
	finished *int,
	start time.Time,
) int {
	timer := time.NewTimer(graceWindow)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return -1

		case sig := <-resultCh:
			*finished++
			entry := entries[sig.index]

			if entry.err != nil {
				Log.Warn("race: %s failed (%v, grace): %v", entry.model, time.Since(start), entry.err)
			} else {
				Log.Info("race: %s responded in %v (grace)", entry.model, time.Since(start))
				if entry.priority < currentBest {
					return sig.index
				}
			}
			if *finished >= len(entries) {
				return -1
			}
		}
	}
}

// finishRace applies the winner's state to the engine.
func (e *Engine) finishRace(entries []*raceEntry, winnerIdx int, start time.Time, upgraded bool) (*RaceResult, error) {
	winner := entries[winnerIdx]

	// Adopt winner's messages (includes tool call history from the race)
	e.Messages = winner.messages

	return &RaceResult{
		TurnResult: winner.result,
		ModelUsed:  winner.model,
		Priority:   winner.priority,
		Latency:    time.Since(start),
		Upgraded:   upgraded,
	}, nil
}

// raceAnthropicTurn runs a standalone Anthropic turn (no engine hooks).
func raceAnthropicTurn(
	ctx context.Context,
	mc *agent.ModelClient,
	model string,
	systemBlocks []sessionMod.NamedBlock,
	messages *[]anthropic.MessageParam,
	tools []agent.Tool,
	thinkingBudget int64,
) (*agent.TurnResult, error) {
	blocks := make([]anthropic.TextBlockParam, len(systemBlocks))
	for i, b := range systemBlocks {
		blocks[i] = anthropic.TextBlockParam{Text: b.Text}
	}
	
	// Enable prompt caching: add cache_control to last system block
	if len(blocks) > 0 {
		blocks[len(blocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	// Filter out OpenAI-sourced tool_use/tool_result pairs to avoid ID confusion
	originalLen := len(*messages)
	*messages = agent.FilterMessagesForAnthropic(*messages)
	if stats := agent.LastFilterStats(); stats.ToolUseFiltered > 0 || stats.ToolResultFiltered > 0 {
		Log.Info("race: filtered %d tool_use, %d tool_result blocks from OpenAI provider (%d→%d messages)",
			stats.ToolUseFiltered, stats.ToolResultFiltered, originalLen, len(*messages))
	}

	a := agent.NewWithSystemBlocks(mc.AnthropicClient, blocks)
	for _, t := range tools {
		a.AddTool(t)
	}
	if thinkingBudget > 0 {
		a.SetThinking(thinkingBudget)
	}

	return a.Run(ctx, model, messages)
}

// raceOpenAITurn runs a standalone OpenAI-compatible turn (no engine hooks).
func raceOpenAITurn(
	ctx context.Context,
	mc *agent.ModelClient,
	systemBlocks []sessionMod.NamedBlock,
	messages *[]anthropic.MessageParam,
	tools []agent.Tool,
) (*agent.TurnResult, error) {
	oaiClient, err := mc.GetOpenAIClient()
	if err != nil {
		return nil, err
	}

	var systemParts []string
	for _, b := range systemBlocks {
		systemParts = append(systemParts, b.Text)
	}
	systemText := strings.Join(systemParts, "\n\n")

	toolMap := make(map[string]agent.Tool)
	for _, t := range tools {
		toolMap[t.Name] = t
	}
	openAITools := agent.ConvertAnthropicToolsToOpenAI(tools)

	result := &agent.TurnResult{}

	for {
		oaiMessages := agent.ConvertAnthropicMessagesToOpenAI(*messages)
		if systemText != "" {
			oaiMessages = append([]agent.OpenAIMessage{
				{Role: "system", Content: systemText},
			}, oaiMessages...)
		}

		req := agent.OpenAIRequest{
			Model:     oaiClient.Model,
			Messages:  oaiMessages,
			MaxTokens: 16384,
		}
		if len(openAITools) > 0 {
			req.Tools = openAITools
		}

		resp, err := oaiClient.ChatCompletion(ctx, req)
		if err != nil {
			return result, fmt.Errorf("OpenAI API call failed: %w", err)
		}

		anthropicResp := agent.ConvertOpenAIResponseToAnthropic(resp)
		result.InputTokens += int(anthropicResp.Usage.InputTokens)
		result.OutputTokens += int(anthropicResp.Usage.OutputTokens)

		*messages = append(*messages, anthropicResp.ToParam())

		if anthropicResp.StopReason == anthropic.StopReasonEndTurn ||
			anthropicResp.StopReason == anthropic.StopReasonMaxTokens {
			for _, block := range anthropicResp.Content {
				if block.Type == "text" {
					result.Text += block.Text
				}
			}
			return result, nil
		}

		if anthropicResp.StopReason == anthropic.StopReasonToolUse {
			var toolResults []anthropic.ContentBlockParamUnion

			for _, block := range anthropicResp.Content {
				if block.Type != "tool_use" {
					continue
				}
				result.ToolCalls++

				tool, ok := toolMap[block.Name]
				if !ok {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, fmt.Sprintf("error: unknown tool %q", block.Name), true))
					continue
				}

				output, toolErr := tool.Run(ctx, string(block.Input))
				if toolErr != nil {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, fmt.Sprintf("error: %s", toolErr), true))
					continue
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, output, false))
			}

			*messages = append(*messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		return result, fmt.Errorf("unexpected stop reason: %s", anthropicResp.StopReason)
	}
}
