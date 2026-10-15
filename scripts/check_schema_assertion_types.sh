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
#   * Comments are stripped first: `--` to the end of the line, then `/* */`
#     blocks. Block comments do not nest here, a `--` inside a block comment
#     still ends its line, and comment markers inside a string literal are
#     read as comments.
#   * An entry counts when its Type is set to anything; the gate does not
#     compare it with the migration's new type. Entries match anywhere in the
#     file, outside `//` comments, in any whitespace layout.
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

# --- 1. typed entries in the schema assertion ---------------------------------
# One "table column" line per entry with a non-empty Type, lowercased. The file
# is folded to one line so an entry split across lines still matches.
ident='[A-Za-z_][A-Za-z0-9_]*'
TYPED_ENTRIES="$(sed 's|//.*$||' "$ASSERTION_FILE" \
  | tr '\n\t' '  ' \
  | grep -oE "\{[[:space:]]*Table:[[:space:]]*\"[^\"]+\"[[:space:]]*,[[:space:]]*Column:[[:space:]]*\"[^\"]+\"[[:space:]]*,[[:space:]]*Type:[[:space:]]*($ident(\.$ident)?|\"[^\"]+\")" \
  | sed -E 's/.*Table:[[:space:]]*"([^"]+)".*Column:[[:space:]]*"([^"]+)".*/\1 \2/' \
  | tr '[:upper:]' '[:lower:]' \
  | sort -u || true)"

# --- 2. timestamp-named up migrations this branch ADDS ------------------------
if ! ADDED_UP="$(git diff --name-only --diff-filter=A --no-renames \
                   "$BASE_REF" HEAD -- "$MIGRATIONS_PATH/*.up.sql")"; then
  echo "ERROR: could not diff HEAD against $BASE_REF." >&2
  exit 1
fi

ADDED_TIMESTAMPED=""
for f in $ADDED_UP; do
  if grep -qE '^[0-9]{14}_' <<<"${f##*/}"; then
    ADDED_TIMESTAMPED="$ADDED_TIMESTAMPED $f"
  fi
done

if [ -z "$ADDED_TIMESTAMPED" ]; then
  echo "OK: no new timestamp-named up migrations added against $BASE_REF."
  exit 0
fi

# --- 3. column type changes in each added migration ---------------------------
# Prints file $1 with `--` and then `/* */` comments removed, as one lowercase
# line without double quotes, one statement per line.
statements() {
  sed 's/--.*$//' "$1" \
    | tr '\n\t' '  ' \
    | sed -E 's#/\*([^*]|\*+[^*/])*\*+/# #g' \
    | tr '[:upper:]' '[:lower:]' \
    | tr -d '"' \
    | tr ';' '\n'
}

# Prints "table column" for every column type change in file $1, once each.
type_changes() {
  local name='[a-z_][a-z0-9_$]*' stmt rest table
  local sp='[[:space:]]+'
  while IFS= read -r stmt; do
    # "<table> <actions>" after the statement's ALTER TABLE head, if any.
    rest="$(sed -nE "s/.*alter${sp}table${sp}(if${sp}exists${sp})?(only${sp})?($name\\.)?($name)(.*)/\\4 \\5/p" <<<"$stmt")"
    [ -n "$rest" ] || continue
    table="${rest%% *}"
    # Each action starts after a space or a comma; the padding gives the first
    # and last action that boundary.
    grep -oE "[[:space:],]alter${sp}(column${sp})?$name${sp}(set${sp}data${sp})?type[[:space:]]" <<<" ${rest#* } " \
      | sed -E "s/^[[:space:],]alter${sp}(column${sp})?($name)${sp}.*/\\2/" \
      | sed "s/^/$table /" || true
  done < <(statements "$1")
}

CHECKED=""
MISSING=0
for f in $ADDED_TIMESTAMPED; do
  if [ ! -r "$f" ]; then
    echo "ERROR: $f is added on HEAD but not readable in the working tree." >&2
    exit 1
  fi
  changes="$(type_changes "$f" | sort -u)"
  while IFS= read -r pair; do
    [ -n "$pair" ] || continue
    table="${pair% *}"
    column="${pair#* }"
    if grep -qxF "$pair" <<<"$TYPED_ENTRIES"; then
      CHECKED="$CHECKED
  $f: $table.$column"
      continue
    fi
    MISSING=$((MISSING + 1))
    echo "ERROR: $f changes the type of $table.$column, and $ASSERTION_FILE has no typed entry for it." >&2
  done <<<"$changes"
done

# --- 4. verdict ------------------------------------------------------------------
if [ "$MISSING" -eq 0 ]; then
  if [ -z "$CHECKED" ]; then
    echo "OK: no new timestamp-named migration changes a column type."
  else
    echo "OK: every column type change in a new timestamp-named migration has a typed entry:$CHECKED"
  fi
  exit 0
fi

echo "" >&2
echo "A timestamp-named migration can be skipped when branches merge out of" >&2
echo "timestamp order; the typed entry is what catches that at boot." >&2
echo "Fix: add {Table: \"<table>\", Column: \"<column>\", Type: <type>} to the" >&2
echo "     typed entries in requiredSchemaColumns in $ASSERTION_FILE." >&2
exit 1
