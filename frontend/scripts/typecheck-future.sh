#!/usr/bin/env bash
# PSY-788: regression gate for the FE strict-mode ratchet.
#
# `tsconfig.future.json` extends `tsconfig.json` and turns on every remaining
# strict-family flag (`noImplicitAny`, `strictNullChecks`, `strictPropertyInitialization`).
# `.tsc-future-baseline` records the current tsc error count under that config.
# This script fails CI when a PR raises the count — preventing silent regressions
# while PSY-789 (noImplicitAny) and PSY-790 (strictNullChecks) burn down the list.
#
# When you fix a `tsconfig.future.json` error, lower the integer in
# `.tsc-future-baseline` by the same amount in the same commit.
set -euo pipefail

cd "$(dirname "$0")/.."

BASELINE=$(cat .tsc-future-baseline)
CURRENT=$(bunx tsc --noEmit -p tsconfig.future.json 2>&1 | grep -cE "error TS[0-9]+:" || true)

if [ "$CURRENT" -gt "$BASELINE" ]; then
  echo "typecheck:future regression — current=$CURRENT baseline=$BASELINE (+$((CURRENT - BASELINE)) errors)"
  exit 1
fi

echo "typecheck:future ok — current=$CURRENT baseline=$BASELINE"
