# Remind

**Last reviewed**: 2026-10-29 against sandst1/remind@7b05347 (2026-10-23).

**Repo:** https://github.com/sandst1/remind (Apache-2.0, active)
**Type:** Memory layer for AI agents (Python, CLI + MCP, published as `remind-mcp` on PyPI)
**Focus:** Memory that turns specific experiences (episodes) into general knowledge (concepts) and tracks facts over time

## What it does

As first reviewed, Remind called an LLM itself to consolidate episodes into concepts. It has since reversed that: it now calls itself a "deterministic memory substrate", makes no LLM calls of its own, and leaves all judgment to the calling agent. What it does now:

- Stores **episodes** (raw experiences), **concepts** (general knowledge) and **facts** with validity tracking. A new fact is put in a cluster by entity overlap and checked against existing facts; clashes come back as conflicts, with ready-to-run commands to resolve them.
- Every `remember` returns the five nearest episodes and concepts, so the agent can spot contradictions at once.
- The agent builds concepts itself with `remind apply` (for example `concept from=ep:11,ep:12 ...`), after reading state with `remind snapshot`. Edits must cite the revision they were based on.
- An append-only change feed (`remind events --since <seq>`).
- **Storage**: by default, plain JSON records in `.remind/memory/`, meant to be committed with the code so memory follows branches and merges. Indexes and embeddings sit in an ignored cache. SQLite, PostgreSQL and MySQL remain options.
- **Embeddings**: local by default (fastembed, no API key). The earlier version used OpenAI embeddings and Anthropic for consolidation.
- CLI, MCP server, web UI and REST API (`/api/v1/`), and `remind skill-install` to write skill files for Claude Code, Cursor, Codex or OpenCode.
- An evaluation suite (`remind-eval`) covering capture, curation, restart and recall.

## How it compares to Inber

| Aspect | Remind | Inber |
|--------|--------|-------|
| Scope | Memory layer only | Full agent framework (orchestration + memory + tools) |
| Memory model | Episodes, concepts and temporal facts, curated by the calling agent | Tag-based chunks competing for context budget |
| Storage | JSON files in the repo by default; SQL optional; local embeddings | SQLite + embeddings, through the memory-store library (`memory/`) |
| Consolidation | The agent writes concepts and resolves fact conflicts through `apply`; Remind itself calls no LLM | Background extraction from finished turns, manual `memory_save`, importance decay (`POST /api/memory/decay`, `server/api_memory.go`) |
| Retrieval | Embedding similarity, plus entity and label lookups | Embedding search + tag filtering + importance and recency scoring (`memory/tools.go`, `memory/auto_context.go`) |
| Integration | MCP server, CLI, REST, Markdown skills | Native — memory is built into the engine |
| Multi-agent | Not designed for it | One memory DB per workspace root (`memory.OpenOrCreate(repoRoot)` in `engine/engine_new.go`), shared by every agent working there |
| Language | Python | Go |

The earlier version of this table said inber's memory DBs were per agent and shared over NATS. Neither is true of the code on 2026-10-29: the store is opened per workspace root, and nothing in `memory/` touches the message bus.

## Key differences

- **Remind is a library/tool, Inber is a framework.** Remind adds memory to existing agents (Claude Code, Cursor, Codex). Inber is the agent.
- **Agent-curated vs. budgeted.** Remind's idea is that the agent turns specific events into general knowledge and keeps facts consistent over time. Inber's approach is budget-based: everything competes for context space, large content is stashed behind pointers, and importance decays.
- **Cost.** The earlier note that Remind's consolidation adds latency and LLM cost no longer applies: Remind makes no model calls. The cost moved into the calling agent's turns. Inber's background extraction, when an agent turns it on, calls a model (`conversation/extract.go`).
- **Memory in git.** Remind's default of committing memory as JSON beside the code has no inber equivalent; inber's memory DB is a local SQLite file.
- **Skills system** is similar in idea to inber's agent config — both use Markdown to define behavior.

## Worth watching

Two ideas inber lacks: facts with validity periods and explicit conflict records, so a new fact supersedes an old one instead of both being recalled; and returning the nearest existing memories on every write, so contradictions show up when they are made. Both would fit inside memory-store.
