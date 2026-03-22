# Remind

**Repo:** https://github.com/sandst1/remind
**Type:** Memory layer for AI agents (Python, MCP)
**Focus:** Generalization-capable memory — extracts concepts from experiences rather than storing verbatim text

## What it does

- Memory layer that mimics human memory consolidation: specific episodes → abstract knowledge
- CLI (`remind remember/recall/consolidate`) + MCP server for IDE agents
- Project-local SQLite DB (`.remind/remind.db`)
- Skills system: Markdown files that teach agents how to use Remind (Claude Code, Cursor, etc.)
- Built-in skills: core memory, planning, spec-driven dev, task execution
- Web UI + REST API via MCP server
- Embedding-based retrieval (OpenAI embeddings) + LLM-powered consolidation (Anthropic)

## How it compares to Inber

| Aspect | Remind | Inber |
|--------|--------|-------|
| Scope | Memory layer only | Full agent framework (orchestration + memory + tools) |
| Memory model | Episode → consolidation → generalized concepts | Tag-based chunks competing for context budget |
| Storage | SQLite + embeddings | SQLite + embeddings (memory package) |
| Consolidation | Explicit `consolidate` step merges episodes into concepts | Auto-save from conversations, manual `memory_save`, decay-based pruning |
| Retrieval | Embedding similarity search | Embedding search + tag filtering + importance scoring |
| Integration | MCP server, CLI, Markdown skills | Native — memory is built into the engine |
| Multi-agent | Not designed for it | Per-agent memory DBs, shared via NATS |
| Language | Python | Go |

## Key differences

- **Remind is a library/tool, Inber is a framework.** Remind adds memory to existing agents (Claude Code, Cursor). Inber IS the agent.
- **Consolidation vs. budgeting.** Remind's big idea is generalizing specific events into abstract knowledge (like human memory). Inber's approach is budget-based: everything competes for context space, large memories get auto-truncated, stale ones decay.
- **Remind is more research-oriented** — the generalization step is interesting but adds latency and LLM cost. Inber prioritizes token efficiency.
- **Skills system** is similar in concept to Inber's agent config — both use Markdown to define behavior.

## Worth watching

The generalization/consolidation approach is genuinely interesting. Most memory systems (including Inber's) do retrieve-and-stuff. Remind's "distill episodes into concepts" is closer to how humans actually remember things. Could inform future memory improvements in Inber.
