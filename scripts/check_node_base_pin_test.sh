#!/usr/bin/env bash
# scripts/check_node_base_pin_test.sh
#
# Self-test for scripts/check_node_base_pin.sh. Copies the real backend/Dockerfile
# into a throwaway tree, mutates it into the shapes the gate has to get right,
# and asserts both the exit code and the actionable part of the message.
#
# Runs in CI (Backend Tests) and locally:
#   bash scripts/check_node_base_pin_test.sh
#
# This gate carries more weight than a normal lint step: CI's own Node version
# is DERIVED from its --print-version output, so a parsing regression would not
# just miss a problem, it would pick the wrong V8 to test the layout worker on.
#
# Seeding from the real committed Dockerfile rather than from hand-written
# fixtures is deliberate: it makes the baseline case an assertion that the tree
# as committed is correctly pinned, so a future restructuring of the stages
# shows up here rather than only in CI.
#
# It touches nothing outside its own mktemp -d.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

mkdir -p "$FIXTURE/scripts" "$FIXTURE/backend"
cp "$SCRIPT_DIR/check_node_base_pin.sh" "$FIXTURE/scripts/"

# A digest-shaped constant that is obviously not the real one.
ZEROS="0000000000000000000000000000000000000000000000000000000000000000"

FAILURES=0
CASE=""

reset_fixture() {
  cp "$REPO_ROOT/backend/Dockerfile" "$FIXTURE/backend/Dockerfile"
}

# sed -i is not portable between GNU and BSD; rewrite through a temp file.
edit() {
  # edit <sed-expression>
  local expr="$1" file="$FIXTURE/backend/Dockerfile"
  sed -E "$expr" "$file" >"$file.tmp" && mv "$file.tmp" "$file"
}

expect() {
  # expect <wanted-exit-code> [needle] [mode-flag]
  local want_code="$1" needle="${2:-}" flag="${3:-}"
  local out code
  # Quoted: an empty flag passes "" as $1, which the script's arg parser treats
  # as "no flag" (report mode).
  out="$(bash "$FIXTURE/scripts/check_node_base_pin.sh" "$flag" 2>&1)"
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

# The committed tree must validate. This is the case that fails first if a
# future edit breaks the pin or reshapes what the gate parses.
CASE="committed Dockerfile is correctly pinned"
reset_fixture
expect 0 "OK: backend/Dockerfile pins node"

# The two print modes are a contract with .github/workflows: --print-version
# feeds actions/setup-node, --print-ref feeds the manifest-list assertion. Both
# must emit the bare value and nothing else.
CASE="--print-version emits a bare version"
reset_fixture
out="$(bash "$FIXTURE/scripts/check_node_base_pin.sh" --print-version 2>&1)"
if printf '%s' "$out" | grep -qxE '[0-9]+\.[0-9]+\.[0-9]+'; then
  echo "ok   [$CASE]"
else
  echo "FAIL [$CASE]: expected one bare x.y.z line, got: $out"
  FAILURES=$((FAILURES + 1))
fi

CASE="--print-ref emits a bare pinned reference"
reset_fixture
out="$(bash "$FIXTURE/scripts/check_node_base_pin.sh" --print-ref 2>&1)"
if printf '%s' "$out" | grep -qxE 'node:[0-9]+\.[0-9]+\.[0-9]+-alpine[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}'; then
  echo "ok   [$CASE]"
else
  echo "FAIL [$CASE]: expected one bare pinned ref, got: $out"
  FAILURES=$((FAILURES + 1))
fi

# A print mode must never hand a caller an unvalidated value.
CASE="--print-version still validates before printing"
reset_fixture
edit "s/@sha256:[0-9a-f]{64}//g"
expect 1 "not pinned in the expected form" --print-version

# Half-applied digest edit: layout-deps would resolve node_modules with a
# different npm than the runtime's node.
CASE="stages carry different digests"
reset_fixture
edit "s|@sha256:[0-9a-f]{64}( AS layout-deps)|@sha256:$ZEROS\1|"
expect 1 "disagree"

CASE="digest dropped, tag alone"
reset_fixture
edit "s/@sha256:[0-9a-f]{64}//g"
expect 1 "not pinned in the expected form"

CASE="tag floated back to a major-only tag"
reset_fixture
edit "s/^FROM node:[0-9]+\.[0-9]+\.[0-9]+-alpine[0-9]+\.[0-9]+@/FROM node:22-alpine@/"
expect 1 "not pinned in the expected form"

CASE="no node stage at all"
reset_fixture
edit "s/^FROM node:[0-9]+\.[0-9]+\.[0-9]+-alpine[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}/FROM alpine:3.24/"
expect 1 "no 'FROM node:...' stage"

# The stage COUNT is deliberately not pinned -- only that the stages agree. A
# restructuring that drops to a single node stage must still pass.
CASE="a single node stage is fine as long as it is pinned"
reset_fixture
edit "s|^FROM node:([0-9]+\.[0-9]+\.[0-9]+-alpine[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}) AS layout-deps|FROM alpine:3.24 AS layout-deps|"
expect 0 "OK: backend/Dockerfile pins node"

CASE="rejects an unknown flag rather than guessing"
reset_fixture
expect 2 "usage:" --print-everything

echo
if [ "$FAILURES" -ne 0 ]; then
  echo "$FAILURES case(s) failed"
  exit 1
fi
echo "OK: node-base-pin gate self-tests passed."
