#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Temp files for capturing output
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

BACKEND_LOG="$TMPDIR_TEST/backend.log"
FRONTEND_LOG="$TMPDIR_TEST/frontend.log"
E2E_LOG="$TMPDIR_TEST/e2e.log"

# Strip ANSI escape codes (perl works on both macOS and Linux)
strip_ansi() {
  perl -pe 's/\e\[[0-9;]*[a-zA-Z]//g; s/\e\([0-9;]*[a-zA-Z]//g'
}

echo "=== Running all test suites in parallel ==="
echo ""

# --- Backend tests ---
echo "[backend]  Starting: go test ./..."
(
  cd "$PROJECT_ROOT/backend"
  go test ./... 2>&1 | strip_ansi > "$BACKEND_LOG"
) &
PID_BACKEND=$!

# --- Frontend unit tests ---
echo "[frontend] Starting: bun run test:run"
(
  cd "$PROJECT_ROOT/frontend"
  bun run test:run 2>&1 | strip_ansi > "$FRONTEND_LOG"
) &
PID_FRONTEND=$!

# --- E2E tests ---
echo "[e2e]      Starting: bun run test:e2e"
(
  cd "$PROJECT_ROOT/frontend"
  bun run test:e2e 2>&1 | strip_ansi > "$E2E_LOG"
) &
PID_E2E=$!

echo ""

# Wait for each and capture exit codes
EXIT_BACKEND=0
EXIT_FRONTEND=0
EXIT_E2E=0

wait $PID_BACKEND || EXIT_BACKEND=$?
echo "[backend]  Finished (exit code: $EXIT_BACKEND)"

wait $PID_FRONTEND || EXIT_FRONTEND=$?
echo "[frontend] Finished (exit code: $EXIT_FRONTEND)"

wait $PID_E2E || EXIT_E2E=$?
echo "[e2e]      Finished (exit code: $EXIT_E2E)"

# --- Summary ---
echo ""
echo "==========================================="
echo "  TEST SUMMARY"
echo "==========================================="
printf "  %-12s %s\n" "Backend:" "$([ $EXIT_BACKEND -eq 0 ] && echo 'PASS ✓' || echo 'FAIL ✗')"
printf "  %-12s %s\n" "Frontend:" "$([ $EXIT_FRONTEND -eq 0 ] && echo 'PASS ✓' || echo 'FAIL ✗')"
printf "  %-12s %s\n" "E2E:" "$([ $EXIT_E2E -eq 0 ] && echo 'PASS ✓' || echo 'FAIL ✗')"
echo "==========================================="

# --- Print output of failing suites ---
ANY_FAILED=0

if [ $EXIT_BACKEND -ne 0 ]; then
  ANY_FAILED=1
  echo ""
  echo "==========================================="
  echo "  FAILED: Backend (go test ./...)"
  echo "==========================================="
  cat "$BACKEND_LOG"
fi

if [ $EXIT_FRONTEND -ne 0 ]; then
  ANY_FAILED=1
  echo ""
  echo "==========================================="
  echo "  FAILED: Frontend Unit (bun run test:run)"
  echo "==========================================="
  cat "$FRONTEND_LOG"
fi

if [ $EXIT_E2E -ne 0 ]; then
  ANY_FAILED=1
  echo ""
  echo "==========================================="
  echo "  FAILED: E2E (bun run test:e2e)"
  echo "==========================================="
  cat "$E2E_LOG"
  echo ""
  echo "NOTE: E2E tests require Docker running and port 8080 free (stop dev backend first)."
fi

if [ $ANY_FAILED -eq 0 ]; then
  echo ""
  echo "All test suites passed!"
fi

exit $ANY_FAILED
