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

# Read mode + ports + the compose project name from .env if available
# (best-effort). STACK_COMPOSE_PROJECT is the *sanitized* name stack-up.sh
# actually created the project with — read it back rather than re-deriving, so
# the two scripts can never disagree (PSY-1203).
STACK_MODE=""
STACK_BACKEND_PORT=""
STACK_FRONTEND_PORT=""
STACK_COMPOSE_PROJECT=""
if [ -f "$STACK_DIR/.env" ]; then
  # tr -d '\r' so a CRLF-rewritten .env (e.g. hand-edited) can't smuggle a
  # trailing CR into STACK_MODE (breaking the "isolated" gate below) or into the
  # compose project name (a no-op teardown → leaked container).
  STACK_MODE="$(grep '^STACK_MODE=' "$STACK_DIR/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r' || true)"
  STACK_BACKEND_PORT="$(grep '^STACK_BACKEND_PORT=' "$STACK_DIR/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r' || true)"
  STACK_FRONTEND_PORT="$(grep '^STACK_FRONTEND_PORT=' "$STACK_DIR/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r' || true)"
  STACK_COMPOSE_PROJECT="$(grep '^STACK_COMPOSE_PROJECT=' "$STACK_DIR/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r' || true)"
fi
log "Mode: ${STACK_MODE:-unknown}"

# Kill native processes by PID file.
#
# PSY-635: the backend.pid points at `go run ./cmd/server`, which spawns a
# compiled `server` child. Killing the parent leaves the child reparented to
# init and still listening on the backend port. `nohup` does NOT make the
# parent a session leader, so the historical `kill -- -PID` (negative PID to
# signal the whole process group) misses the child. We do the PID kill first
# (terminates the go-run wrapper so it can't respawn), then below sweep the
# stack's allocated ports for any survivor — defensive against this exact
# parent/child split, plus any equivalent shape from bun's frontend tree.
for pidfile in "$STACK_DIR/backend.pid" "$STACK_DIR/frontend.pid"; do
  if [ -f "$pidfile" ]; then
    pid="$(cat "$pidfile" 2>/dev/null || true)"
    proc="$(basename "$pidfile" .pid)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      log "Killing $proc (PID $pid)..."
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

# PSY-635: reap port-bound survivors (e.g. the `server` child of `go run`).
# Ports were allocated free per-worktree, so anything still listening on
# STACK_BACKEND_PORT / STACK_FRONTEND_PORT after the PID kill is ours.
reap_port() {
  local label="$1" port="$2"
  [ -z "$port" ] && return 0
  command -v lsof >/dev/null 2>&1 || { log "lsof not found; skipping port sweep for $label."; return 0; }
  local survivors
  survivors="$(lsof -ti tcp:"$port" 2>/dev/null || true)"
  [ -z "$survivors" ] && return 0
  log "Reaping $label survivors on :$port (PIDs: $(echo "$survivors" | tr '\n' ' '))..."
  echo "$survivors" | xargs kill 2>/dev/null || true
  sleep 1
  survivors="$(lsof -ti tcp:"$port" 2>/dev/null || true)"
  [ -z "$survivors" ] && return 0
  log "  still alive on :$port, sending SIGKILL (PIDs: $(echo "$survivors" | tr '\n' ' '))"
  echo "$survivors" | xargs kill -9 2>/dev/null || true
}

reap_port backend "$STACK_BACKEND_PORT"
reap_port frontend "$STACK_FRONTEND_PORT"

# Tear down docker compose project if isolated.
if [ "$STACK_MODE" = "isolated" ]; then
  # Compose project names must be lowercase [a-z0-9_-]; a worktree dir like
  # "PSY-1205-radio-aired-surfaces" is not, so the raw "dispatch-${WORKTREE_ID}"
  # is rejected by docker and the postgres container leaks (PSY-1203). Prefer the
  # sanitized name stack-up.sh persisted; fall back to the SAME sanitize it
  # applies for .env files written before STACK_COMPOSE_PROJECT existed.
  COMPOSE_PROJECT="$STACK_COMPOSE_PROJECT"
  if [ -z "$COMPOSE_PROJECT" ]; then
    COMPOSE_PROJECT="dispatch-$(printf '%s' "$WORKTREE_ID" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '-')"
    COMPOSE_PROJECT="${COMPOSE_PROJECT%-}"
  fi
  log "Tearing down docker compose project $COMPOSE_PROJECT..."
  # Always run from the main repo's backend dir; the compose file is
  # identical in worktree and main, but the worktree may have been removed.
  cd "$REPO_ROOT/backend"
  docker compose -p "$COMPOSE_PROJECT" -f docker-compose.dispatch.yml down -v 2>&1 | tail -5 || true
fi

log "Removing $STACK_DIR..."
rm -rf "$STACK_DIR"

log "Stack down."
