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

# systemd scheduler runs us with a minimal PATH (no $HOME/bin), where `gh`
# lives. Without this, the per-harness `gh api` calls below fail silently
# (stderr swallowed by `2>/dev/null`) and every commit feed is reported as
# "(failed to fetch ...)". Prepend $HOME/bin so `gh` resolves.
export PATH="$HOME/bin:$PATH"

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

# Run the prompt through llm-bridge-server the same way bridge-ui creates a
# session: POST /sessions to spawn, POST /sessions/{id}/send to deliver the
# turn, then stream /sessions/{id}/events until a result/error terminates
# the turn. No local `claude` CLI involved — the server starts the harness
# under whatever instance config we point at.
LLM_BRIDGE="${LLM_BRIDGE_URL:-http://localhost:8160}"
INSTANCE_ID="${HARNESS_WATCH_INSTANCE_ID:-inst-cc-local}"
CLIENT_ID="harness-watch-$(date -u +%s)"

CREATE_BODY=$(jq -nc \
  --arg inst "$INSTANCE_ID" \
  --arg cid "$CLIENT_ID" \
  '{
    harness:     "claude_code",
    instance_id: $inst,
    client_id:   $cid,
    source:      "harness-watch",
    auto_start:  true
  }')

SESSION_JSON=$(curl -sfS -X POST "$LLM_BRIDGE/sessions" \
  -H "Content-Type: application/json" \
  -d "$CREATE_BODY")

BRIDGE_ID=$(printf '%s' "$SESSION_JSON" | jq -r '.bridge_id // empty')
if [[ -z "$BRIDGE_ID" ]]; then
  echo "harness-watch: failed to create session: $SESSION_JSON" >&2
  exit 1
fi
echo "harness-watch: bridge session $BRIDGE_ID"

PROMPT_JSON=$(printf '%s' "$PROMPT" | jq -Rs .)
curl -sfS -X POST "$LLM_BRIDGE/sessions/$BRIDGE_ID/send" \
  -H "Content-Type: application/json" \
  -d "{\"message\":${PROMPT_JSON}}" >/dev/null

# Stream the session's SSE; stop on the first result/error event of the
# current turn. We use a fifo so the curl runs as a tracked background
# process — without that, breaking out of the read loop would orphan curl
# with its pipe still open, and the scheduler would hang on EOF forever.
FIFO="$(mktemp -u)"
mkfifo "$FIFO"
curl -sN "$LLM_BRIDGE/sessions/$BRIDGE_ID/events" \
  -H 'Accept: text/event-stream' >"$FIFO" &
CURL_PID=$!
exec 3<"$FIFO"
rm -f "$FIFO"

cleanup() {
  exec 3<&- 2>/dev/null || true
  kill "$CURL_PID" 2>/dev/null || true
  wait "$CURL_PID" 2>/dev/null || true
  rm -f "$COMMITS_FILE"
}
trap cleanup EXIT

status=1
ev_type=""
while IFS= read -r -u 3 line || [[ -n "$line" ]]; do
  case "$line" in
    "event: "*)
      ev_type="${line#event: }"
      ;;
    "data: "*)
      data="${line#data: }"
      case "$ev_type" in
        result)
          printf '%s\n' "$data" | jq -r '.result.text // empty'
          status=0
          break
          ;;
        error)
          printf '%s\n' "$data" | jq -r '.error.message // .error // .' >&2
          status=1
          break
          ;;
      esac
      ;;
    "")
      ev_type=""
      ;;
  esac
done
exit "$status"
