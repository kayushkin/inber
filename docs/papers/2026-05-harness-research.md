# Harness Research — Early May 2026

Notes from the harness-watch sweep on 2026-05-01. Two arXiv papers from the
last 7-10 days that weren't in the April 29 sweep and bear directly on
inber's harness design. Upstream commit feeds for all eight tracked harness
repos failed this run, so this is a paper-only update.

## Agentic Harness Engineering: Observability-Driven Automatic Evolution of Coding-Agent Harnesses

[arXiv:2604.25850](https://arxiv.org/abs/2604.25850) — submitted 2026-04-28,
revised 2026-04-30

Frames harness engineering as an automatable feedback loop instead of manual
craft. Three observability pillars: **component observability** (each
harness component is file-level editable and reversible), **experience
observability** (raw trajectory data is digested into evidence the agent can
learn from), **decision observability** (every modification is bound to a
testable prediction verified on the next iteration). Drives pass@1 from
69.7% → 77.0% on Terminal-Bench 2 over ten iterations and beats both
hand-tuned and prior self-evolving baselines. The ablation is the actually
interesting bit: **gains come from tools, middleware, and memory edits, not
prose-level prompt rewriting** — and the evolved components transfer across
models, suggesting they encode general engineering experience rather than
benchmark overfitting.

**What inber should consider:** Inber's harness components are already
file-level (engine/, tools/, memory/, registry/, guard/), so the
"component observability" precondition is largely met. The missing piece is
**experience observability** — a structured way to roll up trajectory traces
from `trace/` and `logs/` into evidence usable by an evolution loop. Worth
sketching whether the existing trace export could feed a simple per-component
diff/ablation harness, even if the actual evolution is human-in-the-loop for
now. The tools/middleware/memory ablation also reinforces a finding from the
April Zup paper: when iterating on agent quality, prefer changing the harness
surface over tweaking the prompt.

## Synthesizing Multi-Agent Harnesses for Vulnerability Discovery (AgentFlow)

[arXiv:2604.20801](https://arxiv.org/abs/2604.20801) — submitted 2026-04-22

A typed graph DSL whose search space jointly covers **agent roles, prompts,
tools, communication topology, and coordination protocol** — i.e., the entire
multi-agent harness layout, not just the prompt or tool list. Paired with a
feedback-driven outer loop that reads runtime signals from the target
program itself to diagnose which part of the harness caused a failure and
rewrite that part. Reaches 84.3% on TerminalBench-2 and discovers 10
zero-day Chrome vulnerabilities (incl. two Critical sandbox-escapes) — the
first paper I've seen with explicit CVE numbers attributed to a
harness-search system. Companion thesis to 2604.25850: when the model is
fixed, harness search alone moves end-to-end success several-fold.

**What inber should consider:** Inber's multi-agent surface
(agents.openclaw-example.json, the registry, the bus topology) is currently
configured by hand. AgentFlow's typed-graph framing is closer to what inber
already has than the unstructured natural-language harnesses in 2603.25723,
so it's a more realistic reference if/when inber considers programmatic
multi-agent topology generation. Lower-effort takeaway: the failure-attribution
loop. Inber's `bus` already carries enough run-level signal to attribute a
failed sub-task to a specific agent role or tool — surfacing that
attribution on the inber-party dashboard would be a small win even without
automated rewriting.

## Cross-cutting takeaway

The April-30 corpus sharpens a thesis the April-29 sweep already noted:
**the harness, not the model, is the unit of progress, and harnesses are
becoming search targets, not artifacts.** Both 2604.25850 and 2604.20801
beat hand-tuned baselines by treating the harness as a typed, observable,
mutable object. Inber's component layout is already amenable to this — the
gap is in the trajectory/feedback plumbing required to close the loop. Worth
revisiting once `trace/` exports stabilize.
