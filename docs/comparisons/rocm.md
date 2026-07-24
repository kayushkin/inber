# ROCm Comparison

**Project**: [ROCm](https://github.com/ROCm) (ROCR-Runtime, HIP, ROCK-Kernel-Driver, RCCL)
**Language**: C / C++ (kernel driver, HSA runtime, HIP), open source
**Focus**: Not an agent framework — AMD's open GPU compute stack. Mined here for one thing: how it lets many independent execution agents (CPU + GPUs, across process boundaries) share large data *without copying it* and coordinate *without polling*.
**Key Strengths**: Zero-copy handle-passing (IPC memory handles, dma-buf), an explicit coherence-mode knob (coarse vs fine grained), signal/doorbell readiness primitives, and topology-aware fan-in/fan-out collectives (RCCL).

## Why compare a GPU stack to a harness

Inber's open question in `TEAM-ORCHESTRATION.md` §10 is "where does the durable shared artifact between subagents physically live, and how does a consumer get it without the producer copying it into a prompt?" ROCm has solved the isomorphic problem for 15 years at the hardware layer: N agents, one physical blob, no copies. The vocabulary transfers cleanly even though the substrate does not.

The mapping we care about:

```
ROCm (hardware agents)              Inber (LLM subagents)
─────────────────────────           ──────────────────────────
device allocation (a blob)     →    a produced artifact (findings, a file map, a plan)
IPC memory handle              →    a reference/pointer a subagent opens instead of a copy
IPC event handle / HSA signal  →    "artifact ready" / "I'm done" without polling the board
coarse vs fine grained SVM     →    commit-on-exit vs live-shared working memory
AQL queue + doorbell           →    write a coordination card + wake the consumer (fast path)
barrier-AND / barrier-OR       →    "run when all/any of these subagents finish"
RCCL collectives               →    broadcast one context to N children; gather N results
```

## Architecture Overview

ROCm is a layered stack. From the bottom: the `amdgpu`/KFD kernel driver owns physical GPU memory and exposes it; the ROCr (HSA) runtime gives userspace queues, signals, and a shared virtual address space; HIP is the portable C++ API on top; RCCL sits beside HIP for multi-GPU collectives.

```
Process A                          Process B
  hipMalloc → device blob            hipIpcOpenMemHandle(handle)
  hipIpcGetMemHandle → opaque handle       ↓ maps the SAME physical pages
        │  (copy the ~64-byte handle, not the blob, over any channel)
        └───────────────────────────→ device pointer into A's allocation
  hipIpcGetEventHandle ─── event ───→ hipIpcOpenEventHandle
        (A records; B waits — cross-process readiness, no polling)

ROCr runtime below both:  AQL ring-buffer queue → doorbell → agent consumes
                          hsa_signal_t: wait(EQ/LT/GTE, value)  ← completion
                          barrier-AND / barrier-OR packets ← dependency chains
```

The load-bearing idea, repeated at every layer: **an agent never receives a buffer, it receives a handle to a buffer.** Copies happen only when an agent explicitly asks for one. Readiness is a signal you wait on, never a value you re-read in a loop.

## What ROCm Does Well

### 1. IPC memory handles — a blob is shared by reference, never by copy ⭐️

`hipIpcGetMemHandle(ptr)` turns a device allocation into a small opaque handle. Any other process calls `hipIpcOpenMemHandle(handle)` and gets a pointer into *the same physical memory* — no copy, no serialization. The handle is cheap to produce, can be exported many times, and the imported mapping is released with `hipIpcCloseMemHandle`. Freeing the source allocation invalidates every open handle (fail-loud lifetime, not silent staleness).

The discipline this enforces: the producer owns the blob, everyone else holds a revocable reference, and the thing that crosses the process boundary is tiny.

**Inber connection**: This is the missing primitive. Today a child returns a `SpawnResult{Summary, ...}` that `deliverResult()` *injects into the parent's conversation as text* (`server/spawn.go`) — a copy, into the most expensive place a copy can land (the context window). memory-store (`:8160`) is retrieval-by-similarity, not a handle you hand to a specific peer. There is no "here is a reference to the 40KB file-map I built; open it if you need it" primitive. TEAM-ORCHESTRATION §3.3's distillation rule ("return a pointer-rich index, not prose") is *reaching for exactly this* but leaves the pointed-to artifact's home undefined (§10). IPC handles are the answer: a subagent registers an artifact, gets back an opaque ref, and returns the ref — the consumer opens it on demand.

### 2. IPC event handles — cross-process readiness without polling ⭐️

`hipIpcGetEventHandle` / `hipIpcOpenEventHandle` share a synchronization event across processes (requires the `hipEventInterprocess` + `hipEventDisableTiming` flags). Either side can `record`, `wait`, or `query`. One process signals "ready"; the other blocks until then. No busy loop, no fixed-interval tick.

**Inber connection**: The blackboard's fast path is inert. TEAM-ORCHESTRATION admits kanban-store (`:8305`) has *no event emit*, so hand-offs move at the 5-minute cron tick (scoper→dispatcher→curator loop). A subagent that finishes in 20 seconds waits up to 5 minutes for anyone to notice. An event/readiness primitive — a card-ready signal that wakes the consumer session via `harness.Manager` message injection — is the exact analogue, and it's the unbuilt "coordination engine" in `internal/orchestrator/`.

### 3. Explicit coherence mode — coarse vs fine grained ⭐️

ROCm makes you choose *when a write becomes visible*. **Coarse-grained** (default `hipMalloc`): stores are only guaranteed visible at kernel completion — cheap, no cross-agent coherence traffic. **Fine-grained** (`hipExtMallocWithFlags(hipDeviceMallocFinegrained)`): system-visible immediately, supports CPU/GPU atomics — coherent but costlier. SVM + HMM keep the host page tables in sync so the shared address space stays unified.

The insight is that coherence is a *policy per allocation*, not a global mode. Most data is coarse (publish once, at the end). A small amount is fine (a live shared counter, a claim flag).

**Inber connection**: Inber has an implicit, all-or-nothing version of this. A child's work is coarse-grained by construction — nothing merges to main until the orchestrator approves the per-spawn forge worktree branch (`docs/workspace-redesign.md`), so writes "become visible" only at completion. The noteboard `workspace` is coarse too (read-first, rewrite-last). But there is no fine-grained option — no live shared region two peers can both see mid-flight — and TEAM-ORCHESTRATION §3/§13 treats "≥2 write-active sessions in one subtree" as an *invariant violation*, i.e. it bans the fine-grained case entirely. That's a defensible default, but naming it as a coherence *mode* (coarse = default, fine = opt-in with a claim protocol) is cleaner than treating concurrent writers as always-a-bug.

### 4. Signals and doorbells — the coordination substrate

An `hsa_signal_t` is a value an agent waits on with a condition (`EQ`/`LT`/`GTE`) — "block until this reaches N." A producer writes AQL packets into a ring-buffer queue and rings a **doorbell** (`AMD_SIGNAL_KIND_DOORBELL`) to wake the consumer; the consumer tracks progress with acquire/release atomics. Completion updates a signal, which unblocks whatever was waiting. This is the whole coordination model: publish work, wake the reader, signal done.

**Inber connection**: Maps onto the `bridge kanban post/claim/list` verbs sketched in TOOL-ROUTING/CLI-SURFACE. The "doorbell" is the missing message-injection into a teammate session. The "signal wait" is a subagent parking until a dependency's card flips to done. Inber's `msg.Event` is flat per session+turn with no cross-session trace plumbing (TEAM-ORCHESTRATION §14), so there is currently nothing to wait *on* across agents — signals are the primitive that would make dependency-driven coordination possible instead of poll-driven.

### 5. Barrier-AND / barrier-OR packets — declarative dependencies

A queue can carry barrier packets that say "don't proceed until *all* of these signals fire" (AND) or "until *any one* fires" (OR). Dependencies are data the runtime enforces, not control flow the submitter hand-serializes.

**Inber connection**: Today the parent serializes children by hand — spawn, await result, spawn the next. A barrier-AND is "run the synthesis subagent when all N finder subagents finish"; a barrier-OR is "proceed as soon as any verifier confirms." Expressing fan-in declaratively (a card that lists its blocker card-ids and auto-fires when they clear) beats the parent polling `parent.Children` and threading results manually through `deliverResult()`.

### 6. RCCL — topology-aware fan-in / fan-out

RCCL implements all-reduce, all-gather, broadcast, reduce-scatter, gather, scatter, and point-to-point over ring/tree algorithms, choosing routes from the interconnect topology (xGMI, PCIe, InfiniBand). One call broadcasts a buffer to N GPUs or gathers N results into one.

**Inber connection**: The shapes recur in multi-agent work: **broadcast** one shared context to N children (today each fork copies the whole parent history — `forkSession`); **gather** N childrens' distilled indices into one synthesis. Inber doesn't need ring algorithms, but the *named collective operations* are a good vocabulary for the spawn API — `Broadcast(context, children)` and `Gather(children) → merged` are clearer contracts than N independent `Spawn` + N `deliverResult` callbacks.

## What Inber Should Adopt

### 1. An artifact-handle store: return references, not payloads (HIGH PRIORITY)

The single highest-value idea. Give subagents a way to publish a blob once and return a cheap handle, so the consumer opens it on demand instead of receiving a copy in-context. This is the concrete answer to TEAM-ORCHESTRATION §10 ("where does the artifact live?") and makes the §3.3 distillation rule enforceable.

```go
// A subagent publishes its work and returns a handle, not the bytes.
type ArtifactHandle struct {
    ID       string    // opaque ref crossing the session boundary
    Kind     string    // "file-map" | "findings" | "plan" | "transcript"
    Producer string    // spawner_id that owns lifetime
    Ref      string    // where the bytes actually live (noteboard note id / repo path@sha / board card)
    Bytes    int
    Coherence string   // "coarse" (published at exit) | "fine" (live, claim-guarded)
}

// In SpawnResult, alongside Summary — the pointer-rich index becomes real refs.
type SpawnResult struct {
    // ...existing...
    Artifacts []ArtifactHandle // consumer calls OpenArtifact(id) only if it needs the detail
}
```

Insertion: `server/spawn.go` (`SpawnResult`, `deliverResult` — deliver handles, not prose) + a thin store. Reuse what exists for `Ref`: noteboard notes are already the durable, reversible substrate; memory-store already has `RefType`/`RefTarget` lazy pointers. The new part is the *handle contract*, not new storage.

### 2. A readiness/event channel on kanban-store (HIGH PRIORITY)

Add event emit to kanban-store (`:8305`) and a card-ready → session-wake path, so hand-offs move at seconds, not the 5-minute cron tick. This is the "doorbell + IPC event handle" analogue and unblocks the already-designed `internal/orchestrator/` coordination engine.

```go
// kanban-store emits on card state change; orchestrator injects into the claiming session.
func (o *Orchestrator) onCardReady(card Card) {
    for _, sess := range o.waitersOn(card.ID) {      // signal wait()
        o.harness.InjectMessage(sess, handoffPrompt(card))  // doorbell
    }
}
```

Insertion: kanban-store event bus (it has none today) + `internal/orchestrator/` in llm-bridge-server. Depends on the `bridge` CLI (P5b) and spawn message-injection per IMPLEMENTATION-ROADMAP.

### 3. Name the coherence mode explicitly (MEDIUM PRIORITY)

Make coarse-vs-fine a labelled property of shared state rather than an unspoken rule. Default every artifact to **coarse** (published at subagent exit — matches today's forge-branch merge gate and workspace read-first/rewrite-last). Allow **fine** only behind the existing first-writer-wins card-claim protocol (TEAM-ORCHESTRATION §6), for the rare live-shared case (a shared counter, a progress ledger). This turns "concurrent writers are always an invariant violation" (§3/§13) into "concurrent writers require a fine-grained claim" — same safety, one more supported pattern.

Insertion: the `Coherence` field in §1's `ArtifactHandle`; enforce the claim in the kanban-store `claimed`-tag path that already exists.

### 4. Declarative fan-in via barrier cards (MEDIUM PRIORITY)

Let a coordination card declare its blocker card-ids and auto-fire (barrier-AND) or fire-on-first (barrier-OR) instead of the parent hand-serializing children. Pairs naturally with #2's event channel — the synthesis subagent is just a waiter on an AND-barrier over the finder cards.

Insertion: a `barrier` entity_tag + blocker-id list on coordination cards (extends the `request`/`blocker`/`handoff` types in TEAM-ORCHESTRATION §6); the dispatcher fires when the set clears.

### 5. Collective-shaped spawn verbs (LOW PRIORITY)

Add `Broadcast(context, children)` and `Gather(children) → merged` as first-class spawn operations, borrowing RCCL's vocabulary. Cheaper than it sounds: `Broadcast` is a shared coarse artifact (one handle, N openers — avoids `forkSession` copying full history into every child); `Gather` is a synthesis subagent fed N artifact handles. Naming the collective makes the fan-out/fan-in contract legible in the API.

Insertion: `server/spawn.go` spawn API surface, layered on #1's handles.

## What's Different

- **ROCm shares mutable memory; inber deliberately doesn't.** ROCm's whole point is many agents reading and writing one coherent region concurrently. Inber's design stance (TEAM-ORCHESTRATION §3/§13) is the opposite: a *single* write-active session per subtree, fan-out only for reads/verification. The right borrow is the *handle-passing and readiness* machinery, not the concurrent-writer coherence — adopt coarse-grained-by-default and keep fine-grained rare and claim-guarded.

- **Hardware coherence is free; distributed artifact coherence is not.** ROCm leans on hardware cache-coherence and HMM page-table sync. Inber's "shared memory" is noteboard rows, repo branches, and card claims across processes and (eventually) hosts — there is no coherence fabric underneath, so every borrowed primitive has to be built as an explicit protocol, not assumed.

- **Copies are the expensive thing in both, but for opposite reasons.** ROCm avoids copies to save PCIe/xGMI bandwidth. Inber avoids copies to save *context-window tokens and cache-prefix stability* (CACHE-RULES.md: dynamic data must not enter the cached prefix). Same "pass a handle, not the payload" conclusion, different cost being minimized — which is why the handle idea (#1) matters even more here than it does on a GPU.

- **ROCm's agents are homogeneous and trusted; inber's are LLM sessions.** A doorbell wakes a known kernel; a card-ready event wakes a session that might mis-read the artifact. The readiness primitive (#2) transfers, but inber needs the artifact contract (Kind, schema) to be self-describing in a way a hardware queue never needs.

---

*Source research (2026-07-24): ROCm HIP IPC memory/event handles, ROCR-Runtime HSA signals + AQL queues + doorbell, SVM/HMM coarse-vs-fine coherence, dma-buf export/import, RCCL collectives. Compared against inber `server/spawn.go`, memory-store `:8160`, kanban-store `:8305`, noteboard `workspace` type, and llm-bridge-server `TEAM-ORCHESTRATION.md`. Starting point for discussion, not a committed roadmap.*
