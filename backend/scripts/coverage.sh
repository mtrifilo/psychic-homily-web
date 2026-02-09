#!/bin/bash
# Run test coverage across all business-logic packages.
#
# Excluded packages (no testable logic):
#   internal/models/  — GORM struct defs, TableName(), WebAuthn adapters
#   internal/errors/  — Error constructors, exercised by every service error path
#   internal/logger/  — slog wrappers, logging infrastructure
#   internal/auth/    — SetupGoth() bootstraps global OAuth state
#
# Usage: bash scripts/coverage.sh

set -euo pipefail
cd "$(dirname "$0")/.."

echo "Running tests with coverage..."
go test -coverprofile=coverage.out -count=1 \
  ./internal/api/handlers/ \
  ./internal/api/middleware/ \
  ./internal/config/ \
  ./internal/services/ \
  ./internal/utils/

echo ""
echo "=== Coverage Summary ==="
go tool cover -func=coverage.out | grep "^total:"
echo ""
echo "Per-package breakdown:"
go tool cover -func=coverage.out | grep "^total:" || true
# Show per-file totals
go tool cover -func=coverage.out | awk '
  /^total:/ { next }
  {
    split($1, parts, "/")
    file = parts[length(parts)]
    split(file, fparts, ":")
    pkg_file = fparts[1]
    # just print as-is for the summary
  }
  END {}
'

echo ""
echo "For HTML report: go tool cover -html=coverage.out -o coverage.html"
