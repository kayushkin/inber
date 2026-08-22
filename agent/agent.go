// Package agent implements the core agent loop: send messages to Claude,
// handle tool calls, collect results, repeat until a final text response.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ErrMaxAPICallsExceeded ends a turn that hit inber's own cap on API
// round-trips. It is a sentinel rather than a bare fmt.Errorf because callers
// have to tell it apart from a provider failure: reaching the cap means the
// provider answered every one of those calls, so the error is inber reporting
// on inber and says nothing about the model. engine.recordModelHealth reads it
// for exactly that reason.
var ErrMaxAPICallsExceeded = errors.New("exceeded max API calls")

// isContextLengthError checks if an API error is due to exceeding the model's context window.
//
// Every phrase here is token-shaped on purpose. A request can also be refused
// for its size in bytes, which says the same thing in words none of these match
// — see IsRequestByteSizeLimitError. The two are deliberately separate
// predicates: this one arms the prune-and-retry in Agent.callAPI, and that
// pruner is denominated in tokens, so folding the byte class in here would
// answer a byte overflow with a token head-drop that may not touch the base64
// part causing it. Whether the byte class should arm this pruner is an open
// question on todo 70ae784b.
func isContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "context_length_exceeded") ||
		strings.Contains(msg, "maximum context length") ||
		strings.Contains(msg, "too many tokens")
}

// requestByteSizeLimitMarkers are the phrases that name a byte-size refusal on
// their own, with no second word needed. "request_too_large" is Anthropic's own
// error type for the 32MB cap on its messages endpoint, which Cloudflare emits
// as a 413 before the request reaches the API; the other three are how a 413
// renders in prose across the gateways inber's OpenAI-compatible client talks to.
var requestByteSizeLimitMarkers = []string{
	"request_too_large",
	"request entity too large",
	"payload too large",
	"content too large",
}

// requestByteSizeNouns name the thing being measured, and
// requestByteSizeOverLimitPhrases say it was over. A message has to carry one of
// each to be classified, because every noun here appears in perfectly ordinary
// error text on its own.
var (
	requestByteSizeNouns = []string{
		"content length",
		"request size",
		"request length",
		"request body",
		"payload size",
		"body size",
	}
	requestByteSizeOverLimitPhrases = []string{
		"exceed",
		"too large",
	}
)

// IsRequestByteSizeLimitError reports whether a provider refused the request for
// its size in BYTES rather than for its length in tokens.
//
// The two are different failures with different remedies and inber only had a
// classifier for the token one, so a byte refusal fell through every branch to
// the default and was written to model-store as a provider fault. That store is
// host-shared, persistent, thresholdless and decay-free, so one oversized
// request marked a healthy model unhealthy for every session on the box, and
// selectModel then failed over — possibly to a model with a smaller window,
// which cannot help. engine.errorIsEvidenceAboutTheModel reads this predicate to
// stop that.
//
// Being over a byte cap is inber reporting on inber: the provider answered, and
// what it answered is that the request inber built was too big to accept. It is
// evidence about the request, not about the model.
//
// ⚠️ This predicate classifies. It does not recover — no pruning, no shedding,
// no retry is armed by it, and that omission is deliberate rather than pending.
// inber has no modality-aware shedding step and conversation.EstimateTokens
// prices tokens and never bytes, so inber cannot currently ask whether a pruned
// request is under a byte cap. What the recovery should be is todo 70ae784b's
// open question.
//
// The match is case-insensitive because a 413 reaches inber as prose written by
// whichever gateway refused it, not as a fixed API string.
func IsRequestByteSizeLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range requestByteSizeLimitMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	for _, noun := range requestByteSizeNouns {
		if !strings.Contains(msg, noun) {
			continue
		}
		for _, overLimit := range requestByteSizeOverLimitPhrases {
			if strings.Contains(msg, overLimit) {
				return true
			}
		}
	}
	return false
}

// Tool defines a tool the agent can use.
type Tool struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Run         func(ctx context.Context, input string) (string, error)
}

// Hooks allows callers to observe tool calls and results (e.g., for logging).
type Hooks struct {
	OnRequest    func(params *anthropic.MessageNewParams)                    // called before each API request
	OnResponse   func(resp *anthropic.Message)                               // called after each API response
	// OnMessageID reports the provider's own identifier for the assistant
	// message the turn is producing, as soon as the provider names it. On a
	// streamed response that is the message_start event, which arrives before
	// any text delta of that message, so an observer can key the deltas it is
	// about to receive on the message they belong to. On a response that is
	// not streamed it fires once the response is in hand, alongside
	// OnResponse. It may fire more than once with the same id.
	OnMessageID  func(messageID string)
	OnThinking   func(text string)                                           // called when thinking blocks are received
	OnTextDelta  func(text string)                                           // called for each text chunk during streaming
	OnToolCall   func(toolID, name string, input []byte)
	OnToolResult func(toolID, name, output string, isError bool)
	// ModifyToolResult is called before a tool result is added to the conversation.
	// If it returns a non-empty string, that string replaces the original output.
	// Used for truncation, filtering, or transformation of large results.
	ModifyToolResult func(toolID, name, output string, isError bool) string
	// PostToolResult is called after a tool result is collected. If it returns
	// a non-empty string, that string is appended as an extra user text block
	// in the same turn (useful for build/test feedback injection).
	PostToolResult func(toolID, name, output string, isError bool) string
}

// Agent runs the conversation loop with Claude.
type Agent struct {
	provider       Provider
	system         string
	systemBlocks   []anthropic.TextBlockParam
	tools          []Tool
	hooks              *Hooks
	sidebandCallbacks  *SidebandCallbacks
	readCache          *ReadCache
	agentName          string
	sessionID      string
	thinkingBudget int64 // 0 = disabled, >0 = budget tokens for extended thinking
	contextWindow  int   // max context tokens for the model (0 = no guard)
	isOAuth        bool  // true = Claude Max OAuth token; requires Claude Code identity in system prompt

	// BeforeRequest is called before each API call with a mutable reference to
	// the messages slice. Use it to prune/compact if the conversation is too large.
	// Return the (possibly pruned) messages. Called after OnRequest hook.
	//
	// It takes the turn's context because pruning is not free work done between
	// API calls: it can summarize, which is itself an API call. Handing the
	// callback a root context would leave a cancelled turn paying for one more
	// model round-trip before it stopped.
	BeforeRequest func(ctx context.Context, messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam

	// LimitCheck is called before each API call (after the first) to check
	// whether turn/token limits have been exceeded. If it returns (true, reason),
	// the agent will make one final tool-less API call asking the model to
	// summarize its progress, then return.
	LimitCheck func(result *TurnResult) (exceeded bool, reason string)

	// ToolRefusal is asked about every tool call before it runs, and returns
	// the reason to refuse it or "" to let it through. A refused call is not
	// executed at all: the refusal goes back to the model as an error
	// tool_result, so the model is told what happened and the turn continues.
	//
	// Every dispatch site consults it, including the chained call in a
	// tool_use block's "then" field. A gate on the primary call alone would be
	// no gate: the model can put any tool in the chain.
	//
	// Nil means no gate, which is what an agent built without one has always
	// had.
	ToolRefusal func(tool, input string) string

	// InjectCheck is called before each API call (after the first) to check
	// for mid-run messages from the user. Returns any pending messages to inject
	// into the conversation before the next API call.
	InjectCheck func() []string

	// VolatileContext is per-turn context (fleet status, recent files) that
	// gets prepended to the last user message instead of being in system blocks.
	// This prevents cache busting: system blocks stay stable → BP2 always hits.
	VolatileContext string

	// FrozenIdx is the message index where the frozen/staging boundary is.
	// A cache breakpoint is placed here instead of the second-to-last message.
	// Messages before FrozenIdx are frozen (cached), after are staging (uncached).
	// 0 means no frozen zone → fall back to default breakpoint placement.
	FrozenIdx int

	// turnAnchorIdx is the index of this turn's opening user message. Run sets
	// it; the second cache breakpoint goes there so that every API call of a
	// tool loop reads the conversation prefix instead of re-sending it.
	// Negative means unknown, which places no anchor breakpoint.
	turnAnchorIdx int
}

// New creates an agent with the given system prompt.
func New(provider Provider, system string) *Agent {
	return &Agent{
		provider:      provider,
		system:        system,
		readCache:     NewReadCache(),
		turnAnchorIdx: -1,
	}
}

// NewWithSystemBlocks creates an agent with pre-built system blocks.
func NewWithSystemBlocks(provider Provider, blocks []anthropic.TextBlockParam) *Agent {
	return &Agent{
		provider:      provider,
		systemBlocks:  blocks,
		readCache:     NewReadCache(),
		turnAnchorIdx: -1,
	}
}

// NewWithClient creates an agent with an Anthropic client (backward compatibility).
func NewWithClient(client *anthropic.Client, system string) *Agent {
	return New(NewAnthropicProvider(client), system)
}

// NewWithClientAndSystemBlocks creates an agent with client and system blocks (backward compatibility).
func NewWithClientAndSystemBlocks(client *anthropic.Client, blocks []anthropic.TextBlockParam) *Agent {
	return NewWithSystemBlocks(NewAnthropicProvider(client), blocks)
}

// SetHooks attaches observation hooks for tool calls/results.
func (a *Agent) SetHooks(h *Hooks) {
	a.hooks = h
}

// SetSidebandCallbacks sets the callbacks for sideband fields (done, note, split).
func (a *Agent) SetSidebandCallbacks(cb *SidebandCallbacks) {
	a.sidebandCallbacks = cb
}

// SetThinking enables extended thinking with the given token budget.
// Budget must be >= 1024. Set to 0 to disable.
func (a *Agent) SetThinking(budgetTokens int64) {
	a.thinkingBudget = budgetTokens
}

// SetContextWindow sets the model's context window size for overflow protection.
// SetRepoRoot tells the agent which root its tools resolve relative paths
// against — the same root tools.ScopeToRoot was given. The read cache needs it
// to identify a file by the file it is rather than by the spelling the model
// used, so a relative read and an absolute write meet on one entry.
func (a *Agent) SetRepoRoot(root string) {
	if a.readCache != nil {
		a.readCache.SetRoot(root)
	}
}

// RepoRoot returns the root this agent's tools resolve relative paths against.
// The read cache holds it, because the cache is the only part of the agent that
// has to reason about which file a path names; asking it is how callers read the
// value back without a second copy going stale.
func (a *Agent) RepoRoot() string {
	if a.readCache == nil {
		return ""
	}
	return a.readCache.Root()
}

func (a *Agent) SetContextWindow(tokens int) {
	a.contextWindow = tokens
}

// SetOAuth marks this agent as using a Claude Max OAuth token.
// When set, the system prompt will be prefixed with Claude Code identity
// (required for Claude 4 model access via Max subscription).
func (a *Agent) SetOAuth(isOAuth bool) {
	a.isOAuth = isOAuth
}

// SetBeforeRequest sets a callback invoked before each API call to allow
// pruning messages if they're approaching the context window limit. The
// callback receives the context of the API call it is guarding, so cancelling
// that call also stops the pruning done on its behalf.
func (a *Agent) SetBeforeRequest(fn func(ctx context.Context, messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam) {
	a.BeforeRequest = fn
}

// SetLimitCheck sets a callback that checks turn/token limits before each API call.
// When the callback returns (true, reason), the agent makes one final tool-less call
// asking the model to summarize progress, then returns.
func (a *Agent) SetLimitCheck(fn func(result *TurnResult) (bool, string)) {
	a.LimitCheck = fn
}

// SetToolRefusal sets the gate consulted before every tool call. See the
// ToolRefusal field.
func (a *Agent) SetToolRefusal(fn func(tool, input string) string) {
	a.ToolRefusal = fn
}

// AddTool registers a tool the agent can call.
func (a *Agent) AddTool(t Tool) {
	a.tools = append(a.tools, t)
}

// GetTools returns a copy of the agent's tools slice.
func (a *Agent) GetTools() []Tool {
	tools := make([]Tool, len(a.tools))
	copy(tools, a.tools)
	return tools
}

// TurnResult is what comes back from a single Run call.
type TurnResult struct {
	Text         string // Final text response
	Thinking     string // Thinking/reasoning text (if extended thinking enabled)
	ToolCalls    int    // Total tool calls made during this turn
	InputTokens  int
	OutputTokens int
	// Cache tokens (Anthropic prompt caching)
	CacheCreationTokens int // tokens written to cache (first request)
	CacheReadTokens     int // tokens read from cache (subsequent requests)
	// APICalls holds what each API call of this turn reported, in the order the
	// calls were made. The four counters above are the sum of these, and the sum
	// is what every cost consumer reads; this is the same numbers before they
	// were added up.
	//
	// A turn's calls do not cost the same. A call whose prompt prefix is
	// byte-identical to a cached one is billed at the read rate; a call that
	// diverges from it pays to write the whole prompt again, at 1.25x. Summing
	// hides that difference completely, so one full-price call inside a turn of
	// cheap ones is invisible in every total inber records.
	APICalls []APICallUsage
	// Incomplete is set when the turn ended on an error after the model had
	// already written text. Text then holds what the user watched arrive, not
	// a finished answer. Callers must still treat the turn as failed.
	Incomplete bool
}

// APICallUsage is what one API call reported, and the one thing about the
// request that decides which rate it was billed at.
//
// Recorded per call because the per-turn totals cannot answer "how often does
// a call miss the cache entirely, and what does that cost" — the question a
// force-summary call raises, since it is built without the tools block and so
// matches no cached prefix at all.
type APICallUsage struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	// ToolsWithheld records that this call went out with no tools block while
	// the agent had tools to send.
	//
	// Anthropic hashes the prompt prefix in the order tools -> system ->
	// messages, so dropping the tools array makes a request diverge from every
	// cached prefix at offset 0: the tools breakpoint is gone, and because the
	// divergence is before them, the system and message breakpoints are gone
	// too. The whole prompt is billed as cache creation with zero cache read.
	//
	// An agent that never sends tools is not this case. Its requests all share
	// the same (empty) prefix, so nothing diverges and nothing is re-bought,
	// which is why this reads "had tools and did not send them" rather than
	// "sent no tools".
	ToolsWithheld bool
}

// incompleteResponseNotice marks a stored assistant message as cut off, so a
// later turn can tell a short answer from an interrupted one. It is a separate
// content block: the text the user saw is never rewritten.
const incompleteResponseNotice = "[response cut off: %v]"

// Run sends a conversation to Claude and loops until it gets a final text
// response (no more tool calls). It mutates the messages slice in place,
// appending assistant and tool result messages.
// The model parameter specifies which Claude model to use for this run.
func (a *Agent) Run(ctx context.Context, model string, messages *[]anthropic.MessageParam) (*TurnResult, error) {
	result := &TurnResult{}

	// The last message is this turn's input. Everything the loop below adds —
	// assistant replies, tool results, mid-run injections — lands after it, so
	// it is the newest point in the conversation whose prefix stays byte-identical
	// for the whole turn, and therefore where the turn's cache entry belongs.
	a.turnAnchorIdx = len(*messages) - 1

	// Prepare tools and mapping
	tools := a.prepareTools()

	apiCalls := 0
	for {
		apiCalls++

		// Check context cancellation (e.g. spawn timeout).
		if ctx.Err() != nil {
			if result.Text == "" {
				result.Text = fmt.Sprintf("[Agent stopped: %s after %d API calls]", ctx.Err(), apiCalls-1)
			}
			return result, ctx.Err()
		}

		// Hard cap on API round-trips per turn to prevent runaway agents.
		const maxAPICalls = 50
		if apiCalls > maxAPICalls {
			if result.Text == "" {
				result.Text = fmt.Sprintf("[Agent stopped: exceeded %d API calls in one turn]", maxAPICalls)
			}
			return result, fmt.Errorf("%w (%d)", ErrMaxAPICallsExceeded, maxAPICalls)
		}

		// Check for mid-run injected messages from the user
		if apiCalls > 1 && a.InjectCheck != nil {
			if injected := a.InjectCheck(); len(injected) > 0 {
				// Append injected messages to the last user message (tool results)
				if len(*messages) > 0 {
					lastIdx := len(*messages) - 1
					for _, text := range injected {
						(*messages)[lastIdx].Content = append((*messages)[lastIdx].Content,
							anthropic.ContentBlockParamUnion{
								OfText: &anthropic.TextBlockParam{
									Text: "\n\n[New message from user while you were working]\n" + text,
								},
							},
						)
					}
				}
			}
		}

		// Check limits before each API call (after the first)
		forceSummary := false
		if apiCalls > 1 && a.LimitCheck != nil {
			if exceeded, reason := a.LimitCheck(result); exceeded {
				forceSummary = true
				// Inject limit notice into the last user message
				if len(*messages) > 0 {
					lastIdx := len(*messages) - 1
					(*messages)[lastIdx].Content = append((*messages)[lastIdx].Content,
						anthropic.ContentBlockParamUnion{
							OfText: &anthropic.TextBlockParam{
								Text: fmt.Sprintf("\n\n[BUDGET LIMIT REACHED] %s\n\n"+
									"You must stop making tool calls. Summarize your progress:\n"+
									"1. What you've accomplished so far\n"+
									"2. What remains to be done\n"+
									"3. Any issues or blockers encountered\n"+
									"Keep it concise.", reason),
							},
						},
					)
				}
			}
		}

		// Build request parameters
		params := a.buildRequest(ctx, model, messages, tools, forceSummary)

		// Execute API call with retry logic
		resp, err := a.executeAPICall(ctx, params, messages)
		if err != nil {
			// The call may have failed after the model wrote text the user has
			// already read. Keep it: append it to the conversation and put it on
			// the result, so every layer above can persist it before it handles
			// the error.
			if text := deliveredText(resp); text != "" {
				*messages = append(*messages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(text),
					anthropic.NewTextBlock(fmt.Sprintf(incompleteResponseNotice, err)),
				))
				result.Text += text
				result.Incomplete = true
			}
			return result, err
		}

		// Process response and update result stats
		a.processResponse(resp, result, toolsWereWithheld(params, tools))

		// Append assistant message
		*messages = append(*messages, resp.ToParam())

		// If stop reason is "end" or "max_tokens", extract text and return
		if resp.StopReason == anthropic.StopReasonEndTurn || resp.StopReason == anthropic.StopReasonMaxTokens {
			for _, block := range resp.Content {
				if block.Type == "text" {
					result.Text += block.Text
				}
			}
			return result, nil
		}

		// If stop reason is "tool_use", execute tools and continue
		if resp.StopReason == anthropic.StopReasonToolUse {
			toolResults, postInjections, err := a.executeTools(ctx, resp, tools, result)
			if err != nil {
				return result, err
			}

			if len(postInjections) > 0 {
				toolResults = append(toolResults, anthropic.NewTextBlock(
					"[system: post-write hook]\n"+strings.Join(postInjections, "\n"),
				))
			}
			*messages = append(*messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// Unexpected stop reason
		return result, fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
	}
}

// placeHistoryCacheBreakpoints marks the two points in the conversation worth
// caching, and clears every other cache_control the messages carry.
//
// frozenIdx is the frozen/staging boundary: nothing before it is ever mutated
// again, so an entry written there keeps paying out over many turns. It is
// placed on the last content block of message[frozenIdx-1].
//
// turnAnchorIdx is this turn's opening user message. It is the one that makes a
// tool loop cheap. Anthropic hashes tools, then system, then messages, and
// reads a cache entry whenever the request's prefix up to a breakpoint is
// byte-identical; a multi-tool-call turn only ever appends after its own user
// message, so every API call after the first reads that prefix instead of
// re-sending the whole staging zone at full price. Without it the breakpoint
// sits behind the staging zone and each round trip re-pays for everything the
// previous ones added.
//
// A negative index places no breakpoint. When the caller knows neither index
// the placement falls back to the second-to-last message, which is what this
// did before it had an anchor to aim at.
//
// Two of the four cache_control blocks a request may carry are spent here; the
// tools array and the system prefix hold the other two.
// HistoryCacheBreakpointIndices reports which messages a request built with
// these boundaries will carry a cache breakpoint on, in prefix order.
//
// It is the one place the placement rule lives. The cache blueprint reports
// against it and placeHistoryCacheBreakpoints places by it, so a report of
// where the breakpoints are and the request it describes cannot disagree — the
// blueprint used to guess "second-to-last message", which stopped being true
// the day a frozen zone could move it.
//
// The rule is intent: a message with no content block that can carry
// cache_control is skipped when the request is actually built.
func HistoryCacheBreakpointIndices(messageCount, frozenIdx, turnAnchorIdx int) []int {
	if messageCount < 2 {
		return nil
	}
	var targets []int
	if frozenIdx > 0 && frozenIdx <= messageCount {
		targets = append(targets, frozenIdx-1)
	}
	if turnAnchorIdx >= 0 {
		targets = append(targets, turnAnchorIdx)
	}
	if len(targets) == 0 {
		targets = append(targets, messageCount-2)
	}

	var indices []int
	previous := -1
	for _, idx := range targets {
		if idx < 0 || idx >= messageCount || idx <= previous {
			continue
		}
		indices = append(indices, idx)
		previous = idx
	}
	return indices
}

func placeHistoryCacheBreakpoints(messages []anthropic.MessageParam, frozenIdx, turnAnchorIdx int) {
	// Clear existing cache_control from all message content blocks.
	// System blocks and tools manage their own breakpoints separately.
	var zero anthropic.CacheControlEphemeralParam
	for i := range messages {
		for j := range messages[i].Content {
			b := &messages[i].Content[j]
			if b.OfText != nil {
				b.OfText.CacheControl = zero
			} else if b.OfToolUse != nil {
				b.OfToolUse.CacheControl = zero
			} else if b.OfToolResult != nil {
				b.OfToolResult.CacheControl = zero
			}
		}
	}
	for _, idx := range HistoryCacheBreakpointIndices(len(messages), frozenIdx, turnAnchorIdx) {
		markLastContentBlock(&messages[idx])
	}
}

// shiftBreakpointIndicesAfterHeadDrop moves the two indices this turn's cache
// breakpoints are placed by, after pruning shortened the conversation.
//
// Pruning is the only thing that shortens it, and it drops whole messages off
// the head while keeping the tail (engine/build.go), so subtracting the count
// is exact rather than an estimate. An index that falls off the front is gone:
// the anchor goes negative and stops being placed, and the frozen boundary
// collapses to zero, which is the "no frozen zone" value it started at.
func (a *Agent) shiftBreakpointIndicesAfterHeadDrop(dropped int) {
	a.turnAnchorIdx -= dropped
	a.FrozenIdx -= dropped
	if a.FrozenIdx < 0 {
		a.FrozenIdx = 0
	}
}

// markLastContentBlock puts a cache breakpoint on the message's final content
// block and reports whether it found one to mark.
func markLastContentBlock(msg *anthropic.MessageParam) bool {
	if len(msg.Content) == 0 {
		return false
	}
	last := &msg.Content[len(msg.Content)-1]
	cc := anthropic.NewCacheControlEphemeralParam()
	switch {
	case last.OfText != nil:
		last.OfText.CacheControl = cc
	case last.OfToolUse != nil:
		last.OfToolUse.CacheControl = cc
	case last.OfToolResult != nil:
		last.OfToolResult.CacheControl = cc
	default:
		return false
	}
	return true
}
