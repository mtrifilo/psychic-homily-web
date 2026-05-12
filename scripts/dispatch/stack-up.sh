#!/usr/bin/env bash
# scripts/dispatch/stack-up.sh
#
# Bring up a dev stack for a dispatched worktree's manual repro phase.
#
# Usage: stack-up.sh <worktree-path> --mode={none,shared,isolated}
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
# Output:
#   Writes <worktree>/dispatch-stack/.env with STACK_* vars + .pid files.
#   Prints the .env contents to stdout for the calling agent.

set -euo pipefail

WORKTREE_PATH="${1:?Required: worktree path (e.g. /path/to/.claude/worktrees/agent-XXX)}"
MODE_ARG="${2:-}"

case "$MODE_ARG" in
  --mode=none|--mode=shared|--mode=isolated)
    MODE="${MODE_ARG#--mode=}"
    ;;
  *)
    echo "Usage: $0 <worktree-path> --mode={none,shared,isolated}" >&2
    exit 64
    ;;
esac

WORKTREE_PATH="$(cd "$WORKTREE_PATH" && pwd)"
WORKTREE_ID="$(basename "$WORKTREE_PATH")"
STACK_DIR="$WORKTREE_PATH/dispatch-stack"
COMPOSE_PROJECT="dispatch-${WORKTREE_ID}"

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
    log "Dev backend at :8080 is up. Allocating frontend port."
    FRONTEND_PORT="$(free_port)"
    log "Starting frontend on :$FRONTEND_PORT..."
    cd "$WORKTREE_PATH/frontend"
    # BACKEND_URL is read by the catch-all proxy (app/api/[...path]/route.ts).
    # NEXT_PUBLIC_API_URL is intentionally left unset: SSR pages fall back to
    # http://localhost:8080 in NODE_ENV=development, which IS the user's dev
    # backend in shared mode; browser-side `lib/api-base.ts` then routes
    # through the same-origin /api proxy.
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
  ENVIRONMENT=test \
  ENABLE_TEST_FIXTURES=1 \
  DISCORD_NOTIFICATIONS_ENABLED=false \
  DISABLE_RADIO_FETCH=1 \
  DISABLE_AUTO_PROMOTION=1 \
  DISABLE_ENRICHMENT_WORKER=1 \
  DISABLE_SCHEDULER=1 \
  DISABLE_CLEANUP=1 \
  DISABLE_REMINDERS=1 \
  DISABLE_RELATIONSHIP_DERIVATION=1 \
  DISABLE_AUTH_RATE_LIMITS=1 \
  SESSION_SECURE=false \
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
# (set -e exits the script), stack-down.sh still reads STACK_MODE=isolated +
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
STACK_FRONTEND_URL=$STACK_FRONTEND_URL
STACK_FRONTEND_PORT=$FRONTEND_PORT
BACKEND_URL=$STACK_BACKEND_URL
NEXT_PUBLIC_API_URL=$STACK_FRONTEND_URL/api
EOF

log "Starting frontend on :$FRONTEND_PORT..."
cd "$WORKTREE_PATH/frontend"
# See the file header for the routing convention. NEXT_PUBLIC_API_URL points
# SSR + browser at the frontend's own /api proxy; BACKEND_URL is what that
# proxy (and the per-route admin/AI proxies under app/api/admin/* and
# app/api/ai/*) forwards to. Without BACKEND_URL the proxy defaults to :8080
# and misses the per-worktree backend port.
nohup env NEXT_PUBLIC_API_URL="$STACK_FRONTEND_URL/api" \
  BACKEND_URL="$STACK_BACKEND_URL" \
  bun run dev --port "$FRONTEND_PORT" \
  </dev/null >"$STACK_DIR/frontend.log" 2>&1 &
echo $! > "$STACK_DIR/frontend.pid"
disown 2>/dev/null || true

log "Waiting for frontend at $STACK_FRONTEND_URL..."
wait_for_url "$STACK_FRONTEND_URL" 60

cat "$STACK_DIR/.env"
log "Stack up (mode=isolated): $STACK_FRONTEND_URL -> $STACK_BACKEND_URL -> postgres :$POSTGRES_PORT"
