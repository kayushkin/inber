# Post-Write Hooks v2 — Project-Aware Build/Test

## Problem

Current `PostWriteHook` hardcodes Go detection via `go.mod`. But:
- Different projects need different commands (npm test, cargo build, make, etc.)
- Even within a project, the right check changes over time (new test suites, lint rules)
- Some files shouldn't trigger builds (docs, config, assets)
- Some projects want deploy hooks too (restart server after code changes)

## Design: `.inber/hooks.toml`

Each project defines its hooks in `.inber/hooks.toml` (or inline in `agents.json`).
Falls back to auto-detection if no config exists (current behavior).

```toml
# .inber/hooks.toml

[post-write]
# Which files trigger which hooks (glob patterns)
# Hooks run in order, stop on first failure

[[post-write.hooks]]
name = "build"
trigger = ["*.go", "go.mod", "go.sum"]
command = ["go", "build", "./..."]
timeout = "30s"

[[post-write.hooks]]
name = "test"
trigger = ["*.go"]
command = ["go", "test", "./..."]
timeout = "120s"
depends_on = "build"  # only runs if build succeeds

[[post-write.hooks]]
name = "lint"
trigger = ["*.go"]
command = ["golangci-lint", "run"]
timeout = "60s"
optional = true  # failure is warning, not error

# Node.js example
# [[post-write.hooks]]
# name = "typecheck"
# trigger = ["*.ts", "*.tsx"]
# command = ["npx", "tsc", "--noEmit"]
# timeout = "30s"
#
# [[post-write.hooks]]
# name = "test"
# trigger = ["*.ts", "*.tsx", "*.test.*"]
# command = ["npm", "test", "--", "--passWithNoTests"]
# timeout = "60s"

# Rust example
# [[post-write.hooks]]
# name = "check"
# trigger = ["*.rs", "Cargo.toml"]
# command = ["cargo", "check"]
# timeout = "60s"
#
# [[post-write.hooks]]
# name = "test"
# trigger = ["*.rs"]
# command = ["cargo", "test"]
# timeout = "120s"
# depends_on = "check"

[post-write.options]
# Run hooks in background? (don't block tool response)
background = false
# Deduplicate identical errors
dedup = true
# Max error lines to show
max_error_lines = 5
# Ignore patterns (never trigger hooks for these files)
ignore = ["*.md", "*.txt", "*.json", ".gitignore", "docs/**"]
```

## How It Evolves

The agent itself can update `.inber/hooks.toml` as the project grows:
- Added a new test framework? Update the test command
- Set up a linter? Add a lint hook
- Deploy target? Add a post-push hook
- New file types? Update trigger patterns

The hook config is version-controlled with the project, so it stays in sync.

## Auto-Detection Fallback

If no `.inber/hooks.toml` exists, fall back to current behavior:
1. `go.mod` → `go build ./...` + `go test ./...`
2. `package.json` → `npm run build` (if script exists) + `npm test`
3. `Cargo.toml` → `cargo check` + `cargo test`
4. `Makefile` → `make` (if default target exists)

## Implementation Plan

1. Define `HookConfig` struct matching TOML schema
2. Load from `.inber/hooks.toml` in `PostWriteHook` constructor
3. Match changed file against trigger globs
4. Execute matching hooks in dependency order
5. Compact error output (existing `compactGoError` generalizes)
6. Add `inber hooks` CLI command to list/test hooks

## Key Differences Per Project Type

| Aspect | Go | Node/TS | Rust | Python |
|--------|-----|---------|------|--------|
| Build check | `go build ./...` | `tsc --noEmit` | `cargo check` | (none) |
| Test | `go test ./...` | `npm test` | `cargo test` | `pytest` |
| Lint | `golangci-lint` | `eslint` | `clippy` | `ruff` |
| Format | `gofmt` (enforced) | `prettier` | `rustfmt` | `black` |
| Speed | Fast (<5s) | Medium (5-15s) | Slow (10-60s) | Fast (<5s) |
| Watch mode | N/A | Hot reload | N/A | N/A |

## Post-Push Hooks (Future)

```toml
[[post-push.hooks]]
name = "deploy"
command = ["./deploy.sh"]
timeout = "60s"
confirm = true  # ask user before running
```
