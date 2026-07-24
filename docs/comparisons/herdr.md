# Herdr Comparison

**Project**: [Herdr](https://herdr.org/)
**Language**: Not disclosed (hosted platform; no public source or architecture)
**Focus**: Meta-harness of the *orchestration* kind — a control layer that sits **above** many agent harnesses and routes work between them, rather than a harness that runs an agent
**Key Claims**: Context/cost/policy/urgency-aware routing, shared workflow memory across agents, human gates for sensitive actions, fleet observability (trace decisions, inspect handoffs, measure throughput)

> **Two projects share this name — do not conflate them.** This doc covers
> **herdr.org**, the hosted agent-orchestration platform. A separate project,
> **herdr.dev** ([github.com/ogulcancelik/herdr](https://github.com/ogulcancelik/herdr)),
> is a Rust TUI terminal multiplexer — "tmux for AI coding agents" — with 14+ agent
> integrations (Claude Code, Codex, Pi, Amp, Droid, Hermes, OpenCode, Grok, …), built on
> ratatui/crossterm/tokio, client-server with detach/reattach, AGPL-3.0. That one is a
> workspace/observability shell closer in spirit to `commander.md` and inber-party, not an
> orchestrator. See the last section.

## What "meta-harness" means here

The term carries two meanings in the 2026 literature (see `meta-harness.md`):

1. **Harness optimization** — an outer loop that *searches over harness code* (prompts,
   retrieval, memory, tools) to improve a frozen model. This is the arXiv Meta-Harness
   paper (2603.28052) and AgentFlow (2604.20801).
2. **Harness orchestration** — a control layer *above* many agent harnesses that routes,
   composes, gates, and observes them. This is Herdr, Databricks Omnigent, and friends.

Inber spans both: it is itself an orchestration-type meta-harness (registry + bus topology +
per-turn agent runtime), which makes Herdr a same-category comparison rather than a
tool inber merely wraps.

## Architecture Overview

Herdr publishes a capability description, not an architecture. From its site, it presents as a
hosted control plane over a fleet of agents:

```
Incoming task
    → Router        — pick model / tool / specialist agent by context, cost, policy, urgency
    → Workflow memory — shared state: prior decisions, system rules, human feedback
    → Human gates    — escalate sensitive actions without stalling the automated flow
    → Fleet observability — trace decisions, inspect handoffs, measure throughput
```

Positioning: "turn many autonomous workers into one managed system"; "modern AI teams need
coordination, not another isolated bot." Advertised use cases are org workflows — customer
operations triage, sales research, engineering intake, compliance. A live counter on the site
reports 18 active agents, 2,841 routed tasks, and 4 manual reviews, i.e. it sells a
high-autonomy / low-human-touch routing story.

Because no source or design doc is public, everything below compares **stated capabilities and
positioning** against inber's implemented internals — not code against code.

## What Herdr Does Well (as stated)

### 1. Policy- and cost-aware routing as a first-class layer ⭐️

Herdr's headline is routing on four axes at once — **context, cost, policy, urgency**. Task
goes to the right model/tool/specialist, not to a single default agent.

**Inber connection**: Inber selects a *model* per turn (failover, model health via model-store)
but routes *tasks* to agents largely by hand — the registry and bus topology are configured, not
chosen per task by a cost/policy engine. A routing layer that picks the agent (not just the
model) by cost + policy + urgency is a real gap. Closest existing inber surface: the
kanban dispatcher/scoper loop and the autoworker, which already pick up work — but they select by
board/column and availability, not by a cost/policy scoring function.

### 2. Shared workflow memory across agents

Herdr keeps runs "grounded in shared state, prior decisions, system rules, and human feedback" —
memory that spans the *fleet*, not one agent's session.

**Inber connection**: Inber's memory is per-agent/per-session (SQLite semantic memory, memory-store
on bridge-server :8160). Cross-agent shared state today rides the NATS bus and whatever a curator
writes back to noteboard/kanban. A named, queryable *workflow memory* that every routed agent reads
and writes — decisions, system rules, human feedback — maps onto inber's blackboard/kanban thread
but isn't a single first-class store yet.

### 3. Human gates that don't stall the flow

Escalation of sensitive actions is a named feature, framed as *non-blocking to the rest of the
fleet* — one task waits for a human while the others proceed.

**Inber connection**: Inber already has this shape and arguably better plumbing — the PreToolUse
prehook (permission gating in bridge-server), the noteboard `hold` field + `auto_hold_at_usd` spend
ceiling, and the herald `ask` channel that relays a question to the user as a dash session. The
lesson from Herdr is *framing*: present these as one coherent "human gate" concept across the fleet
with per-task escalation that never blocks sibling tasks, rather than three separate mechanisms.

### 4. Fleet observability: decisions, handoffs, throughput

Herdr sells visibility across the fleet — "trace decisions, inspect handoffs, and measure
throughput."

**Inber connection**: Inber has the raw material (logstack, the bus, session state, the
kanban-classifier curator) and a visualization surface in inber-party. What's missing is the
*handoff* and *routing-decision* view — why a task went to agent X, where it was handed off, and
fleet-level throughput. inber-party shows agents-as-adventurers and token-as-XP but not the routing
trace. Surfacing routing/attribution (which the AgentFlow note in `2026-05-harness-research.md`
also calls out) is the shared takeaway.

## What Inber Should Adopt

### 1. A task router that scores on cost + policy + urgency (MEDIUM PRIORITY)

Inber picks the *model* per turn but not the *agent* per task. Add a routing step — cost (from
model-store pricing), policy (permission-store rules), urgency (task priority / SLA), and fit (agent
skills) — that selects which agent picks up a unit of work. This generalizes the kanban dispatcher
from "board/column + availability" to a scored decision, and gives the autoworker a principled
pick beyond "first unblocked, not held." Emit the score so it shows up in observability.

### 2. A first-class fleet/workflow memory (MEDIUM PRIORITY)

Promote cross-agent shared state to a named store every routed agent reads/writes: prior decisions,
active system rules, and human feedback, keyed by workflow rather than session. inber already has
the pieces (noteboard, kanban, memory-store, the bus); the gap is a single addressable "workflow
memory" surface instead of reconstructing fleet state from three sources per turn.

### 3. A routing-decision / handoff trace in inber-party (LOW–MEDIUM PRIORITY)

Add a view that records, per task: which agent was chosen and *why* (the routing score), every
handoff, and fleet throughput. This lands on the same infrastructure the AgentFlow
failure-attribution note recommends — the bus already carries enough run-level signal to attribute a
sub-task to an agent role or tool. Small win, high legibility.

## What's Different

| Dimension | Herdr (herdr.org) | Inber |
|-----------|-------------------|-------|
| **Meta-harness kind** | Orchestration (layer *above* harnesses) | Orchestration **and** runtime harness (spans both) |
| **Task→agent routing** | First-class: context/cost/policy/urgency | Mostly hand-configured; model chosen per turn, agent picked by board/availability |
| **Shared memory** | Fleet-level "workflow memory" (stated) | Per-agent/session SQLite + memory-store; fleet state via bus/kanban |
| **Human gates** | One named non-blocking escalation feature | Prehook + noteboard `hold`/`auto_hold_at_usd` + herald `ask` (more plumbing, less unified framing) |
| **Observability** | Fleet: decisions, handoffs, throughput | logstack + bus + inber-party (RPG framing; no routing/handoff trace yet) |
| **Openness** | Hosted, closed; no public source/architecture | Internal, but full source + implemented internals |
| **Deployment** | SaaS control plane | Ecosystem of Go services (registry, bus, model-store, …) |
| **Evidence level** | Marketing capability claims + a live counter | Running system with inspectable code |

## What's Different — the *other* herdr (herdr.dev, Rust multiplexer)

Included so the two never get merged in future notes:

| Dimension | herdr.dev (Rust TUI) | Inber |
|-----------|----------------------|-------|
| **What it is** | tmux-for-agents: workspaces/tabs/panes + live agent-state sidebar (blocked/working/done/idle) | Multi-agent orchestration server + agents |
| **Scope** | Local terminal workspace / observability shell over CLI agents | Distributed runtime that *is* the agents |
| **Integrations** | 14+ coding agents (Claude Code, Codex, Pi, Amp, Droid, Hermes, OpenCode, Grok, QoderCLI, OMP, generic) | Anthropic-focused via model-store; harnesses via llm-bridge-* |
| **Stack** | ratatui 0.30 / crossterm 0.29 / tokio 1; client-server, detach/reattach | Go services + NATS + SQLite |
| **License** | AGPL-3.0-or-later (+ commercial) | Internal |

herdr.dev overlaps inber-party (a fleet dashboard) and `commander.md` (multi-agent desktop
orchestrator with worktree isolation) far more than it overlaps inber's engine. If it's ever worth a
full study, group it with those, not with the orchestration meta-harnesses.

## Key Takeaway

Herdr (herdr.org) is a same-category system to inber's orchestration half, so it's a mirror rather
than a source of novel mechanism — and since no architecture is public, treat its value as
**framing, not blueprint**. The one capability it foregrounds that inber under-develops is
**scored task→agent routing** (cost + policy + urgency + fit), together with a **fleet-level
workflow memory** and a **routing/handoff trace**. Inber already owns stronger primitives for the
human-gate and permission story; the gap is presenting routing as a first-class, observable
decision instead of hand-wired topology. Do **not** merge it with the Rust `herdr.dev` multiplexer —
that's a separate tmux-for-agents workspace tool that belongs next to commander.md and inber-party.
