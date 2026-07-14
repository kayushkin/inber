# Agentic Design Patterns Research Notes

*Source: [AgenticDesignPatterns Repository](https://github.com/DanieleSalatti/AgenticDesignPatterns) by Antonio Gulli*

## Overview

This repository provides practical implementations of 21 key agentic design patterns for building intelligent systems. The patterns cover core architectural concepts like orchestration, memory, communication, and safety that are highly relevant to inber's multi-agent design.

## Key Design Patterns

### 1. **Prompt Chaining** (Chapter 1)
- **Concept**: Breaking complex tasks into sequential steps with each step's output feeding the next
- **Implementation**: Simple sequential execution with output passing
- **inber Relevance**: Similar to our context passing between agents

### 2. **Routing** (Chapter 2)
- **Concept**: Intelligent delegation based on request analysis
- **Implementation**: Coordinator agent analyzes requests and routes to specialized handlers
- **Key Architecture**: 
  ```python
  coordinator_router_chain = prompt | llm | output_parser
  delegation_branch = RunnableBranch(conditions, handlers)
  ```
- **inber Applications**: Could enhance our agent spawning logic with smarter routing

### 3. **Parallelization** (Chapter 3)
- **Concept**: Concurrent execution of independent sub-tasks
- **Implementation**: `ParallelAgent` runs multiple researchers simultaneously, followed by synthesis
- **Key Pattern**:
  ```python
  # Multiple agents with output_key store results in session state
  parallel_research = ParallelAgent(sub_agents=[...])
  synthesis = SequentialAgent([parallel_research, merger_agent])
  ```
- **inber Applications**: Perfect for our multi-agent spawning and coordination

### 4. **Reflection** (Chapter 4)
- **Concept**: Iterative self-improvement through critique and refinement
- **Implementation**: Generate → Reflect → Refine loop with stopping conditions
- **Key Features**:
  - Separate generate/refine and reflect stages
  - Building conversation history for context
  - Clear stopping conditions ("CODE_IS_PERFECT")
- **inber Applications**: Could enhance agent reasoning quality

### 5. **Tool Use** (Chapter 5)
- **Concept**: Structured function calling with proper error handling
- **Implementation**: Multiple frameworks (LangChain, CrewAI, ADK) with function schemas
- **inber Relevance**: Already well-covered in our tool system

### 6. **Planning** (Chapter 6)
- **Concept**: Explicit planning phase before execution
- **Implementation**: Two-stage process - create plan, then execute plan
- **Key Pattern**: Tasks explicitly request both planning and execution phases
- **inber Applications**: Could improve complex task decomposition

### 7. **Multi-Agent Collaboration** (Chapter 7)
- **Concept**: Hierarchical and parallel agent coordination
- **Patterns**:
  - **Coordinator**: Parent agent with sub-agents for delegation
  - **Sequential**: Ordered execution pipeline
  - **Parallel**: Concurrent execution
  - **Loop**: Iterative multi-agent processes
- **Key Architecture**:
  ```python
  coordinator = LlmAgent(sub_agents=[...])
  # Parent-child relationships automatically established
  assert greeter.parent_agent == coordinator
  ```
- **inber Applications**: Directly applicable to our agent orchestration

### 8. **Memory Management** (Chapter 8)
- **Concept**: Persistent and session-based state management
- **Implementations**:
  - `InMemoryMemoryService`: Development/testing
  - `VertexAiRagMemoryService`: Production with RAG-based search
  - Session state with explicit `output_key` storage
- **Key Patterns**:
  ```python
  # Agents store results in session state
  agent = LlmAgent(output_key="specific_result")
  # Other agents access via state["specific_result"]
  ```
- **inber Applications**: Could enhance our context system and memory persistence

### 9. **Adaptation** (Chapter 9)
- **Concept**: Self-modifying agents that evolve behavior
- **Implementation**: OpenEvolve-style self-improvement
- **inber Relevance**: Longer-term capability for agent evolution

### 10. **Model Context Protocol (MCP)** (Chapter 10)
- **Concept**: Standardized protocol for agent-tool communication
- **Implementation**: FastMCP server with ADK client agents
- **Key Features**:
  - Standardized tool interfaces
  - Server-client architecture for tools
  - Cross-platform compatibility
- **inber Applications**: Could standardize our tool interfaces

### 11. **Goal Setting and Monitoring** (Chapter 11)
- **Concept**: Explicit goal definition with iterative achievement tracking
- **Implementation**: 
  - Goals defined upfront as concrete criteria
  - LLM-based evaluation of goal achievement
  - Iterative refinement until goals met
- **Key Pattern**:
  ```python
  for iteration in range(max_iterations):
      result = generate_solution(goals)
      feedback = evaluate_against_goals(result, goals)
      if goals_achieved(feedback): break
  ```
- **inber Applications**: Could improve task completion reliability

### 12. **Exception Handling and Recovery** (Chapter 12)
- **Concept**: Graceful degradation with fallback mechanisms
- **Implementation**: Sequential agents with state-based fallback logic
- **Key Pattern**:
  ```python
  primary_handler = Agent(tools=[primary_tool])
  fallback_handler = Agent(
      instruction="Check state['primary_failed'] and use fallback"
  )
  robust_agent = SequentialAgent([primary, fallback, response])
  ```
- **inber Applications**: Critical for production reliability

### 13. **Human-in-the-Loop** (Chapter 13)
- **Concept**: Escalation to human operators for complex decisions
- **Implementation**: Customer support agents with escalation triggers
- **inber Relevance**: Important for our human oversight patterns

### 14. **Knowledge Retrieval (RAG)** (Chapter 14)
- **Concept**: Augmenting agent knowledge with external information
- **Implementations**: Multiple RAG approaches (LangChain, VertexAI, Google Search)
- **inber Applications**: Could enhance our web_search and knowledge tools

### 15. **Inter-Agent Communication (A2A)** (Chapter 15)
- **Concept**: Agents exposing themselves as services for other agents
- **Implementation**: REST APIs with AgentCard specifications
- **Key Features**:
  - Standardized agent discovery via AgentCard
  - HTTP-based communication
  - Skill-based capability advertisement
- **inber Applications**: Could enable agent ecosystem growth

### 16. **Resource-Aware Optimization** (Chapter 16)
- **Concept**: Intelligent resource management and cost optimization
- **Implementation**: Model routing based on complexity and cost constraints
- **inber Applications**: Important for production deployment

### 17. **Reasoning Techniques** (Chapter 17)
- **Patterns**:
  - **Chain of Thought (CoT)**: Explicit step-by-step reasoning
  - **Self-Correction**: Iterative reasoning refinement  
  - **Code Execution**: Using programming for complex calculations
- **inber Applications**: Could enhance agent reasoning capabilities

### 18. **Guardrails/Safety Patterns** (Chapter 18)
- **Concept**: Safety mechanisms and content filtering
- **Implementations**:
  - LLM-based content validation
  - Tool validation layers
  - Multi-stage safety checks
- **inber Applications**: Critical for production safety

### 19. **Evaluation and Monitoring** (Chapter 19)
- **Concept**: Automated agent performance assessment
- **Patterns**:
  - LLM-as-a-Judge for response quality
  - Correctness and relevance metrics
  - Multi-dimensional evaluation frameworks
- **inber Applications**: Essential for agent quality assurance

### 20. **Prioritization** (Chapter 20)
- **Concept**: Task scheduling and priority management
- **Implementation**: SuperSimplePM-style project management
- **inber Applications**: Could improve our task orchestration

### 21. **Exploration and Discovery** (Chapter 21)
- **Concept**: Agent laboratories for experimentation
- **Implementation**: Sandbox environments for agent testing
- **inber Applications**: Could enhance our development workflow

## Architectural Insights for inber

### 1. **Context System Enhancement**
- **State-based Communication**: Agents store results in session state with explicit keys
- **Cross-Agent Data Flow**: Standardized state access patterns
- **Memory Persistence**: RAG-based memory for long-term context

### 2. **Agent Orchestration Patterns**
- **Hierarchical Coordination**: Parent-child agent relationships
- **Parallel Execution**: True concurrency with result synthesis  
- **Sequential Pipelines**: Ordered execution with state passing
- **Routing Logic**: Intelligent delegation based on request analysis

### 3. **Tool Use Architecture**
- **MCP Protocol**: Standardized tool interfaces
- **Validation Layers**: Multi-stage safety and correctness checks
- **Fallback Mechanisms**: Graceful degradation when tools fail

### 4. **Memory Management**
- **Session State**: Explicit result storage with keys
- **RAG Integration**: Vector-based knowledge retrieval
- **Cross-Session Persistence**: Long-term memory capabilities

## Concrete Ideas Worth Exploring

### High Priority for inber

1. **Enhanced Agent Spawning with Routing Logic**
   ```python
   # Current: Simple spawning
   # Enhanced: Intelligent routing like Chapter 2
   coordinator = Agent(
       instruction="Analyze request type and route to specialists",
       sub_agents=[researcher, coder, planner]
   )
   ```

2. **Parallel Agent Execution**
   ```python
   # Implement Chapter 3's parallel pattern
   parallel_tasks = ParallelAgent(
       researchers=[web_researcher, doc_researcher, code_researcher]
   )
   synthesis_agent = Agent(
       instruction="Combine results from {web_result}, {doc_result}, {code_result}"
   )
   ```

3. **State-based Agent Communication**
   ```python
   # Chapter 8's session state pattern
   agent_a = Agent(output_key="research_findings")
   agent_b = Agent(
       instruction="Process {research_findings} and create summary",
       input_dependencies=["research_findings"]
   )
   ```

4. **Robust Exception Handling**
   ```python
   # Chapter 12's fallback pattern
   primary_agent = Agent(tools=[advanced_tool])
   fallback_agent = Agent(
       instruction="If {primary_failed}, use basic_tool",
       condition="state.get('primary_failed')"
   )
   ```

### Medium Priority

5. **Goal-Driven Task Execution** (Chapter 11)
   - Explicit goal definition for spawned agents
   - Iterative refinement until goals achieved
   - LLM-based goal achievement validation

6. **Reflection Loops** (Chapter 4)
   - Self-critique and improvement cycles
   - Quality gates for agent outputs
   - Iterative refinement patterns

7. **Agent-to-Agent Communication** (Chapter 15)
   - REST API exposure for agents
   - AgentCard-based capability discovery
   - Cross-instance agent collaboration

### Lower Priority

8. **Resource Optimization** (Chapter 16)
   - Model routing based on task complexity
   - Cost-aware execution strategies
   - Performance monitoring and optimization

9. **Advanced Reasoning Patterns** (Chapter 17)
   - Chain of Thought integration
   - Self-correction mechanisms
   - Hybrid reasoning approaches

## Multi-Agent Coordination Concepts

### Coordination Patterns
1. **Hierarchical**: Parent coordinator with specialized children
2. **Sequential**: Pipeline execution with state passing
3. **Parallel**: Concurrent execution with result aggregation
4. **Loop**: Iterative multi-agent refinement
5. **Routing**: Intelligent delegation based on request analysis

### Communication Mechanisms
1. **Session State**: Shared state with explicit keys
2. **HTTP APIs**: RESTful agent-to-agent communication
3. **Message Passing**: Event-based communication
4. **Memory Sharing**: RAG-based knowledge exchange

### Failure Handling
1. **Graceful Degradation**: Fallback agents for primary failures
2. **Retry Logic**: Automatic retry with backoff
3. **Human Escalation**: Hand-off to human operators
4. **Alternative Routing**: Dynamic re-routing on failure

## Implementation Recommendations

### Phase 1: Core Enhancements
- Implement parallel agent execution (Chapter 3)
- Add state-based agent communication (Chapter 8)
- Enhance spawning with routing logic (Chapter 2)

### Phase 2: Robustness
- Add exception handling and fallbacks (Chapter 12)
- Implement goal-driven execution (Chapter 11)
- Add reflection loops for quality (Chapter 4)

### Phase 3: Advanced Features
- Agent-to-Agent communication (Chapter 15)
- Resource optimization (Chapter 16)
- Advanced reasoning patterns (Chapter 17)

## Key Takeaways

1. **Explicit State Management**: Session state with named keys enables robust agent coordination
2. **Separation of Concerns**: Distinct agents for generation, critique, synthesis, and execution
3. **Graceful Degradation**: Always have fallback mechanisms for production reliability
4. **Goal-Oriented Design**: Explicit goals and success criteria improve task completion
5. **Standardized Interfaces**: MCP and AgentCard patterns enable agent ecosystem growth

The patterns in this repository provide a comprehensive roadmap for enhancing inber's agent orchestration, particularly around parallel execution, state management, and robust error handling. The modular, composable nature of these patterns aligns well with inber's architecture philosophy.

---

## Harness-watch — May 2026: Codex shipping patterns

Three concrete primitives that landed in [openai/codex](https://github.com/openai/codex)
between 2026-04-26 and 2026-05-03. Codex doesn't yet have its own
`docs/comparisons/codex.md`, so the observations live here as cross-cutting
patterns inber could adopt.

### 1. Multi-environment context per turn

[PR 20646](https://github.com/openai/codex/pull/20646) (stack: 20669, 20647) —
extends the rendered `environment_context` block so a single turn can carry
multiple selected environments, each with its own id and cwd. The model picks
an environment id, and process tools route via that `environment_id`. Single-
and zero-environment turns still go through the legacy cwd-only render path,
so this is purely additive.

**What inber should consider:** Inber's `forge` already manages isolated git
worktree slots per agent session, but the *current run's* tool execution still
binds to a single cwd. Codex's pattern is the natural next step: surface every
available worktree to the model in one prompt, let it pick, and route the
tool-call to that worktree. This is a small change to inber's tool dispatch —
add an optional `environment_id` parameter to file/exec tools and resolve it
through `forge` — but it unlocks the agent reasoning over multiple
checkouts/branches in one turn without spawning subagents per worktree.

### 2. Self-describing memory extensions

[PR 20602 + 20606](https://github.com/openai/codex/pull/20606) — Codex's
memory subsystem now supports pluggable "extensions." Each extension lives
under `memories/extensions/<name>/` and ships an `instructions.md` that tells
the consolidation agent how to read/write that extension's notes. The
write-pipeline seeds the `instructions.md` template the first time it lands.
Extension-specific behavior (an `ad_hoc` extension, a `prune` extension)
lives in dedicated modules under `memories/write/src/extensions/` rather than
a flat helper file.

**What inber should consider:** Inber's `memory-store` is a single uniform
schema (importance, decay, embeddings). Codex is asserting that *different
kinds of memory want different curation rules* — a hard rule, a personal fact,
an ad-hoc working note — and the consolidation agent should be told the rule
by the memory itself, not by hardcoded handlers. This maps cleanly onto
inber's existing memory tagging — a `kind` column plus a per-kind
`instructions.md` lookup would let agents author new memory categories without
shipping new code. Worth a sketch in `docs/memory-extraction-evaluation.md`
alongside the ByteRover comparison.

### 3. Effective config lockfile (export/replay)

[PR 20405](https://github.com/openai/codex/pull/20405) — Codex now writes
`<thread_id>.config.lock.toml` at session start, capturing the *resolved*
config after CLI overrides, defaults, feature aliases, model-catalog values,
and prompt setup have all been layered. A later run can load that lockfile,
regenerate the effective config, and fail-early on drift. There's an explicit
"allow_codex_version_mismatch" flag for tolerating binary drift while still
comparing the rest.

**What inber should consider:** Inber's reproducibility story today is
"replay the session log." That doesn't capture the *resolved* state of agent
config, model selection, tool registry, or memory snapshot at the moment of
the run. A config lockfile per session — written by the engine when a run
starts, validated against on replay — would turn "I can't reproduce this
trace" into a structured diff. This composes well with the trajectory-export
work already foreshadowed for evolution loops (see April Zup paper, AHE
paper). Smallest first cut: have `engine/` dump a resolved `agent.lock.json`
into the session directory.

## Harness-watch — May 2026: MCP capability negotiation goes richer

Two independent harnesses landed expansions of the MCP `initialize`
handshake in the last week. Both push the same idea: **the host advertises
non-tool capabilities (UI rendering, user elicitation) during MCP init,
and tool servers light up features against those capabilities.** This is a
shift from MCP being a tool-call transport to being a richer
capability-negotiated channel.

### 1. Goose MCP Apps — host UI capability flows into MCP init, tool results carry replayable payloads

[PR 8623](https://github.com/block/goose/pull/8623) +
[PR 8632](https://github.com/block/goose/pull/8632) (goose). Goose 2 (the
ACP client) declares an MCP-Apps host capability during ACP `initialize`;
`goose serve` stores it per ACP connection; the runtime translates it into
the downstream MCP `initialize` payload as
`capabilities.extensions["io.modelcontextprotocol/ui"] = {mimeTypes:
["text/html;profile=mcp-app"]}`. When an app-capable tool completes, the
runtime reads the `ui://...` resource the tool returns, attaches the
hydrated snapshot to `tool_result.meta.goose.mcpApp`, and forwards it
through `tool_call_update._meta.goose` for both live render and replay.
The payload survives session reload, so reopening an old session
reconstructs the rich tool-result view without re-running the tool. Two
architectural points worth pulling out: (a) host UI capability is an
*ACP-client property*, not a runtime guess from platform — the runtime
stops inferring; (b) the rich payload is *backend-owned and persisted*,
not derived in the UI from raw tool output.

**What inber should consider:** Tool-store MCP servers in inber today
return text/JSON; BridgeUI renders a generic tool result. The Goose
pattern maps cleanly onto inber's three-layer setup: (1) BridgeUI declares
a UI capability shape during dash's MCP init to tool-store; (2) tool-store
MCP servers can return a `ui://session/<id>/...` resource alongside their
text; (3) llm-bridge-server reads the resource at tool-completion time and
attaches it to the canonical message under a stable `_meta` key, so it
flows through the bus → log-store → bridge-ui replay path identically to
plain tool output. The persistence-on-tool-result detail matters: without
it, rich payloads get lost the moment the session reloads, and we end up
re-fetching or losing fidelity. Worth a sketch in
`docs/multi-agent-design.md` or a new `docs/mcp-apps.md` before
prototyping — there is a real chance llmux/dash diverge on how they want
to render `ui://` resources, so the *backend payload contract* should be
nailed down first.

### 2. Codex — MCP elicitation capability adoption

[PR 20562](https://github.com/openai/codex/pull/20562) (codex). Codex
moves to the **2025-06-18 MCP elicitation capability shape**, the
spec-blessed channel for an MCP tool to ask the user for input mid-call
(approve a destructive action, pick from N options, supply a missing
field) without the tool inventing its own ad-hoc protocol. Goose 2's MCP
init in PR 8623 above already advertises `elicitation: {}` in its
capability block — i.e., two harnesses converging on the same canonical
spec in the same week.

**What inber should consider:** Inber's permission/guard layer currently
intercepts dangerous tool calls and surfaces them via the existing
permission-prompt flow (HookEvent awaiting_resolution, shipped 2026-05-01
— see `project_permission_prompt_followups`). MCP elicitation is the same
shape one layer down: the *tool itself* asks the user a question through
the host. For tool-store MCP servers, supporting elicitation means
inber's MCP client (in llm-bridge-server) needs to advertise
`elicitation: {}` on init and route incoming elicitation requests through
the same UI surface the permission prompts use. This is a small wire-up
job, not an architecture change — but it should land before tool-store
authors start inventing their own clarification protocols and we end up
with N incompatible flavors.

### Cross-cutting takeaway

The theme is **capability-negotiated MCP**: the host says what it can do
(render UI, ask the user a question, sample from a model) on init, the
tool server tailors its responses to fit, and the resulting payloads
travel as first-class metadata on tool results — replayable, persisted,
and decoupled from any one frontend. Inber's tool-store + bridge-ui +
log-store split is already the right shape for this; the pieces missing
are (a) a richer init handshake on the llm-bridge-server MCP client and
(b) a stable `_meta` slot on canonical tool-result messages so rich
payloads survive the bus → log-store → bridge-ui round trip. Both are
small changes that compose with each other and with the existing
permission-prompt work.

## Harness-watch — 2026-05-09: One tool, one shape (codex apply_patch)

Codex this week collapsed `apply_patch` from two invocation paths to one.
[PR 21651](https://github.com/openai/codex/pull/21651) deletes the
function-style (JSON-args) registration of `apply_patch`; [PR
21687](https://github.com/openai/codex/pull/21687) flips the freeform/
grammar-constrained variant to default-on. The grammar variant lets the
model emit the patch payload as a natural diff body inside a custom tool
call instead of stuffing it inside a JSON string parameter — the diff
format is the tool-call format.

The design statement is the interesting part, not the diff: **two ways to
invoke the same tool is a contract bug, not a feature.** Both shapes were
maintained for several releases; codex's read of the data is that having
both paths created surface ambiguity for the model (which shape to pick),
the harness (two parsers to keep in sync), and the test corpus (two
trajectory shapes per task). Collapsing to a single canonical shape is a
deliberate tightening, and the choice of the freeform/grammar shape over
the JSON-args shape is itself a vote: when a tool's payload has a
natural format (a unified diff, a SQL query, a shell command), making
that format *be* the tool-call body avoids a layer of escape/quote/
re-parse that the model has to navigate inside its tokens.

**What inber should consider:** Inber's tool surface today is JSON-args
across the board — `Read`, `Edit`, `Bash`, `Grep`, etc. Most of those
tools have payloads that are unambiguously JSON-friendly (paths,
patterns, flags). The exception is `Edit`: the `old_string`/`new_string`
parameters are quoted blobs of source code that the model has to escape
into JSON, and that escaping is a known source of "edit didn't apply
because of a stray backslash" failures. Codex's bet is that the patch
shape should be the tool body, not a JSON-encoded string; for inber,
the smallest version of that bet is to add a freeform/diff-body variant
of `Edit` (or a new `ApplyPatch`) that takes a unified diff as the body
and reduces the model's escaping burden. This is a tools-side
experiment, not an architectural one — but it's worth measuring against
inber's existing edit-failure rate before committing.

The cross-cutting principle, worth flagging beyond the apply_patch
specifics: **when a tool has multiple invocation shapes, the harness
owns a contract bug.** Inber's tool registry should pick one shape per
tool and never carry two. If a new shape proves better, the migration
is a one-shot replacement, not an additive co-existence.

## Harness-watch — 2026-05-10: Subagent denies must propagate (opencode #26597 + arXiv 2605.05440)

Two signals from the same week converge on a design gap that inber's
permission layer has not yet addressed: **delegation across an agent
boundary is a permission-propagation event, and it has more than one
scope to carry.**

[opencode PR 26597](https://github.com/sst/opencode/pull/26597) (merged
2026-05-09) fixes a Plan Mode escape: opencode has two permission
scopes, an *agent* ruleset (e.g. Plan Mode's `edit: deny *`) and a
*session* ruleset, and the `task` tool that spawns subagents was only
forwarding the parent **session's** denies. A plan-mode session calling
`task` to spawn a `general` subagent silently produced a child with
full edit/write rights. The fix extracts a shared
`subagent-permissions.ts` helper that merges *both* scopes' denies
into the spawned subagent — and the regression test imports the
production helper so an inline reimplementation in `task.ts` can't
silently drop the agent-scope step again.

[arXiv:2605.05440 — Authorization Propagation in Multi-Agent AI
Systems](https://arxiv.org/abs/2605.05440) (submitted 2026-05-06)
formalizes exactly this class of bug as the general problem.
Authorization invariants must be maintained "as non-human principals
retrieve data, delegate tasks, and synthesize results across changing
boundaries"; the paper identifies three sub-problems (transitive
delegation, aggregation inference, temporal validity) and seven
structural requirements, and notes that "ordinary system behavior, not
only adversarial action" already produces these failures in production
deployments. The opencode bug is a clean instance of the transitive-
delegation case: a guard at one boundary fails to compose with the
next boundary down.

**What inber should consider:** Inber's permission gating now lives in
the bridge-server prehook (per `project_permission_prompt_followups`),
which is a single check at one boundary. The system has at least two
*scopes* it could attach denies to today — per-agent (agent-store
config) and per-session (the prehook ruleset that lives with the
session) — and at least one place where delegation crosses a boundary
without a documented contract: subagent spawning via the agent system
(forge worktree slots + spawned agents from agent-store). Concrete
moves the convergent signals support:

- Audit subagent spawning paths to confirm the prehook applies to the
  child's tool calls under the **union** of parent-agent and parent-
  session denies, not just the child's own ruleset. The opencode bug
  shape — "the child got a fresh permission context that quietly
  dropped the parent's restrictions" — is the failure mode to look
  for.
- Make the propagation logic a *single shared function* called by the
  spawn path and asserted by a regression test that imports the same
  function. The opencode fix is small (one helper, one test) precisely
  because the prior version inlined the merge in two places.
- Treat permission propagation as a first-class harness concern, not a
  per-tool concern. The paper's framing — "identity governance as
  infrastructure, evaluated continuously, enforced at every interaction
  boundary" — argues against bolting on per-tool denylists and for a
  uniform propagation rule applied at every spawn, hand-off, or
  retrieval step.

A complementary design point from earlier work the paper cites
([arXiv:2603.05344](https://arxiv.org/abs/2603.05344)) is that the
strongest version of subagent permission scoping is **schema-level
filtering** — the child's tool schema literally omits the tools it
can't use, so the model never sees a denied capability and the
runtime check becomes a defense-in-depth layer rather than the only
guard. Worth pairing with the opencode pattern when inber writes up
its subagent permission contract: agent-scope denies should also
filter the tool schema the child sees, with the prehook as the
fallback.

## Harness-watch — 2026-06-02: Codex idle-steering, log compaction, per-tool concurrency

Codex still has no own comparison file; its patterns live here. Five
non-trivial codex changes landed this window.

### 1. Goal extension: idle-turn steering toward a persisted goal

[PR 25096](https://github.com/openai/codex/pull/25096) adds an extension-owned
`GoalApi` (get/set/clear a thread-level goal); [PR 25576](https://github.com/openai/codex/pull/25576)
moves the *steering* prompts into three embedded templates — `continuation.md`
(keep going), `budget_limit.md` (approaching turn/cost budget),
`objective_updated.md` (goal changed mid-run) — with XML-escaped user objectives
and budget-counter variables; [PR 25577](https://github.com/openai/codex/pull/25577)
removes the Plan-mode early-return from `try_start_turn_if_idle`, keeping
idle-injection a generic turn-lifecycle primitive with mode policy at the caller
boundary. Together: when a session goes idle, inject a steering message
re-pointing the agent at a standing goal, kept-alive in-process rather than by
respawning.

**What inber should consider:** inber's kanban task-completion-loop drives goals
by *spawning fresh sessions* (scoper + dispatcher). Add an in-session
idle-injection goal loop (a continuation / budget / objective-changed steering
set) as a cheaper alternative to full session revival for goals that fit one
context window.

### 2. Cold rollout (session-log) compression with a materialize-before-append invariant

A four-PR stack ([25089](https://github.com/openai/codex/pull/25089),
[25087](https://github.com/openai/codex/pull/25087),
[25654](https://github.com/openai/codex/pull/25654),
[25659](https://github.com/openai/codex/pull/25659)) zstd-compresses cold session
rollouts behind a flag. The load-bearing rule: writers only ever append to plain
`.jsonl`; a `.jsonl.zst` is purely a cold *read* form, and any write path (resume,
metadata append) must **materialize back to plain JSONL first** (the guarded
failure is appending raw JSONL bytes onto a zstd file). The compactor scans only
*archived* (never active) sessions, caps two in-flight jobs, and uses a 6-hour
staleness lock-file to throttle rescans.

**What inber should consider:** inber's log-store keeps full session event logs
forever. Compress only archived (never active) logs to `.zst` with a hard
"plain form is the only write target, materialize-before-append" invariant and a
throttled, bounded-concurrency background compactor — reclaims disk without ever
risking a resumable session.

### 3. Per-tool parallel-call safety flag, not a global serial default

[PR 25702](https://github.com/openai/codex/pull/25702): standalone web-search
inherited the shared *serial* executor lock and ran one call at a time though the
backend is concurrency-safe. The fix has the tool *advertise* parallel-call
support, with a test asserting the flag. Parallel-safety is a per-tool property
the tool declares and the executor honors — a conservative serial default
silently bottlenecks safe tools.

**What inber should consider:** give inber tools an explicit `parallel-safe`
capability flag honored by the executor (read/grep/web-search safe; write/edit/
bash sharing a cwd not), rather than a single global serial-vs-parallel switch.

### 4. `followup_task`: re-task a still-running subagent in place

[PR 25636](https://github.com/openai/codex/pull/25636) renames MultiAgentV2's
`assign_task` → `followup_task`, surfacing the design point: the *initial spawn*
is separate from a dedicated tool whose only job is sending an *additional* task
to an existing, still-running agent — long-lived subagents you re-task without
respawning. The trace reducer keeps accepting the legacy `assign_task` event name
so older logs still classify.

**What inber should consider:** inber's subagents are spawn-and-collect. Add a
`followup` tool to re-task a running subagent in place — and, like codex, keep the
old event name accepted in any trace/log reducer when renaming such a tool.

### 5. Guardian auto-review: headless default must not silently disable the policy

[PR 23767](https://github.com/openai/codex/pull/23767) adds an
`auto_review_model_override` catalog field so a parent model can steer auto-review
to a different reviewer slug; [PR 23763](https://github.com/openai/codex/pull/23763)
fixes `codex exec` (headless) unconditionally forcing `approval_policy = never`,
which silently bypassed the reviewed write path even when the reviewer was
`AutoReview`. Principle: a global headless convenience default must not quietly
turn off a safety layer that was explicitly configured.

**What inber should consider:** audit that inber's "unattended → auto-approve"
scheduler default cannot silently bypass the permission prehook when a
review/approval policy was explicitly configured; consider a separate
reviewer-model slot (with a catalog override) gating writes, distinct from the
executing model.
## Harness-watch — 2026-06-03: Codex permission profiles, environment-scoped grants, per-thread multi-agent runtime

Codex still has no own comparison file; its patterns live here. Three related
changes this window, all about *where a permission/runtime decision is scoped*.

### 1. Permission profiles subsume sandbox policy

[PR 25926](https://github.com/openai/codex/pull/25926) expresses implicit sandbox
defaults as built-in `PermissionProfile` objects, and [PR 25739](https://github.com/openai/codex/pull/25739)
derives the built-in profiles from raw policies (with [PR 25943](https://github.com/openai/codex/pull/25943)
removing the dead profile→sandbox fallback). The move: the legacy `SandboxPolicy`
was the primary shape; now a named *permission profile* is, and sandbox mode is one
projection of it. Defaults become explicit and observable rather than hidden in a
policy projection — trusted/untrusted roots resolve to `:workspace` (write),
trust-undecided roots to `:read-only`, unsandboxed Windows to `:read-only`.

**What inber should consider:** inber currently keeps sandbox/filesystem confinement
and the permission prehook as separate concepts. Codex's unification argues for a
single named **permission profile** (e.g. `read-only`, `workspace`, `trusted`) that
*derives* the sandbox confinement, the prehook's allow/deny defaults, and the
auto-approve posture together — instead of three knobs that can silently disagree.
Make the default profile per-root *observable* (logged at session start), not an
implicit fallback.

### 2. Permission grants keyed by environment id, not cwd

A stack ([25850](https://github.com/openai/codex/pull/25850),
[25858](https://github.com/openai/codex/pull/25858),
[25862](https://github.com/openai/codex/pull/25862)) makes remembered
("sticky") permission grants environment-aware: grants are now indexed by
`environment_id` at turn and session scope, and the pending request retains the
selected `TurnEnvironmentSelection` so the approval records against the correct
environment. Previously, with both local and remote executors in play, a grant was
effectively cwd-only and could leak across executor contexts. The external
`request_permissions` API is unchanged; omitted targeting still binds to the primary
turn environment.

**What inber should consider:** this is the harness-level enforcement of "approval
in one context doesn't extend to the next." If/when inber runs a turn across more
than one executor (local worktree + remote/sandbox environment), a remembered grant
must be keyed by `(environment, action, resource)` — never by cwd alone — or an
approval granted in a throwaway sandbox silently authorizes the same action against
the real tree. Relative-path permission requests must resolve against the *selected*
environment, not a turn-global cwd.

### 3. Multi-agent runtime resolved per thread, from persisted metadata

A stack ([25720](https://github.com/openai/codex/pull/25720)–[25724](https://github.com/openai/codex/pull/25724),
plus [25841](https://github.com/openai/codex/pull/25841) keeping startup prewarm
aligned) adds typed multi-agent-runtime metadata and **resolves the effective
runtime per thread** from persisted metadata + inherited runtime + current model
selection, with a tested remote-runtime override. This extends the MultiAgentV2 /
`followup_task` work (2026-06-02 entry): a thread now carries which runtime its
subagents execute on (local vs remote), persisted so resume picks the same one, and
prewarm targets the *resolved* runtime rather than a global default.

**What inber should consider:** inber's subagent execution backend is effectively a
process-global choice. Codex makes it a per-thread, persisted property: each session
records its multi-agent runtime so a resumed or revived session re-binds to the same
executor, and any warm-up cost is paid against the resolved runtime, not a default
that may be wrong. Worth a field on inber's session/thread record (kanban task-
completion-loop revival especially) capturing the intended subagent runtime so
revived sessions don't silently switch executors mid-goal.
## Harness-watch — 2026-06-04: Skills as a context-budget problem (codex per-turn catalog vs. static registries) + MAv2 isolation/compaction polish

Four harnesses shipped skills systems in the same week, and the *design split*
is the story. The question every one of them is answering: a growing
`SKILL.md` library can't all live in context, so when and how do you load it?

### 1. Codex: per-turn skill catalog, injected as prompt fragments (not tools)

A stack ([#25953](https://github.com/openai/codex/pull/25953) scaffold,
[#25959](https://github.com/openai/codex/pull/25959) turn-input contributors,
[#26167](https://github.com/openai/codex/pull/26167) v1 prompt injection,
[#26106](https://github.com/openai/codex/pull/26106) per-turn catalog) builds a
`codex-skills-extension` that resolves the *available* skill catalog **per turn**.
The key move is #26106: skills moved from a `TurnLifecycleContributor` hook (which
only had a turn id) to a `TurnInputContributor` that fires *during turn assembly,
after the turn's environments are resolved* — so the catalog query is scoped to
the turn's executor authorities (environment ids, cwds). Selected skills are then
**injected as prompt fragments**, not exposed as tools, with bounded rendering
(SKILL.md truncation) and per-thread mutable skills state. Selections resolve from
structured `UserInput::Skill`, `skill://` paths, skill-file mentions, and text
mentions.

By contrast, [opencode #30617](https://github.com/sst/opencode/pull/30617)
(SkillV2 registry, multi-source directory/URL discovery) and
[cline #11161](https://github.com/cline/cline/pull/11161) (plugin-bundled skills,
deduped discovery, skills as a first-class plugin capability per
[#11244](https://github.com/cline/cline/pull/11244)) build **static registries** —
skills discovered once and merged into a global list. Goose already supplies the
*measurement* side inber documented earlier (`goose skills` per-skill token counts,
goose.md §3). So the field is splitting into discover-globally vs. resolve-per-turn,
with codex furthest toward context-efficiency.

**What inber should consider:** inber loads its skill/SKILL.md surface into the
system prompt once per session (good for caching). Codex's per-turn injection is
the opposite trade — worse cache locality, but the catalog never carries skills
irrelevant to *this* turn's environment. The synthesis worth prototyping: keep a
small, stable, cacheable *core* skill set in the system prompt, but resolve an
*environment-scoped extension catalog* per turn (keyed by the same worktree/agent-
store environment inber already tracks) and inject only those as prompt fragments
with bounded rendering. Pair it with goose-style per-skill token accounting so the
per-turn injection has a visible budget. This matches the "Scaling Laws of Skills"
finding (papers/2026-06): routing accuracy decays logarithmically with library
size, so *not* putting the whole library in front of the model is the win.

### 2. Codex MAv2: subagent metadata hidden by default; compaction rewrites instead of drops

Two refinements past the per-thread runtime work (2026-06-03 entry).
[#26114](https://github.com/openai/codex/pull/26114) flips `hide_spawn_agent_metadata`
to **true by default**: the `spawn_agent` tool spec no longer shows the parent
model/reasoning_effort/service_tier or "available model overrides" — a deliberate
isolation boundary so a parent can't (by default) micromanage a subagent's model.
[#26144](https://github.com/openai/codex/pull/26144) + [#26179](https://github.com/openai/codex/pull/26179)
formalize a three-phase agent lifecycle: running → **completed-but-open (still
consuming a concurrency slot)** → closed; workers may not `close_agent` themselves,
parents must reclaim the slot. Separately, [#26251](https://github.com/openai/codex/pull/26251)
changes remote compaction to **rewrite oversized tool outputs to smaller versions
rather than deleting them**, preserving the causal tool-call↔result pairing
("incrementality") even under token pressure.

**What inber should consider:** (a) inber's subagent spawn currently lets the
parent pass model overrides freely — make metadata-hiding the default and require
an explicit flag to expose it, so subagent model choice is an isolation boundary
not an accident. (b) Track that a *finished* subagent still occupies a concurrency
slot until explicitly reaped — relevant to the kanban task-completion-loop, where
"done" sessions that aren't closed silently starve the pool. (c) When inber's
`turn_summary.go` compacts, prefer **shrinking** large tool results (head/tail +
elision marker) over dropping them, so the tool-call→result link survives
compaction — a cheaper variant of the parallel-per-block compaction in the
papers doc.


## Harness-watch — 2026-06-05: Codex scoped context discovery (AGENTS.md through the execution environment + logical paths) and tool-surface ≠ tool-availability

### 1. Context/instruction discovery must run against the environment the agent actually executes in

[PR 26205](https://github.com/openai/codex/pull/26205) routes workspace
`AGENTS.md` resolution through the selected `EnvironmentManager` filesystem
instead of the host FS, so remote workspaces and child agents read instructions
from *their own* environment, and introduces `LoadedAgentsMd` (ordered
user/project/internal sources) that the thread exposes so the app-server reports
**exactly what was loaded** rather than re-deriving a guess. [PR 26465](https://github.com/openai/codex/pull/26465)
fixes the ancestor walk to follow the **logical** (configured) path, not the
symlink-canonicalized physical path — so `logical-repo/workspace` loads
`logical-repo/AGENTS.md` as its parent, matching user intent (file reads still
follow symlinks; only the discovery walk uses the logical path).

**What inber should consider:** as inber's memory/per-harness context layer
(project_memory_layer_split) and forge worktree slots mature, instruction/context
discovery (`INBER.md`, project blocks) must run against the **agent's actual
execution environment** — the worktree/sandbox/remote runtime the child runs in,
not the orchestrator host — and inber should report the *actually-loaded* sources
back to bridge-ui/curators, not a re-derived list (closes the report-vs-reality
gap). And when walking ancestors for project context, walk the logical configured
path so a symlinked worktree doesn't silently reparent which `INBER.md` applies.

### 2. Tool-surface exclusion is presentation-scoped, not capability removal

[PR 26320](https://github.com/openai/codex/pull/26320) lets `code_mode` exclude
tool namespaces from its *nested tool surface and descriptions* while the tools
stay registered, still appear in mixed mode, and remain reachable via top-level
`tool_search` (deferred-tool guidance is derived after filtering so `exec` never
advertises a hidden tool). The design point: separate "what's discoverable on a
given surface" from "what's available." This is a direct analogue of inber's own
deferred-tool / `tool_search` layering (TOOL-ROUTING) — a tool can be absent from
one presentation layer's schema (saving prefix budget) yet still fetchable on
demand, which pairs with the tool-schema-compression paper (papers/2026-06).
(Codex also moved reasoning-effort to a model-advertised open string set —
[#26444](https://github.com/openai/codex/pull/26444)/[#26446](https://github.com/openai/codex/pull/26446)
— a sound "defer capability knobs to model metadata" pattern, mostly plumbing.)

## Harness-watch — 2026-06-06: org-level managed permission allowlists — a non-overridable policy layer above user/agent-selectable profiles

[PR 24852](https://github.com/openai/codex/pull/24852) adds
`allowed_permission_profiles` to codex's layered requirements files: a **closed
allowlist** of which permission profiles a user/session may select at all.
Profiles set `true` are permitted; missing or `false` are denied — including
built-ins like `:danger-full-access`, blocked unless explicitly allowed. It
merges across requirement layers (higher layers override specific entries) and
forces `default_permissions` to resolve to an allowed profile. This sits *above*
the per-session permission-profiles + env-scoped grants documented in the 06-03
entry: those are what a session chooses; this bounds what it is allowed to
choose, set by whoever owns the deployment, not the user or the agent.

**What inber should consider:** inber's permission prehook
(`project_permission_prompt_followups`) decides per-call allow/deny but has no
notion of a *deployment-level ceiling* on which permission posture a session may
even request. A small closed-allowlist layer — keyed in deploy config, merged
ambient→project, defaulting to deny-by-omission for dangerous postures
(full-FS, unsandboxed exec) — would let unattended surfaces (autoworker, kanban
task-completion-loop) run under a hard org ceiling that a steered or
prompt-injected agent cannot widen, independent of the per-call prehook logic.

## Harness-watch — 2026-06-07: a single per-model flag can flip tool-execution locus AND the request contract ("Responses Lite")

Codex added an opt-in **Responses Lite** mode gated by a per-model catalog flag
(`ModelInfo.use_responses_lite`) and signaled on the wire by a transport header
([#26487](https://github.com/openai/codex/pull/26487) catalog flag +
reasoning/parallel override, [#26490](https://github.com/openai/codex/pull/26490)
standalone tools, [#26542](https://github.com/openai/codex/pull/26542) transport
header + WS reconnect). In Lite mode the provider runs **no hosted tools**, so the
harness must emit empty hosted-tool specs and route web-search/image-gen through
its own client-executed "standalone" executors — and the same flag also forces
`reasoning.context=all_turns` and disables `parallel_tool_calls`. Two design
points: (1) one model-advertised capability flag flips both *where a tool
executes* (hosted vs harness-owned) and *the shape of the request* (reasoning
persistence, parallelism); (2) the marker is **request-scoped over HTTP but
connection-scoped over WebSocket**, so a pooled socket opened for the opposite
mode must be reconnected or it silently sends the wrong contract.

**What inber should consider:** model "where does this tool execute" as a
first-class, model-driven dimension in the tool-contract layer (TOOL-ROUTING) —
don't assume a hosted/server-side tool is always available; a per-model flag
should be able to swap a tool to a harness-owned executor *and* adjust request
knobs (thinking, parallel calls) in one switch. Operationally: any keep-alive /
connection pooling in inber's transport must key the pooled connection on
connection-scoped contract markers and force-reconnect on change — the same
hazard class as the opencode session-scoped prompt-cache key (opencode 2026-06-06).

## Harness-watch — 2026-06-09: Codex MAv2 grows a *fleet* model — residency ≠ identity, concurrency = active execution, interrupt ≠ close

Four related PRs this window turn Multi-Agent V2 from "spawn a subagent" into a
managed *fleet* of durable logical agents, decoupling four things inber currently
conflates into one "subagent is alive" notion.

1. **Residency LRU separate from logical identity** ([#26632](https://github.com/openai/codex/pull/26632)).
   A v2 agent is a *durable logical agent*, addressable even when its thread is not
   loaded. An `AgentControl`-scoped LRU bounds how many subagents are *resident* in
   `ThreadManager`; spawning/reloading reserves residency first, and at capacity the
   least-recently-used **idle** subagent is paged out — still registered and
   revivable, just not in memory. `ThreadManager` stays a dumb loaded-thread store; it
   does not own the eviction policy.
2. **Concurrency counts active execution, not existence** ([#26969](https://github.com/openai/codex/pull/26969)).
   The execution limit now counts *active non-root turns* via an RAII guard owned by
   `RunningTask`, checked before spawning or waking an idle agent — not resident or
   durable thread count. Automatic idle continuations are exempt so `/goal` work isn't
   dropped when capacity is briefly full; root and V1 turns stay outside the limiter.
   (Best-effort: synchronous admission can overshoot slightly under races.)
3. **`close_agent` → `interrupt_agent`** ([#26994](https://github.com/openai/codex/pull/26994)).
   Once residency owns capacity, the model-facing verb is renamed to describe what it
   actually does: interrupt the target's *current turn* (`Op::Interrupt`) without
   making the agent unavailable for future tasks. Interrupted agents stay registered;
   stale resident entries route through the dead-thread cleanup path so they don't keep
   consuming capacity. Root/self targets are rejected.
4. **Resume stays lazy** ([#26997](https://github.com/openai/codex/pull/26997)).
   Resuming a v2 root no longer eagerly walks and reopens the persisted descendant
   tree — descendants stay unloaded until explicitly needed. Idle *interrupted*
   residents are also made LRU-evictable (previously a set full of interrupted agents
   could pin every slot and block new spawns with `AgentLimitReached`).

The through-line: **logical identity, residency (in-memory), execution (a turn
running), and reachability are four independent properties**, each with its own
lifecycle, rather than one boolean. This supersedes the simpler three-phase
(running → completed-but-open → closed) lifecycle from the 2026-06-04 entry.

**What inber should consider:** inber's kanban task-completion-loop already revives
and spawns sessions, but treats "session exists" ≈ "session occupies a slot." Adopt
the four-way split: (a) keep finished/sub-agent sessions *durable and revivable by
task name* while paging idle ones out of memory under an LRU — directly fixes the
"done-but-unclosed sessions starve the pool" hazard noted on 06-04; (b) count the
concurrency ceiling by *active turns*, not session-record count, so a pool of mostly
idle revivable sessions doesn't read as full (exempt scoper/dispatcher continuations
the way codex exempts idle `/goal` continuations); (c) make "interrupt the current
turn" distinct from "retire the session" in the dispatcher's vocabulary; (d) on
revive, do **not** eagerly reopen the whole child tree — load lazily on demand.

### Skill contract: progressive disclosure picks *files*, not *fractions*

[#27044](https://github.com/openai/codex/pull/27044) tightens the per-turn skill
model (06-04 entry): the main agent must read selected `SKILL.md` files **completely
through EOF** — continuing truncated/paginated reads — and must read task-required
instruction references *itself* rather than delegating their interpretation to a
subagent. Rationale: partial reads skip routing/verification requirements that live
later in a skill file, and delegated summaries silently drop constraints. Progressive
disclosure selects *which* files are relevant; it does not license partial reads of a
selected file. **What inber should consider:** inber's reference-based prompt
architecture (`reference-based-prompt-architecture.md`) and skill surface rely on the
agent fetching referenced files — encode the same contract: once a SKILL.md/reference
is selected for the turn, require a complete read before acting on it, and don't let a
subagent's summary stand in for the main agent reading a constraint-bearing reference.

### Context budget as a tool the agent can query, not just a number it's told

[codex #27518](https://github.com/openai/codex/pull/27518) adds a **context-remaining
tool** the model can invoke mid-turn to read how much of its context window is still
available, alongside [#27663](https://github.com/openai/codex/pull/27663) which keys the
token-budget context by `thread_id` so the figure is per-conversation rather than
global. The shift is from *passive* budget signalling — a token count injected into the
system prompt or a silent truncation the model never sees — to an *on-demand* contract:
the agent decides when it needs to know, and gets an authoritative answer it can act on
(wrap up, summarise, spawn a fresh sub-thread) before the engine forces a compaction.
This is the same posture as goose's "trust the live context window" (goose.md, 06-02
entry) but exposed *through the tool surface* instead of a prompt fragment, so the model
can branch on it deterministically.

**What inber should consider:** inber tracks remaining context centrally (smart-truncation
/ context-loading) but the agent only learns about pressure *reactively*, when truncation
or compaction has already happened. Add a cheap read-only `context_remaining` tool (or a
field on an existing status tool) that returns remaining tokens for the **current bridge
session**, keyed by session id the way codex keys by thread — so a long autoworker/scoper
run can voluntarily checkpoint to `memory-store` and hand off to a fresh session *before*
hitting the wall, rather than getting silently compacted mid-task. Pairs with the
budget-eviction paper below (`docs/papers/2026-06-harness-research.md`, Beyond Compaction):
the tool gives the agent the signal, graduated eviction is what the engine does when the
agent doesn't act on it in time.

## Harness-watch — 2026-06-14: split-credential storage — key in the OS vault, large ciphertext in namespaced files

Codex landed a four-PR encrypted-auth stack this window:
[#27504](https://github.com/openai/codex/pull/27504) (feature/config gate),
[#27535](https://github.com/openai/codex/pull/27535) (auth-specific secret namespaces),
[#27539](https://github.com/openai/codex/pull/27539) (CLI auth on encrypted local
secrets), and [#27541](https://github.com/openai/codex/pull/27541) (MCP OAuth on the same
backend). The forcing function is mundane but instructive: Windows Credential Manager caps
a generic credential blob at 2,560 bytes, and serialized ChatGPT-login / OAuth-refresh
payloads blow past it. The design response is the reusable part — **don't put the secret in
the OS keyring, put only the *encryption key* there**, and store the (large) ciphertext in
an `age`-encrypted file on disk. Two further moves: (1) **per-auth-class namespacing** —
separate encrypted files (`cli_auth.age`, `mcp_oauth.age`, `local.age`) rather than one
credential blob, so CLI-auth and MCP-OAuth have isolated blast radius and rotate
independently; (2) **migration with stale-copy cleanup** — on a successful encrypted save
(and on logout) the old plaintext `auth.json` / direct-keyring / fallback copies are
actively deleted, so a credential never lingers in two stores after the format flips.

**What inber should consider:** inber's `auth-store` (`:8303`, `reference_auth_store`) is
the canonical vault and already does OAuth refresh + lease enforcement, but it stores
credentials directly. Two cheap, independent borrows: (a) for oversized payloads
(multi-field OAuth refresh blobs, provider session bundles) adopt the **key-in-vault /
ciphertext-at-rest split** so the vault row holds a small key reference and the bulk
ciphertext lives in an encrypted partition — keeps the hot path small and bounds what a
single vault read exposes; (b) **namespace credentials by consumer-class** (per-service /
per-harness) instead of one flat table, so a leak or rotation is scoped to one namespace
rather than the whole vault. The migration discipline is the must-have regardless of (a)/(b):
when auth-store rewrites or re-keys a credential, delete the prior representation in the
same transaction — pairs with `feedback_audit_deployed_env` (a credential lingering in two
places is exactly the "what's actually live" drift that audit lesson is about).

## Harness-watch — 2026-06-16: a nested review/approval agent must NOT inherit skills or memory (trust boundary)

Codex [#28285](https://github.com/openai/codex/pull/28285) ("guardian: isolate review
context from skills and memories") hardens the Guardian — codex's nested approval/review
agent that ingests the **parent session transcript as untrusted evidence** to score an
action. Two changes, both about keeping the derived review session minimal and trustworthy:
(1) **skip skill/plugin discovery** when building Guardian turns, so a `$skill` mention
sitting in the assessed transcript stays *visible only as transcript text* and is never
expanded into an injected skill body — i.e. the transcript can't smuggle new instructions
into the reviewer (a prompt-injection vector); (2) **disable memory context and the memory
tools** in the derived Guardian config, because user/project memory is unrelated
model-visible context that biases an approval decision and bloats the request. The general
rule: when you spin up a sub-agent to *judge* a transcript, the transcript is data, not a
control channel — auto-injection of skills/memories that's correct for a *working* agent is
a contamination + injection risk for a *reviewing* one.

**What inber should consider:** inber's review/verify/approval surfaces (security-guidance
v2's agentic reviewer, `/code-review` subagents, the PreToolUse prehook when it escalates to
an LLM judge) all ingest parent context, and inber now auto-selects skills
(`reference_skill_store`) and injects memory (`project_memory_layer_split`, memory-store on
:8160). Give the review/judge sub-agent a **derived config that disables skill auto-selection
and memory injection by default** — the judged transcript must reach it as inert evidence,
not as a source of `$skill`/memory directives the parent (or an attacker upstream of the
parent) can use to steer the verdict. Concretely: a "reviewer" agent-type/bundle whose
resolver returns zero skills and no memory tools, and a request-layout test asserting that a
skill mention in the input transcript appears only as quoted text. Composes with the
isolate-subagent-model boundary (06-04 entry) and the tiered security-review ladder
(claude-code.md) — same instinct, applied to the reviewer's *context* rather than its model.

## Harness-watch — 2026-06-17: three small Codex inter-agent contracts (interruptible wait, error precedence, typed envelope)

Codex landed three message/tool-contract refinements that sharpen the steer-queue and
multi-agent threads already tracked here. They share one principle: *the boundary between
agents should carry typed, observable signals — not opaque blobs or events that overwrite
each other.*

**1. Interruptible `sleep` tool ([PR 28429](https://github.com/openai/codex/pull/28429)).**
A native `sleep` tool (behind a `sleep_tool` flag) takes a bounded `duration_ms` and pauses
the agent — but ends early the moment steered user input or mailbox (inter-agent) input
arrives, and reports elapsed wall-clock in both completed and interrupted outputs. It's a
first-class `SleepItem` retained in thread history, not a shelled-out `sleep`. The novel bit
is the *coupling*: a voluntary back-off wait wired into the same steer/mailbox queues, so it
is cancellable and visible instead of an opaque blocked subprocess.
- **What inber should consider:** give inber a native `wait`/`sleep` tool whose pause is
  cancelled at the next steer/mailbox boundary (the same model-call boundary dexto's 06-13
  refinement defines), emitting a history-retained wait item — so an autoworker/scoper
  polling for external work backs off without a dead shell `sleep`, and a user steer or a
  sub-agent message wakes it immediately rather than after the full delay. This is the
  agent-facing complement to the `feedback_polling_loops` rule (no leading-sleep polling).

**2. Terminal subagent-error precedence ([PR 28375](https://github.com/openai/codex/pull/28375)).**
When a subagent exhausted retries it emitted `Error`, but the generic lifecycle then emitted
`TurnComplete(None)`, which *overwrote* `Errored` with `Completed(None)` — so "failed child"
looked identical to "child that finished with no answer," and an unattended root silently
continued. The fix gives terminal-error *precedence* in the status reducer (a closing
`TurnComplete` can no longer erase an immediately-preceding `Errored`), forwards the retained
error to the parent capped at 1,000 model-visible tokens, and attaches a `next_action` hint
pointing at `followup_task`. It stays queue-only (seen at the next sampling boundary).
- **What inber should consider:** in the kanban task-completion-loop dispatcher, ensure a
  child session's terminal-error status takes precedence over its closing "turn complete"
  event so a failed sub-card can't be rolled up as an empty success — and forward a bounded
  error excerpt plus a concrete recovery action (revive/re-task) to the parent rather than a
  null completion. Pairs with goose #9521 below (structured status out of a delegate).

**3. Typed plaintext-header inter-agent envelope ([PR 28368](https://github.com/openai/codex/pull/28368)).**
MAv2 messages now carry a uniform routing envelope — `Message Type: <NEW_TASK | MESSAGE |
FINAL_ANSWER>` / `Task name:` (recipient) / `Sender:` / `Payload:` — so the model always sees
*what kind* of interaction occurred, *who* sent it, and *which* agent it targets in one
shape. The wire trick: no new Responses API field — an encrypted delivery is just a plaintext
`input_text` header item immediately followed by the existing `encrypted_content` item, so
the *routing metadata stays model-visible while the payload stays encrypted*. `NEW_TASK`
starts a turn, `MESSAGE` is a queued send that doesn't, `FINAL_ANSWER` is the terminal
child→parent result (also used for errored/shutdown/missing agents).
- **What inber should consider:** give inber's inter-agent/sub-agent messages a uniform typed
  envelope (message-type + sender + recipient/task path + payload) so a receiver branches
  deterministically on interaction kind, and keep the routing header model-visible even when
  the payload is opaque/encrypted — one self-describing shape across spawn, mid-flight
  message, and final answer instead of distinct ad-hoc notification formats.

## Harness-watch — 2026-06-18: environment is part of the approval cache key + an explicit approval-mode precedence ladder

Two Codex changes finish wiring the *execution environment* into the permission layer (the
2026-06-03 entry keyed remembered grants by environment id; these extend it to the
**command-approval cache** and the **default approval mode**).

**1. Command-approval cache keyed by environment id ([PR 28738](https://github.com/openai/codex/pull/28738)).**
The "sticky" approval cache for `shell` / `unified-exec` previously keyed on `(command, cwd)`
only — so `echo ok` approved in local `/workspace` was silently reused for `echo ok` in an
*executor's* `/workspace`. The fix adds the selected environment id to the cache key, carries
it through the approval request so the client can show *which* environment is being approved
(surfaced as a required-nullable `environmentId` in the inline TUI prompt), and keeps older
approval events compatible when the field is absent.
- **What inber should consider:** wherever inber caches a remembered command approval (the
  prehook's allow/deny memory), the key must be `(environment, command, resource)` — never
  `(command, cwd)` alone — or an approval granted against a throwaway sandbox/worktree
  authorizes the same command against the real tree. Show the environment in the approval
  prompt so the human knows which tree they're authorizing. This is the command-level
  counterpart to the 06-03 sticky-grant rule.

**2. App-level default approval mode with a documented precedence ladder ([PR 27965](https://github.com/openai/codex/pull/27965)).**
Adds `[apps._default] default_tools_approval_mode` and pins an *explicit* resolution order:
**managed (org policy) → per-tool `approval_mode` → per-app default → `apps._default` default
→ built-in `auto` fallback**. The value is exposed through `config/read` so clients can show
the effective mode.
- **What inber should consider:** inber's approval posture is resolved across several layers
  (org/managed allowlist per 06-06, per-tool prehook rules, session/profile defaults). Make the
  precedence a single documented, *queryable* ladder — most-specific binding wins, with one
  named final fallback — rather than implicit ordering scattered across the prehook. Expose the
  *effective* mode for a given tool so it's observable at session start, not inferred.

## Harness-watch — 2026-06-19: volatile context as compaction-surviving reminders + a shared multi-thread token ledger

A coherent Codex stack this window separates *stable* context (injected once, cache-safe)
from *volatile* context (re-derived per turn / re-stated after compaction), and builds a
general reminder mechanism for the volatile half.

**1. Turn-scoped vs thread-scoped context contributions ([PR 28911](https://github.com/openai/codex/pull/28911)).**
The single `ContextContributor` trait is split into thread-scoped (assembled once for the
conversation) and turn-scoped (assembled from turn-local state at each request) contribution
methods. The cacheable, slow-changing fragments stay in the stable prefix; only the volatile
fragments get re-assembled per turn — a structural separation that keeps the prompt prefix
cache-stable while still letting extensions inject fresh per-turn state.

**2. Reminders that are force-refreshed after every compaction (current-time: [PR 28822](https://github.com/openai/codex/pull/28822) + [28824](https://github.com/openai/codex/pull/28824); rollout budget: [PR 28746](https://github.com/openai/codex/pull/28746) + [28494](https://github.com/openai/codex/pull/28494)).**
Two unrelated features land on the *same* delivery shape: a **developer message recorded into
history immediately before a due model request**, on a configurable cadence
(`reminder_interval_model_requests` for the clock; `reminder_interval_tokens` for the budget),
with one explicit invariant — **the reminder is always re-stated immediately after a compaction
event**, because compaction destroys the prior copy and the model would otherwise silently lose
the time / remaining-budget state. This is push, not pull: the engine restates volatile state
the model can't recompute, rather than exposing a tool the model must remember to call (contrast
the 06-09 *pull* `context_remaining` tool).

**3. A shared rollout token ledger across ALL threads under one rollout ([PR 28746](https://github.com/openai/codex/pull/28746) + [28494](https://github.com/openai/codex/pull/28494)).**
Distinct from the existing per-thread `token_budget`: `rollout_budget` is one ledger every
subagent thread debits from when it samples, with **separately weighted sampling vs prefill
tokens** (`sampling_token_weight` 1.0, `prefill_token_weight` 0.1 by default) so cache-read
prefill counts at a fraction of fresh generation. Lives in shared `AgentControl` state; crossing
a threshold appends the model-visible "you have N weighted tokens left in the shared session"
reminder. Pairs with the Token Budgets paper below (`docs/papers/2026-06-harness-research.md`,
06-19 sweep) — that paper catalogs the failure modes a shared budget hits when accounting isn't
ownership-typed.

**What inber should consider:**
- For state the model can't recompute and that compaction silently drops — remaining shared
  budget, current wall-clock time, an active goal/deadline — adopt the **restate-after-compaction
  invariant**: the compaction step should re-emit those reminders into the fresh context, not just
  the summary. inber's summarization path currently rebuilds a digest; it should also carry a small
  set of *live volatile fragments* re-derived at restate time, not summarized from stale copies.
- Mirror the **thread-scoped / turn-scoped split** in inber's context assembly so the cache-stable
  prefix (system prompt, skills catalog, repo bundle) never moves while volatile fragments (time,
  budget, goal) are layered in per turn — the cache-continuity discipline from the TokenPilot entry
  (06-18) applied at the contributor boundary.
- inber's Workflow already has a **shared token pool across the main loop + all sub-workflows**
  (`budget.spent()` is cross-agent). Borrow the **prefill/sampling weighting** so cache-read tokens
  don't burn the budget at full rate, and push a threshold reminder into long autoworker/scoper fleets
  rather than only hard-failing at the ceiling.


## Harness-watch — 2026-06-20: delegation policy as mutable per-turn state (not static prompt) + the description cap belongs at the render edge, not at storage

Two Codex stacks this window both push the same boundary: keep the *durable* artifact
full-fidelity, and shape only what the **model sees** at the rendering edge — for delegation
policy and for skill descriptions respectively.

**1. Multi-agent delegation mode is a per-turn/thread selection injected incrementally, not baked into the system prompt ([PR 28685](https://github.com/openai/codex/pull/28685), [28792](https://github.com/openai/codex/pull/28792)).**
MAv2 previously carried an explicit-request-only delegation rule inside its *static* usage hint —
to flip a session to proactive delegation you'd have to rewrite static guidance and prior context.
The stack makes delegation mode (`explicitRequestOnly` | `proactive`) a **session selection settable
via `turn/start.multiAgentMode` / `thread/start.multiAgentMode`**, with the effective model-visible
mode derived *per turn* (gated default-off behind `features.multi_agent_mode`; unset → `explicitRequestOnly`).
The rule moves **out of the static usage hint into a bounded, tagged developer-context fragment** that is
emitted in initial context and **only re-emitted when the effective mode changes** — historical rollout
items are never rewritten; cold resume restores the latest persisted effective mode. Selected value is
kept distinct from effective value (`null` = no client selection) and persisted in `TurnContextItem` as
the resume baseline. This is the *same* volatile-vs-stable split as the 06-19 reminder work, now applied
to a *policy*: don't bake the orchestration policy into the cached prefix (expensive to change, drifts on
resume) — inject it as an incremental developer message that updates only on transition.

**2. The orchestrator's skill/MCP surface is gated independently from the worker surface ([PR 28942](https://github.com/openai/codex/pull/28942)).**
Orchestrator-provided skills and Codex Apps MCP tools add model-visible instructions/resources/tools
*beyond* the local workspace. `[orchestrator.skills].enabled` and `[orchestrator.mcp].enabled`
(both default `true`, carried in the config lock) let a host disable the **orchestrator-owned** surfaces
without touching regular skills or regular MCP servers — disabling orchestrator skills suppresses the
`skills` namespace and its injected context entirely. The delegating agent's tool surface is a separate,
separately-revocable axis from the executing agent's.

**3. Skill-description cap belongs at the model-visible list boundary, not at load/migration ([PR 29006](https://github.com/openai/codex/pull/29006)).**
A refinement of the 06-04 per-turn skill-catalog entry. Codex previously enforced a 1024-char description
limit *while loading/migrating* skills — which rejected valid skills and discarded metadata non-model
consumers need. The fix: **preserve full `description`/`metadata`/`SKILL.md` on disk**, and cap to
1021 chars + `...` **only when rendering** the implicit available-skills catalog and the on-demand
`skills.list` response. Implicit selection stays two-tier: a bounded catalog (name + capped description +
locator) lets the model make the semantic pick, then `skills.read` pulls the full resource. Same family as
the 06-05 "tool-surface exclusion is presentation-scoped, not capability removal" rule.

**What inber should consider:**
- inber's spawn path passes delegation behavior implicitly via agent-store config + prompt. Make
  **delegation mode a per-session selection with an effective value derived per turn**, and deliver any
  proactive-vs-explicit policy as an **incremental developer/system fragment re-emitted only on change**
  (and re-stated after compaction, per 06-19) — not as static system-prompt text that forces a prefix
  rewrite and silently reverts on resume. Persist the effective mode as the resume baseline.
- Add a config axis that gates the **orchestrator/parent's** own skill + MCP/tool surface separately from
  the worker's. In the kanban task-completion-loop and Workflow fan-outs, the dispatcher/parent often
  needs a *narrower* tool surface than its workers; today they share one allowlist.
- When inber renders its skills catalog into the prompt, **cap at the render boundary, not at ingest** —
  keep full SKILL.md + descriptions in skill-store (other consumers and `skills.read`-equivalents need
  them) and truncate only the model-visible catalog line, with goose-style per-skill token accounting on
  the rendered slice.

## Harness-watch — 2026-06-20: give each context window a stable opaque identity + expose its lineage to the model

Two Codex PRs add a context-identity primitive distinct from the token-budget *accounting*
documented on 06-19. The unit being named is the **context window** — the span of history
between two compaction/`new_context` boundaries.

**1. UUIDv7 window IDs ([PR 28953](https://github.com/openai/codex/pull/28953)).** A context
window was identified only by a thread-local monotonic `window_number`. The PR keeps that
number but adds a **UUIDv7 `window_id`** to `CompactedItem`: a stable opaque identity that
stays fixed for the life of a window and *rotates* when compaction or `new_context` opens the
next one. It's generated/rotated with the auto-compaction state and **reconstructed on resume
and rollback** (legacy records where the numeric id meant the window number are still
accepted). Notably the UUID is used **only in the model-visible token-budget context** —
request headers/metadata keep `thread_id:window_number` — i.e. a stable identity for the
*model's* reasoning, decoupled from the wire identity used for routing.

**2. Window lineage ([PR 29256](https://github.com/openai/codex/pull/29256)).** The rendered
`<token_budget>` fragment now carries `thread_id`, `first_window_id`, `previous_window_id`,
and the current window id — all UUIDv7, all **stable across compaction, resume, and rollback**
(persisted in compacted checkpoints, restored during reconstruction, optional-field-compatible
with older records). The model can now tell "this is the thread's *first* window" from "I am N
compactions deep, and the window just before me was X" — lineage it cannot otherwise recover
once the prior window's text is summarized away.

**What inber should consider:**
- inber tracks compaction as a summarization event but gives the compacted span no durable
  **identity**. Assign each context window a stable opaque id (UUIDv7 or similar) that rotates
  on compaction and survives resume/rollback, and expose `first`/`previous`/`current` window
  ids in the model-visible budget/state fragment — so a long autoworker/scoper session can
  reason about *how deep into compaction it is* and reference a prior window without its text.
  This is the identity layer under the 06-19 restate-after-compaction reminder work.
- Keep the **model-facing identity distinct from the wire identity**: the reasoning id (stable,
  lineage-bearing) need not equal the routing/cache key (`thread_id:window_number`,
  `BridgeSessionID`). This mirrors `reference_harness_session_id_contract` — the id the model
  sees is not automatically the id the transport keys on; conflating them is the bug.
- Lineage ids are a cheap substrate for the kanban task-completion-loop and bundle-store: a
  durable per-window id lets the dispatcher correlate "which compaction window produced this
  artifact / this card update" across resumes without diffing summarized prose.

## Harness-watch — 2026-06-22: offer volatile state as a pull-tool, not only a pushed reminder; tier web access

Two Codex PRs this week, each a *contract* refinement rather than a new mechanism.

**1. Clock as an on-demand tool ([PR 29011](https://github.com/openai/codex/pull/29011)).** The
current-time work documented on 06-19 (#28822/#28824) was *push* — the engine restates the
wall-clock in a compaction-surviving reminder. This adds the *pull* path: a read-only
`clock`/current-time tool the model can **invoke** to get the same UTC string (structured JSON in
Code Mode). It's the exact pull-vs-push split codex already drew for `context_remaining` (06-09):
the same volatile fact is now available both as a fragment the engine restates *and* as a tool the
model calls when it needs to branch on it deterministically.

**2. Graduated web-access tier ([PR 28489](https://github.com/openai/codex/pull/28489); rename-only
[#29095](https://github.com/openai/codex/pull/29095)).** `web_search` gains a third mode, `indexed`,
between `cached` and `live`: queries run live but page *fetches* are restricted to a server-admitted
URL allowlist. One resolved mode is computed once and shared by both the hosted and standalone
executors, so the two surfaces can't drift to different trust levels for the same turn.

**What inber should consider:**
- Where inber injects volatile state as a reminder (time, remaining budget, turn count, current
  card/plan id), also expose a **cheap read-only tool form** of the same fact. A pushed fragment
  forces the model to *react* to whatever was last restated; a pull-tool lets it *decide when* to
  re-check and branch deterministically — and it bridges the gap between compaction restates. Pair,
  don't replace: keep the push for passive awareness, add the pull for control flow.
- Model tool web/network access as a **graduated trust tier** (cached → indexed → live), **resolved
  once per turn and shared by every executor** (tool-store CLI tools, MCP fetchers, the browser
  MCPs), rather than a per-tool boolean. A single resolved tier that every fetch path reads from
  prevents the "search tool is sandboxed but the MCP fetcher isn't" drift, and gives the
  approval/policy layer one axis to gate instead of N.
- Minor but real ([#28260](https://github.com/openai/codex/pull/28260)): codex added a default-on
  `auto_compaction` flag you can **turn off so a run fails loud on context overflow** instead of
  silently compacting. For inber's reproducible/optimization runs (evals, scoper decompositions) a
  silent compaction window corrupts the experiment — offer a per-session "no auto-compaction, error
  on overflow" mode. (Aligns with the host "fail fast and loud" directive.)

## Harness-watch — 2026-06-23: deferring MCP tools behind tool_search becomes the unconditional default (not a scale/flag trigger)

[Codex PR 29486](https://github.com/openai/codex/pull/29486) flips the policy
documented on 06-05 (tool-surface exclusion is presentation-scoped) into a
**default**. Previously MCP tools were only placed behind `tool_search` when a
feature flag was set *or* there were ≥100 installed tools — so the model's tool
flow depended on both rollout config and tool count. Now, whenever the
model/provider supports `tool_search` + namespaced tools, **all** effective MCP
tools are deferred unconditionally; the model never sees them in the first
request — it receives `tool_search`, searches, gets the matching tool, then calls
it on the next turn. Direct exposure is kept only as a fallback for older
model/provider combos that can't search. The `tool_search_always_defer_mcp_tools`
flag and the 100-tool threshold are both **retired**. The design point: deferral
isn't a scale optimization you switch on when the prefix gets big — it's the
correct steady-state shape for *external* tools, so make it the default and
delete the knob.

**What inber should consider:** inber already has the deferred-tool / `tool_search`
machinery (TOOL-ROUTING), but if it still gates deferral on a threshold or an
opt-in flag (the way codex used to), adopt codex's stance: default external/MCP
and tool-store CLI tools to deferred whenever the active model supports
search-then-call, and keep eager exposure only as the can't-search fallback.
Two concrete consequences worth pre-empting: (a) the first turn's prompt should
carry `tool_search` + in-process/native tools only, never the full external
catalog — this is the prefix-budget win, and it's stable per session so it stays
cacheable; (b) any test/eval that assumes a tool is callable on turn 1 must be
reworked to the search→receive→call two-turn flow, or it will silently pass
against the fallback path and never exercise the real one.

**Two smaller identity/control refinements the same week:**
- *Root session id survives cold resume* ([#29327](https://github.com/openai/codex/pull/29327)).
  A cold-resumed subagent kept its durable thread id but could be handed a *new*
  session id, splitting one agent tree across multiple sessions after a restart.
  Codex now persists the **root** session id in every rollout `SessionMeta` and
  restores it before re-initializing the resumed session, so a nested tree
  (`root R → parent P → child C`) still rolls up to `R` after resume; legacy
  rollouts with no `session_id` synthesize it from `id` for back-compat. This
  extends the context-window-identity thread (06-19/06-20): inber's nested trees
  (autoworker, kanban task-completion-loop) need the **tree-root** identity to be
  durable across revive/resume, not just the per-window id — otherwise parent
  rollup and curator closure see a tree fragmented across sessions after a host
  restart.
- *One delegation-mode enum, with a "tools-without-policy-text" option*
  ([#29324](https://github.com/openai/codex/pull/29324)). Three controls that
  could disagree (`multiAgentMode`, `features.multi_agent_mode`,
  `usage_hint_enabled`) collapse into one sticky enum: `none` (multi-agent tools
  available but **no delegation-policy text injected**), `explicitRequestOnly`
  (default), `proactive`. The reusable point for inber: keep "are delegation
  tools present" and "is delegation-policy prose in context" as **independent**
  axes — some sessions want the orchestration surface without paying the
  policy-prompt tokens — and report the concrete resolved mode every turn rather
  than a nullable that clients must re-derive.

## Harness-watch — 2026-06-24: token-budget compaction = fresh-window *reset* (no LLM summarization) + code-mode host-handshake protocol

Two codex changes this window, both about what happens at the boundary where a
turn's context is rebuilt.

**1. Under `Feature::TokenBudget`, compaction is a hard reset to a new context
window — not a summarization round-trip** ([#29743](https://github.com/openai/codex/pull/29743),
followed by [#29762](https://github.com/openai/codex/pull/29762)). The usual
compaction path asks the server (or a local model) to summarize old history and
carries the summary forward. Token-budget compaction instead routes manual
`/compact` and inline auto-compaction through `start_new_context_window`: it drops
all prior user/assistant transcript and tool output and installs a *fresh* window
seeded only with the standard injected context (AGENTS.md, world-state baseline,
the compaction-surviving reminders). No server summarization call is made. It's
still a "compaction" from the client/hook lifecycle's view — pre/post compact hooks
fire and a `ContextCompaction` item is emitted — but observable side effects aside,
nothing of the old conversation survives except what the injected context already
re-states. This is the natural endpoint of the volatile-context-as-reminders thread
(06-19): once your durable state lives in reminders that survive compaction, a
reset is cheaper, deterministic, and avoids the summarizer's lossy paraphrase.

**What inber should consider:** inber's compaction (CONTEXT-MIGRATION / smart-truncation)
assumes summarize-then-continue. Add a **reset-mode compaction** as a selectable
strategy: when a session's durable state is fully captured in compaction-surviving
injected context (plan-file path, world-state baseline, persistent reminders), drop
the transcript and start a fresh window instead of paying a summarization round-trip
— while still firing the same compact hooks / emitting the same `ContextCompaction`
event so the kanban curator and any Stop-time reviewers see an identical lifecycle.
The precondition is the hard part: reset-mode is only safe if nothing
decision-relevant lives *only* in the transcript, so gate it on "is durable state
externalized?" Pairs with *Less Context Better Agents* (2606.10209, last-N+summarize)
and *Beyond Compaction* (budget-eviction) already in `docs/papers/2026-06-harness-research.md`.

**2. Code mode gets a transport-neutral host↔runtime handshake protocol**
([#29515](https://github.com/openai/codex/pull/29515); cluster also renames
`CodeModeService` and drops `Session::is_alive()`). Codex's "code mode" is the
tool-execution-as-code architecture: the model writes code that runs in a sandbox
and calls back to the host for tool execution, instead of emitting one tool-call
block per action. This PR is additive scaffolding — versioned `protocol-version`,
capability negotiation, session-identifier types, and explicit `ClientToHost` /
`HostToClient` JSON envelopes for connect/open/close, with strict unknown-field
rejection and round-trip wire tests; cell, tool-callback, and failure-domain
messages are deferred. The design stance worth noting: code mode is being built as
a **versioned, capability-negotiated wire protocol between two actors** (host and
code runtime), not as an in-process call — so the runtime can be sandboxed/remote
and the host can refuse incompatible versions at handshake.

**What inber should consider:** the empirical case for code mode is real — *From Tool
Orchestration to Code Execution: A Study of MCP Design Choices*
([arXiv:2602.15945](https://arxiv.org/abs/2602.15945)) shows a code-execution MCP
agent (CE-MCP) significantly cuts token usage and execution latency vs per-tool
calls by delegating orchestration/data-shuffling to a sandbox, **but vastly expands
the attack surface** (it catalogs 16 attack classes incl. exception-mediated code
injection and unsafe capability synthesis). So if inber ever lets an agent batch
tool orchestration as sandboxed code (instead of N MCP round-trips), copy codex's
framing: make it a **versioned, capability-negotiated handshake between the host and
an isolated runtime** — never an in-process eval — so the host can pin the protocol
version, scope which tool callbacks the runtime may invoke, and contain the blast
radius. This composes with the MCP `roots` scoping (opencode 06-15) and the
tiered-permission ladder (claude-code 06-02): code mode is exactly the surface where
those containment controls earn their keep.

## Harness-watch — 2026-06-25: batched stale-read rewrite to protect the prefix cache + serializable snapshot/restore as durable state + compaction-knob churn

**1. cline ships the production version of TokenPilot's batch-eviction rule** ([cline #11471](https://github.com/cline/cline/pull/11471)). cline's `MessageBuilder` rewrites stale `read`/`read_files` tool results to `[outdated - see the latest file content]` to reclaim context — the standard outdated-read trick inber also does. Previously it fired *eagerly on every re-read*, mutating bytes in the **middle** of the provider-facing transcript. Any mid-transcript mutation invalidates provider prefix caches (Anthropic `cache_control`, DeepSeek auto-cache, MiniMax) from that message to the end, so the next request re-ingests the entire tail at full uncached price (plus 1.25× cache-write on Anthropic). Because agents re-read constantly (read → edit → re-read to verify), long sessions paid this repeatedly: cline measured ~39% cache-miss on DeepSeek where comparable agents hit ~2%. The fix keeps the rewrite but **batches** it — pending rewrites accumulate and commit in one shot only once reclaimable bytes cross `DEFAULT_MIN_OUTDATED_REWRITE_BYTES = 64KB ≈ 16K tokens` (~2–3 executor-capped reads), amortizing one cache break over a large reclaim.

**What inber should consider:** this is the concrete, shipped instance of rule (2) from the TokenPilot writeup (`docs/papers/2026-06-harness-research.md`: "batch evictions on a turn schedule, never rewrite the cached head"). inber does the same outdated-read rewrite in its compaction path — audit whether it mutates results *in place mid-transcript* on every re-read. If so, adopt cline's exact shape: accumulate pending stale-read rewrites and flush them as a single batch only past a byte/token threshold, so a session re-reading 8 files pays one cache break instead of eight. This is cheap to add and directly attacks the cache-miss line item the ScheduleWakeup cache-TTL economics already make us sensitive to.

**2. Serializable snapshot + restore/revert lands in codex *and* opencode the same week.** codex's `WorldState` (its diff baseline for environment/world facts) was kept as live Rust objects keyed by process-local `TypeId` — unwriteable to a rollout, so on resume it reconstructed an *approximation* from `TurnContextItem`. [codex #29833](https://github.com/openai/codex/pull/29833) (first of a 3-PR stack with [#29835](https://github.com/openai/codex/pull/29835)/[#29837](https://github.com/openai/codex/pull/29837)) requires every section to declare a **stable ID + a serializable snapshot type**, stores only the snapshot in `ContextManager`, and renders diffs by restoring the typed snapshot — so the baseline survives resume, fork, and rollback exactly rather than approximately. Independently, [opencode #33226](https://github.com/sst/opencode/pull/33226) adds a backend-neutral Git service with per-worktree snapshot storage, captures a snapshot at each settled session step, and exposes stateless **revert-preview** + durable **revert-commit** APIs with optional file restoration.

**What inber should consider:** these converge on one principle — *durable agent state must be a serializable snapshot, not reconstructed-on-resume*. This is the missing half of the 06-20 context-window-identity work: inber gives a window a durable id but reconstructs its content approximately. (a) For world/environment baselines, follow codex: stable section id + serializable snapshot persisted into the rollout, restored verbatim on resume/fork/rollback — no lossy reconstruction. (b) For filesystem effects, follow opencode: snapshot the worktree at each settled step and expose a revert-preview/revert-commit pair so the kanban dispatcher (or a human) can roll a failed session's *file changes* back to a known-good step, not just truncate its transcript. Together these make "resume from window N" and "undo step N" first-class instead of best-effort.

**3. codex reverses the compaction opt-out we recommended on 06-24 — and adds a guidance knob instead.** [codex #29815](https://github.com/openai/codex/pull/29815) removes the default-on `auto_compaction` flag entirely: automatic compaction is no longer suppressible with `--disable auto_compaction` (the escape hatch from #28260, which we cited approvingly at 06-24 as "offer a per-session no-auto-compaction error mode"). Manual `/compact` is unchanged. In the same window [codex #29936](https://github.com/openai/codex/pull/29936) adds a configurable `<context_window_guidance>` developer section rendered immediately after `<context_window>` (1000-byte cap) so a deployment can tell the model *how to prepare* for an impending window transition.

**What inber should consider:** soften the 06-24 recommendation. codex concluded that a global "fail loud on overflow instead of compacting" switch was the wrong default and pulled it. The durable need it served — eval/repro runs that must not silently lose context — is better met not by disabling compaction but by **making the boundary observable and steerable**: keep compaction always-on, but (a) pair the read-only `context_remaining` pull-tool (06-20) with a deployment-set guidance string (`<context_window_guidance>`-style) telling the model what to externalize before the reset, and (b) emit the `ContextCompaction` lifecycle event so a repro harness can *detect* a compaction occurred and fail/branch on it explicitly, rather than pre-emptively forbidding it. The control inber wants is "the model and the harness both know the boundary is near and act," not "no boundary allowed."

## Harness-watch — 2026-06-26: volatile-reminder push gains a delivery-mode axis + code-mode host fills the deferred failure domain

**1. codex generalizes the fixed-interval clock reminder into a configurable `delivery_mode` — adding a response-boundary trigger alongside the request-counted cadence.** The 06-19 thread documented codex pushing the current-time reminder on a fixed `reminder_interval_model_requests` cadence (a developer message recorded just before a due request, re-stated after every compaction). This window adds a *second axis*: [codex #30031](https://github.com/openai/codex/pull/30031) introduces a `delivery_mode` config for the current-time reminder, [#30033](https://github.com/openai/codex/pull/30033) implements an `on response boundaries` mode (the reminder fires tied to the turn lifecycle — after each model response — rather than every N requests), and [#30029](https://github.com/openai/codex/pull/30029) lets the interval be set to `0`. So the same volatile fragment can now be refreshed either by a *counted cadence* (every N requests, cheaper, drifts relative to wall-clock work) or at a *lifecycle boundary* (every turn, deterministic w.r.t. the agent's own steps, more cache churn) — chosen per deployment.

**What inber should consider:** when inber adopts the 06-19 restate-after-compaction volatile-reminder mechanism, make the *refresh trigger* a first-class config axis rather than a single hardcoded cadence. Two modes are worth supporting: **request-counted** (refresh every N model requests — minimizes prompt-cache breaks, right for slow-drift state like remaining budget) and **boundary-triggered** (refresh at a turn/response lifecycle boundary — deterministic, right for state the model acts on each turn like an active deadline or current time). inber already runs hooks at turn boundaries, so the boundary-triggered mode is nearly free to wire; just account for the extra cache churn (each refresh mutates a volatile fragment near the tail) and keep the refresh confined to the turn-scoped contributor so the cache-stable prefix never moves.

**2. code-mode host implements the failure domain that 06-24 noted was deferred.** The 06-24 entry documented codex's code-mode host↔runtime handshake as a versioned, capability-negotiated wire protocol, explicitly noting "cell, tool-callback, and **failure-domain** messages are deferred." This window ships them: [#29804](https://github.com/openai/codex/pull/29804) defines the process-host wire protocol, [#30111](https://github.com/openai/codex/pull/30111) implements a *standalone* (out-of-process) code-mode host, [#30108](https://github.com/openai/codex/pull/30108) extends its IPC transport, and [#30110](https://github.com/openai/codex/pull/30110) adds **host-side failure-supervision hooks** — the host now supervises the runtime process lifecycle and handles its failures, rather than treating the runtime as an in-process call that crashes the session with it.

**What inber should consider:** this confirms the 06-24 containment framing with a concrete shape — if inber ever batches tool orchestration as sandboxed code, the host must not only pin the protocol version but **own the runtime process's lifecycle and failure handling** as a supervised, out-of-process child (standalone host + supervision hooks), so a runtime crash, hang, or hostile exit is a contained, observable failure the host recovers from, not a session-killing fault. Pairs with the blast-radius argument from *From Tool Orchestration to Code Execution* ([arXiv:2602.15945](https://arxiv.org/abs/2602.15945)) already cited at 06-24.

## Harness-watch — 2026-06-27: codex `WorldState` matures from serializable diff-baseline into an extensible, availability-gated context-assembly layer

The 06-25 entry documented codex's `WorldState` foundation — every section declares a stable id + a serializable snapshot, persisted into the rollout so the diff baseline survives resume/fork/rollback. This window builds the layer *on top* of that foundation, turning `WorldState` into the general mechanism by which structured context reaches the model:

**1. Extensions contribute named sections; core owns persistence, diffing, and incremental injection** ([codex #30100](https://github.com/openai/codex/pull/30100)). An extension implements `contribute_world_state(...) -> Vec<WorldStateSectionContribution>` — a section id + a JSON snapshot + a renderer. Core compares each section's snapshot against the previous one and emits **either nothing, or one incremental model-visible update** (`ContextualUserFragment`). So skills/runtime stay in their extension instead of moving into core, and the model only ever sees *deltas* — stable sections are never re-rendered turn-over-turn. This is diff-based context injection generalized into a pluggable contributor API.

**2. Sections are availability-projected, cached by their selected root** ([codex #30088](https://github.com/openai/codex/pull/30088)). A section's content (e.g. an executor's skill catalog) is rendered through `WorldState` *only while that environment is ready*: unavailable → hide, ready → discover-once-and-cache, unavailable-again → hide but keep cache, ready-again → reuse cache. Cache key is the full `SelectedCapabilityRoot`; availability toggling never invalidates it (only dropping the thread state does). The model sees a tool/skill section appear and disappear with live capability availability without rescanning stable files on every sample. ([#30225](https://github.com/openai/codex/pull/30225) overlaps the underlying skill-file reads with plugin-namespace discovery to cut the remote-executor latency barrier.)

**3. Resume reconciles the snapshot against retained history before trusting it** ([codex #30152](https://github.com/openai/codex/pull/30152)). Diff-based injection has a subtle desync bug: a persisted `WorldState` snapshot says "section X was already shown," but after compaction the actual model-visible fragment for X may have been dropped from retained history — so the next diff is "unchanged → emit nothing," and the model resumes never having been told X. The fix lets a contribution optionally name the concrete fragment that must remain in history; on resume, *matching fragment present → trust snapshot, emit nothing; missing → treat the section as absent and re-render*. The snapshot is a cache, and the retained transcript is the source of truth it must be reconciled against.

**What inber should consider:** if inber moves context assembly from ad-hoc per-turn string-building toward structured contributors (the natural endpoint of the 06-19/06-26 volatile-reminder work), copy this three-part shape rather than just the diffing: (a) a **contributor API** where each source (memory-store, tool-store availability, agent-store config, active deadline) owns a named section = stable id + serializable snapshot + renderer, with the harness — not the contributor — owning persistence/diff/injection so only deltas hit the context and the cache-stable prefix never moves; (b) **availability projection** so a tool/skill section is rendered only while its backing service/capability is actually reachable, cached by a capability-root key that survives availability flapping (directly relevant to inber's tool-store/MCP provisioning, where a section should vanish when a server is down, not error); and (c) the **resume reconciliation rule** — a persisted "already shown" snapshot must be validated against what's still in retained history after compaction, or the model silently loses sections it believes it has. (c) is the concrete failure inber's own snapshot-on-resume work (06-20 window identity, 06-25 serializable snapshot) is exposed to and has not yet addressed.

## Harness-watch — 2026-06-28: codex + opencode converge on paginated/bounded access to durable session history (a persisted, fail-closed storage contract) + goose disables reasoning on fast-model utility calls

**1. Both codex and opencode replace "replay the whole session" with cursor-paginated reads over the durable event log — independently, same week.** This is distinct from the 06-25 serializable-snapshot work (that was about the *diff baseline*; this is about how the *transcript itself* is read). [codex #29927](https://github.com/openai/codex/pull/29927) adds `history_mode = legacy | paginated` to `Thread`, persisted in `SessionMeta` (JSONL rollout) and a SQLite `thread_metadata` column; paginated threads get fail-closed read behavior and SQLite-backed memory filtering. [codex #30261](https://github.com/openai/codex/pull/30261) makes `history_mode` **immutable after the thread's first canonical `SessionMeta`** — it is "the persisted thread storage contract," and the immutability exists specifically to stop an older binary from appending a `SessionMeta` that omits the field and letting serde silently downgrade a paginated thread back to `legacy`. Independently, [opencode #34097](https://github.com/sst/opencode/pull/34097) adds `GET /api/session/:id/history` — a **bounded finite read** of one session's durable event log, paged from an optional exclusive aggregate-sequence cursor (`after`) with moving-head semantics, returning a `{ data, hasMore }` envelope — while leaving the existing replay-and-tail stream untouched. Same shape both places: history is a durable, append-only log you *page through with a cursor*, not a blob you replay in full; the access mode is a persisted, version-stable contract.

**What inber should consider:** inber resumes/forks sessions by replaying rollout history, which scales linearly with session length in both tokens scanned and prefix-cache exposure. Adopt the cursor-paginated read shape: expose history as a bounded `after`-cursor read over the durable log (opencode's `{data, hasMore}` + last-`seq` cursor is a clean wire form), so resume, fork, and UI scrollback fetch only the slice they need instead of materializing the whole transcript. And if inber ever stores a per-session *history strategy* (e.g. "this session is paginated/SQLite-indexed vs. legacy-flat"), make that mode an **immutable, persisted field written once at session creation** — codex's bug is the general one: any storage contract that a second writer (older binary, concurrent worker, migration) can silently default-downgrade will corrupt sessions that depend on the stricter mode. Pin it at creation, fail closed on mismatch.

**2. goose stops spending reasoning tokens on mechanical fast-model calls.** [goose #9815](https://github.com/block/goose/pull/9815): fast-model utility calls — compaction summaries, session naming, orchestrator routing — were inheriting the global `GOOSE_THINKING_EFFORT` and paying extended-thinking tokens + latency on every call for zero benefit. `ModelConfig::with_fast()` now sets `reasoning = Some(false)`, honored via a `reasoning_disabled()` helper across the anthropic, google, databricks, and openai_responses formats. Concrete gotcha worth stealing: **Gemini 2.5 Flash needs an explicit `thinkingBudget: 0`** — merely omitting the field leaves its dynamic thinking on.

**What inber should consider:** audit inber's own utility-model calls (compaction/summarization, title/slug generation, the kanban classifier and curator, scoper/dispatcher routing) — these are mechanical extractions that gain nothing from extended thinking but silently inherit whatever reasoning effort the default config carries. Explicitly disable reasoning on the fast/utility model path, and note the per-provider quirk: for Anthropic drop the `thinking` block, but for Gemini-class models you must set the budget to `0` rather than omit it, or dynamic thinking stays on. Cheap, pure-win latency/cost reduction on the highest-frequency calls.

## Harness-watch — 2026-06-30: whether to spend context on skills-usage *prose* is a per-model metadata flag, not a hardcoded model-name match

[codex #29740](https://github.com/openai/codex/pull/29740) adds a `include_skills_usage_instructions` field to **model metadata** (false by default; enabled for the bundled `gpt-5.5` entry) and consumes it in both core and extension skill rendering — deleting the old hardcoded legacy-model name matching and its marker plumbing. The "how to use skills at all" preamble (distinct from the per-turn *catalog* of available skills, 06-04) is scaffolding that a capable model already internalizes; injecting it wastes context and cache budget on every turn. The pattern: rather than a brittle `if model_name in {...}` test scattered across renderers, the *capability* ("does this model need skills-usage hand-holding?") is a declarative flag carried by the model record, read at the single render site. This is the same "defer capability knobs to model metadata" stance as the 06-07 Responses-Lite per-model flag — applied here to a pure context-budget decision rather than an execution-locus one.

**What inber should consider:** inber injects skill-usage guidance into the prompt and (per the 06-20 entry) caps skill descriptions at the render edge — but the decision of *whether to inject the usage-instructions preamble at all* is the next knob. Add a boolean to model-store's per-model record (e.g. `skills_usage_instructions: needed|omit`) and gate the preamble injection on it at the render site, defaulting to "omit" for frontier models (Opus/Sonnet 4.x, GPT-5.5+) that don't need it and "needed" only for weaker/older models. Crucially, do *not* re-encode this as a hardcoded model-name allowlist in the renderer — that violates inber's single-source-of-truth rule and rots on every new model; the model record is the canonical place. Same shape applies to any other always-injected scaffolding prose (tool-use etiquette, output-format reminders): make "does this model need it" a model-store field, not a string match.

## Harness-watch — 2026-07-12: memory consolidation is a *background agent with an execution contract* — inherit-and-tighten the parent sandbox, sync workspace roots to the memory root, fail-closed artifact validation with retry

Section 2 of this doc ("Self-describing memory extensions", codex #20602/#20606) covered the *curation-rule* design of codex's memory subsystem — what to write and how. This window ships the complementary piece: the **runtime contract of the consolidation agent itself** — the background job that reads a finished turn and writes durable memory. Three codex commits, one coherent posture: a memory-writer is a subprocess that must be *more* constrained than the turn it summarizes, and its success is defined by validated output, not a clean exit.

**1. The consolidation agent inherits the parent turn's permission profile, then tightens it — it does not get a fresh sandbox** ([codex #32441](https://github.com/openai/codex/pull/32441)). The parent's effective permission profile (including thread-level permission and legacy sandbox overrides) is passed through to the consolidation agent; disabled and externally-enforced profiles are *preserved* rather than swapped for a managed sandbox. When the parent uses Codex-managed permissions, consolidation is further restricted to the memory root with **no network access**. The floor is "at most what the turn could do," and for the managed case, strictly less (memory-root-only, offline).

**2. Sandbox policy is applied through `Config` so workspace roots follow the memory root, not the parent cwd** ([codex #32197](https://github.com/openai/codex/pull/32197)). The consolidation agent `chdir`s to the memory root, but applying its sandbox policy directly to the permissions object left the *workspace roots* still inherited from the parent config — a scope leak where a memory-writer restricted "to the memory root" could still touch the parent's checkout. Routing the policy through `Config` synchronizes workspace roots with the working directory. The lesson: sandbox scope has two independent axes (cwd + writable roots) and moving only one silently widens the grant.

**3. A clean exit is not success — required artifacts are validated, and failure is retryable without a baseline reset** ([codex #32193](https://github.com/openai/codex/pull/32193)). A completed Phase-2 run is marked successful only if `MEMORY.md` is actually a file and `memory_summary.md` starts with the `v1` version marker; a completed run with invalid artifacts **fails without resetting the workspace baseline**, so the job can be retried against the same state. And a "clean workspace" whose required artifacts are invalid triggers a real consolidation run instead of taking the no-change success path — so a corrupt/partial prior write can't masquerade as "nothing to do." Fail-closed on the *output*, not the *exit code*.

**What inber should consider:** inber's `memory-store` (bridge-server :8160, `project_memory_layer_split`) and its consolidation path need the same execution contract, and it's a clean three-part copy. (a) When inber spawns a background memory-write/consolidation pass, derive its sandbox from the *originating session's* effective permission profile and then tighten — memory-root-only, no network by default — rather than running it unconstrained or with a fresh broad grant; a memory writer should never be able to do more than the turn it distills. (b) Constrain *both* the working directory and the writable-roots set together (inber's `forge` worktree slot is the natural root boundary) so "restricted to the memory dir" doesn't leak write access to the session's checkout. (c) Define consolidation success as *validated artifacts* — the expected memory files exist, parse, and carry the current schema/version marker — and on invalid output fail-closed **without** clobbering the prior good state so the pass is retryable; never let a clean subprocess exit or an empty diff count as "consolidated." This complements the memory-*poisoning* defenses in the June papers note ([2606.04329](https://arxiv.org/abs/2606.04329), provenance-tag-at-write + promote-external-origin-only-on-confirmation): that work governs *what content* is trustworthy to consolidate; this governs *how much authority* the consolidator runs with and *what counts as a valid write*. Note the deliberate contrast with the 06-16 entry (a nested review/approval agent must **not** inherit skills or memory): a consolidation agent should not inherit the parent's *context* (skills/memory) either, but it **must** inherit the parent's *sandbox floor* — inheritance of trust context and inheritance of the permission ceiling are separate decisions, and this pair sets them opposite ways on purpose.

## Harness-watch — 2026-07-13: authority has a *principal*, and the principal must live outside the model's reach — consent provenance, an approval precedence ladder, and auto-trusting hook code by origin

Three harnesses shipped patches to the same invariant this window: **an approval is only worth what its source is worth, and the harness must be able to say who granted it.** Prior entries here (06-06 org-managed allowlists, 06-18 approval-mode precedence ladder, 07-12 sandbox inheritance) all answer *what may be done*. None of them answer *who said so* — and that turns out to be the exploitable gap.

**1. Consent cannot be sourced from inside the transcript** ([claude-code 2.1.205](https://github.com/anthropics/claude-code/commits/main/CHANGELOG.md), 2.1.207). Three changelog lines, one theme. Background task notifications "now explicitly state that no human input has occurred, **preventing fabricated in-transcript approvals from being acted on**" — i.e. an agent was reading tool-result text in its own context and treating it as evidence a human approved something. An auto-mode rule now "blocks tampering with session transcript files" — the transcript becomes a *protected resource the agent may not write*, because an agent that can rewrite its own transcript can rewrite its own approval history. And 2.1.207 fixes remote managed settings from a **non-interactive run (`claude -p`, the SDK) being permanently recorded as consented without ever showing the consent dialog** — a headless surface banking a durable consent it never obtained. The unifying rule: *consent must be attributable to a principal outside the model's reach*, and every channel that cannot show a human a dialog must be structurally incapable of recording one. This is the prompt-injection threat model aimed at the **authorization** channel rather than the instruction channel.

**2. Approval authorities have an explicit precedence order, and the decision source is a first-class field** ([codex #32232](https://github.com/openai/codex/pull/32232)). A new `core/src/tools/approvals.rs` centralizes all approval resolution: permission hooks run **first**, and a hook returning `Allow`/`Deny` short-circuits the entire pipeline — **including when `strict_auto_review` is on**. Only if hooks abstain (`None`) does it fall through to the LLM Guardian reviewer, then to the human. The ladder is *deterministic programmable policy → probabilistic reviewer → human*, which redefines "strict auto-review" to mean "no *unreviewed* execution," not "the LLM must re-litigate every call you already codified a rule for." Every path returns one `ApprovalResolution { decision, rejection, source }`, and `source` is stamped into telemetry (`Hook → Config`, `Guardian → AutomatedReviewer`, `User → User`). The decision *source* is data, not a log string.

**3. Executable hook code can be auto-trusted — but only by narrow provenance, with a hash pin, fail-closed** ([codex #32301](https://github.com/openai/codex/pull/32301)). Codex hooks are gated by a TOFU hash pin (`hooks.state.<key>.trusted_hash`). When a remote plugin is materialized, codex auto-writes `trusted_hash` — skipping the human trust prompt — but *only* for plugins satisfying three independent predicates: `scope == Workspace` **AND** `discoverability == Listed` **AND** `authenticated_account_id == current account`. Hook code runs on every tool call, so this is the highest-privilege auto-trust decision in the product, and the guardrails say so: the config write is serialized `Exclusive` on a global queue, the account is re-checked *after* computing the edits and abandoned if it changed, and **a failed write leaves the hooks untrusted** — never trusted-by-default.

**What inber should consider:** inber is maximally exposed on (1) and has no answer at all to (2). The autoworker runs with **unattended auto-allow** (`project_autoworker_leak`) and the herald `ask` channel is explicitly "autonomous-not-interactive so tool prehooks auto-allow" — so inber already has two surfaces that grant permission with no human anywhere, which is precisely the shape 2.1.207 had to patch. Concretely: **(a)** give bridge-server's prehook decision an explicit **`principal` field** (`human | agent | system | unattended-default`) and make it structurally impossible for a non-`human` principal to satisfy a rule that requires human consent — today a parent agent's injected message and a real user's reply are probably indistinguishable by the time they reach the gate, and kanban/dispatcher/herald-injected text is *agent-authored* and must not be able to unblock an `awaiting_permission` session. **(b)** Persist `decision_source ∈ {prehook, reviewer, user, unattended-default}` on the approval event in the rollout — inber currently cannot answer "who approved this tool call?" from its own session record, which is the question you most want answered after an incident. **(c)** Adopt codex's precedence ladder as inber's: the prehook (deterministic, codified) should short-circuit *above* any future LLM review gate, not below it. **(d)** Make session/rollout files **non-writable by the harness's own Edit/Write tools** — inber persists sessions to disk and an agent that can edit its own rollout can forge its own history. **(e)** Audit whether "unattended auto-allow" is recorded anywhere *durable* as consent, or only as a per-run policy; the former is the 2.1.207 bug. And when skill-store/tool-store ingest starts carrying **executable** content (a hook, a skill `scripts/`, an MCP command line — today it is prose/config, so this is a *before it bites* item), copy #32301 wholesale: content-hash pin per artifact stored beside the registry row, auto-trust only under a narrow provenance predicate (repo on inber's allowlist **AND** ingested under the current auth-store account), re-verify the hash on every load so an upstream edit silently revokes trust, and fail closed on any verify/write error. That predicate is the same shape as the existing autoworker approval gate (`project_autoworker_approval_gate`: allowlisted repos auto-dispatch, everything else needs a human) — applied to *code trust* instead of *dispatch*. Related: [ActPlane (2606.25189)](https://arxiv.org/abs/2606.25189) in `docs/papers/2026-07-harness-research.md` is the research half of this — it shows that even a correctly-attributed approval of `Bash(./deploy.sh)` is blind to everything the script then does, because the gate sees a tool call and the damage happens at the syscall.

## Harness-watch — 2026-07-13: compaction is an *invalidatable projection over an append-only canonical history*, not a mutation of it — and the compactor itself needs a token budget

The 06-24 entry framed token-budget compaction as a fresh-window *reset*, and 06-28 covered paging a durable history. This window, cline and dexto independently ship the missing structural claim: **the model's view is a derived projection; the transcript is the source of truth; the two must never be the same object.** inber currently violates this outright, and the violation is load-bearing.

**1. Compaction as a hash-anchored sidecar over an append-only transcript** ([cline #10651](https://github.com/cline/cline/pull/10651), [#11900](https://github.com/cline/cline/pull/11900)). A new `SessionCompactionState` — `{version, source_message_count, source_prefix_hash, source_last_message_key, messages, system_prompt}` — is persisted **atomically as a separate sidecar file**, while `messages.json` stays full-fidelity and append-only. The contract change is the interesting part: **`prepareTurn` becomes request-projection only** — it may change what goes to the provider for one turn, but must not rewrite persisted messages. Staleness is caught by a `sourcePrefixHash()` (SHA-256 over the first *N* messages, fixed field order, normalized user text) plus a message-boundary key; if the canonical prefix no longer hashes to the sidecar's anchor, **the sidecar is discarded rather than silently mis-projected**. Forks re-anchor. Manual `/compact` updates the sidecar *instead of restarting the session*, and #11900 makes the VS Code path persist the sidecar **before** reporting success — previously it showed a compacted state that a later resume silently undid by reloading the full transcript. Reclassifying compaction from a history mutation to a derived, invalidatable view is what makes it reversible, forkable, and re-summarizable at a different budget.

**2. The store owns the summary boundary and reports it** ([dexto #871](https://github.com/truffle-ai/dexto/pull/871)). `ConversationStore` grows `loadModelHistory() → {messages, stats: {returnedMessages, skippedPreSummaryMessages, summaryMessageId}}` alongside raw `listMessages`. "What the model can see" becomes a **first-class store query** rather than a filter the context manager re-derives every turn — so compaction, token estimation, retries, and formatting agree on one boundary *by construction* rather than by convention. (Be skeptical of the perf framing: the current impl still filters in memory. Today the win is the **contract**; the contract is what lets a SQL store later implement it as an indexed `WHERE id >= summary_id`.)

**3. The compactor is a model call that can itself overflow** ([cline #12142](https://github.com/cline/cline/pull/12142)). A new **pure** `buildBudgetProjection(messages, budget, options) → {messages, actions[], warnings[]}` — no IO, no provider calls — becomes the chokepoint before *every* model boundary. Four ideas worth stealing: a **`BudgetPolicyIntent`** (`agentic_summary | basic_compaction_projection | normal_provider_request`) where `dropThinkingBlocks` is `true` for the compaction intents and **`false` for the provider request** — thinking is disposable when feeding a summarizer but must be replayed verbatim to the provider; a **protected live tail** that walks back to the last `tool_use` with no matching `tool_result` and never drops past it; **tool-pair closure** (BFS over `tool_use.id ↔ tool_result.tool_use_id`) so dropping a message drops its whole pair-closure and you can never orphan a `tool_result`; and **warnings instead of silent violation** (`budget_impossible`, `budget_unachievable_with_protections`) when the protections make the budget unreachable. Critically, the summary input is budgeted against **the summarizer model's** window, not the active model's — the load-bearing insight being that *everyone budgets the provider request and almost nobody budgets the summarizer's input, against the summarizer's own often-smaller window.*

**What inber should consider:** inber does the opposite of (1) today, and its own docs already assume otherwise. `engine/lifecycle.go:43` calls `conversation.SummarizeConversation(...)` and assigns the shortened list straight back — **`e.Messages = summarized`** — destroying canonical history in place; `conversation/manage.go`'s `ManageConversation` (dedup-file-refs → stash → prune) likewise mutates the live list. The only survivor is a *lossy text render* (`messagesToText(oldMessages)`, stored `IsLazy: true` in memory-store), not the message structs — so inber **cannot** re-project, re-summarize at a different budget, or fork pre-compaction. This also silently falsifies inber's own `docs/cache-optimization.md:75`, which states as fact: *"Conversation history is append-only — previous messages never mutate."* It does mutate, exactly when compaction fires. Concretely: **(a)** keep the persisted turn log as the append-only canonical store and add a **compaction sidecar anchored by a prefix hash**, discarding it on mismatch; make the summarize/prune path a **projection applied at request-build time in `turn_prompt.go`** rather than a mutation of `e.Messages` — that also hands you a principled answer for where the prompt-cache prefix begins, since the summary head is the natural stable breakpoint (dexto's `stats` payload is the cheap part to copy verbatim: it turns "did compaction actually shrink the model view" into an observable number instead of an inference). **(b)** Extract inber's shrink logic into a **pure projection function** (`messages + budget + intent → messages + action records`) called at *every* model boundary — provider request, summarizer input, and the kanban scoper/classifier calls — and adopt the two invariants inber lacks: never drop a message without its tool-pair closure, and never drop from the live tail past an unresolved `tool_use`. Note that `conversation/repair.go`'s `RepairDanglingToolUse` / `RepairMissingToolResults` are *reactive cleanups for exactly this hazard* — closure-aware dropping makes them unreachable. **(c)** Bound the summarizer's input: `conversation/summarize.go` renders `oldText := messagesToText(oldMessages)` and ships it with **no bound at all**, which is precisely the "compaction overflows the compactor" case — and inber already lets `SummarizeConfig.Model` differ from the active model, so budget it against *that* model's window. Related: [Self-GC (2607.00692)](https://arxiv.org/abs/2607.00692) in `docs/papers/2026-07-harness-research.md` reaches the same conclusion from the research side — pruned content must remain **recoverable by ID in a sidecar**, because prose summaries "hide exact evidence, locators, and editable artifacts."

## Harness-watch — 2026-07-14: a context-selection policy ships in *shadow mode* first — rank, inject nothing, and measure recall against what the model actually reached for

codex is building lexical skill retrieval, and the rollout method is the part worth stealing. [codex #32761](https://github.com/openai/codex/pull/32761) adds an opt-in `skill_search` feature: a bounded weighted-lexical selector ranks every prompt-visible skill against each turn's user input — and then **deliberately throws the ranking away**. The ranked selection is kept *out of model-visible context* and the rendered skill catalog does not change. What it records instead is three metrics: selection **cost**, **catalog reduction** (how much smaller the catalog *would* have been), and — the load-bearing one — whether the model's *subsequent actual* skill invocations (implicit, or an explicit `skills.read`) **landed inside the ranked candidate set**. That last one is a recall measurement of the retrieval policy against ground truth produced by the *un-retrieved* baseline: because the model still sees everything, its real choices tell you exactly what a pruning policy would have broken. [codex #32780](https://github.com/openai/codex/pull/32780) then enables shadow selection by default — still shadow — so the metric accrues fleet-wide before the policy is ever allowed to touch context.

[codex #32768](https://github.com/openai/codex/pull/32768) is the subtle correction, and it generalizes: shadow candidates are restricted to **enabled, prompt-visible skills from host or orchestrator sources**, because executor skills' invocations are *not observable* to the experiment, and including them "can skew the resulting metrics." **You can only measure recall over the subset whose usage you can actually see** — a shadow experiment whose candidate pool is wider than its observability window silently flatters the policy.

**What inber should consider:** this lands directly on work inber has scaffolded but not written. `repo-store` + `bundle-store` (:8306/:8307, `project_session_bundling`) exist to auto-select skills/tools/MCPs per repo+task, and the resolver in llm-bridge-server is **still unbuilt Phase 1** — so the pruning policy does not exist yet and there is no baseline to judge it against. Build it in this order. **(a)** Ship the bundle resolver as a **shadow selector first**: compute the ranked bundle each turn, log it, inject nothing, keep provisioning the full catalog. **(b)** Instrument the **recall** metric, not just the reduction metric — catalog-size reduction is the number that flatters you; "did the model then call a tool the bundle would have pruned" is the number that tells you whether shipping it breaks sessions. inber can measure this today at zero cost, because `engine/build_tools.go:12` `buildTools()` resolves a **static** set (the agent config's fixed list, else `tools.All()`), so every existing session is already the full-catalog baseline a shadow experiment needs, and the `tool_use` blocks in the rollout are the ground truth. **(c)** Scope the candidate pool to what inber can actually observe: in-process and MCP tool calls leave structured traces in the rollout, but a *skill* consumed as prose the model imitates leaves none — those are codex's unobservable "executor skills," and counting them as candidates makes the hit rate meaningless. **(d)** Only once the hit rate is boringly high does the resolver get to prune the real catalog. The general rule, which applies to every future context-trimming knob inber adds (skill selection, tool-store provisioning, memory retrieval): **a policy that removes things from context can only be evaluated against a baseline where nothing was removed — and the only run that produces that baseline is the one where the policy does nothing.**

## Harness-watch — 2026-07-14: the append-only log is truth; the *index* and the *model context* are both bounded, rebuildable projections over it — ordinals, a byte-offset checkpoint, and a fail-safe full-replay fallback

The 06-28 entry documented codex's `history_mode = legacy | paginated` as an immutable persisted storage *contract*. This window ships the machinery underneath it, and it completes the arc the compaction entry above began: cline argued the model's view is a projection over an append-only transcript; codex now says **the database is one too.**

**1. Records carry durable ordinals** ([codex #32332](https://github.com/openai/codex/pull/32332)). Paginated rollout lines get an optional zero-based ordinal; appending and resuming continue from the last valid record (including after gaps or an incomplete tail), and overflow is rejected rather than appended. The stated purpose is exactly the point: *"consumers can process a rollout suffix without rebuilding all earlier history."* Ordering becomes a property of the record, not of your position in a scan.

**2. SQLite is a rebuildable view, never a second source of truth** ([codex #32923](https://github.com/openai/codex/pull/32923)). Rollout records are *projected* into SQLite tables (turns, items, projection progress) **while JSONL stays the source of truth**. Writes, shutdown, and deletion are serialized per thread so a projection update cannot race a cleanup, and projected rows are removed when the thread is deleted.

**3. The projection is checkpointed by byte offset and heals its own suffix** ([codex #32928](https://github.com/openai/codex/pull/32928)). The rationale is stated precisely: *"If a SQLite projection fails after a durable append, the next write must catch up the unprojected suffix instead of skipping it."* Materialization starts at the offset in `thread_history_projection_state`; only **complete newline-terminated** records are projected, leaving a trailing partial line for a later pass; missing files, invalid offsets, and missing or out-of-order ordinals fail **without advancing projection state**. A durable write the index missed is a bug the index must heal, not a row that quietly vanishes.

**4. Model context loads from a bounded *reverse* scan, not a replay** ([codex #32896](https://github.com/openai/codex/pull/32896)). `load_latest_model_context` reverse-scans the JSONL from the end until it finds the newest **safe** bounded suffix — anchored at a compaction checkpoint plus completed-turn metadata — and rebuilds replay-ready context from that alone. Crucially it **falls back to complete history** for legacy/compressed rollouts *and whenever compaction or rollback records make a bounded cutoff unsafe*. The optimization is allowed to decline itself: correctness is the invariant, boundedness is only the fast path.

**What inber should consider:** inber is the worst case on all four counts, and one line proves it — `session/resume.go:36` `LoadMessages(logFile)` opens the session JSONL and `bufio.Scan`s **every line from the top**, unmarshalling all of them, to rebuild the message list on *every* resume. No ordinals, no cursor, no offset, no anchor: resume cost is O(entire session), forever. And `session/checkpoint.go`'s `Checkpoint` is the wrong shape to fix it — it stores `{Summary, KeyFacts, Messages: last-N, TotalTokens}` in a **separate file with no positional link to the JSONL at all**. That is the whole difference in one sentence: **codex's checkpoint is an *index into* the log, so a bounded read can always fall back to the full log; inber's checkpoint is a *substitute for* the log, so it cannot.** A lossy 30-message copy can never be validated against the history it claims to summarize. Concretely: **(a)** stamp a monotonic **ordinal** on every rollout record at append time — the cheap prerequisite for everything else, and it changes no existing read path. **(b)** Re-anchor `Checkpoint` to a **record ordinal + byte offset** instead of a message copy, so resume becomes *reverse-scan back to the newest safe anchor* rather than *replay from zero* — and keep the **full-replay fallback** for codex's exact reason: if a compaction or rollback record crosses the cutoff, the suffix is not provably complete and you must take the slow path. **(c)** When inber indexes sessions into SQLite for search/UI, treat that index as a **projection with its own byte-offset checkpoint that heals its unprojected suffix**, never as a second writer of truth — and project only complete newline-terminated records, because a crash mid-append leaves a partial line, and a projector that trusts it persists a corrupt row no later pass will ever revisit. This is the same claim as the compaction entry above (`e.Messages = summarized` destroys canonical history in place) seen from the storage side, and the two are blocked on one fix: **inber currently has no artifact it can honestly call the canonical log** — which is why neither the compaction projection nor the bounded-suffix read can be built until it does.

## Harness-watch — 2026-07-14: two follow-ons to the authority entry — an auto-approval *floor* that survives delegation, and denials attributed back to the tool call that caused them

The 07-13 entry above argued that authority needs a principal outside the model's reach, and named two things inber has no answer for. Two upstream patches this window supply exactly those mechanisms.

**1. A per-request policy that auto-approval is structurally unable to cross, preserved across delegation** ([dexto #885](https://github.com/truffle-ai/dexto/pull/885)). A top-level `autoApproval` policy on the generic approval contract, whose `disallowed` value is **enforced before the global auto-approve shortcut** and during replay validation — and, critically, **preserved when a sub-agent's approval delegates to a parent runtime**. The stated motivation: hosts need a way to require explicit human consent for *billed, destructive, credential, or egress* actions **even when the session otherwise runs on auto-approval**. Note the shape — this is a **floor, not a rule**. Auto-approve remains the default; the policy is simply a class of action it cannot reach.

**2. A policy-blocked request is resolved back to its owning tool call and kills it** ([codex #32897](https://github.com/openai/codex/pull/32897)). A blocked proxy request's execution ID is resolved to the *registered active network call* before the denial is recorded; the denial then **cancels the owning call**, an outcome already recorded for that call is preserved, and it holds with multiple calls in flight concurrently.

**What inber should consider:** (1) is the right shape for the open `PERMISSION-STORE GATE OPEN` issue, because the problem there was never that unattended auto-allow exists — it is that **nothing sits underneath it**. With both seed deny rules disabled, an autoworker's auto-allow currently reaches `rm -rf /`. A `disallowed` floor is precisely a class of action (destructive, credential-touching, egress, billed) that **no auto-allow path can satisfy and only a human can**, which is a strictly better primitive than re-enabling a deny rule that the next priority-200 allow can shadow. And dexto's delegation clause is the one inber needs most: the autoworker, herald and dispatcher all **spawn sub-sessions**, and a floor a child can shed by delegating upward is not a floor. *(This remains the user's pending decision — recorded here as the upstream shape to adopt, not applied.)* (2) closes the **attribution** half of the gap flagged yesterday via [ActPlane (2606.25189)](https://arxiv.org/abs/2606.25189): the gate approves `Bash(./deploy.sh)` and is then blind to what the script does at the syscall. Codex does not close that in general, but it ensures a denial arising *below* the tool layer is routed back to the tool call that owns it and terminates that call — rather than surfacing as a detached proxy error the agent simply retries in a loop. If inber's prehook ever grows egress policy, copy this: the denial must **cancel the owning tool call by execution ID**, not merely fail the request.
