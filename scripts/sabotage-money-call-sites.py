#!/usr/bin/env python3
"""Score the call sites that price money in inber.

The helper pair in session/timeline_cost.go -- CalcCost and CalcCostWithCache --
is thoroughly tested in session/timeline_cost_test.go. This script asks the
different question that card 3388f950 is about: does anything notice when a
CALLER stops asking them correctly?

Each mutation below is one of the two defects the helpers' own doc comments
record as having actually shipped:

  nil-store   the caller passes nil instead of the model registry, so every
              model is priced at the unknown-model flat rate of $3/$15 per
              million. The comment records a Haiku sub-agent billed at twelve
              times its registered price and an Opus one at a fifth of its own.

  no-cache    the caller drops the cache adjustment by calling CalcCost instead
              of CalcCostWithCache, so cache reads are billed at the full input
              rate and cache writes are not billed at all.

Both leave the helpers untouched and both compile. A mutation that no test
reddens is a call site nothing holds.

Two controls run alongside the mutations, because an instrument that cannot say
"no" is not evidence:

  CONTROL-CRYWOLF   nothing mutated. Must stay GREEN, or the tree was already
                    red and every CAUGHT below is a lie.
  CONTROL-POSITIVE  the helper itself is mutated (the 1.25 cache-write factor
                    becomes 1.0). Must go RED, or the suite cannot see a
                    pricing change at all and no SURVIVED result means anything.

The case list has moved once since it was first scored, and a score is only
comparable to another score taken on the same list:

  2026-08-17  M13 added. session/session_logging.go was carrying a nil-registry
              row (M9) and no cache-drop row, though both defects are reachable
              at that one call site and the second is the more expensive of the
              two. Every before/after pair recorded after this date is over 13
              rows; the 2/12 and 5/12 figures in card d75191e6 are over 12 and
              cannot be compared to them directly.

  2026-08-17  M14 added, for the same omission one call site along. The
              failed-turn path in server/server.go carried a nil-registry row
              (M6) and no cache-drop row. Scores over 14 rows are not comparable
              to the 13-row pair either; the pair recorded with this change was
              re-taken on the 14-row list.

Two rows are known unscoreable through the SPAWN path, and the reason is worth
reading before anyone tries again — it is not a weak assertion, it is a fixture
that cannot be built without changing shipping code:

  M4, M5      A spawn's model is NOT the one its caller asked for.
              SpawnRequest.Model reaches the child only as AgentConfig.Model,
              and Engine.initAgent overwrites that with the agent's model as
              registered in agent-store unless EngineConfig.ModelExplicitlySet
              is set — which only applyRequestOverrides sets, and Spawn passes a
              ZERO RunRequest. Measured 2026-08-17: asking a spawn for
              "a-model-priced-either-side-of-the-fallback" yields a child engine
              running "claude-sonnet-4-5"; the same model asked for through
              RunRequest.Model arrives intact.

              So a spawn fixture cannot choose its model, and the model it gets
              is priced at exactly the $3.00/$15.00 unknown-model fallback — on
              which a nil registry and a live one compute the IDENTICAL figure.
              Both mutations would be unobservable even if the turn ran.

              ⛔ And driving a real spawn is not free. Measured the same day:
              the child selected an OpenAI model at turn time and sent a live
              request to api.openai.com on this host's stored credentials.
              ANTHROPIC_BASE_URL does not redirect that path — agent/clients.go
              takes the OpenAI base URL from a hardcoded constant with no
              environment override. A spawn fixture that assumes the Anthropic
              seam sends real traffic and cannot tell that it did.

Usage:  python3 scripts/sabotage-money-call-sites.py [--only M3]
"""

import argparse
import pathlib
import subprocess
import sys

REPO = pathlib.Path(__file__).resolve().parent.parent
PACKAGES = ["./session/...", "./server/...", "./engine/..."]


class Mutation:
    def __init__(self, name, path, old, new, note):
        self.name = name
        self.path = path
        self.old = old
        self.new = new
        self.note = note


# --- the call sites ---------------------------------------------------------
#
# Every anchor below was checked for uniqueness inside its file before being
# used. A non-unique anchor replaces the wrong line, or replaces two, and the
# row then reports something other than the site it names.

MUTATIONS = [
    Mutation(
        "M1", "engine/turn_postprocess.go",
        "result.CacheReadTokens, result.CacheCreationTokens, e.modelStore)\n\te.Tokens.Cost += turnCost",
        "result.CacheReadTokens, result.CacheCreationTokens, nil)\n\te.Tokens.Cost += turnCost",
        "recordTurnUsage: turn charge and the MaxCost guard priced with no registry",
    ),
    Mutation(
        "M2", "engine/turn_postprocess.go",
        "turnCost := sessionMod.CalcCostWithCache(e.Model, result.InputTokens, result.OutputTokens,\n"
        "\t\tresult.CacheReadTokens, result.CacheCreationTokens, e.modelStore)",
        "turnCost := sessionMod.CalcCost(e.Model, result.InputTokens, result.OutputTokens, e.modelStore)",
        "recordTurnUsage: cache adjustment dropped from the turn charge",
    ),
    Mutation(
        "M3", "engine/turn_postprocess.go",
        "call.CacheReadTokens, call.CacheCreationTokens, e.modelStore)",
        "call.CacheReadTokens, call.CacheCreationTokens, nil)",
        "reportCallsThatBoughtNoCache: the no-cache report priced with no registry",
    ),
    Mutation(
        "M4", "server/spawn.go",
        "tokens.CacheRead, tokens.CacheWrite, g.modelStore)",
        "tokens.CacheRead, tokens.CacheWrite, nil)",
        "spawn: the child request row priced with no registry",
    ),
    Mutation(
        "M5", "server/spawn.go",
        "tokens.Cost = sessionMod.CalcCostWithCache(child.Engine.Model, tokens.Input, tokens.Output,\n"
        "\t\t\t\ttokens.CacheRead, tokens.CacheWrite, g.modelStore)",
        "tokens.Cost = sessionMod.CalcCost(child.Engine.Model, tokens.Input, tokens.Output, g.modelStore)",
        "spawn: cache adjustment dropped from the child request row",
    ),
    Mutation(
        "M6", "server/server.go",
        "result.CacheReadTokens, result.CacheCreationTokens, g.modelStore)",
        "result.CacheReadTokens, result.CacheCreationTokens, nil)",
        "failed-turn path: the error request row priced with no registry",
    ),
    Mutation(
        "M14", "server/server.go",
        "cost := sessionMod.CalcCostWithCache(sess.Engine.Model, result.InputTokens, result.OutputTokens,\n"
        "\t\t\t\t\tresult.CacheReadTokens, result.CacheCreationTokens, g.modelStore)",
        "cost := sessionMod.CalcCost(sess.Engine.Model, result.InputTokens, result.OutputTokens, g.modelStore)",
        "failed-turn path: cache adjustment dropped from the error request row",
    ),
    Mutation(
        "M7", "server/server.go",
        "tokens.CacheRead, tokens.CacheWrite, g.modelStore)",
        "tokens.CacheRead, tokens.CacheWrite, nil)",
        "completed-turn path: TokenUsage.Cost, the bridge's TotalUSD, priced with no registry",
    ),
    Mutation(
        "M8", "server/server.go",
        "tokens.Cost = sessionMod.CalcCostWithCache(sess.Engine.Model, tokens.Input, tokens.Output,\n"
        "\t\t\ttokens.CacheRead, tokens.CacheWrite, g.modelStore)",
        "tokens.Cost = sessionMod.CalcCost(sess.Engine.Model, tokens.Input, tokens.Output, g.modelStore)",
        "completed-turn path: cache adjustment dropped from the bridge's TotalUSD",
    ),
    Mutation(
        "M9", "session/session_logging.go",
        "tokens.CacheRead, tokens.CacheWrite, s.modelStore)",
        "tokens.CacheRead, tokens.CacheWrite, nil)",
        "session log: the per-turn logged cost priced with no registry",
    ),
    Mutation(
        "M13", "session/session_logging.go",
        "cost := CalcCostWithCache(s.model, tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite, s.modelStore)",
        "cost := CalcCost(s.model, tokens.Input, tokens.Output, s.modelStore)",
        "session log: cache adjustment dropped from the per-turn logged cost",
    ),
    Mutation(
        "M10", "session/timeline_jsonl.go",
        "entry.CacheRead, entry.CacheWrite, store)",
        "entry.CacheRead, entry.CacheWrite, nil)",
        "timeline read: every replayed request priced with no registry",
    ),
    Mutation(
        "M11", "session/session.go",
        "return CalcCostWithCache(s.model, s.totalIn, s.totalOut, s.totalCacheRead, s.totalCacheWrite, s.modelStore)",
        "return CalcCostWithCache(s.model, s.totalIn, s.totalOut, s.totalCacheRead, s.totalCacheWrite, nil)",
        "Session.Cost: the session total priced with no registry",
    ),
    Mutation(
        "M12", "engine/display_stats.go",
        "cost := session.CalcCost(model, result.InputTokens, result.OutputTokens, store)",
        "cost := session.CalcCost(model, result.InputTokens, result.OutputTokens, nil)",
        "DisplayStats: the printed turn cost priced with no registry",
    ),
]

CONTROL_POSITIVE = Mutation(
    "CONTROL-POSITIVE", "session/timeline_cost.go",
    "cacheWriteCost := float64(cacheWrite) * info.InputCostPer1M * 1.25",
    "cacheWriteCost := float64(cacheWrite) * info.InputCostPer1M * 1.0",
    "the helper's own cache-write factor -- MUST redden, or the suite is blind to pricing",
)


def run_suite():
    """Return (ok, first_failing_lines). ok is True when the suite is green."""
    proc = subprocess.run(
        ["go", "test", "-count=1"] + PACKAGES,
        cwd=REPO, capture_output=True, text=True,
    )
    return proc.returncode == 0, proc.stdout + proc.stderr


def apply(mut):
    path = REPO / mut.path
    text = path.read_text()
    count = text.count(mut.old)
    if count != 1:
        raise SystemExit(
            f"{mut.name}: anchor occurs {count} times in {mut.path}, expected exactly 1. "
            "A non-unique anchor scores a line other than the one it names."
        )
    path.write_text(text.replace(mut.old, mut.new))
    return text


def restore(mut, original):
    (REPO / mut.path).write_text(original)


def score(mut):
    original = apply(mut)
    try:
        ok, output = run_suite()
    finally:
        restore(mut, original)
    return ok, output


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--only", help="run one row by name, e.g. M3")
    args = ap.parse_args()

    dirty = subprocess.run(["git", "status", "--porcelain"], cwd=REPO,
                           capture_output=True, text=True).stdout.strip()
    if dirty:
        raise SystemExit(
            "working tree is dirty; this harness restores from its own in-memory copy "
            "but a crash would leave a mutation behind. Commit first.\n" + dirty
        )

    print("CONTROL-CRYWOLF  (nothing mutated)")
    ok, output = run_suite()
    if not ok:
        print(output)
        raise SystemExit("baseline is RED -- every CAUGHT below would be meaningless. Stop.")
    print("  GREEN as required\n")

    print(f"{CONTROL_POSITIVE.name}  {CONTROL_POSITIVE.note}")
    ok, _ = score(CONTROL_POSITIVE)
    print(f"  {'GREEN -- INSTRUMENT IS BLIND, stop' if ok else 'RED as required'}\n")
    if ok:
        raise SystemExit("the suite cannot see a pricing change; no SURVIVED result would mean anything.")

    rows = MUTATIONS if not args.only else [m for m in MUTATIONS if m.name == args.only]
    caught = 0
    for mut in rows:
        ok, output = score(mut)
        verdict = "SURVIVED" if ok else "CAUGHT"
        caught += 0 if ok else 1
        print(f"{mut.name:5s} {verdict:9s} {mut.note}")
        if not ok:
            for line in output.splitlines():
                if line.startswith("--- FAIL") or line.startswith("    --- FAIL"):
                    print(f"        {line.strip()}")
    print(f"\n{caught} of {len(rows)} caught")


if __name__ == "__main__":
    main()
