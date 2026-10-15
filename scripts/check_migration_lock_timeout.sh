#!/usr/bin/env bash
# scripts/check_migration_lock_timeout.sh
#
# CI gate: a migration this branch ADDS that takes a strong table lock must run
# with a bounded lock_timeout. An unbounded lock wait at boot stalls the deploy
# and queues every later query on the table behind the migration.
#
# A migration takes a strong lock when its SQL (`--` and nested `/* */`
# comments stripped) contains ALTER TABLE, LOCK TABLE, DROP TABLE, or
# CREATE [UNIQUE] INDEX without CONCURRENTLY. It sets its own bound when the
# SQL contains lock_timeout (e.g. `SET LOCAL lock_timeout = '5s';`).
#
# Separately, CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS is always a
# WARNING (exit 0): a failed concurrent build leaves an INVALID index under
# that name, and the retry's IF NOT EXISTS keeps it.
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

# --- 2. up migrations this branch ADDS on top of the base --------------------
if ! ADDED_UP="$(git diff --name-only --diff-filter=A --no-renames \
                   "$BASE_REF" HEAD -- "$MIGRATIONS_PATH/*.up.sql")"; then
  echo "ERROR: could not diff HEAD against $BASE_REF." >&2
  exit 1
fi

if [ -z "$ADDED_UP" ]; then
  echo "OK: no new up migrations added against $BASE_REF."
  exit 0
fi

# --- 3. classify and report each added up migration --------------------------
# Prints file $1 with its comments removed, in one lowercase line so a
# statement split across lines still matches. A `--` comment runs to the end
# of its line; a `/* */` comment nests, may span lines, and reads as one space.
# Inside a block comment `--` is plain text, and inside a line comment `/*` is.
# String literals are not parsed: a comment marker inside one is treated as a
# comment.
folded_sql() {
  awk '
    {
      rest = $0
      while (rest != "") {
        open_at = index(rest, "/*")
        if (depth > 0) {
          close_at = index(rest, "*/")
          if (close_at == 0 && open_at == 0) break
          if (open_at > 0 && (close_at == 0 || open_at < close_at)) {
            depth++
            rest = substr(rest, open_at + 2)
          } else {
            depth--
            rest = substr(rest, close_at + 2)
            if (depth == 0) printf " "
          }
        } else {
          dash_at = index(rest, "--")
          if (open_at == 0 && dash_at == 0) { printf "%s", rest; break }
          if (dash_at > 0 && (open_at == 0 || dash_at < open_at)) {
            printf "%s", substr(rest, 1, dash_at - 1)
            break
          }
          printf "%s", substr(rest, 1, open_at - 1)
          depth = 1
          rest = substr(rest, open_at + 2)
        }
      }
      printf " "
    }
  ' "$1" | tr '\t' ' ' | tr '[:upper:]' '[:lower:]'
}

# Prints the strong-lock statement kinds found in $1 (output of folded_sql),
# one per line.
strong_locks() {
  local sql="$1" index_heads
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

# warn <file> <message>: a WARNING line, or a file annotation on GitHub Actions.
warn() {
  if [ "${GITHUB_ACTIONS:-}" = true ]; then
    echo "::warning file=$1::$2"
  else
    echo "WARNING: $2"
  fi
}

CIC_IF_NOT_EXISTS='create[[:space:]]+(unique[[:space:]]+)?index[[:space:]]+concurrently[[:space:]]+if[[:space:]]+not[[:space:]]+exists'

UNBOUNDED=0
for f in $ADDED_UP; do
  sql="$(folded_sql "$f")"
  if grep -qE "$CIC_IF_NOT_EXISTS" <<<"$sql"; then
    warn "$f" "$f uses CREATE INDEX CONCURRENTLY IF NOT EXISTS: a failed concurrent build leaves an INVALID index that the IF NOT EXISTS retry keeps."
  fi
  locks="$(strong_locks "$sql")"
  [ -n "$locks" ] || continue
  if grep -qi 'lock_timeout' <<<"$sql"; then
    continue
  fi
  UNBOUNDED=$((UNBOUNDED + 1))
  msg="$f takes a strong lock ($(tr '\n' ',' <<<"$locks" | sed 's/,$//; s/,/, /g')) and sets no lock_timeout."
  if [ "$RUNNER_BOUND" = no ]; then
    echo "ERROR: $msg" >&2
  else
    warn "$f" "$msg $ENTRYPOINT bounds it at boot; a manual migrate run is unbounded."
  fi
done

# --- 4. verdict ------------------------------------------------------------------
if [ "$UNBOUNDED" -eq 0 ]; then
  echo "OK: every new strong-lock migration sets its own lock_timeout (or none takes a strong lock)."
  exit 0
fi

if [ "$RUNNER_BOUND" = yes ]; then
  echo "OK (warnings only): the runner-level lock_timeout in $ENTRYPOINT is in place."
  exit 0
fi

echo "" >&2
echo "$ENTRYPOINT does not pass a lock_timeout to the migrate CLI" >&2
echo "(expected '$RUNNER_OPTION<duration>' and no -database \"\$DATABASE_URL\")." >&2
echo "Fix: restore the runner-level lock_timeout in $ENTRYPOINT, or add" >&2
echo "     SET LOCAL lock_timeout = '5s'; before the first locking statement." >&2
exit 1
