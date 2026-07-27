#!/usr/bin/env bash
# Regenerate frontend/types/api.d.ts from the backend's OpenAPI document.
#
# The spec is produced by `go run ./cmd/openapi-spec`, NOT fetched from a
# running server, so this is hermetic: no database, no deployed environment,
# nothing that has to be up. That is what lets CI run the same command and fail
# on a diff (PSY-1550).
#
# Usage: bun run api:types    (from frontend/)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/frontend/types/api.d.ts"
SPEC="$(mktemp -t psy-openapi-XXXXXX.yaml)"
trap 'rm -f "$SPEC"' EXIT

# stderr carries service-init warnings (e.g. WebAuthn without a DB) that are
# irrelevant to the document; only stdout is the spec.
(cd "$ROOT/backend" && go run ./cmd/openapi-spec 2>/dev/null) > "$SPEC"

if [ ! -s "$SPEC" ]; then
  echo "gen-api-types: backend produced an empty spec" >&2
  exit 1
fi

"$ROOT/frontend/node_modules/.bin/openapi-typescript" "$SPEC" --output "$OUT"
echo "wrote $OUT"
