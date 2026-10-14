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
# ./main is what the entrypoint execs last.
printf '#!/bin/sh\nexit 0\n' >"$STUB_BIN/nc"
printf '#!/bin/sh\necho "stub migrate ran"\nexit 0\n' >"$STUB_BIN/migrate"
printf '#!/bin/sh\necho "stub main ran"\nexit 0\n' >"$WORK/main"
chmod +x "$STUB_BIN/nc" "$STUB_BIN/migrate" "$WORK/main"

FAILURES=0
CASE=""
OUTPUT=""
STATUS=0

# run [DATABASE_URL] runs the entrypoint with only PATH and that variable set;
# with no argument, DATABASE_URL is absent from the environment.
run() {
  OUTPUT="$(cd "$WORK" && env -i PATH="$STUB_BIN:$PATH" ${1+"DATABASE_URL=$1"} \
    "$ENTRYPOINT_SHELL" "$UNDER_TEST" 2>&1)"
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

# ---------------------------------------------------------------- result

if [ "$FAILURES" -gt 0 ]; then
  echo "check_entrypoint_redaction_test: $FAILURES failure(s)"
  exit 1
fi
echo "check_entrypoint_redaction_test: all cases passed ($ENTRYPOINT_SHELL)"
