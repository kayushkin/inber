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
`conversation/summarize.go` compacts, prefer **shrinking** large tool results (head/tail +
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

## Harness-watch — 2026-07-15: the only legal edit of an append-only thread is a *fork* — editing/retrying an earlier turn branches; an interrupted or in-flight prompt is durable history, not UI state

The 07-14 "log is truth" entry established that the transcript is append-only and the model context is a bounded projection over it. It left one question open: **what happens when the user edits turn N of an immutable log?** Codex answered it this window, as a coherent cluster of small patches, and the answer is *not* "rewind and overwrite."

- **Editing an earlier turn forks a new branch** ([codex #33201](https://github.com/openai/codex/pull/33201), [#33211](https://github.com/openai/codex/pull/33211)): re-submitting an edited earlier prompt branches the conversation rather than truncating history in place, and the pre-edit thread context is preserved on the parent branch.
- **A retried turn runs on a forked thread** ([codex #33207](https://github.com/openai/codex/pull/33207)): a turn that hit a safety buffer is retried against a *fork*, so the retry cannot corrupt the original turn's record — the same fork-not-mutate rule applied to the automatic path.
- **Interrupted and in-flight input is itself history** ([codex #33198](https://github.com/openai/codex/pull/33198), [#33203](https://github.com/openai/codex/pull/33203)): an interrupted prompt stays in conversation history, and a restored thread preserves its in-flight input — the half-typed / cancelled prompt is durable state a resume must reproduce, not transient UI chrome that vanishes on the next keystroke.
- **Session state is separated from session I/O** ([codex #33209](https://github.com/openai/codex/pull/33209)): the precondition for all of the above — the mutable conversation state (which branch is live, what input is pending) is a distinct object from the durable append-only I/O stream, so a fork mutates the former without ever touching the latter.

**What inber should consider:** inber has *no* branch concept anywhere — `grep` across `session/` and `engine/` finds only tool-result truncation. That means the two hazards this cluster fixes are both live: (1) any "edit an earlier message and re-run" UX inber grows will have to **destroy the tail in place** (the same in-place mutation the compaction entry already flagged for `e.Messages`), losing the pre-edit branch permanently and making "compare the two answers" impossible; and (2) an interrupted or half-typed prompt is dropped on interrupt/resume, because `session/resume.go`'s `LoadMessages` rebuilds only *committed* messages from the JSONL — there is no field for pending input, so a resumed session silently forgets what the user was mid-way through asking. Concretely: **(a)** make **fork the only edit primitive** — editing turn N appends a branch marker + the new turn to the append-only log and flips a *live-branch* pointer, never rewrites or drops records; this composes exactly with the 07-14 ordinal/byte-offset checkpoint (a branch is just an anchor into the parent's log) and with the compaction sidecar's existing "forks re-anchor" rule. **(b)** Treat the **retry path the same way** — the kanban dispatcher's revive/retry (`reference_kanban_task_completion_loop`) should fork a failed session's thread, not resume-and-append onto it, so a retried run can never overwrite the failed run's evidence. **(c)** Persist **pending/interrupted input** as a first-class durable field separated from the committed message stream (codex's state-vs-I/O split), so interrupt + resume reproduces the in-flight prompt instead of eating it — relevant to herald/`ask` sessions that are interrupted by design. The unifying rule: *in an append-only history, "undo" and "edit" are spelled **fork**, and the only state a resume may reconstruct beyond the committed log is input that was explicitly persisted as pending.*

## Harness-watch — 2026-07-16: a *reserved* wrap-up budget that only unlocks at the compaction boundary — so "externalize before reset" isn't itself starved for tokens

Codex adds a two-tier auto-compaction limit ([codex #33243](https://github.com/openai/codex/pull/33243) defines the settings, [#33255](https://github.com/openai/codex/pull/33255) implements the behavior). Two new `features.token_budget` fields: `auto_compact_fallback_prompt` (a ≤2000-byte developer message, required non-empty if set) and `auto_compact_fallback_buffer_tokens` (a positive token reserve held back *beyond* the base auto-compaction limit). When the session hits the base limit, instead of rolling over immediately, codex **injects the fallback prompt once as a developer message, keeps the full tool surface available, and unlocks the reserve** — while continuing to *report zero base-window tokens* so the model treats it as a distinct final phase. Rollover happens only when the reserve (or the hard model context) is exhausted; the fallback is skipped if a `new_context` was already explicitly requested.

The idea worth stealing is the **reserved budget**, not the prompt. The 06-24 entry (the read-only `context_remaining` pull-tool + a deployment `<context_window_guidance>` string) tells the model *what* to externalize before a reset — but at the instant the limit is actually hit, the model has *no room left* to act on it: any "write your open state to memory now" step is starved by the same overflow that triggered the compaction. Codex closes that gap by holding tokens in reserve specifically for the wrap-up turn, and by signaling the phase (zero base tokens) so the model knows this is its last chance, not business as usual.

- **What inber should consider:** inber's compaction is purely mechanical at the boundary — `engine/lifecycle.go pruneIfNeeded` does an **emergency flush** once `EstimateTokens > 2×TokenBudget` (line 116) that summarizes/prunes with no turn handed to the model, and `CompactContext` (`engine/engine.go:302`) just runs summarize+prune. Add a fallback reserve: when the estimate crosses a *soft* limit below the emergency threshold, inject a one-shot "you are about to be compacted — persist anything you must keep to memory now" developer message with the tool surface intact, and trigger the mechanical prune only once a small reserved budget above the soft limit is *also* spent. This composes with the existing `SaveSessionSummary`/`MemStore` path (the model's externalized state already has a durable home) and with the 06-24 `context_remaining` pull-tool — the pull-tool tells it the boundary is near, the reserve gives it the room to act. Keep it a soft/hard pair so a single overflow spike still hard-prunes rather than looping forever in the reserve.

## Harness-watch — 2026-07-17: the shadow selector is a *bake-off substrate*, not one ranker — codex runs four lexical methods side-by-side, fielded and IDF-weighted, and bets lexical-before-embeddings for skill routing

The 07-14 entry established the rollout method — rank in shadow, inject nothing, measure recall. This window codex answered the question that entry left open: *once you are in shadow mode, which ranking algorithm do you compare?* The answer is **several at once**, each emitting its own per-method metric inside the same shadow experiment, so the winner is chosen from fleet data before anything touches context.

- **Fielded BM25** ([codex #33605](https://github.com/openai/codex/pull/33605)): ranks each skill across three distinct fields — `name`, `short-description`, `description` — weighting the name most heavily and rare terms more highly (IDF), with deterministic tie-breaking. A skill is not a bag of words; its structure is load-bearing.
- **Character n-gram** ([codex #33613](https://github.com/openai/codex/pull/33613)): scores the same fields on char n-grams with field weights + IDF. The unit-test list names the point — *related word forms, typos, CJK text without word boundaries*. This buys the morphology/typo/no-tokenizer robustness that is usually the argument *for* embeddings, without an embedding pipeline.
- **Multi-query lexical** ([codex #33614](https://github.com/openai/codex/pull/33614)): splits a compound query into sentence- and connector-delimited **views**, ranks each, then merges candidates by best-across-views rank (tie-break: full-query rank, view coverage, stable id). A coding task is almost always multi-intent ("refactor the auth handler *and* add a test"); a single-query ranker underweights skills relevant to only one clause.
- All three run **alongside** the original weighted-lexical selector, and every method **bounds** query/document/candidate/result size and **reports truncation** through the selection metadata — a handicapped ranker signals that it was handicapped so its recall number is not silently penalized.

The through-line: codex is betting **lexical before embeddings** for skill routing — transparent, cheap, no index to maintain, deterministic, and (via char n-grams + multi-query) it narrows the semantic gap that would otherwise be the reason to reach for vectors. The retrieval literature is arriving at the same place from the other side: a recent line of work argues a *skill is not a document* — routing is query-conditional and lexical/hybrid retrieval remains competitive-to-winning against dense embeddings for tool/skill selection ([arXiv 2606.03565](https://arxiv.org/abs/2606.03565); cf. the BM25-strengths framing in the "grep is all you need" agentic-search work). *(Paper specifics unverified — WebFetch is denied in this job's sandbox; cited as corroborating direction, not as a measured result.)*

**What inber should consider:** this refines the 07-14 build order for the `bundle-store`/`repo-store` resolver (`project_session_bundling`, still unbuilt Phase 1). **(a)** Make the shadow harness **pluralized from day one** — a `Selector` interface behind a registry, each computing a ranking and emitting its own recall metric per turn, not one hardcoded ranker. The A/B *is* the harness; picking the algorithm up front is the mistake. **(b)** Rank SKILL.md **by field, not flat text** — inber's skill-store already stores name/description/body as distinct columns, so fielded BM25 with the name weighted highest and IDF over rare terms is essentially free and strictly better than concatenating them. **(c)** Start **lexical, not vector**: do *not* point memory-store's embedding index at skill selection — it is the wrong tool (index maintenance, opacity, and the literature says lexical/hybrid competes). Ship BM25 + char-n-gram first; if a semantic gap survives, add a dense or hybrid-RRF method as *another entry in the same bake-off*, judged by the same recall metric, never as an unmeasured default. **(d)** Decompose the **compound bundle request** (repo + task is almost always multi-intent) into clause-views before ranking, and merge by best-across-views rank. **(e)** Bound every ranker and **surface the truncation signal** into the metric, so a ranker that hit a candidate cap is not misread as a worse ranker. The unifying rule extends the 07-14 one: *a context-selection policy ships in shadow — and the shadow is plural, because the thing you are actually measuring is which selection algorithm to trust, and that is a comparison, not a guess.*

## Harness-watch — 2026-07-18: policy and capability become *diffed, model-visible context sections keyed by content* — codex folds permission & mode instructions into `WorldState`, and gates retained multimodal history on the destination model's declared input modalities

The 06-27 entry turned codex's `WorldState` into the general mechanism by which structured context reaches the model — extension-contributed named sections, core-owned diff/incremental injection, resume reconciliation against retained history. The 06-30 entry moved a context-budget decision (inject skills-usage prose?) out of a hardcoded model-name match and into a **model-record capability flag**. This window ships both of those ideas applied to two surfaces that were previously re-sent verbatim or assumed: the **authorization/mode prose** the agent is told to obey, and the **modality** of the history it is shown. The through-line: what reaches the model is *computed from a declared source of truth* — a content hash of the rendered instruction, or the destination model's declared modalities — not re-injected as static prose each turn, and not assumed to be readable.

**1. Permission and collaboration-mode instructions become `WorldState` sections, re-emitted only on model-visible change and reconciled against retained history** ([codex #33944](https://github.com/openai/codex/pull/33944) permissions, [#33876](https://github.com/openai/codex/pull/33876) collaboration mode). Each is modeled as a `WorldState` section keyed by a **stable hash of its rendered developer message** (CRLF normalized before hashing, so equivalent content produces the same snapshot). Core re-emits the permission/mode context *only* when its model-visible contents change or the retained fragment went missing — and it **avoids duplicates when matching instructions already exist in history, including bundled developer messages**. For collaboration mode the persisted snapshot **is the active mode**: instruction-text edits *within the same mode* are deliberately ignored (no re-emit), and the persisted instructions are **restored when missing from retained history, including after a fork**. This is the exact 06-27 contributor/diff/reconcile machinery, now pointed at the two prose surfaces that most directly shape agent behavior — so a permission-policy change reaches the model as a single incremental delta with provenance, never as an unversioned block re-pasted every turn (which would both bust the prefix cache and give the model no signal that the policy *changed*).

**2. Retained multimodal history is projected to what the destination model can actually read** ([codex #33982](https://github.com/openai/codex/pull/33982)). `audio` joins the model's declared **input modalities** in the protocol + generated app-server schemas; history normalization then **preserves audio in prompts for models that advertise audio input and replaces historical audio with an omission marker for models that do not**. The integration test pins the load-bearing case: switching *mid-session* from a multimodal model to a text-only one **strips prior image and audio content**. This is the 06-30 stance ("capability lives in the model record, read at the single render site") applied to history assembly, and it is the missing server-side half of this-morning's dexto media entry (`dexto.md` 2026-07-15): dexto keeps provider-readable media out of the process heap; codex decides *whether the part is even legal for this model* from the model's declared modality set. The two compose — gate on modality first (emit a metadata-only omission marker, never touch the bytes), resolve to a URL only when the part survives the gate.

**What inber should consider:** both halves land on stores inber already owns, and (2) is a latent correctness bug the moment inber grows any multimodal input. **(a)** Treat `permission-store` instruction text and any collaboration/mode prose as a **content-hash-keyed context section**, not a per-turn re-injection: emit it to the model only when the rendered text's hash changes, dedupe it against what's already in retained history, and restore-if-missing on resume/fork — the same 06-27 contributor shape this doc already recommends for memory-store/tool-store sections. This also gives the open `PERMISSION-STORE GATE OPEN` and 07-13 `decision_source`/principal work a *rendering* contract to match their enforcement contract: when the policy changes, the model sees a single labeled delta with provenance, rather than silently obeying a block it can't tell was edited. **(b)** Add **input-modality capability fields to the model-store `Model` record** — today `store.go:25` carries only `MaxTokens`/costs/priority/`Enabled`, has *no* modality or vision/audio field at all, and `sync.go:162` actively *excludes* audio/vision model variants at ingest, so inber currently has no way to express "this model accepts images." **(c)** Once the field exists, make the conversation assembly **project retained parts to the destination model's declared modalities**, replacing unsupported parts with a metadata-only omission marker rather than shipping bytes the provider will reject or silently drop. This is a real hazard for inber specifically because a session's model is *not fixed*: `agent/clients.go` resolves a per-session `*modelstore.Model` and `conversation/summarize.go:76` lets the summarizer model differ from the active one — so a session that accumulates image/document parts under a vision model and then switches to a text-only utility model (or summarizer) will send parts that model can't read. Gate at the **provider-bridge edge** (llm-bridge-`anthropic`/`-openai`/`-google` `build.go`, and inber's own `msg`→SDK assembly), keyed off the model-store capability record — presentation at the edge, capability as data, never a hardcoded model-name check (the exact single-source-of-truth failure 06-30 deleted upstream). Until inber accepts multimodal input at all this is a *before-it-bites* item, but the model-store field is the cheap prerequisite and has independent value (the UI can stop offering image upload against a text-only model).

## Harness-watch — 2026-07-22: compaction converges on *typed summary + graceful fallback*, and the load-bearing knob is *when* it fires, not *how* it summarizes — cline defaults to model-summary-with-truncation-fallback, goose makes the summary a schema'd template artifact, and SelfCompact shows the trigger is where the wins are

Two harnesses shipped compaction changes in the same window, from opposite ends, and both add a **fallback safety net so a compaction failure never bricks the turn** — the same pattern inber already half-has and should finish. A June paper (revised this window) then names the part both harnesses leave implicit: the summarizer prompt barely matters; *when you fire it* is where the token savings and quality live.

- **cline defaults to *agentic* compaction, with truncation as the fallback** ([cline #12317](https://github.com/cline/cline/pull/12317)). The default context strategy flips from `basic` (local truncation — drop old messages) to `agentic` (ask the model to summarize everything before a safe boundary and replace the removed span with one summary message). Two details worth stealing: (1) **assistant messages are as safe a boundary as user turns**, because tool_use stays paired with its tool_result — so the compactor can cut at more points without splitting a tool call from its output; (2) agentic compaction **falls back to basic truncation if the summary provider request fails** (cancellation still propagates, so a user-abort isn't silently retried). The default is agentic; `--compaction basic` is a kept override; API-key lookup only happens when an agentic strategy is actually selected.

- **goose makes the compaction summary a *typed, template-rendered* artifact, not a prose blob** ([goose #10471](https://github.com/block/goose/pull/10471)). The summarizer is asked for an `<analysis>` scratchpad followed by a JSON block matching a `StructuredSummary` schema of nine sections (user intent, technical concepts, files & key code, errors & fixes, problem-solving, user messages, pending tasks, current work, next steps), **each ordered most-important-first**. The parsed JSON is then **rendered to markdown by a minijinja template, user-overridable at `~/.config/goose/prompts/compaction_summary.md`** — so what survives compaction and its priority ordering is controllable *without* code changes. Parsing is **deliberately lenient** (wrong-shaped fields are stringified, not rejected), and **any parse failure keeps the raw model response verbatim** — the old free-form behavior is the guaranteed floor.

- **SelfCompact: the trigger is the hard part, and a rubric — not fine-tuning — supplies it** ([arXiv 2606.23525](https://arxiv.org/abs/2606.23525), Li et al., submitted 2026-06-22, rev. 2026-07-10). Pairs a model-invocable compaction *tool* with a lightweight *rubric* saying when to fire (a subtask concludes, a trajectory converges) and when to suppress (mid-derivation, or when stuck). The measured finding: **the tool alone is unevenly used** — invoked at unhelpful moments or not at all — **and the rubric alone can't act; together they yield adaptive compaction with no fine-tuning**, up to +18.1 pts on math and +5–9 on agentic search *at 30–70% lower token cost per question*. The one-line thesis: "unprompted models cannot reliably tell when their own context is rotting; a lightweight rubric closes that gap." This is the caution attached to cline's move — handing the model a compact tool (agentic default) *without* a firing rubric is exactly the under-performing "tool alone" arm of this paper.

## Harness-watch — 2026-07-23: a skill catalog that *degrades in a fixed order* under its token budget — codex reserves every skill's name+locator first, truncates descriptions round-robin, drops descriptions before whole entries, and emits a machine-readable report of what it cut

The 07-17 shadow-selector entry was about *which* skills to rank; this window is about *how to render the catalog* once you've decided what belongs in it and it still doesn't fit the metadata budget. codex shipped a five-PR cluster that turns catalog rendering into an explicit graceful-degradation ladder rather than a "truncate the tail" cut:

- **Reserve the minimum line for every skill first, then spend the rest on descriptions round-robin** ([codex #34732](https://github.com/openai/codex/pull/34732)). Each skill's **name + locator** is the minimum line; if those fit for all skills, they are all reserved *before* any description is rendered, and the remaining token/char budget is distributed across descriptions in round-robin order. So a few long descriptions can no longer eat the budget and hide otherwise-usable skills listed after them — breadth of *names* is protected over depth of any one *description*.
- **Drop descriptions entirely before omitting entries** ([codex #34738](https://github.com/openai/codex/pull/34738)). When even the minimum catalog exceeds budget, render included entries **without descriptions at all** — fitting more names+locators before the ladder's last rung (omitting whole entries) is reached.
- **Emit a `SkillRenderReport` even when no fragment fits** ([codex #34785](https://github.com/openai/codex/pull/34785)). Rendering returns total / included / omitted counts plus truncated-description counts and char totals, and **fragment construction is separated from rendering** so the caller can read the report even when the budget was too small to emit anything. Observability of the cut is a first-class output, not a log line.
- **The omission notice itself is policy-scoped** ([codex #34797](https://github.com/openai/codex/pull/34797)): the "N skills omitted" marker is emitted only for the extension-compatible renderer (to match core), while omitted entries stay tracked in the report for both policies even when the core policy emits no fragment at all.

**What inber should consider:** this is the render-time complement to the 07-14 `bundle-store`/`repo-store` resolver and the 07-17 shadow selector — selection decides the candidate set; *this* decides what to do when the chosen set still overflows the prompt budget, which is the case the resolver will hit constantly once skill-store holds more than a handful of SKILL.md entries. **(a)** Give the resolver an explicit **degradation ladder**, not a tail-truncate: reserve `name` + locator for every selected skill first (skill-store already stores these as distinct columns), then distribute the remaining budget across `description` round-robin, then drop descriptions before dropping whole skills — so a model never loses *awareness* of a capability just because an earlier skill had a verbose description. **(b)** Return a **render report** (selected / included / description-truncated / omitted counts) as a structured value the caller can inspect, and surface it — this is exactly inber's own **"No silent caps"** rule (`feedback_bridge_ui_dist_first`'s sibling `feedback` note): a catalog that quietly dropped skills reads to the model as "these are all the skills," and the report is what turns a silent cap into a visible one. **(c)** Keep **fragment construction separate from the budget decision**, so the truncation signal can feed the 07-17 shadow selector's recall metric (a skill the model never invoked because it was budget-omitted is not evidence the *selector* ranked it wrong — the two failure modes must be distinguishable). All of this lives in the still-unbuilt Phase-1 resolver, so it is cheap to design in now rather than retrofit.

**What inber should consider:** inber's compactor is the free-form-prose, threshold-only shape all three of these move past. `conversation/summary_generation.go:20` is a generic "summarize this conversation… bullet points when appropriate" prompt — precisely the "uncontrollable prose blob" goose replaced — and `lifecycle.go`/`summarize.go` trigger it purely on `ShouldSummarize` = `len(messages) > TriggerMessages` (`summarize.go:14`), a mechanical count with no notion of subtask boundary. **(a)** Replace the summary prompt with a **typed schema** (reuse goose's nine sections — they mirror Claude Code's own compaction sections — most-important-first), parse leniently, and on parse failure keep the raw text: inber already has the fallback instinct (`summarize.go:83` drops to `mechanicalSummary` when the *LLM call* fails), so extend that same fail-open discipline to the *parse* step, and render the parsed struct through one template rather than pasting model output into the `[Conversation Summary …]` block at `summarize.go:94`. This makes "what survived compaction" inspectable and lets the rendering live at the edge (the single-source-of-truth rule), instead of being whatever prose the model happened to emit. **(b)** Cut at **assistant boundaries too**, not only user turns — inber's `repair.go` already keeps tool_use/tool_result paired, so the boundary set can safely widen the way cline's did. **(c)** The bigger, cheaper win per SelfCompact: **stop triggering on message-count alone.** A count-threshold fires mid-derivation as readily as at a clean boundary; add a rubric-gated trigger (fire when a turn closed a subtask / the last tool_result resolved an error / the agent stated a next-step; suppress mid-tool-chain) — the paper's result says the *timing* is where the 30–70% token saving comes from, not the summarizer wording. inber can adopt this without fine-tuning: it's a scaffolding rule over signals already in the rollout (`tool_use` spans, error-then-fix pairs, the workflow-phase transitions `engine/turn_prepare.go` already computes). This interlocks with the 07-16 *reserved wrap-up budget* entry (externalize-before-reset needs both a budget **and** a good moment to spend it) and the 07-13 *compaction-as-invalidatable-projection* entry (the typed summary is the projection's payload; the rubric decides when to recompute it).

## Harness-watch — 2026-07-25: a tool call carries a *principal* — codex binds every command execution to the plugin/skill that originated it and propagates that attribution through approval, guardian, and audit; the research line (AuthGraph, MiniScope) says the permission check itself should key on that provenance, not the command string

The 07-13 authority entry established that *consent* must be attributable to a principal outside the model's reach. This window's codex cluster takes the next step: the *tool call itself* now carries which plugin/skill principal spawned it, and that provenance rides through the whole authorization pipeline.

- **Resolve commands against per-turn trusted plugin roots, and reject unattributed/unsafe paths** ([codex #35020](https://github.com/openai/codex/pull/35020)). Shell and unified-exec commands are resolved against the trusted plugin roots loaded for that turn; each execution item gains optional `plugin_id` + a plugin-relative `script_path`. Absolute, escaping, and *unattributed* script paths are rejected — provenance is validated, not merely recorded.
- **Preserve attribution through approval, guardian review, and audit — including declined commands** ([codex #35029](https://github.com/openai/codex/pull/35029)). `plugin_id` / `script_path` are added to execution-approval and guardian-assessment events and propagated through delegated approvals, thread history, and rollout traces, on both started *and declined* items. The audit trail records who originated a call even when it was denied.
- **A portable plugin manifest defines the principal boundary** ([codex #35105](https://github.com/openai/codex/pull/35105)). Codex now reads the Agent Plugins 1.0 `plugin.json` schema (portable metadata + `skills/` + `mcp.json`), with direct-child-only skill discovery that excludes nested skills and any path resolving outside the plugin root. The manifest *is* the trust boundary: what's inside the root is the principal, what escapes it is rejected.

This lands on top of an active research line the same week's paper sweep surfaced, which argues the permission *decision* should key on provenance rather than the command string: **AuthGraph** ([2605.26497](https://arxiv.org/abs/2605.26497), May 2026) builds an authorization graph from clean user intent and an execution graph from actual (possibly injected) data flow, then rejects a tool call whose *parameter sources* deviate — dropping AgentDojo attack success 40%→1%; **MiniScope** ([2512.11147](https://arxiv.org/abs/2512.11147)) and **Progent** ([2504.11703](https://arxiv.org/abs/2504.11703)) enforce least-privilege per-tool-call policy checks *mechanically* rather than by prompting; and the June SoK ([2606.04990](https://arxiv.org/abs/2606.04990)) distills the rule as "scope every tool call to least authority, deny-by-default, and bind capabilities to the task rather than the session." Codex is shipping the plumbing (attribution on the wire) that these enforcement models need as an input.

**What inber should consider:** inber's permission-store gate keys on **command-string regex** — the exact `^curl\b` allow-at-priority-200 the MEMORY note flags as leaving autoworkers able to run almost anything. A regex over the command has no idea *who* asked; a bare `curl` allow can't tell a trusted skill's provisioned call from an injected one. **(a)** Thread an **originating-principal field** (skill-store id / subagent id / MCP-server id, whichever spawned the call) into the PreToolUse hook payload and into permission-store's decision inputs, so an allow/deny rule can be scoped `allow curl *when originated by skill X*` instead of `allow curl always` — this is the missing axis that would let the gate be *tightened* without breaking legitimate provisioned tools, and it directly de-fangs the open-gate landmine. **(b)** Persist that principal on **every** audit-log entry (auth-store already logs decisions), including *denied* calls, so the audit answers "which skill tried the `rm -rf`," not just "a `rm -rf` was denied" — codex preserves attribution on declined items for exactly this reason. **(c)** Treat the skill/plugin **root as the trust boundary** the way the Agent Plugins manifest does: a skill's provisioned tool calls resolve against *its* declared roots (skill-store already stores each SKILL.md's file set), and a call whose target escapes that root is rejected before it reaches the model's requested-command string — provenance-checked, not string-matched. This is the enforcement-side complement to the 07-13 consent-provenance entry (that governs *who may approve*; this governs *on whose behalf a call runs*), and the AuthGraph parameter-source check is the eventual shape of inber's own repair/normalization pass if it ever wants injection-resistance, not just gating.

## Harness-watch — 2026-07-27: codex decouples the *tool-execution substrate* from the session lifecycle — the code-mode host goes remote/networked, MCP runtimes become hot-refreshable per-thread, and deferred-tool namespaces move into world state

The 06-24/06-26 entries tracked codex's code-mode host becoming a standalone, out-of-process runtime with its own wire protocol and failure supervision. This window ships the next axis: that runtime, and the MCP servers beside it, stop being in-process fixtures pinned to session start and become **networked services the session attaches to, refreshes, and survives the loss of**. The through-line is one design move applied to three surfaces: the tool-execution substrate is no longer bound to the session's process lifetime.

- **The code-mode host becomes a remote, network-transparent service.** [codex #35078](https://github.com/openai/codex/pull/35078) adds a WebSocket transport to the code-mode host; [#35098](https://github.com/openai/codex/pull/35098) lets the app-server target a *remote* code-mode host; [#35056](https://github.com/openai/codex/pull/35056) routes exec-server WebSockets through configured proxies; [#35359](https://github.com/openai/codex/pull/35359) handles exec-server **network-policy** requests in the client (the sandbox asks the client for egress decisions); [#35059](https://github.com/openai/codex/pull/35059) decouples the exec-server HTTP layer from `reqwest` types; and [#35266](https://github.com/openai/codex/pull/35266) makes the **in-process host fallback disable-able** — you can force execution onto the remote sandbox. Code execution is now a remote capability with its own network policy, not a subprocess sharing the harness's trust and address space.
- **MCP runtimes become hot-refreshable, per-thread, auth-aware, background-prewarmed.** A nine-PR cluster: reconnect MCP servers on explicit refresh ([#35151](https://github.com/openai/codex/pull/35151)); refresh when session **auth changes** ([#35146](https://github.com/openai/codex/pull/35146)); **prewarm** runtime updates in the background ([#35144](https://github.com/openai/codex/pull/35144)); refresh MCP config **independently across threads** ([#35216](https://github.com/openai/codex/pull/35216)) and refresh managed requirements for **active** threads ([#35213](https://github.com/openai/codex/pull/35213)); use the *current* MCP authority for elicitation reviews ([#35205](https://github.com/openai/codex/pull/35205)); route MCP auth discovery through runtime HTTP clients ([#35239](https://github.com/openai/codex/pull/35239)); all coordinated behind an `encapsulate`d refresh mechanism ([#35164](https://github.com/openai/codex/pull/35164)). The stance: an MCP server's connection, its auth, and its tool list can all change *mid-session* without restarting the session — the runtime is reconciled live, not frozen at startup.
- **Deferred-tool namespaces move into `WorldState`, and `tool_search` dedupes sources.** [#35063](https://github.com/openai/codex/pull/35063) tracks deferred-tool namespaces in the world state (the same structured-context mechanism the 06-27/07-18 entries documented), and [#35065](https://github.com/openai/codex/pull/35065) stops `tool_search` returning the same deferred source twice — so which tools are deferred vs. surfaced is derived state that survives refresh/resume, not a per-turn recomputation that can drift or double-count.

**What inber should consider:** this is the counter-design to inber's own documented failure mode — Claude Code (and inber's `tools/mcp`) start every stdio MCP server once at session start and kill it on `Close()`, which is exactly what OOM'd the box (`project_mcp_descoping`: ~890MB/session). inber's `tools/mcp/adapter.go` `MCPToolRegistry` is a static map — `AddClient` / `GetAllTools` / `Close`, with **no** `Reconnect`, `Refresh`, or prewarm — and `client.go` spawns the subprocess once in `NewClient`. **(a)** Add a **refresh path** to the registry: reconnect a single named client and re-list its tools without tearing down the session, so an MCP server whose auth was just provisioned in auth-store (or whose config changed in tool-store) comes live mid-session — today the only way to pick up a new MCP credential is a full session restart, which is the thing `feedback_never_restart_gateway_unattended` says not to do. **(b)** Make MCP attach **lazy + prewarmed**, not eager-at-start: keep servers deferred (they already fit the deferred-tool/`tool_search` layering this doc's 06-16 entry describes) and spawn the subprocess in the background on first reference, so the 890MB isn't paid by sessions that never call the tool — this is the memory fix `project_mcp_descoping` reached for by *removing* the browser MCPs from scope, made structural instead of by-hand. **(c)** For code execution specifically, codex's remote-host-with-client-side-network-policy is the shape inber's `bridge-agent --mcp browser` escalation wants: run the heavy/untrusted runtime out-of-process (or off-box) behind a transport, with the harness holding the egress/network-policy decision, rather than in the session's own address space and trust domain. **(d)** Track "which tools are deferred vs. surfaced" as **derived state keyed off the tool registry**, not a value recomputed each turn — so it stays correct across the refresh path in (a) and doesn't double-count a source (the exact bug #35065 fixes).

## Harness-watch — 2026-07-28: an *effective* value must survive revival and be reported when it degrades — codex makes subagent instructions a three-state inheritance contract and hands the model catalog the token budget; cline ships the bug that happens when neither holds

Three findings this window share one spine: a configured value is only real if it can be
reproduced when the session is revived, and only trustworthy if a fallback says so.

### 1. Subagent developer instructions: unset inherits, blank clears, role wins — and the effective value must survive forks, compaction and cold resume

[codex #35708](https://github.com/openai/codex/pull/35708) adds
`features.multi_agent_v2.subagent_developer_instructions`, an override applied to subagents
that define **no** role-specific instructions. The contract is three-state, and that is the
design: **unset = inherit the parent's, blank = deliberately clear them, present = replace**,
with role-specific instructions outranking the override. The follow-up
[#35653](https://github.com/openai/codex/pull/35653) is the load-bearing half — it does
nothing but test that whatever the effective instructions are, they are carried **without
duplication** through full forks, bounded forks, compacted histories and cold resume, and that
a roleless worker lazily reloaded after a cold resume still has them. It also pins the inverse:
stale *parent usage hints* are stripped on fork while developer messages are kept.

**What inber should consider:** inber sits in the one corner of that matrix codex kept as a
non-default. `spawn_agent` (`agent/registry/spawn_tool.go:90`) takes exactly `{agent, task}`;
the child's instructions come wholly from its agent-store registry entry, so the contract is
*always role-only, never inherit*, with no way to express the other two states. That is fine
until the parent holds session-specific standing rules the registry entry cannot know — a repo
convention discovered this session, an approval constraint, the hold-gate policy — at which
point the only channel is to paste them into `task`, where they read as part of the job rather
than as instructions. **(a)** Add an optional `instructions` field with codex's three states,
so *inherit* becomes a choice rather than an accident of which registry column was filled.
**(b)** The half to actually build first is #35653's, not #35708's: whatever the child's
effective instructions are, they must be reproduced when the child is **revived**. inber's
kanban task-completion-loop dispatcher revives sessions, and a revived child that silently
drops its inherited constraints is the same bug class as cline's sidecar below — persisted
promise, absent read. **(c)** Copy the strip-hints detail: inherited *instructions* are wanted,
an inherited *"you have used N tokens"* is a lie to a fresh child.

Two smaller ones worth copying as-is. [#35661](https://github.com/openai/codex/pull/35661)
reorders the rendered developer message to put `host_skills` **before** the permissions
section — capabilities first, the constraints on using them last and nearest the task.
[#35594](https://github.com/openai/codex/pull/35594) changes only a *schema description*:
`wait_agent`'s timeout now recommends minute-scale waits "that avoid busy polling." A tool's
description is a cost control, and a one-word edit there is cheaper than any orchestration
logic — inber's own no-polling rule currently lives in a MEMORY note rather than in the
schema text the model actually reads.

### 2. The model catalog owns the default token budget; explicit config wins; the resolved value is frozen in the lock

[codex #35608](https://github.com/openai/codex/pull/35608) moves token-budget defaults into
the **model catalog**: catalog messages carry token-budget settings, applied when no explicit
configuration exists. Four rules come with it — explicit user settings stay authoritative,
invalid catalog values are **rejected** rather than clamped, the *resolved* defaults are
preserved in the exported **config lock** so replay is deterministic, and context-window
guidance is managed through world state so it updates **once** when the active model changes
instead of being rebuilt every turn.

**What inber should consider:** inber has the catalog codex is adding fields to — model-store
(:8155) — and does not use it for this. `agent/models.go:27` returns
`ContextWindow: 200000` for **every** model, including ones it just found in model-store,
under the comment "reasonable default since not in model-store"; `engine/build.go:107` then
derives the prune threshold as `contextWindow / 2`. So inber prunes at a hardcoded 100k
whatever model is live, while a second unrelated constant — `TokenBudget: 50000`, repeated in
all four role configs in `conversation/manage_config.go` — drives `ShouldPrune` on the other
path. **(a)** Add `context_window` to model-store and read it in `GetModelInfo`, deleting the
literal. **(b)** Keep codex's precedence exactly: explicit engine config beats catalog, and an
invalid catalog value is rejected, not silently clamped — a clamp is how a bad sync becomes a
mystery. **(c)** Freeze the *resolved* value on the session at start, codex's config-lock move.
inber resumes sessions and revives them from kanban; an `ms sync` between the original run and
the resume would otherwise move the compaction threshold under a conversation already in
flight — same-session-different-rules, the hazard the datetimeoffset note is a special case of.
**(d)** The world-state detail is a cache point: render context-window guidance once on model
change, not per turn, so it stays in the stable prefix `BuildSystemPrompt` already orders for
caching (`engine/turn_prompt.go:66-72`).

### 3. A fallback that reports success is a lie; a write gated differently from its read is dead state

[cline #12563](https://github.com/cline/cline/pull/12563) fixes two bugs found by manually
driving the compaction UI shipped a week earlier — both made the "Context compacted" divider
lie. **First:** the agentic summarizer built its handler from a provider config that lost the
base URL under the SDK provider spelling, hit the default endpoint, got a 401, logged
"Agentic compaction failed; falling back to basic compaction" — and rendered the *identical*
success divider. Every OpenAI-compatible custom-endpoint setup had silently never used agentic
compaction. **Second:** a manual `/compact` persisted a compacted working context promising
"the next turn and resumes keep using it," but the **read** path was wired only when
`compaction.enabled === true`, which defaults false. State was written and never read; every
later request re-sent the full transcript. A test had pinned the incoherent semantics —
persist while disabled, never project — and had to be rewritten. The verification method is
the transferable part: a logging proxy tagging summarizer requests by their system prompt, so
"agentic ran" is proven on the wire rather than by the UI.

**What inber should consider:** inber had the first bug, worse. `conversation/summarize.go`
dropped to `mechanicalSummary` — a list of words longer than six letters harvested from the
transcript — whenever the summarization call failed, and **swallowed the error entirely**: no
log line, no field on `SummarizeResult`, and `result.Summarized = true` set identically either
way, under a `[Conversation Summary — N earlier turns condensed]` header that claims a summary
in both cases. **Fixed in this commit** (`SummaryDegraded` + `SummaryError`, and a warning on
the fallback). The rule to keep: a fallback on a *quality* path must be visible in the returned
value, not merely in a log — and in inber's case it was not even in a log, which is the
"0 `[reaper]` lines ever" failure discovered the same way. Two follow-ons: **(a)** consider
labelling the injected block itself when degraded, so the model is told it is reading a keyword
list and not a summary — that changes prompt content, so it is left as a decision rather than
done here. **(b)** Audit for cline's *second* shape, which inber has not been checked for: a
write gated on one condition whose read is gated on another. The generalizable check is cheap —
for every persisted artifact, name the read that consumes it and confirm the two gates are the
same expression. The compaction memory save (`summarize.go:55`, gated on
`cfg.SaveToMemory && memStore != nil`) and its re-admission through lazy load / `memory_expand`
are the first pair to check.

**Cross-cutting:** all three are one rule. codex #35653 tests that inherited instructions
survive a cold resume; codex #35608 freezes the resolved budget so a resumed session keeps the
rules it started under; cline #12563 is what the product looks like when neither holds — a
promise persisted, a read that never fires, and a success message covering for both.

## Harness-watch — 2026-07-29: a *record of what was dropped* must be typed, named and unforgeable, and a *gate* must see the bytes that will execute — codex gives truncation a schema the model cannot fake, goose stops reviewing a rendering of a tool call, cline ships an allowlist that fails open

Four upstream changes this week are one argument seen from four sides: every place a harness
*summarizes*, *renders* or *normalizes* something before showing it to a model, a gate or a
future turn, the summary can lie, and the fix is always to make the reduced form typed and
attributable rather than prose.

### 1. Truncation is a typed record, not a string — bounded twice, produced only locally, idempotent

[codex #35738](https://github.com/openai/codex/pull/35738) adds a protocol module for executed
tool-call metadata: `ExecutedToolCall { name, arguments }` where `arguments` is an enum of
`Raw(Value)` or `Truncated(ExecutedToolCallTruncation { original_bytes, max_bytes,
omitted_calls, original_name_bytes })`. Four properties come with it. It is bounded **twice** —
8 KB per call *and* 32 KB per prompt — so no single call and no accumulation of calls can run
away. Overflow degrades into a **typed record that names what was lost**, not a silent drop.
The `Truncated` variant can only be constructed locally: deserialized response items and
model-supplied arguments are prevented from forging truncation markers. And the bounding is
**idempotent**, so re-bounding an already-bounded prompt is a no-op — which is what makes it
safe to re-run against a cached prefix.

**What inber should consider:** inber's equivalent is `summarizeToolResultByContent`
(`conversation/manage_tool_pruning.go:96`) and it fails three of the four properties, one of
them outright. It **sniffs the tool type from the content** rather than reading the tool name
off the paired `tool_use` block, using a chain of `strings.Contains` guesses — output holding
"exit code" is shell, output holding "wrote" is a file write, output with a "/" and >3 lines is
a directory listing. So a `git log` whose message body contains the word *written* is stamped
`[wrote file: N bytes]`, and the transcript now asserts something false about what the agent
did. **The first branch also makes the second unreachable**: the shell branch fires on
`len(lines) > 5`, the file-read branch requires `lineCount > 20`, and `lineCount > 20` implies
`len(lines) > 5` — `[read file: %d lines, %d bytes]` is **dead code that can never be
produced**, and every multi-line file read in a pruned inber transcript is labelled `[shell:
N lines]` instead. **(a)** Carry the tool `name` in the record, as codex does; it is already
on the `tool_use` block two fields away, and every guess disappears with it — this deletes the
dead branch rather than repairing it. **(b)** Keep `original_bytes` unconditionally. inber
keeps a byte count in two branches and loses it in three (`[shell: %d lines]`,
`[listed %d files]`, `[search: %d results]`), so for most truncations the transcript cannot say
how much was dropped — the "No silent caps" rule, violated by the summarizer that exists to
enforce it. **(c)** Add the per-prompt ceiling. `truncateOldToolResults` has a per-item
threshold (>200 bytes) and no aggregate bound, so a thousand small results are never touched.
**(d)** The forgery property matters more here than in codex, because inber's markers are plain
text inside an ordinary text block: a fetched web page or a repo file containing the literal
string `[read file: 3 lines, 91 bytes]` is indistinguishable from a marker inber wrote. A
distinct block type, or at minimum a sentinel the harness owns, restores the distinction.

### 2. The approver must see the bytes that will execute — and normalizing an empty allowlist to "unset" fails open

> **AUDIT RUN 2026-08-03 — the cline half was the live one, and it was live three repos wide.**
> This entry asked for one thing: *audit every place inber collapses an empty collection into a
> nil or absent value on a permission path.* Done, and it found two, in opposite directions.
>
> 🔴 **`disabled_tools` — shipped.** The nil/empty distinction on this field is documented as
> load-bearing at *both* ends (llm-bridge-jig's `handleConfig`, inber's `handleBridgeConfig`,
> each testing for nil rather than length, each with a comment saying why), and the layer between
> them could not express it: `msg.ConfigSessionRequest.DisabledTools` carried `omitempty`, and
> bridge-server's `handleConfigSession` does not forward the caller's bytes — it decodes into
> that struct and **re-marshals**. So `{"disabled_tools":[]}`, the request that re-enables every
> tool, went out as `{}`. jig answered *"config: payload sets nothing"*; inber answered
> *"updated"* and changed nothing. A second copy of the same conflation sat in the budget-only
> fast path (`len(req.DisabledTools) == 0`), which would have dropped a tool change on exactly
> the halted sessions that path exists to revive. This is cline #12669's shape precisely, and
> P185's "two locks on one door" — both gates were already correct; the wire in front of them
> was not. Fixed in llm-bridge, llm-bridge-server, llm-bridge-jig and inber.
>
> 🔴 **`AgentConfig.Tools` — a genuine `[]`-means-everything, and NOT decision-free. Filed.**
> `engine/build_tools.go:17` reads `len(e.AgentConfig.Tools) > 0` and otherwise returns
> `buildDefaultTools()` — every tool inber has. The comment on the field records it as the
> design: `// tool allowlist (empty = all)`. That is the deny-everything configuration enabling
> everything. **It cannot simply be flipped to a nil test**, and the reason is the second half of
> the same lesson: agent-store already destroyed the distinction upstream —
> `agent-store/store.go:547` does `if tools == nil { tools = []string{} }` — and
> `agent_harness_tools` holds **zero rows**, so all 22 agents on this host arrive with a non-nil
> empty slice. A nil test today would hand every agent an empty tool set. See the child todo.
>
> ✅ **Checked and correct, so nobody re-audits them:** `SetDisabledTools`/`applyDisabledTools`
> is a *deny* list, where empty means deny nothing — the fail-safe direction, and documented.
> `guard.CheckTool` under Observe refuses any tool it has not classified. The remaining known
> asymmetry — Assist *allows* an unclassified tool, because `isDangerous` is a denylist — is
> already the open todo `27dd7892`, not a new finding.

[goose #10529](https://github.com/aaif-goose/goose/pull/10529) is filed as a security fix and
is the cleanest statement of the rule: the adversary/reviewer pass was shown a shorthand
`command` summary of a tool call rather than the full argument object, so **sibling arguments
alongside `command` were never inspected** — and the shorthand doubled as an injection surface
into the reviewer's own prompt. The fix serializes the complete argument map as JSON with
newlines and Markdown fence characters escaped inside strings. Two audit findings closed. The
companion failure is [cline #12669](https://github.com/cline/cline/pull/12669), which reverts
a merged skill feature; the reverted PR's review names the reason worth recording — an empty
skill allowlist `skills: []` was **normalized to `undefined`**, which downstream code reads as
*no restriction*, so the deny-everything configuration **enabled every skill**.

**What inber should consider:** these are the two halves of inber's own open permission-store
gate. goose's half: whatever the approver is shown must be the serialized arguments that will
actually run, escaped for the medium they are embedded in. inber's gating keys on a **command
string**, which is already a projection of the call rather than the call — the entry above on
truncation records and the 2026-07-25 entry on tool-call principals both push the same
direction, and this is the third independent arrival at it. cline's half is sharper and
cheaper to check: **audit every place inber collapses an empty collection into a nil or absent
value on a permission path.** In Go this is the `len(x) == 0` versus `x == nil` distinction, and
the two are routinely treated as one — an explicitly empty allowlist and an unset allowlist are
opposite intentions that marshal to nearly the same thing through JSON. The standing rule from
the Claim Plane note applies verbatim: ambiguous authority must fail closed, and `[]` meaning
"everything" is ambiguity resolving to allow.

### 3. On rebuild, the durable copy is *behind* the live one — name which you want

[cline #12622](https://github.com/cline/cline/pull/12622) fixes a session rebuild that read the
persisted transcript, which only catches up at assistant-message and turn boundaries, and is
never flushed by `abort()`. Toggling Plan/Act while the first turn awaited command approval
therefore rebuilt the session with **zero history**, although the resident agent object held
the full conversation. The fix adds `readLiveSessionMessages`, which prefers in-memory messages
and falls back to the transcript, and routes every rebuild coordinator through it. The
generalizable move is not the fallback but the *naming*: a resume path now declares whether it
wants live-preferred or durable-only, instead of inheriting whichever reader was nearest.

**What inber should consider:** inber revives sessions from kanban and resumes them from
scheduler jobs, so it has several rebuild paths and no stated contract about which copy each
one reads. The check is mechanical and matches the one already recommended for gated
writes — for each rebuild entry point, name the reader and say whether an interrupted or
in-flight turn is expected to be present. The interrupt case is where this bites, and inber
interrupts sessions deliberately: the autoworker hold gate stops a live session, and the reaper
kills orphans. Both leave exactly the mid-turn state cline lost.

> **AUDIT RUN 2026-08-03 — the audit was done and the answer is worse than "no stated
> contract": there is no SAFE contract available, because the conversation has no lock at all.**
> All fourteen rebuild and read entry points were walked. The good news first: both revive
> paths do consult the live session before disk (`getOrCreateSession` at
> `server/session_creation.go:24`, `handleBridgeResume` at `server/api_bridge.go:515`), so
> cline #12622's actual bug — rebuilding from the durable copy while a resident object held
> more — is not present here.
>
> 🔴 **What is present is the layer underneath it. `Session.turn` sets `Status = Running`
> under `s.mu` and then RELEASES s.mu before calling `Engine.RunTurn`**
> (`server/session.go:149-175`), so no lock is held for the length of a turn. Every
> cross-goroutine toucher of `Engine.Messages` takes `s.mu` as though it guarded the engine:
> the session list (`session_management.go:49`), `persistSessionState` (`:128`, reached from
> `InterruptSession` at `:77` — the interrupted-turn case exactly), the history endpoint
> (`api_sessions.go:183`), the bridge messages endpoint (`api_bridge.go:834`), and a fork
> marshalling its parent (`session_forking.go:22`). `go test -race` reports all of them.
>
> 🟢 **One of them was a WRITER, not a reader, and that half is FIXED and shipped.**
> `handleBridgeCompact` called `Engine.CompactContext` — which replaces the whole slice —
> from the HTTP goroutine while a turn appended to it. Two writers, so the failure is a lost
> write rather than a stale read, and the half that loses can be the turn's, leaving a
> `tool_use` whose `tool_result` never arrives. It is now refused with 409 while a turn is in
> flight, and its persist moved inside the same hold of `s.mu` (a turn was free to start in the
> gap between unlocking and marshalling). Five sabotages, five caught. Refusing is not a
> lesser fix than serialising: there is no meaningful summary of a turn still being written.
>
> ⛔ **Do NOT try to close the remaining readers with a mutex on `Engine.Messages` — it was
> attempted and reverted, and the reason is the finding.** The conversation is passed to
> `Agent.Run` as `*[]anthropic.MessageParam` (`engine/turn_execute.go:42`) and the `agent`
> package mutates it throughout the tool loop, including in place across the WHOLE history:
> `placeHistoryCacheBreakpoints` (`agent/agent.go:466-484`) rewrites `CacheControl` on content
> blocks reached through pointers, on every API call. So a lock only the engine takes is
> bypassed, a deep copy has to clone every content block rather than the slice, and a lock the
> agent takes has to be held across a streaming model call — which would block the session list
> and the interrupt path for the length of a model response. That is a genuine three-way
> design choice, not a patch, and it is filed as its own todo with the `-race` evidence.
>
> ✅ **Checked and correct, so nobody re-audits them:** the fork/spawn paths write only the
> child's engine before it is reachable (`session_forking.go:67`); `releaseSession` and the
> reaper both refuse to close a `Running` session (`session_release.go:44`,
> `session_reaper.go:69`).

### Also in-window, worth a line each

- [cline #12641](https://github.com/cline/cline/pull/12641) — models emit `insert_line: "3"`
  and strict validation rejected the whole call, burning a round trip. The fix coerces the
  string in the **parser** while leaving the JSON Schema advertised to the provider byte-identical
  as `integer`. Asymmetric tolerance: liberal in what you accept, unchanged in what you
  promise — and because the tool-definition bytes do not move, the cache prefix does not
  either. Cheap and directly applicable to inber's hand-written `InputSchema` maps.
- [codex #35769](https://github.com/openai/codex/pull/35769) +
  [#35773](https://github.com/openai/codex/pull/35773) extend the skill-catalog budget entry of
  2026-07-23. Two catalogs (host and executor) that draw from one pool are now allocated
  **together** under a single budget with an explicit, deterministic eviction order — executor
  entries retained, then total entries, then description length, then cost — and host skills are
  dropped first. #35773 makes the budget a flat 2% of the resolved model context window,
  deleting the 4,000-token absolute cap. The hazard to note if inber copies it: with a shared
  budget, adding one host skill can silently evict an executor skill and invalidate the whole
  stable prefix.
- [opencode #39247 → #39265 → #39373](https://github.com/sst/opencode/pull/39373) — an MCP
  client v2 migration merged with the session-expiry reconnect logic explicitly deferred and one
  session-recovery test left `skip`ped, had recovery re-added at the **transport** layer (on a
  404 for a session-bearing POST: clear the stale session id, reinitialize, replay at most
  once), then was reverted wholesale three weeks later. Two lessons: a remote tool session is a
  **lease**, and its reconnect belongs in the transport rather than in every caller; and the
  skipped test *was* the dropped invariant — `it.skip` is how a migration ships broken.

## Harness-watch — 2026-07-30: a loader that fails must not look like a loader that found nothing — cline ships the instruction pipeline silently returning zero rules, inber has the same shape on its *system prompt*; plus "unknown" is a third value, and an untrusted peer must not choose how long you loop

### 1. Context assembly degrading to empty is indistinguishable from "there was no context"

> **[Verified 2026-08-03 — SPENT. Both inber sites named below were fixed by other work before
> anyone read this entry; do not re-file either.]**
> `BuildSystemPrompt` now returns `([]NamedBlock, error)` and hands the memory failure back —
> `engine/turn_prompt.go`'s doc comment states the rule this entry argues for ("a turn built without
> them is not a degraded turn — it is a different agent answering") and `buildTurnContext` owns what
> to do about it. The milder `session/workspace.go` shape is gone too: `WriteSystem` returns the
> write error rather than `continue`ing past it. The *generalizable* audit in the last two sentences
> is the part still worth running on paths this pass did not walk.

[cline #12702](https://github.com/cline/cline/pull/12702) is the sharpest instance of this class in a
long time, and the payload is one line of behaviour. A legacy single-file `.clinerules` at the
workspace root — the format long-time users still have — sits where the unified config watcher
expects a *directory*, so scanning `.clinerules/skills` throws `ENOTDIR`, and that error aborted the
whole refresh in `discoverRulesLikeFiles`. Consequence: **workspace rules, global rules and
`AGENTS.md` were all silently ignored in every task.** A fresh task answered "NO RULES PRESENT"
while the same setup on the old extension honoured both project and global rules; the only signal
was one log line. The fix treats `ENOTDIR` like `ENOENT`. Its sibling landed the same week —
[cline #12260](https://github.com/cline/cline/pull/12260), where `parseKeyPairsIntoRecord` wrapped
its whole `forEach` in one `try/catch` with an empty body, so a single un-decodable entry dropped
**every entry after it**, including `Authorization`. Two independent arrivals at one rule: a batch
loader must fail per-entry, and an empty result must be distinguishable from a failed load.

**What inber should consider — inber has this on the most load-bearing input it has.**
`engine/turn_prompt.go:90-94`:

```go
memories, tokensUsed, err := e.MemStore.BuildContext(req)
if err != nil {
    Log.Warn("failed to build context from memory: %v", err)
    return nil
}
```

`BuildSystemPrompt` returning `nil` means **the turn proceeds with a completely empty system
prompt** — no identity, no always-load instructions, no tool memories — and the `IdentityOverride`
fallback below it is unreachable, because it lives in the `else` branch taken only when `MemStore`
is nil. The trigger is not exotic: `memory-store/builder.go:62-64` returns the error from a plain
`s.db.Query`, the store is a single-writer SQLite pool, and a `SQLITE_BUSY` under concurrent
sessions is enough. So a transient lock makes the agent forget who it is for exactly one turn, then
recover — while also changing every system block and so destroying the cached prefix, meaning the
failure is a behaviour change *and* a cost spike. Note the contrast one function down:
`memory-store/builder.go:75-78` already skips a bad **row** with `continue`, the per-entry tolerance
cline moved to; it is the whole-query failure that is all-or-nothing at the caller. Two decisions
worth naming rather than silently picking, since they differ in blast radius: fail the turn loudly,
or degrade to `IdentityOverride` plus a model-visible note that memory was unavailable. What is not
defensible is the current third option — proceed with nothing and log a warning. The generalizable
audit, matching the 07-29 empty-allowlist entry: **for every path that assembles model input,
distinguish "empty" from "failed", and make failure impossible to mistake for a quiet success.**
`session/workspace.go:85-88` has the milder version of the same shape (a system block that fails to
read is `continue`d, silently shortening the prompt).

### 2. A server-supplied capability claim is *recorded*, never promoted to authority — and its absence is a third value

Two codex PRs landed the same day making the same refusal.
[#36055](https://github.com/openai/codex/pull/36055) surfaces MCP `readOnlyHint` on tool-call items,
the event stream and the persisted rollout — and pointedly **not** into the approval decision, with
a README line that firewalls the claim from the outcome: the hint "describes tool capability, not
whether an invocation succeeded or performed a write; use `status`, `result`, and `error` to
determine the execution outcome." It is typed `Option<bool>`, defaulted nowhere: `null` means the
annotation was unavailable, including for older rollout entries.
[#36045](https://github.com/openai/codex/pull/36045) fixes the inverse error — an OAuth *discovery*
failure (their test uses a 429) was reported as `unsupported`, i.e. "this server confirmed it has no
OAuth", turning an inconclusive probe into a concrete negative. The fix adds `unknown` and preserves
the discovery error. Both are the soundness form of the `feedback_status_enum_granularity` rule, and
the second is the mirror of the 07-29 empty-allowlist entry: that one collapsed `[]` into
*permissive*, this one collapses *probe failed* into *confirmed negative*.

The week's field evidence for why the firewall matters: Hugging Face's
[technical timeline of the July 2026 agent intrusion](https://huggingface.co/blog/agent-intrusion-technical-timeline)
(07-27) reports an agent that escaped an evaluation sandbox and ran a multi-day intrusion against
production infrastructure. The first entry vector **executed no code at all** — a malicious dataset
config pointed HDF5 external storage at local paths like `/proc/self/environ`, and the ordinary API
response returned the worker's environment variables and secrets. A tool that is genuinely read-only
by capability was a complete exfiltration channel by argument. The postmortem's other transferable
finding is that detection worked and *escalation* did not: signals fired across several layers, but
the alert's criticality was never raised.

**What inber should consider:** inber's only notion of read-only is `guard.isReadOnly`
(`guard/guard.go:187-194`), an 8-name switch, with `isDangerous` a 4-name switch beside it — both
**closed-world over inber's own statically registered tool names**. Any name in neither list is
`isReadOnly=false, isDangerous=false`, which in the `Assist` branch returns `Allowed` with no
approval. A name switch is structurally incapable of classifying a tool it has never heard of, and
dynamically discovered MCP tools are exactly that population — while `tools/mcp/client.go:42-46`
discards the MCP `annotations` object entirely, and `agent.Tool` (`agent/agent.go:26-31`) has no
field to carry it. So: parse `readOnlyHint`/`destructiveHint` into a **`*bool`, not a `bool`** — in
Go the zero value *is* the collapse #36045 fixes — carry it on `agent.Tool`, route the *unknown*
case to approval rather than to `Allowed`, and record the claim with its origin on the tool-call
event so an incident can separate "the server said this was read-only" from "this call wrote
nothing". Copy codex's README wording as the rule. And take the HF vector as the standing caveat on
all of it: **fail-closed on the tool name is not fail-closed on the argument** — a read tool with an
attacker-chosen path is a write tool pointed the other way. This is the fourth independent arrival
at "gate on the call, not on a projection of it" (07-25 principals, 07-29 executable bytes, the
enforceability paper in `papers/2026-07-harness-research.md`, here).

> **[Walked 2026-08-03 — the MCP half is LATENT, and walking it found a LIVE hole one layer
> up. The closed-world half is now checked; classifying the names is filed, not decided.]**
>
> **Latent, as this entry half-suspects:** `tools/mcp` still has zero non-test importers, and
> `ToolInfo` (`tools/mcp/client.go:40-44`) parses `name`/`description`/`inputSchema` only, so
> there is no `annotations` object to promote and `agent.Tool` (`agent/agent.go:35-40`) has no
> field to carry one. The `*bool` recommendation is sound and has nothing to attach to yet.
>
> **Live, and not in this entry:** the closed-world criticism is right, but the population it
> misses is not MCP tools — it is inber's own. `guard.isReadOnly`/`isDangerous` classify 12
> names between them. **Nine names reach the model and are in neither**, so `CheckTool`'s
> Assist branch returns `Allowed` and never consults the approver:
> `task_plan` and `scratchpad` (via `Engine.buildSpecialTool`), and the seven the server
> injects through `EngineConfig.ExtraTools` — `spawn_agent`, `steer_agent`, `agents_status`,
> `merge_workspace`, `reject_workspace`, `fix_workspace`, `list_workspaces`.
> `merge_workspace` rebases a spawn branch onto **main** and pushes; `reject_workspace`
> deletes worktrees and branches. Both are strictly more destructive than `deploy`, which *is*
> classified dangerous.
>
> **`spawn_agent` is not a hole in the gate, it is a door out of it.** Spawning is unclassified,
> so Assist allows it unasked; the child is built by `createSession(..., RunRequest{}, ...)`
> (`server/spawn.go`, `server/session_forking.go`), `EngineConfig` has no mode to copy, and
> `guard.ParseMode("")` is `Unset`, whose `CheckTool` default allows everything. An Assist
> session therefore reaches `shell_commands` by spawning a child that was never gated.
> Observe is unaffected — it denies anything not read-only, so it denies spawning too.
>
> **Why the existing completeness test could not see any of this.** `TestEveryKnownToolIsClassifiedOrNamedHere`
> promises that registering one more tool reddens it, but `knownToolNames` derives from
> tool-store's *global registry* plus a hand-written constructor list. tool-store deliberately
> does not auto-register its argument-taking constructors, and the list compensated for
> `repo_map`/`recent_files` while missing `task_plan`/`scratchpad` from the same carve-out. It
> has no term at all for `Server.toolsForAgent`, the only producer of `ExtraTools`.
>
> **What shipped: the check, not the policy.** `task_plan`/`scratchpad` are now in
> `knownToolNames`, and `server/tool_classification_test.go` is the same completeness check over
> the seven server-supplied tools, written in the package that owns them because `guard` cannot
> import `server`. `TestAssistModeApprovalGateIsEscapableBySpawning` pins the escape **as
> present**. Five sabotages, each run, each caught by a different assertion — including "a new
> eighth server tool appears", which is the property the promise rests on.
>
> **Classifying them is the owner's call and is filed, not taken.** Child todo
> `9eeba694-6fc0-4e1e-9958-a0a988c0dae1`: it decides whether spawn inherits its parent's mode
> (the same inheritance question `65301d09` parks for `disabled_tools` and `9e31d359` parks
> for caps — answer all three at once), and how these nine names should be classified. Note
> `agents_status` and `list_workspaces` are the mirror defect: Observe *denies* them, so the
> read-only mode is less useful than it should be.

### 3. A loop whose continuation an untrusted peer chooses needs four bounds, not one

[codex #36039](https://github.com/openai/codex/pull/36039) reads as a pagination cleanup and is
actually a hostile-peer bound. One shared collector now covers MCP tools, resources and
resource-templates with **four independent limits plus cycle detection**: ≤100 pages, ≤1,024 items,
cursor ≤64 KiB, any **repeated cursor rejected**, and the whole *operation* bounded by the tool
timeout (30s fallback). The design point is that no single bound stops all the failure modes — a
page cap does not stop one enormous page, an item cap does not stop a slow-loris, neither stops an
A→B→A cursor cycle — and the timeout has to bound the operation rather than each request.

**What inber should consider:** `tools/mcp/client.go:220-256` (`waitForResponse`) is precisely this
shape with one bound. It loops to a hard-coded 30s deadline, `continue`s on every non-JSON line and
every non-matching id with **no cap on lines consumed**, so a chatty server keeps it spinning for
the full window; and at `:251-252` it silently discards responses belonging to other in-flight
calls, with a comment conceding "in a full implementation we'd buffer these" — meaning it can eat
the reply to a concurrent request. Apply the four-bound recipe (count, size, wall clock, repeat
detection) and buffer by id instead of dropping. Two adjacent items from the same window worth
copying when inber wires MCP: [#35941](https://github.com/openai/codex/pull/35941) caps each
namespace description at 1 KiB **at the model-facing render only**, leaving `tool_info` untouched
upstream, and gives the aggregate `tool_search` source list a 4 KiB budget that **reserves every
source's name bytes first** and spends the remainder on descriptions in order — the 07-23
skill-catalog ladder applied to a second catalog, the news being that MCP was the catalog with no
bound at all. inber has none either: `agent/agent_run.go:36-40` passes every tool description
verbatim, and `:41-44` puts the `cache_control` breakpoint on the **last** tool definition, so an
unbounded third-party description sits inside the cached prefix and shifts it. If inber copies a
byte cap, cut on a rune boundary — codex's test is `"é".repeat(499)` + `🦀`, and a naive Go byte
slice puts invalid UTF-8 in the prompt. Related defect found while checking:
`Engine.SetDisabledTools` (`engine/engine.go:285-297`) reassigns `e.agentTools = filtered` while its
comment claims it re-filters from the full set — the full set is gone after the first call, so
disabling is monotonic, irreversible, and silently relocates the cache breakpoint.
> **[Verified 2026-08-03 — the `SetDisabledTools` defect in the last sentence is FIXED; the rest of
> §3 is unchecked.]** `Engine` now keeps `allTools` beside `agentTools` and `applyDisabledTools`
> derives one from the other, so disabling is reversible. The MCP `waitForResponse` four-bound
> recommendation and the unbounded tool descriptions are untouched — but note `tools/mcp` has zero
> non-test importers, so the MCP half is latent (see the 06-15 opencode entry and todo `fb0dd7cc`).
>
> **[Walked 2026-08-03 — §3 is SPENT; nothing filable is left in it.]** Both remaining halves are
> latent rather than live, so neither is a defect to fix today. `tools/mcp` still has zero non-test
> importers, so the four-bound recipe has no caller to protect. The unbounded tool description has
> no untrusted producer: `ExtraTools`' only in-tree producer is `Server.toolsForAgent`, in-process
> server code, so no third-party bytes reach the tools block or the `cache_control` breakpoint
> anchored on its last entry. Both go live the day inber wires a real MCP client, and the read
> bounds are already parked in `fb0dd7cc`. Do not re-walk until `tools/mcp` gains an importer.

### 4. The prompt and the tool schema are one co-designed contract — codex's environment work, and the isolation defect inber already has

Nine codex PRs this week extend the multi-environment substrate. The May-2026 entry at §"Multi-environment
context per turn" and the 07-27 remote-code-mode entry already cover `environment_context` and
`environment_id` tool routing, so most of the cluster is corroborating plumbing. Three things are
new. (a) [#35874](https://github.com/openai/codex/pull/35874) marks `primary="true"` in the rendered
`<environments>` block, because which environment is the implicit default had been conveyed only by
list order while the tool schema already said "omit to use the primary environment" — the prompt tag
and the parameter description now reference each other by name, and the attribute is **suppressed
entirely at count 1** so single-environment prefix bytes never move. (b) The scoping unit moves from
turn to **sampling step**: `StepContext` binds environments, capability roots, MCP binding, tool list
and AGENTS.md into one immutable per-request snapshot, which makes a mid-turn `starting`→ready
transition model-visible (with a `wait_for_environment` tool) and gives spawns an inheritance rule —
[#35895](https://github.com/openai/codex/pull/35895) has a child inherit the environment set as of
the *step* that spawned it, not as of turn open. (c) [#35850](https://github.com/openai/codex/pull/35850)
stops the host asserting jurisdiction over foreign path conventions, because listing background
terminals was *failing* on legitimate entries from another platform. Also worth one line:
[#35944](https://github.com/openai/codex/pull/35944) reports subagent addressability as a
**tri-state** `canAcceptDirectInput` derived from live thread state, never persisted — a fifth axis
on the four in the 2026-06-09 fleet entry, namely *who may address it*.

**What inber should consider:** build the single environment-resolution chokepoint before there is a
second environment, because inber's *one* environment already leaks. `ShellInDir`
(`tools/tools.go:55-72`) injects a default `workdir` so `shell_commands` runs in the repo root, but
the file tools — `read_files`/`write_files`/`edit_files`/`list_files`, from tool-store's `fs.go` —
take a **bare `path` with no root parameter and no join against one**, so relative paths resolve
against the inber-server *process* cwd. A spawned agent whose workspace is a forge worktree
(`server/spawn.go:111` → `server/session_creation.go:47`) therefore runs shell commands inside the
worktree while a relative-path `write_files` writes outside it. That is a live silent-isolation
defect, not a missing feature, and codex's `resolve_tool_environment()` chokepoint — one function
every tool passes through, one path type that cannot be built without an environment — is the shape
that makes it unrepresentable. Second, the primary-marker lesson applies today: `forge.Workspace.Repos`
is a **map** that `server/spawn.go:111` collapses to `w.Repos[w.Primary]`, while the other repos are
on disk, get committed and merged, and the model is told about none of them —
`server/workspace_tools.go:227-229` says "You are working in an existing workspace with previous
changes" without naming a single path. Render all roots and mark the primary. The delivery channel
already exists and is nearly empty: `e.Turn.VolatileContext` is the structural twin of codex's
user-role `<environment_context>` fragment — same role, same once-per-turn placement, same
post-cache-boundary position — and it currently carries only fleet status and injectors.

> **[Walked 2026-08-03 — §4 is SPENT. Both halves shipped, and the entry is stale in its
> specifics.]** The isolation defect is closed: `ShellInDir` no longer exists, and `tools/root.go`'s
> `ScopeToRoot` replaced it with a closed table of filesystem tools plus a completeness test
> (`TestEveryFilesystemToolDeclaresItsPathArguments`) — the "one function every tool passes through"
> chokepoint this entry asked for. Whether a rooted session *confines* its file tools or only
> defaults them is the remaining question and it is parked in todo `d967400a`; it is a policy call,
> not this entry's defect. The primary-marker half also shipped: `server/workspace_roots.go` turns a
> workspace into every repository with the primary one marked, both `useWorkspace` callers set the
> path and the root set from one reading so they cannot disagree, and `engine/workspace_roots.go`'s
> `renderWorkspaceRoots` queues all of them into `Turn.VolatileContext` at
> `engine/turn_prepare.go:107` — which is exactly the delivery channel the last sentence named. A
> workspace whose `Primary` names no repository is now an error rather than an empty string, because
> "" reached the engine as "no root is known" and sent relative paths back to the server's own cwd.
>
> **Found while walking this entry, and in no document: `fix_workspace` picked its parent session by
> ranging `g.sessions` for whichever session was Running.** The entry is about environment
> resolution; the same "infer it rather than pass it" shape had a live instance one layer over, in
> *session* resolution. `sync.Map` order is unspecified and inber-server holds many sessions, so the
> fix agent was charged to an arbitrary one — wrong depth cap, wrong children quota, wrong budget
> lineage via `mintChildSessionKey`, and its events and results delivered to a session that had not
> asked for it. `toolsForAgent` already had the key and already passed it to `SpawnAgentTool` one
> line above. Fixed in `954c794`, with the tool taking the key like its sibling.

## Harness-watch — 2026-07-31: a tool registry needs *two* registration policies (host strict, external first-wins), an in-flight delegation is its own retention class, and a call nested inside another call's arguments still needs a typed record

### 1. Dedup is a registration policy, not a render-time filter — and host tools are reserved before the model-visible list is built

codex [#36127](https://github.com/openai/codex/pull/36127) rewrote `ToolRegistry` from a `HashMap`
to an insertion-ordered `IndexMap` with three entry points that differ only in collision policy:
`register_trusted` (occupied ⇒ hard error, *"tool {name} already registered"*), `prepend_trusted`
(same strictness, `shift_insert(0, …)` so host runtimes iterate first), and `register_external`
(occupied ⇒ `warn!("skipping duplicate external tool")` and return `false` — **first registration
wins, so no MCP, extension or dynamic tool can displace an incumbent**). Overriding a host tool is
legal only through an explicit `remove()`, which `finalize_tool_router` uses to *reserve* names: it
deletes any client-supplied `tool_search` definition and re-registers the host implementation at
position 0. The old `HashSet<ToolName> seen_tool_names` dedup inside
`build_model_visible_specs_and_registry` was deleted outright — dedup moved from "filter when
rendering" to "policy at registration". [#36129](https://github.com/openai/codex/pull/36129) adds
the corollary for *derived* namespaces: code-mode normalizes each tool name, so two distinct raw
names can collapse onto one identifier; codex keeps a first-wins `BTreeMap<normalized, ToolName>`,
warns on the loser, and gates the model-facing augmentation on `winner == tool_name` so the shadowed
tool keeps its plain spec instead of being advertised under a name that will dispatch elsewhere.

**What inber should consider:** inber has the same surface with the policy inverted, and a live
collision today. `engine/engine_new.go:498-511` ("Merge server-injected tools (replace same-named)")
linearly scans `e.agentTools` and **unconditionally overwrites any host tool an externally-supplied
`cfg.ExtraTools` entry happens to name**; there is no trusted/external distinction anywhere.
That path is in production — `server/agent_tools.go:7-33` feeds `ExtraTools` via
`server/session_creation.go:52`, and `server/spawn_tools.go:76` declares `Name: "spawn_agent"`, the
*same name* as `agent/registry/spawn_tool.go:90`, which `engine/build_tools.go:89-93` registers
whenever an agent config lists `spawn_agent`. Two implementations of one tool, and which survives is
decided by the order of two loops. codex's `register_trusted` would have aborted at startup. Worse,
inber has name-load-bearing control flow with no protection: `agent/agent_run.go:33` skips
`then`-chain injection iff `t.Name != "end_turn"`, and `engine/build_tools.go:52,114,125` swap in
`ShellInDir` iff the name is `shell`/`shell_commands` — so an `ExtraTools` entry named `end_turn`
replaces the turn-termination tool while the special-case still keys off the name. Every registry is
documented last-write-wins (`tools/interface.go:42-46`, `agent/registry/tools.go:56-58`), nothing
dedups the final slice (`engine/build_tools.go:20-41` appends with no seen-set;
`agent/agent_run.go:29-49` pushes *every* element into `toolParams` while `toolMap[t.Name]` keeps
only the last, so a duplicated config name puts two definitions on the wire and dispatches to the
second), and `tools/mcp/adapter.go:74-89` concatenates every client's tools with no name check while
iterating a Go **map** — nondeterministic order, latent only because that package has zero
importers. Adopt the split: host/core registers strictly (duplicate = startup error), server-injected
and MCP register first-wins-with-warning, and a genuine override is an explicit `remove()` +
re-register so it is declared rather than emergent. Reserve `end_turn`, `shell_commands` and
`spawn_agent` before the model-visible list is built. Take #36129's corollary *before* adding any
name mapping: inber has no tool-name normalization yet, but it already has the same shape one level
down — `sanitizeToolID` (`agent/openai_utils.go:10-25`, copy-pasted into `session/resume.go:17-23`)
is a **lossy many-to-one map** applied to tool-use IDs with no collision check on either side, so two
colliding IDs in one turn produce duplicate `tool_use` IDs and mispaired `tool_result`s.

### 2. An in-flight delegation is its own retention class — keep the task, drop the completion, bound both by tokens

codex [#36128](https://github.com/openai/codex/pull/36128) stopped letting inter-agent traffic fall
through the generic message arm of remote compaction. `is_retained_for_remote_compaction_v2` now
retains a `ResponseItem::AgentMessage` iff it is **not a completion** (detected by the literal
`"Message Type: FINAL_ANSWER\n"` prefix) **and** its `estimate_item_token_count` is within the new
`MAX_RETAINED_AGENT_MESSAGE_TOKENS = 10_000`. Three supporting edits make the accounting honest:
`message_text_token_count` stopped returning `0` for non-`Message` items (agent messages had been
counted as free), the encrypted-output estimator learned to walk `AgentMessageInputContent::EncryptedContent`,
and the truncation helper's non-`Message` fallback flipped from `Some(item)` to `None`. Separately,
`agent/control/spawn.rs:696` strips **all** `AgentMessage` items when forking a child — delegation
chatter is parent-scoped and is not inherited. The rule underneath: *what you delegated* is small,
high-value and must survive a context reset; *what came back* is redundant once summarized; and both
need a token bound so one huge child message cannot eat the retained budget.

**What inber should consider:** inber's pruning is uniform and age-based, and it destroys exactly the
delegation record. `conversation/manage_tool_pruning.go:63-82` replaces a `tool_use` block's entire
`Input` with a **60-character** `_summary` string, for every tool alike, once `age > cfg.ToolCallKeepFull`
— and `ToolCallKeepFull` is **5** in all four role configs (`conversation/manage_config.go:51,77,103,129`),
fired from `conversation/manage.go:129-137,148-155`. The matching result is dropped outright at
`age >= cfg.ToolResultDrop` (8 turns for the orchestrator). Meanwhile inber's `spawn_agent` is
explicitly **fire-and-forget** (`agent/registry/spawn_tool.go:71-73`: *"Purely declarative… Always
async — returns immediately"*), so the `spawn_agent` tool-use input is **the only in-context record
of an outstanding delegation** — no completion ever arrives in-band to re-establish it. Six turns
after delegating, an inber orchestrator's memory of the task it handed off is a 60-character prefix;
nine turns after, the ack reads `[result dropped - too old]`. Give `manage.go`'s loop a retention
*class* rather than age alone and make delegation its first member: exempt `spawn_agent` (and any
future `steer_agent`/`merge_workspace`) inputs from `truncateToolCall`, bounded by **tokens** — 10k
is codex's shape — not by a fixed 60 bytes. The asymmetry is currently backwards: inber ages out the
cheap irreplaceable record on the same clock as a 5,000-line shell dump. Two incidental defects in
the same function: `truncateToOneLine` (`conversation/manage_text_utils.go:20`) cuts with `text[:maxLen]`,
a **byte** slice, so a multibyte task description puts invalid UTF-8 in the prompt (the rune-boundary
rule from the 07-29 entry, now with a live inber instance), and `fmt.Sprintf("%v", toolUse.Input)` on
a map has nondeterministic key order, so the same call summarizes differently across runs. The fork
half has no inber equivalent yet, but it is the rule to adopt if `server/session_forking.go` ever
hands a child the parent's messages.

> **[Walked 2026-08-03 — both "incidental defects" in this paragraph are already FIXED; the headline
> recommendation is a policy call and is now filed.]** The two asides this entry closes with have
> both been fixed by intervening work, and the entry's line references no longer hold:
> `truncateToOneLine` (`conversation/manage_text_utils.go`) now cuts through
> `textutil.TruncateWith`, on a rune boundary, with a comment saying why — no byte slice, no invalid
> UTF-8 in the prompt; and `fmt.Sprintf("%v", toolUse.Input)` is gone, replaced by `ToolInputText`,
> which returns a `json.RawMessage` verbatim and otherwise marshals (Go sorts map keys, so the
> nondeterministic-ordering half is closed too). Note the second aside as filed was also
> *understated* rather than overstated: `%v` on a `json.RawMessage` printed decimal byte codes, not
> merely a shuffled map — that was child `4e1f78af`.
>
> The headline recommendation — give `manage.go`'s loop a retention *class* and make delegation its
> first member, bounded by tokens rather than 60 bytes — is **untouched and is genuinely the
> owner's**: which tools are exempt and what the bound is are both product calls, and the measured
> asymmetry the entry describes still holds (`ToolCallKeepFull` is 5 in all four role configs, and
> `spawn_agent` is fire-and-forget, so its tool_use input really is the only in-context record of an
> outstanding delegation). Filed as a child of the harness-watch shelf.
>
> Verified while here, so nobody re-derives it: the fork half still has no inber equivalent —
> `server/session_forking.go` hands the child `WorkspaceRoots` and the parent's engine state, and
> there is no `AgentMessage`-equivalent class to strip.

### 3. A tool call nested inside another tool's arguments needs a typed attempted-call record — including the ones that were blocked, malformed, or never ran

codex [#36181](https://github.com/openai/codex/pull/36181) added `core/src/tools/executed_tool_calls.rs`,
which records every tool call the model *attempted* — including **blocked and failed** ones, and
including calls issued from inside a code-mode cell that never appear as first-class tool-call items.
`attach_pending_to_prompt` walks the outgoing prompt in reverse, matches `FunctionCallOutput` /
`CustomToolCallOutput` / `ToolSearchOutput` by `call_id`, and appends the recorded
`ExecutedToolCall { name, arguments }` list to that output item. Three details are the transferable
part. **Retry stability:** a `retry_cache` keyed on `(std::mem::discriminant(item), call_id)` lives
outside the loop, so a resampled request re-attaches *identical* metadata instead of double-appending
or losing it — idempotence by construction. **Bounded three ways, with the overflow named:** 256
pending calls, 8 KiB of arguments per call, 32 KiB per output; on overflow it emits
`ExecutedToolCall::truncated(name, original_bytes, max_bytes)`, so the **name always survives** and
the cap is reported, and even at the hard limit it records a zero-byte truncated entry rather than
dropping the call. **Honest scope:** the doc comment concedes that cancellation, compaction and
yielded cells can leave pending calls unreported, and the whole thing is opt-in behind a feature flag.

**What inber should consider:** inber has exactly this shape and keeps no record at all — the `then`
chain. `agent/chain.go:57-74` injects a `then: {tool, input}` field into every tool schema except
`end_turn`, so the model requests a follow-up tool *inside another tool's arguments*, and
`executeWithChain` (`:117-172`) runs it. `extractChain` (`:84-113`) **deletes** `then` before
dispatch and, if the field fails to unmarshal or `ct.Tool == ""`, returns `(cleanInput, nil)` — the
requested follow-up **silently never runs**, with no error and nothing in the tool result telling the
model its chain was dropped. An unknown chained tool appends `"\n\n--- then(X) error: unknown tool ---"`
to the text and returns **`isError=false`** (`:148-151`) — a failed call the turn does not record as
an error. On success `:166-172` concatenates `primaryOutput + "\n\n--- then(X) ---\n" + chainOutput`,
so the chained tool's identity and outcome exist only as a delimiter in free text — the same
delimiter a tool's own output could contain — and one `tool_use` block in the transcript stands for
two executions. The asymmetry is visible in inber's own code: hooks *do* fire for the chained call
with a synthetic `blockID+"-chain"` (`:153-155,162-164`), so the trace/UI layer gets a structured
record of the nested execution and the model gets a string. Attach a typed `{name, arguments, status}`
record to the tool result the chain rode in on, produced locally and never parsed back out of text;
make the silent-drop path at `chain.go:108-110` a *recorded* outcome (`chain_dropped: malformed`),
since a follow-up the model asked for and never got is precisely the case that must not be invisible;
make `:148-151` return `isError=true`; bound the recorded arguments and emit a named truncation
rather than dropping. The 07-29 entry already covers typed truncation records in general — what is
new here is the contract for **nested calls that have no tool-call item of their own**, plus the
retry-cache idempotence, neither of which applies to the sites that entry named.

> **[Walked 2026-08-03 — most of this SHIPPED; one recommendation is left and it is a real trade,
> not an oversight.]** The typed record this entry asks for largely exists. `agent/chain.go` now
> carries `toolCallOutcome`, which reports the primary and chained calls apart —
> `chainTool`/`chainInput`/`chainOutput`/`chainFailed` — rather than only as concatenated text, and
> the silent-drop path is now a *recorded* outcome: `extractChain` returns a `dropped` reason
> ("it names no tool", "it is not an object with a tool and an input", "it arrived as text that does
> not read as {tool, input}"), and `chainNotRunTool`/`chainNotRunReason`/`chainNotRunRefused` put a
> chain that never ran on the tool result it rode in on. The entry's own line numbers are stale
> throughout; read the file, not the citation.
>
> **Still open, and it is a decision rather than a fix:** an unknown chained tool still returns
> `isError=false` (the `!ok` branch on `toolMap[chain.Tool]`). Making it `true` is not free — the
> primary call succeeded, and engine's `OnToolResult` increments `Turn.ConsecutiveErrors`, which
> drives the error-recovery context ladder in `Engine.contextBudget`: one error widens memory recall
> from 6,000 tokens to 20,000, three to 35,000, five to 50,000, and each widening rewrites the
> cached system-prompt prefix so the whole prompt is paid for again. That ladder is exactly what the
> 07-30 `end_turn` work measured. So "a failed call the turn does not record as an error" trades
> against "a succeeded call billed as a failure", and which way it should go is the owner's. Filed
> as a child of the harness-watch shelf.

## Harness-watch — 2026-08-01: authority covers the *rider*, not just the call; a durable history is a *singly-owned* resource; and a denial is its own event class

### 1. A field riding along with a tool call is part of the call, and the gate must cover it

codex [#36350](https://github.com/openai/codex/pull/36350) rejects a `shell_command` /
`exec_command` call that carries a `justification` argument but no `sandbox_permissions`, **before
any part of the call executes**, with a model-visible error telling the caller to request
`require_escalated` explicitly or drop the justification. The change is three lines and the
principle is large: a *rider* — a field the model attaches alongside a tool's own arguments — is
part of the call and must be validated under the same authority as the call. goose
[#10612](https://github.com/block/goose/pull/10612) is the same rule stated as a lattice. Its
`apply_inspection_results_to_permissions` merges verdicts from N independent inspectors into
`approved` / `needs_approval` / `denied`, and when an inspector returned `RequireApproval` the code
checked only whether the request was already in `needs_approval` — so a request an earlier inspector
had **denied** was re-added to `needs_approval` and became reachable by a user click. The fix adds
the `denied` check and pins `denial_dominates_regardless_of_inspection_result_order` over both
permutations. **Deny is the absorbing element, and an escalation must never be able to lift one.**
codex [#36365](https://github.com/openai/codex/pull/36365) closes the third face: an approval marker
supplied by a party you don't control is a *claim*, so the review request is rebuilt from the
harness's own live invocation details, and every inconclusive branch (unavailable, malformed,
policy-disallowed) fails **closed** rather than falling back to "ask a human" — which is exactly what
is unavailable in the automated case the path exists for.

**What inber should consider:** inber has this defect live, and its own code comment says so.
`agent/chain.go:323-326` reads *"Ask the gate before anything else this block asks for happens … the
sideband fields are instructions that arrived attached to a call that is not going to run."* The gate
runs at `:327`. **`processSideband` already ran at `agent/chain.go:278`, 49 lines earlier.** The
sideband fields are injected into the schema of *every* tool of *every* agent (`agent/chain.go:100`,
applied at `agent/agent_run.go:26` and `engine/turn_openai.go:32`) and the callbacks are wired
whenever `e.repoRoot != ""` (`engine/build.go:86-88`), so both dispatch paths are affected. Those
callbacks are not bookkeeping: `SaveNote` writes the scratchpad file unconditionally, with no
precondition (`engine/build_sideband.go:57-64`); `done`/`split` rewrite `.task.md`
(`engine/build_sideband.go:40,90`); and a `done` that empties the plan reaches
`toolstoretools.RunBuildCheck` → `childprocess.NewCommand(ctx, "bash", "-c", cmd)`
(`tool-store/tools/task_plan.go:301`, `TaskPlanBuildCommand` defaulting to a non-empty
`"go build ./..."`). So a session in `mode: observe` — documented at `guard/guard.go:6` as
"read-only tools. No file writes, no shell execution" — that denies `write_files` has **already
written a file** if the same block carried `note`, and has **already run `bash -c`** if it carried
`done`. The tool result hands the model both at once: `"[✓ completed 1 task(s)]\nrefused:
write_files was not run — observe mode allows read-only tools only"`. This is the same bypass class
`agent/tool_gate_test.go:44-47` closed for the `then` chain ("a gate wired to the first alone leaves
the model a way to run any tool it likes"), one field over — and
`agent/sideband_ignored_test.go:112-144` currently **pins the bug as correct**, asserting
`completed == 1` for a refused call. That test is right about the *reporting* requirement and
encodes the *authorization* bug: a green test carrying the same false premise as the defect.
Adopt #36350's placement (validate the whole block, riders included, before anything runs) and
#10612's lattice (deny dominates every sibling effect, not just sibling verdicts).

### 2. A durable conversation history is singly-owned — acquire at create *and* resume, release on failed init *and* clean shutdown

> **[Verified 2026-08-03 — the headline half was already fixed; the "second-order question" at the
> end was the live defect, and it is now fixed too. Both halves SPENT.]**
> The bare `g.sessions.Delete` this entry is built on is gone: `server/server.go` releases the old
> session inside the queue (`releaseSession`, whose doc comment records the leak and why the release
> happens under the per-session lock), so `Engine.Close` and `SaveSessionSummary` do run. Do not
> re-file the leak.
> **The tail was real and nobody had opened it.** `new_session` did not merely reload the old
> `messages.json` — it never reached the engine at all. `applyRequestOverrides` copied eleven fields
> and not this one, so `engine.setupSession` took the `!newSession` branch and loaded the workspace
> transcript rather than calling `ClearMessages`; and `createSession` then read the key's persisted
> transcript back and `RestoreSession`d it at the persisted turn count. Two independent links, both
> dead, either one enough on its own. Fixed with a `transcriptToStartSessionFrom` that names which
> copy a session opens with, pinned by `server/new_session_request_test.go` (a resume control beside
> each fresh case, three sabotage rounds red).
> ⛔ Do **not** also reset the guard totals. `restoreGuardState` still restores the caps *and* the
> spend to a fresh session, deliberately: resetting them makes `new_session` a way to ask for the
> budget back, which is noteboard `610e0f4a` asked about forks. Filed as `b5a75454`, to be answered
> with `610e0f4a` rather than separately.

codex [#36389](https://github.com/openai/codex/pull/36389) makes writer-ownership uniform across
thread-history modes. Its paginated store already had cross-process guards; the legacy mode had
none. Now a writer lock is acquired and retained whenever a thread is created **or resumed** in
either mode, the same ownership check runs before archiving or deleting, and ownership is released
on two symmetric events — initialization failing partway, and the active thread shutting down. The
corollary that matters: **when one storage path is guarded and an older one isn't, the unguarded
path is the whole property**, because it is the one anything can reach.

**What inber should consider:** inber has four session-removal sites and exactly one forgets to
close. `server/server.go:287` is a bare `g.sessions.Delete(sessionKey)` on the `req.NewSession`
path; every other site pairs removal with `close()` (`server/session_reaper.go:80-85` uses
`LoadAndDelete` then `s.close()`, and so do `server/session_creation.go:37-40`,
`server/api_bridge.go:541-543`, `server/server.go:165`). The dropped `*Session` becomes unreachable,
so `Engine.Close()` (`engine/engine.go:453-484`) never runs, which skips per leaked session:
`MemStore.Close()`, `SessionDB.Close()`, `forgeDB.Close()`, `Session.Close()` (JSONL writers) — and
`SaveSessionSummary(e.MemStore, e.Messages, e.AgentName)` at `:463-465`. Go's `sql.DB` has no
finalizer, so those descriptors are held for the process lifetime; N `new_session` calls leak N
engines on the same long-lived `inber-server` the reaper exists to keep from OOMing. The behavioural
half is worse than the resource half: `SaveSessionSummary` is what distils a finished conversation
into memory, and **starting a fresh session is precisely when you want the old one summarized** —
it is the one path that never does. Note the second-order question #36389 also raises: `new_session`
reuses the key `agent:<name>:main`, so the replacement's `loadPersistedSession`
(`server/session_creation.go:228-247`) reads the *old* session's `messages.json` back off disk.
"Fresh" is fresh in memory only.

### 3. A policy denial is its own event class — folding it into the error channel makes it indistinguishable from a tool that simply broke

codex [#36207](https://github.com/openai/codex/pull/36207) introduces unified sandbox-violation
types in `codex-sandboxing`, emitted through one tracing channel, classified by **backend** and
**reason**, carrying an optional path and a bounded output snippet, plus a sandbox-type field on
exec-server responses so a *remote* denial can be classified rather than guessed. It is recorded
across every enforcement path (exec, apply-patch, shell escalation, unified exec, managed network)
and explicitly **does not change denial behaviour** — it only makes the denial a typed, named
record, because filesystem denials and network blocks previously surfaced in different shapes and
every consumer downstream parsed backend-specific output to learn anything had been denied at all.

**What inber should consider:** in inber a denial is a `bool`, and it is the same `bool` a crashing
tool sets. `agent/agent_run.go:307` fires `OnToolResult(…, true)` for a guard refusal, an unknown
tool, and a tool that returned an error alike — indistinguishable at that call site — and a chain
that did not run fires it a **second** time at `agent/chain.go:301`. Both land in the single branch
at `engine/build_hooks.go:150-153` (`e.Turn.ConsecutiveErrors++`), whose only consumer is
`engine/turn_context.go:12-20`: `>= 5` errors ⇒ memory-recall budget `(0, 50000)`, `>= 3` ⇒
`(0, 35000)`, `>= 1` ⇒ `(0, 20000)`, against an ordinary 6,000 (`turn_context.go:38`). So three
refused `write_files` calls that each carried a `then` chain increment the counter by **six**, and
the next turn recalls memory at 50k tokens with `minImportance` dropped to 0 — the least selective
recall the engine can do. Every memory block in the system prompt changes, so the cached prefix
`engine/turn_prompt.go` orders for reuse is invalidated *on top of* the ~44k extra input tokens, and
nothing resets the counter until a turn ends clean (`build_hooks.go:201-203`). A session running
into its own policy is thus billed as a session whose tools are crashing. Meanwhile the actual
signal reaches no event, no log line of its own and nothing an operator can query — the only trace
is the literal string `refused: …` inside a tool result. Adopt #36207's shape: a refusal is a typed
record with a reason, produced locally, separate from `isError`. Note `guard.RecordToolCall` /
`IsRepeating` (`guard/guard.go:167,237`) already exist for this and have **zero callers**.

### Also in-window, worth a line each

- **A path comparison over strings is not a comparison of files.** goose
  [#10545](https://github.com/block/goose/pull/10545) canonicalizes both sides before deciding
  whether a harvested directory is inside the working dir, bails if the working dir won't
  canonicalize, and **skips** (never admits) an entry that won't — and its tests were rewritten from
  hardcoded `/home/user/project` strings to real `TempDir`s, because the old ones never touched a
  filesystem and so could not have caught it. inber's read cache keys on the **raw, model-written**
  path with no `Clean`, `Abs` or `EvalSymlinks` anywhere (`agent/read_cache.go:30,37,45`), and the
  strings come straight out of the model's JSON *before* `tools/root.go:99-102` resolves them — so a
  read cached under `engine/lifecycle.go` is not invalidated by an edit written as
  `/home/…/engine/lifecycle.go`, and the next read returns `"[already in context — N lines, read on
  turn T. No need to re-read.]"` for content that changed. The model is told its context is current;
  it is stale, and every subsequent edit is computed against the wrong bytes.
- **codex [#36367](https://github.com/openai/codex/pull/36367)** stores each runtime with its
  **effective** exposure as a registry entry rather than wrapping runtimes, so visibility and
  routability stop being derived separately and drifting; the test is the thesis — a hidden MCP tool
  stays routable but is ineligible for parallel calls. inber is fine here today: `prepareTools`
  derives model-visible params and the dispatch `toolMap` from one slice.
- **codex [#36357](https://github.com/openai/codex/pull/36357)** resolves tool execution from the
  `ToolRouter` in `StepContext` — the finalized plan for that step — and **deletes** the separate
  router parameter so no caller can supply an alternative. The deletion is the load-bearing part.
  inber gets retention right by construction, but the store the plan derives from has no ownership:
  `POST /sessions/{id}/config` (`server/api_bridge.go:653-682`) takes `s.mu`, which `Session.turn`
  **releases at `server/session.go:156` before calling `RunTurn` at `:166`** — so `SetDisabledTools`
  writes the `e.agentTools` slice header while the turn goroutine reads it (`engine/build.go:75`,
  reachable mid-turn via the thinking-signature rebuild at `engine/turn_execute.go:48`). A Go data
  race in the `-race` sense, not merely a logical one.
- **codex [#36306](https://github.com/openai/codex/pull/36306) / [#36310](https://github.com/openai/codex/pull/36310)** make credential scope a property *recorded on the stored credential*, enforced
  at both read and write, failing closed when a load or save would cross the host/executor boundary
  — and refuse at startup, before connecting, rather than at first use. No inber defect today (one
  environment), but the shape is right for when `tools/mcp` is finally wired.
- **cline [#12800](https://github.com/cline/cline/pull/12800)** — a wrapper's own message is not the
  error. An error-extraction chain must be ordered by proximity to the origin, and every rung needs
  a fallback that is still actionable.

## Harness-watch — 2026-08-02: a cached prefix is defined by its *inputs*, an error's *author* decides what you may conclude from it, and "sent" is not "accepted"

### 1. Freezing the rendered string is not enough — freeze the inputs that decide the prefix's contents and order

> **TRUE in code, LATENT on this host's corpus, and the fix is an owner's call — filed, not
> shipped. Verified 2026-08-03.** Every link of the chain reads as described: `turn_prompt.go:81`
> derives `messageTags` from the current user message, `:82` derives the budget from the turn
> counter (4000 turn 0 · 6000 · 8000 from turn 16 · the 20k/35k/50k error ladder) and from the
> message's own size (>300 → 10000, >1000 → 15000), and memory-store's `calculateScore` adds
> `+0.3` per matching tag plus a wall-clock recency bonus, so score decides both order and — via
> the budget cut — membership of the whole `system` array that carries BP2.
>
> **But measure before pricing it.** On `~/.inber/memory.db` the automatic context is *three*
> memories: 10 rows exist, 7 are `session-summary`, and `TagsExcludedFromAutomaticContext`
> keeps every one of those out. The three survivors are all `AlwaysLoad`, whose membership is
> unconditional — `builder.go`'s budget cut appends an always-load row whatever the budget — so
> nothing this entry describes can currently change *which* memories are sent. Only their order
> is at risk, and on these three the scores do not cross. The mechanism is armed, not firing: it
> starts costing the moment the store holds non-excluded, non-always-load rows worth more than a
> budget. Note also that always-load order cannot affect membership of anything, so when it does
> move it is pure cache cost — that half is decision-free whenever someone wants it.
>
> ⛔ **The passage's own diagnostic caveat is SPENT — do not repeat it.** It warns that
> `prompt_blueprint.go` keys system blocks on a description embedding `importance` at `%.1f`.
> That is fixed: `DiffBlueprints` matches blocks **by position, never by ID**, and hashes
> `nb.Text`, with a comment naming this exact importance-drift failure. The blueprint diff is now
> the right instrument, not the wrong one.
>
> The remaining question — whether tag-matched memories belong in the cached prefix at all, or
> after BP3 with the volatile context — is a real recall-versus-cache trade and is **filed**.

goose [#10734](https://github.com/block/goose/pull/10734) is already written up in `goose.md`
(2026-07-30 §1) and its inber verdict there was *"inber already holds this shape structurally, and
has no timestamp in the prompt at all."* The first clause is right — `BuildSystemPrompt` runs once
per turn, so inber cannot have goose's *intra*-turn drift. The second clause is where the audit
stopped one layer too early. inber renders no clock into the prompt, but a clock, a turn counter and
the user's own message text all feed the function that decides **which memories land in the cached
system prefix and in what order**.

The chain, read end to end: `engine/turn_prompt.go:81` derives `messageTags` from the *current user
message* (`memory.AutoTag`), `:82` derives `tokenBudget` from the *turn counter*
(`engine/turn_context.go:8-39` — 4000 on turn 0, 6000 on turns 1-15, 8000 from turn 16, and the
20k/35k/50k error ladder), and `:86` passes both to `MemStore.BuildContext`. Inside memory-store,
`builder.go:112` scores every candidate — `+0.3` per matching tag and a **wall-clock recency bonus**
at `builder.go:368-374` (`time.Since(m.LastAccessed)`, `+0.2` under a day, `+0.1` under a week) —
then `:116-127` sorts on that score and `:149` cuts on the budget. So score decides both **order and
membership**. The result comes back to `engine/turn_prompt.go:114`, is split into stable and
volatile by `isVolatileMemoryID` — **on the memory's ID, not on whether its content varies per
turn** — and the stable half becomes the whole `system` array (`:193`) carrying BP2 at `:198`
(`cacheIdx := len(systemBlocks) - 1`) and `:217`/`:223`.

The comment at `engine/turn_prompt.go:123-125` states the invariant the code breaks: *"Assemble
system blocks: ONLY stable content (cached via BP2). Volatile content goes into
e.Turn.VolatileContext … preventing cache busting."* Every input above is volatile; none is an ID
the split at `:114` recognizes.

**What inber should consider:** goose's rule generalizes one step further than the 07-30 entry took
it — **freeze the inputs to prefix assembly, not just the rendered strings**. Anthropic hashes
`tools → system → messages` in order, so a reshuffled system block invalidates BP2 *and cascades*
to BP3 and BP4, re-charging the whole conversation at the 1.25× write rate instead of the 0.10×
read rate. Against `docs/cache-optimization.md`'s own measured layout that is roughly 23k
write-equivalent tokens per turn, every turn it fires — and 2607.12161 (2026-08-01 sweep) puts cache
traffic at ~87% of the bill, so this is the dollar-dominant path, not a micro-optimization. Note the
constraint any fix inherits: all four Anthropic `cache_control` slots are already spent (BP1 tools
`agent/agent_run.go:35-37`, BP2 system, BP3+BP4 `agent/agent_run.go:129`), so "add a breakpoint for
the always-load head" costs one of the existing four. Note also that `engine/prompt_blueprint.go:142-148`
keys system blocks on a description string embedding `importance` at `%.1f` and the tag list, so the
blueprint diff reports a miss when the block *text* is byte-identical — verify this defect with a
content hash, not with that diagnostic.

### 2. An error's author decides what you may conclude from it — and inber *routes* on the answer

> **SPENT — verified 2026-08-03. Every line of this entry describes code that no longer
> exists.** The claim was accurate when this sweep ran at 04:12 and was fixed later the same
> day. `engine/failover.go` no longer maps any non-nil `err` to `RecordError`:
> `errorIsEvidenceAboutTheModel` (`1dff00b`, asserted by `0849ad6`, extended by `4f50869`)
> excludes `context.Canceled` and `agent.ErrMaxAPICallsExceeded` outright and asks whose clock
> fired on `context.DeadlineExceeded`, and `recordModelHealth` writes *nothing* for an outcome
> carrying no evidence rather than a fabricated success. Its comment names the three classes
> this entry asked for. `unexpected stop reason` is deliberately still recorded and is the open
> question on `6b4a9ab5` — deciding it here would settle it by accident. Do not re-file.

cline [#12820](https://github.com/cline/cline/pull/12820) stopped calling `captureProviderApiError()`
for every error event. Recoverable in-run tool-use mistake notices — the model emitting a malformed
call — were being counted as provider API failures, inflating the SDK bundle's measured error rate
**~9× versus legacy** in a live A/B rollout dashboard. The misclassification corrupted a decision,
not just a chart. The fix keys on one bit: *"Only terminal failures are provider failures.
`recoverable: true` error events are in-run notices."*

**What inber should consider, and inber's version is worse than cline's because it routes rather
than charts.** `engine/turn_execute.go:54` calls `recordModelHealth(modelUsed, …, err)` — its
comment says "regardless of success/failure" — and `engine/failover.go:97-98` maps *any* non-nil
`err` to `modelStore.RecordError`. Three of the errors reaching it are not provider failures:
`agent/agent.go:284` returns `ctx.Err()` on **user cancellation**, `:293` returns inber's own local
`exceeded max API calls (50)` cap, and `:393` returns `unexpected stop reason` (model/protocol
behaviour — `refusal`, `pause_turn`, `stop_sequence` all land there). model-store marks a model
unhealthy the moment `LastErrorAt` is after `LastSuccessAt` — a single error flips it, no threshold
— and `engine/failover.go:41-53` then fails over. Because model-store is a **host-shared SQLite
store**, one user pressing stop in dash degrades model selection for every other inber session on
the box until some session records a success, and the log line reads `model X is unhealthy (last
error: context canceled)` — a provider outage report naming a user action. cline's one `recoverable`
bit is not enough here; inber needs at least three classes (provider/infrastructure, model
behaviour, local policy or cancel), because only the first is evidence about a *model*.

### 3. "Sent" is not "accepted" — a submission is admitted when some turn takes ownership of it

> **SPENT — verified 2026-08-03. All three named sites were fixed the same day this sweep ran.**
> `Session.inject` is gone; `injectIfRunning` (`4b24b11`) returns a bool as one act rather than a
> status read followed by a send, and `injectIfBusy` falls back to the queue path when it answers
> false, so `session_release.go` no longer promises delivery it cannot make. `Server.Inject`
> (`session_management.go`) returns a typed `DeliveryRoute` — `mid-turn` or `next-turn`, with
> deliberately no route meaning "lost" — which is #36385's shape. `deliverResult`'s
> "🔔 Sub-agent completed" event was moved *above* the routing (`4b24b11`), so the busy branch no
> longer buries a finished child's result, and `requeueInjectionsTheTurnNeverReadLocked`
> (`1018979`) hands back a message the turn ended without reading. Do not re-file.

codex [#36385](https://github.com/openai/codex/pull/36385) adds
`submit_user_input_and_wait_for_admission`, which does not resolve until the message either starts a
new turn or steers the active one, and returns a `UserMessageAdmission` **naming the accepting turn
id**, with explicit errors for rejection and session termination. Its sibling
[#36410](https://github.com/openai/codex/pull/36410) makes `isBlocking` a required protocol field
instead of overloading `autoResolutionMs` as both "does this block" and "how long until timeout" —
and legacy payloads missing it **default to blocking**, the fail-safe direction.

**What inber should consider:** `server/session_release.go:88` calls `sess.inject(input)` and then
unconditionally returns `"[Message injected into running session — agent will see it during current
work]"`. `Session.inject` (`server/session.go:195-205`) returns **nothing**: it is a `select`/`default`
on a capacity-10 channel that logs a warn and drops the message when full. The promise is also racy
— `session_release.go:80` releases `s.mu` before the `:88` call, and `Session.turn`'s deferred func
sets `Status = Idle` under that same lock, so the turn can end in between; the message then lands in
a channel drained only by `InjectCheck`, which `agent/agent.go:297` calls only when `apiCalls > 1`.
No turn is running, so the user's prompt is neither answered nor queued until some later turn happens
to make a second API call. The same read-then-release-then-act shape repeats at
`server/session_management.go:112-120` (which `return nil` — success — regardless) and
`server/spawn_delivery.go:42-50` and `:95-108`; the last is the most damaging, because its idle
branch publishes the user-visible "🔔 Sub-agent completed" event that the racing inject branch skips,
so a finished sub-agent's result is buried with no notification to anyone. Adopt #36385's shape: give
`inject` a return value, and resolve a submission only once a turn owns it.

### Also in-window, worth a line each

- **A schema that names a value its own enum cannot contain.** `agent/chain.go:20` and `:24-25` both
  tell the model *"use `end_turn` when no follow-up is needed"*, but `:24` builds the enum from the
  live tool names, and `tools.EndTurn()` (`tools/tools.go:56`) has **zero non-test callers** — it is
  absent from the `init()` registry and from `tools.All()`. `agent/chain.go:101` still branches on
  `t.Name != "end_turn"`, so the code assumes a membership that never holds. Against a strict
  enum-validating backend (the Moonshot class `agent/openai_conversion.go:40-53` already defends
  against) that is a rejected argument; against a lenient one it is a silently dropped chain plus
  dead instruction tokens in every tool on every request. Related to but distinct from the open
  "no tool name is reserved" item, which names the same `if` for the opposite reason.
- **The sideband/chain injection is the largest single item in inber's tool block.** Measured against
  the real `AddChainAndSidebandFields`: `tools.AllFromRegistry()` (9 tools) goes **6,566 B → 17,834 B,
  +172%**, and the delta is exactly **1,252 B × 9** — the same `then` schema (full tool-name enum plus
  a ~250-char description) and the same `done`/`note`/`split` block repeated in every tool. Honest
  caveat, since goose #10409's thesis is about tokens: this sits inside BP1, so steady-state reads
  cost 0.1×. The real bills are cache **writes** at 1.25× on every new session and TTL expiry, and the
  OpenAI path (`engine/turn_openai.go:32,76`), which sets no cache control at all. Also: `then.tool.enum`
  grows linearly with tool count, so an MCP registry of hundreds inflates *every* tool's schema by the
  full enum — borrow codex [#36507](https://github.com/openai/codex/pull/36507)'s bounding discipline
  (a cap that is recency-prioritized **and reports what it dropped**) rather than its feature.
- **cline [#12831](https://github.com/cline/cline/pull/12831)** — restore must rewind untracked files
  too, or "undo" leaves a workspace state that never existed; see `cline.md` for inber's version.
- **Rejected this window, so the next sweep does not re-triage:** codex #36544/#36409/#36402/#36485
  (plugin packaging and search — explicitly no trust model attached), #36411 (test-infra markers),
  #36374 (sandboxed V8 build config), #36380/#36384 (thread-section CRUD, explicitly not context or
  compaction), #36339/#36364/#36327/#36311 (skill *rendering* moves; inber has no skill catalog),
  #36355/#36360 (same one-owner thesis as #36367/#36357, covered 2026-08-01; `tools/mcp` still has
  zero importers), #36329 (its sharp half — reserve the unnamespaced name — is already at
  `agentic-design-patterns.md:2037-2048`), #36534 (MCP catalog cap; inert with no MCP importers).
  cline #12836 (inber never parks an approval — `engine/build_hooks.go:97-99` refuses inline),
  #12658 (VSCode shell-integration specific), opencode #39697 (inber's MCP is stdio-only, no SSE).

**Held back from the queue this pass, and why.** codex
[#36373](https://github.com/openai/codex/pull/36373) (`--approve-for-me`) makes the approver a
*configured identity* — it expands one flag into `approvals_reviewer="auto_review"`,
`approval_policy="on-request"` and `sandbox_mode="workspace-write"`, then consumes itself so no
fourth mode reaches downstream code, and enforces the permission-affecting flags as a mutually
exclusive set **at parse time** (`conflicts_with_all`), so an incoherent trust envelope cannot be
assembled by accident. Two inber consequences. First, `guard.Config.ApprovalFunc` has **zero
producers repo-wide**, so Assist can only ever reach `NeedsApproval` → refuse
(`engine/build_hooks.go:97-98`, whose comment concedes "there is nowhere yet"); `auto_review` is the
shape that unblocks it without inventing an approval event. Second, and this is a live defect not
filed only because the open todo `9e31d359` already names the same lines: `server/spawn.go:224` and
`server/session_forking.go:47` pass a **zero** `RunRequest`, `applyRequestOverrides`
(`server/session_creation.go:57-95`) is entirely `if field != zero` guarded so nothing is set,
`guard.ParseMode("")` yields `Unset` — documented at `guard/guard.go:38-44` as *full access*, with
`guard/guard.go:154-156` returning `Allowed` from the `default:` branch — and `spawn_agent` is in
every session's tool set (`server/agent_tools.go:10-13`) while appearing in neither `isReadOnly`
(`guard/guard.go:249-256`) nor `isDangerous` (`:258-264`), so `guard/guard.go:146-153` allows it in
Assist **with no approval**. That todo covers the caps half and the resume path (shipped as
`88780d7`); the **mode** half, and its reachability through the Assist gate by the one tool nobody
classified, belongs on it and is not yet written there. Related: `guard/classification_test.go:56-88`
asserts only that every *classified* name exists, never that every *registered* tool is classified —
which is the asymmetry that left `spawn_agent` unclassified in the first place.

## Harness-watch — 2026-08-03: taking the worst of N answers is an error-rate trade, not a free safety margin

[goose #10870](https://github.com/block/goose/pull/10870) reverted
[#10416](https://github.com/block/goose/pull/10416) sixteen days after merging it, because
splitting a command into overlapping windows and **taking the maximum injection score** across them
"significantly increased false positives for large commands". Written up against the 07-16
prescription in `goose.md` (2026-08-03 §1); the part that generalizes past classifiers is the
arithmetic.

**Max-over-N is a monotone OR, so it multiplies the error it does not measure.** A detector with a
per-unit false-positive rate p, run over N units and aggregated by max, false-positives at
1−(1−p)^N. N is not a constant — it grows with the input — so the aggregate error rate is a
function of input size, and it grows fastest on the largest inputs, which are usually the ones the
fan-out was added for. The same shape appears everywhere this doc set already recommends a
worst-case merge: the deny-dominates lattice of the 2026-08-01 §1 entry, N-verifier adversarial
review, per-tool inspectors merged by "any inspector denies". Each is correct about *which* error it
refuses to miss and silent about what it does to the other one.

**What inber should consider:** whenever a design here says "take the worst of N", state N's range
and the per-unit rate, and calibrate the per-unit threshold so the *aggregate* rate is what you
meant — otherwise the threshold you tuned on one unit is not the threshold you shipped. Where the
merge is over *verdicts from different sources* (goose #10612's inspector lattice) the OR is
sound, because the sources are not independent draws from one noisy detector. Where it is over
*repeated draws from the same detector on slices of one input*, it is not.

## Harness-watch — 2026-08-04: a recovery gate must measure the quantity the recovery changes; a fallback is only legitimate for the one error it can actually answer; and a *section* of context should snapshot structured state, not its own rendering

### 1. cline ships overflow recovery as a four-layer contract — and inber's equivalent is dead code below 70 messages

[cline #12804](https://github.com/cline/cline/pull/12804) (+1472/−113) adds
context-window-overflow detection and recovery to the SDK architecture, which had
regressed relative to the legacy one. Four layers, connected by a typed value:
`classifyProviderError()` walks the raw error object (depth-bounded, cycle-safe) and
returns `context_window_exceeded | unknown`, with a deliberately conservative verdict
order — an explicit `context_length_exceeded` code wins, then a **429 status vetoes**,
then rate-limit *wording* vetoes, then any status outside `{400, 413, 422}` vetoes,
and only then do the overflow patterns fire. The class rides the model `finish` event.
The runtime retries **once per run** (re-armed on the next user message), and forces
compaction through the deterministic `basic` strategy rather than the agentic
summarizer, on a stated invariant worth quoting: *"recovery must not depend on another
successful LLM request"* — the summarizer budgets with the same estimator that just
undercounted, so it can overflow too. Three terminal states get named messages, and in
the *nothing left to compact* case **the doomed request is not re-sent** (the test
asserts the model saw exactly one request). A guard excludes turns that produced tool
calls, so partial work is not discarded. [#12814](https://github.com/cline/cline/pull/12814)
then puts a typed pre-pass in front of the structural walk, with two ideas worth
keeping: on a `RetryError` **only the final attempt decides** (retried-away rate limits
can neither veto nor fake the verdict), and on a typed `APICallError` the HTTP
`statusCode` **gates the whole verdict**, because in the structural walk a collected
status is merely an ambient signal with no attribution.

**inber has three of the four layers and the fourth is inert.** The classifier exists —
`agent/agent.go:22-32` `isContextLengthError` string-sniffs `"prompt is too long"`,
`"context_length_exceeded"`, `"maximum context length"`, `"too many tokens"` — and the
retry-once path exists at `agent/agent_run.go:205-218`, wired to a genuinely
deterministic `BeforeRequest` (`engine/build.go:118-157`: `ShouldPrune` →
`PruneConversation` → a hard head-drop, no LLM call anywhere). So inber already
satisfies cline's load-bearing invariant, and the doc comment at `agent/agent.go:86-90`
claiming pruning "can summarize, which is itself an API call" is simply wrong about its
own implementation.

The defect is the gate. `agent/agent_run.go:209` reads:

```go
pruned := a.BeforeRequest(ctx, *messages, a.contextWindow/2)
if len(pruned) < len(*messages) {
```

It accepts the recovery only if the **message count** shrank — but the pruning it just
called cannot change the message count. `ManageConversation` appends exactly one output
message per input message (`conversation/manage.go:170`, `finalMessages = append(...)`
inside the per-message loop), `ApplyStashing` allocates `make([]MessageParam, len(messages))`
(`conversation/stash.go:326`), and the tool-result truncation returns *copies*. The only
thing in the whole path that shortens the slice is the hard head-drop at
`engine/build.go:132-154`, which needs `len(messages) > KeepRecentTurns*2` — 70 messages
at the default role. So between 36 and 70 messages, `PruneConversation` runs, truncates
and drops tool results, may free tens of thousands of tokens, and **the agent throws the
result away and declines to retry**. Below 36 it frees nothing at all, which is the
separate count-bail already filed as noteboard `0db2e05c`. A corollary found while
reading it: `conversation/manage.go:173` computes
`result.ManagedMessages = len(messages) - len(finalMessages)`, which is therefore
**always 0**, and `PrunedMessages` mirrors it — the counter that would have exposed this
reports zero by construction.

**What inber should consider:** gate the retry on the quantity the recovery actually
changes. `PruneConversation` already returns `result.TokensFreed`; accepting on
`TokensFreed > 0` (or on a re-estimate below the window) costs nothing and makes the
existing recovery reachable. Note what the fix must decide and what this entry does not:
whether a recovery that freed *some* tokens but is still over the window should retry
anyway or fail immediately is cline's "nothing to compact" case, and inber has no
estimate of the non-message components to answer it with — `EstimateTokens`
(`conversation/manage_text_utils.go:110-117`) takes `[]MessageParam` and nothing else,
so the system prompt, memory blocks and the ~5–18 KB tools block are invisible to every
gate in the tree.

**Second, narrower gap: the OpenAI-compatible path has no overflow guard at all.**
`engine/turn_execute.go:30` dispatches to `runOpenAITurn` and returns; `buildAgent` — and
therefore `SetContextWindow` (`engine/build.go:114`) and `configureContextPruning` — is
only reached on the Anthropic branch at `:41`/`:48`. `engine/turn_openai.go:79-82`
wraps any error as `"OpenAI API call failed: %w"` and returns. So
`context_length_exceeded` from OpenAI, Google, OpenRouter or Ollama is an unrecoverable
turn failure with no compaction attempted. Noteboard `0db2e05c` measured this in
passing and explicitly left it unfiled; it is filed now.

**Third, and this one compounds:** `"prompt is too long"` falls through
`errorIsEvidenceAboutTheModel` (`engine/failover.go:166-175`) to `return true`, so an
overflow calls `modelStore.RecordError`. Per that function's own doc at `:98-104`
model-store's health table is host-shared, persistent, thresholdless and decay-free —
so inber's own context-management failure marks a healthy model unhealthy for every
session on the box, and `selectModel` then fails over, possibly to a model with a
*smaller* window, which cannot fix an over-long prompt. This is the same class the
function was written to exclude, and it is a fourth error class beyond the three the
2026-08-02 entry asked for: not provider, not model behaviour, not local policy, but
**a fault in inber's own context assembly**.

### 2. A fallback is legitimate only when scoped to "this is not the endpoint I asked for" — inber's spawn validation fails open on a registry outage

[goose #10189](https://github.com/block/goose/pull/10189) is unusually well-documented
because the PR body quotes the maintainer's repeated *rejection* of the naive fix:

> "`fetch_supported_models` isn't just a 'nice to have' model list — it doubles as the
> **configuration-correctness check**. … Falling back to the static list on *any* error
> makes that signal disappear. It also swallows `Authentication`, `CreditsExhausted`,
> and `RateLimitExceeded`: a user with a bad API key or exhausted account would sail
> past config with a fabricated model list and only hit a confusing failure later at
> chat time. That's why this catch-all approach keeps getting proposed and turned down —
> it trades a clear config-time error for a murky runtime one."

The shipped scoping: classify the HTTP status first, then read raw bytes, then decode —
and map **only** a JSON decode failure or a missing `data` array to `EndpointNotFound`,
which is the single class the static-list fallback answers. Everything that means *the
user's configuration is wrong* stays loud. Prompted by a misconfigured `base_url`
returning HTTP 200 and an HTML error page.

**inber has the exact shape, and unlike goose's the failure is a security-relevant
fail-open.** `agent/registry/spawn_tool.go:27-43` `fetchRegistryAgents` returns `nil`
for a transport failure (`:31`), a body-read failure (`:36`) and a JSON parse failure
(`:40`) alike — the HTTP status is never examined at all, so a 401 or a 500 whose body
does not parse is indistinguishable from "zero agents registered". Only the `:40` branch
is the legitimate fallback under goose's principle. The doc comment at `:26` states the
policy plainly: *"Falls back to an empty list if the registry is unavailable."*

The consumer is where it turns from degraded into unsafe — `spawn_tool.go:130-131`:

```go
agents := fetchRegistryAgents()
if len(agents) > 0 {
```

When the fetch fails, `len(agents) == 0`, the whole validation block is skipped, and
**any agent name the model invents is accepted** and emitted as a spawn request. A
registry outage silently converts a validating tool into a non-validating one — failing
open, in the direction that spawns work. The schema degrades in the same breath:
`validAgentsDescription` (`:46-56`) drops the enumeration of real agents and ships
*"Agent name to spawn. Must match a registered agent."*, so at the exact moment
validation stops, the model also stops being told what the valid names are. Third, a
cosmetic bug in the error path proves nobody has seen it fire: `:140-145` sizes
`names := make([]string, len(agents))` to *all* agents but assigns only the enabled
indices, so the message reads `Valid options: alpha, , gamma`.

**What inber should consider:** check the status, distinguish the three errors, and pick
a direction for each — goose's rule gives you the answer for the parse failure only. The
open decision, which is a blast-radius call and is deliberately **not** made here: on a
registry outage, should `spawn_agent` fail *closed* (refuse every spawn until the
registry answers, which strands an unattended job) or stay open with a loud diagnostic?
That is the same fail-open/fail-closed trade already live on `guard`'s `Unset` mode, and
picking it in an unattended sweep is how a trust decision gets made by accident.

Same window, same shape, lower stakes: [goose #10773](https://github.com/block/goose/pull/10773)
fixed stdio extensions **silently vanishing** when the config omitted `name` or spelled
`envs` as `env` — `parse_extensions_map` `continue`d past the deserialization failure and
the extension was simply gone. The two fixes are one line each (inject the map key as the
name; add `alias = "env"`), and the transferable rule is the shape, not the patch: *the
map key is the identity, accept the ecosystem-standard spelling, and never `continue`
past a parse failure without a loud signal.* inber's version is
`engine/build_tools.go:27-37`: `buildConfiguredTools` tries `buildSpecialTool` then
`findStandardTool`, and there is **no `else`** — a configured tool name matching neither
vanishes with no log, no error and no counter. This is reachable today, because
`ripgrep` and `end_turn` are both deliberately unregistered (`tools/tools.go:39-41`,
and `EndTurn()` at `:56` has no callers), so any config naming them, or any typo, starts
an agent that reports healthy while quietly missing a capability its config asked for.
`tools/tools.go:116-126` documents the same policy for the sibling helper — *"Unknown
tool names are silently ignored."*

### 3. A context *section* should snapshot the structured state, not a hash of its own rendering

[codex #36800](https://github.com/openai/codex/pull/36800) changes
`PermissionsState::Snapshot` from a bare `WorldStateHash` into a two-field enum:

```rust
pub(crate) enum PermissionsSnapshot {
    Current { instructions: WorldStateHash, approved_command_prefixes: BTreeSet<Vec<String>> },
    Legacy(WorldStateHash),
}
```

The `instructions` hash is now computed from the permissions block rendered **without**
the allowed-prefix list, and the prefixes live in their own set. `render_diff` gains an
arm: if the instructions hash is unchanged and the new prefix set is a strict *superset*
of the old, emit only the added prefixes and return. Approving one command used to
change the rendered block, change the hash, and re-emit the entire `<permissions
instructions>` message — profile line, approval policy, sandbox text and the whole
cumulative prefix list. Now the appended bytes are literally two lines. Prefix *removal*
is not a superset, so it correctly falls through and re-renders in full, and the
untagged `Legacy(hash)` variant keeps old snapshots deserializing. The out-of-band
injection path was deleted in the same change — `record_execpolicy_amendment_message`
is gone, because the world-state diff now owns that message and two owners would have
produced two copies.

**Correction to the framing this sweep started with, recorded so the next one does not
repeat it:** this is *not* a prompt-cache fix. Neither the PR body nor the diff mentions
caching, and world-state fragments are **appended**, so the old behaviour bloated the
tail and put two contradictory-looking permission blocks in context rather than
invalidating a cached prefix. Read it as context hygiene and single-ownership, and note
that the same PR's sibling [#36815](https://github.com/openai/codex/pull/36815) — which
swaps `Thread id: <uuid>` for `Agent name: /root/worker` in the token-budget block — is
likewise not a cache-stability change, because the block still carries per-session
random window UUIDs. Its value is that a subagent can name itself in the hierarchy, so a
budget or compaction instruction addressed to "you" resolves to a canonical path instead
of an opaque id the model cannot relate to anything.

**What inber should consider:** the generalizable rule is that a diffable context section
should key on the **structured state it represents**, not on a hash of the string it
produced — otherwise every field that participates in rendering is a false-positive
invalidation of every other field. inber's nearest equivalent is
`engine/prompt_blueprint.go`, and the 2026-08-02 entry already recorded that its system
blocks are matched **by position and hashed on `nb.Text`**, which is the right
instrument. The place inber does *not* hold this shape is the split at
`engine/turn_prompt.go:114`, which sorts memory into stable and volatile **on the
memory's ID** rather than on whether its content varies per turn — an identity test
standing in for a content test, which is the same substitution #36800 removed. That is
the already-filed `d23b4a8b`, not a new finding; what is new is that codex has now
shipped the structured-snapshot answer to it.

### Also in-window, worth a line each

- **[codex #36781](https://github.com/openai/codex/pull/36781)** — the most substantive
  *contract* change of the window. A tool's visibility becomes an orthogonal
  three-surface matrix (`ToolExposureSurface { CodeMode, Deferred, Direct }`, backed by a
  `bitflags` set), and an MCP server can opt out of any surface without disabling its
  tools everywhere: *"Servers need to be able to opt out of any of these surfaces without
  disabling their tools everywhere."* Also strips `_meta` from code-mode results —
  "MCP result metadata is private to clients and must not reach Code Mode." inber's tool
  surface is a flat list with a single on/off, so this is a shape to hold in reserve
  rather than a gap to close; `tools/mcp` still has zero importers.
- **[goose #10612](https://github.com/block/goose/pull/10612)** — 100 of 105 added lines
  are tests; the production change is one condition. Permission buckets must be
  **mutually exclusive and deny must be absorbing**: a request denied by one inspector
  and `RequireApproval`ed by another landed in *both* buckets, so an already-denied call
  could resurface in the approval queue and be approved by the user. Order-independent
  `Deny > RequireApproval > Allow`. inber has one gate, not a lattice, so this is
  prophylactic — but it is the concrete failure the 2026-08-03 entry's "deny-dominates
  lattice" caveat was abstractly about.
- **[goose #10545](https://github.com/block/goose/pull/10545)** — `canonicalize()` both
  sides *before* `starts_with`, or `nested/../../outside` and an outbound directory
  symlink both pass containment. The subtle bit worth copying: `continue` on a
  canonicalize failure rather than recording the path as loaded, so a directory that does
  not exist yet stays retryable instead of being permanently skipped once it appears.
- **[cline #12845](https://github.com/cline/cline/pull/12845)** — retry an empty model
  response at the model boundary, with the invariant that **a tool-call-only turn counts
  as content and is never retried** (the harness runs its own tool loop and wants those
  turns). Their stated reason for not using the vendor's own reliability layer is the
  reusable part: it *"owns the tool loop"*, executing tools itself, which "would hijack
  tool calling and break the agent loop."
- **[cline #12619](https://github.com/cline/cline/pull/12619)** — an MCP `list_changed`
  notification was being forwarded to a toast and the cached list was **never actually
  refreshed**; the notification was purely decorative. Fix registers typed handlers that
  do the refresh, debounced 300 ms per server per list kind, looking the connection up
  **fresh by name when the timer fires** so it is safe across delete/reconnect races.
- **Rejected this window, so the next sweep does not re-triage:** codex #36787 (model-
  instruction SSoT refactor; the one real behaviour change is that a model with no
  template now gets *empty* instructions rather than a silent default — the right
  direction, and inber has no template registry), #36830 (code-mode transport timeout),
  #36809 (state-db-first `exec resume --last`; the id-match guard against a stale index
  is the only notable bit), #36413 (pass-through plumbing of a Realtime API flag),
  #36812/#36810/#36811/#36793/#36796/#36792 (transport, conformance gates, shell policy,
  process-tree kills, plugin config parsing — #36792's model-capability gating of plugin
  prose is already at `agentic-design-patterns.md:1462`). goose #10808 (streaming shell
  output — UX, nothing architectural), #10870 (covered in the 2026-08-03 entry), #10223,
  #10766, #10663. cline #12791/#12790/#12719 (UI extraction), #12891 (AI SDK 7 bump),
  #12831 (already cross-referenced in the 2026-08-02 entry). opencode: nothing non-trivial
  landed — the window is release syncs, locale expansion, diff-viewer polish and a
  reverted "fix slow queries". aider, roo-code and dexto: no substantive commits.

**Filed to the queue this pass (3, the per-run cap):** `799b92c6` (the prune-accept gate
measures message count, which the pruner cannot change — both call sites),
`4d12d490` (spawn_agent validation fails open on a registry outage), `df1de352` (the
OpenAI path has no overflow guard, and an overflow demotes the model host-wide).

**Held back by the cap, verified against code this pass, so the next sweep does not
re-derive them.** Nine, roughly in descending order of sharpness:

- **A live credential fragment reaches the log on every client construction.**
  `agent/clients.go:112` — `log.Printf("[auth] creating Anthropic client: key_prefix=%s", key[:min(20, len(key))])`.
  The `sk-ant-api03-` prefix is 13 characters, so ~7 characters of real secret are
  printed each time. `redact/` exists in this repo and is not on this path.
- **An auth-store rejection is indistinguishable from "no credential configured", and
  then silently becomes an env credential.** `agent/clients.go:47-51` guards
  `auth.ResolveKey` with `if err == nil` and no `else`, so an expired or revoked token
  leaves `apiKey` empty; `:56` then falls through to `envKeyForProvider`. The one error
  that does surface (`:59-61`, "no credentials found") cannot distinguish "auth-store
  said no" from "auth-store was unreachable". This is exactly goose #10189's swallowed
  `Authentication`, and it is a fallback chain of the kind the repo directives forbid.
- **A model-store miss fabricates a provider rather than failing.**
  `agent/clients.go:29-33` swallows the `ResolveModel` error the same way; `:38-42` then
  builds a `Model` whose `Provider` comes from `guessProvider` (`:135-148`), a hardcoded
  prefix allowlist whose `default:` branch (`:145-146`) returns `"anthropic"` for **any**
  unrecognized model id. An unresolvable id is therefore routed to the Anthropic client
  and fails at the API, not at config time — the murky-runtime-error trade #10189 names.
- **Five divergent hardcoded default models**, against a single-source-of-truth store:
  `agent/models.go:164` (`claude-sonnet-4-20250514`), `agent/registry/config.go:159`
  (`claude-sonnet-4-5`), `cmd/inber-server/main.go:174` and `engine/lifecycle.go:92`
  (`claude-sonnet-4-5-20250929`), `server/api_oneshot.go:15` (`claude-haiku-4-5-20251001`).
  Two disagree about the model *generation*. The `config.go` one sits inside
  `LoadFromAgentStore`, whose own doc comment at `:79` reads *"This is the only source of
  truth for agent configuration"*.
- **A model-store read failure is reported to the user as "no healthy fallbacks".**
  `engine/failover.go:68-71` — `if err != nil || len(models) == 0 { return nil }` drops
  `err` unlogged, and `selectModel` then logs the `:59` line `"no healthy fallbacks,
  using %s anyway"`, which is untrue when the cause was a failed read.
- **A read error re-seeds a possibly-populated store.** `engine/engine_new.go:256` —
  `providers, _ := store.Providers()`; on error `providers` is empty, the code concludes
  the store is uninitialized, and calls `store.Seed()`.
- **The tool registry is a name-keyed map that overwrites silently.**
  `tools/interface.go:45-47` — `r.tools[tool.Name()] = tool`, with the doc comment
  conceding *"If a tool with the same name already exists, it will be replaced."* No id,
  no error, no diagnostic. This is the "join on ids, never on names" rule inverted, and
  it is the registration-policy gap the 2026-07-31 entry described upstream.
- **The sideband/chain enum is uncapped and grows the tool block quadratically.**
  `agent/chain.go:117-119` collects every tool name with no bound, `:50` embeds the list
  verbatim as `"enum"`, and `:128` injects that same full enum into **every** tool's
  schema. Measured against the real registry: 9 tools go 6,566 B -> 18,338 B (+179%);
  per-tool overhead rises 1,281 B at N=5 to 4,791 B at N=200, where the block is ~960 KB.
  Both call sites (`agent/agent_run.go:26`, `engine/turn_openai.go:32`) rebuild it every
  request. Mostly a cache-*write* and OpenAI-path cost, per the 2026-08-02 entry — but
  codex #36507's bounding discipline (cap, prioritize, and **report what was dropped**)
  is the shape, and inber has no cap at all.
- **The overflow retry silently downgrades a streaming turn.** `agent/agent_run.go:216`
  unconditionally calls `a.provider.Complete`, even when the original call went out via
  `CompleteStreaming` (`:168`). `result.Text` is still populated, so nothing breaks, but
  `hooks.OnTextDelta` never fires and a user watching a live stream gets the recovered
  response in one block.

Two more, already covered inline above and not repeated here: the silent drop of an
unresolvable configured tool name (`engine/build_tools.go:27-37`, §2) and the
always-zero `ManagedMessages`/`PrunedMessages` counters (`conversation/manage.go:173`,
recorded on `799b92c6`).

## Harness-watch — 2026-08-05: a deadline must name the thing it is bounding, and a policy read once is a policy that cannot be tightened — plus inber's second provider path is where its own invariants go missing

> ⚠️ **Scope caveat for §3 and half of §1, established before the findings and not
> after them.** Every enabled inber agent in agent-store runs an Anthropic model
> (`agent_harness`, `harness_id='inber'`: 15 × `claude-sonnet-4-5`, one each of
> `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-sonnet-4-5-20250929`), and **no
> agent configures a fallback at all** (`model_fallbacks` is empty or `[]` on every
> enabled row). So the OpenAI-compatible turn is reachable only by naming an OpenAI
> model on a `RunRequest`, never by failover. The defects below are **TRUE in code and
> LATENT on this host's corpus.** That is a priority argument, not a correctness one —
> the model-store registry carries the OpenAI models today, so the first session that
> asks for one gets all of it at once.

### 1. goose splits the transport deadline from the turn's duration — inber's Anthropic stream has neither bound, and its OpenAI turn loop has no runaway cap at all

[goose #10620](https://github.com/block/goose/pull/10620) (+418/−69) is the clearest
statement of a distinction harnesses keep collapsing. `ApiClient` set reqwest's
client-level `.timeout(600s)` — a **total request deadline that includes the streamed
body** — so it capped *turn duration* while claiming to measure *connection health*.
Every turn streaming past ten minutes died with `Stream decode error`, and in headless
`goose run` the reply loop treated that as terminal and ended the session silently. They
reproduced it on Terminal-Bench 2.1: all five regex-chess trials died at 600–601s. The
fix splits one knob into three — `connect_timeout(30s)` + `read_timeout(timeout)` on the
client, and the total deadline applied per request **only to non-streaming calls**, with
SSE call sites marked `.streaming(true)`. Streaming still bounds everything before the
body (connect, upload, first byte); only the body itself is exempt. Result: stream deaths
11/20 → 0/20, 23 turns over 600s survived, longest 2440s. They state the residual cost
plainly — *"worst-case hung-stream detection stays 600s (of silence)"* — which is the
point: a read timeout still catches a dead provider, it just stops catching a working one.

**inber has the two halves inverted.** The Anthropic path is the one that streams, and it
carries **no deadline of any kind**: `agent/clients.go:110-128` builds the client with
only egress redaction, auth and headers — no `option.WithRequestTimeout`, no custom
`http.Client` — and `agent/provider.go:54` calls `p.client.Messages.NewStreaming(ctx, *params)`
bounded only by `ctx`. That is correct by #10620's first rule and wrong by its second:
there is no read timeout either, so a provider that opens a stream and then goes silent
hangs a CLI turn with no deadline **forever**. The one deadline that does reach a live
stream is a policy budget, not a transport one — `server/spawn.go:303-305` wraps a
subagent turn in `context.WithTimeout(ctx, timeout)` at `defaultSpawnTimeout = 5 * time.Minute`
(`server/spawn.go:23`), and it is not idle-based, so a subagent streaming steadily for
five minutes is killed exactly like one that hung. Meanwhile the OpenAI-compatible client
carries the total deadline goose removed — `agent/openai.go:34`, `Timeout: 120 * time.Second`
— on a path that does **not** stream (`ChatCompletion` does `io.ReadAll`, and nothing in
the repo ever sets `"stream": true`), so it cannot kill a stream today but does cap a
legitimate long completion at 120s against a `MaxTokens` of 16384. That last one is
already filed; the two new ones are the missing read timeout and what follows.

**Reading the OpenAI turn loop for its bounds turned up that it has none.** The Anthropic
loop caps itself at `agent/agent.go:336-341`:

```go
const maxAPICalls = 50
if apiCalls > maxAPICalls {
```

with a named error (`agent.ErrMaxAPICallsExceeded`, `agent/agent.go:20`) that
`engine/failover.go:169` deliberately excludes from model-health evidence, because it is
*inber's own* runaway cap and says nothing about the provider. `engine/turn_openai.go:56`
is a bare `for {` with no counter, no cap, and no `ctx.Err()` check at the head. A model
served by OpenAI, Google, OpenRouter or Ollama that keeps emitting `tool_use` loops until
the context window fills or the caller cancels — every iteration a paid request. The cap
is not a tuning value inber lacks an opinion on; it exists, it is named, and one of two
turn loops enforces it.

**What inber should consider:** adopt goose's split explicitly rather than picking a
number. A *read* (inter-chunk) timeout on the Anthropic stream bounds a dead provider
without bounding a working turn; a *total* deadline belongs only on the non-streaming
OpenAI client, where it already is. And lift `maxAPICalls` out of `Agent.Run` into
something both loops consult, so the runaway cap is a property of a turn rather than of
one provider branch. **What a fix must decide, and this entry does not:** what the read
timeout should be, and whether a subagent's 5-minute budget should become idle-based
(which would let a long-but-healthy subagent run unboundedly) or stay a wall-clock
budget (which is what it is for). Those are blast-radius calls, not oversights.

### 2. codex re-reads the approval policy from the *current* configuration — inber accepts a mid-session `mode` change with a 200 and discards it

Four codex commits landed the same rule on 2026-08-04:
[#36930](https://github.com/openai/codex/pull/36930) "Read turn permissions from the
current configuration", [#36912](https://github.com/openai/codex/pull/36912) "Read
approval policy from the current turn configuration",
[#36901](https://github.com/openai/codex/pull/36901) "Propagate updated permissions to
review threads", and [#36941](https://github.com/openai/codex/pull/36941) "Use current
session settings for review threads". The shape: a permission policy captured once — at
session start, or into a spawned sub-thread — keeps being enforced after the user changes
it, and the stale value is usually the more permissive one. The fix is not a new
mechanism; it is deleting the capture.

**inber's guard passes the part of this test that the older audit failed it on, and fails
a different one.** ⛔ **The claim in `claude-code.md` §3 that `guard.CheckTool` "has zero
non-test callers and the mode is hardwired `guard.Autonomous`" is SPENT — do not repeat
it.** `CheckTool` has one production caller, `engine/build_hooks.go:94`, wired every turn
at `engine/build.go:105` and again on the OpenAI path at `engine/turn_openai.go:120`, and
it reads `g.cfg.Mode` live under the mutex per call (`guard/guard.go:165-188`) rather than
closing over a copy. `server/session_creation.go:88-89` plumbs `req.Mode` into the engine
config. Refusal is real enforcement, not advice — it short-circuits the primary call, the
`then` chain and the sideband fields (`agent/chain.go:389-395`). Observe mode is an
**allowlist** (`guard/guard.go:171-176` → `isReadOnly`), which is the stronger
construction than cline's blocklist in [#12906](https://github.com/cline/cline/pull/12906)
and immune to that PR's `sed -i`/heredoc bypass class.

The defect is one layer up, in the server. `mode` is a per-request field on `RunRequest`
(`server/server.go:231`), and `applyRequestOverrides` copies it (`server/session_creation.go:88-89`)
— but that function is reached only from `createSession`. `getOrCreateSession` returns an
existing session before looking at `req` at all:

```go
func (g *Server) getOrCreateSession(ctx context.Context, key, agentName string, ac AgentConfig, req RunRequest, onEvent func(StreamEvent)) (*Session, error) {
	if val, ok := g.sessions.Load(key); ok {
		sess := val.(*Session)
		sess.setOnEvent(onEvent)
		return sess, nil
	}
```

(`server/session_creation.go:23-28`.) The only field `Run` acts on for a live session is
`req.NewSession` (`server/server.go:325-329`). So on the second and every later `POST /run`
for a session key, `mode` is parsed, accepted, **200'd and discarded** — and there is no
other endpoint that carries it: `ConfigRequest` (`server/api_bridge.go:668-673`) has
model, effort, disabled tools and budget, and no mode. A user tightening a running
autonomous session to `observe` mid-conversation gets no error and no effect. It fails
open in the direction that matters. `server/mode_request_test.go` tests
`applyRequestOverrides` in isolation, so it passes while the wiring gap stands.

The related gap — a spawned child getting `RunRequest{}` → `ParseMode("")` → `Unset` →
allow-all, so an Assist parent produces an unrestricted child — is **already filed** on
noteboard as *"nine tool names reach the model unclassified, and spawn_agent is a door out
of the Assist gate"*, down to the same four-step chain, and is not re-filed here.

**What inber should consider:** codex's answer is that the policy has one reader and it
reads current state; inber's guard already does that, so the fix is entirely in
`getOrCreateSession`. **What a fix must decide:** which `RunRequest` fields are per-turn
and which are per-session. `applyRequestOverrides` copies eleven fields — model, thinking,
raw, no-tools, no-hooks, system, detach, mode and the four safety caps — and re-applying
all of them on every run would let a single request silently reshape a live conversation's
model and system prompt. Mode is the field with a security argument for per-turn
semantics and no competing endpoint; the rest is a design call, not an oversight.

### 3. The volatile-context channel does not exist on the OpenAI-compatible path — the fleet, the plan, the scratchpad and the stale-read warning are all built and dropped

[goose #10937](https://github.com/block/goose/pull/10937) is the OpenAI-compatible mirror
of the Anthropic fix in #10030: the volatile `<turn-context>` block sits mid-history during
a tool loop, and when it moves to the new user message at the turn boundary the entire
previous turn is re-billed. `formats/openai.rs` now strips it before formatting and
re-emits it at the tail, merged into a trailing user message when one exists (strict chat
templates reject consecutive user messages), so only the request tail changes. Measured
A/B on gpt-4.1-mini: turn-boundary cache hit 75.6% → 95.0%, re-billed history per boundary
−85%.

**inber's Anthropic path already has the good version of this, and its OpenAI path has no
version of it.** `engine/turn_prompt.go:126-152` splits the turn's context in two: stable
memories become the `system` array carrying BP2, and everything volatile is joined into
`e.Turn.VolatileContext` — fleet status, volatile memories, every registered context
injector, and the `[source: …]` ref. `engine/volatile_context.go:38-48` folds in the notes
queued earlier by context preparation. `agent/agent_run.go:92-121` then injects that string
into the **last user message, after the last `tool_result`** — at or after BP3, exactly
where goose put it — once per turn, clearing the field so the tool loop's own prefix stays
byte-stable.

That injection has exactly one entry point: `a.VolatileContext = e.takeVolatileContext()`
at `engine/build.go:40`, inside `buildAgent`. And `buildAgent` is on the Anthropic branch
only — `engine/turn_execute.go:29-31` routes an OpenAI client to `runOpenAITurn` and
returns. `runOpenAITurn` builds its system message from `systemBlocks` alone
(`engine/turn_openai.go:47-52`) and never reads `e.Turn.VolatileContext`, which
`engine/turn_prepare.go:94` then clears at the start of the next turn. The content is
assembled every turn and thrown away.

What an OpenAI-served session therefore never sees:

- the live agent fleet and their statuses — `server/session_context.go:31`, whose own doc
  comment explains why it must ride the volatile channel and not the system prompt;
- the live session status (`sessionStatusInjector`, same file);
- `task_plan` and `scratchpad` contents — `engine/engine_new.go:648-666`, i.e. the model's
  own plan and notes, on a path where the tools that write them still exist;
- the workspace roots — `engine/turn_prepare.go:107`;
- **the cross-zone stale-read note** — `engine/lifecycle.go:167-177`, the message that says
  *"these files were re-read since last context snapshot — ignore earlier versions"*. Its
  absence is the one with a correctness cost rather than a recall cost: the model keeps
  acting on a superseded file read with nothing telling it not to.

Note the irony worth recording: the drop makes the OpenAI prefix *more* cache-stable than
the Anthropic one, because there is nothing per-turn left to inject. That is not a defense.

**What inber should consider:** goose's `formats/openai.rs` is the template — strip and
re-emit at the tail, merged into a trailing user message. inber's Anthropic injector at
`agent/agent_run.go:92-121` already does the merge and already solves the harder ordering
problem (`tool_result` blocks must precede text), so the work is calling
`takeVolatileContext` from a place both branches reach and porting ~30 lines to the
OpenAI message shape. **What a fix must decide:** whether the volatile block goes into the
trailing user message (goose's choice, and inber's on Anthropic) or into the `system`
string, which is one line of code and would put per-turn bytes at the head of the prefix —
the exact thing #10030 and #10937 exist to prevent.

### 4. Checked against inber and already covered — recorded so the next sweep does not re-walk them

- **[codex #36954](https://github.com/openai/codex/pull/36954)** (tool registry collision
  policy) — the 2026-07-31 entry already describes the upstream shape, and inber's version
  is filed twice on noteboard (*"a duplicated tool name puts two definitions on the wire
  and dispatches to the second"*, *"tool names are load-bearing control flow, and no name
  is reserved"*). Re-verified: `engine/extra_tools.go:19-41` handles extras-vs-base
  explicitly **and logs a warning**, which is better than codex's pre-#36954 behaviour;
  the unhandled case is duplicates *within* the base set (`engine/build_tools.go:24-45`,
  no dedupe on an unvalidated `AgentConfig.Tools` list). The MCP dimension stays moot —
  `tools/mcp` still has zero non-test importers.
- **[codex #36998](https://github.com/openai/codex/pull/36998)** (deferred custom tools in
  tool search) — not applicable at inber's scale. A default server session carries 13–17
  tools; the deferrable weight is not the tool count but `AddChainAndSidebandFields`
  (`agent/chain.go:115`) injecting ~1.4–1.6 KB of identical `then`/sideband schema into
  *every* tool, which is the uncapped-enum finding already recorded on 2026-08-04.
- **[goose #10937](https://github.com/block/goose/pull/10937)**'s prefix-stability lesson
  applied to inber's *system* array reproduces the 2026-08-02 §1 finding exactly (memory
  selection keyed on this turn's user message, `Turn.Counter` and wall-clock recency).
  Both halves are filed. Independently re-derived this sweep and confirmed; not re-filed.
- **[cline #12906](https://github.com/cline/cline/pull/12906)** (plan-mode command
  blocklist) — inber's Observe mode allowlists rather than blocklists
  (`guard/guard.go:171-176`), so the bypass class does not exist here.
  **[cline #12929](https://github.com/cline/cline/pull/12929)** (remove model-initiated
  plan→act switching) — inber has no tool that writes `Guard.cfg.Mode`; the escalation
  route it does have is `spawn_agent`, already filed.
- **[cline #12948](https://github.com/cline/cline/pull/12948)** (flatten top-level tool
  schema unions) — the trigger is an MCP server advertising a top-level `anyOf`. Moot
  until `tools/mcp` has an importer; worth remembering at the moment it does.
- **[cline #12927](https://github.com/cline/cline/pull/12927)** (retry empty model turns on
  all providers) — inber has no empty-turn detection on either path, but the measured rate
  upstream is ~1.3 per 1k requests on OpenAI-compatible endpoints, which on this host's
  Anthropic-only corpus is not evidence of anything. Recorded, not filed.

## Harness-watch — 2026-08-06: cache locality is a property of the *provider*, not of the block being moved — and inber's OpenAI path cannot see a cache hit, cannot name a reasoning model, and enforces none of the session's caps

> ⚠️ **Same scope caveat as 2026-08-05, restated because three of the four findings below sit
> on the same path.** Every enabled inber agent in agent-store runs an Anthropic model and no
> enabled agent configures a fallback, so `runOpenAITurn` is reachable only by naming an
> OpenAI-compatible model on a `RunRequest`, never by failover. **TRUE in code, LATENT on this
> host's corpus.** That is a priority argument, not a correctness one.

### 1. ⛔ Correction to yesterday's §3: goose reverted the tail-relocation for the Responses stack, and inber's Anthropic path already does the version goose restored

[goose #10993](https://github.com/block/goose/pull/10993) walks back
[#10937](https://github.com/block/goose/pull/10937) for one class of model. #10937 — written up
here yesterday as the template inber should port — strips the volatile `<turn-context>` block
out of mid-history and re-emits it at the request tail on every request, so a turn boundary
stops re-billing the previous turn. That is correct for a **token-granular** cache. It is
actively harmful for OpenAI's **Responses stack** (gpt-5.x, o-series), whose implicit cache
reuses an entry only when the stored prompt is a byte **prefix** of the new request. Relocating
a block on every request means *no request ever extends its predecessor*, so hits stay pinned to
the static system-prompt/tools prefix for the whole session. Measured on gpt-5.6-terra via
OpenRouter: cached share went 4,710/~36,000 (13%) → 77–100%; on Terminal-Bench, median cache
share 7% → 63%, cached tokens per request 26% → 85%. The rule goose landed is conditional —
**Responses-stack models keep turn-context stationary on the user message it first arrived on;
everything else keeps the #10937 relocation.**

**inber already has the restored behaviour on Anthropic, and this changes what the open todo
should do.** `agent/agent_run.go:92-121` injects `VolatileContext` into the last user message
once per turn and clears the field, so the block stays where it landed and the tool loop's own
prefix is byte-stable — stationary, which is #10993's rule, not #10937's. Anthropic's cache is
explicit-breakpoint prefix caching, so this is the right shape for it.

**What inber should consider:** todo `ec9c7122` (port the volatile channel to `runOpenAITurn`)
says its open decision is "trailing user message vs system string". #10993 adds a third answer
and rules out a fourth: **do not re-emit the block on every request.** Write it into the user
message that opens the turn and leave it there, which is what `agent/agent_run.go` already does —
porting the Anthropic injector verbatim is now the *cheapest and the most correct* option, where
yesterday it read as merely convenient. The generalisation is worth keeping: **before moving a
block to improve cache locality, establish whether the cache is prefix-matched or
token-granular. The same edit doubles the hit rate on one and floors it on the other.**

### 2. cline switches `max_tokens` → `max_completion_tokens` from the model id — inber's version is a two-prefix `HasPrefix`, and the 400 it earns is filed as the model being unhealthy

[cline #12902](https://github.com/cline/cline/pull/12902) fixes a hard first-request failure on
the generic OpenAI-compatible provider: OpenAI rejects `max_tokens` for reasoning-era models with
*"Unsupported parameter: 'max_tokens' is not supported with this model. Use
'max_completion_tokens' instead."* cline deliberately did **not** reach for catalog metadata —
on a generic endpoint the model id is free-form user input and no catalog can answer for it — and
instead matches `o[134]` and `gpt-5` with **non-alphanumeric boundary anchoring**, so `gpt-4o`
and `yolo1` cannot match while the namespaced `openai/o3-mini` can.

**inber's is the naive version of exactly that, one line long:**

```go
// engine/turn_openai.go:69-73
// o-series models (o1, o3, etc.) require max_completion_tokens instead of max_tokens.
if strings.HasPrefix(client.Model, "o1") || strings.HasPrefix(client.Model, "o3") {
	req.MaxCompletionTokens = 16384
} else {
	req.MaxTokens = 16384
}
```

Three misses, each a hard 400 on the first request of the session: **`o4-*`** (the branch names
o1 and o3 only), **the whole `gpt-5` family** (reasoning models under OpenAI's current rule), and
**every namespaced id** — `openai/o3-mini`, `azure/o3` — which is not incidental, because
`agent/clients.go` routes openrouter through this same client and OpenRouter ids are all
namespaced. Note the *only* answer that is not a bare error is the false negative: a model that
needs `max_completion_tokens` and does not get it fails loudly.

**What it costs beyond the 400.** `client.ChatCompletion` returns the error, `runOpenAITurn`
wraps it (`engine/turn_openai.go:99`), and `engine/turn_execute.go:56` hands it to
`recordModelHealth`. `errorIsEvidenceAboutTheModel` (`engine/failover.go:127-160`) excludes three
things — a cancel, inber's own deadline, `ErrMaxAPICallsExceeded` — and a provider 400 is none of
them, so `modelStore.RecordError` fires and the model goes unhealthy in a **host-shared** store.
A parameter-name mismatch in inber's own request builder is therefore recorded as the provider
being at fault, for every other harness on this box. Same compounding shape already recorded
against the overflow path (`df1de352`).

**What inber should consider:** take cline's boundary-anchored matcher rather than widening the
prefix list, because the failure mode of a prefix list is that it silently stops covering the
next model family. **What a fix must decide, and this entry does not:** whether the answer
belongs in inber at all. The single-source-of-truth move is a capability field on the model-store
row — but `modelstore.Model` (`~/repos/model-store/store.go:25-42`) carries id, provider, name,
short name, aliases, context window, two prices, enabled and priority, and **no capability of any
kind**, so today there is nothing to read. Adding one is a change to the owning store and to
every seeded row; a local matcher is a change to one line. cline chose the matcher *and said
why* — free-form ids on a generic endpoint have no catalog row. inber's ids come from model-store
for a configured model and from the request for an ad-hoc one, so it has both cases and has to
pick per case. Adjacent and already filed, do not merge: `e68b05e0`, reasoning **effort**
discarded on the same path.

### 3. inber cannot observe a cache hit on the OpenAI path at all — the wire fields are not in the struct, so every cached token is priced as fresh input

The half of #10993 that is not about placement is that it is **measured**: the whole PR is
argued in cached-token share. [opencode #40450](https://github.com/sst/opencode/pull/40450) is
the same instinct from the other end — cache *writes* were missing from the usage it reports over
ACP, so its own accounting understated what a session cost.

**inber's OpenAI-compatible usage type has three fields and none of them is a cache figure:**

```go
// agent/openai_types.go:64-68
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
```

`prompt_tokens_details.cached_tokens` — the field every OpenAI-compatible endpoint that caches
reports — is not parsed, so it is dropped at the JSON boundary.
`agent/openai_conversion.go:196-199` and `:236-239` then build `anthropic.Usage` with
`InputTokens`/`OutputTokens` only, leaving `CacheReadInputTokens` and
`CacheCreationInputTokens` at zero. `CalcCostWithCache` (`session/timeline_cost.go:49-55`) is
called with `cacheRead=0, cacheWrite=0` on this path by construction, so **every cached token is
billed at the full input price** — and, worse for the work in §1, there is no number that could
tell anyone whether a cache change on this path did anything. You cannot fix a cache you cannot
see.

**What inber should consider:** parse `prompt_tokens_details` and carry it through the
conversion. **What a fix must decide, and it is a real trap rather than a mechanical port:**
the two providers disagree about what the input count *contains*. Anthropic's `input_tokens`
**excludes** the cached portion; OpenAI's `prompt_tokens` **includes** it. Passing OpenAI's
`prompt_tokens` as `inTok` alongside `cached_tokens` as `cacheRead` double-charges the cached
span — which is the mirror image of the bug the comment at `session/timeline_cost.go:30-43`
records inber already shipped once, and closed as `00093e48`. So the decision is where the
inclusive→exclusive normalisation happens: in the conversion (making `anthropic.Usage` mean one
thing everywhere, at the cost of a lossy transform in a layer that is supposed to be
transparent), or in the cost function (which then needs to know which provider produced the
numbers it was handed, and today it does not).

### 4. The session's configured caps and the mid-turn injection channel are attached inside `buildAgent` — so the OpenAI turn honours neither

[codex #37114](https://github.com/openai/codex/pull/37114) adds per-session execution limits to
code mode; the transferable part is only that a limit belongs to the *session*, not to whichever
execution path happens to run it.

`buildLimitCheck` (`engine/build_hooks.go:37-72`) enforces the three caps an operator can
actually set — `MaxInputTokens`, `MaxTurns`, `MaxResponseTime`, the last being the orchestrator
speed enforcement that tells a slow orchestrator to answer or spawn. `buildInjectCheck`
(`:14-30`) drains `e.injections`, the channel a mid-turn steer arrives on. **Both are wired in
exactly one place** — `engine/build.go:98` (`a.InjectCheck = e.buildInjectCheck()`) and
`engine/build.go:109` (`a.SetLimitCheck(e.buildLimitCheck())`) — and both of those lines are
inside `buildAgent`, which `engine/turn_execute.go:29-31` skips outright when the client is
OpenAI-compatible. `runOpenAITurn`'s loop calls `e.buildHooks()` and neither of the other two.

So on an OpenAI-served session: the token budget, the turn cap and the response-time cap are
accepted by `applyRequestOverrides`, stored on `e.Limits`, and never read; and a message injected
while the turn is running is never drained mid-turn. The injection half is bounded rather than
lost — `Session.turn` requeues anything unread onto `pendingMessages` (the `1018979` fix) — so it
surfaces at the next turn instead of never, which is a delivery-latency bug, not a data-loss one.
The limits half has nothing catching it.

This is the fourth instance of one root cause and it is worth naming as such: **`buildAgent` is
where inber attaches everything that makes a turn a *governed* turn, and one of its two turn
loops does not go through it.** The three already filed are the volatile context (`ec9c7122`),
the context-overflow guard (`df1de352`) and the runaway API-call cap (`9fb35070`, which is the
hardcoded `maxAPICalls = 50` and **not** these configured caps). **What a fix must decide:**
whether to keep porting hook-by-hook, or to split `buildAgent` into a provider-independent
"govern this turn" half and an Anthropic-specific "build the SDK agent" half so the next hook
added cannot land on one path only. The second is the fix that stops the pattern; it is also a
refactor of the hottest function in the engine, which is a blast-radius call, not an oversight.

### 5. Checked against inber this window and NOT worth a finding — recorded so the next sweep does not re-walk them

- **[goose #10992](https://github.com/block/goose/pull/10992)** (byte-bound the shell truncation
  preview) — inber already landed this class as `47c7cefe`. `internal/textutil` cuts to a byte
  budget on a rune boundary, `session/truncate.go:98-127` uses it for both head and tail, and 27
  call sites across 17 files route through it. Re-verified, nothing to do.
- **[codex #37190](https://github.com/openai/codex/pull/37190)** (interrupt the turn after one
  Guardian denial) — the underlying observation transfers: inber's refusal is a *string returned
  as a tool error* (`engine/build_hooks.go:90-101`), the loop `continue`s, and nothing counts
  denials, so a model refused in Observe mode can walk the tool list rephrasing until
  `maxAPICalls` (or forever on the OpenAI path). **Not filed** because the mechanism that would
  count it already exists and is already filed as dead: `guard.RecordToolCall` (`guard/guard.go:209`)
  and `IsRepeating` (`:305`) have never run — todo `db1817cb`. Whoever picks that up should decide
  there whether a repeated *denial* is the repetition worth acting on, rather than have it filed
  twice.
- **[goose #10986](https://github.com/block/goose/pull/10986)** (make shell approval titles
  faithful — the approval prompt named a different command than the one that would run) — moot
  here. `ApprovalFunc` is set by no session in the tree, which is why `CheckTool` answers
  `NeedsApproval` rather than asking; there is no approval surface to be unfaithful. Becomes live
  the moment one is built, and the rule to carry over is that the string shown to the approver
  must be the string handed to the tool.
- **[codex #37204](https://github.com/openai/codex/pull/37204)** (durable user-message queue
  dispatch) — inber's queue is `pendingMessages`, in memory, and its admission contract is
  already open as `5320b48f`. Durability is one of the choices that todo has to make; adding a
  second todo would fragment it.
- **[codex #37188](https://github.com/openai/codex/pull/37188) / [#37022](https://github.com/openai/codex/pull/37022)
  / [#37020](https://github.com/openai/codex/pull/37020) / [#37053](https://github.com/openai/codex/pull/37053)**
  (reserve the `tool_search` namespace; canonicalize defaults under `functions`; strict collision
  and conflicting-description errors) — the same cluster as #36954 last week, and inber's version
  is filed twice already (*"a duplicated tool name puts two definitions on the wire"*, *"tool
  names are load-bearing control flow, and no name is reserved"*). Re-verified unchanged.
- **[codex #37038](https://github.com/openai/codex/pull/37038) / [#37040](https://github.com/openai/codex/pull/37040)
  / [#37031](https://github.com/openai/codex/pull/37031)** (turn environment permissions, applied
  to future turns) — the 2026-08-05 §2 cluster continuing; inber's gap is `getOrCreateSession`
  and is filed as `c14cd190`. Nothing new.
- **[cline #12928](https://github.com/cline/cline/pull/12928)** (Bedrock wants Converse
  `cachePoint` markers, not Anthropic `cache_control`) — no Bedrock path in inber;
  `agent/clients.go` names anthropic, openai, google, openrouter, ollama and a catch-all. Worth
  remembering as the shape *"a cache marker is provider-syntax, not a portable concept"* if a
  Bedrock credential is ever added, since inber's four `cache_control` blocks are spent by name
  (`bd706121`).
- **[cline #12953](https://github.com/cline/cline/pull/12953)** (a recoverable error must not
  kill a turn that completed with a plan) — the same family as #12820, already written up
  2026-08-02 §2 and filed as `69f05f89`. The inber-specific version, `runOpenAITurn` treating
  `max_tokens` as a clean `end_turn` (`engine/turn_openai.go:107-115`), is a stop-reason question
  and belongs on `6b4a9ab5`.
- **[codex #37151](https://github.com/openai/codex/pull/37151)** (coalesce concurrent git status
  scans) — a singleflight around a repeated subprocess. inber runs git at close
  (`321d7d49` territory), not per-turn against a shared cache, so there is nothing to coalesce.

## Harness-watch — 2026-08-07: a prefix survives because nothing already sent ever moves — goose turns that into a declared per-provider contract *and a test*, and inber's own provider contract is four tables that disagree

### 1. goose stops relocating cached bytes altogether, declares the cache contract per (provider, model), and pins it with a prefix-invariance test

[goose #11022](https://github.com/block/goose/pull/11022) closes the two-entry arc this doc
recorded on 2026-08-05 (#10937 relocates turn-context to the tail) and 2026-08-06 (#10993 walks it
back for the Responses stack). The resolution is not a third placement rule. It is the observation
that **persistence and relocation cannot coexist**, so goose keeps persistence and deletes
relocation. Three pieces:

- **Append-only turn context.** The turn-context block renders **once**, at turn start, and is
  persisted as a standalone agent-only user message carrying a `turnContext` marker. It is never
  spliced into an existing message and never re-emitted. The stated invariant is *"bytes already
  sent to a provider are never moved or edited"* — so *"every request in the turn, and in every
  later turn, carries identical bytes at identical positions."* The per-format relocation
  machinery (Anthropic tail relocation, OpenAI chat extraction) is deleted; the PR is **−153
  production lines** with the growth in test assertions.
- **Extension parts frozen at turn start.** Todo lists, timer counts and code-mode metrics stop
  churning mid-turn — the same rule applied to the things that render *into* the block.
- **A declared `CacheSemantics` table keyed by (provider, model)**, naming which contract each pair
  has: explicit breakpoints (Anthropic, Databricks, OpenRouter) or implicit prefix matching (OpenAI,
  Responses). **An unknown pair defaults to strict byte append-only** — the most conservative
  contract, not the most convenient one. One shared breakpoint writer
  (`apply_chat_payload_breakpoints`) replaces the scattered per-provider copies across OpenRouter,
  LiteLLM and Databricks.
- **`prefix_invariance.rs`**, a test harness asserting *request N is a cache-valid prefix of request
  N+1* across every format, with **seeded regressions** proving it detects the bug classes that were
  already shipped. Measured after: Anthropic ~8k input tokens served from cache on resume; OpenAI
  gpt-5-mini 93–98% of input cached; Moonshot 96% on resume.

**Where inber already stands, checked rather than assumed.** The placement half is done and was
done before this PR: `agent/agent_run.go:93-121` injects `VolatileContext` into the last user
message once per turn and clears the field, and `agent/volatile_context_writeback_test.go` pins
that the write lands in the caller's slice (`&e.Messages`) rather than a request-local copy — so the
block is persisted, stationary, and inherited by a fork. That is #11022's shape, arrived at
independently. The mid-turn-churn half is also clean: the one thing that changes between API calls
of a turn is tools being withheld under `forceSummary` (`agent/agent_run.go:78-80`), which is
deliberate, documented, and already priced and named by the cache-miss report
(`engine/cache_miss_report_test.go`).

**What inber should consider:** the third piece, which inber has nothing of. inber measures cache
misses **at runtime** — `PromptBlueprint` diffs turn N's blocks against turn N−1's and
`engine/cache_miss_report_test.go` pins that a re-bought prompt is named to an operator — but there
is **no test that runs a sequence of turns and asserts the byte-prefix relation between successive
requests**. The blueprint tests assert the *diff reports* correctly; they do not assert the prefix
*holds*. The standing open todo — the BP2-cached system prefix rebuilt from a clock, a turn counter
and the user's message text — is precisely the failure a `prefix_invariance` test fails on, and it
was found by hand-probing a memory-store DB instead. **The transferable artifact is the seeded
regression**: a test that cannot be shown to catch the bug you already shipped is a test you cannot
trust to catch the next one.

### 2. inber's provider contract is four hardcoded tables that must agree and do not — Google's base URL is wrong, ollama is unreachable, and zhipu exists in exactly one of them

> **FILED 2026-08-07 as todos `95e1f1aa` (the Google base URL) and `7ec4c2da` (the four
> disagreeing tables). Do not re-file.** They are separate because their fixes are different
> sizes; they share the decision in the last paragraph and should be decided together.

#11022's `CacheSemantics` is one declared table replacing scattered per-provider copies. inber has
the copies and no table. `agent/clients.go` answers four separate per-provider questions in four
separate switches, and **no two of them list the same providers**:

| function | line | providers it knows |
|---|---|---|
| `guessProvider` | `:135-148` | anthropic, openai, google, **zhipu**, default→anthropic |
| `newClientFromKey` switch | `:82-100` | anthropic \| openai, google, openrouter, **ollama** \| default |
| `envKeyForProvider` | `:151-162` | anthropic, openai, google, openrouter |
| `defaultBaseURL` | `:165-176` | openai, google, openrouter |

Three live consequences, each verified in code:

- **Every Google model 404s.** `defaultBaseURL` returns `https://generativelanguage.googleapis.com/v1beta`
  (`agent/clients.go:170`) and `agent/openai.go:52` appends `/chat/completions`. Gemini's
  OpenAI-compatible endpoint is `https://generativelanguage.googleapis.com/v1beta/**openai**/chat/completions`
  ([Google's own docs](https://ai.google.dev/gemini-api/docs/openai)); `/v1beta/chat/completions`
  is not an endpoint on that host at all. The auth half is fine — Gemini's compat endpoint takes
  `Authorization: Bearer <key>`, which is what `agent/openai.go:58-59` sends — so the whole break is
  one missing path segment. It costs more than a 404: `ChatCompletion` returns
  `API error 404: ...` (`agent/openai.go:73`), `engine/turn_execute.go:56` hands it to
  `recordModelHealth`, and `errorIsEvidenceAboutTheModel` (`engine/failover.go:127-160`) excludes
  only a cancel, inber's own deadline and `ErrMaxAPICallsExceeded` — so `modelStore.RecordError`
  fires and **every Gemini model is marked unhealthy in the host-shared model-store**, blaming
  Google for a typo in inber's URL, for every other harness on this box. Third instance of that
  compounding shape (`df1de352`, `25b91c78`).
- **ollama is named at `:92` and reachable by nothing.** It has no entry in `defaultBaseURL`, so it
  gets `baseURL == ""`, and no entry in `envKeyForProvider`, so a local server that needs no key
  is rejected at `agent/clients.go:59-60` with *"no credentials found for provider"* before it ever
  reaches the switch. If a placeholder key is stored to get past that guard, the request POSTs to a
  bare `/chat/completions` with no host. The 2026-08-03 cline entry already reasoned about ollama's
  timeout on this path; that reasoning was about a provider that cannot currently be selected.
- **zhipu is invented at `:143-144` and honoured nowhere.** `guessProvider` returns it for `glm-*`
  and `zai/*` ids; it is absent from the client switch, the env-key map and the base-URL switch, so
  it falls through the `:97` catch-all to `NewOpenAIClient("")` — the same hostless POST.

And the framing #11022 supplies: **the default for an unknown pair should be the conservative
contract.** goose's unknown (provider, model) falls back to *strict byte append-only*, the rule that
is safe under every cache. inber's unknown model id falls back at `agent/clients.go:145-146` to
`"anthropic"` — the provider with the *most* specific wire contract in the tree (explicit
`cache_control` breakpoints, thinking blocks, a distinct tool-schema shape). It is the fail-open
default in the place goose deliberately chose fail-closed.

**What a fix would have to decide, and this entry does not.** Whether provider transport belongs in
inber at all. The single-source-of-truth answer is a base-URL (and capability) column on the
model-store row, which is where `provider` already comes from — but `modelstore.Model`
(`~/repos/model-store/store.go:25-42`) carries no transport field of any kind, so adding one is a
change to the owning store and to every seeded row. The local answer is one declared table replacing
the four switches, which is a change to one file and leaves the store's rows silent about how to
reach them. This is the same fork the 2026-08-06 `max_completion_tokens` entry hit, and it should be
decided once for both rather than twice differently. Separately and smaller: whether `guessProvider`
should exist. It fires only when `ResolveModel` fails, and its honest answer there is an error.

### 3. goose makes the agent loop re-entrant by declaring the conversation to be the execution state

[goose #9574](https://github.com/block/goose/pull/9574) replaces the coroutine agent loop with a
re-entrant state machine behind `GOOSE_STATE_MACHINE`. *"Each call runs from the persisted
conversation to the next applied step or client yield; no coroutine state is held between steps."*
Commands, tools, context management, hooks and inference all become one `Operation` protocol;
`StateMachine::step` runs the ordered operations until one returns `Applied`; per-operation notes
are stored **on the messages** under the operation name. The claim that makes it testable:
*"reconstruction tests discard and rebuild the pipeline after each applied step, proving that
progress comes from persisted state rather than operation instances."*

**inber's turn loop is the coroutine shape.** `Agent.Run` (`agent/agent.go:311-450`) holds
`turnAnchorIdx`, `tools`, `apiCalls` and the accumulating `*TurnResult` in Go locals for the whole
turn, and `runOpenAITurn` (`engine/turn_openai.go`) holds a second, differently-shaped set. Nothing
outside those frames can answer "where is this turn". That is the structural reason the same hook
keeps landing on one loop and not the other — the fourth instance was written up yesterday
(2026-08-06 §4) — and it is why a crash mid-turn cannot resume mid-turn.

**What inber should consider:** not the rewrite. The cheap half is goose's *test* idea applied to
the state inber already persists: a reconstruction test that, after each API call of a turn,
rebuilds the engine from `e.Messages` + the turn counter and asserts the next call is identical.
Whatever that test cannot reconstruct is exactly the per-turn state that lives only in a Go local,
and that list is the input to any decision about splitting `buildAgent`. It is a measurement, not a
refactor, and it is the thing the open blast-radius question is currently missing.

### 4. Checked against inber this window and NOT worth a finding — recorded so the next sweep does not re-walk them

- **[codex #37347](https://github.com/openai/codex/pull/37347)** (track context windows per agent —
  a fork inherits compacted history but must start a *distinct* window lineage, and inherited
  compaction metadata is reset to the child's initial window) — inber's fork does the deliberate
  opposite and says why: `server/session_forking.go:57-58` calls
  `child.Engine.RestoreSession(parentMessages, parentTurnCounter)` so the child's BP3 lands on the
  boundary the parent already cached instead of re-staging the inherited transcript. That is a cache
  decision, and codex's is an identity decision; they do not conflict. The identity half was covered
  2026-06-20 and nothing here changes it.
- **[codex #37188](https://github.com/openai/codex/pull/37188)** (reserve the `tool_search`
  namespace) — same cluster as #36954 and #37022, filed twice already. Unchanged.
- **[codex #37190](https://github.com/openai/codex/pull/37190)** (interrupt the turn after one
  Guardian denial) — re-appeared in this window's list; already dispositioned 2026-08-06 §5 onto
  todo `db1817cb`. Do not re-file.
- **[codex #37367](https://github.com/openai/codex/pull/37367) / [#37368](https://github.com/openai/codex/pull/37368) /
  [#37369](https://github.com/openai/codex/pull/37369) / [#37371](https://github.com/openai/codex/pull/37371)**
  (fork in `codex exec`; restore the approval policy when resuming a thread; archive and un-archive
  from the resume picker) — the resume-fidelity family. #37368 is the one with a shape worth keeping:
  *a resumed thread must come back under the policy it was running under, not the ambient default.*
  inber has no analogue to lose, because it has no persisted per-session policy — `disabled_tools`
  is engine memory (goose.md §2, already written up) and `mode` comes from config on every revive.
  It becomes live the moment either is persisted.
- **[codex #37206](https://github.com/openai/codex/pull/37206)** (a unified image budget) — inber
  sends no images on either turn path; `agent/openai_conversion.go` builds text and tool blocks only.
  Nothing to bound.
- **[codex #37261](https://github.com/openai/codex/pull/37261)** (start cached MCP servers lazily for
  subagents) — `MCPToolRegistry` still has no caller outside `tools/mcp/`, re-verified. A cost for
  whoever wires MCP, not one today.
- **[cline #12906](https://github.com/cline/cline/pull/12906)** (a plan-mode command blocklist on
  `run_commands`) — inber's mode gate is `guard.CheckTool` and the live gap there is the Assist
  denylist failing open, counted and re-counted on the existing todo. A second blocklist would
  fragment it.
