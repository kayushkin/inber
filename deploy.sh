#!/usr/bin/env bash
# Build inber-server, install the binary, write a host-local systemd
# user unit from the template, and restart the service.
#
# Env:
#   AUTH_STORE_TOKEN   bearer token the unit will send to auth-store.
#                      Required — fail loud if unset (single source of truth).
#
# The template at deploy/systemd/inber-server.service.template ships with
# __HOME__ and __AUTH_STORE_TOKEN__ placeholders so the in-repo file stays
# host-agnostic; this script substitutes them when writing the host unit.

set -euo pipefail

if [[ -x "$HOME/.local/bin/mise" ]]; then
  eval "$("$HOME/.local/bin/mise" activate bash)"
fi

# Required for systemctl --user to reach the session bus when invoked from
# non-login shells (e.g. agent/CI invocations).
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=${XDG_RUNTIME_DIR}/bus}"

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$HOME/bin/inber-server"
UNIT_NAME="inber-server.service"
UNIT_DIR="$HOME/.config/systemd/user"
TEMPLATE="$REPO_DIR/deploy/systemd/inber-server.service.template"

if [[ ! -f "$TEMPLATE" ]]; then
  echo "missing systemd template: $TEMPLATE" >&2
  exit 1
fi

if [[ -z "${AUTH_STORE_TOKEN:-}" ]]; then
  echo "AUTH_STORE_TOKEN env var is required (the bearer the unit sends to auth-store)" >&2
  exit 1
fi

cd "$REPO_DIR"

echo "==> Compiling inber-server..."
go build -o /tmp/inber-server-build ./cmd/inber-server

# Checked BEFORE the install, for the same reason the smokes are: an unidentifiable
# binary compiles perfectly and reads clean in the log, so installing first would put
# it in front of live sessions and only then tell us it cannot be traced to a commit.
echo "==> Checking provenance..."
buildinfo="$(go version -m /tmp/inber-server-build)"
vcs_revision="$(printf '%s\n' "$buildinfo" | awk -F= '$1 ~ /[[:space:]]vcs\.revision$/ {{print $2}}')"
vcs_modified="$(printf '%s\n' "$buildinfo" | awk -F= '$1 ~ /[[:space:]]vcs\.modified$/ {{print $2}}')"
if [ -z "$vcs_revision" ]; then
    echo "    REFUSING TO INSTALL: this binary carries no vcs.revision, so nothing can tie" >&2
    echo "    it back to a commit. 'go build' writes no VCS stamp when it cannot find a .git" >&2
    echo "    DIRECTORY, and it does not fail when that happens -- not even with -buildvcs=true." >&2
    echo "    The usual cause is building from a git worktree, whose .git is a pointer file." >&2
    echo "    Build from a real clone or checkout instead." >&2
    exit 1
fi
echo "    vcs.revision=$vcs_revision"
if [ "$vcs_modified" = "true" ]; then
    echo "    WARNING: built from a DIRTY tree (vcs.modified=true). $vcs_revision names the" >&2
    echo "    commit this binary was built NEAR, not the source it was built FROM, and that" >&2
    echo "    source is not recoverable from any commit. Commit first for a reproducible build." >&2
fi

echo "==> Stopping service..."
systemctl --user stop "$UNIT_NAME" 2>/dev/null || true
sleep 1

echo "==> Installing binary to $BIN..."
mkdir -p "$HOME/bin"
cp /tmp/inber-server-build "$BIN.new"
mv -f "$BIN.new" "$BIN"

echo "==> Rendering systemd unit from template..."
mkdir -p "$UNIT_DIR"
sed \
  -e "s|__HOME__|$HOME|g" \
  -e "s|__AUTH_STORE_TOKEN__|$AUTH_STORE_TOKEN|g" \
  "$TEMPLATE" > "$UNIT_DIR/$UNIT_NAME"
chmod 600 "$UNIT_DIR/$UNIT_NAME"   # contains AUTH_STORE_TOKEN

systemctl --user daemon-reload
systemctl --user enable "$UNIT_NAME" 2>/dev/null || true

echo "==> Starting service..."
systemctl --user start "$UNIT_NAME"

echo "==> Verifying..."
sleep 2
systemctl --user is-active "$UNIT_NAME"

echo "==> Health check..."
curl -fsS http://127.0.0.1:8200/api/health || {
  echo "FAILED — health check did not respond"
  exit 1
}
echo

echo "==> Done. inber-server is live on :8200."
