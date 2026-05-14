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
