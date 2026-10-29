# Agent Zero Comparison

**Last reviewed**: 2026-10-29 against agent0ai/agent-zero@ff2a5b9 (2026-10-13).

**Project**: [Agent Zero](https://github.com/agent0ai/agent-zero)
**Language**: Python
**Focus**: General-purpose AI agent framework — blank-slate personal assistant that dynamically acquires capabilities
**Key Strengths**: Self-extending via code execution, prompt-driven architecture, organic memory growth, Docker-contained full OS environment

## Architecture Overview

Agent Zero is a Python app running inside a Docker container (the base image is still Kali Linux, `docker/base/Dockerfile`). The agent has few built-in tools — instead, it writes its own code (Python, Node.js, bash) to get things done. Behavior is defined in editable Markdown prompt files, not hardcoded logic.

Since this doc was first written much of the core has moved into plugins under `plugins/`: memory (`_memory`), chat compaction (`_chat_compaction`), sign-in to model plans (`_oauth`), Telegram, WhatsApp and email channels, a desktop and browser in the container, "time travel" over past chat states, and a plugin hub with over 100 community plugins. Plugins and the core hook into the turn through named extension points (`extensions/python/<hook>/`, for example `before_main_llm_call`, `tool_execute_before`, `monologue_end`). It also speaks MCP and A2A.

```
Web UI (port 80)
    → AgentContext (session container)
    → Agent 0 (top-level)
        → writes/executes code in terminal
        → calls subordinate Agent 1, 2, ...
        → saves to vector memory (local embeddings)
```

Multi-agent is hierarchical: Agent 0 delegates to subordinates via `call_subordinate`. Each agent can have a different profile (prompt overrides). Results flow back up the tree.

Uses LiteLLM (`requirements.txt`) for provider support (Anthropic, OpenAI, Google, local). The `_oauth` plugin adds sign-in to account plans instead of API keys: Codex/ChatGPT by device code, served to the rest of the app as a local OpenAI-compatible endpoint, and GitHub Copilot. Embeddings can run locally through sentence-transformers (`models.py`), so memory search need not call an API.

## What Agent Zero Does Well

### 1. Self-Extending via Code Execution ⭐️

The most distinctive feature. Instead of a large built-in tool library, Agent Zero has a minimal set: search, memory, communication, and code execution. Everything else — file operations, API integrations, data processing — the agent creates on the fly by writing and executing code.

This means the agent's capabilities are unbounded. Need to query a database? Write a script. Need to process images? Install a library and use it. The Docker container provides a full OS environment for this.

**Inber connection**: Inber takes the opposite approach — pre-built tools with defined schemas, loaded from tool-store (`agent/registry/tools.go`) plus any MCP servers (`tools/mcp/`). The tradeoff: inber's tools are predictable and cacheable, but limited to what's been anticipated. Agent Zero's approach is flexible but less predictable.

### 2. Prompt-Driven Architecture

All behavior is defined in Markdown files under `prompts/`. Change the prompts and you fundamentally change the agent. The system prompt is assembled from modular `.md` files: role, communication style, problem-solving approach, tips, behaviour, environment.

Agent profiles (`agents/<profile>/prompts/`) allow per-agent prompt overrides with an `{{include original}}` inheritance syntax. This means you can create specialized agents by only overriding the parts that differ.

**Inber connection**: Inber's agents load from agent-store (`agent/registry/config.go`), but the prompt is assembled in Go code (`engine/turn_prompt.go`, `engine/prompt_blueprint.go`), not from files a user edits.

### 3. Dynamic Behavior Adjustment

The `behaviour_adjustment` tool (now part of the memory plugin, `plugins/_memory/prompts/agent.system.tool.behaviour.md`) lets the agent modify its own system prompt rules at runtime. Changes are persisted as `behaviour.md` in the memory directory and injected into subsequent prompts. The agent literally rewrites its own instructions based on experience.

### 4. Multi-Layer Memory

Memory has distinct categories:
- **User-provided** — explicit facts (names, preferences, API keys)
- **Fragments** — auto-extracted from conversations, updated continuously
- **Solutions** — successful approaches saved for future reference
- **Behaviour** — self-modified runtime rules

The areas are `main`, `fragments` and `solutions` (`plugins/_memory/helpers/memory.py`), stored in a FAISS vector index, with a consolidation pass that merges near-duplicates (`memory_consolidation.py`).

**Inber connection**: Inber's memory (the memory-store module, wrapped by `memory/memory.go`) has tags, importance, access tracking and embeddings for search, but doesn't categorize memories by type (fragment vs solution vs behaviour). The solution-recording pattern could help agents avoid repeating mistakes.

### 5. Docker as the Computer

The agent doesn't just run in Docker for isolation — the Docker container IS the computer. Full Kali Linux environment, SearXNG for private search, SSH access. The agent treats the OS as its tool surface. This provides both isolation and capability.

## What Inber Should Adopt

### 1. Solution Memory Pattern (MEDIUM PRIORITY)

Agent Zero explicitly saves "solutions" — successful approaches to problems. When a similar problem arises, the solution is retrieved and adapted. Inber's memory system stores raw facts and references but doesn't distinguish between "this worked" and "this is information."

**What to adopt:**
- Tag memories with outcome (success/failure)
- When saving tool call results, note whether the approach worked
- Prioritize successful solutions in memory search for similar tasks

### 2. Dynamic Behavior Rules (LOW PRIORITY)

The ability to modify runtime behaviour and persist it is powerful for agents that learn from their mistakes. Inber could allow agents to append to their own context rules via memory:

- Agent saves a "behaviour" memory: "When modifying Go files, always run `go build` after edits"
- This memory auto-loads into subsequent system prompts
- Importance decays normally — rules that aren't useful fade out

Inber already has half of this: the system prompt starts with "always-load" memories (identity, instructions, tools; `engine/turn_prompt.go` `BuildSystemPrompt`). What is missing is a tool the agent itself uses to add or remove such a rule.

### 3. Categorized Memory Types (MEDIUM PRIORITY)

Separating memories into fragments, solutions, and user-provided info enables different retrieval strategies:
- Solutions get boosted when the current task is similar
- User-provided info never decays
- Fragments can be aggressively compacted

Inber's memory has `source` and `tags` but no first-class type distinction, and memory-store notes that nothing reads `source` yet.

## What's Different

| Aspect | Agent Zero | Inber |
|--------|-----------|-------|
| **Philosophy** | Blank slate, self-extending | Purpose-built agents with defined identities |
| **Tools** | Minimal built-in; agent writes its own code; plugins, MCP, A2A | tool-store tools plus MCP client (`tools/mcp/`) |
| **Multi-agent** | Hierarchical tree (parent → child delegation) | Named agents on a NATS bus, plus a spawn tool capped by `MaxSpawnDepth` (`server/config.go`) |
| **Memory** | FAISS vector index, local embeddings, areas | SQLite with embeddings, importance/access scoring (memory-store) |
| **Model support** | Any via LiteLLM; Codex and Copilot sign-in | Anthropic SDK first; OpenAI-compatible path (`engine/turn_openai.go`) that can use the Codex ChatGPT plan (`agent/codex_subscription.go`) |
| **Isolation** | Full Docker container (Kali Linux) | No container; per-session git worktrees from the forge library (`server/workspace_roots.go`) and execution modes (`guard/guard.go`) |
| **Prompts** | Modular Markdown files, user-editable | Assembled in Go, agent identity from agent-store |
| **UI** | Built-in web UI | None of its own; serves llm-bridge-compatible routes (`server/api_bridge.go`) for the llm-bridge chat UI |
| **Target** | Personal assistant, single user | Multi-agent server, distributed |

## Key Takeaway

Agent Zero's "organically growing" philosophy — minimal built-in tools, self-extension via code, dynamic behavior adjustment — is the polar opposite of inber's approach. Inber is built for predictability and efficiency (defined tools, token and cost budgets, cache control). The actionable insights for inber are: **categorize memories by type** (especially the "solution" pattern), and consider **agent-authored behavior rules** as a lightweight way to make agents learn from experience without changing code.
