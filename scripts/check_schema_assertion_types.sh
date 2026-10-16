#!/usr/bin/env bash
# scripts/check_schema_assertion_types.sh
#
# CI gate: a timestamp-named up migration this branch ADDS that changes a
# column's type must come with a typed entry for that column in
# backend/db/schema_assertion.go:
#
#   {Table: "<table>", Column: "<column>", Type: <type>}
#
# golang-migrate skips a timestamp-named migration whose version is below the
# recorded schema_migrations pointer, which happens when branches merge out of
# timestamp order, and it reports success. The typed entry is what turns that
# skipped type change into a boot refusal instead of a wrong column type.
#
# Scope:
#   * Timestamp-named migrations only (YYYYMMDDhhmmss_, the format
#     check_migration_timestamps.sh enforces). Sequentially numbered migrations
#     (000NNN_) cannot be skipped that way, so their type changes are ignored.
#   * A type change is `ALTER TABLE [IF EXISTS] [ONLY] <table> ... ALTER
#     [COLUMN] <column> [SET DATA] TYPE <type>`, any number of them per
#     statement, anywhere in the file (a DO block included). A schema prefix on
#     the table is dropped; identifiers compare case-insensitively with double
#     quotes removed.
#   * Comments are stripped first, by the rule in
#     scripts/lib/sql_strip_comments.sh: `--` to the end of the line and
#     nested `/* */` blocks, neither inside a string literal.
#   * The entry's Type must be the sql* constant for the migration's new type;
#     type_constant below maps each type the gate knows to its constant, and a
#     type it does not know fails the gate, naming the type. A string-literal
#     Type counts when it is a spelling of that constant. Entries match
#     anywhere in the file, outside `//` comments, in any whitespace layout,
#     with the fields in the order Table, Column, Type.
#   * Only the up migrations the branch adds are read, so merged migrations are
#     never re-checked and no warning mode is needed for them.
#
# Usage:
#   scripts/check_schema_assertion_types.sh [base-ref]
#
# base-ref defaults to origin/main. The CALLER makes that ref current; this
# script does no network I/O. The "added" set is the same two-dot
# --diff-filter=A --no-renames diff check_migration_timestamps.sh uses.

set -euo pipefail
# No globbing: filenames flow through unquoted word-splitting below.
set -f

BASE_REF="${1:-origin/main}"
MIGRATIONS_PATH="backend/db/migrations"
ASSERTION_FILE="backend/db/schema_assertion.go"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# shellcheck source=scripts/lib/sql_strip_comments.sh
. "$SCRIPT_DIR/lib/sql_strip_comments.sh"

if ! git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null; then
  echo "ERROR: base ref '$BASE_REF' not found in this repository." >&2
  echo "       Fetch it first, e.g. 'git fetch origin main'." >&2
  exit 1
fi

if [ ! -f "$ASSERTION_FILE" ]; then
  echo "ERROR: $ASSERTION_FILE not found; this gate reads its typed entries." >&2
  echo "       If the list moved, point ASSERTION_FILE in $0 at it." >&2
  exit 1
fi

# --- 1. timestamp-named up migrations this branch ADDS ------------------------
if ! ADDED_UP="$(git diff --name-only --diff-filter=A --no-renames \
                   "$BASE_REF" HEAD -- "$MIGRATIONS_PATH/*.up.sql")"; then
  echo "ERROR: could not diff HEAD against $BASE_REF." >&2
  exit 1
fi

ADDED_TIMESTAMPED="$(grep -E '(^|/)[0-9]{14}_[^/]*$' <<<"$ADDED_UP" || true)"

if [ -z "$ADDED_TIMESTAMPED" ]; then
  echo "OK: no new timestamp-named up migrations added against $BASE_REF."
  exit 0
fi

# --- 2. typed entries in the schema assertion ---------------------------------
# One "table column Type" line per entry whose Type is set (an identifier, or a
# non-empty string literal with its quotes), table and column lowercased. The
# file is folded to one line so an entry split across lines still matches.
TYPED_ENTRIES="$(sed 's|//.*$||' "$ASSERTION_FILE" \
  | tr '\n\t' '  ' \
  | grep -oE '\{[[:space:]]*Table:[[:space:]]*"[^"]+"[[:space:]]*,[[:space:]]*Column:[[:space:]]*"[^"]+"[[:space:]]*,[[:space:]]*Type:[[:space:]]*([A-Za-z_][A-Za-z0-9_]*|"[^"]+")' \
  | sed -E 's/.*Table:[[:space:]]*"([^"]+)".*Column:[[:space:]]*"([^"]+)".*Type:[[:space:]]*/\1 \2 /' \
  | awk '{ type = $0; sub(/^[^ ]* [^ ]* /, "", type); print tolower($1), tolower($2), type }' \
  | sort -u || true)"

# entry_types <table> <column>: the Type of each typed entry for the column,
# one per line.
entry_types() {
  awk -v table="$1" -v column="$2" \
    '$1 == table && $2 == column { sub(/^[^ ]* [^ ]* /, ""); print }' <<<"$TYPED_ENTRIES"
}

# type_constant <type>: the sql* constant in $ASSERTION_FILE that a column of
# the lowercase SQL type <type> satisfies at boot, or nothing when this gate
# has no mapping for the type. Each type is listed in the two spellings
# information_schema reports (data_type, then udt_name), which are the ones
# the boot check accepts. To allow a new type, add its constant to
# $ASSERTION_FILE and its spellings here.
type_constant() {
  case "$1" in
    bigint | int8) echo sqlBigint ;;
    "timestamp with time zone" | timestamptz) echo sqlTimestamptz ;;
  esac
}

# names_constant <Type> <constant>: whether an entry's Type (an identifier, or
# a string literal with its quotes) is the constant, or a literal in one of
# the constant's spellings, in any case (the boot check ignores case).
names_constant() {
  local literal
  case "$1" in
    \"*\")
      literal="${1#\"}"
      literal="${literal%\"}"
      [ "$(type_constant "$(tr '[:upper:]' '[:lower:]' <<<"$literal")")" = "$2" ]
      ;;
    *) [ "$1" = "$2" ] ;;
  esac
}

# --- 3. column type changes in each added migration ---------------------------
# Prints file $1 with its comments stripped (lib/sql_strip_comments.sh),
# lowercase and without double quotes, one statement per line.
statements() {
  sql_strip_comments "$1" \
    | tr '\t' ' ' \
    | tr -d '"' \
    | tr '[:upper:]' '[:lower:]' \
    | tr ';' '\n'
}

# Prints "table column type" for every column type change in file $1, once
# each. The type is the new type with any USING or COLLATE clause and any
# timestamp precision dropped (information_schema reports neither), and its
# whitespace collapsed.
type_changes() {
  local name='[a-z_][a-z0-9_$]*' sp='[[:space:]]+' table actions
  # One "<table> <actions>" line per statement with an ALTER TABLE head.
  statements "$1" \
    | sed -nE "s/.*alter${sp}table${sp}(if${sp}exists${sp})?(only${sp})?($name\\.)?($name)(.*)/\\4 \\5/p" \
    | while read -r table actions; do
        # Each action starts after a space or a comma; the padding gives the
        # first and last action that boundary. A type runs to the next comma
        # outside parentheses, so numeric(10,2) or a USING call stays whole.
        grep -oE "[[:space:],]alter${sp}(column${sp})?$name${sp}(set${sp}data${sp})?type${sp}([^,(]|\\([^)]*\\))*" <<<" $actions " \
          | sed -E "s/^[[:space:],]alter${sp}(column${sp})?($name)${sp}(set${sp}data${sp})?type${sp}/$table \\2 /" || true
      done \
    | sed -E 's/[[:space:]](using|collate)[[:space:]].*$//; s/[[:space:]]+/ /g; s/ $//' \
    | sed -E 's/ (timestamptz|timestamp) ?\( ?[0-6] ?\)( with time zone)?$/ \1\2/' \
    | sort -u
}

CHECKED=""
MISSING=0
WRONG_TYPE=0
for f in $ADDED_TIMESTAMPED; do
  if [ ! -r "$f" ]; then
    echo "ERROR: $f is added on HEAD but not readable in the working tree." >&2
    exit 1
  fi
  changes="$(type_changes "$f")"
  while read -r table column new_type; do
    [ -n "$table" ] || continue
    types="$(entry_types "$table" "$column")"
    if [ -z "$types" ]; then
      MISSING=$((MISSING + 1))
      echo "ERROR: $f changes the type of $table.$column, and $ASSERTION_FILE has no typed entry for it." >&2
      continue
    fi
    want="$(type_constant "$new_type")"
    if [ -z "$want" ]; then
      WRONG_TYPE=$((WRONG_TYPE + 1))
      echo "ERROR: $f changes the type of $table.$column to $new_type, a type with no sql* constant in this gate." >&2
      echo "       Add the constant to $ASSERTION_FILE and its spellings to type_constant in scripts/check_schema_assertion_types.sh." >&2
      continue
    fi
    matched=no
    have=""
    while IFS= read -r entry_type; do
      if names_constant "$entry_type" "$want"; then
        matched=yes
      fi
      have="${have:+$have, }Type: $entry_type"
    done <<<"$types"
    if [ "$matched" = yes ]; then
      CHECKED="$CHECKED
  $f: $table.$column ($want)"
      continue
    fi
    WRONG_TYPE=$((WRONG_TYPE + 1))
    echo "ERROR: $f changes the type of $table.$column to $new_type, so its typed entry in $ASSERTION_FILE needs Type: $want (it has $have)." >&2
  done <<<"$changes"
done

# --- 4. verdict ------------------------------------------------------------------
if [ "$MISSING" -eq 0 ] && [ "$WRONG_TYPE" -eq 0 ]; then
  if [ -z "$CHECKED" ]; then
    echo "OK: no new timestamp-named migration changes a column type."
  else
    echo "OK: every column type change in a new timestamp-named migration has a typed entry of its new type:$CHECKED"
  fi
  exit 0
fi

echo "" >&2
echo "A timestamp-named migration can be skipped when branches merge out of" >&2
echo "timestamp order; the typed entry is what catches that at boot." >&2
if [ "$MISSING" -gt 0 ]; then
  echo "Fix: add {Table: \"<table>\", Column: \"<column>\", Type: <type>} to the" >&2
  echo "     typed entries in requiredSchemaColumns in $ASSERTION_FILE." >&2
fi
exit 1
