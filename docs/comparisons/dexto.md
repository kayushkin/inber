# Dexto Comparison

**Project**: [Dexto](https://github.com/truffle-ai/dexto)
**Language**: TypeScript (Node.js monorepo)
**Focus**: Agent harness / orchestration layer — config-driven agents via YAML with multi-interface runtime
**Key Strengths**: YAML-driven agent definitions, multi-interface (CLI/Web/REST/MCP/bots), pluggable storage (SQLite/PostgreSQL/Redis), sub-agent orchestration with signal bus, OpenTelemetry built-in

## Architecture Overview

Dexto is a monorepo (~20 packages) with a layered architecture. YAML defines the agent, `core` provides the runtime, tool packages provide capabilities, storage backends provide persistence, and interface packages provide access.

```
YAML Agent Config
    → core (DextoAgent: lifecycle, streaming, state machine)
        → llm/ (50+ models via provider adapters)
        → tools/ (MCP-first + modular tool packages)
        → memory/ (persistent across sessions)
        → session/ (create, resume, list, search)
    → orchestration/ (signal-bus, task-registry, condition-engine)
    → interfaces: CLI | Web UI | REST+SSE server | MCP server | Discord | Telegram
    → storage: PostgreSQL | SQLite | Redis | in-memory (pluggable)
```

Same agent runs as CLI, web UI, REST API server, MCP server, or embedded via SDK. The `--mode` flag switches between interfaces.

## What Dexto Does Well

### 1. YAML-Driven Agent Configuration ⭐️

Each agent is a single YAML file defining: LLM provider/model, MCP servers, system prompt (with static and dynamic contributors), memory settings, storage backends, permissions, tools, hooks, and telemetry. Environment variables are interpolated. Agents are data, not code.

**Inber connection**: Inber defines agents in agent-store SQLite with programmatic configuration. YAML is more accessible for non-developers and easier to version-control. A YAML export/import for agent-store would bridge the gap.

### 2. Multi-Interface Runtime

Same agent definition runs across 6 different interfaces without code changes. This is the "write once, run everywhere" promise for agents.

**Inber connection**: Inber achieves something similar via the server (HTTP API + SSE + NATS bus), with `si` adapting to Discord/Telegram/Slack. But Dexto's single-binary approach is simpler to deploy.

### 3. Pluggable Storage Tiers

Three independent storage tiers: cache (Redis/in-memory), database (PostgreSQL/SQLite/in-memory), blob (local filesystem). Production uses Redis + PostgreSQL; dev uses in-memory + SQLite. Configured per-agent in YAML.

**Inber connection**: Inber uses SQLite everywhere (agent-store, model-store, session DB). Pluggable storage isn't needed today but would matter if scaling beyond a single server.

### 4. Signal Bus for Sub-Agent Communication

The orchestration package provides a `signal-bus` for inter-agent communication and a `task-registry` for managing spawned agents. Sub-agents run ephemerally with auto-cleanup. Tool approvals propagate from sub-agent to parent.

**Inber connection**: Inber uses NATS JetStream for agent communication, which is more robust (persistent, distributed) but heavier than an in-process signal bus.

### 5. Bidirectional MCP

Dexto is both an MCP client (connects to MCP servers for tools) and can expose itself as an MCP server (for integration into Claude Code, Cursor, etc.). This makes it interoperable in both directions.

### 6. OpenTelemetry Built-In

Distributed tracing with OTLP export. Enables observability dashboards without custom logging.

**Inber connection**: Inber uses logstack for centralized logging. OpenTelemetry would provide richer observability (traces, spans, metrics) with industry-standard tooling.

## What Inber Should Adopt

### 1. Agent Config Export/Import as YAML (MEDIUM PRIORITY)

Add `inber agents export <name> > agent.yaml` and `inber agents import agent.yaml` to agent-store. Makes agent definitions portable, version-controllable, and shareable without database access.

### 2. OpenTelemetry Integration (LOW PRIORITY)

Replace or complement logstack with OpenTelemetry for richer observability. Traces per RunTurn, spans per phase (prepare, context, execute, postprocess), metrics for token usage and latency. Works with any OTLP-compatible backend (Jaeger, Grafana Tempo, etc.).

### 3. Approval Propagation for Sub-Agents (MEDIUM PRIORITY)

When a sub-agent needs approval (tool confirmation), propagate the request up to the parent agent or user. Dexto's pattern: sub-agent approval surfaces through the parent's interface. Inber could route approval requests through the bus to the original requestor.

## What's Different

| Aspect | Dexto | Inber |
|--------|-------|-------|
| **Agent definition** | YAML files | SQLite agent-store |
| **Agent identity** | Generic (coding-agent, explore-agent) | Named with Irish mythology identities |
| **Multi-agent** | In-process signal bus, sub-agent spawning | NATS JetStream across processes |
| **Interfaces** | CLI, Web, REST, MCP, Discord, Telegram, SDK | Server (HTTP+SSE), NATS, si adapters |
| **Storage** | Pluggable (PG/SQLite/Redis/in-memory) | SQLite everywhere |
| **Observability** | OpenTelemetry | logstack (custom JSONL) |
| **Model support** | 50+ via provider adapters | Anthropic-focused via model-store |
| **License** | Elastic 2.0 (restrictive) | Internal |
| **Deployment** | Single Node.js process | Ecosystem of Go services |

## Key Takeaway

Dexto's YAML-driven agent definitions and multi-interface runtime make it the most accessible agent framework for getting started. For inber, the main takeaways are: **YAML export/import for agent configs** (portability), **OpenTelemetry for observability** (industry-standard tracing), and **approval propagation for sub-agents** (safety UX). The fundamental architectural difference — single-process vs distributed services — means inber is more robust for production multi-agent workloads, but Dexto is easier to prototype with.
