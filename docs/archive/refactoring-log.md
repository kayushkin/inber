# inber Refactoring Log

## Phase 1: Extract Tools (Completed 2026-03-01)

**Goal:** Reduce inber's context footprint by extracting reusable components into standalone libraries.

### What Was Extracted

Created **github.com/kayushkin/agentkit** — a reusable tools library for Claude-based agents.

**Moved from inber:**
- `tools/shell/` → `agentkit/tools/shell.go`
- `tools/fs/` → `agentkit/tools/fs.go`
- `tools/internal/` → `agentkit/schema/`

**Repository:** https://github.com/kayushkin/agentkit

### Architecture Changes

**Before:**
```
inber/
  tools/
    shell/shell.go          (shell tool implementation)
    fs/read.go              (file read tool)
    fs/write.go             (file write tool)
    fs/edit.go              (file edit tool)
    fs/list.go              (directory listing)
    internal/helpers.go     (schema builders)
    internal/truncate.go    (output truncation)
    tools.go                (public API)
```

**After:**
```
github.com/kayushkin/agentkit/
  tool.go                   (Tool interface)
  tools/shell.go            (shell tool)
  tools/fs.go               (all file operations)
  schema/schema.go          (schema builders)
  schema/truncate.go        (truncation utilities)

inber/
  tools/
    tools.go                (adapter layer wrapping agentkit)
    integration_test.go     (compatibility tests)
    hooks_test.go           (verify hooks still work)
```

### Adapter Pattern

inber's `tools/tools.go` now wraps agentkit tools:

```go
import (
    "github.com/kayushkin/agentkit"
    agentkittools "github.com/kayushkin/agentkit/tools"
    "github.com/kayushkin/inber/agent"
)

func wrap(t agentkit.Tool) agent.Tool {
    return agent.Tool{
        Name:        t.Name,
        Description: t.Description,
        InputSchema: t.InputSchema,
        Run:         t.Run,
    }
}

func Shell() agent.Tool { return wrap(agentkittools.Shell()) }
// ... etc
```

This preserves the existing `tools.All()` API with zero breaking changes.

### Verification

**Tests added:**
- `tools/integration_test.go` — End-to-end tool execution
- `tools/hooks_test.go` — Verify hooks still intercept calls

**Manual testing:**
```bash
$ ./inber run "use shell to echo 'test'"
⚡ shell $ echo 'test'
test
```

**Hooks verification:**
The hook flow remains unchanged:
```
User message → Agent.Run() → OnToolCall hook → tool.Run() → OnToolResult hook
```

### Impact

**Lines removed from inber:** ~2300  
**Context budget saved:** Significant (tools no longer in repo map)  
**Breaking changes:** None (adapter layer preserves API)  
**Tests passing:** ✅ All tests pass

### Benefits

1. **Reusability:** Anyone building Claude agents can use agentkit
2. **Separation of concerns:** Tool implementations are framework-agnostic
3. **Reduced context:** inber's repo map is smaller
4. **Maintainability:** Tools have their own repo, issues, releases

### Next Steps

**Phase 2:** Extract `context/` package to `github.com/kayushkin/llmcontext`
- Tag-based context builder
- Budget-aware chunk filtering
- Auto-loading system
- File loaders, repo maps, recency detection

**Phase 3:** Extract `memory/` after embeddings stabilize
- Wait until we replace TF-IDF with real embeddings
- Create `github.com/kayushkin/agentmem` with pluggable embedder interface

## Lessons Learned

1. **Adapter pattern works well** — preserving existing APIs prevents breaking changes
2. **Test before pushing** — integration tests caught compatibility issues early  
3. **SSH key setup matters** — had to use `GIT_SSH_COMMAND="ssh -i ~/.ssh/ghkayushkin"` for pushing
4. **Tool interface is stable** — agentkit.Tool → agent.Tool conversion is trivial

## References

- agentkit repo: https://github.com/kayushkin/agentkit
- Commit: `09e867b` - "Extract tools to github.com/kayushkin/agentkit"
- Discussion: Started 2026-03-01, completed same day
