#!/usr/bin/env bash
# scripts/dispatch/stack-cleanup.sh
#
# Find and remove orphaned dispatch stacks. Runnable anytime — agents that
# crash or worktrees that get deleted leave residue (compose projects, native
# PIDs, stack dirs); this script is the authoritative reaper.
#
# Usage: stack-cleanup.sh [--dry-run | -n]
#
# Cleans, in order:
#   1. docker compose projects whose name starts with "dispatch-" — torn
#      down with `down -v` (removes the project's containers, volumes,
#      networks in one shot).
#   2. PID files in .claude/worktrees/agent-*/dispatch-stack/*.pid whose
#      processes are dead — file removed.
#   3. PID files whose worktree no longer exists — process killed AND
#      file removed.
#   4. dispatch-stack/ directories whose parent worktree is gone — rm -rf.

set -euo pipefail

DRY_RUN=0
case "${1:-}" in
  --dry-run|-n) DRY_RUN=1 ;;
  '') ;;
  *) echo "Usage: $0 [--dry-run]" >&2; exit 64 ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

log() { echo "[stack-cleanup] $*"; }
DO() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "  [dry-run] would run: $*"
  else
    echo "  exec: $*"
    "$@" || true
  fi
}

# === 1. Compose projects ===
if command -v docker >/dev/null 2>&1; then
  log "Scanning compose projects starting with 'dispatch-'..."
  PROJECTS="$(docker compose ls -a --format json 2>/dev/null \
    | jq -r '.[] | .Name' 2>/dev/null \
    | grep -E '^dispatch-' || true)"
  if [ -n "$PROJECTS" ]; then
    while IFS= read -r project; do
      [ -z "$project" ] && continue
      log "Found compose project: $project"
      DO docker compose -p "$project" -f "$REPO_ROOT/backend/docker-compose.dispatch.yml" down -v
    done <<< "$PROJECTS"
  else
    log "  (no dispatch-* compose projects)"
  fi
else
  log "docker not found; skipping compose-project cleanup."
fi

# === 2-4. PID files + stack dirs ===
WORKTREES_DIR="$REPO_ROOT/.claude/worktrees"
if [ ! -d "$WORKTREES_DIR" ]; then
  log "No worktrees directory at $WORKTREES_DIR; nothing more to clean."
  log "Cleanup complete."
  exit 0
fi

log "Scanning $WORKTREES_DIR for orphaned stacks..."
LIVE_WORKTREES="$(git -C "$REPO_ROOT" worktree list --porcelain 2>/dev/null \
  | awk '/^worktree / { print $2 }' || true)"

shopt -s nullglob
ANY_FOUND=0
for stack_dir in "$WORKTREES_DIR"/*/dispatch-stack; do
  ANY_FOUND=1
  worktree_dir="$(dirname "$stack_dir")"
  worktree_id="$(basename "$worktree_dir")"

  WORKTREE_LIVE=0
  if echo "$LIVE_WORKTREES" | grep -qx "$worktree_dir"; then
    WORKTREE_LIVE=1
  fi

  if [ "$WORKTREE_LIVE" -eq 0 ]; then
    log "Orphan stack (worktree gone): $worktree_id"
    for pidfile in "$stack_dir/backend.pid" "$stack_dir/frontend.pid"; do
      [ -f "$pidfile" ] || continue
      pid="$(cat "$pidfile" 2>/dev/null || true)"
      if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        log "  killing orphan PID $pid"
        DO kill "$pid"
      fi
    done
    DO rm -rf "$stack_dir"
  else
    # Worktree alive — clean dead PID files only (process exited but file
    # never got removed; benign but accumulates over time).
    for pidfile in "$stack_dir/backend.pid" "$stack_dir/frontend.pid"; do
      [ -f "$pidfile" ] || continue
      pid="$(cat "$pidfile" 2>/dev/null || true)"
      if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
        log "Dead PID file: $pidfile"
        DO rm -f "$pidfile"
      fi
    done
  fi
done

if [ "$ANY_FOUND" -eq 0 ]; then
  log "  (no dispatch-stack directories found in any worktree)"
fi

log "Cleanup complete."
