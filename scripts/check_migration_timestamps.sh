#!/usr/bin/env bash
# scripts/check_migration_timestamps.sh
#
# CI gate: every migration a branch ADDS must be stamped strictly ahead of
# every migration already merged on the base branch.
#
# Why this exists (stage outage 2026-08-02): two parallel branches authored
# timestamp migrations in author-time order (20260801120000, then
# 20260801143000) but merged in the reverse order. golang-migrate compares a
# migration's version against the schema_migrations pointer and SILENTLY SKIPS
# anything at or below it, reporting "no change" and exiting 0. The
# lower-stamped file never ran, its columns never existed, and every stage
# deploy failed for four hours until the DDL was replayed by hand.
#
# The other Migration Lint checks inspect a file in isolation (duplicate
# version, missing down.sql). This is the only check that compares a new file
# against what is already MERGED, which is the axis that failed.
#
# Usage:
#   scripts/check_migration_timestamps.sh [base-ref]
#
# base-ref defaults to origin/main. The CALLER is responsible for making that
# ref current (`git fetch origin main`) -- this script does no network I/O so
# it behaves identically in CI and on a laptop.
#
# Implementation notes:
#   * The "added" set is a two-dot diff against the base ref TIP, not a
#     three-dot merge-base diff. With --diff-filter=A the two are equivalent
#     (both mean "paths present in HEAD, absent from the base"), and the
#     two-dot form needs only the two tip trees -- so CI can keep its shallow
#     checkout and merge commits inside the branch are a non-issue.
#   * --no-renames is deliberate: a rename is exactly how a bad stamp gets
#     fixed, and a rename to a STILL-backdated name must not slip through as
#     an untouched "R" entry.
#   * The comparison is timestamp-format only. Legacy sequential migrations
#     (000001-000078) are ignored when computing the merged maximum, but a
#     NEWLY added sequential-format file is rejected: new migrations use the
#     timestamp format by convention.

set -euo pipefail
# No globbing: filenames flow through unquoted word-splitting below, and a
# stray glob metacharacter in a path must not expand against the working tree.
set -f

BASE_REF="${1:-origin/main}"
MIGRATIONS_PATH="backend/db/migrations"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

if ! git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null; then
  echo "ERROR: base ref '$BASE_REF' not found in this repository." >&2
  echo "       Fetch it first, e.g. 'git fetch origin main'." >&2
  exit 1
fi

# --- 1. newest migration timestamp already merged on the base ref ------------
# Every git invocation below is checked, and an empty result is treated as a
# broken assumption rather than "nothing to compare against": a fail-open gate
# is worse than no gate, because it looks green.
if ! BASE_TREE="$(git ls-tree -r --name-only "$BASE_REF" -- "$MIGRATIONS_PATH")"; then
  echo "ERROR: could not list $MIGRATIONS_PATH on $BASE_REF." >&2
  exit 1
fi

BASE_MAX="$(printf '%s\n' "$BASE_TREE" \
            | sed -E 's|.*/||' \
            | grep -oE '^[0-9]{14}_' \
            | tr -d '_' \
            | sort -n \
            | tail -1 || true)"

if [ -z "$BASE_MAX" ]; then
  echo "ERROR: no timestamp-format migrations found in $MIGRATIONS_PATH on $BASE_REF." >&2
  echo "       There is nothing to compare new migrations against, so this gate" >&2
  echo "       cannot do its job. Check the ref and the migrations path." >&2
  exit 1
fi

# --- 2. migration files this branch ADDS on top of the base ------------------
if ! HEAD_DIFF="$(git diff --name-only --diff-filter=A --no-renames \
                    "$BASE_REF" HEAD -- "$MIGRATIONS_PATH")"; then
  echo "ERROR: could not diff HEAD against $BASE_REF." >&2
  exit 1
fi

ADDED="$(printf '%s\n' "$HEAD_DIFF" | grep -E '\.sql$' || true)"

if [ -z "$ADDED" ]; then
  echo "OK: no new migration files added against $BASE_REF."
  exit 0
fi

# --- 3. classify each added file ---------------------------------------------
BAD_FORMAT=""
BACKDATED=""

for f in $ADDED; do
  base_name="${f##*/}"
  if ! printf '%s' "$base_name" | grep -qE '^[0-9]{14}_'; then
    BAD_FORMAT="$BAD_FORMAT $f"
    continue
  fi
  stamp="${base_name%%_*}"
  if [ "$((10#$stamp))" -le "$((10#$BASE_MAX))" ]; then
    BACKDATED="$BACKDATED $f"
  fi
done

if [ -z "$BAD_FORMAT" ] && [ -z "$BACKDATED" ]; then
  echo "OK: all new migrations are stamped ahead of $BASE_REF (newest merged: $BASE_MAX)."
  for f in $ADDED; do echo "  $f"; done
  exit 0
fi

# --- 4. report -----------------------------------------------------------------
# Suggested replacement stamp: now (UTC), or merged-max plus one if the clock
# somehow trails the newest merged migration.
NEW_STAMP="$(date -u +%Y%m%d%H%M%S)"
if [ "$((10#$NEW_STAMP))" -le "$((10#$BASE_MAX))" ]; then
  NEW_STAMP="$((10#$BASE_MAX + 1))"
fi

if [ -n "$BAD_FORMAT" ]; then
  echo "ERROR: new migration(s) do not use the YYYYMMDDhhmmss timestamp format:" >&2
  for f in $BAD_FORMAT; do echo "         $f" >&2; done
  echo "" >&2
  echo "       The legacy sequential numbering (000001-000078) is closed. New" >&2
  echo "       migrations must be stamped so that merge order is total across" >&2
  echo "       parallel branches." >&2
  echo "" >&2
fi

if [ -n "$BACKDATED" ]; then
  echo "ERROR: new migration(s) stamped at or below the newest migration already" >&2
  echo "       merged on $BASE_REF ($BASE_MAX):" >&2
  for f in $BACKDATED; do
    base_name="${f##*/}"
    echo "         $f (stamped ${base_name%%_*})" >&2
  done
  echo "" >&2
  echo "       golang-migrate SILENTLY SKIPS any migration at or below the" >&2
  echo "       current schema_migrations pointer: it reports \"no change\" and" >&2
  echo "       exits 0. These files would never apply on an environment that" >&2
  echo "       has already run $BASE_MAX." >&2
  echo "" >&2
fi

echo "Fix: rename <file> to a current timestamp (contents unchanged); parallel-authored migrations must be stamped in merge order." >&2
echo "" >&2
echo "     Suggested stamp: $NEW_STAMP" >&2
for f in $BAD_FORMAT $BACKDATED; do
  dir_name="${f%/*}"
  base_name="${f##*/}"
  echo "     git mv $f $dir_name/${NEW_STAMP}_${base_name#*_}" >&2
done

exit 1
