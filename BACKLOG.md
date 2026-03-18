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
- [~] **Reduce engine/ file count** — Currently ~15 files in engine/. Can any be merged? (e.g., lifecycle.go + turn.go? display.go + log.go?)
- [x] **Simplify conversation package** — `summarize.go`, `prune.go`, `stash.go`, `extract.go` — are all 4 needed? Can prune+stash merge?
- [~] **Remove hardcoded model list** — `agent/models.go` has a static model list partially superseded by model-store. Use model-store as sole source of truth.
- [x] **Audit server/ package** — It's the largest package. What can be extracted? (e.g., bus integration → separate package?)

## 🧩 Modularization

- [x] **Make memory store swappable** — Define a `MemoryStore` interface. Current SQLite impl becomes one backend. Could swap in Redis, filesystem, or even in-memory for tests.
- [x] **Make session store swappable** — Same pattern. Interface + SQLite impl. Enables future Postgres or distributed backends.
- [x] **Extract bus client** — `server/bus.go` could be its own small package or even a separate module. Other tools (logpush, dashboard) also need bus access.
- [x] **Extract forge/workspace interface** — `server/forge_iface.go` is already an interface — verify it's clean and could be swapped for a different workspace isolation strategy.
- [x] **Plugin/extension system** — Can tools be loaded dynamically? (Go plugins, or exec-based like MCP?) Research feasibility.

## 🧹 Code Quality

- [ ] **Add package-level doc comments** — Every package should have a doc.go or comment explaining its purpose
- [ ] **Consistent error handling** — Audit for bare `fmt.Errorf` vs `%w` wrapping, ensure errors are traceable
- [ ] **Reduce global state** — Check for package-level vars that should be struct fields
- [ ] **Test coverage audit** — Which packages have 0% coverage? Add basic tests for untested code paths
- [ ] **Dependency audit** — `go mod tidy`, check for unused or heavy deps that could be replaced

## 📐 Architecture

- [ ] **Document the actual data flow** — From user input → engine → provider → tool execution → response. A single clear diagram in docs/
- [ ] **Define package boundaries** — Write a brief "who calls whom" rule. engine → memory ✓, memory → engine ✗, etc.
- [ ] **Evaluate splitting into Go modules** — Could `memory/`, `agent/`, `engine/` be separate Go modules? Pros/cons of mono-module vs multi-module.

---

## 💡 Ideas

- Benchmark inber's startup time — how fast from `inber run` to first API call?
- Profile memory usage during long sessions
- Compare inber's token efficiency against pi-mono and openclaw on identical tasks
- Could inber's memory system work as a standalone library?
