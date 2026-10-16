#!/usr/bin/env bash
# scripts/check_schema_assertion_types_test.sh
#
# Self-test for scripts/check_schema_assertion_types.sh. Builds a throwaway git
# repository in a temp dir, replays the migration and schema-assertion shapes
# the gate has to get right, and asserts the exit code and the actionable part
# of the message.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/check_schema_assertion_types_test.sh
#
# It touches nothing outside its own mktemp -d and needs no network.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/check_schema_assertion_types.sh"
REAL_ASSERTION="$SCRIPT_DIR/../backend/db/schema_assertion.go"

FIXTURE="$(mktemp -d)" || exit 1
trap 'rm -rf "$FIXTURE"' EXIT

FAILURES=0
CASE=""
BASE_REF=""
OUT=""
CODE=0

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

# assertion <go-lines>: a schema_assertion.go whose requiredSchemaColumns holds
# the given lines, committed.
assertion() {
  cat >"$FIXTURE/backend/db/schema_assertion.go" <<EOF
package db

var requiredSchemaColumns = []requiredColumn{
	{Table: "user_bookmarks", Column: "scene_digest_sent_at"},
$1
}
EOF
  g add -A
  g commit -m "assertion"
}

# Reset the fixture to: main holding a schema assertion with one typed entry
# and one merged timestamp-named type change without an entry (merged files
# are never re-checked), with a fresh branch checked out on top. The gate's
# base ref is main.
reset_fixture() {
  CASE="$1"
  BASE_REF=main
  rm -rf "$FIXTURE"
  mkdir -p "$FIXTURE/scripts" "$FIXTURE/backend/db/migrations"
  cp "$UNDER_TEST" "$FIXTURE/scripts/check_schema_assertion_types.sh"
  cp -R "$SCRIPT_DIR/lib" "$FIXTURE/scripts/lib"
  g init -b main
  g config user.email test@example.com
  g config user.name "Schema Assertion Types Gate Test"
  assertion '	{Table: "audit_logs", Column: "actor_id", Type: sqlBigint},'
  migration "20260801000000_merged_widen" "ALTER TABLE t ALTER COLUMN merged TYPE BIGINT;"
  g checkout -b feature
}

# run_gate: run the gate against $BASE_REF, leaving its output (stderr folded
# into stdout) in OUT and its exit code in CODE.
run_gate() {
  OUT="$(bash "$FIXTURE/scripts/check_schema_assertion_types.sh" "$BASE_REF" 2>&1)"
  CODE=$?
}

# fail <reason>: count a failed assertion and show the gate's output.
fail() {
  echo "FAIL [$CASE]: $1"
  printf '%s\n' "$OUT" | sed 's/^/    /'
  FAILURES=$((FAILURES + 1))
}

# expect <code> [needle...]: run the gate once and assert its exit code and
# that its output contains every needle.
expect() {
  local want_code="$1" needle
  shift
  run_gate
  if [ "$CODE" -ne "$want_code" ]; then
    fail "exit $CODE, wanted $want_code"
    return
  fi
  for needle in "$@"; do
    if [[ $OUT != *"$needle"* ]]; then
      fail "output missing expected text: $needle"
      return
    fi
  done
  echo "ok   [$CASE]"
}

# expect_absent <needle>: assert the last gate run's output lacks the needle.
expect_absent() {
  if [[ $OUT == *"$1"* ]]; then
    fail "output has unexpected text: $1"
    return
  fi
  echo "ok   [$CASE: no '$1']"
}

# --- outcomes ----------------------------------------------------------------
reset_fixture "no new migrations passes"
expect 0 "no new timestamp-named up migrations added"

reset_fixture "merged type change is not re-checked"
migration "20260901000000_select_only" "SELECT 1;"
expect 0 "no new timestamp-named migration changes a column type"

reset_fixture "type change with a typed entry passes"
migration "20260901000000_widen" "ALTER TABLE audit_logs ALTER COLUMN actor_id TYPE BIGINT;"
expect 0 "20260901000000_widen.up.sql: audit_logs.actor_id"

reset_fixture "type change without a typed entry fails"
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 1 "ERROR: backend/db/migrations/20260901000000_widen.up.sql changes the type of users.username, and backend/db/schema_assertion.go has no typed entry for it."

reset_fixture "failure gives the reason and the fix"
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 1 "timestamp order; the typed entry is what catches that at boot." \
  'Fix: add {Table: "<table>", Column: "<column>", Type: <type>}'

reset_fixture "entry added in the same branch passes"
assertion '	{Table: "users", Column: "username", Type: sqlBigint},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE bigint;"
expect 0 "users.username"

reset_fixture "an untyped entry for the column does not count"
assertion '	{Table: "users", Column: "username"},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 1 "changes the type of users.username"

reset_fixture "an empty-string Type does not count"
assertion '	{Table: "users", Column: "username", Type: ""},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 1 "changes the type of users.username"

reset_fixture "a typed entry inside a Go comment does not count"
assertion '	// {Table: "users", Column: "username", Type: sqlText},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 1 "changes the type of users.username"

reset_fixture "a typed entry split across lines counts"
assertion '	{
		Table:  "users",
		Column: "username",
		Type:   sqlBigint,
	},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE bigint;"
expect 0 "users.username"

reset_fixture "a string-literal Type counts"
assertion '	{Table: "users", Column: "username", Type: "bigint"},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE bigint;"
expect 0 "users.username"

reset_fixture "every added migration is read, and only timestamp-named ones"
migration "20260901000000_typed" "ALTER TABLE audit_logs ALTER COLUMN actor_id TYPE BIGINT;"
migration "20260901000001_untyped" "ALTER TABLE users ALTER COLUMN username TYPE text;"
migration "000079_sequential" "ALTER TABLE users ALTER COLUMN email TYPE text;"
expect 1 "20260901000001_untyped.up.sql changes the type of users.username"
expect_absent "changes the type of audit_logs.actor_id"
expect_absent "users.email"

reset_fixture "every missing column is named"
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;
ALTER TABLE users ALTER COLUMN email TYPE text;"
expect 1 "changes the type of users.email" "changes the type of users.username"

# --- the entry's Type matches the new type -----------------------------------
reset_fixture "bigint needs sqlBigint"
migration "20260901000000_widen" "ALTER TABLE audit_logs ALTER COLUMN actor_id TYPE bigint;"
expect 0 "audit_logs.actor_id (sqlBigint)"

reset_fixture "int8 needs sqlBigint"
migration "20260901000000_widen" "ALTER TABLE audit_logs ALTER COLUMN actor_id TYPE int8;"
expect 0 "audit_logs.actor_id (sqlBigint)"

reset_fixture "timestamptz needs sqlTimestamptz"
assertion '	{Table: "events", Column: "starts_at", Type: sqlTimestamptz},'
migration "20260901000000_zone" "ALTER TABLE events ALTER COLUMN starts_at TYPE timestamptz;"
expect 0 "events.starts_at (sqlTimestamptz)"

reset_fixture "timestamp with time zone needs sqlTimestamptz"
assertion '	{Table: "events", Column: "starts_at", Type: sqlTimestamptz},'
migration "20260901000000_zone" "ALTER TABLE events ALTER COLUMN starts_at TYPE TIMESTAMP
    WITH  TIME ZONE;"
expect 0 "events.starts_at (sqlTimestamptz)"

reset_fixture "a timestamp precision and a USING clause are allowed"
assertion '	{Table: "events", Column: "starts_at", Type: sqlTimestamptz},
	{Table: "events", Column: "ends_at", Type: sqlTimestamptz},'
migration "20260901000000_zone" "ALTER TABLE events
    ALTER COLUMN starts_at TYPE timestamptz(3) USING starts_at AT TIME ZONE 'UTC',
    ALTER COLUMN ends_at TYPE timestamp (6) with time zone;"
expect 0 "events.starts_at (sqlTimestamptz)" "events.ends_at (sqlTimestamptz)"

reset_fixture "a USING call with a comma does not run into the next action"
assertion '	{Table: "audit_logs", Column: "actor_id", Type: sqlBigint},
	{Table: "audit_logs", Column: "entity_id", Type: sqlBigint},'
migration "20260901000000_widen" "ALTER TABLE audit_logs
    ALTER COLUMN actor_id TYPE bigint USING coalesce(actor_id, 0),
    ALTER COLUMN entity_id TYPE bigint;"
expect 0 "audit_logs.actor_id (sqlBigint)" "audit_logs.entity_id (sqlBigint)"

reset_fixture "a mismatched Type fails, naming the expected constant"
assertion '	{Table: "events", Column: "starts_at", Type: sqlTimestamptz},'
migration "20260901000000_widen" "ALTER TABLE events ALTER COLUMN starts_at TYPE bigint;"
expect 1 "ERROR: backend/db/migrations/20260901000000_widen.up.sql changes the type of events.starts_at to bigint, so its typed entry in backend/db/schema_assertion.go needs Type: sqlBigint (it has Type: sqlTimestamptz)."
expect_absent "Fix: add {"

reset_fixture "timestamp without time zone is not timestamptz"
assertion '	{Table: "events", Column: "starts_at", Type: sqlTimestamptz},'
migration "20260901000000_zone" "ALTER TABLE events ALTER COLUMN starts_at TYPE timestamp(3);"
expect 1 "changes the type of events.starts_at to timestamp, a type with no sql* constant in this gate."

reset_fixture "an unknown type fails, naming the type"
assertion '	{Table: "users", Column: "username", Type: sqlText},'
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE varchar(50);"
expect 1 "ERROR: backend/db/migrations/20260901000000_widen.up.sql changes the type of users.username to varchar(50), a type with no sql* constant in this gate." \
  "Add the constant to backend/db/schema_assertion.go and its spellings to type_constant in scripts/check_schema_assertion_types.sh."
expect_absent "Fix: add {"

reset_fixture "a string-literal Type in the other spelling counts, in any case"
assertion '	{Table: "audit_logs", Column: "entity_id", Type: "INT8"},
	{Table: "events", Column: "starts_at", Type: "timestamptz"},'
migration "20260901000000_widen" "ALTER TABLE audit_logs ALTER COLUMN entity_id TYPE bigint;
ALTER TABLE events ALTER COLUMN starts_at TYPE timestamp with time zone;"
expect 0 "audit_logs.entity_id (sqlBigint)" "events.starts_at (sqlTimestamptz)"

reset_fixture "a string-literal Type of another type does not count"
assertion '	{Table: "audit_logs", Column: "entity_id", Type: "integer"},'
migration "20260901000000_widen" "ALTER TABLE audit_logs ALTER COLUMN entity_id TYPE bigint;"
expect 1 'needs Type: sqlBigint (it has Type: "integer").'

reset_fixture "missing, mismatched and unknown are each reported"
assertion '	{Table: "audit_logs", Column: "actor_id", Type: sqlBigint},
	{Table: "events", Column: "starts_at", Type: sqlBigint},
	{Table: "users", Column: "username", Type: sqlText},'
migration "20260901000000_widen" "ALTER TABLE events ALTER COLUMN starts_at TYPE timestamptz;
ALTER TABLE users ALTER COLUMN username TYPE text;
ALTER TABLE users ALTER COLUMN email TYPE text;
ALTER TABLE audit_logs ALTER COLUMN actor_id TYPE bigint;"
expect 1 "events.starts_at to timestamptz, so its typed entry" \
  "users.username to text, a type with no sql* constant" \
  "changes the type of users.email, and" \
  "Fix: add {"
expect_absent "changes the type of audit_logs.actor_id"

reset_fixture "a final statement without a semicolon is read"
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE bigint"
expect 1 "changes the type of users.username, and"

# --- scope -------------------------------------------------------------------
reset_fixture "sequential migration with a type change is ignored"
migration "000079_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 0 "no new timestamp-named up migrations added"

reset_fixture "type change only in down.sql is ignored"
printf 'SELECT 1;\n' >"$FIXTURE/backend/db/migrations/20260901000000_add.up.sql"
printf 'ALTER TABLE users ALTER COLUMN username TYPE varchar(50);\n' >"$FIXTURE/backend/db/migrations/20260901000000_add.down.sql"
g add -A && g commit -m "down only"
expect 0 "no new timestamp-named migration changes a column type"

reset_fixture "type change only inside a line comment is ignored"
migration "20260901000000_note" "-- ALTER TABLE users ALTER COLUMN username TYPE text;
SELECT 1;"
expect 0 "no new timestamp-named migration changes a column type"

reset_fixture "type change only inside a block comment is ignored"
migration "20260901000000_note" "/* ALTER TABLE users
   ALTER COLUMN username TYPE text; */
SELECT 1;"
expect 0 "no new timestamp-named migration changes a column type"

reset_fixture "a block comment does not hide the statement after it"
migration "20260901000000_widen" "/* widen */ ALTER TABLE users /* why */ ALTER COLUMN username TYPE text;"
expect 1 "changes the type of users.username"

# The comment rule is the shared one in scripts/lib/sql_strip_comments.sh,
# which check_migration_lock_timeout_test.sh covers case by case; these are
# the shapes where it differs from a non-nesting, quote-blind stripper.
reset_fixture "type change only inside a nested block comment is ignored"
migration "20260901000000_note" "/* outer /* inner */ ALTER TABLE users ALTER COLUMN username TYPE text; */
SELECT 1;"
expect 0 "no new timestamp-named migration changes a column type"

reset_fixture "comment markers inside string literals do not hide a type change"
migration "20260901000000_widen" "SELECT '/*';
ALTER TABLE users ALTER COLUMN username TYPE text;
SELECT '*/';"
expect 1 "changes the type of users.username"

reset_fixture "'--' inside a block comment does not hide what follows"
migration "20260901000000_widen" "/* note -- why */ ALTER TABLE users ALTER COLUMN username TYPE text;"
expect 1 "changes the type of users.username"

reset_fixture "other column actions are not type changes"
migration "20260901000000_alter" "ALTER TABLE users ALTER COLUMN username SET DEFAULT 'x',
  ALTER COLUMN username DROP NOT NULL, ADD COLUMN type_name text;"
expect 0 "no new timestamp-named migration changes a column type"

reset_fixture "ALTER TYPE on a composite type is not a table change"
migration "20260901000000_composite" "ALTER TYPE address ALTER ATTRIBUTE zip TYPE text;"
expect 0 "no new timestamp-named migration changes a column type"

# --- spellings ---------------------------------------------------------------
reset_fixture "multi-action ALTER TABLE finds every type change"
migration "20260901000000_widen" "ALTER TABLE audit_logs
    ADD COLUMN note text,
    ALTER COLUMN actor_id TYPE BIGINT,
    ALTER COLUMN entity_id TYPE BIGINT USING entity_id::bigint;"
expect 1 "changes the type of audit_logs.entity_id"
expect_absent "changes the type of audit_logs.actor_id"

reset_fixture "SET DATA TYPE spelling is a type change"
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username SET DATA TYPE text;"
expect 1 "changes the type of users.username"

reset_fixture "ALTER without COLUMN is a type change"
migration "20260901000000_widen" "ALTER TABLE users ALTER username TYPE text;"
expect 1 "changes the type of users.username"

reset_fixture "lowercase, multi-line, IF EXISTS ONLY, schema-qualified, quoted"
migration "20260901000000_widen" 'alter table if exists only public."Users"
    alter column "UserName"
    type text;'
expect 1 "changes the type of users.username"

reset_fixture "type change inside a DO block is found"
migration "20260901000000_widen" "DO \$\$ BEGIN
  ALTER TABLE users ALTER COLUMN username TYPE text;
END \$\$;"
expect 1 "changes the type of users.username"

reset_fixture "a typed entry matches case-insensitively"
migration "20260901000000_widen" "ALTER TABLE AUDIT_LOGS ALTER COLUMN ACTOR_ID TYPE BIGINT;"
expect 0 "audit_logs.actor_id"

# --- the repository's own schema assertion -----------------------------------
# The real file's typed entries must parse, so a reshaped list fails here
# rather than failing every future type-changing migration. Both columns are
# type changes of timestamp-named migrations in backend/db/migrations, which
# the list's invariant keeps typed.
reset_fixture "the repository's typed entries are recognised"
cp "$REAL_ASSERTION" "$FIXTURE/backend/db/schema_assertion.go"
g add -A && g commit -m "real assertion"
migration "20260901000000_widen" "ALTER TABLE webauthn_credentials ALTER COLUMN sign_count TYPE BIGINT;
ALTER TABLE entity_edit_audit_logs ALTER COLUMN created_at TYPE TIMESTAMPTZ;"
expect 0 "entity_edit_audit_logs.created_at" "webauthn_credentials.sign_count"

# --- fail closed -------------------------------------------------------------
reset_fixture "missing schema assertion file fails closed"
g rm backend/db/schema_assertion.go && g commit -m "drop assertion"
expect 1 "backend/db/schema_assertion.go not found"

reset_fixture "an added migration missing from the working tree fails closed"
migration "20260901000000_widen" "ALTER TABLE users ALTER COLUMN username TYPE text;"
rm "$FIXTURE/backend/db/migrations/20260901000000_widen.up.sql"
expect 1 "20260901000000_widen.up.sql is added on HEAD but not readable in the working tree."

reset_fixture "missing base ref fails closed"
BASE_REF=origin/main
expect 1 "not found in this repository"

reset_fixture "no base ref argument defaults to origin/main"
BASE_REF=""
expect 1 "base ref 'origin/main' not found"

if [ "$FAILURES" -ne 0 ]; then
  echo ""
  echo "$FAILURES schema assertion types gate self-test(s) failed."
  exit 1
fi

echo ""
echo "OK: schema assertion types gate self-tests passed."
