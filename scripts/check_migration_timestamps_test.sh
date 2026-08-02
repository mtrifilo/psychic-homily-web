#!/usr/bin/env bash
# scripts/check_migration_timestamps_test.sh
#
# Self-test for scripts/check_migration_timestamps.sh. Builds a throwaway git
# repository in a temp dir, replays the shapes the gate has to get right, and
# asserts both the exit code and the actionable part of the message.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/check_migration_timestamps_test.sh
#
# It touches nothing outside its own mktemp -d.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/check_migration_timestamps.sh"

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

FAILURES=0
CASE=""

# Fixture git. Commit signing and a global core.hooksPath are neutralised so
# the self-test behaves the same on a CI runner and on a laptop that has either
# turned on globally.
g() {
  git -C "$FIXTURE" -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@" >/dev/null 2>&1
}

add_migration() {
  # add_migration <version>_<name>
  local slug="$1"
  printf 'SELECT 1;\n' >"$FIXTURE/backend/db/migrations/${slug}.up.sql"
  printf 'SELECT 1;\n' >"$FIXTURE/backend/db/migrations/${slug}.down.sql"
}

# Assert the gate's exit code, and (optionally) that its output contains a
# needle. Output is captured with stderr folded into stdout.
expect() {
  local want_code="$1" needle="${2:-}"
  local out code
  out="$(bash "$FIXTURE/scripts/check_migration_timestamps.sh" main 2>&1)"
  code=$?
  if [ "$code" -ne "$want_code" ]; then
    echo "FAIL [$CASE]: exit $code, wanted $want_code"
    printf '%s\n' "$out" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
    return
  fi
  if [ -n "$needle" ] && ! printf '%s' "$out" | grep -qF "$needle"; then
    echo "FAIL [$CASE]: output missing expected text: $needle"
    printf '%s\n' "$out" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
    return
  fi
  echo "ok   [$CASE]"
}

# Reset the fixture to: main holding one legacy sequential migration plus one
# merged timestamp migration, with a fresh branch checked out on top.
# Pass --legacy-only as a second argument to leave the timestamp migration off
# main, which is the "nothing to compare against" fail-closed case.
reset_fixture() {
  CASE="$1"
  rm -rf "$FIXTURE"
  mkdir -p "$FIXTURE/scripts" "$FIXTURE/backend/db/migrations"
  cp "$UNDER_TEST" "$FIXTURE/scripts/check_migration_timestamps.sh"
  g init -b main
  g config user.email test@example.com
  g config user.name "Migration Gate Test"
  add_migration "000078_legacy_sequential"
  if [ "${2:-}" != "--legacy-only" ]; then
    add_migration "20260801000000_merged_baseline"
  fi
  g add -A
  g commit -m baseline
  g checkout -b feature
}

reset_fixture "no new migrations passes"
printf 'unrelated\n' >"$FIXTURE/README.md"
g add -A && g commit -m "no migrations"
expect 0 "no new migration files added"

reset_fixture "forward-stamped migration passes"
add_migration "20260901000000_add_column"
g add -A && g commit -m "forward stamp"
expect 0 "stamped ahead of main"

reset_fixture "backdated migration fails"
add_migration "20260101000000_add_show_times"
g add -A && g commit -m "backdated"
expect 1 "parallel-authored migrations must be stamped in merge order"

reset_fixture "backdated migration names the offending file"
add_migration "20260101000000_add_show_times"
g add -A && g commit -m "backdated"
expect 1 "backend/db/migrations/20260101000000_add_show_times.up.sql (stamped 20260101000000)"

reset_fixture "migration equal to merged max fails"
printf 'SELECT 2;\n' >"$FIXTURE/backend/db/migrations/20260801000000_same_stamp_other_name.up.sql"
printf 'SELECT 2;\n' >"$FIXTURE/backend/db/migrations/20260801000000_same_stamp_other_name.down.sql"
g add -A && g commit -m "equal stamp"
expect 1 "must be stamped in merge order"

reset_fixture "new sequential-format migration fails"
add_migration "000079_add_column"
g add -A && g commit -m "sequential"
expect 1 "do not use the YYYYMMDDhhmmss timestamp format"

reset_fixture "backdated rename of a merged migration fails"
g mv backend/db/migrations/20260801000000_merged_baseline.up.sql \
     backend/db/migrations/20260101000000_merged_baseline.up.sql
g mv backend/db/migrations/20260801000000_merged_baseline.down.sql \
     backend/db/migrations/20260101000000_merged_baseline.down.sql
g add -A && g commit -m "rename backwards"
expect 1 "must be stamped in merge order"

reset_fixture "merge commit inside the branch does not confuse the diff"
g checkout main
add_migration "20260903000000_sibling_merged"
g add -A && g commit -m "sibling merged"
g checkout feature
g merge main -m "merge main into feature"
add_migration "20260904000000_branch_work"
g add -A && g commit -m "branch work re-stamped after the merge"
expect 0 "stamped ahead of main"

reset_fixture "branch behind a newer sibling migration fails"
add_migration "20260802000000_our_work"
g add -A && g commit -m "our work"
g checkout main
add_migration "20260803000000_sibling_merged"
g add -A && g commit -m "sibling merged"
g checkout feature
expect 1 "must be stamped in merge order"

reset_fixture "non-sql file in the migrations dir is ignored"
printf 'notes\n' >"$FIXTURE/backend/db/migrations/README.md"
g add -A && g commit -m "docs"
expect 0 "no new migration files added"

# Fail-closed cases: when the gate cannot see what it is comparing against it
# must go RED, never quietly green.
reset_fixture "nothing to compare against fails closed" --legacy-only
add_migration "20260901000000_add_column"
g add -A && g commit -m "forward stamp on an unusable base"
expect 1 "no timestamp-format migrations found"

CASE="missing base ref fails closed"
missing_out="$(bash "$FIXTURE/scripts/check_migration_timestamps.sh" origin/main 2>&1)"
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
  echo "$FAILURES migration-timestamp gate self-test(s) failed."
  exit 1
fi

echo ""
echo "OK: migration-timestamp gate self-tests passed."
