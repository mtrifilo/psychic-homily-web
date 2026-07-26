#!/bin/bash
set -euo pipefail

# PSY-1557: backend/.env is gitignored, so a freshly created worktree does not
# have one. The server does NOT fail on a missing env file — cmd/server only
# logs a warning and config.Load() falls back to a default DATABASE_URL — so
# the symptom shows up later as a confusing connection error instead of a
# clear message here. Fail fast and name the file.
#
# Skipped when the environment already supplies the config (the dispatch stack
# and the E2E harness both inject every var explicitly and never source .env),
# so this only guards the manual local-dev path.
#
# Both vars are checked, not just DATABASE_URL: a stale DATABASE_URL left
# exported in a shell would otherwise wave the guard through with JWT_SECRET_KEY
# still unset — and that one has no default in internal/config, so you'd land
# right back in a confusing downstream error.
env_file=".env.${ENVIRONMENT:-development}"
if [ ! -f "$env_file" ] && [ ! -f .env ] &&
  { [ -z "${DATABASE_URL:-}" ] || [ -z "${JWT_SECRET_KEY:-}" ]; }; then
  echo "run-dev.sh: no $env_file and no .env in $(pwd)," >&2
  echo "and DATABASE_URL / JWT_SECRET_KEY are not both set in the environment." >&2
  echo "" >&2
  echo "Create one, then re-run:" >&2
  echo "  cp .env.example .env          # template, then fill in the values" >&2
  echo "  # or copy a working one from your main checkout:" >&2
  echo "  cp ~/dev/psychic-homily-web/backend/.env .env" >&2
  exit 1
fi

go run cmd/server/main.go
