# Workspace Redesign — Ephemeral Multi-Repo Workspaces

## Overview

Replace the persistent slot/worktree system with ephemeral, on-demand workspaces.
Agent work stays on spawn branches until the orchestrator explicitly merges to main.

## Design Principles

- **Ephemeral**: worktrees created per-spawn, deleted after
- **Isolated**: agents never touch base repos or main branch directly  
- **Multi-repo**: one workspace can span multiple repos
- **Orchestrator-gated**: nothing merges to main without orchestrator approval
- **Auto-retry**: transient agent failures retry before escalating

## Architecture

### Ownership

| Action | Owner | Notes |
|--------|-------|-------|
| Create/destroy worktree | Forge | Workspace lifecycle |
| Assign workspace to agent | Gateway | Knows which agent is spawning |
| Set agent cwd | Gateway | Configures engine |
| Edit/build/test | Agent | Its job |
| Commit changes | Forge | Auto-commit on spawn branch |
| Deploy to staging | Forge | Branch → staging env |
| Merge to main | Forge | Triggered by orchestrator |
| Push to origin | Forge | Part of merge |
| Deploy prod | Forge | Post-merge |
| Cleanup | Forge | Remove worktree + branch |
| Orchestrate sequence | Gateway | Calls forge in order |
| Approve/reject merge | Orchestrator | Claxon decides what hits main |

### Forge API

```go
type Workspace struct {
    ID       string            // "brigid-1710512345"
    Repos    map[string]string // name → worktree path
    Primary  string            // primary repo name
    BaseDir  string            // ~/forge/work/brigid-1710512345/
    Branch   string            // spawn/brigid-1710512345
    Status   string            // created|working|done|staged|merged|rejected|expired
}

type CommitResult struct {
    Hash  string
    Dirty bool
}

type MergeResult struct {
    Status    string   // "ok", "conflict", "error"
    Conflicts []string // conflicting file paths
    Error     string
}

forge.CreateWorkspace(agent string, projects []string) (*Workspace, error)
forge.CommitAll(ws, message string) (map[string]CommitResult, error)
forge.DeployToStaging(ws, envSlot int) (map[string]error)
forge.MergeToMain(ws) (map[string]MergeResult)
forge.PushAll(ws) (map[string]error)
forge.Cleanup(ws) error
forge.ReopenWorkspace(ws) error  // for fix/retry in same workspace
```

### Concurrency

In-memory semaphore per project. Default max 3 concurrent workspaces.
No SQLite for slot tracking — worktree existence on disk is source of truth.
On restart, scan ~/forge/work/ to recover orphaned workspaces.

## Lifecycle

### Phase 1: SETUP (forge)

```
forge.CreateWorkspace("brigid", ["kayushkin", "si"])
```

Creates:
```
~/forge/work/brigid-<ts>/
  kayushkin/    ← git worktree on branch spawn/brigid-<ts>
  si/           ← git worktree on branch spawn/brigid-<ts>
```

Errors:
- Concurrency limit → reject or queue spawn
- Git worktree add fails → report, no spawn
- Disk space → report

### Phase 2: WORK (agent)

```
engine.RunTurn(task) — cwd = ws.Primary path
```

Auto-retry on transient errors (API timeout, rate limit, model hiccup):
- Up to 2 retries, same workspace, same session
- Non-retryable errors (config, context too long) → skip to Phase 3
- All retries exhausted → commit partial work, report to orchestrator

Agent rules:
- Uses relative paths only
- Doesn't know it's in a worktree
- Doesn't commit, push, deploy, or SSH (workspace rules in prompt)

### Phase 3: COMMIT (forge)

```
forge.CommitAll(ws, "brigid: <task summary>")
```

Per repo: if dirty → git add -A && commit on spawn branch.

Errors:
- Nothing to commit → clean status reported
- Git commit fails → per-repo error reported

Changes stay on spawn branch. Main is untouched.

### Phase 4: STAGE (forge)

```
forge.DeployToStaging(ws, envSlot)
```

Deploy spawn branch to staging env for verification.
Staging env worktree checks out the spawn branch directly.

Errors:
- No staging env available → skip, orchestrator reviews via diff
- Build fails → report
- SSH fails → report
- Services don't start → health check failure reported

### Phase 5: REVIEW (orchestrator)

Gateway delivers result to orchestrator:
```
[Spawn completed]
Agent: brigid
Task: add login button
Status: success
Repos changed: kayushkin (abc123), si (def456)
Staging: env-1 (http://1.dev.kayushkin.com)
Workspace: brigid-1710512345
Branch: spawn/brigid-1710512345

Available actions:
  merge(workspace_id)              → merge to main + push + deploy
  reject(workspace_id)             → discard + cleanup
  fix(workspace_id, instructions)  → re-spawn agent in same workspace
```

Orchestrator can:
1. **merge** — changes look good
2. **reject** — discard everything
3. **fix** — re-spawn agent in same workspace with new instructions
4. **ignore** — workspace stays alive (TTL expiry, default 1hr)

### Phase 6: MERGE (forge, on orchestrator approval)

```
forge.MergeToMain(ws)
```

Per repo:
1. In worktree: `git fetch origin main`
2. In worktree: `git rebase origin/main`
3. In base repo: `git merge spawn/brigid-<ts> --ff-only`
4. `git push origin main`

Errors:
- Rebase conflict → report conflicting files to orchestrator
  - Orchestrator can: spawn agent to resolve, reject, or escalate to human
- FF merge fails → report (shouldn't happen after rebase)
- Push rejected → fetch + retry once, then report
- Partial success (repo A ok, repo B conflict) → per-repo status
  - Orchestrator decides: rollback A or force-fix B

### Phase 7: CLEANUP (forge)

```
forge.Cleanup(ws)
```

1. `git worktree remove <path>` per repo
2. `git branch -D spawn/brigid-<ts>` per repo
3. Release concurrency semaphore
4. Remove workspace base dir

Errors:
- Worktree locked → force remove
- Branch delete fails → log warning, periodic GC

## Workspace States

```
CREATED  → worktrees exist, agent not started
WORKING  → agent is running
DONE     → agent finished, changes on spawn branch
STAGED   → deployed to staging env
MERGED   → merged to main, pushed
REJECTED → discarded by orchestrator
EXPIRED  → TTL hit, auto-cleaned
```

## Agent Config

```json
{
  "brigid": {
    "projects": ["kayushkin", "si"],
    "primary": "kayushkin"
  },
  "oisin": {
    "projects": ["si"],
    "primary": "si"
  }
}
```

Projects registered in forge:
```json
{
  "kayushkin": {
    "repo": "~/life/repos/kayushkin.com",
    "max_concurrent": 3
  },
  "si": {
    "repo": "~/life/repos/si",
    "max_concurrent": 2
  }
}
```

## What Gets Removed

### Forge (compat.go)
- `AcquireV2()` / `ReleaseV2()`
- `CommitSlotChanges()`
- `ResetSlotBranch()`
- `CleanSlot()` / `SlotPull()`
- `SlotStatus()`
- SQLite `slots` table
- SQLite `projects.base_repo` (replaced by new config)

### Gateway (spawn.go)
- All inline `exec.Command("git", ...)` calls
- `deploySlot()` method entirely
- Stash/pop logic
- Forge slot acquire/release calls
- `deploy-staging.sh` invocation

### Filesystem
- `~/forge/slots/` directory (persistent worktrees)
- `~/life/repos/.pools/` directory (agent-bench worktrees)
- Associated branches: `forge/slot-*`, `pool/*`

### Keep
- `~/life/repos/.envs/env-{0,1,2}/` — staging Docker Compose envs (separate concern)
- `forge-env restart` system (unchanged)
- `forge deploy prod` system (unchanged, called post-merge)
- Forge v3 staging slot system (OpenSlot/ForceCloseSlot — for Docker envs)
