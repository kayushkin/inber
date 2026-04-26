# LLM Gateway (RLM Harness) Comparison

**Project**: [llm-gateway](https://github.com/hvent90/llm-gateway) — RLM harness ([docs](https://mintlify.wiki/hvent90/llm-gateway/api/harnesses/rlm), [source](https://github.com/hvent90/llm-gateway/tree/main/packages/ai/rlm))
**Language**: TypeScript (Node.js)
**Focus**: Composable "harness" abstraction where each harness yields a typed event stream and harnesses compose to form a graph; RLM (Recursive Language Model) is one harness designed for arbitrarily long inputs
**Key Strengths**: Generator-harness as a uniform composition primitive, RLM treats input as a REPL variable instead of context, sandboxed JS REPL with `FINAL()`/`exec()`/`llm_query()` primitives, HITL relays via async queue bridging

## Architecture Overview

llm-gateway is built around three ideas: **a harness yields events, harnesses compose, and the events form a graph.** Every component (provider call, agent loop, recursive processor, orchestrator) implements `GeneratorHarnessModule` and yields a typed async iterator of events. Composition is just wrapping one harness with another.

```
Provider harness  (zen / anthropic / openai / openrouter)
    ↑ wrapped by
Agent harness     (createAgentHarness — tool loop)
    ↑ wrapped by
RLM harness       (createRlmHarness — REPL loop over long input)
    ↑ orchestrated by
AgentOrchestrator (concurrent agents, HITL relays)
```

The **RLM harness** is the interesting one for inber. It implements `GeneratorHarnessModule` like any provider, so it composes with the agent harness and orchestrator transparently — but internally it runs a recursive REPL loop instead of a normal completion.

### How RLM Works

Standard pattern: cram the input into context, hope the model can handle it.
RLM pattern: input is a REPL variable named `context`; the model **never sees it**. Instead it sees metadata (length, prefix snippet) and writes JavaScript to explore.

```
1. Extract user prompt → create REPL with prompt as `context` variable
2. Build system prompt with metadata only (length, prefix) — NOT the full input
3. Loop:
   a. Stream LLM response (yields text/reasoning/usage)
   b. Extract code from fenced block (exactly one per turn)
   c. Execute in sandboxed REPL → yields repl_input/repl_progress/repl_output
   d. Append stdout/error to message history
4. If FINAL() called → yield final text, break
5. Else if maxIterations → break with reason="max_iterations"
```

REPL primitives available to the model:
- `context` — the input data (typically the long document)
- `FINAL(answer)` — completion signal
- `llm_query(...)` — sub-LLM calls (e.g. cheap model for simple sub-questions)
- `exec(cmd)` — shell commands, gated by HITL relay if `permissions` are set

## What llm-gateway / RLM Does Well

### 1. Generator Harness as a Composition Primitive ⭐️

Every component is a `GeneratorHarnessModule` that yields events. Provider, agent, RLM, orchestrator — same interface. This means the agent harness can wrap an RLM harness can wrap a provider harness, and the orchestrator doesn't know or care which.

```typescript
const rlm = createRlmHarness({
  rootHarness: createGeneratorHarness(),  // any provider
  config: { maxIterations: 50 },
});
// rlm is itself a GeneratorHarnessModule and composes anywhere a provider would.
```

**Inber connection**: Inber treats provider, agent loop, and tool execution as separate concerns with bespoke wiring. A uniform "yields events, can be composed" interface would let inber stack capabilities (e.g. "add caching", "add HITL", "add recursive expansion") without each being a one-off integration.

### 2. RLM: Input as REPL Variable, Not Context ⭐️

The core insight: most "long context" problems are really "the model needs to find a few relevant pieces of a large input." Instead of paying tokens to send the whole input, send metadata and let the model write code to extract what it needs.

This generalizes beyond long inputs — any time the model needs to interact with a large structured artifact (a codebase, a database, a log file), the RLM pattern fits.

**Inber connection**: Inber currently does smart-truncation and conversation pruning to fit context. These are lossy compressions. RLM is a different answer: don't compress, give the model tools to query what it needs. Could be a powerful complement for inber's memory system — instead of summarizing old conversations, expose them as a queryable REPL.

### 3. Typed Event Stream as the Universal Output

Every harness yields the same event types (`text`, `reasoning`, `tool_call`, `tool_result`, `usage`, `harness_start`, `harness_end`, `relay`, plus harness-specific ones like `repl_input`/`repl_progress`/`repl_output`). The orchestrator and clients consume this stream uniformly.

This is similar in spirit to `llm-bridge`'s canonical message types, but more granular at the *streaming* level — it's not just "what's the conversation," it's "what's happening right now."

### 4. HITL Permission Relays via AsyncQueue

When `permissions` are passed to `invoke()`, `exec()` calls are gated through a relay event. The exec callback pushes a relay onto an async queue; the harness yields the relay event from the generator; the orchestrator (or UI) calls `resolveRelay(id, decision)`, which unblocks the exec callback inside the REPL.

This is a clean async pattern for "pause execution, ask a human, resume" without blocking the generator or polluting the event stream with custom protocol.

**Inber connection**: Inber's tool execution doesn't have a generic HITL gate. For sensitive operations (especially shell), an inline relay pattern would be valuable — pause the agent, surface the request to a UI/channel, resume on decision.

### 5. Sub-Harness for Cheap Calls

The RLM REPL has `llm_query(...)` that calls a separate (typically cheaper) harness for sub-questions. The "expensive model writes plans, cheap model answers sub-questions" pattern is built in.

**Inber connection**: Inber doesn't formalize the "expensive planner + cheap workers" pattern. Could be a useful primitive for cost-sensitive workflows.

## What Inber Should Adopt

### 1. Recursive / REPL Harness for Long Inputs (HIGH PRIORITY for memory)

Build an inber harness that exposes long memory or large documents as a REPL variable instead of stuffing them into context. Model writes Go-equivalent expressions or shell-like commands to query (e.g. `grep_memory("auth")`, `summarize_session(id)`). Iterates until it has what it needs, then generates the final answer.

This is a different answer to the "context is too large" problem than smart-truncation. Worth piloting on the memory subsystem specifically: instead of injecting top-K memories into context, expose memories as queryable.

### 2. Uniform Composable Harness Interface (MEDIUM PRIORITY)

Define a Go interface like:

```go
type Harness interface {
    Invoke(ctx context.Context, req Request) (<-chan Event, error)
}
```

Every existing component (Anthropic SDK call, agent loop, tool dispatch, future RLM-style harness) implements it. Wrap freely: `caching(rlm(agent(provider)))`. Inber's engine becomes one specific composition of harnesses, not the only way to wire things up.

This is similar to llm-bridge's bridge interface but at the *streaming* level (events, not whole responses) and inside the engine, not just for external bridges.

### 3. Inline HITL Relay Pattern for Sensitive Tools (MEDIUM PRIORITY)

For tools like shell, file write, or anything destructive, add a relay primitive:

- Tool's `Execute` can yield a `relay` event with the proposed action
- Agent pauses; relay surfaces to UI / si / bus
- Decision (`approve` / `deny` / `always-allow`) resolves the relay
- Tool execution proceeds or aborts

This complements inber's existing permission system (which is allowlist-based) with a per-call approval path.

### 4. `llm_query()`-Style Sub-Harness (LOW PRIORITY)

Make it easy for an agent to delegate a sub-question to a cheaper model without spawning a full sub-agent. Useful for "summarize this 5KB blob" or "is this string a path?" kinds of micro-questions where a full agent invocation is overkill.

## What's Different

| Aspect | llm-gateway / RLM | Inber |
|--------|-------------------|-------|
| **Language** | TypeScript | Go |
| **Composition primitive** | `GeneratorHarnessModule` (yields events) | Bespoke wiring per concern |
| **Long-input strategy** | RLM: input as REPL variable, model writes code | Smart truncation + conversation pruning |
| **Tool gating** | HITL relays via AsyncQueue | Allowlist-based permissions |
| **Sub-LLM calls** | First-class `llm_query()` REPL primitive | Spawn full sub-agent |
| **Event stream** | Typed events at every layer (text, repl_input, relay, etc.) | Internal logging + final response |
| **Sandboxing** | JS REPL with persistent scope, FINAL() signal | Tool-level execution, no REPL |
| **Provider model** | Per-provider harness (zen, anthropic, openai, openrouter) | Anthropic SDK + llm-bridge for others |
| **Orchestrator** | `AgentOrchestrator` with concurrent spawn + relays | Server package + bus |
| **Use case** | Library to build LLM-app infra | Full multi-agent runtime |

## Key Takeaway

llm-gateway's central bet is that **harnesses are composable and yield events**, which makes capabilities stackable rather than each requiring custom wiring. The **RLM harness** is the standout idea: instead of fighting context limits with compression, treat the input as a REPL variable and let the model write code to query it.

For inber, two actionable insights:

1. **Pilot an RLM-style harness over the memory system.** Inber's smart-truncation and pruning are answers to "context is finite." RLM is a different answer: don't compress, expose. Let the agent query memory through a tool/REPL instead of pre-injecting top-K results. This could meaningfully reduce token usage on memory-heavy workflows.
2. **Adopt a uniform streaming-harness interface internally.** The current engine wires concerns together case-by-case; a `Harness` interface that yields events would make features like caching, HITL relays, RLM-style recursion, or sub-LLM delegation composable rather than bespoke integrations.

The HITL relay pattern is also worth borrowing for sensitive tool execution, complementing inber's allowlist permissions with per-call approval.
