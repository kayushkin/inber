# Token Efficiency Comparison

**Last reviewed**: 2026-10-29 against earendil-works/pi@e7a0df2 (2026-10-29) and openclaw/openclaw@2f47962 (2026-10-29), and inber's own code on that date.

> **Read this first.** The March 2026 sections below are an analysis, not a measurement. Every token count and success rate in their tables is an estimate nobody ran, and several of the claims behind them were wrong then or are wrong now (listed in the next section). The only measured numbers in this doc are in *Measured, 2026-10-28*.

## Review notes, 2026-10-29

What the March text gets wrong about the three projects today:

- **Pi and OpenClaw both summarize.** Pi (now `earendil-works/pi`) has automatic compaction when context passes a threshold, a `/compact` command, and branch summaries (`packages/coding-agent/docs/compaction.md`). OpenClaw compacts older turns into a summary kept in the transcript (`docs/concepts/compaction.md`) and also prunes sessions. "Simple truncation, no summarization" for pi, and "sliding window" for OpenClaw, no longer describe them.
- **OpenClaw retired its heartbeat**, which the March text credits with saving tokens; scheduled checks are now plain cron jobs.
- **Inber does not use provider-specific tokenizers.** Its estimate is about three characters per token over the JSON of a message (`estimateMarshalled` in `conversation/manage_text_utils.go`, calling memory-store's `EstimateTokens`). Real counts come from the provider's usage report after each call.
- **The inber code example was never inber's code.** There is no `ConversationManager`, `shouldPrune` or `pruneConversation`. Summarizing starts when a conversation passes a message count (`ShouldSummarize` in `conversation/summarize.go`), and tool-result pruning is in `conversation/manage_tool_pruning.go`. The pi and OpenClaw snippets are likewise illustrations, not their code.
- **Inber now runs OpenAI models too**, through a separate loop (`engine/turn_openai.go`) that can use the ChatGPT subscription (`agent/codex_subscription.go`), and sends the session id as `prompt_cache_key` so later calls in a session hit OpenAI's cache.
- **Inber no longer has a CLI in this repo** (it moved to `inber-cli`), so "add token usage metrics to CLI" now means the server's API; per-turn tokens and cost are already stored (`session/db_turns.go`).

## Measured, 2026-10-28

One measurement exists, run on 2026-10-28: inber, Codex and Claude Code on three small tasks against a small Go fixture (read a bug and explain it, fix it, add a feature), two runs per cell. All fix and feature runs passed the fixture's tests. Token counts come from each tool's own usage record.

- **Fixed cost of a one-word reply**, tools included: inber about 9.4k input tokens, Codex about 11.7k, Claude Code about 20.6k (clean directory, no extra prompt).
- **Bare, same model (gpt-5.5 on an API key)**: inber sent 15–28k input tokens per task, Codex 37–106k, Claude Code (on an Anthropic model) 64–87k. Inber was cheapest on all three tasks.
- **With the full host prompt each tool gets in real use** (about 49k characters), inber and Codex on the same ChatGPT subscription model: inber sent 43k / 88k / 100k input tokens on read / fix / feature, Codex 81k / 179k / 210k — roughly half. Uncached input was close (inber 10k / 20k / 13k, Codex 17k / 20k / 21k) because Codex's prompt is better cached.
- **Caching**: inber's first call in a session always misses the cache, because the subscription endpoint keys the cache by session id and Codex warms it before its first call. Sending the session id as both `prompt_cache_key` and a `session-id` header (`agent/codex_subscription.go`) raised inber's cached share on later calls to 56–78%, against 85–93% for Codex.
- **Correctness**: fix and feature were correct on every run for all three. On the read task with the host prompt, inber answered one of two runs wrong (it read a comment, not the code).

Small sample, one fixture, two runs per cell: this shows inber sends fewer tokens on small tasks, not that it does so on long sessions, which is what the March tables guess at.

---

*The rest of this doc is the March 2026 analysis, kept as history.*

**Focus**: Comparing token usage patterns, context management, and conversation efficiency across inber, pi-mono, and OpenClaw  
**Date**: March 2026  
**Methodology**: Analysis of context management strategies, memory systems, and token optimization approaches  

## Framework Overview

| Framework | Language | Context Strategy | Memory Approach | Token Optimization |
|-----------|----------|------------------|-----------------|-------------------|
| **Inber** | Go | Summarization + Pruning | SQLite + Embeddings | Aggressive compaction |
| **Pi-mono** | TypeScript | Simple truncation | In-memory + optional persistence | Provider-dependent |  
| **OpenClaw** | Node.js | Sliding window + context injection | Session transcripts | Dynamic context sizing |

## Context Management Strategies

### Inber
- **Summarization-first**: Uses Claude to create dense summaries of old conversations
- **Embedding-based retrieval**: Searches past conversations using cosine similarity 
- **Adaptive pruning**: Removes messages when context exceeds limits
- **Memory compaction**: Periodically compresses conversation history
- **Token counting**: Precise token estimation with provider-specific tokenizers *(not true: a chars/3 estimate; see review notes)*

**Code Example:**
```go
// Illustration only: inber has no such functions (see review notes)
func (cm *ConversationManager) shouldPrune(messages []anthropic.Message) bool {
    totalTokens := cm.estimateTokens(messages)
    return totalTokens > cm.config.MaxContextTokens
}

func (cm *ConversationManager) pruneConversation(messages []anthropic.Message) ([]anthropic.Message, error) {
    // Intelligent pruning keeping system messages, recent context, and summaries
    summary, err := cm.summarizer.SummarizeMessages(oldMessages)
    return append(systemMessages, summary, recentMessages...)
}
```

### Pi-mono  
*(Outdated: pi now compacts with summaries; see review notes.)*
- **Simple truncation**: Drops old messages when context limit reached
- **Provider-agnostic**: Relies on provider's context limits without optimization
- **Tool memory**: Keeps tool definitions but minimal conversation history
- **No summarization**: Generally starts fresh conversations frequently

**Code Example:**
```typescript
// Typical pi-mono approach
function truncateMessages(messages: Message[], maxTokens: number): Message[] {
    let tokenCount = 0;
    const result = [];
    for (let i = messages.length - 1; i >= 0; i--) {
        const msgTokens = estimateTokens(messages[i]);
        if (tokenCount + msgTokens > maxTokens) break;
        result.unshift(messages[i]);
        tokenCount += msgTokens;
    }
    return result;
}
```

### OpenClaw
*(Outdated: OpenClaw now compacts with summaries and has retired heartbeat; see review notes.)*
- **Sliding window**: Maintains recent messages + injected context
- **Session transcripts**: Persists full conversations but selectively injects
- **Heartbeat system**: Uses periodic check-ins to maintain state without full context
- **Dynamic sizing**: Adjusts context window based on session type and urgency

**Code Example:**
```javascript
// OpenClaw's context injection approach  
function buildContext(sessionKey, recentLimit = 20) {
    const recent = getRecentMessages(sessionKey, recentLimit);
    const injected = getRelevantContext(sessionKey, recent);
    return [...systemPrompts, ...injected, ...recent];
}
```

## Token Efficiency Analysis

### Multi-turn Coding Session (Estimated)

*These figures were never measured. For measured numbers on small tasks see* Measured, 2026-10-28 *above.*

**Scenario**: 50-turn conversation implementing a new feature with file edits, testing, and iteration.

| Framework | Total Tokens | Context Tokens | Response Tokens | Efficiency Score |
|-----------|--------------|----------------|-----------------|------------------|
| **Inber** | ~85,000 | ~45,000 | ~40,000 | **High** |
| **Pi-mono** | ~120,000 | ~70,000 | ~50,000 | Medium |
| **OpenClaw** | ~95,000 | ~50,000 | ~45,000 | Medium-High |

**Inber's advantages:**
- Summarization reduces redundant context by ~40%
- Intelligent pruning preserves critical information 
- Embedding search surfaces relevant past context without full history
- Compaction prevents conversation drift

**Pi-mono's challenges:**
- Simple truncation loses important context
- Frequently starts new conversations to avoid context limits
- No cross-session memory leads to repeated explanations
- Provider-dependent optimization means inconsistent efficiency

**OpenClaw's approach:**
- Selective context injection balances efficiency vs. continuity
- Session management reduces redundant initialization
- Heartbeat system maintains state with minimal tokens
- Better than pi-mono but less optimized than inber

### Memory Lookup Efficiency

**Scenario**: "What was that bug we fixed last week in the auth module?"

| Framework | Approach | Token Cost | Success Rate |
|-----------|----------|------------|--------------|
| **Inber** | Embedding search → targeted retrieval | ~500 tokens | **95%** |
| **Pi-mono** | Manual search or new conversation | ~0-50,000 tokens | 20% |
| **OpenClaw** | Session transcript search | ~2,000 tokens | 70% |

### Context Management Overhead

**Per-turn overhead for maintaining conversation context:**

| Framework | Setup Tokens | Memory Retrieval | Context Building | Total Overhead |
|-----------|--------------|-----------------|------------------|----------------|
| **Inber** | ~200 | ~100-500 | ~300 | ~600-1000 |
| **Pi-mono** | ~100 | ~0 | ~50 | ~150 |
| **OpenClaw** | ~300 | ~200 | ~400 | ~900 |

**Key insight**: Inber's overhead pays for itself after 3-4 turns by avoiding context repetition.

## What Inber Does Best

### 1. **Intelligent Summarization**
- Uses Claude's summarization capabilities to compress conversation history
- Preserves decision context while removing implementation details
- Maintains thread coherence across long sessions

### 2. **Embedding-Based Memory**  
- Vector search finds relevant past conversations without full context replay
- Enables "remember when we discussed X?" queries efficiently
- Scales better than transcript search for large conversation histories

### 3. **Adaptive Context Management**
- Dynamically adjusts context window based on conversation complexity
- Preserves system prompts and recent critical messages
- Intelligent pruning based on message importance, not just age

### 4. **Token Accounting**
- Token estimation before a call (a chars/3 estimate, not a tokenizer), and the provider's reported usage after it
- Proactive context management before hitting limits
- Cost tracking and optimization opportunities

## What Inber Could Improve

### 1. **Configurable Context Strategies**
Learn from OpenClaw's flexibility:
```go
type ContextStrategy interface {
    BuildContext(session *Session, maxTokens int) []Message
}

type AdaptiveStrategy struct{}  // Current inber approach
type SlidingWindowStrategy struct{}  // OpenClaw approach  
type TruncationStrategy struct{}  // Pi-mono approach
```

### 2. **Cross-Session Context Sharing**
Enable efficient context sharing between related sessions without duplication:
```go
type ContextPool struct {
    sharedPrompts map[string][]Message  // Reusable system prompts
    projectContext map[string]*Embedding  // Project-specific context
}
```

### 3. **Provider-Specific Optimization**
Different providers have different strengths:
- **Anthropic**: Excellent summarization, use for compression
- **OpenAI**: Fast inference, use for quick context decisions  
- **Local models**: Cheap tokens, use for memory encoding

### 4. **Token Budget Management**
```go
type TokenBudget struct {
    MaxPerTurn   int
    ContextRatio float64  // What % should be context vs new generation
    MemoryRatio  float64  // What % for memory vs current conversation
}
```

## Recommendations for Inber

### Immediate (High ROI)
1. **Add token usage metrics to CLI** — Show per-turn and session costs
2. **Implement context strategy selection** — Let users choose truncation vs summarization
3. **Optimize summarization triggers** — Only summarize when content is repetitive

### Medium-term  
1. **Cross-session memory sharing** — Reuse context between related conversations
2. **Provider-specific strategies** — Use each model's strengths for token efficiency
3. **User-configurable token budgets** — Let users optimize for cost vs capability

### Long-term
1. **Advanced memory compression** — Use specialized models for context encoding
2. **Predictive context loading** — Pre-load likely relevant context
3. **Collaborative context sharing** — Share efficient context patterns across users

## Bottom Line

**Inber leads in token efficiency** due to its sophisticated memory and summarization systems. A typical long conversation uses 30-40% fewer tokens than pi-mono and 10-15% fewer than OpenClaw. *(2026-10-29: these percentages were never measured. The 2026-10-28 measurement compared inber with Codex and Claude Code, not with pi or OpenClaw, on small tasks only.)*

**Key advantages:**
- Intelligent summarization prevents context bloat
- Embedding search enables efficient memory retrieval  
- Adaptive pruning preserves critical information
- Precise token accounting enables optimization

**Growth opportunity:** Make token efficiency configurable. Some users want maximum context (OpenClaw's approach), others want minimum cost (pi-mono's approach), and power users want inber's current optimization. Supporting all three strategies would make inber more flexible while maintaining its efficiency leadership.

The token efficiency comparison shows inber's **memory-first architecture** pays significant dividends in multi-turn conversations, making it ideal for extended coding sessions, complex problem-solving, and knowledge work where context accumulation provides value.