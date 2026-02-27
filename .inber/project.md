# Inber Project Context

**Project:** inber — Go-based agent orchestration framework  
**Repo:** github.com/kayushkin/inber  
**Language:** Go 1.21+

## Architecture

- **agent/** — Core agent loop, tool execution, hooks
- **context/** — Tag-based context system, auto-loading
- **memory/** — SQLite-backed persistent memory
- **tools/** — Built-in tools (shell, files, etc)
- **session/** — JSONL logging, session tracking
- **cmd/inber/** — CLI REPL and commands
- **agents/** — Agent definitions (markdown + JSON config)

## Build & Test Commands

```bash
# Build
go build -o inber ./cmd/inber/

# Test (unit tests, no API needed)
go test ./context/ -v
go test ./memory/ -v

# Test with API key (agent integration tests)
export $(cat .env | xargs)
go test ./agent/ -v -timeout=120s

# Test everything
export $(cat .env | xargs)
go test ./... -v

# Run
./inber                    # default agent
./inber -a fionn "task"    # specific agent
```

## Pre-Push Checklist

**Every commit MUST:**
1. Build cleanly: `go build ./cmd/inber/`
2. Pass all tests: `go test ./...`
3. Only then: `git push`

No exceptions. Broken builds = broken trust.

## Deployment

This is a CLI tool, not a service. "Deployment" means:
1. Build binary: `go build -o inber ./cmd/inber/`
2. Move to PATH: `mv inber ~/bin/` or `/usr/local/bin/`
3. Verify: `inber --version` (when implemented)

For releases:
```bash
# Tag release
git tag v0.1.0
git push origin v0.1.0

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o inber-linux-amd64 ./cmd/inber/
GOOS=darwin GOARCH=amd64 go build -o inber-darwin-amd64 ./cmd/inber/
GOOS=darwin GOARCH=arm64 go build -o inber-darwin-arm64 ./cmd/inber/
```

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` — Claude API
- `github.com/kayushkin/aiauth` — Auth management (OAuth + API keys)
- `github.com/mattn/go-sqlite3` — Memory storage (CGO dependency)
- `github.com/google/uuid` — UUID generation
- `github.com/joho/godotenv` — .env loading
- `github.com/spf13/cobra` — CLI framework

## Environment Setup

Required in `.env` (gitignored):
```bash
ANTHROPIC_API_KEY=sk-ant-...
```

Optional:
```bash
OPENAI_API_KEY=sk-...  # for avatar generation
```

## Git Workflow

- **main** branch is always buildable
- Feature branches for new work
- Squash commits before merging
- Clear commit messages: `feat: add X`, `fix: Y`, `refactor: Z`

## Code Style

- `gofmt` formatted (enforced)
- Clear variable names, no abbreviations unless obvious
- Comments explain "why", not "what"
- Error handling: always check errors, return early
- Tests: table-driven tests preferred

## Common Issues

**CGO errors with sqlite3:**
- Ensure GCC installed: `sudo apt-get install build-essential`
- Or use pure-Go sqlite: `modernc.org/sqlite` (future)

**API rate limits:**
- Claude API: tier-based limits
- Use smaller models for testing (sonnet vs opus)

**Memory corruption:**
- Memory DB at `.inber/memory.db`
- Safe to delete and rebuild if corrupted
- Backed up in git? No—add to .gitignore

## Project-Specific Conventions

- Agent identities are markdown in `agents/` and `agents/templates/`
- Agent configs are JSON in `agents.json`
- Context chunks use tags, not relevance scores
- All file paths relative to repo root
- Session logs in `logs/` (gitignored)
