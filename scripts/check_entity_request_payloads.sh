#!/usr/bin/env bash
# scripts/check_entity_request_payloads.sh
#
# PSY-869 CI parity check: every entity_type accepted by the entity_requests
# table's CHECK constraint MUST have a matching Go payload struct registered in
# payloadRegistry, and vice versa. Drift in either direction fails the build.
#
# This mirrors the grep-based migration-policy checks in .github/workflows/ci.yml
# (duplicate-version / missing-down checks). It is intentionally a dumb,
# dependency-free text comparison so it runs anywhere with bash + grep + sort.
#
# Sources of truth (kept deliberately close together so drift is a one-file
# diff in review):
#   1. The newest entity_requests CHECK constraint — the `entity_type IN (...)`
#      list across all *_entity_requests migrations. We scan ALL such migrations
#      and union the values so a follow-up migration that ALTERs the CHECK to
#      add a value is picked up.
#   2. payloadRegistry in
#      internal/models/community/entity_request_payloads.go — the map literal
#      whose keys are the EntityRequest* constants.
#
# Exit non-zero (failing CI) on any mismatch, printing the offending values.

set -euo pipefail

# Resolve repo paths relative to this script so it works from any cwd.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MIGRATIONS_DIR="$REPO_ROOT/backend/db/migrations"
PAYLOADS_FILE="$REPO_ROOT/backend/internal/models/community/entity_request_payloads.go"

if [ ! -f "$PAYLOADS_FILE" ]; then
  echo "ERROR: payload registry file not found: $PAYLOADS_FILE" >&2
  exit 1
fi

# --- 1. entity_type values from the migration CHECK constraint(s) ------------
# Grep every migration that mentions entity_requests, pull the
# `entity_type IN ('a', 'b', ...)` list, and extract the quoted values. We
# scope to lines that pair `entity_type` with `IN (` to avoid matching the
# source_context / decision_state CHECK lists in the same file.
CHECK_VALUES="$(grep -rhoE "entity_type[[:space:]]+IN[[:space:]]*\([^)]*\)" \
                  "$MIGRATIONS_DIR"/*entity_requests*.up.sql 2>/dev/null \
                | grep -oE "'[a-z_]+'" \
                | tr -d "'" \
                | sort -u || true)"

if [ -z "$CHECK_VALUES" ]; then
  echo "ERROR: could not extract any entity_type values from the" >&2
  echo "       entity_requests CHECK constraint in $MIGRATIONS_DIR." >&2
  echo "       Did the migration filename or CHECK shape change?" >&2
  exit 1
fi

# --- 2. registered payload entity_types from the Go registry -----------------
# payloadRegistry maps EntityRequest<Type> constants to payload structs. Pull
# the constant values from their `const` definitions so we compare the actual
# string values (not the Go identifiers).
#
# Constant block looks like:
#   EntityRequestArtist = "artist"
REGISTRY_VALUES="$(grep -oE 'EntityRequest[A-Z][a-zA-Z]+[[:space:]]*=[[:space:]]*"[a-z_]+"' \
                     "$PAYLOADS_FILE" \
                   | grep -oE '"[a-z_]+"' \
                   | tr -d '"' \
                   | sort -u || true)"

if [ -z "$REGISTRY_VALUES" ]; then
  echo "ERROR: could not extract any EntityRequest* constant values from" >&2
  echo "       $PAYLOADS_FILE." >&2
  exit 1
fi

# --- 3. compare ---------------------------------------------------------------
# In the CHECK but missing a payload constant → a request of that type would be
# accepted by the DB but have no typed payload struct (the exact bug this guards).
MISSING_STRUCT="$(comm -23 <(printf '%s\n' "$CHECK_VALUES") <(printf '%s\n' "$REGISTRY_VALUES"))"
# Registered in Go but not allowed by the DB CHECK → an insert would fail at the
# DB. Also a drift worth failing on.
MISSING_CHECK="$(comm -13 <(printf '%s\n' "$CHECK_VALUES") <(printf '%s\n' "$REGISTRY_VALUES"))"

STATUS=0
if [ -n "$MISSING_STRUCT" ]; then
  echo "ERROR: entity_type(s) allowed by the entity_requests CHECK constraint" >&2
  echo "       but missing a payload struct in entity_request_payloads.go:" >&2
  printf '         %s\n' $MISSING_STRUCT >&2
  echo "       Add an EntityRequest<Type> const + a <Type>RequestPayload struct" >&2
  echo "       and register it in payloadRegistry." >&2
  STATUS=1
fi
if [ -n "$MISSING_CHECK" ]; then
  echo "ERROR: entity_type(s) registered in payloadRegistry but NOT allowed by" >&2
  echo "       the entity_requests CHECK constraint:" >&2
  printf '         %s\n' $MISSING_CHECK >&2
  echo "       Add the value to the CHECK constraint via a new migration." >&2
  STATUS=1
fi

if [ "$STATUS" -eq 0 ]; then
  echo "OK: entity_requests entity_type CHECK constraint and payloadRegistry agree:"
  printf '  %s\n' $CHECK_VALUES
fi

exit "$STATUS"
