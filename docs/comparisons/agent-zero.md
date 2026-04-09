# Agent Zero Comparison

**Project**: [Agent Zero](https://github.com/agent0ai/agent-zero)
**Language**: Python
**Focus**: General-purpose AI agent framework — blank-slate personal assistant that dynamically acquires capabilities
**Key Strengths**: Self-extending via code execution, prompt-driven architecture, organic memory growth, Docker-contained full OS environment

## Architecture Overview

Agent Zero is a Python monolith running inside a Docker container (Kali Linux). The agent has very few built-in tools — instead, it writes its own code (Python, Node.js, bash) to accomplish tasks. All behavior is defined in editable Markdown prompt files, not hardcoded logic.

```
Web UI (port 80)
    → AgentContext (session container)
    → Agent 0 (top-level)
        → writes/executes code in terminal
        → calls subordinate Agent 1, 2, ...
        → saves to vector memory (local embeddings)
```

Multi-agent is hierarchical: Agent 0 delegates to subordinates via `call_subordinate`. Each agent can have a different profile (prompt overrides). Results flow back up the tree.

Uses LiteLLM for model-agnostic provider support (Anthropic, OpenAI, Google, local). Embeddings are local SentenceTransformer models — no API calls for memory search.

## What Agent Zero Does Well

### 1. Self-Extending via Code Execution ⭐️

The most distinctive feature. Instead of a large built-in tool library, Agent Zero has a minimal set: search, memory, communication, and code execution. Everything else — file operations, API integrations, data processing — the agent creates on the fly by writing and executing code.

This means the agent's capabilities are unbounded. Need to query a database? Write a script. Need to process images? Install a library and use it. The Docker container provides a full OS environment for this.

**Inber connection**: Inber takes the opposite approach — pre-built tools (agentkit) with defined schemas. The tradeoff: inber's tools are predictable and cacheable, but limited to what's been anticipated. Agent Zero's approach is flexible but less predictable.

### 2. Prompt-Driven Architecture

All behavior is defined in Markdown files under `prompts/`. Change the prompts and you fundamentally change the agent. The system prompt is assembled from modular `.md` files: role, communication style, problem-solving approach, tips, behaviour, environment.

Agent profiles (`agents/<profile>/prompts/`) allow per-agent prompt overrides with an `{{include original}}` inheritance syntax. This means you can create specialized agents by only overriding the parts that differ.

**Inber connection**: Inber's agents have system prompts from agent-store (soul, principles, values, userContext), but the prompt structure is less modular — it's built programmatically in `turn_prompt.go`.

### 3. Dynamic Behavior Adjustment

The `behaviour_adjustment` tool lets the agent modify its own system prompt rules at runtime. Changes are persisted as `behaviour.md` in the memory directory and injected into subsequent prompts. The agent literally rewrites its own instructions based on experience.

### 4. Multi-Layer Memory

Memory has distinct categories:
- **User-provided** — explicit facts (names, preferences, API keys)
- **Fragments** — auto-extracted from conversations, updated continuously
- **Solutions** — successful approaches saved for future reference
- **Behaviour** — self-modified runtime rules

All backed by local vector embeddings (SentenceTransformer). No API calls for search.

**Inber connection**: Inber's memory has tags, importance, and access tracking, but doesn't categorize memories by type (fragment vs solution vs behaviour). The solution-recording pattern could help agents avoid repeating mistakes.

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

### 3. Categorized Memory Types (MEDIUM PRIORITY)

Separating memories into fragments, solutions, and user-provided info enables different retrieval strategies:
- Solutions get boosted when the current task is similar
- User-provided info never decays
- Fragments can be aggressively compacted

Inber's memory has `source` and `tags` but no first-class type distinction.

## What's Different

| Aspect | Agent Zero | Inber |
|--------|-----------|-------|
| **Philosophy** | Blank slate, self-extending | Purpose-built agents with defined identities |
| **Tools** | Minimal built-in; agent writes its own code | Pre-built tool library (agentkit) |
| **Multi-agent** | Hierarchical tree (parent → child delegation) | Named agents, bus-based orchestration |
| **Memory** | Vector DB, local embeddings, categorized | SQLite, importance/access scoring |
| **Model support** | Any via LiteLLM | Anthropic-focused via model-store |
| **Isolation** | Full Docker container (Kali Linux) | In-process, forge worktrees |
| **Prompts** | Modular Markdown files, user-editable | Built programmatically, agent-store |
| **UI** | Built-in web UI | Separate dashboard (inber-party) |
| **Target** | Personal assistant, single user | Multi-agent server, distributed |

## Key Takeaway

Agent Zero's "organically growing" philosophy — minimal built-in tools, self-extension via code, dynamic behavior adjustment — is the polar opposite of inber's approach. Inber is engineered for predictability and efficiency (defined tools, token budgets, cache control). The actionable insights for inber are: **categorize memories by type** (especially the "solution" pattern), and consider **agent-authored behavior rules** as a lightweight way to make agents learn from experience without changing code.
