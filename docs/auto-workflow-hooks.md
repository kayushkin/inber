# Auto-Workflow Hooks Design

## Current State

We already have a `PostWriteHook` that auto-runs `go build` and `go test` after file edits. It's working well:

✅ **Already Automated:**
- `go build` after every `write_file` / `edit_file`
- `go test` (runs after build passes)
- Error deduplication (doesn't spam same error)
- Silent on success (only speaks up on failure)
- Project type detection (`go.mod` → Go project)

## What Could Be Automated Further?

### 1. **Git Auto-Commit** (Low-Hanging Fruit)

**Current:** Model manually calls `shell("git add . && git commit -m '...'")` 

**Proposed:** Auto-commit after every successful write with a smart commit message:

```go
// After write_file or edit_file succeeds:
type GitAutoCommit struct {
    repoRoot string
    enabled  bool
}

func (g *GitAutoCommit) OnToolResult(toolName, filePath, content string) {
    if !g.enabled || (toolName != "write_file" && toolName != "edit_file") {
        return
    }
    
    // Smart commit message based on what changed
    msg := g.generateCommitMessage(toolName, filePath)
    
    cmd := exec.Command("git", "-C", g.repoRoot, "add", filePath)
    cmd.Run()
    
    cmd = exec.Command("git", "-C", g.repoRoot, "commit", "-m", msg)
    cmd.Run()
}

func (g *GitAutoCommit) generateCommitMessage(tool, path string) string {
    if tool == "write_file" {
        return fmt.Sprintf("Create %s", filepath.Base(path))
    }
    return fmt.Sprintf("Update %s", filepath.Base(path))
}
```

**Benefits:**
- No more `git add . && git commit` spam in conversation
- Clean git history
- Model doesn't waste tokens on git commands

**Risks:**
- What if model makes multiple changes that should be one commit?
- What about commit message quality?

**Solution:** Auto-commit to a temporary branch `inber/<session-id>`, let user merge/squash later:

```bash
# On session start:
git checkout -b inber/abc123

# On session end:
echo "Session complete. Merge with:"
echo "  git merge --squash inber/abc123"
```

---

### 2. **Auto-Branch Creation** (Session Isolation)

**Current:** Model works on whatever branch you're on

**Proposed:** Auto-create a branch per session:

```go
// On session start:
func (e *Engine) initSessionBranch() error {
    branchName := fmt.Sprintf("inber/%s-%s", e.AgentName, e.Session.SessionID()[:8])
    
    // Check if branch exists
    cmd := exec.Command("git", "rev-parse", "--verify", branchName)
    if cmd.Run() != nil {
        // Create new branch
        cmd = exec.Command("git", "checkout", "-b", branchName)
        return cmd.Run()
    }
    
    // Resume existing branch
    cmd = exec.Command("git", "checkout", branchName)
    return cmd.Run()
}
```

**Benefits:**
- Session isolation (each agent session gets its own branch)
- Easy rollback (just delete the branch)
- Resume support (continue work on same branch)
- No conflicts with other sessions

**When to merge:**
- Manual: `inber session merge` command
- Auto: `--auto-merge` flag merges on clean exit

---

### 3. **Smart Test Selection** (Faster Feedback)

**Current:** Runs ALL tests after every change

**Proposed:** Only run tests for changed files:

```go
func (h *PostWriteHook) runGoTests(changedFile string) string {
    // If editing foo.go, run TestFoo* only
    if strings.HasSuffix(changedFile, "_test.go") {
        return h.runTestFile(changedFile)
    }
    
    // Find corresponding test file
    testFile := strings.TrimSuffix(changedFile, ".go") + "_test.go"
    if fileExists(testFile) {
        return h.runTestFile(testFile)
    }
    
    // Fall back to package tests
    pkg := filepath.Dir(changedFile)
    return h.runTestPackage(pkg)
}
```

**Benefits:**
- Faster feedback (0.1s vs 2s for large repos)
- Less noise in output

**Risks:**
- Might miss integration failures
- Needs to detect cross-package dependencies

**Solution:** Two-tier testing:
1. **Fast:** Test only changed package (runs immediately)
2. **Full:** Test everything (runs on `--test-all` or before merge)

---

### 4. **Auto-Format on Write** (Code Consistency)

**Current:** Model might produce poorly formatted code

**Proposed:** Auto-run `gofmt` / `prettier` / `rustfmt` after writes:

```go
func (h *PostWriteHook) formatCode(filePath string) {
    switch h.projectType {
    case "go":
        exec.Command("gofmt", "-w", filePath).Run()
    case "node":
        exec.Command("prettier", "--write", filePath).Run()
    case "rust":
        exec.Command("rustfmt", filePath).Run()
    }
}
```

**Benefits:**
- Consistent style
- Fewer formatting commits
- Works with any formatter

---

### 5. **Auto-Dependency Installation** (Fix "package not found")

**Current:** Model gets `package X not found`, then manually runs `go get X`

**Proposed:** Auto-detect missing imports and install:

```go
func (h *PostWriteHook) autoInstallDeps(buildOutput string) string {
    // Parse error: "package github.com/foo/bar not found"
    re := regexp.MustCompile(`package ([^\s]+) not found`)
    matches := re.FindAllStringSubmatch(buildOutput, -1)
    
    for _, match := range matches {
        pkg := match[1]
        cmd := exec.Command("go", "get", pkg)
        if err := cmd.Run(); err == nil {
            fmt.Fprintf(os.Stderr, "✓ installed %s\n", pkg)
        }
    }
    
    // Re-run build
    return h.runGo()
}
```

**Benefits:**
- One less round-trip
- Faster development

**Risks:**
- Might install wrong version
- Security concerns (auto-installing unknown packages)

**Solution:** Dry-run mode + confirmation:
```
⚠️ Missing packages detected:
  - github.com/foo/bar
  - github.com/baz/qux
Run 'go get' automatically? [y/N]
```

---

## Recommendation: Tiered Approach

### **Phase 1: Low-Hanging Fruit (Add Now)**

1. **Auto-branch per session** - zero downside, huge upside
2. **Auto-commit with smart messages** - saves 1-2 tool calls per change
3. **Auto-format on write** - silent improvement

These are safe, predictable, and save tokens immediately.

### **Phase 2: Smart Optimizations (Add Later)**

4. **Smart test selection** - needs more logic but big speedup
5. **Auto-dependency installation** - needs safety checks

### **Phase 3: Advanced (Research)**

6. **Incremental compilation** - cache build artifacts
7. **Parallel testing** - run tests concurrently
8. **AI-powered commit messages** - use LLM to summarize changes

---

## Configuration

Add to `EngineConfig`:

```go
type AutoWorkflowConfig struct {
    AutoBranch     bool   // Create branch per session
    AutoCommit     bool   // Commit after every write
    AutoFormat     bool   // Run formatter on write
    SmartTests     bool   // Only run relevant tests
    AutoInstall    bool   // Install missing deps
    BranchPrefix   string // e.g. "inber/" or "ai/"
}
```

Default: All enabled except `AutoInstall` (opt-in for security).

---

## Implementation Checklist

- [ ] Extract `PostWriteHook` into `cmd/inber/workflow_hooks.go`
- [ ] Add `GitAutoCommit` hook
- [ ] Add `AutoBranch` logic to `NewEngine()`
- [ ] Add `AutoFormat` to `PostWriteHook`
- [ ] Add `SmartTestSelection` (optional)
- [ ] Add config flags: `--no-auto-branch`, `--no-auto-commit`, etc.
- [ ] Update docs with auto-workflow behavior
- [ ] Add session-end summary: "Changes committed to branch inber/abc123"

---

## Example Session

```bash
$ inber run "Add a /health endpoint"

# Engine automatically:
# 1. Creates branch: inber/run-abc123
# 2. Agent writes handler.go
# 3. Auto-runs: go build (success)
# 4. Auto-runs: gofmt -w handler.go
# 5. Auto-commits: "Create handler.go"
# 6. Agent writes handler_test.go
# 7. Auto-runs: go test ./...
# 8. Auto-commits: "Add handler tests"

✓ Session complete. Changes on branch: inber/run-abc123
  Merge with: git merge --squash inber/run-abc123
```

**Token savings per session:**
- 2-3 git commands avoided: ~60 tokens
- 1-2 build commands avoided (they already run): ~0 tokens
- Cleaner conversation: ~100 tokens (less noise)

**Total:** ~150 tokens/session, or **5% savings on typical coding session**.

---

## Open Questions

1. **Should auto-commit be per-file or batched?**
   - Per-file: Finer-grained history
   - Batched: Fewer commits, cleaner log

2. **What about merge conflicts?**
   - Auto-branch prevents most conflicts
   - On conflict, pause and ask user

3. **Should the model know about auto-workflows?**
   - Yes: Add to system prompt ("Files are auto-committed, no need for git commands")
   - No: Let it discover organically

**Recommendation:** Add 1-2 sentences to identity:
```
Your workspace automatically commits changes and runs build/test after 
file edits. Focus on code quality—the tooling handles git and testing.
```
