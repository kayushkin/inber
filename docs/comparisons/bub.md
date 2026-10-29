# Bub Comparison

**Last reviewed**: 2026-10-29 against bubbuild/bub@ad05783 (2026-10-28). Active; rewritten since the first version of this doc (see "Why We Rewrote Bub" on bub.build).

**Project**: [Bub](https://github.com/bubbuild/bub)
**Language**: Python
**Focus**: Hook-first runtime for agents that live alongside humans in shared environments (group chats, multi-actor channels)
**Key Strengths**: Pluggy-based hook architecture, "tape context" (append-only records over mutable session state), one runtime across CLI/Telegram/custom channels, operator equivalence between humans and agents

## Architecture Overview

Bub is a small Python runtime built on [pluggy](https://pluggy.readthedocs.io/) (the same plugin system used by pytest). The entire turn pipeline is a sequence of hooks that any plugin can override or replace. It's built on the [agents.md](https://agents.md/) and [Agent Skills](https://agentskills.io/) standards and originated from group-chat scenarios where multiple humans and agents needed shared, inspectable context.

```
inbound message
    → resolve_session
    → load_state
    → build_prompt
    → run_model
    → save_state
    → render_outbound
    → dispatch_outbound
```

Each stage is a `pluggy` hookspec. Builtins register first; external plugins load via Python entry points (`[project.entry-points."bub"]`) and take precedence at runtime. There are no framework-only shortcuts — the builtins use the same hook surface as third-party code.

Channels (CLI, Telegram, etc.) are adapters that feed the same inbound pipeline. The runtime model is identical regardless of surface.

Since the rewrite the code splits in two (`AGENTS.md` in the repo):
- **`bub.core`, a small agent kernel** with no settings or plugin dependencies: the agent loop over a tape store (`src/bub/core/agent.py`), a model adapter over the Republic library (`core/model.py`), an event bus, and a "code mode" that runs model-written code in a subprocess. `pip install bub` installs only this, so you can embed an agent in your own program.
- **The app** (`bub[app]`): `framework.py`, the hooks, channels, skills and builtins.

The hook list has grown (`src/bub/hooks/specs.py`). Beside the turn stages it now has `before_llm_call` / `after_llm_call`, `before_tool_call` / `after_tool_call` (which can veto or rewrite a call), `admit_message` (whether a message joins a running turn), a steering inbox for messages that arrive mid-turn, tape sidecars, and `provide_environment`. The builtins add a subagent tool, deferred tools the model loads by name, and `tape.handoff`, which writes an anchor so later context starts from it. When a provider reports that the context is too long, the agent makes one automatic handoff and retries (`core/agent.py`, `MAX_AUTO_HANDOFF_RETRIES`). `bub login codex` signs in through the Codex CLI's credentials (`builtin/auth.py`). A plugin hub lives at hub.bub.build.

## What Bub Does Well

### 1. Hook-First, Not Framework-First ⭐️

Every turn stage is a hook. There is no "core" that calls into "extensions" — the pipeline is composed entirely of hook implementations, with builtins registered alongside plugins. Override one stage (e.g. `build_prompt`) without touching anything else, or replace the whole flow.

This inverts the typical framework relationship: instead of subclassing or implementing a sprawling interface, you write a small class with `@hookimpl` decorators on the specific stages you care about.

**Inber connection**: Inber's engine is a tightly coupled turn loop where customization usually means changing engine code or adding tools. `agent.Hooks` (`agent/agent.go`) lets callers observe a turn (`OnRequest`, `OnToolCall`, `OnToolResult`) and make a few changes (`ModifyToolResult`, `BeforeRequest` for pruning), but no hook replaces a whole stage. A hook-based pipeline would let agent variants override specific stages (e.g. a custom `build_prompt` per agent) without forking the engine.

### 2. Tape Context (Append-Only Records) ⭐️

Context is rebuilt from append-only records on every turn rather than carried as mutable session state. This makes inspection, replay, and handoff trivial — there's no hidden state, just a tape of events that can be re-projected.

This is the same pattern as event sourcing: the source of truth is the event log, and any view (current prompt, full history, summary) is a projection over it.

**Inber connection**: Inber holds the current message list as a slice of `anthropic.MessageParam` in the agent; the conversation package's pruning, summarizing and stashing return a changed list that replaces it (`conversation/manage.go`). One step toward a tape already exists: summarizing saves the turns it replaces, verbatim, as a tagged memory (`conversation/archive_tags.go`), and stashing leaves a pointer to the content it lifts out. *(Updated 2026-10-29: this said "a `Conversation` struct"; there is no such type.)* An append-only event tape would make these operations into projections instead of destructive mutations — easier to audit, easier to debug a "why did the model see this" question.

### 3. One Runtime Across Surfaces

CLI, Telegram, and custom channels all feed the same turn pipeline. Adapters change the inbound/outbound transport, not the runtime model. A skill or hook written for one channel works on all of them.

**Inber connection**: Inber has the engine for in-process invocation and a separate server package for HTTP/bus integration. The runtime is largely the same but the wiring differs. Bub's "one pipeline, many adapters" model is cleaner.

### 4. Operator Equivalence

Humans and agents work inside the same runtime boundaries. There's no privileged "operator" class — when a human posts in a group chat, they go through the same pipeline as an agent (minus the model call). This makes handoff between human and agent natural.

### 5. Builtins Are Replaceable

Every builtin (CLI channel, Telegram channel, model runner, tool dispatcher, skill loader) is registered through the same hook system as external plugins. There's no special "core" code — anything can be swapped without forking.

## What Inber Should Adopt

### 1. Pipeline Hooks for the Turn Loop (HIGH PRIORITY)

Refactor the turn loop — `RunTurn` at `engine/engine.go:245` *(corrected 2026-08-07: this
said `engine/turn.go`, which does not exist; the split proposed in `ENGINE_REFACTOR_PLAN.md`
was never carried out)* *(2026-10-29: now `engine/engine.go:256`; the turn is spread over `engine/turn_prepare.go`, `turn_execute.go`, `turn_postprocess.go` and, for OpenAI models, `turn_openai.go`)* — so each stage of a turn (resolve session, load state, build prompt, call model, save state, render output) is dispatched through a hook list. Builtins register the default behavior; agents or plugins can override individual stages.

This unblocks:
- Per-agent prompt customization without touching engine code
- Test doubles that replace `run_model` with deterministic responses
- Cross-cutting concerns (logging, metrics, cost tracking) as hooks instead of scattered calls

Concrete first step: define a `TurnHook` interface with optional methods for each stage, then wire engine.go to walk a list of hooks for each stage instead of calling functions directly.

### 2. Append-Only Event Tape for Conversations (MEDIUM PRIORITY)

Replace the mutable message list with an append-only event log + projections. Each user message, assistant response, tool call, prune decision, and summary becomes an event. The current "what does the model see" view is a projection, not the canonical state.

Benefits:
- Pruning becomes a projection filter, not a destructive operation
- Replay is free (rebuild any prior state by replaying up to event N)
- Audit ("why did the model see this?") is trivial
- Stashing is just a tag on events, not a separate table

This is a bigger change but aligns well with the existing memory system, which is already event-shaped.

### 3. Surface-Agnostic Pipeline (LOW PRIORITY)

Currently engine vs. server code paths diverge for in-process vs. networked invocation. Push the pipeline lower so both paths feed the same hook chain — adapters only handle transport (HTTP request → message, NATS event → message, CLI stdin → message).

## What's Different

| Aspect | Bub | Inber |
|--------|-----|-------|
| **Language** | Python | Go |
| **Plugin system** | pluggy (entry points) | Hardcoded in engine, no plugin loader |
| **Customization model** | Hooks override stages | Edit engine code or add tools |
| **State model** | Append-only tape, rebuilt per turn | Message slice replaced by pruning/summarizing; replaced turns archived to memory |
| **Channels** | CLI, Telegram, custom (all same pipeline) | None in inber; HTTP, NATS bus and llm-bridge-compatible routes (`server/api_bridge.go`), with chat apps reached through other services |
| **Standards** | agents.md, Agent Skills | Custom agent-store schema |
| **Origin** | Group chats with multi-actor context | CLI + server for personal multi-agent infra |
| **Operator model** | Humans and agents go through same pipeline | Humans interact via UI/bus, agents run in engine |
| **Codebase shape** | Small core, replaceable builtins | Multi-package monorepo, engine-centric |
| **Distribution** | `pip install bub` (kernel only) or the app installer | `inber` binary, deploy-as-service |

## Key Takeaway

Bub's central bet is that **plugins shouldn't be second-class** — every stage of the runtime is a hook, and builtins use the same surface as third-party code. The append-only tape is the second major idea: state is a projection over events, not a mutable struct.

For inber, the actionable insight is **pipeline hooks for the turn loop**. Inber's engine has grown organically and customization currently means editing engine code. A pluggy-style hook layer would let agents override specific stages (prompt building, model selection, output rendering) without forking the engine — and would make per-agent variants much cheaper to express. The append-only tape is the longer-term play, but worth piloting on a single subsystem (e.g. memory events) before reshaping the full conversation model.
