#!/usr/bin/env bash
# scripts/check_migration_lock_timeout_test.sh
#
# Self-test for scripts/check_migration_lock_timeout.sh. Builds a throwaway git
# repository in a temp dir, replays the shapes the gate has to get right in
# both modes (runner-level lock_timeout present: warn; absent: fail), and
# asserts the exit code and the actionable part of the message.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/check_migration_lock_timeout_test.sh
#
# It touches nothing outside its own mktemp -d and needs no network.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/check_migration_lock_timeout.sh"
REAL_ENTRYPOINT="$SCRIPT_DIR/../backend/docker-entrypoint.sh"

FIXTURE="$(mktemp -d)" || exit 1
trap 'rm -rf "$FIXTURE"' EXIT

FAILURES=0
CASE=""

# The fixture's own GITHUB_ACTIONS is unset so the warning text is the plain
# WARNING: form on every host.
unset GITHUB_ACTIONS

# Fixture git. Commit signing and a global core.hooksPath are neutralised so
# the self-test behaves the same on a CI runner and on a laptop that has either
# turned on globally.
g() {
  git -C "$FIXTURE" -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@" >/dev/null 2>&1
}

# migration <version>_<name> <up-sql>: an up/down pair, committed.
migration() {
  printf '%s\n' "$2" >"$FIXTURE/backend/db/migrations/$1.up.sql"
  printf 'SELECT 1;\n' >"$FIXTURE/backend/db/migrations/$1.down.sql"
  g add -A
  g commit -m "$1"
}

# entrypoint bound|unbound|commented|bare-url: the runner shapes the gate reads.
# "bound" is the repository's real entrypoint, so a rewrite of it that the gate
# no longer recognises fails the bound cases below.
entrypoint() {
  local f="$FIXTURE/backend/docker-entrypoint.sh"
  case "$1" in
    bound)
      cp "$REAL_ENTRYPOINT" "$f"
      ;;
    unbound)
      cat >"$f" <<'EOF'
#!/bin/sh
migrate -path /app/db/migrations -database "$DATABASE_URL" up
EOF
      ;;
    commented)
      cat >"$f" <<'EOF'
#!/bin/sh
# MIGRATE_LOCK_TIMEOUT_OPTION='options=-c%20lock_timeout%3D5s'
migrate -path /app/db/migrations -database "$MIGRATE_DATABASE_URL" up
EOF
      ;;
    bare-url)
      # The option is defined, but one migrate call still gets the bare URL.
      cat >"$f" <<'EOF'
#!/bin/sh
MIGRATE_LOCK_TIMEOUT_OPTION='options=-c%20lock_timeout%3D5s'
MIGRATE_DATABASE_URL="${DATABASE_URL}?${MIGRATE_LOCK_TIMEOUT_OPTION}"
migrate -path /app/db/migrations -database "$DATABASE_URL" up
EOF
      ;;
  esac
  g add -A
  g commit -m "entrypoint $1"
}

# Reset the fixture to: main holding the given entrypoint shape and one merged
# migration that takes a strong lock without lock_timeout (merged files are
# never re-checked), with a fresh branch checked out on top.
reset_fixture() {
  CASE="$1"
  rm -rf "$FIXTURE"
  mkdir -p "$FIXTURE/scripts" "$FIXTURE/backend/db/migrations"
  cp "$UNDER_TEST" "$FIXTURE/scripts/check_migration_lock_timeout.sh"
  g init -b main
  g config user.email test@example.com
  g config user.name "Lock Timeout Gate Test"
  entrypoint "$2"
  migration "20260801000000_merged_alter" "ALTER TABLE t ADD COLUMN c int;"
  g checkout -b feature
}

# expect <code> [needle] [base-ref]: assert the gate's exit code and that its
# output (stderr folded into stdout) contains the needle. The base ref
# defaults to main. The output stays in LAST_OUT for refute.
LAST_OUT=""
expect() {
  local want_code="$1" needle="${2:-}" base_ref="${3:-main}"
  local out code
  out="$(bash "$FIXTURE/scripts/check_migration_lock_timeout.sh" "$base_ref" 2>&1)"
  code=$?
  LAST_OUT="$out"
  if [ "$code" -ne "$want_code" ]; then
    echo "FAIL [$CASE]: exit $code, wanted $want_code"
    printf '%s\n' "$out" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
    return
  fi
  if [ -n "$needle" ] && ! printf '%s' "$out" | grep -qF -- "$needle"; then
    echo "FAIL [$CASE]: output missing expected text: $needle"
    printf '%s\n' "$out" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
    return
  fi
  echo "ok   [$CASE]"
}

# refute <needle>: assert the output of the last expect does not contain it.
refute() {
  if printf '%s' "$LAST_OUT" | grep -qF -- "$1"; then
    echo "FAIL [$CASE]: output contains unexpected text: $1"
    printf '%s\n' "$LAST_OUT" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
  fi
}

# --- runner-level bound absent: strong-lock migrations without their own
# lock_timeout fail ------------------------------------------------------------
reset_fixture "no new migrations passes" unbound
expect 0 "no new up migrations added"

reset_fixture "merged strong-lock migration is not re-checked" unbound
migration "20260901000000_select_only" "SELECT 1;"
expect 0 "every new strong-lock migration sets its own lock_timeout"

reset_fixture "unbound: ALTER TABLE without lock_timeout fails" unbound
migration "20260901000000_widen" "ALTER TABLE webauthn_credentials ALTER COLUMN sign_count TYPE BIGINT;"
expect 1 "backend/db/migrations/20260901000000_widen.up.sql"

reset_fixture "unbound: failure names the fix" unbound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "SET LOCAL lock_timeout = '5s'; before the first locking statement"

reset_fixture "unbound: SET LOCAL lock_timeout passes" unbound
migration "20260901000000_widen" "SET LOCAL lock_timeout = '5s';
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 0 "every new strong-lock migration sets its own lock_timeout"

reset_fixture "unbound: LOCK TABLE fails" unbound
migration "20260901000000_lock" "LOCK TABLE user_bookmarks IN SHARE ROW EXCLUSIVE MODE;"
expect 1 "takes a strong lock (LOCK TABLE)"

reset_fixture "unbound: DROP TABLE fails" unbound
migration "20260901000000_drop" "DROP TABLE IF EXISTS old_things;"
expect 1 "takes a strong lock (DROP TABLE)"

reset_fixture "unbound: plain CREATE INDEX fails" unbound
migration "20260901000000_idx" "CREATE INDEX IF NOT EXISTS idx_t_c ON t (c);"
expect 1 "CREATE INDEX without CONCURRENTLY"

reset_fixture "unbound: CREATE UNIQUE INDEX on a quoted name fails" unbound
migration "20260901000000_idx" 'CREATE UNIQUE INDEX "Idx_T_C" ON t (c);'
expect 1 "CREATE INDEX without CONCURRENTLY"

reset_fixture "unbound: CREATE INDEX CONCURRENTLY passes" unbound
migration "20260901000000_idx" "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_c ON t (c);"
expect 0 "every new strong-lock migration sets its own lock_timeout"

reset_fixture "unbound: a plain CREATE INDEX beside a CONCURRENTLY one fails" unbound
migration "20260901000000_idx" "CREATE INDEX CONCURRENTLY idx_t_a ON t (a);
CREATE INDEX idx_t_b ON t (b);"
expect 1 "CREATE INDEX without CONCURRENTLY"

reset_fixture "unbound: lowercase, multi-line ALTER TABLE fails" unbound
migration "20260901000000_widen" "alter
    table t alter column c type bigint;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: every kind in one file is listed" unbound
migration "20260901000000_many" "ALTER TABLE t ADD COLUMN d int;
DROP TABLE u;
CREATE INDEX idx_t_d ON t (d);"
expect 1 "takes a strong lock (ALTER TABLE, DROP TABLE, CREATE INDEX without CONCURRENTLY)"

reset_fixture "unbound: ALTER TABLE only inside a comment passes" unbound
migration "20260901000000_note" "-- ALTER TABLE t is done in the next migration.
SELECT 1;"
expect 0 "every new strong-lock migration sets its own lock_timeout"

reset_fixture "unbound: lock_timeout only inside a comment fails" unbound
migration "20260901000000_widen" "-- lock_timeout is set by the runner.
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "20260901000000_widen.up.sql"

reset_fixture "unbound: a strong lock only in down.sql passes" unbound
printf 'SELECT 1;\n' >"$FIXTURE/backend/db/migrations/20260901000000_add.up.sql"
printf 'DROP TABLE t;\n' >"$FIXTURE/backend/db/migrations/20260901000000_add.down.sql"
g add -A && g commit -m "down only"
expect 0 "every new strong-lock migration sets its own lock_timeout"

# --- block comments are stripped like line comments, and nest as in Postgres --
reset_fixture "unbound: lock_timeout only inside a block comment fails" unbound
migration "20260901000000_widen" "/* lock_timeout is set by the runner. */
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "20260901000000_widen.up.sql"

reset_fixture "unbound: lock_timeout only inside a multi-line block comment fails" unbound
migration "20260901000000_widen" "/*
 * lock_timeout is set by the runner.
 */
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "20260901000000_widen.up.sql"

reset_fixture "unbound: lock_timeout inside a nested block comment fails" unbound
migration "20260901000000_widen" "/* outer /* inner */ lock_timeout still commented */
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "20260901000000_widen.up.sql"

reset_fixture "unbound: ALTER TABLE only inside a block comment passes" unbound
migration "20260901000000_note" "/* ALTER TABLE t is done
   in the next migration. */
SELECT 1;"
expect 0 "every new strong-lock migration sets its own lock_timeout"

reset_fixture "unbound: '--' inside a block comment does not hide what follows" unbound
migration "20260901000000_widen" "/* -- note */ ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: '/*' inside a line comment opens no block" unbound
migration "20260901000000_widen" "-- see /* below
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: a block comment between keywords separates them" unbound
migration "20260901000000_widen" "ALTER/* c */TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: '/*' inside a string literal opens no block" unbound
migration "20260901000000_widen" "COMMENT ON TABLE t IS 'paths like /api/* stay text';
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: '--' inside a string literal does not hide what follows" unbound
migration "20260901000000_widen" "SELECT '--'; ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: a comment after a doubled quote is still stripped" unbound
migration "20260901000000_widen" "SELECT 'it''s'; -- lock_timeout is set by the runner.
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "20260901000000_widen.up.sql"

reset_fixture "unbound: comment markers in a multi-line string literal are text" unbound
migration "20260901000000_widen" "COMMENT ON TABLE t IS 'line one
-- not a comment /* nor this';
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "takes a strong lock (ALTER TABLE)"

reset_fixture "unbound: SET LOCAL lock_timeout after a block comment passes" unbound
migration "20260901000000_widen" "/* widen */ SET LOCAL lock_timeout = '5s';
ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 0 "every new strong-lock migration sets its own lock_timeout"

# --- CREATE INDEX CONCURRENTLY IF NOT EXISTS warns in both modes -------------
reset_fixture "unbound: CONCURRENTLY IF NOT EXISTS warns and passes" unbound
migration "20260901000000_idx" "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_c ON t (c);"
expect 0 "WARNING: backend/db/migrations/20260901000000_idx.up.sql uses CREATE INDEX CONCURRENTLY IF NOT EXISTS: a failed concurrent build leaves an INVALID index that the IF NOT EXISTS retry keeps."

reset_fixture "bound: UNIQUE, lowercase, multi-line CONCURRENTLY IF NOT EXISTS warns" bound
migration "20260901000000_idx" "create unique index concurrently
    if not exists idx_t_c on t (c);"
expect 0 "uses CREATE INDEX CONCURRENTLY IF NOT EXISTS"

reset_fixture "bound: CONCURRENTLY IF NOT EXISTS gets a file annotation" bound
migration "20260901000000_idx" "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_c ON t (c);"
GITHUB_ACTIONS=true expect 0 "::warning file=backend/db/migrations/20260901000000_idx.up.sql::"

reset_fixture "unbound: CONCURRENTLY without IF NOT EXISTS does not warn" unbound
migration "20260901000000_idx" "CREATE INDEX CONCURRENTLY idx_t_c ON t (c);"
expect 0 "every new strong-lock migration sets its own lock_timeout"
refute "WARNING"

reset_fixture "unbound: CONCURRENTLY IF NOT EXISTS only inside comments does not warn" unbound
migration "20260901000000_idx" "/* CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_c ON t (c); */
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_d ON t (d);
SELECT 1;"
expect 0 "every new strong-lock migration sets its own lock_timeout"
refute "WARNING"

# --- runner-level bound present: the same migrations only warn ----------------
reset_fixture "bound: ALTER TABLE without lock_timeout warns and passes" bound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 0 "WARNING: backend/db/migrations/20260901000000_widen.up.sql takes a strong lock (ALTER TABLE) and sets no lock_timeout"

reset_fixture "bound: warning mode says so" bound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 0 "OK (warnings only): the runner-level lock_timeout"

reset_fixture "bound: GitHub Actions gets a file annotation" bound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
GITHUB_ACTIONS=true expect 0 "::warning file=backend/db/migrations/20260901000000_widen.up.sql::"

# --- the runner check itself must not be fooled ------------------------------
reset_fixture "commented-out runner option is strict mode" commented
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "does not pass a lock_timeout to the migrate CLI"

reset_fixture "a migrate call on the bare DATABASE_URL is strict mode" bare-url
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "does not pass a lock_timeout to the migrate CLI"

reset_fixture "missing entrypoint is strict mode" bound
g rm backend/docker-entrypoint.sh && g commit -m "drop entrypoint"
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "does not pass a lock_timeout to the migrate CLI"

reset_fixture "missing base ref fails closed" bound
expect 1 "not found in this repository" origin/main

if [ "$FAILURES" -ne 0 ]; then
  echo ""
  echo "$FAILURES migration lock_timeout gate self-test(s) failed."
  exit 1
fi

echo ""
echo "OK: migration lock_timeout gate self-tests passed."
