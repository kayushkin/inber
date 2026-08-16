#!/usr/bin/env python3
"""Score inber's tests for the limit-check WIRING by breaking it.

The scoring engine — and the rules it enforces as refusals — lives in
scripts/sabotage.py. This file is only the case list: one edit per mechanism
the suite is meant to pin.

    python3 scripts/sabotage-limit-check-wiring.py [--diffs] [--crosstable]

⚠️ **This is the engine's TENTH copy, and it is the same blob as the other
nine.** md5 `9a81a32e5827b59c1a3093bf88187b17`. Diff before editing; an
eleventh blob is a fork. Take the blob off the BRANCH, not out of a working
tree — those checkouts sit on whatever branch their last pass left them on, and
md5summing them answers about the wrong commit (221st). inber had no scorer
before tonight.

Why this seam is worth a scorer, and why the coverage census pointed somewhere
else entirely:

`engine/limit_check_test.go` is thorough — fourteen cases over `buildLimitCheck`,
covering turns, tokens, response time, accumulation and the CLI/agent-config
precedence. Every one of them calls `buildLimitCheck` directly. Not one goes
through `configureAgent`, which is the only thing that installs the callback on
an agent, and it installs it behind a three-armed disjunction:

    if e.Limits.MaxTurns > 0 || e.Limits.MaxInputTokens > 0 || e.Limits.MaxResponseTime > 0 {
        a.SetLimitCheck(e.buildLimitCheck())
    }

Measured on `main`: a `panic()` on the first line of `agent.SetLimitCheck` left
`go test ./...` **green**. The callback was pinned and its installation was not.
That is a fail-open with a well-tested guard sitting behind it — drop an arm
from the disjunction and a session that asked for a response-time cap runs with
no cap at all, while all fourteen callback tests keep passing.

⚠️ **Each arm needs its own fixture, and the realistic fixture is the one that
cannot see the bug** (227th, inverted from conjunction to disjunction). A test
that sets all three limits at once satisfies the condition through `MaxTurns`
whatever the other two arms say, so it scores nothing when an arm is dropped.
The suite impoverishes each fixture until only the arm under test can carry it,
which is what makes cases 1-3 below distinguishable at all.

📄 The tool gate four lines above is the deliberate contrast and is pinned with
it: `SetToolRefusal` is called unconditionally, and `buildToolRefusal` returns
nil when there is no guard — so "who decides whether this session is gated" has
exactly one answer, and it is the guard, not `configureAgent`.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from sabotage import REPO, Case, score  # noqa: E402

TARGETS = [REPO / "engine" / "build.go"]
PACKAGES = ["./engine/"]

CONDITION = ("\tif e.Limits.MaxTurns > 0 || e.Limits.MaxInputTokens > 0 "
             "|| e.Limits.MaxResponseTime > 0 {")

CASES = [
    # ---- the three arms of the disjunction, one case each ----
    Case(
        "the disjunction loses its MaxTurns arm",
        [(CONDITION,
          "\tif e.Limits.MaxInputTokens > 0 || e.Limits.MaxResponseTime > 0 {")],
    ),
    Case(
        "the disjunction loses its MaxInputTokens arm",
        [(CONDITION,
          "\tif e.Limits.MaxTurns > 0 || e.Limits.MaxResponseTime > 0 {")],
    ),
    Case(
        "the disjunction loses its MaxResponseTime arm",
        [(CONDITION,
          "\tif e.Limits.MaxTurns > 0 || e.Limits.MaxInputTokens > 0 {")],
    ),

    # ---- the condition as a whole ----
    Case(
        "the condition is inverted, so only unlimited sessions get a limit check",
        [(CONDITION,
          "\tif !(e.Limits.MaxTurns > 0 || e.Limits.MaxInputTokens > 0 "
          "|| e.Limits.MaxResponseTime > 0) {")],
    ),
    Case(
        "the limit check is installed unconditionally",
        [(CONDITION, "\tif true {")],
    ),

    # ---- what gets installed, not merely that something does ----
    Case(
        "the installed check is built from a fresh engine carrying no limits",
        [("\t\ta.SetLimitCheck(e.buildLimitCheck())",
          "\t\ta.SetLimitCheck((&Engine{}).buildLimitCheck())")],
    ),

    # ---- the contrast: the tool gate must not learn about the limits ----
    Case(
        "the tool gate becomes conditional on the limits, like the check below it",
        [("\ta.SetToolRefusal(e.buildToolRefusal())",
          "\tif e.Limits.MaxTurns > 0 {\n\t\ta.SetToolRefusal(e.buildToolRefusal())\n\t}")],
    ),

    # ---- controls ----
    # Known-positive: nothing installs the callback at all. Every wiring
    # assertion in the suite reads this line, so a green run here means the
    # suite is not running.
    Case(
        "CONTROL known-positive: the limit check is never installed",
        [("\t\ta.SetLimitCheck(e.buildLimitCheck())",
          "\t\t_ = e.buildLimitCheck()")],
    ),
    # Known-negative: the two wiring calls swap order. A real edit that changes
    # the file's bytes and cannot change any outcome — the setters are
    # independent writes to two different fields on a freshly built agent.
    # Without a negative, a suite that reported CAUGHT for everything would
    # score perfectly here (33rd).
    Case(
        "CONTROL known-negative: the tool gate and the limit check swap order",
        [("\ta.SetToolRefusal(e.buildToolRefusal())\n"
          "\n"
          "\t// Wire up turn/token/time limit checks\n"
          + CONDITION + "\n"
          "\t\ta.SetLimitCheck(e.buildLimitCheck())\n"
          "\t}",
          "\t// Wire up turn/token/time limit checks\n"
          + CONDITION + "\n"
          "\t\ta.SetLimitCheck(e.buildLimitCheck())\n"
          "\t}\n"
          "\n"
          "\ta.SetToolRefusal(e.buildToolRefusal())")],
        expected_unnoticed=(
            "the two setters write different fields on a freshly built agent and neither reads "
            "the other, so the order between them is not observable from anywhere — including "
            "from the turn loop, which reads both only after configureAgent has returned"
        ),
    ),
]


if __name__ == "__main__":
    sys.exit(score(TARGETS, PACKAGES, CASES))
