# BACKLOG.md — Inber

Focus: **simplification, modularization, swappable components**. Not feature creep.
Look at other frameworks for inspiration. Make inber lean and composable.

Pick the top unclaimed item, do it, push. Mark `[x]` when done, `[~]` in progress.

---

## 📚 Framework Comparisons

Study these projects for architectural ideas. For each, write a brief comparison note in `docs/comparisons/<name>.md` — what they do well, what inber could adopt, what's different.

- [x] **pi-mono** (github.com/badlogic/pi-mono) — TypeScript monorepo. Clean package separation: `pi-ai` (unified LLM API), `pi-agent-core` (runtime + tools + state), `pi-coding-agent` (CLI), `pi-tui`, `pi-web-ui`. Study how they decouple the LLM provider from the agent runtime. Inber currently couples Anthropic SDK deeply — could we have a thin provider interface?
- [x] **OpenClaw** (github.com/openclaw/openclaw) — Node.js. Study session management, tool policies, heartbeat system, channel abstractions. What does inber do better? What should inber steal?
- [x] **Claude Code** (Anthropic's CLI) — Study their tool permission model, session resume, and how they handle multi-file edits
- [x] **Goose** (github.com/block/goose) — Rust agent. Study their extension/plugin system and how tools are modular
- [x] **Aider** (github.com/paul-gauthier/aider) — Python coding agent. Study their repo-map approach, git integration, and how they keep context lean
- [x] **Hermes Agent** (from skill-recommendations.md) — Study orchestration patterns

## 🔧 Simplification

- [x] **Extract LLM provider interface** — Create a thin `Provider` interface (`Complete(messages) → response`) that wraps Anthropic SDK. This unblocks future provider swaps (OpenAI, local models) without touching agent logic. Start in `agent/provider.go`.
- [x] **Extract tool interface** — Ensure tools are fully self-contained (name, description, schema, execute). Should be possible to register a tool with zero knowledge of inber internals. Check current `agentkit/tools/` — how close are we?
- [x] **Simplify engine/build.go** — After the context/memory merge, this file may have dead branches or unnecessary complexity. Audit and trim.
- [x] **Reduce engine/ file count** — Currently ~15 files in engine/. Can any be merged? (e.g., lifecycle.go + turn.go? display.go + log.go?)
- [x] **Simplify conversation package** — `summarize.go`, `prune.go`, `stash.go`, `extract.go` — are all 4 needed? Can prune+stash merge?
- [x] **Remove hardcoded model list** — `agent/models.go` has a static model list partially superseded by model-store. Use model-store as sole source of truth.
- [x] **Audit server/ package** — It's the largest package. What can be extracted? (e.g., bus integration → separate package?)

## 🧩 Modularization

- [x] **Make memory store swappable** — Define a `MemoryStore` interface. Current SQLite impl becomes one backend. Could swap in Redis, filesystem, or even in-memory for tests.
- [x] **Make session store swappable** — Same pattern. Interface + SQLite impl. Enables future Postgres or distributed backends.
- [x] **Extract bus client** — `server/bus.go` could be its own small package or even a separate module. Other tools (logpush, dashboard) also need bus access.
- [x] **Extract forge/workspace interface** — `server/forge_iface.go` is already an interface — verify it's clean and could be swapped for a different workspace isolation strategy.
- [x] **Plugin/extension system** — Can tools be loaded dynamically? (Go plugins, or exec-based like MCP?) Research feasibility.

## 🧹 Code Quality

- [x] **Extract turn lifecycle helpers from engine/turn.go** — Extracted summarizeIfNeeded, pruneConfig, pruneIfNeeded, checkpointIfNeeded, saveMessages, LogUser, LogAssistant, SaveSessionSummary, and FindRepoRoot functions into turnLifecycle.go. Reduced turn.go from 618 to 413 lines (~205 line reduction). Improved separation of concerns: turn.go focuses on core turn execution while turnLifecycle.go handles conversation lifecycle management.
- [x] **Move CosineSimilarity to embedding.go** — Moved the cosineSimilarity function from memory.go (897→879 lines) to embedding.go where it belongs logically. Better modularity and cleaner separation of concerns.
- [x] **Extract memory compaction logic** — Moved the compaction functionality (CompactionResult struct, Compact method) from memory.go into memory/compaction.go. Reduced memory.go from 879 to 735 lines (~144 line reduction) and improved modularity by separating distinct compaction concern.
- [x] **Add package-level doc comments** — Every package should have a doc.go or comment explaining its purpose
- [x] **Consistent error handling** — Audit for bare `fmt.Errorf` vs `%w` wrapping, ensure errors are traceable
- [x] **Reduce global state** — Check for package-level vars that should be struct fields
- [x] **Test coverage audit** — Which packages have 0% coverage? Add basic tests for untested code paths
- [x] **Dependency audit** — Replaced mattn/go-sqlite3 (CGO) with modernc.org/sqlite (pure Go). This eliminates the need for CGO in builds, making the binary more portable and simplifying compilation. The Anthropics SDK pulls in heavy cloud dependencies but those are necessary for functionality.
- [x] **Split conversation/manage.go** — 997 lines, combines pruning + stashing responsibilities. Extract stashing logic into separate `stash.go` file with clear interface boundaries. Keep pruning logic in `manage.go` or rename to `prune.go`.
- [x] **Add ModelStore field to EngineConfig** — Added ModelStore field to EngineConfig to enable sharing model store instances between server and engine. Engine now uses provided ModelStore instead of always opening its own. Resolves TODO in server/session.go and improves resource efficiency.

## 📐 Architecture

- [x] **Document the actual data flow** — From user input → engine → provider → tool execution → response. A single clear diagram in docs/
- [x] **Define package boundaries** — Write a brief "who calls whom" rule. engine → memory ✓, memory → engine ✗, etc.
- [x] **Evaluate splitting into Go modules** — Could `memory/`, `agent/`, `engine/` be separate Go modules? Pros/cons of mono-module vs multi-module.

- [x] **Extract session management from memory/memory.go** — The memory.go file contains both memory storage/retrieval and session management (SaveSession, TrackMemoryUsage, Session struct). Extract session-related code into memory/sessions.go to improve separation of concerns and reduce the size of memory.go from 736 lines.
- [x] **Split conversation/manage.go** — 737 lines combining configuration, core management logic, and utilities. Extract configuration types/functions into `manage_config.go`, keep core logic in `manage.go`, and move helper functions to `manage_utils.go`. Better separation of concerns and easier to maintain.
- [x] **Extract configuration from server/server.go** — Extracted Config, AgentConfig types and LoadConfig, ConfigFromAgents functions into `server/config.go`. Reduced server.go from 651 to 590 lines (~61 line reduction). Better separation of concerns between server logic and configuration handling.

---

## 🧹 Code Quality (Continued)

- [x] **Split server/spawn.go** — 626 lines combining spawning logic, result delivery, memory management, and tool definitions. Extract tool definitions into `spawn_tools.go` and delivery logic into `spawn_delivery.go`. Keep core spawning logic in `spawn.go`. Better separation of concerns.

- [x] **Split memory/memory.go** — Extracted search logic into `search.go` (145 lines), memory management into `management.go` (132 lines), and utility functions into `util.go` (52 lines). Reduced memory.go from 646 to 335 lines (48% reduction). Better separation of concerns: core store operations, search functionality, memory management, and utilities are now in focused modules.

- [x] **Split session/session.go** — Extracted logstack integration into `session/logstack.go`. Created LogstackAdapter to encapsulate logstack functionality. Reduced session.go from 612 to 557 lines (55-line reduction). Better separation of concerns between session logging and logstack integration.

---

## 🧹 Code Quality (New)

- [x] **Split engine/build.go** — Extracted tool building logic into `build_tools.go` (112 lines), system prompt logic into `build_prompts.go` (150 lines), and hook building logic into `build_hooks.go` (188 lines). Reduced build.go from 586 to 78 lines (87% reduction). Better separation of concerns between tool configuration, prompt construction, hook setup, and core agent building.
- [x] **Split conversation/summarize.go** — Extracted configuration into `summarize_config.go` (51 lines), message utilities into `message_utils.go` (217 lines), and summary generation into `summary_generation.go` (125 lines). Reduced main summarize.go from 480 to 118 lines (75% reduction). Better separation of concerns between configuration, core logic, message analysis, and summary generation.

- [~] **Split agent/openai.go** — 472 lines combining OpenAI client, type definitions, message conversion, and tool conversion. Extract types into `openai_types.go`, conversion logic into `openai_conversion.go`, and utilities into `openai_utils.go`. Keep core client in `openai.go`. Better separation of concerns and easier maintenance.

---

## 💡 Ideas

- Benchmark inber's startup time — how fast from `inber run` to first API call?
- Profile memory usage during long sessions
- Compare inber's token efficiency against pi-mono and openclaw on identical tasks
- Could inber's memory system work as a standalone library?
