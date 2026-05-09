# Goose Comparison

**GitHub:** github.com/block/goose
**Language:** Rust
**Focus:** AI agent framework with CLI and desktop interfaces, extensible via MCP (Model Context Protocol)

## Architecture Overview

Goose is a comprehensive AI agent framework built in Rust with a multi-tier architecture:

```
┌─ UI Layer ──────────────────────────────────────┐
│  CLI (goose-cli) | Desktop (Electron) | Custom  │
└─────────────────┼─────────────────────────────────┘
                  │
┌─ Server Layer ──┼──────────────────────────────────┐
│  goose-server (REST API) + Agent Client Protocol  │
└─────────────────┼─────────────────────────────────┘
                  │
┌─ Core Layer ────┼──────────────────────────────────┐
│  Provider System | Extension Manager | Config      │
└─────────────────────────────────────────────────────┘
```

## Extension/Plugin Architecture

**MCP-Centric Design:**
- Uses Model Context Protocol (MCP) as the primary extension mechanism
- Extensions can be: builtin (Rust), stdio processes, HTTP servers, or inline Python
- Clean separation: extensions provide tools, resources, and instructions to the core

**Extension Types:**
```rust
enum ExtensionConfig {
    Builtin { name, timeout },
    Platform { name },  // In-process Rust extensions
    Stdio { cmd, args, envs },
    StreamableHttp { uri, headers },
    InlinePython { code, dependencies },
}
```

**Tool Management:**
- Tools are automatically prefixed (`extension__tool_name`) unless marked as "unprefixed"
- Extensions declare which tools they provide via MCP
- Runtime discovery and registration of tools
- Built-in tool validation and parameter checking

## Provider System

**Clean Abstraction:**
```rust
#[async_trait]
pub trait Provider {
    async fn stream(&self, model_config, session_id, system, messages, tools) -> MessageStream;
    async fn complete(&self, ...) -> (Message, ProviderUsage);
    fn get_model_config(&self) -> ModelConfig;
    // ... other methods
}
```

**Dual Provider Support:**
- **Declarative Providers:** JSON configuration files for OpenAI-compatible APIs
- **Code Providers:** Full Rust implementations for complex providers (Anthropic, etc.)
- Provider metadata includes model capabilities, context limits, cost information

## Key Architectural Strengths

### 1. **Separation of Concerns**
- Core logic, providers, extensions, and UIs are cleanly separated
- Each crate has a single responsibility
- Clear interfaces between layers

### 2. **Extension Modularity**
- MCP standard enables language-agnostic extensions
- Extensions are self-contained (declare their own tools, resources, instructions)
- Dynamic loading and unloading of extensions
- Built-in security model for extension execution

### 3. **Multi-Interface Support**
- CLI, desktop app, and custom UIs all use the same core
- Server mode enables web/mobile/IDE integrations
- Agent Client Protocol (ACP) for rich programmatic access

### 4. **Recipe System**
- YAML-based workflow definitions
- Parameter substitution and templating
- Sub-recipes and subagent composition for complex workflows
- Shareable and distributable workflow packages

## What Inber Should Adopt

### 1. **Provider Interface Design**
Goose's `Provider` trait is excellent:
- Clean async interface with streaming support
- Metadata-driven provider discovery
- Support for both declarative (JSON) and code-based providers
- Usage tracking and model capability detection

**Inber Action:** Extract a similar `Provider` interface in `agent/provider.go` that wraps the current Anthropic SDK coupling.

### 2. **Extension Architecture**
The MCP-based extension system is superior to inber's current approach:
- **Self-describing:** Extensions declare their tools, not the core
- **Multi-transport:** stdio, HTTP, in-process
- **Standard protocol:** MCP is becoming an industry standard
- **Security model:** Clear boundaries and permission system

**Inber Action:** 
- Evaluate MCP adoption for inber's tool system
- Move towards self-describing tools vs. hardcoded tool registration
- Consider stdio and HTTP tool execution models

### 3. **Configuration Structure**
Goose's config hierarchy is clean:
```
- Provider configs (declarative JSON + secrets)
- Extension configs (with environment substitution) 
- Recipe configs (workflow definitions)
- Global settings
```

**Inber Action:** Simplify inber's config system with similar separation.

### 4. **Crate Organization**
```
goose/              # Core logic
goose-cli/          # CLI interface
goose-server/       # HTTP API server  
goose-acp/          # Agent Client Protocol
goose-mcp/          # MCP client implementations
```

**Inber Action:** Consider breaking inber into similar focused modules.

## What Goose Does Differently

### 1. **Recipe-First Workflows**
- Complex multi-step workflows are first-class citizens
- YAML configuration for reproducible agent behaviors
- Sub-recipe composition and parallel execution

*Inber equivalent:* Could add recipe/workflow system to inber.

### 2. **Multi-Model Orchestration**
- Lead/worker model patterns built-in
- Fast model fallbacks
- Model capability-aware tool routing

*Inber potential:* Inber could benefit from multi-model support.

### 3. **Rich Session Management**  
- Session persistence and resumption
- Session-specific extensions and configurations
- Cross-session subagent spawning

*Inber gap:* Inber has simpler session model.

## What Inber Does Better

### 1. **Memory Architecture**
Inber's memory system (conversation summarization, pruning, stashing) is more sophisticated than Goose's basic session persistence.

### 2. **Context Management**
Inber's smart truncation and context loading is more advanced than Goose's approach.

### 3. **Simplicity**
Inber's current model is simpler and more focused, while Goose tries to be everything to everyone.

## Recommended Adoptions for Inber

**High Priority:**
1. **Provider interface extraction** - Decouple from Anthropic SDK
2. **Self-describing tools** - Move towards MCP-style tool registration
3. **Declarative provider configs** - Support JSON-based provider definitions

**Medium Priority:**
4. **Extension stdio/HTTP support** - Beyond just in-process tools
5. **Recipe system** - For workflow standardization
6. **Module reorganization** - Break into focused packages

**Low Priority:**
7. **Multi-model support** - Lead/worker patterns
8. **Desktop interface** - Beyond CLI

The key insight: **Goose's architecture is more modular and extensible**, while **inber's core agent logic is more sophisticated**. Inber should adopt Goose's modularity patterns while preserving its advanced memory and context management.

---

## Harness-watch — 2026-05-09: Projects as backend sources, system-prompt injection

[PR 8739](https://github.com/block/goose/pull/8739) (merged 2026-05-07) graduates "projects" from a Tauri-only frontend IPC concept into a first-class ACP `sources` entity served by `goose serve`, with two design choices worth pulling out:

- **System-prompt injection at the agent layer.** Project instructions previously came from the desktop client and were prepended to every user turn. Now the project source is read server-side and injected into the *system prompt* once per conversation. This is a direct prompt-cache hit-rate win — the cacheable prefix stops being invalidated by a per-turn prepended payload.
- **Storage as `.md` + YAML frontmatter under `Paths::data_dir()/projects/`.** Project definitions live as plain files with structured metadata (name, description, instructions, working dirs), making them human-editable and version-controllable. Skills become project-scoped via the same working-dir scan.

**What inber should consider:** Inber has at least two surfaces today that prepend per-turn rather than inject into the system prompt — the conversation summary header and the project-context block built by `engine/turn_prompt.go`. Whatever is *stable for the duration of a session* (project-level `INBER.md`, agent identity card, tool inventory description) belongs in the system prompt where it'll cache, not in the user turn where each new turn pushes it past the cache breakpoint. The goose pattern also argues for promoting any "project" concept inber adopts (today closest to forge worktree slots + agent-store config) to a server-side source rather than something the chat frontend owns. Worth a section in `docs/cache-optimization.md` cross-referencing `reference-based-prompt-architecture.md` — the two notes already converge on this thesis.