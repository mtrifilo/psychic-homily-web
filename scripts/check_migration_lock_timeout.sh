#!/usr/bin/env bash
# scripts/check_migration_lock_timeout.sh
#
# CI gate: a migration this branch ADDS that takes a strong table lock must run
# with a bounded lock_timeout. An unbounded lock wait at boot stalls the deploy
# and queues every later query on the table behind the migration.
#
# A migration takes a strong lock when its SQL (comments stripped) contains
# ALTER TABLE, LOCK TABLE, DROP TABLE, or CREATE [UNIQUE] INDEX without
# CONCURRENTLY. It sets its own bound when the SQL contains lock_timeout
# (e.g. `SET LOCAL lock_timeout = '5s';`).
#
# Two modes, chosen by reading backend/docker-entrypoint.sh:
#   * Runner-level bound in place: the entrypoint passes a lock_timeout option
#     to the migrate CLI and never hands it the bare DATABASE_URL. A strong-lock
#     migration without its own lock_timeout is reported as a WARNING (exit 0):
#     the boot run is bounded, a manual `migrate` run is not.
#   * Runner-level bound absent: the same migration is an ERROR (exit 1).
#
# Usage:
#   scripts/check_migration_lock_timeout.sh [base-ref]
#
# base-ref defaults to origin/main. The CALLER makes that ref current; this
# script does no network I/O. The "added" set is the same two-dot
# --diff-filter=A --no-renames diff check_migration_timestamps.sh uses.

set -euo pipefail
# No globbing: filenames flow through unquoted word-splitting below.
set -f

BASE_REF="${1:-origin/main}"
MIGRATIONS_PATH="backend/db/migrations"
ENTRYPOINT="backend/docker-entrypoint.sh"
RUNNER_OPTION='options=-c%20lock_timeout%3D'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

if ! git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null; then
  echo "ERROR: base ref '$BASE_REF' not found in this repository." >&2
  echo "       Fetch it first, e.g. 'git fetch origin main'." >&2
  exit 1
fi

# --- 1. is the runner-level bound in place? ----------------------------------
# Comment lines are dropped so a commented-out option does not count. A missing
# entrypoint reads as "absent", which is the strict mode.
ENTRY_CODE=""
if [ -f "$ENTRYPOINT" ]; then
  ENTRY_CODE="$(grep -v '^[[:space:]]*#' "$ENTRYPOINT" || true)"
fi

# Here-strings, not pipes, feed every `grep -q` in this script: under pipefail
# an early-exiting grep can SIGPIPE its writer and turn a match into a miss.
RUNNER_BOUND=no
if grep -qF "$RUNNER_OPTION" <<<"$ENTRY_CODE" \
   && ! grep -qF -- '-database "$DATABASE_URL"' <<<"$ENTRY_CODE"; then
  RUNNER_BOUND=yes
fi

# --- 2. migration files this branch ADDS on top of the base ------------------
if ! HEAD_DIFF="$(git diff --name-only --diff-filter=A --no-renames \
                    "$BASE_REF" HEAD -- "$MIGRATIONS_PATH")"; then
  echo "ERROR: could not diff HEAD against $BASE_REF." >&2
  exit 1
fi

ADDED_UP="$(printf '%s\n' "$HEAD_DIFF" | grep -E '\.up\.sql$' || true)"

if [ -z "$ADDED_UP" ]; then
  echo "OK: no new up migrations added against $BASE_REF."
  exit 0
fi

# --- 3. classify each added up migration --------------------------------------
# Prints the strong-lock statement kinds found in $1 (SQL text), one per line.
# The caller strips line comments; the text is folded to one lowercase line so
# a statement split across lines still matches.
strong_locks() {
  local sql index_heads
  sql="$(tr '\n\t' '  ' <<<"$1" | tr '[:upper:]' '[:lower:]')"
  grep -qE 'alter[[:space:]]+table' <<<"$sql" && echo "ALTER TABLE"
  grep -qE 'lock[[:space:]]+table' <<<"$sql" && echo "LOCK TABLE"
  grep -qE 'drop[[:space:]]+table' <<<"$sql" && echo "DROP TABLE"
  # One line per CREATE INDEX, ending in the word after INDEX.
  index_heads="$(grep -oE 'create[[:space:]]+(unique[[:space:]]+)?index[[:space:]]+[^[:space:]]+' <<<"$sql" || true)"
  if [ -n "$index_heads" ] && grep -qvE '[[:space:]]concurrently$' <<<"$index_heads"; then
    echo "CREATE INDEX without CONCURRENTLY"
  fi
  return 0
}

UNBOUNDED=""
for f in $ADDED_UP; do
  sql="$(sed 's/--.*$//' "$f")"
  locks="$(strong_locks "$sql")"
  [ -n "$locks" ] || continue
  if grep -qi 'lock_timeout' <<<"$sql"; then
    continue
  fi
  UNBOUNDED="$UNBOUNDED $f"
  echo "  $f takes a strong lock: $(tr '\n' ',' <<<"$locks" | sed 's/,$//; s/,/, /g')"
done

if [ -z "$UNBOUNDED" ]; then
  echo "OK: every new strong-lock migration sets its own lock_timeout (or none takes a strong lock)."
  exit 0
fi

# --- 4. report -----------------------------------------------------------------
if [ "$RUNNER_BOUND" = yes ]; then
  for f in $UNBOUNDED; do
    msg="$f takes a strong lock and sets no lock_timeout. $ENTRYPOINT bounds it at boot; a manual migrate run waits indefinitely unless the file adds: SET LOCAL lock_timeout = '5s';"
    if [ "${GITHUB_ACTIONS:-}" = true ]; then
      echo "::warning file=$f::$msg"
    else
      echo "WARNING: $msg"
    fi
  done
  echo "OK (warnings only): the runner-level lock_timeout in $ENTRYPOINT is in place."
  exit 0
fi

echo "ERROR: new migration(s) take a strong lock with no lock_timeout, and" >&2
echo "       $ENTRYPOINT does not pass one to the migrate CLI" >&2
echo "       (expected '$RUNNER_OPTION<duration>' and no -database \"\$DATABASE_URL\"):" >&2
for f in $UNBOUNDED; do echo "         $f" >&2; done
echo "" >&2
echo "Fix: restore the runner-level lock_timeout in $ENTRYPOINT, or add" >&2
echo "     SET LOCAL lock_timeout = '5s'; before the first locking statement." >&2
exit 1
