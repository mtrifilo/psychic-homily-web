#!/usr/bin/env bash
# scripts/dispatch/stack-up.sh
#
# Bring up a dev stack for a dispatched worktree's manual repro phase.
#
# Usage: stack-up.sh <worktree-path> --mode={none,shared,isolated}
#                    [--backend-only] [--auth-rate-limits] [--secure]
#
# Modes:
#   none      No stack. Backend-only changes without migration; integration
#             tests are the manual repro (PSY-592 pattern). Writes a marker
#             .env and exits 0.
#   shared    Frontend-only changes. Probes :8080; if a dev backend is up,
#             starts a frontend on a free port pointing at it. If :8080 is
#             free, escalates to isolated mode automatically.
#   isolated  Fullstack changes, migration changes, or anything that
#             requires backend code isolation. Spins up per-worktree
#             postgres (via docker compose) + native backend + native
#             frontend, all on free ports.
#
# Frontend → backend routing convention (PSY-629):
#   Frontend talks to the backend via the in-process `/api` proxy
#   (app/api/[...path]/route.ts), which reads `BACKEND_URL` to forward.
#   Both browser and SSR fetches go same-origin through the proxy so the
#   `auth_token` cookie (SameSite=Lax) survives. In shared mode the SSR
#   dev fallback (`http://localhost:8080`) is correct; in isolated mode
#   we point `NEXT_PUBLIC_API_URL` at `$STACK_FRONTEND_URL/api` so SSR
#   resolves to the same proxy instead of hardcoding :8080.
#
#   OAuth is the exception (PSY-1649). The Google button is a full-page
#   redirect to the backend's /auth/login/google, and that redirect chain
#   does not survive the proxy, so it reads a SEPARATE variable,
#   `NEXT_PUBLIC_OAUTH_BACKEND_URL`, which must be the backend's own
#   origin. In isolated mode that is `$STACK_BACKEND_URL`; in shared mode
#   it is left unset so `lib/api-base.ts` falls back to :8080, which IS
#   the user's dev backend there. Before the split both requirements were
#   read off `NEXT_PUBLIC_API_URL`, and no single value satisfied both on
#   a non-default backend port.
#
# Isolated-stack options. They configure the stack this script starts, so mode
# none and shared mode on a running dev backend refuse them:
#   --backend-only      Start postgres and the backend, not the frontend. The
#                       .env still names the frontend port and origin the
#                       backend was configured for, so a frontend started
#                       there later works against it.
#   --auth-rate-limits  Keep the auth, passkey and OAuth rate limiters on.
#                       Env equivalent: STACK_AUTH_RATE_LIMITS=1. Off by
#                       default, because every client of a stack shares
#                       127.0.0.1.
#   --secure            SESSION_SECURE=true, so auth cookies carry Secure.
#                       Chromium stores and sends Secure cookies on
#                       http://localhost.
#
#   The isolated backend always gets FRONTEND_URL and WEBAUTHN_RP_ORIGINS set
#   to the stack's own frontend origin, so OAuth results, link-token mints and
#   passkey ceremonies resolve against this stack rather than :3000. It also
#   gets BACKEND_PUBLIC_URL set to its own origin, so the default OAuth
#   provider callback URLs and backend-hosted email links point at this stack
#   rather than :8080. The dev backend shared mode reuses keeps its own values
#   for all of these.
#
# Output:
#   Writes <worktree>/dispatch-stack/.env with STACK_* vars + .pid files.
#   Prints the .env contents to stdout for the calling agent.

set -euo pipefail

WORKTREE_PATH="${1:?Required: worktree path (e.g. /path/to/.claude/worktrees/agent-XXX)}"
shift

usage() {
  echo "Usage: $0 <worktree-path> --mode={none,shared,isolated} [--backend-only] [--auth-rate-limits] [--secure]" >&2
  exit 64
}

MODE=""
BACKEND_ONLY=0
AUTH_RATE_LIMITS="${STACK_AUTH_RATE_LIMITS:-0}"
SESSION_SECURE_ON=0
for arg in "$@"; do
  case "$arg" in
    --mode=none|--mode=shared|--mode=isolated) MODE="${arg#--mode=}" ;;
    --backend-only) BACKEND_ONLY=1 ;;
    --auth-rate-limits) AUTH_RATE_LIMITS=1 ;;
    --secure) SESSION_SECURE_ON=1 ;;
    *) usage ;;
  esac
done
[ -n "$MODE" ] || usage

# The backend honors DISABLE_AUTH_RATE_LIMITS only when it is exactly "1", so
# "0" turns the limiters on even when the caller's environment carries a "1".
case "$AUTH_RATE_LIMITS" in
  1) DISABLE_AUTH_RATE_LIMITS_VALUE=0 ;;
  0) DISABLE_AUTH_RATE_LIMITS_VALUE=1 ;;
  *) echo "STACK_AUTH_RATE_LIMITS must be 0 or 1." >&2; exit 64 ;;
esac
if [ "$SESSION_SECURE_ON" = 1 ]; then SESSION_SECURE_VALUE=true; else SESSION_SECURE_VALUE=false; fi

# Set explicitly rather than left to the backend's non-production defaults, so
# a value inherited from the caller's shell or a backend .env file cannot point the
# stack's ceremonies elsewhere. An RP ID carries no port, so "localhost" covers
# every stack frontend port; WEBAUTHN_RP_ORIGINS pins this stack's frontend.
STACK_WEBAUTHN_RP_ID=localhost
STACK_WEBAUTHN_RP_NAME="Psychic Homily (dispatch stack)"

refuse_isolated_options() {
  if [ "$BACKEND_ONLY" = 1 ] || [ "$AUTH_RATE_LIMITS" = 1 ] || [ "$SESSION_SECURE_ON" = 1 ]; then
    echo "--backend-only, --auth-rate-limits and --secure configure a stack this script starts; $1. Use --mode=isolated." >&2
    exit 64
  fi
}

WORKTREE_PATH="$(cd "$WORKTREE_PATH" && pwd)"
WORKTREE_ID="$(basename "$WORKTREE_PATH")"
STACK_DIR="$WORKTREE_PATH/dispatch-stack"
# Compose project names must be lowercase [a-z0-9_-]; worktree dirs like
# "PSY-1045+content-first-profile" contain uppercase and '+', so sanitize.
# This value is persisted as STACK_COMPOSE_PROJECT (below) and read back by
# stack-down.sh — keep its legacy fallback sanitize in sync if you change this.
COMPOSE_PROJECT="dispatch-$(printf '%s' "$WORKTREE_ID" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '-')"
COMPOSE_PROJECT="${COMPOSE_PROJECT%-}"

mkdir -p "$STACK_DIR"

log()       { echo "[stack-up:$WORKTREE_ID] $*"; }
free_port() { python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()"; }

wait_for_url() {
  local url="$1" timeout_sec="${2:-60}"
  local start; start=$(date +%s)
  while true; do
    if curl -fsS -o /dev/null --max-time 2 "$url" 2>/dev/null; then return 0; fi
    if [ "$(($(date +%s) - start))" -ge "$timeout_sec" ]; then
      echo "Timeout waiting for $url" >&2
      return 1
    fi
    sleep 0.5
  done
}

dev_backend_up() {
  # The /health handler always returns 200 — even when the database is down
  # the body reports status:"unhealthy". Parse the body so we don't pick a
  # broken backend and silently skip the isolated-mode setup.
  local body
  body=$(curl -fsS --max-time 2 http://localhost:8080/health 2>/dev/null) || return 1
  echo "$body" | jq -e '.status == "healthy"' >/dev/null 2>&1
}

# === Mode dispatch ===

if [ "$MODE" = "none" ]; then
  refuse_isolated_options "mode none starts no backend"
  log "Mode: none. No stack required (backend integration tests are the manual repro)."
  cat > "$STACK_DIR/.env" <<EOF
STACK_MODE=none
STACK_WORKTREE_ID=$WORKTREE_ID
EOF
  cat "$STACK_DIR/.env"
  exit 0
fi

if [ "$MODE" = "shared" ]; then
  if ! dev_backend_up; then
    log "Dev backend at :8080 not responding. Escalating to isolated mode."
    MODE="isolated"
  else
    refuse_isolated_options "shared mode reuses the dev backend at :8080"
    log "Dev backend at :8080 is up. Allocating frontend port."
    FRONTEND_PORT="$(free_port)"
    log "Starting frontend on :$FRONTEND_PORT..."
    cd "$WORKTREE_PATH/frontend"
    # BACKEND_URL is read by the catch-all proxy (app/api/[...path]/route.ts).
    # NEXT_PUBLIC_API_URL is intentionally left unset: SSR pages fall back to
    # http://localhost:8080 in NODE_ENV=development, which IS the user's dev
    # backend in shared mode; browser-side `lib/api-base.ts` then routes
    # through the same-origin /api proxy. NEXT_PUBLIC_OAUTH_BACKEND_URL is
    # left unset for the same reason: its fallback is that same :8080.
    nohup env BACKEND_URL="http://localhost:8080" \
      bun run dev --port "$FRONTEND_PORT" \
      </dev/null >"$STACK_DIR/frontend.log" 2>&1 &
    echo $! > "$STACK_DIR/frontend.pid"
    disown 2>/dev/null || true

    # PSY-633: write .env BEFORE the frontend health check. If the wait times
    # out (set -e exits the script), stack-down.sh still has the identifiers
    # it needs to clean up. Shared mode doesn't run docker compose, but the
    # PID files plus a present .env keep cleanup paths consistent.
    cat > "$STACK_DIR/.env" <<EOF
STACK_MODE=shared
STACK_WORKTREE_ID=$WORKTREE_ID
STACK_BACKEND_URL=http://localhost:8080
STACK_FRONTEND_PORT=$FRONTEND_PORT
STACK_FRONTEND_URL=http://localhost:$FRONTEND_PORT
BACKEND_URL=http://localhost:8080
EOF

    log "Waiting for frontend at http://localhost:$FRONTEND_PORT..."
    wait_for_url "http://localhost:$FRONTEND_PORT" 60

    cat "$STACK_DIR/.env"
    log "Stack up (mode=shared): http://localhost:$FRONTEND_PORT -> http://localhost:8080"
    exit 0
  fi
fi

# === Isolated mode (reached directly or via shared falling through) ===

log "Mode: isolated. Allocating ports..."
POSTGRES_PORT="$(free_port)"
BACKEND_PORT="$(free_port)"
FRONTEND_PORT="$(free_port)"

STACK_POSTGRES_URL="postgres://dispatchuser:dispatchpassword@localhost:$POSTGRES_PORT/dispatchdb?sslmode=disable"
STACK_BACKEND_URL="http://localhost:$BACKEND_PORT"
STACK_FRONTEND_URL="http://localhost:$FRONTEND_PORT"

log "Postgres :$POSTGRES_PORT, Backend :$BACKEND_PORT, Frontend :$FRONTEND_PORT"

cd "$WORKTREE_PATH/backend"

log "Starting postgres + migrate (project=$COMPOSE_PROJECT)..."
# Don't pass --wait: the migrate one-shot exits with 0, which `compose up
# --wait` treats as failure. setup-db.sh waits for migrate to finish via
# `compose ps -a --format json migrate`, and the db's healthcheck is
# observed transitively (migrate `depends_on: db: condition: service_healthy`,
# so migrate completing implies db is healthy). Mirrors frontend/e2e/global-setup.ts.
POSTGRES_PORT="$POSTGRES_PORT" \
  docker compose -p "$COMPOSE_PROJECT" -f docker-compose.dispatch.yml up -d

log "Seeding database (full E2E fixture)..."
DATABASE_URL="$STACK_POSTGRES_URL" \
COMPOSE_PROJECT="$COMPOSE_PROJECT" \
COMPOSE_FILE="docker-compose.dispatch.yml" \
bash "$WORKTREE_PATH/frontend/e2e/setup-db.sh"

log "Starting backend on :$BACKEND_PORT..."
cd "$WORKTREE_PATH/backend"
nohup env \
  DATABASE_URL="$STACK_POSTGRES_URL" \
  API_ADDR="localhost:$BACKEND_PORT" \
  CORS_ALLOWED_ORIGINS="$STACK_FRONTEND_URL" \
  FRONTEND_URL="$STACK_FRONTEND_URL" \
  BACKEND_PUBLIC_URL="$STACK_BACKEND_URL" \
  WEBAUTHN_RP_ID="$STACK_WEBAUTHN_RP_ID" \
  WEBAUTHN_RP_NAME="$STACK_WEBAUTHN_RP_NAME" \
  WEBAUTHN_RP_ORIGINS="$STACK_FRONTEND_URL" \
  ENVIRONMENT=test \
  ENABLE_TEST_FIXTURES=1 \
  DISCORD_NOTIFICATIONS_ENABLED=false \
  DISABLE_RADIO_FETCH=1 \
  DISABLE_AUTO_PROMOTION=1 \
  DISABLE_ENRICHMENT_WORKER=1 \
  DISABLE_COLLECTION_DIGEST=1 \
  DISABLE_CLEANUP=1 \
  DISABLE_REMINDERS=1 \
  DISABLE_RELATIONSHIP_DERIVATION=1 \
  DISABLE_STREET_GEOCODE_SWEEP=1 \
  DISABLE_RADIO_TRACKS_REBUILD="${DISABLE_RADIO_TRACKS_REBUILD:-1}" \
  DISABLE_WEBAUTHN_CHALLENGE_SWEEP=1 \
  DISABLE_AUTH_RATE_LIMITS="$DISABLE_AUTH_RATE_LIMITS_VALUE" \
  SESSION_SECURE="$SESSION_SECURE_VALUE" \
  SESSION_SAME_SITE=lax \
  JWT_SECRET_KEY=dispatch-jwt-secret-key-for-testing-only \
  OAUTH_SECRET_KEY=dispatch-oauth-secret-key-for-testing-only \
  SESSION_SECRET=dispatch-session-secret-for-testing-only \
  go run ./cmd/server \
  </dev/null >"$STACK_DIR/backend.log" 2>&1 &
echo $! > "$STACK_DIR/backend.pid"
disown 2>/dev/null || true

log "Waiting for backend at $STACK_BACKEND_URL/health..."
wait_for_url "$STACK_BACKEND_URL/health" 90

# PSY-633: write .env BEFORE the frontend health check. If the wait times out
# (the script exits 1), stack-down.sh still reads STACK_MODE=isolated +
# STACK_COMPOSE_PROJECT and runs `docker compose down -v` to reap postgres.
# All values are stable at this point: ports were allocated upfront and the
# backend just passed its health check — the frontend port is identifier
# only, the dev server doesn't need to come up for stack-down to use it.
cat > "$STACK_DIR/.env" <<EOF
STACK_MODE=isolated
STACK_WORKTREE_ID=$WORKTREE_ID
STACK_COMPOSE_PROJECT=$COMPOSE_PROJECT
STACK_POSTGRES_URL=$STACK_POSTGRES_URL
STACK_POSTGRES_PORT=$POSTGRES_PORT
STACK_BACKEND_URL=$STACK_BACKEND_URL
STACK_BACKEND_PORT=$BACKEND_PORT
STACK_BACKEND_PUBLIC_URL=$STACK_BACKEND_URL
STACK_FRONTEND_URL=$STACK_FRONTEND_URL
STACK_FRONTEND_PORT=$FRONTEND_PORT
STACK_BACKEND_ONLY=$BACKEND_ONLY
STACK_AUTH_RATE_LIMITS=$AUTH_RATE_LIMITS
STACK_SESSION_SECURE=$SESSION_SECURE_ON
STACK_WEBAUTHN_RP_ID=$STACK_WEBAUTHN_RP_ID
STACK_WEBAUTHN_RP_NAME="$STACK_WEBAUTHN_RP_NAME"
STACK_WEBAUTHN_RP_ORIGINS=$STACK_FRONTEND_URL
BACKEND_URL=$STACK_BACKEND_URL
NEXT_PUBLIC_API_URL=$STACK_FRONTEND_URL/api
NEXT_PUBLIC_OAUTH_BACKEND_URL=$STACK_BACKEND_URL
EOF

if [ "$BACKEND_ONLY" = 1 ]; then
  cat "$STACK_DIR/.env"
  log "Stack up (mode=isolated, backend only): $STACK_BACKEND_URL -> postgres :$POSTGRES_PORT"
  exit 0
fi

log "Starting frontend on :$FRONTEND_PORT..."
cd "$WORKTREE_PATH/frontend"
# See the file header for the routing convention. NEXT_PUBLIC_API_URL points
# SSR + browser at the frontend's own /api proxy; BACKEND_URL is what that
# proxy (and the per-route admin/AI proxies under app/api/admin/* and
# app/api/ai/*) forwards to. Without BACKEND_URL the proxy defaults to :8080
# and misses the per-worktree backend port. NEXT_PUBLIC_OAUTH_BACKEND_URL
# deliberately bypasses the proxy. See the header (PSY-1649).
nohup env NEXT_PUBLIC_API_URL="$STACK_FRONTEND_URL/api" \
  NEXT_PUBLIC_OAUTH_BACKEND_URL="$STACK_BACKEND_URL" \
  BACKEND_URL="$STACK_BACKEND_URL" \
  bun run dev --port "$FRONTEND_PORT" \
  </dev/null >"$STACK_DIR/frontend.log" 2>&1 &
echo $! > "$STACK_DIR/frontend.pid"
disown 2>/dev/null || true

log "Waiting for frontend at $STACK_FRONTEND_URL..."
if ! wait_for_url "$STACK_FRONTEND_URL" 60; then
  log "The backend is up at $STACK_BACKEND_URL and the stack is recorded in $STACK_DIR/.env, but the frontend did not answer; see $STACK_DIR/frontend.log. Tear down with stack-down.sh, or rerun with --backend-only when the repro needs no frontend."
  exit 1
fi

cat "$STACK_DIR/.env"
log "Stack up (mode=isolated): $STACK_FRONTEND_URL -> $STACK_BACKEND_URL -> postgres :$POSTGRES_PORT"
