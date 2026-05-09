# OpenCode Comparison

**GitHub:** github.com/anomalyco/opencode
**Language:** TypeScript (Bun runtime)
**Focus:** Open-source, provider-agnostic AI coding agent with TUI-first design

## Architecture Overview

OpenCode is a large TypeScript monorepo (~19 packages) using Turbo for build orchestration and SST for deployment. It follows a client/server architecture with clean separation between the core engine and UI frontends.

```
┌─ UI Layer ──────────────────────────────────────────┐
│  TUI (primary) | Desktop (Electron) | Web | Slack   │
└─────────────────┼───────────────────────────────────┘
                  │
┌─ Core Engine ───┼───────────────────────────────────┐
│  Agent System | Session | Provider | Tool | MCP     │
│  Control Plane | Permission | Config | Bus          │
└─────────────────┼───────────────────────────────────┘
                  │
┌─ Dev Integration┼───────────────────────────────────┐
│  LSP | Git | Worktree | Shell/PTY | IDE | Snapshot  │
└─────────────────────────────────────────────────────┘
```

### Monorepo Packages

| Package | Purpose |
|---------|---------|
| `opencode` | Core engine — agents, tools, providers, sessions |
| `app` | Application shell |
| `sdk` | Public SDK for programmatic access |
| `plugin` | Plugin system |
| `desktop-electron` | Electron desktop client |
| `web` | Web interface |
| `ui` | Shared UI components |
| `slack` | Slack integration |
| `identity` | Auth/identity management |
| `enterprise` | Enterprise features |
| `console` | Console/admin interface |

## Agent System

**Dual Built-in Agents:**
- **build** — Full-access development agent with unrestricted file and command permissions
- **plan** — Read-only agent for exploration, requests permission before modifications

**Subagent Model:**
- A **general** subagent handles complex searches and multi-step reasoning
- Invoked via `@general` mentions within conversation
- Tab key switches between primary agents

This is simpler than inber's 10-agent system but follows a similar principle of role-based agent specialization.

## Provider System

**Provider Agnostic:**
- Supports Claude, OpenAI, Google, and local models
- Not locked to any single provider
- Commercial "OpenCode Zen" option provides hosted model access

**Comparison to inber:** Inber is currently Anthropic-coupled. OpenCode's multi-provider approach validates the direction of extracting a provider interface (similar to what Goose does).

## Tool & Extension Architecture

**MCP Integration:**
- Full Model Context Protocol support for tool extensibility
- `mcp/` module for MCP client implementations

**Plugin System:**
- Dedicated `plugin/` package for extending functionality
- `.opencode` project directories for agent specs and behavioral policies

**Skills:**
- `skill/` module — likely pre-built capability bundles (similar to Claude Code's slash commands)

**Comparison to inber:** OpenCode's plugin + MCP approach is more modular than inber's current hardcoded tool registration. The skill system is worth examining for inber's tool abstraction.

## Development Integration

OpenCode has deeper IDE/editor integration than most competitors:

- **LSP support** — Out-of-the-box Language Server Protocol for code intelligence
- **Worktree management** — Git worktree isolation for parallel work
- **Snapshot system** — State snapshots and recovery
- **PTY support** — Full pseudo-terminal for shell interaction
- **IDE module** — Direct editor integration
- **Zed extension** — First-party editor plugin

**Comparison to inber:** Inber has worktree support via forge but lacks LSP integration and snapshot recovery. The LSP integration is notable — it gives the agent richer code understanding without relying solely on the LLM.

## Session & State

- Session persistence and management
- Storage abstraction layer
- Sync mechanisms for state coordination
- Share functionality for collaboration

**Comparison to inber:** More developed session model with sharing capabilities. Inber's memory system is likely more sophisticated for long-term context, but OpenCode's session sharing is a feature inber lacks.

## Permission Model

- Dedicated `permission/` module
- Agent-level permission scoping (build = full access, plan = read-only)
- Command approval workflows

Similar in spirit to Claude Code's permission system.

## Infrastructure & Scale

- **Effect system** — Uses functional effect patterns (likely Effect-TS) for structured concurrency and error handling
- **Event bus** — Internal message bus for component communication
- **Control plane** — Central coordination layer

The Effect-TS usage is notable — it's a sophisticated choice for managing async complexity in TypeScript, similar to how Go uses goroutines/channels.

## What Inber Should Note

### 1. **LSP Integration**
OpenCode's LSP support gives agents real-time code intelligence (go-to-definition, references, diagnostics) without consuming LLM tokens. This is a meaningful architectural advantage.

**Inber action:** Evaluate LSP client integration for Go-aware code navigation within agent sessions.

### 2. **Snapshot & Recovery**
The snapshot system allows rolling back agent changes — a safety net that inber currently lacks beyond git-level undo.

**Inber action:** Consider checkpoint/snapshot support in forge worktree slots.

### 3. **Multi-Frontend Architecture**
OpenCode ships TUI, desktop, web, and Slack frontends from the same core. This validates inber's direction with si (communications layer) and the bus architecture, but OpenCode's execution is more polished.

### 4. **Provider Independence**
Multi-provider support from the start. Inber should continue pursuing provider abstraction via model-store.

### 5. **Plugin vs MCP Dual Extension**
Having both a plugin system and MCP support gives two levels of extensibility — lightweight plugins for common cases, MCP for standard tool integration.

## What Inber Does Better

### 1. **Memory Architecture**
Inber's memory system (conversation summarization, pruning, stashing) is more sophisticated than OpenCode's session-based approach. Long-term context retention across sessions is a genuine differentiator.

### 2. **Agent Depth**
Inber's 10-agent system with distinct personalities and specializations is richer than OpenCode's build/plan/general trio.

### 3. **Go Ecosystem**
Single binary, no runtime dependencies, lower resource footprint. OpenCode requires Bun and a Node.js ecosystem.

### 4. **Message Bus Architecture**
Inber's NATS-backed bus with si adapter layer is more robust for distributed agent communication than OpenCode's internal bus.

### 5. **Context Management**
Inber's smart truncation and context loading is more automatic and fine-tuned.

## Recommended Adoptions for Inber

**High Priority:**
1. **LSP integration** — Real code intelligence without LLM token cost
2. **Snapshot/checkpoint system** — Safety net for agent modifications

**Medium Priority:**
3. **Session sharing** — Collaborative features
4. **Skill/recipe abstraction** — Pre-built capability bundles

**Low Priority:**
5. **Desktop client** — Electron or similar GUI (inber's TUI is sufficient for now)
6. **Effect-style structured concurrency** — Go already handles this well with goroutines

---

## Harness-watch — 2026-05-09: `scout` agent + `@reference` external sources

[PR 24149](https://github.com/sst/opencode/pull/24149) (merged 2026-05-08) adds a built-in `scout` subagent dedicated to **research over external repos**, distinct from the existing `general` subagent that operates on the working tree. Two design pieces worth noting:

- **Named external references.** Config lets users register git repos or local dirs as first-class `@<name>` references. The scout agent receives those names as targets; opencode handles cloning into a managed cache, exposing them via two new managed tools `repo_clone` and `repo_overview`. The agent never sees a clone URL or a path — only the alias.
- **Research-only tool surface.** Scout's tool list is read/search/overview only; it cannot write to the working tree or shell out beyond the managed clone cache. The contract is "go look at this thing and report back" — the parent gets the report, not the side effects of the look-around.

**What inber should consider:** Two of inber's existing subagents (notably the documentation-researcher and the upstream-comparison agents implied by harness-watch itself) are already in scout's shape conceptually — research-only with bounded read surface — but they currently use the same Read/Grep/Bash trio as executor agents and manage their own cwd. The opencode pattern argues for a **named-reference layer** between the user and the subagent: instead of agents chasing paths, the harness owns a registry of "places to look" with managed clone/cache and a stable alias the model can reason about. For inber, the natural home is `forge` (which already manages worktree slots) extended with a read-only "external repo" slot type — and a `research` tool category that's allowlisted to those slots only. Worth a sketch in `docs/multi-agent-design.md`, paired with the SWE-Edit viewer/editor decomposition (2604.26102) which is the same "isolated read-only context" pattern from a different angle.

[PR 24712](https://github.com/sst/opencode/pull/24712) (merged 2026-05-08) is a separate but related move: opencode replaces its dependency on Vercel's AI SDK with an in-house Effect-based `packages/llm` covering 10+ providers, with a recorded-cassette test harness (`http-recorder`) that replays HTTP traffic against typed schemas. The cassette pattern is the borrowable bit — inber's provider bridges (`llm-bridge-anthropic`, `llm-bridge-openai`, `llm-bridge-google`) are integration-tested against live APIs today; a recorded-cassette layer would let the bridge tests run hermetically in CI without losing the wire-format coverage.
