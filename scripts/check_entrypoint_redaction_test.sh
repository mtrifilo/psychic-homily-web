#!/usr/bin/env bash
# scripts/check_entrypoint_redaction_test.sh
#
# Self-test for the DATABASE_URL boot-log line in backend/docker-entrypoint.sh.
# Runs the real entrypoint under `sh` (or the shell named by $1) in a temp
# dir, with stub `nc`, `migrate` and `./main` so no database is needed, once per
# fixture URL. Each case asserts the exact DATABASE_URL line and that no part of
# the fixture's user or password appears ANYWHERE in the boot output, so a new
# line that prints the URL fails here too.
#
# The migrate-options cases at the end assert the URL the stub migrate
# received and the boot lines for MIGRATE_LOCK_TIMEOUT and
# MIGRATE_STATEMENT_TIMEOUT: defaults, valid overrides, invalid overrides.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/check_entrypoint_redaction_test.sh [shell, default sh]
#
# It touches nothing outside its own mktemp -d and needs no network.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/../backend/docker-entrypoint.sh"
ENTRYPOINT_SHELL="${1:-sh}"

FIXTURE="$(mktemp -d)" || exit 1
[ -d "$FIXTURE" ] || exit 1
trap 'rm -rf "$FIXTURE"' EXIT

STUB_BIN="$FIXTURE/bin"
WORK="$FIXTURE/work"
mkdir -p "$STUB_BIN" "$WORK"

# The database wait and the migration succeed without touching a network;
# ./main is what the entrypoint execs last. The stub migrate writes its
# arguments, one per line, to MIGRATE_ARGS rather than to the boot output.
MIGRATE_ARGS="$FIXTURE/migrate-args"
printf '#!/bin/sh\nexit 0\n' >"$STUB_BIN/nc"
printf '#!/bin/sh\nprintf "%%s\\n" "$@" >"%s"\necho "stub migrate ran"\nexit 0\n' \
  "$MIGRATE_ARGS" >"$STUB_BIN/migrate"
printf '#!/bin/sh\necho "stub main ran"\nexit 0\n' >"$WORK/main"
chmod +x "$STUB_BIN/nc" "$STUB_BIN/migrate" "$WORK/main"

FAILURES=0
CASE=""
OUTPUT=""
STATUS=0

# run [DATABASE_URL [NAME=value...]] runs the entrypoint with only PATH, that
# variable and the given assignments set; with no argument, DATABASE_URL is
# absent from the environment.
run() {
  rm -f "$MIGRATE_ARGS"
  OUTPUT="$(cd "$WORK" && env -i PATH="$STUB_BIN:$PATH" ${1+"DATABASE_URL=$1"} \
    "${@:2}" "$ENTRYPOINT_SHELL" "$UNDER_TEST" 2>&1)"
  STATUS=$?
}

fail() {
  FAILURES=$((FAILURES + 1))
  echo "FAIL [$CASE]: $1"
  echo "  output was:"
  printf '%s\n' "$OUTPUT" | sed 's/^/    /'
}

expect_status() {
  [ "$STATUS" -eq "$1" ] || fail "expected exit $1, got $STATUS"
}

expect_line() {
  grep -qxF -- "$1" <<<"$OUTPUT" || fail "expected the line '$1'"
}

expect_absent() {
  local secret
  for secret in "$@"; do
    if grep -qF -- "$secret" <<<"$OUTPUT"; then
      fail "output contains '$secret'"
    fi
  done
}

expect_booted() {
  expect_status 0
  expect_line "Migrations completed successfully!"
  expect_line "stub main ran"
}

# expect_migrate_url <url>: the stub migrate got exactly this -database value.
expect_migrate_url() {
  local got
  got="$(sed -n '/^-database$/{n;p;q;}' "$MIGRATE_ARGS" 2>/dev/null)"
  [ "$got" = "$1" ] || fail "migrate got -database '$got', expected '$1'"
}

# ---------------------------------------------------------------- cases

CASE="Railway-shaped URL keeps scheme, host, port and database"
run "postgresql://fixtureuser7:Zq9fixturePW7731@postgres.railway.internal:5432/railway"
expect_booted
expect_line "DATABASE_URL: postgresql://***@postgres.railway.internal:5432/railway"
expect_absent "fixtureuser7" "Zq9fixturePW7731" "Zq9"

CASE="query string is dropped (it can carry password=)"
run "postgres://fixtureuser7:Zq9fixturePW7731@localhost:5432/appdb?sslmode=disable&password=QryFixtureSecret"
expect_booted
expect_line "DATABASE_URL: postgres://***@localhost:5432/appdb"
expect_absent "fixtureuser7" "Zq9fixturePW7731" "QryFixtureSecret" "sslmode"

CASE="password with a literal '@' and '/' stays hidden"
run "postgres://fixtureuser7:AtPart@SlashPart/TailPart@db.example.test:5432/appdb"
expect_booted
expect_line "DATABASE_URL: postgres://***@db.example.test:5432/appdb"
expect_absent "fixtureuser7" "AtPart" "SlashPart" "TailPart"

CASE="password with a literal ':' stays hidden"
run "postgres://fixtureuser7:Colon:Fixture:7731@db.example.test/appdb"
expect_booted
expect_line "DATABASE_URL: postgres://***@db.example.test/appdb"
expect_absent "fixtureuser7" "Colon" "Fixture:7731"

CASE="fragment is dropped"
run "postgres://fixtureuser7:Zq9fixturePW7731@db.example.test:5432/appdb#FragFixture"
expect_booted
expect_line "DATABASE_URL: postgres://***@db.example.test:5432/appdb"
expect_absent "fixtureuser7" "Zq9fixturePW7731" "FragFixture"

CASE="URL without userinfo prints without ***@"
run "postgres://localhost:5432/appdb"
expect_booted
expect_line "DATABASE_URL: postgres://localhost:5432/appdb"

CASE="'?' in the password is ambiguous with a query: not shown"
run "postgres://fixtureuser7:Qm?FixturePW@db.example.test:5432/appdb"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Qm?" "FixturePW"

CASE="'#' in the password is ambiguous with a fragment: not shown"
run "postgres://fixtureuser7:Hs#FixturePW@db.example.test:5432/appdb"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Hs#" "FixturePW"

CASE="empty host part: not shown"
run "postgres://fixtureuser7:Zq9fixturePW7731@"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Zq9fixturePW7731"

CASE="empty scheme: not shown"
run "://fixtureuser7:Zq9fixturePW7731@db.example.test:5432/appdb"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Zq9fixturePW7731"

# Railway-shaped on purpose: the non-Railway branch's DB_HOST extraction is a
# separate parser that this case does not exercise.
CASE="'@' inside a query value is ambiguous with userinfo: not shown"
run "postgresql://fixtureuser7:Zq9fixturePW7731@db.railway.internal:5432/railway?password=QryHead@QryTail"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Zq9fixturePW7731" "QryHead" "QryTail"

CASE="host part with characters outside the allowed set: not shown"
run "postgres://fixtureuser7:Zq9fixturePW7731@db.example.test:5432/app%20db"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Zq9fixturePW7731"

CASE="no scheme, but a '://' later in the query: not shown"
run "fixtureuser7:Zq9fixturePW7731@db.example.test:5432/appdb?next=https://elsewhere.test"
expect_booted
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "Zq9fixturePW7731"

CASE="keyword/value DSN (no scheme): not shown"
run "host=db.example.test user=fixtureuser7 password=KvFixtureSecret dbname=appdb"
expect_line "DATABASE_URL: set (not shown)"
expect_absent "fixtureuser7" "KvFixtureSecret"

CASE="empty DATABASE_URL: NOT SET, boot refused"
run ""
expect_status 1
expect_line "DATABASE_URL: NOT SET"
expect_line "ERROR: DATABASE_URL is not set!"

CASE="unset DATABASE_URL: NOT SET, boot refused"
run
expect_status 1
expect_line "DATABASE_URL: NOT SET"
expect_line "ERROR: DATABASE_URL is not set!"

# ------------------------------------------------- migrate options cases

OPTS_URL="postgresql://fixtureuser7:Zq9fixturePW7731@postgres.railway.internal:5432/railway"
LOCK_5S="options=-c%20lock_timeout%3D5s"
NO_STATEMENT_TIMEOUT_LINE="Migrations get no statement_timeout from this script (MIGRATE_STATEMENT_TIMEOUT=0)."

CASE="defaults: 5s lock_timeout, no statement_timeout"
run "$OPTS_URL"
expect_booted
expect_absent "WARNING"
expect_line "Migrations wait at most 5s for any lock (lock_timeout)."
expect_line "$NO_STATEMENT_TIMEOUT_LINE"
expect_migrate_url "$OPTS_URL?$LOCK_5S"

CASE="empty overrides read as the defaults"
run "$OPTS_URL" MIGRATE_LOCK_TIMEOUT= MIGRATE_STATEMENT_TIMEOUT=
expect_booted
expect_absent "WARNING"
expect_line "Migrations wait at most 5s for any lock (lock_timeout)."
expect_line "$NO_STATEMENT_TIMEOUT_LINE"
expect_migrate_url "$OPTS_URL?$LOCK_5S"

CASE="valid overrides with units, joined after an existing query"
run "$OPTS_URL?sslmode=disable" MIGRATE_LOCK_TIMEOUT=30s MIGRATE_STATEMENT_TIMEOUT=10min
expect_booted
expect_absent "WARNING"
expect_line "Migrations wait at most 30s for any lock (lock_timeout)."
expect_line "Each migration statement runs at most 10min (statement_timeout)."
expect_migrate_url "$OPTS_URL?sslmode=disable&options=-c%20lock_timeout%3D30s%20-c%20statement_timeout%3D10min"

CASE="valid overrides as bare milliseconds name the unit"
run "$OPTS_URL" MIGRATE_LOCK_TIMEOUT=2500 MIGRATE_STATEMENT_TIMEOUT=60000
expect_booted
expect_absent "WARNING"
expect_line "Migrations wait at most 2500ms for any lock (lock_timeout)."
expect_line "Each migration statement runs at most 60000ms (statement_timeout)."
expect_migrate_url "$OPTS_URL?options=-c%20lock_timeout%3D2500ms%20-c%20statement_timeout%3D60000ms"

for unit in ms s min h d; do
  CASE="lock_timeout unit $unit is accepted"
  run "$OPTS_URL" MIGRATE_LOCK_TIMEOUT="7$unit"
  expect_booted
  expect_absent "WARNING"
  expect_migrate_url "$OPTS_URL?options=-c%20lock_timeout%3D7$unit"
done

CASE="statement_timeout alone keeps the default lock_timeout"
run "$OPTS_URL" MIGRATE_STATEMENT_TIMEOUT=30s
expect_booted
expect_absent "WARNING"
expect_line "Migrations wait at most 5s for any lock (lock_timeout)."
expect_line "Each migration statement runs at most 30s (statement_timeout)."
expect_migrate_url "$OPTS_URL?$LOCK_5S%20-c%20statement_timeout%3D30s"

for unit in ms s min h d; do
  CASE="statement_timeout unit $unit is accepted"
  run "$OPTS_URL" MIGRATE_STATEMENT_TIMEOUT="7$unit"
  expect_booted
  expect_absent "WARNING"
  expect_migrate_url "$OPTS_URL?$LOCK_5S%20-c%20statement_timeout%3D7$unit"
done

CASE="statement_timeout of 0s adds nothing"
run "$OPTS_URL" MIGRATE_STATEMENT_TIMEOUT=0s
expect_booted
expect_absent "WARNING"
expect_line "$NO_STATEMENT_TIMEOUT_LINE"
expect_migrate_url "$OPTS_URL?$LOCK_5S"

for bad in abc 5S "5 s" -5s 1.5s 5sec s 5s5 0 0ms 000s 1us 500us "5s&options=-c%20x" "5s%20-c%20lock_timeout%3D0"; do
  CASE="invalid MIGRATE_LOCK_TIMEOUT '$bad' falls back to 5s"
  run "$OPTS_URL" MIGRATE_LOCK_TIMEOUT="$bad"
  expect_booted
  expect_line "WARNING: MIGRATE_LOCK_TIMEOUT is not a positive whole number with an optional unit (ms, s, min, h, d); using 5s."
  expect_line "Migrations wait at most 5s for any lock (lock_timeout)."
  expect_migrate_url "$OPTS_URL?$LOCK_5S"
done

for bad in abc -1 30S 1.5s 1us "30s%20-c%20lock_timeout%3D0"; do
  CASE="invalid MIGRATE_STATEMENT_TIMEOUT '$bad' falls back to none"
  run "$OPTS_URL" MIGRATE_STATEMENT_TIMEOUT="$bad"
  expect_booted
  expect_line "WARNING: MIGRATE_STATEMENT_TIMEOUT is not a whole number with an optional unit (ms, s, min, h, d); using 0 (not set)."
  expect_line "$NO_STATEMENT_TIMEOUT_LINE"
  expect_migrate_url "$OPTS_URL?$LOCK_5S"
done

CASE="an options parameter in DATABASE_URL is reported as shadowing both"
run "$OPTS_URL?options=-c%20work_mem%3D8MB" MIGRATE_LOCK_TIMEOUT=30s MIGRATE_STATEMENT_TIMEOUT=30s
expect_booted
expect_line "WARNING: DATABASE_URL carries its own 'options' parameter; migrations run WITHOUT the 30s lock_timeout or any statement_timeout from MIGRATE_STATEMENT_TIMEOUT."
expect_absent "Migrations wait at most" "Each migration statement runs at most"
expect_migrate_url "$OPTS_URL?options=-c%20work_mem%3D8MB&options=-c%20lock_timeout%3D30s%20-c%20statement_timeout%3D30s"

# ---------------------------------------------------------------- result

if [ "$FAILURES" -gt 0 ]; then
  echo "check_entrypoint_redaction_test: $FAILURES failure(s)"
  exit 1
fi
echo "check_entrypoint_redaction_test: all cases passed ($ENTRYPOINT_SHELL)"
