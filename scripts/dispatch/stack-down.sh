#!/usr/bin/env bash
# scripts/dispatch/stack-down.sh
#
# Tear down a dispatched worktree's stack. Reads <worktree>/dispatch-stack/.env
# to know what was started and cleans accordingly.
#
# Usage: stack-down.sh <worktree-path>

set -euo pipefail

WORKTREE_PATH="${1:?Required: worktree path}"
WORKTREE_PATH="$(cd "$WORKTREE_PATH" && pwd)"
WORKTREE_ID="$(basename "$WORKTREE_PATH")"
STACK_DIR="$WORKTREE_PATH/dispatch-stack"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

log() { echo "[stack-down:$WORKTREE_ID] $*"; }

if [ ! -d "$STACK_DIR" ]; then
  log "No stack found at $STACK_DIR. Nothing to do."
  exit 0
fi

# Read mode from .env if available (best-effort).
STACK_MODE=""
if [ -f "$STACK_DIR/.env" ]; then
  STACK_MODE="$(grep '^STACK_MODE=' "$STACK_DIR/.env" 2>/dev/null | cut -d= -f2- || true)"
fi
log "Mode: ${STACK_MODE:-unknown}"

# Kill native processes by PID file.
for pidfile in "$STACK_DIR/backend.pid" "$STACK_DIR/frontend.pid"; do
  if [ -f "$pidfile" ]; then
    pid="$(cat "$pidfile" 2>/dev/null || true)"
    proc="$(basename "$pidfile" .pid)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      log "Killing $proc (PID $pid)..."
      # SIGTERM to the process group so go run / bun run children also exit.
      # Negative PID = signal to process group leader's group (requires the
      # process to have been a session leader, which `nohup` doesn't guarantee
      # but is best-effort).
      kill -- -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
      sleep 1
      if kill -0 "$pid" 2>/dev/null; then
        log "  $proc still alive, sending SIGKILL"
        kill -9 -- -"$pid" 2>/dev/null || kill -9 "$pid" 2>/dev/null || true
      fi
    fi
    rm -f "$pidfile"
  fi
done

# Tear down docker compose project if isolated.
if [ "$STACK_MODE" = "isolated" ]; then
  COMPOSE_PROJECT="dispatch-${WORKTREE_ID}"
  log "Tearing down docker compose project $COMPOSE_PROJECT..."
  # Always run from the main repo's backend dir; the compose file is
  # identical in worktree and main, but the worktree may have been removed.
  cd "$REPO_ROOT/backend"
  docker compose -p "$COMPOSE_PROJECT" -f docker-compose.dispatch.yml down -v 2>&1 | tail -5 || true
fi

log "Removing $STACK_DIR..."
rm -rf "$STACK_DIR"

log "Stack down."
