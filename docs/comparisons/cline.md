# Cline Comparison

**Project**: [Cline](https://github.com/cline/cline)
**Language**: TypeScript (VS Code extension)
**Focus**: Autonomous coding agent in-editor with strict human-in-the-loop approval for every action
**Key Strengths**: AST-based context management, checkpoint/restore, browser automation, MCP extensibility, ~60k stars

## Architecture Overview

Cline is a VS Code extension where the agent interacts with the codebase through the editor's APIs — file system, terminal, diagnostics, diff views. Every action (file write, terminal command, browser action) surfaces in the VS Code GUI for explicit user approval.

```
VS Code Sidebar (React webview)
    → Task execution loop
    → Tool calls (file ops, terminal, browser, MCP)
    → Human approval gate (VS Code diff view / dialog)
    → LLM (Anthropic, OpenAI, Google, Bedrock, Vertex, OpenRouter, etc.)
```

The checkpoint system takes workspace snapshots at each step, enabling diff-and-restore to any prior state. MCP support enables dynamic tool extension — the agent can even create new MCP servers on the fly.

## What Cline Does Well

### 1. AST-Based Context Management ⭐️

Cline uses tree-sitter to parse source code structure and builds a semantic codebase index with embeddings. Instead of naively stuffing files into context, it extracts symbol definitions and uses targeted searches. This enables working on large projects without exhausting the context window.

**Inber connection**: Inber's context system builds prompts from memory entries with token budgets (`turn_context.go`), but doesn't analyze code structure. AST-based context would significantly improve coding task efficiency.

### 2. Checkpoint System with Diff/Restore

Git worktree-based snapshots at each step. Users can compare any checkpoint against the current state and restore to any prior point. Finer-grained than git commits — captures intermediate states within a single task.

**Inber connection**: Inber saves messages to workspace JSON and logs to JSONL, but doesn't capture workspace state snapshots. For agents with file write access, checkpointing would provide safety and undo capability.

### 3. Browser Automation

Launches a headless browser (Puppeteer/Playwright), interacts with web pages (click, type, scroll), captures screenshots and console logs. Enables visual debugging and end-to-end testing workflows.

### 4. MCP as Extensibility Primitive

Rather than a custom plugin system, Cline uses MCP for tool extension. It can connect to MCP servers and even create new ones on-the-fly ("add a tool that fetches Jira tickets"). This bets on ecosystem effects and interoperability.

### 5. Mention System for Context Injection

`@file`, `@folder`, `@url` (fetches and converts to markdown), `@problems` (workspace diagnostics) give users fine-grained control over what the agent sees. Simple but effective context management UX.

## What Inber Should Adopt

### 1. Codebase Indexing for Context (MEDIUM PRIORITY)

Tree-sitter parsing + embedding-based semantic search for code context. When an agent is working on code, retrieve relevant symbols and definitions rather than relying on the agent to manually read files.

This would enhance `turn_prompt.go`'s context building for coding tasks without changing the overall architecture.

### 2. Workspace Checkpointing (MEDIUM PRIORITY)

For agents with file write access, take git snapshots at turn boundaries:
- Before RunTurn: snapshot workspace state
- After RunTurn: if files changed, create a checkpoint commit
- Expose checkpoint list via session history endpoints

This provides undo capability without requiring the agent to manage git itself.

### 3. Linter/Compiler Monitoring (LOW PRIORITY)

Cline watches for diagnostic errors after file changes and proactively fixes issues. Inber's workflow hooks already run build/test after tool calls (`workflow_build.go`), but feeding compiler output back as context for the next turn would close the loop.

## What's Different

| Aspect | Cline | Inber |
|--------|-------|-------|
| **Runtime** | VS Code extension | Standalone Go server |
| **Approval model** | Every action needs GUI approval | Tool allowlists per agent |
| **Context** | AST + embeddings + mentions | Memory store + token budget |
| **Multi-agent** | Single agent per task | 10 named agents, bus-based |
| **Persistence** | VS Code workspace state | SQLite agent-store |
| **Extensibility** | MCP-first | agentkit + MCP |
| **Deployment** | Desktop only (VS Code) | Server, headless, CLI |
| **Target** | Individual developer | Multi-agent infrastructure |

## Key Takeaway

Cline's most transferable innovation is **AST-based context management** — using code structure analysis to build efficient, relevant context within token budgets. This would directly improve inber's coding task performance. The checkpoint system is also valuable for any agent with file write access. Cline's strict human-in-the-loop model is the opposite of inber's autonomous approach, but the underlying tools (code indexing, checkpointing, compiler monitoring) are universally useful.

## Harness-watch — 2026-06-07: skills travel *bundled inside plugins*, discovered as extra search roots (no plugin-specific skill format)

[PR 11161](https://github.com/cline/cline/pull/11161) lets a plugin ship skills by
placing a normal `skills/<name>/SKILL.md` tree inside the package; the plugin
system contributes additional **skill search roots** rather than a plugin-specific
skill API. The same SKILL.md loader (frontmatter + body, identical to
workspace/global/managed skills) parses them — the only new logic is walking up
from each plugin entry file to the package root and adding `<root>/skills` when
present, gated by the same `plugins` config (disabled plugins are filtered before
their skill dirs are considered). [#11219](https://github.com/cline/cline/pull/11219)
groups discovered skills in settings **by owning plugin** using filesystem
ownership (so opening settings never executes plugin code), and
[#11220](https://github.com/cline/cline/pull/11220) restores enabled skills to the
`/` slash autocomplete alongside the searchable picker.

**What inber should consider:** inber's skill-store (`:8301`) is a centralized flat
registry (one row per SKILL.md, ingested by cloning whole repos). Cline's inverse
pattern offers two cheap wins: (1) **group-by-source** — skill-store already stores
`source` per skill, so a "these N skills came from upstream X" view in the
dash/bridge-ui Skills surface gives the same provenance UX without flattening it
away. (2) **co-distribution** — Cline ships skills next to plugin tools so one
install brings both; inber splits skill-store and tool-store (`:8302`) with no link
between a tool and its sibling skill from the same upstream. Worth deciding whether
to join them by shared source-group. The key reusable stance: keep bundled skills
**file-backed** (no imperative registration API) so they behave identically to
standalone skills — which matches inber's "everything is a SKILL.md row" model and
argues against ever inventing a plugin-specific skill format.

## Harness-watch — 2026-06-13: two-layer tool-output bounding + parallel-tool-calls as a cost lever

Two benchmark-driven context changes landed this window.

**1. Executor-level output caps with model-facing pagination** ([PR 11480](https://github.com/cline/cline/pull/11480), with [#11465](https://github.com/cline/cline/pull/11465)/[#11463](https://github.com/cline/cline/pull/11463)). Cline now caps `run_commands` combined stdout/stderr at 48,000 chars and `read_files` whole-file/oversized reads at 2,000 lines / 48,000 chars (plus a 2,000-char per-line cap to defang minified files). Two design details worth stealing: (a) truncation switched from keep-first-N to **head+tail sampling** (start and end preserved, middle elided) because build/test failures live at the *end* of output; (b) the cap is applied at the **executor** layer (bounds what enters session history at all — JSON files, memory, UI, summarizer inputs), sized deliberately *below* the MessageBuilder per-string backstop (50k) so capped content + truncation notice survives request-build instead of being re-truncated into a generic marker. The notice reports the *original* size and tells the model how to recover (grep/head/tail, or paginate with `start_line`/`end_line`) — a model-facing recovery path, not silent loss. This converges with the terminal-coding-agent harness research ([arXiv:2605.18747](https://arxiv.org/abs/2605.18747), already in `docs/papers/2026-06-harness-research.md`) and Pi's "[Showing lines 1-2000 of 50000. Use offset=2001]" nudge.

**What inber should consider:** inber's `smart-truncation`/`context-loading` bound context at request-build time. Add a *second*, earlier bound at the tool-execution edge so megabyte tool blobs never enter session history (memory-store, log-store, summarizer inputs) in the first place — and switch large-output truncation to head+tail sampling with an original-size + pagination notice, since the diagnostically-important bytes (stack traces, test failures) sit at the tail that keep-first-N discards.

**2. Parallel tool calls are a prompt problem, not a runtime problem** ([PR 11514](https://github.com/cline/cline/pull/11514)). Benchmark traces showed Cline emitting ~1 tool call per assistant turn vs OpenCode batching 2–4; because each turn resends the accumulated conversation, one-tool turns multiply the resend cost of every prior result. The fix was purely additive prompting — system-prompt + per-tool-description guidance to batch independent reads/searches/fetches/safe commands into one response — leaving runtime scheduling untouched. Note the open hypothesis: a **singular** tool surface (`read_file` vs `read_files`) may matter, because many models are trained to express parallelism as multiple separate tool-call blocks rather than an array arg.

**What inber should consider:** tool-calls-per-turn is a measurable cost lever independent of whether tools run concurrently. Audit inber's per-turn tool-call count on multi-read tasks; if it skews to one-per-turn, add explicit batch-independent-calls guidance to the agent system prompt and tool descriptions before touching the executor's concurrency config. Weigh exposing singular tool variants if traces show models reluctant to use array-shaped batch tools.
