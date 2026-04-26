#!/usr/bin/env bash
# harness-watch.sh
#
# Periodic scan of upstream coding-agent harnesses + recent agent/harness
# research papers. Runs Claude Code in the inber repo to evaluate findings
# and document anything worth importing into inber/docs/comparisons/.
#
# Triggered by the scheduler service (weekly). Safe to run manually too:
#   bash ~/repos/inber/scripts/harness-watch.sh

set -euo pipefail

REPO_DIR="${REPO_DIR:-$HOME/repos/inber}"
LOOKBACK_DAYS="${LOOKBACK_DAYS:-7}"
SINCE="$(date -u -d "${LOOKBACK_DAYS} days ago" +%Y-%m-%dT%H:%M:%SZ)"

# Curated upstream harnesses we track. Each entry maps a comparison doc
# under docs/comparisons/ to its canonical GitHub repo. Update both sides
# when adding a new harness.
HARNESSES=(
  "claude-code|anthropics/claude-code"
  "codex|openai/codex"
  "cline|cline/cline"
  "aider|Aider-AI/aider"
  "goose|block/goose"
  "roo-code|RooCodeInc/Roo-Code"
  "opencode|sst/opencode"
  "dexto|truffle-ai/dexto"
)

cd "$REPO_DIR"

# Make sure local main is current before Claude starts editing.
git fetch origin --quiet
git checkout main --quiet
git pull --ff-only --quiet

COMMITS_FILE="$(mktemp)"
trap 'rm -f "$COMMITS_FILE"' EXIT

{
  echo "# Upstream harness commits since ${SINCE}"
  echo
  for entry in "${HARNESSES[@]}"; do
    slug="${entry%%|*}"
    repo="${entry#*|}"
    echo "## ${slug} (${repo})"
    echo
    # gh api can fail (rate limit, repo rename, network); record the failure
    # in-line rather than aborting — the rest of the report is still useful.
    if ! gh api -X GET "repos/${repo}/commits" \
          -f since="${SINCE}" -f per_page=50 \
          --jq '.[] | "- " + (.sha[0:8]) + " " + (.commit.author.date) + " " + (.commit.message | split("\n")[0])' \
          2>/dev/null; then
      echo "_(failed to fetch commits for ${repo})_"
    fi
    echo
  done
} > "$COMMITS_FILE"

PROMPT="$(cat <<EOF
You are running as a scheduled harness-watch job inside the inber repo.

Two tasks:

1. Review the upstream harness commits below from the last ${LOOKBACK_DAYS} days.
   For each harness, decide whether anything noteworthy landed (new tool
   contract, prompt-cache trick, context strategy, permission model change,
   subagent design, novel UX, etc). Boring refactors / dep bumps / version
   bumps don't count. Compare against what is already in
   docs/comparisons/<slug>.md before deciding it is "new to inber".

2. Search the web for new papers (last ${LOOKBACK_DAYS}-30 days) on coding
   agent harnesses, tool use, context management, prompt caching,
   multi-agent orchestration, agent memory, or related topics that could
   improve inber. arXiv, HuggingFace blog, lab blogs (Anthropic, DeepMind,
   Meta AI), top-tier conference proceedings are all fair game.

If — and only if — you find something genuinely useful for inber:
  - Update the relevant docs/comparisons/<slug>.md with a dated section
    summarizing the change and its implication for inber, OR
  - Add a new note under docs/ (e.g. docs/papers/<slug>.md) for paper
    findings, OR
  - Append to docs/comparisons/agentic-design-patterns.md if it is a
    cross-cutting pattern.

  Then:
    git add -A
    git commit -m "harness-watch: <short summary>"
    git push origin main

If nothing is worth documenting, do NOT commit. Print a one-line summary
of what you checked and exit.

Keep edits tight: link to the upstream commit / paper, one paragraph of
context, and a concrete "what inber should consider" bullet. No fluff.

---
Upstream commits report:

$(cat "$COMMITS_FILE")
EOF
)"

# --dangerously-skip-permissions because this runs unattended under the
# scheduler. Scoped to the inber repo cwd. WebSearch needed for paper hunt.
exec claude -p \
  --dangerously-skip-permissions \
  --add-dir "$REPO_DIR" \
  "$PROMPT"
