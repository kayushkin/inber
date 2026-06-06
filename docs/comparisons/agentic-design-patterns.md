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
