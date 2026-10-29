# Aider Comparison

**Last reviewed**: 2026-10-29 against Aider-AI/aider@5dc9490bb (2026-05-22).

**Project**: [Aider](https://github.com/Aider-AI/aider)
**Status**: Not archived, but quiet. The last commit is 2026-05-22 (82 commits in 2026, none since), the last tagged release is v0.86.2, and `main` reports `0.86.3.dev`. Treat it as maintained in spurts, not as a fast-moving upstream.
**Language**: Python
**Focus**: Pair programming in the terminal, built around git.
**Key Strengths**: Repository map, git integration, context efficiency, lint/test loop, editor watch mode, voice input.

## Architecture Overview

Aider is a terminal coding assistant that works inside an existing git repo. The user adds files to the chat; Aider sends them with a repository map of the rest, the model answers in one of several edit formats, and Aider applies the edits and commits them. It has no MCP support and no subagents.

## What They Do Well

### 1. Repository Map ⭐️

- **Tree-sitter tags**: `aider/repomap.py` parses each file with tree-sitter queries (`grep_ast`, `tree_sitter`) to find definitions and references. Bash tags were added in May 2026.
- **Signatures, not bodies**: the map shows classes, functions and their signatures.
- **Graph ranking**: files and symbols are ranked by how often others reference them, and the top of the ranking fills the budget.
- **Budget**: `--map-tokens` (default 1024 in `RepoMap`), multiplied by `map_mul_no_files` (8) when no files are in the chat.

Example map:
```
aider/coders/base_coder.py:
⋮...
│class Coder:
│ abs_fnames = None
⋮...
│ @classmethod
│ def create(
│ self,
│ main_model,
│ edit_format,
│ io,
│ skip_model_availabily_check=False,
│ **kwargs,
⋮...
│ def run(self, with_message=None):
```

### 2. Git Integration
- **Auto-commits** on by default (`--auto-commits`), with a model-written commit message.
- **Dirty files** are committed first, so the user's own edits and Aider's stay apart.
- **`/undo`** reverts Aider's last commit.
- **Attribution**: by default a `Co-authored-by` trailer (`--attribute-co-authored-by`, default on); the older "(aider)" author-name suffix is now opt-in.

### 3. Context Management
- Token budget for the map, grown when no files are in the chat.
- Relevance from the reference graph.

### 4. Multi-file Editing and the Lint/Test Loop
- Edits several related files in one reply, using the map to find them.
- Runs the linter and tests after each change and feeds failures back to the model.

## What Inber Could Adopt

### 1. A Repository Map (HIGH PRIORITY, still open)

inber has no repo map. `codeindex/codeindex.go` was written as Aider's map in Go, but every method is a `TODO` that returns nothing, and the engine opens it without reading from it (`engine/engine.go`, `engine/engine_types.go` `EnableCodeIndex`). Building it means: tree-sitter (or `go/ast` for Go first) to extract definitions and references, a reference graph with PageRank-style ranking, and a token budget.

### 2. Better Commit Messages

inber already auto-commits each file write by default (`engine/workflow_hooks.go`, `DefaultAutoWorkflowConfig`), with an explicit pathspec so it never sweeps up another agent's staged files (`engine/workflow_git.go`). Its messages are mechanical ("Create x.go", "Update x.go"). Aider's model-written messages and co-author trailer are the parts worth taking.

### 3. Lint/Test Feedback

inber's workflow hooks format after a write and can run build checks (`engine/workflow_format.go`, `engine/workflow_build.go`), but compiler and test output does not go back to the model as context.

## What's Different

### Scope & Purpose
- **Aider**: a coding tool for one person in one repo, run from the terminal.
- **Inber**: a Go server that runs several named agents, each a general tool-using agent, reached over HTTP, llm-bridge and a NATS bus.

### Session Model
- **Aider**: chat history in files beside the repo; git history is the durable record.
- **Inber**: server sessions in SQLite, resumable and forkable (`server/api_bridge.go`), plus memory-store for what outlives a session.

### Tool Philosophy
- **Aider**: no tool calls; the model writes edits in a fixed format that Aider applies.
- **Inber**: tool calls (shell, files, memory, spawn and others from tool-store).

### Context Strategy
- **Aider**: repository-centred, from the map.
- **Inber**: memory-centred: tag-matched memories within a token budget (`engine/turn_context.go`), large blocks moved out behind `memory_expand` pointers (`conversation/stash.go`).

## Key Takeaway

Aider's repository map is still the idea inber most lacks for coding work, and the stub in `codeindex/` shows the plan was made and never carried out. Git auto-commit, the other half of Aider's design, inber already has in a cruder form.
