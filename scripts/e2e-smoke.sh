#!/usr/bin/env bash
# Boot-and-answer smoke test for inber-server.
#
# Builds cmd/inber-server from THIS source tree, boots it on a temp port against a
# throwaway HOME, data dir and workspace, and drives a real write-then-read-back
# through the memory store — a path that needs no LLM, no API key and no network.
#
# Why this exists: `go build` proves the tree compiles, not that the binary is
# alive. Go 1.22+ http.ServeMux panics on a conflicting route pattern at
# REGISTRATION time — the compiler never sees it, `go vet` never sees it, and the
# process dies on its first breath. Tier 1 of the repo guard calls such a tree
# green. This is tier 2: it boots the thing and makes it answer.
#
# ############################################################################
# 🔴 READ THIS BEFORE CHANGING ANY ENV VAR BELOW: A LEAKY RUN KILLS PROD.
#
# inber-server takes a PID-FILE LOCK at $DataDir/inber.pid, and Acquire()
# (server/pidfile.go:26-60) does not merely refuse to start when that file names a
# living process — it SIGTERMs it, waits 5s, then SIGKILLs it.
#
# DataDir defaults to $HOME/.inber/server (server/server.go, the cfg.DataDir default in New) and there is NO
# env var to override it. So an inber-server booted with the REAL $HOME reads the
# LIVE server's pid out of the LIVE pid file and KILLS THE LIVE SERVER — while
# reporting a clean, successful startup. It would look exactly like a passing test.
#
# Two things prevent that here, and BOTH are load-bearing:
#   1. HOME is redirected to a temp dir.
#   2. data_dir is ALSO pinned explicitly in the config file, so the default path
#      is never consulted at all.
# And because a mistake here is unrecoverable-by-a-test-run, the trap below checks
# on EVERY exit path that the live inber-server is still breathing.
# ############################################################################
#
# NATS: inber is fail-soft on the bus (bus.NewClient returns nil and logs), but
# NATS_URL DEFAULTS TO nats://localhost:4222 and cannot be disabled with an empty
# value (main.go:74-78). Left alone on this host it would connect to the LIVE bus
# and QueueSubscribe("chat.inbound.inber") in queue group "inber-server" — the SAME
# queue group as prod — and start stealing roughly half of the live inbound chat
# messages. So NATS_URL is pointed at a closed port. It must stay that way.
#
# --require is OMITTED on purpose. The live unit passes --require=nats,workspace,
# which promotes those checks to FATAL (server/selftest.go:27-95). With no
# --require, only agent-store is critical — and that is satisfied by the fresh
# SQLite DB the temp HOME gives us. This is how inber boots with no bus, no
# workspace and no ANTHROPIC_API_KEY.
#
# READINESS IS /health, NOT /api/health. /api/health reports status=error and
# returns HTTP 503 whenever the bus is absent (api_health.go:36-38) — which is
# always, here, by design. /health (api_bridge.go, handleBridgeHealth) is bus-independent.
#
# Exits 0 on success, non-zero on the FIRST failing assertion, dumping the server
# log to stderr.
#
# Env:
#   E2E_PORT   — inber-server port (default 19132)
#   E2E_KEEP=1 — leave $TMP_DIR in place after the run

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${E2E_PORT:-19132}"
BASE="http://127.0.0.1:$PORT"

for bin in go curl jq; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "ERROR: required tool '$bin' not found on PATH" >&2
    exit 2
  fi
done

TMP_DIR="$(mktemp -d -t inber-e2e.XXXXXX)"
BIN_DIR="$TMP_DIR/bin"
FAKE_HOME="$TMP_DIR/home"
DATA_DIR="$TMP_DIR/data"
WORKSPACE="$TMP_DIR/ws"
CONFIG="$TMP_DIR/config.json"
SERVER_LOG="$TMP_DIR/server.log"
mkdir -p "$BIN_DIR" "$FAKE_HOME" "$DATA_DIR" "$WORKSPACE"

# --- live-process integrity: the thing this smoke must never break -------------
# Fingerprint the live inber-server BEFORE we build or boot anything. If our
# process ever resolves the real DataDir, it will kill this pid — see the header.
# Checked from the trap so that a run which FAILS still tells us whether it took
# prod down on the way out. An assertion that only runs on the success path cannot.
LIVE_INBER_PID="$(pgrep -f 'inber-server' | head -1 || true)"

SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [ -n "$LIVE_INBER_PID" ] && ! kill -0 "$LIVE_INBER_PID" 2>/dev/null; then
    echo "" >&2
    echo "*** THE LIVE inber-server (pid $LIVE_INBER_PID) IS GONE ***" >&2
    echo "    This smoke killed it. That means the server it booted resolved the" >&2
    echo "    REAL data dir and took the pid-file lock (server/pidfile.go:26-60)," >&2
    echo "    which SIGTERM/SIGKILLs whatever inber it finds there." >&2
    echo "    Check HOME and data_dir before running this again." >&2
    rm -rf "$TMP_DIR"
    exit 1
  fi
  if [ "${E2E_KEEP:-}" = "1" ]; then
    echo "[e2e] keeping $TMP_DIR"
  else
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT INT TERM

step() { printf '\n==> %s\n' "$*"; }
dump_logs() {
  echo "----- server.log -----" >&2
  cat "$SERVER_LOG" >&2 2>/dev/null || true
}
fail() { echo "FAIL: $*" >&2; dump_logs; exit 1; }

AGENT="smoke"
CONTENT="e2e-memory-$$-$(od -An -N4 -tu4 </dev/urandom | tr -d ' ')"

step "build cmd/inber-server from $REPO_DIR"
cd "$REPO_DIR"
# Explicit -o: a bare `go build ./cmd/inber-server` writes the binary into the CWD,
# which in a clean clone is the repo root.
# CGO stays ENABLED on purpose — inber pulls mattn/go-sqlite3 in through
# agent-store/tool-store/repo-store/forge, and that driver is cgo-only. Forcing
# CGO_ENABLED=0 must fail the BUILD here rather than ship a binary that dies
# opening its own database.
go build -o "$BIN_DIR/inber-server" ./cmd/inber-server
echo "    inber-server: $(ls -lh "$BIN_DIR/inber-server" | awk '{print $5}')"
[ -n "$LIVE_INBER_PID" ] && echo "    live inber-server pid $LIVE_INBER_PID (must survive this run)"

step "write a throwaway config (data_dir pinned — see the header)"
# The agent MUST declare a workspace: handleMemorySave 400s on an empty one and,
# unlike the GET path, does NOT fall back to the default agent (api_memory.go:154).
# The memory DB is created under <workspace>/.inber/memory.db.
cat >"$CONFIG" <<EOF
{
  "agents": {
    "$AGENT": {
      "name": "$AGENT",
      "workspace": "$WORKSPACE",
      "model": "claude-sonnet-4-5-20250929"
    }
  },
  "default_agent": "$AGENT",
  "data_dir": "$DATA_DIR"
}
EOF
echo "    $CONFIG"

step "launch inber-server on :$PORT (no bus, no workspace check, no API key)"
env -i \
  PATH="$PATH" \
  HOME="$FAKE_HOME" \
  NATS_URL="nats://127.0.0.1:1" \
  "$BIN_DIR/inber-server" --addr "127.0.0.1:$PORT" --config "$CONFIG" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
echo "    pid: $SERVER_PID"

# Poll — never sleep-and-hope. Abort the instant the pid dies: that is how a route
# registration panic (or a selftest fatal) surfaces as a named failure rather than
# a mystery timeout.
health=""
for _ in $(seq 1 75); do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    fail "inber-server exited during startup (route registration panic? selftest fatal? see log)"
  fi
  if health=$(curl -fsS --max-time 2 "$BASE/health" 2>/dev/null); then break; fi
  health=""
  sleep 0.2
done
[ -n "$health" ] || fail "inber-server did not answer $BASE/health within 15s"
[ "$(jq -r '.status' <<<"$health")" = "ok" ] || fail "/health did not report status=ok: $health"
echo "    health OK: $health"

step "SANDBOX: the pid-file lock must have landed in the TEMP data dir"
# If this file is missing, DataDir resolved somewhere else — and the only other
# place it resolves to is the live one, whose pid file names the live server.
# Assert it here, loudly, rather than discovering it from a dead prod process.
[ -f "$DATA_DIR/inber.pid" ] \
  || fail "no pid file at $DATA_DIR/inber.pid — data_dir was NOT honoured, and this process may have taken the LIVE lock"
LOCKED_PID="$(cat "$DATA_DIR/inber.pid")"
[ "$LOCKED_PID" = "$SERVER_PID" ] \
  || fail "the temp pid file names $LOCKED_PID, not our server ($SERVER_PID)"
[ -n "$LIVE_INBER_PID" ] && { kill -0 "$LIVE_INBER_PID" 2>/dev/null \
  || fail "the live inber-server (pid $LIVE_INBER_PID) died during our boot — we took its lock"; }
echo "    pid file: $DATA_DIR/inber.pid -> $LOCKED_PID (ours), live server intact"

step "GET /harnesses — the bridge surface answers"
HARNESSES=$(curl -fsS --max-time 10 "$BASE/harnesses") || fail "GET /harnesses failed"
[ "$(jq -r 'type' <<<"$HARNESSES")" = "array" ] || fail "/harnesses is not an array: $HARNESSES"
echo "    $(jq -c . <<<"$HARNESSES")"

step "GET /api/memory — must be EMPTY (proves we are on a fresh temp memory DB)"
# A nil slice serialises as `null` here, not `[]` — so accept either, but nothing
# else. A non-empty list means we opened a REAL memory database.
MEMS=$(curl -fsS --max-time 10 "$BASE/api/memory?agent=$AGENT") || fail "GET /api/memory failed"
COUNT=$(jq -r 'if . == null then 0 else length end' <<<"$MEMS")
[ "$COUNT" = "0" ] \
  || fail "a fresh workspace should hold 0 memories but /api/memory returned $COUNT — ABORTING, this may be a REAL memory database"
echo "    0 memories"

step "POST /api/memory — write (pure SQLite: local embedder, no API key, no network)"
CREATED=$(curl -fsS --max-time 20 -X POST "$BASE/api/memory" \
  -H 'Content-Type: application/json' \
  -d "{\"content\":\"$CONTENT\",\"tags\":[\"e2e\"],\"agent\":\"$AGENT\"}") \
  || fail "POST /api/memory failed"
MEM_ID=$(jq -r '.id // empty' <<<"$CREATED")
[ -n "$MEM_ID" ] || fail "POST /api/memory returned no id: $CREATED"
echo "    created memory id=$MEM_ID"

# The memory DB is the workspace's, not the data dir's. If this file is absent,
# workspaceForAgent() gave back something other than our temp workspace.
[ -f "$WORKSPACE/.inber/memory.db" ] \
  || fail "no memory db at $WORKSPACE/.inber/memory.db — the agent's workspace was not honoured"

step "GET /api/memory/{id} — read the write back out of SQLite"
# ⚠ memory-store's Memory struct carries NO json tags (memory-store/memory.go:10),
# so inber serialises it with Go FIELD NAMES: .Content, .Tags, .Importance —
# capitalised. It is NOT the lowercase DTO that memory-store's own HTTP server
# returns. Read the struct; never guess the wire shape.
GOT=$(curl -fsS --max-time 10 "$BASE/api/memory/$MEM_ID?agent=$AGENT") || fail "GET /api/memory/$MEM_ID failed"
[ "$(jq -r '.Content' <<<"$GOT")" = "$CONTENT" ] || fail "read-back Content != what we wrote: $GOT"
[ "$(jq -r '.ID'      <<<"$GOT")" = "$MEM_ID"  ] || fail "read-back ID != $MEM_ID: $GOT"
[ "$(jq -r '.Tags | index("e2e") // "no"' <<<"$GOT")" != "no" ] || fail "read-back lost the e2e tag: $GOT"
# Save() populates the embedding with a local bag-of-words vector. An empty one
# means the write path silently skipped it, and semantic search would be dead.
EMBED_LEN=$(jq -r '.Embedding | if . == null then 0 else length end' <<<"$GOT")
[ "$EMBED_LEN" -gt 0 ] 2>/dev/null || fail "the stored memory has no embedding — Save() did not run its embedder: $GOT"
echo "    read back: Content=$CONTENT, tags=[e2e], embedding=${EMBED_LEN} dims"

step "GET /api/memory — the list now contains exactly our memory"
MEMS=$(curl -fsS --max-time 10 "$BASE/api/memory?agent=$AGENT")
[ "$(jq -r 'length' <<<"$MEMS")" = "1" ] || fail "expected exactly 1 memory in the list: $MEMS"
[ "$(jq -r '.[0].ID' <<<"$MEMS")" = "$MEM_ID" ] || fail "the listed memory is not the one we wrote: $MEMS"
echo "    1 memory, id=$MEM_ID"

step "process still alive, and so is prod"
kill -0 "$SERVER_PID" 2>/dev/null || fail "inber-server died while serving"
[ -n "$LIVE_INBER_PID" ] && { kill -0 "$LIVE_INBER_PID" 2>/dev/null \
  || fail "the live inber-server (pid $LIVE_INBER_PID) is gone"; }

printf '\nsmoke test OK (inber-server boots without a bus and round-trips a memory on :%s)\n' "$PORT"
