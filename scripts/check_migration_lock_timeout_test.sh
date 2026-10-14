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

FIXTURE="$(mktemp -d)" || exit 1
[ -d "$FIXTURE" ] || exit 1
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
entrypoint() {
  local f="$FIXTURE/backend/docker-entrypoint.sh"
  case "$1" in
    bound)
      cat >"$f" <<'EOF'
#!/bin/sh
MIGRATE_LOCK_TIMEOUT_OPTION='options=-c%20lock_timeout%3D5s'
MIGRATE_DATABASE_URL="${DATABASE_URL}?${MIGRATE_LOCK_TIMEOUT_OPTION}"
migrate -path /app/db/migrations -database "$MIGRATE_DATABASE_URL" up
EOF
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

# Assert the gate's exit code, and (optionally) that its output contains a
# needle. Output is captured with stderr folded into stdout.
expect() {
  local want_code="$1" needle="${2:-}"
  local out code
  out="$(bash "$FIXTURE/scripts/check_migration_lock_timeout.sh" main 2>&1)"
  code=$?
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
expect 1 "takes a strong lock: LOCK TABLE"

reset_fixture "unbound: DROP TABLE fails" unbound
migration "20260901000000_drop" "DROP TABLE IF EXISTS old_things;"
expect 1 "takes a strong lock: DROP TABLE"

reset_fixture "unbound: plain CREATE INDEX fails" unbound
migration "20260901000000_idx" "CREATE INDEX IF NOT EXISTS idx_t_c ON t (c);"
expect 1 "CREATE INDEX without CONCURRENTLY"

reset_fixture "unbound: CREATE UNIQUE INDEX on a quoted name fails" unbound
migration "20260901000000_idx" 'CREATE UNIQUE INDEX "Idx_T_C" ON t (c);'
expect 1 "CREATE INDEX without CONCURRENTLY"

reset_fixture "unbound: CREATE INDEX CONCURRENTLY passes" unbound
migration "20260901000000_idx" "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_c ON t (c);"
expect 0 "every new strong-lock migration sets its own lock_timeout"

reset_fixture "unbound: lowercase, multi-line ALTER TABLE fails" unbound
migration "20260901000000_widen" "alter
    table t alter column c type bigint;"
expect 1 "takes a strong lock: ALTER TABLE"

reset_fixture "unbound: every kind in one file is listed" unbound
migration "20260901000000_many" "ALTER TABLE t ADD COLUMN d int;
DROP TABLE u;
CREATE INDEX idx_t_d ON t (d);"
expect 1 "takes a strong lock: ALTER TABLE, DROP TABLE, CREATE INDEX without CONCURRENTLY"

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

# --- runner-level bound present: the same migrations only warn ----------------
reset_fixture "bound: ALTER TABLE without lock_timeout warns and passes" bound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 0 "WARNING: backend/db/migrations/20260901000000_widen.up.sql takes a strong lock and sets no lock_timeout"

reset_fixture "bound: warning mode says so" bound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 0 "OK (warnings only): the runner-level lock_timeout"

reset_fixture "bound: GitHub Actions gets a file annotation" bound
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
CASE="bound: GitHub Actions gets a file annotation"
gha_out="$(GITHUB_ACTIONS=true bash "$FIXTURE/scripts/check_migration_lock_timeout.sh" main 2>&1)"
gha_code=$?
if [ "$gha_code" -eq 0 ] && printf '%s' "$gha_out" | grep -qF "::warning file=backend/db/migrations/20260901000000_widen.up.sql::"; then
  echo "ok   [$CASE]"
else
  echo "FAIL [$CASE]: exit $gha_code"
  printf '%s\n' "$gha_out" | sed 's/^/    /'
  FAILURES=$((FAILURES + 1))
fi

# --- the runner check itself must not be fooled ------------------------------
reset_fixture "commented-out runner option is strict mode" commented
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "does not pass one to the migrate CLI"

reset_fixture "a migrate call on the bare DATABASE_URL is strict mode" bare-url
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "does not pass one to the migrate CLI"

reset_fixture "missing entrypoint is strict mode" bound
g rm backend/docker-entrypoint.sh && g commit -m "drop entrypoint"
migration "20260901000000_widen" "ALTER TABLE t ALTER COLUMN c TYPE BIGINT;"
expect 1 "does not pass one to the migrate CLI"

CASE="missing base ref fails closed"
missing_out="$(bash "$FIXTURE/scripts/check_migration_lock_timeout.sh" origin/main 2>&1)"
missing_code=$?
if [ "$missing_code" -eq 1 ] && printf '%s' "$missing_out" | grep -qF "not found in this repository"; then
  echo "ok   [$CASE]"
else
  echo "FAIL [$CASE]: exit $missing_code"
  printf '%s\n' "$missing_out" | sed 's/^/    /'
  FAILURES=$((FAILURES + 1))
fi

if [ "$FAILURES" -ne 0 ]; then
  echo ""
  echo "$FAILURES migration lock_timeout gate self-test(s) failed."
  exit 1
fi

echo ""
echo "OK: migration lock_timeout gate self-tests passed."
