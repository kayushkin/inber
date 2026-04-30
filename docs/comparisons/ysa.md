# YSA Comparison

**Project**: [ysa](https://github.com/ysa-ai/ysa)
**Language**: TypeScript (Bun) + shell + a Containerfile
**Focus**: Secure container runtime for AI coding agents — every agent task runs inside a hardened, rootless Podman sandbox with its own git worktree
**Key Strengths**: Defense-in-depth sandboxing (seccomp, cap-drop, read-only fs, rootless), shadow `git` wrapper that strips dangerous config, pre-push branch guard, MITM network proxy with firewall enforcement, auditable security test suite (155 attack tests + 60 proxy tests), local-first / no-cloud stance

## Architecture Overview

ysa positions itself as "a CLI and SDK, nothing else" — a primitive (`runTask()`) for spawning a coding agent inside a hardened sandbox, on which any orchestration layer can be built. The repo's CLAUDE.md reveals a richer stack than the README markets: a tRPC+Hono server, React 19 dashboard, Drizzle ORM over SQLite, Bun runtime — but the security story is the differentiator.

```
ysa run "prompt"
    → CLI (commander) creates task in SQLite (~/.ysa/core.db)
        → runtime/worktree.ts: cuts a git worktree under .ysa/worktrees/<task-id>/
        → runtime/container.ts: launches rootless Podman with:
              --cap-drop ALL --read-only --security-opt no-new-privileges
              --security-opt seccomp=container/seccomp.json (~190/400 syscalls)
              --tmpfs /tmp --memory 4g --cpus 2 --pids-limit 512
        → runtime/proxy.ts: optional MITM proxy + firewall (GET-only, rate-limited)
        → providers/{claude,mistral}.ts: agent adapter runs inside the container
              git is shadowed by container/git-safe-wrapper.sh
              pre-push hook (git-push-guard.sh) blocks pushes off the task branch
        → mise + apt provision per-project runtimes from .ysa.toml
```

The SDK shape: `import { runTask } from "@ysa-ai/ysa/runtime"`. Providers register through a small adapter interface (`ProviderAdapter` in `src/providers/types.ts`).

## What YSA Does Well

### 1. Defense-in-Depth Container Sandbox ⭐️

Every agent runs inside a rootless Podman container with multiple independent kernel-level constraints: all capabilities dropped, read-only root filesystem, `no-new-privileges`, a 190-syscall seccomp whitelist (blocks `clone3`, memfd tricks, namespace escalation paths), tmpfs scratch, hard memory/CPU/PID limits, and the daemon itself runs as an unprivileged user. This is "trust the kernel, not the agent" — the agent's runtime privileges are mathematically smaller than the user's.

**Inber connection**: Inber currently runs agents as host processes with full user privileges. Agents can read `~/.ssh`, push to remotes, exfiltrate env vars, fork bombs, etc. — the only safety net is tool gating in tool-store + the harness's tool list. ysa's model is the strongest available answer to "what if an agent is buggy or compromised?".

### 2. Shadow `git` Wrapper That Strips Dangerous Config ⭐️

`container/git-safe-wrapper.sh` shadows `/usr/bin/git` inside the container and strips 38+ config keys an agent could weaponize: `core.hooksPath`, `core.sshCommand`, `core.fsmonitor`, `filter.*`, `http.proxy`, `credential.helper`, `gpg.program`, etc. Even if the agent talks the LLM into running `git config --global …`, the wrapper rejects the dangerous keys before invoking the real binary.

**Inber connection**: Inber's `forge` package manages git worktrees but invokes git directly. Even without containerizing, a sanitized git wrapper would prevent a misbehaving agent from rewriting hooks, redirecting fetches through a malicious proxy, or hijacking ssh.

### 3. Pre-Push Branch Guard

`container/git-push-guard.sh` blocks every push that isn't going to the task's own branch. Agents can't push to `main`, can't push to other tasks' branches, can't force-push over someone else's work. Combined with the worktree-per-task layout, this contains the blast radius of "agent decided to commit and push something weird."

**Inber connection**: Inber relies on the agent following instructions to not push. A push guard would be cheap to add to forge.

### 4. Network Policy Enforced at Two Layers

Optional `--network strict` mode routes all outbound traffic through a local MITM proxy (`container/network-proxy.ts`): GET-only, no request body, rate-limited, byte-budgeted per task. The container's network namespace also has firewall rules so traffic that bypasses the proxy is dropped at the kernel level. Two independent enforcement points instead of one.

**Inber connection**: llm-bridge-server already proxies model API traffic, but tool-initiated egress (curl, npm install, git fetch) is unconstrained. A per-task network budget would catch exfiltration patterns and runaway downloads.

### 5. Auditable Security Test Suite

`container/tests/attack-test.sh` runs 155 attack-pattern tests across 38 categories (privilege escalation, fs escapes, hook injection, signal abuse, credential theft) — inside the actual container. `network-proxy-test.sh` runs 60 tests for proxy/firewall enforcement (exfiltration attempts, method bypasses, rule verification). The test scripts ship with the repo so users can re-run them on their own machine.

This is rare. Most "secure agent" frameworks publish a threat-model paragraph; ysa publishes the exploit attempts and the proof they fail.

**Inber connection**: Inber has no negative security tests — only happy-path tests for tool gating. Even without containerization, attack-pattern tests would surface gaps in tool-store's guard logic (e.g. "can the agent talk this CLI tool into reading `/etc/shadow`?").

### 6. mise-Backed Universal Toolchain

One container image plus mise (and a few apt fallbacks) provides Node, Python, Go, Rust, Ruby, PHP, Java/Maven, Java/Gradle, .NET, Elixir, C/C++. Per-project runtime config in `.ysa.toml` (`ysa runtime add node@22`) is materialized into the sandbox at task start. Reproducible toolchain without per-language images.

**Inber connection**: Inber agents inherit whatever the host has installed. Fine on a single dev machine, fragile across machines or CI. mise inside a sandbox would give reproducible per-project runtimes for free.

### 7. Local Sovereignty as a First-Class Feature

"No cloud, no telemetry, no data leaving your machine" is on the README, in the marketing, and in the code: config is local SQLite, no auth service, no analytics. The Apache-2.0 license and the explicitly transparent security layer (`seccomp.json`, `git-safe-wrapper.sh`, `sandbox-run.sh`, `network-proxy.ts` are all open and called out as "read them, audit them, don't trust what you can't verify") make it a credible choice for users with hard data-locality constraints.

**Inber connection**: Inber is also local-first but doesn't lean into it. Tool-store, model-store, agent-store, log-store all run locally. The story is there; it's just not framed.

## What Inber Should Adopt

### 1. Sanitized `git` Wrapper in forge (HIGH PRIORITY)

Smallest, highest-value lift: port `git-safe-wrapper.sh`'s config-key blacklist into a Go `gitexec` helper inside `forge`. Reject `--global`/`--system` writes to dangerous keys, reject `-c <dangerous-key>=…` overrides, reject env vars that reach the same surface (`GIT_SSH_COMMAND`, `GIT_PROXY_COMMAND`, `GIT_CONFIG_SYSTEM`). Costs nothing at runtime, blocks an entire class of agent misbehavior.

Concrete first step: forge wraps `exec.Command("git", …)` in a `forge.Git()` helper that scrubs args + env before invoking. All inber callers route through it.

### 2. Pre-Push Branch Guard for Worktrees (HIGH PRIORITY)

When forge creates a worktree, install a `pre-push` hook that compares the pushed ref against the slot's allowed branch. A `forge.Slot.AllowedRefs []string` field encoded into the hook makes the policy data-driven. Costs ~50 lines, prevents "agent pushed to main" forever.

### 3. Negative Security Test Suite (MEDIUM PRIORITY)

Add `tools/attack-test/` with patterns inber agents shouldn't be able to execute regardless of intent: read SSH keys, write to `~/.bashrc`, install a global git hook, exfiltrate env vars over DNS, push to a non-task branch, escalate via `sudo`. Run them against a real agent in CI. The test surface is a forcing function for tightening tool-store's guard rules.

### 4. Per-Task Network Budget (MEDIUM PRIORITY)

llm-bridge-server already sees all model traffic. Add per-session byte and request budgets, surfaced to the user as warnings/cutoffs. For tool-initiated egress, route HTTP-using tools (curl, fetch helpers) through a local proxy that participates in the same budget. Won't stop a determined exfiltrator without containerization, but catches the unintentional and the lazy.

### 5. Optional Container Mode for forge Slots (LOW / EXPLORATORY)

The honest assessment: containerizing inber agents is a large architectural shift (host bus, host SQLite stores, host filesystem, host secrets — none of which extend cleanly into a rootless Podman sandbox). But a container-mode flag on `forge.Slot` could give security-conscious users an opt-in: at slot creation, materialize the worktree into a Podman container, run the harness binary inside, expose the bus over a unix socket. Probably not worth doing unless real users hit a use case (e.g. running untrusted third-party agents).

### 6. Frame the Local-First Story (LOW PRIORITY, MARKETING)

Inber is already local-first; the README and ARCHITECTURE.md don't say so prominently. A README section explicitly stating "no cloud calls except the LLM provider you configure; all data, sessions, memories, and logs stay on your machine" would land with users who've burned themselves on telemetry-heavy frameworks.

## What's Different

| Aspect | YSA | Inber |
|--------|-----|-------|
| **Primary value prop** | Security boundary for agent execution | Multi-agent orchestration with persistent identity |
| **Isolation model** | Rootless Podman container per task | Host process, no isolation; trust via tool gating |
| **Git workflow** | Worktree + shadow git wrapper + push guard | Worktree (forge) + direct git invocation |
| **Network policy** | MITM proxy + firewall, GET-only strict mode | None; tools call out unconstrained |
| **Toolchain** | mise + apt inside one container image | Inherits host toolchain |
| **Multi-agent** | Single-task primitive, "build orchestration on top" | Engine + bus + 10 named agents, NATS JetStream |
| **Surface** | CLI + SDK (`runTask()`) + dashboard via tRPC | CLI + server + bus adapters + bridge-ui dash |
| **Provider model** | `ProviderAdapter` interface, registry | llm-bridge-* per provider/harness, more layers |
| **State** | SQLite (Drizzle) at `~/.ysa/core.db` | SQLite across agent-store, model-store, harness-store, memory-store, log-store |
| **License** | Apache 2.0 | Internal |
| **Threat model** | Explicit + tested (attack-test.sh) | Implicit; relies on tool gating |
| **Distribution** | `npm install -g @ysa-ai/ysa` | `inber` Go binary + ecosystem of services |
| **Platform** | macOS / Linux (Podman 5.x+) | Linux (systemd user units) |

## Key Takeaway

ysa and inber barely overlap in scope — ysa is a single-task secure runtime, inber is a multi-agent orchestration framework — but ysa solves the **security boundary problem** that inber currently sidesteps. Inber's bet is that tool gating + careful instructions are enough; ysa's bet is that the kernel should enforce it because the agent will eventually misbehave.

The actionable insights for inber are pragmatic and don't require swallowing the whole containerization model:

1. **Sanitized git wrapper in forge** — prevents an entire class of agent misbehavior in ~100 lines.
2. **Pre-push branch guard** — closes "agent pushed to main" permanently.
3. **Negative security test suite** — turns tool-store's guard rules from "we think this is safe" into "here's what we tested."
4. **Per-task network budget** — leverages llm-bridge-server's existing position as the model-traffic chokepoint.

The deeper question ysa raises: is "agent runs as host user with full privileges" a sustainable default, or a temporary one we're tolerating because containerizing the bus + stores is hard? Worth answering deliberately rather than by inertia.
