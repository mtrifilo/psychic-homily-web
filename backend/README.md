# Psychic Homily Backend API

A Go-based REST API for the Psychic Homily music website, built with Huma framework and PostgreSQL.

## Features

- RESTful API with OpenAPI documentation
- PostgreSQL database with GORM ORM
- Docker containerization
- Database migrations with golang-migrate
- Graceful shutdown handling
- CORS support for frontend integration
- Automated backups to Google Cloud Storage
- Production deployment scripts

## Tech Stack

- **Framework**: Huma v2 (Go)
- **Database**: PostgreSQL 17.5
- **ORM**: GORM
- **Migrations**: golang-migrate
- **Containerization**: Docker & Docker Compose
- **Router**: Chi
- **Backup Storage**: Google Cloud Storage
- **Production**: Nginx, Let's Encrypt SSL

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.24+ (for local development)

### Using Docker (Recommended)

1. **Start all services (including migrations):**

   ```bash
   docker compose up -d

   // run everything except the app
   docker compose up db migrate pgadmin -d
   ```

2. **Check if services are running:**

   ```bash
   docker compose ps
   ```

3. **View logs:**

   ```bash
   docker compose logs -f
   ```

4. **Test the API:**
   ```bash
   curl http://localhost:8080/health
   ```

### Local Development

#### Option 1: Database in Docker, App Locally (Recommended)

This approach provides the fastest development cycle with hot reloading and better debugging.

1. **Start only the database and run migrations:**

   ```bash
   docker compose up -d db migrate
   ```

2. **Run the API locally:**

   ```bash
   go run cmd/server/main.go
   ```

3. **Test the API:**
   ```bash
   curl http://localhost:8080/health
   ```

#### Option 2: Everything in Docker

Use this for testing the full containerized environment.

```bash
docker compose up -d
```

#### Hot Reload Development (Optional)

For automatic restarts when you make code changes:

1. **Install air for hot reload:**

   ```bash
   go install github.com/cosmtrek/air@latest
   ```

2. **Run with hot reload:**
   ```bash
   air
   ```

#### Development Workflow

1. **Make code changes** - Edit your Go files
2. **Auto-restart** - If using `air`, the app restarts automatically
3. **Manual restart** - If using `go run`, restart the process (Ctrl+C, then `go run cmd/server/main.go`)
4. **Test changes** - `curl http://localhost:8080/health`

#### Environment Configuration

The app automatically loads the correct environment file:

- **Development**: `.env.development` (loaded when `NODE_ENV=development`)
- **Production**: `.env.production` (loaded when `NODE_ENV=production`)

The database connection is configured for Docker networking (`db:5432`) when running in containers.

## Test-only env flags

- **`ENABLE_TEST_FIXTURES=1`** (PSY-432): registers the admin-only `POST /admin/test-fixtures/reset` endpoint used by Playwright worker teardown to wipe a test user's mutable rows. The server **refuses to boot** with this flag set unless `ENVIRONMENT` is `test`, `ci`, or `development` (default-deny — any other value including unset, `production`, `staging`, `preview` causes startup to fail). The endpoint itself also requires an admin JWT, the `X-Test-Fixtures: 1` header, and a target user whose email ends in `@test.local`. Local dev normally leaves this flag unset; E2E global-setup enables it when spawning its private backend.
- **`DISABLE_AUTH_RATE_LIMITS=1`** (PSY-475): replaces the IP-scoped auth (10/min) + passkey (20/min) rate limiters with no-op middleware. Same default-deny `ENVIRONMENT` gate — startup panics if the flag is on in `production`/`staging`/`preview`/unset. Exists because all parallel Playwright workers share `127.0.0.1`, exhausting the per-IP budget and intermittently flaking `register.spec.ts` + `magic-link.spec.ts`. Production + staging keep the limiters; only test-env skips them.

## Deployment commands to run

> **Provisioning a new environment?** Read
> [Infrastructure Sizing and Monitoring](#infrastructure-sizing-and-monitoring)
> first. A production outage was caused by a Postgres volume that could not
> survive a write burst, and the volume looked fine right up until it filled.

### Development

docker-compose up -d

### Production

docker-compose -f docker-compose.prod.yml up -d

### Production with explicit env

NODE_ENV=production docker-compose -f docker-compose.prod.yml up -d

## Management Scripts

The project includes several scripts for common operations. All scripts are located in the `backend/scripts/` directory.

### **Backup and Restore Scripts**

#### **backup-to-gcs.sh**

Creates a database backup and uploads it to Google Cloud Storage.

```bash
# Create backup and upload to GCS
./scripts/backup-to-gcs.sh

# Output:
# Backup created: backups/backup_20250119_020000.sql
# Backup uploaded to: gs://psychic-homily-backups/backups/backup_20250119_020000.sql
# Monthly cost: ~$0.12 (for daily backups)
```

**Features:**

- Creates PostgreSQL dump
- Uploads to Google Cloud Storage
- Keeps 7 days of local backups
- Keeps 30 days of remote backups
- Automatic cleanup of old backups

#### **restore-from-gcs.sh**

Restores database from a backup stored in Google Cloud Storage.

```bash
# Restore from specific backup
./scripts/restore-from-gcs.sh backup_20250119_020000.sql

# List available backups
./scripts/restore-from-gcs.sh

# Restore from GCS URL
./scripts/restore-from-gcs.sh gs://psychic-homily-backups/backups/backup_20250119_020000.sql
```

**Features:**

- Downloads backup from GCS if not local
- Supports both local and remote backup files
- Validates backup file existence
- Restores to production database

#### **verify-gcs-backups.sh**

Verifies the integrity of backups stored in Google Cloud Storage.

```bash
# Verify backup integrity
./scripts/verify-gcs-backups.sh

# Output:
# === Psychic Homily Backup Verification Report ===
# Date: Sat Jul 19 21:30:00 UTC 2025
#
# Local backups: 7
# Remote backups in GCS: 30
# Latest backup: gs://psychic-homily-backups/backups/backup_20250119_020000.sql
# ✅ Latest backup is accessible
#
# Monthly cost estimate: ~$0.12
```

**Features:**

- Counts local and remote backups
- Tests download accessibility
- Shows latest backup information
- Estimates monthly costs

### **Deployment Scripts**

#### **deploy-to-production.sh**

Deploys the application to production environment.

```bash
# Deploy to production
./scripts/deploy-to-production.sh

# Output:
# 🚀 Deploying Psychic Homily Backend...
# Generating strong database password...
# Waiting for database to be ready...
# ✅ Deployment complete!
# 🌐 API available at: https://psychichomily.com/api/
# 💾 Database backup created
```

**Features:**

- Generates strong passwords if not set
- Stops existing services
- Builds and starts new containers
- Runs database migrations
- Creates initial backup
- Waits for database health

#### **update-production.sh**

Updates the application in production with latest code.

```bash
# Update production
./scripts/update-production.sh

# Output:
# 🔄 Updating Psychic Homily Backend...
# ✅ Update complete!
```

**Features:**

- Pulls latest code from Git
- Rebuilds containers
- Runs migrations
- Minimal downtime deployment

### **Automated Backup Scheduling**

Set up automated daily backups:

```bash
# Edit crontab
crontab -e

# Add daily backup at 2 AM
0 2 * * * /path/to/your/project/backend/scripts/backup-to-gcs.sh >> /var/log/backup.log 2>&1

# Check backup logs
tail -f /var/log/backup.log
```

### **Script Usage Examples**

#### **Complete Backup Workflow**

```bash
# 1. Create backup
./scripts/backup-to-gcs.sh

# 2. Verify backup integrity
./scripts/verify-gcs-backups.sh

# 3. List available backups
./scripts/restore-from-gcs.sh

# 4. Restore if needed
./scripts/restore-from-gcs.sh backup_20250119_020000.sql
```

#### **Production Deployment Workflow**

```bash
# 1. Deploy to production
./scripts/deploy-to-production.sh

# 2. Update with new code
./scripts/update-production.sh

# 3. Verify deployment
curl https://api.psychichomily.com/health/ready   # 503 = up but a dependency is unreachable
```

#### **Emergency Recovery**

```bash
# 1. Check available backups
./scripts/restore-from-gcs.sh

# 2. Restore from latest backup
./scripts/restore-from-gcs.sh $(gsutil ls gs://psychic-homily-backups/backups/ | tail -1)

# 3. Verify restoration
docker compose -f docker-compose.prod.yml exec db psql -U $POSTGRES_USER -d $POSTGRES_DB -c "SELECT COUNT(*) FROM shows;"
```

### **Script Configuration**

All scripts use environment variables from `.env.production`:

```bash
# Required environment variables
DATABASE_URL=postgres://psychicadmin:${PROD_DB_PASSWORD}@localhost:5432/psychicdb_prod
POSTGRES_USER=psychicadmin
POSTGRES_PASSWORD=${PROD_DB_PASSWORD}
POSTGRES_DB=psychicdb_prod
GCS_BUCKET=psychic-homily-backups
```

### **Troubleshooting Scripts**

#### **Common Issues**

**Backup fails:**

```bash
# Check GCS authentication
gcloud auth application-default login

# Verify bucket exists
gsutil ls gs://psychic-homily-backups/

# Check environment variables
cat .env.production
```

**Restore fails:**

```bash
# Check backup file exists
gsutil ls gs://psychic-homily-backups/backups/ | grep backup_20250119_020000.sql

# Verify database connection
docker compose -f docker-compose.prod.yml exec db pg_isready -U $POSTGRES_USER
```

**Deployment fails:**

```bash
# Check Docker Compose file
docker compose -f docker-compose.prod.yml config

# View deployment logs
docker compose -f docker-compose.prod.yml logs api
```

## Database Migrations

### Migration Workflow

The project uses `golang-migrate` for database schema management, automatically run via Docker Compose.

#### **First Time Setup**

```bash
# Start database and run all migrations
docker compose up -d db migrate

# Verify migrations completed
docker compose logs migrate
```

#### **Creating New Migrations**

```bash
# Create new migration files
migrate create -ext sql -dir db/migrations -seq add_user_table

# This creates:
# db/migrations/000002_add_user_table.up.sql
# db/migrations/000002_add_user_table.down.sql
```

#### **Running Migrations**

```bash
# Run all pending migrations
docker compose run --rm migrate

# Run migrations and see output
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" up

# Check migration status
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" version

# Check migration history
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" version
```

#### **Rolling Back Migrations**

```bash
# Rollback one migration
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" down 1

# Rollback to specific version
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" goto 1

# Rollback all migrations
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" down
```

#### **Development Reset**

```bash
# Complete reset (removes all data and runs migrations)
docker compose down -v
docker compose up -d

# Reset database only (keeps volumes)
docker compose down
docker compose up -d db migrate
```

### Migration Best Practices

#### **File Naming Convention**

```markdown
db/migrations/
├── 000001_create_initial_schema.up.sql
├── 000001_create_initial_schema.down.sql
├── 000002_add_user_table.up.sql
├── 000002_add_user_table.down.sql
└── ...
```

#### **Migration Guidelines**

- **Always create both `.up.sql` and `.down.sql` files**
- **Test rollbacks** before committing migrations
- **Use descriptive names** that explain the change
- **Keep migrations small and focused** - one logical change per migration
- **Never modify existing migrations** - create new ones instead
- **Use transactions** for complex migrations

## Essential Docker Compose Commands

### Basic Operations

```bash
# Start all services in background
docker compose up -d

# Start and see logs
docker compose up

# Stop all services
docker compose down

# Stop and remove volumes (WARNING: deletes database data)
docker compose down -v

# Build images
docker compose build

# Build and start
docker compose up -d --build

# Force rebuild (ignores cache)
docker compose build --no-cache
```

### Service Management

```bash
# Start specific service
docker compose up -d api
docker compose up -d db

# Stop specific service
docker compose stop api

# Restart specific service
docker compose restart api

# Check running services
docker compose ps
```

### Logs and Monitoring

```bash
# All services logs
docker compose logs

# Follow logs in real-time
docker compose logs -f

# Specific service logs
docker compose logs api
docker compose logs db

# Last N lines
docker compose logs --tail=50
```

### Database Operations

```bash
# Connect to Postgres directly
docker compose exec db psql -U psychicadmin -d psychicdb

# Create database backup
docker compose exec db pg_dump -U psychicadmin psychicdb > backup.sql

# Run SQL file
docker compose exec -T db psql -U psychicadmin -d psychicdb < backup.sql

# Check database health
docker compose exec db pg_isready -U psychicadmin
```

### Development Workflows

```bash
# Fresh start (removes all data)
docker compose down -v
docker compose up -d --build

# Quick restart (API changes)
docker compose restart api

# Reset database only
docker compose down
docker compose up -d db

# Access container shells
docker compose exec api sh
docker compose exec db bash
```

## API Endpoints

### Health Check

Two endpoints, deliberately split. **They are not interchangeable — see
[Infrastructure Sizing and Monitoring](#infrastructure-sizing-and-monitoring)
before pointing anything at either.**

```bash
GET|HEAD /health        # liveness  — ALWAYS 200 while the process serves
GET|HEAD /health/ready  # readiness — 503 when a critical dependency is unreachable
```

`/health` is Railway's **deploy** healthcheck (`railway.toml`). It gates a new
deployment going live — if it never returns 200 the deployment is marked failed
and the previous one keeps serving. Railway does **not** poll it after a
deployment is live, so it is not a runtime monitor and a 503 there would not
restart anything. What it *would* do is block every deploy for as long as the
database was down, which is why it reports component detail in the body and
never in its status code.

**`/health/ready` is the endpoint uptime monitoring and alerting should
watch**; no deploy is gated on its result.

Both answer GET and HEAD. Use the exact paths — **no trailing slash**
(`/health/ready/` is a 404, not a redirect).

**Response (both, when healthy — HTTP 200):**

```json
{
  "$schema": "https://<api host>/schemas/HealthResponseBody.json",
  "status": "healthy",
  "components": {
    "database": { "status": "healthy", "latency": "1.23ms" }
  },
  "timestamp": "2026-01-15T10:30:00Z"
}
```

`$schema` is injected into every response body by Huma's schema-link
transformer (see `internal/api/routes/routes.go`) — it is part of the wire
contract, not an artifact of this example.

With the database unreachable, `/health` returns the same shape with
`"status": "unhealthy"` and **still HTTP 200**; `/health/ready` returns **HTTP
503** and a problem+json body whose `detail` names each failing component
(`not ready: database: ping failed`).

The dependency probe is bounded at 2s, so a database that hangs rather than
refuses still produces a 503 rather than an open connection. Component error
strings are fixed literals — no host, DSN, or driver text reaches these
responses.

Both paths are exempt from the public-read rate limiter
(`infraPathsExemptFromRateLimit`). That list is **exact-match** — a new health
path, or a rename, needs an entry there or probes land on the anonymous per-IP
budget and a 429 pages someone about a healthy service.

### Submit Show

```bash
POST /shows
Content-Type: application/json
```

**Request Body:**

```json
{
  "artists": [
    {
      "name": "Psychic Homily",
      "instagram": "psychichomily",
      "bandcamp": "psychichomily.bandcamp.com"
    }
  ],
  "venue": "Valley Bar",
  "date": "2025-01-15",
  "cost": "15",
  "ages": "21+",
  "city": "Phoenix",
  "state": "AZ"
}
```

**Response:** the created show, as `contracts.ShowResponse`
(`internal/api/handlers/catalog/show.go`). Rather than reproduce it here where
it would drift, read the generated contract in `frontend/types/api.d.ts` under
`post-shows`, or the OpenAPI document.

> Huma serializes a handler's `Body` field **as** the response body — there is
> no `{"body": …}` envelope on the wire, and every body carries an injected
> `$schema`. Older examples in this file predate that correction; trust
> `frontend/types/api.d.ts` (generated from the live OpenAPI document) over any
> hand-written example here.

## Environment Variables

### Environment Files

The project uses environment-specific configuration files:

- `.env.example` - Template with all available variables
- `.env.development` - Development environment settings
- `.env.production` - Production environment settings

### Setting Environment

```bash
# Development (default)
NODE_ENV=development docker compose up

# Production
NODE_ENV=production docker compose up
```

### Environment Variables

| Variable               | Default                     | Description                              |
| ---------------------- | --------------------------- | ---------------------------------------- |
| `NODE_ENV`             | `development`               | Environment name                         |
| `API_ADDR`             | `127.0.0.1:8080`            | API server address                       |
| `API_PORT`             | `8080`                      | API server port                          |
| `DATABASE_URL`         | `postgres://...`            | Database connection string               |
| `POSTGRES_USER`        | `psychicadmin`              | Database username                        |
| `POSTGRES_PASSWORD`    | `secretpassword`            | Database password                        |
| `POSTGRES_DB`          | `psychicdb`                 | Database name                            |
| `POSTGRES_HOST`        | `db`                        | Database host                            |
| `POSTGRES_PORT`        | `5432`                      | Database port                            |
| `CORS_ALLOWED_ORIGINS` | `https://psychichomily.com` | Comma-separated CORS origins             |
| `LOG_LEVEL`            | `debug`                     | Logging level (debug, info, warn, error) |

### Background Service Env Flags

Each scheduled background service in `cmd/server/main.go` can be individually
disabled at startup by setting the corresponding `DISABLE_*` env var to `"1"`.
Any other value (including unset) leaves the service enabled, so local
`go run ./cmd/server` keeps starting everything by default.

The frontend E2E harness (`frontend/e2e/global-setup.ts`) sets all of these flags
to `"1"` so the E2E backend runs lean: no scheduled tickers, no log spam,
no nondeterministic DB state changes from ambient background jobs. A new
`DISABLE_*` flag belongs there too.

| Variable                          | Disables                                                        |
| --------------------------------- | --------------------------------------------------------------- |
| `DISABLE_RADIO_FETCH`             | Radio playlist ingestion, affinity computation, re-matching     |
| `DISABLE_AUTO_PROMOTION`          | Daily user trust-tier evaluation / auto-promotion               |
| `DISABLE_ENRICHMENT_WORKER`       | Post-import enrichment worker (processes enrichment queue)      |
| `DISABLE_COLLECTION_DIGEST`       | Weekly collection-subscription digest emails (PSY-350)          |
| `DISABLE_CLEANUP`                 | Account cleanup service (permanent deletion of soft-deleted)    |
| `DISABLE_REMINDERS`               | Show reminder service (24h-before email reminders)              |
| `DISABLE_RELATIONSHIP_DERIVATION` | Derived artist relationships (shared_bills + shared_label)      |
| `DISABLE_STREET_GEOCODE_SWEEP`    | Daily venue street-geocode reconciliation via Nominatim (PSY-1544) |
| `DISABLE_SWEEP_HEALTH_CHECK`      | Overdue-sweep alerting — reports stopped background loops to Sentry (PSY-1612) |
| `DISABLE_SHOW_NOTIFY_OUTBOX`      | Follower notifications for ingest-created shows (PSY-1894). **The kill switch for outbound show email** |

`DISABLE_SHOW_NOTIFY_OUTBOX` gates the ENQUEUE as well as the drain, so setting it
stops rows being written rather than letting a backlog build for a burst on
re-enable. Both halves re-read it live, so it takes effect **without a restart**.

Two caveats for an incident. It is read **per process**, so an ingest CLI run from
elsewhere (`cmd/discovery-import`) needs it in that environment too. And rows
already `pending` when it was set will drain once it is cleared, so clear the
backlog first (`DELETE FROM show_notify_queue WHERE status = 'pending'`) if you
want a clean restart; that is safe precisely because those rows have not notified
anyone. Jobs older than `SHOW_NOTIFY_OUTBOX_MAX_JOB_AGE_HOURS` are dropped unsent
regardless, which bounds any burst.

To retry a job that went `failed`, reuse the row rather than deleting it:
`UPDATE show_notify_queue SET status='pending', attempts=0, last_error=NULL WHERE id=...`.
`DELETE` drops the "already considered" record, so a later re-ingest of that show
could notify people the first pass had already reached.

Tuning knobs: `SHOW_NOTIFY_OUTBOX_INTERVAL_SECONDS` (default `60`),
`SHOW_NOTIFY_OUTBOX_BATCH` (default `5` shows per tick),
`SHOW_NOTIFY_OUTBOX_STALE_RECLAIM_MINUTES` (default `30`, and **not measured**:
see the poller's `reclaimStale` doc), `SHOW_NOTIFY_OUTBOX_MAX_JOB_AGE_HOURS`
(default `24`).

The street-geocode sweep's cadence and per-run network budget are tunable:
`STREET_GEOCODE_SWEEP_INTERVAL_HOURS` (default `24`),
`STREET_GEOCODE_SWEEP_LIMIT` (default `25` lookups per run) and
`STREET_GEOCODE_SWEEP_START_DELAY_MINUTES` (default `15`). The start delay is
now only a **fallback**: the sweep's schedule comes from persisted run state
(see below), so it is used solely when there is no such state yet — a fresh
database, or a run-state table that cannot be read. It shares the
process-wide Nominatim client (and its 1 req/s limiter) with inline venue
write-path geocoding, so it is safe alongside live traffic; large backlogs
should still use the `geocode-venue-addresses` CLI off-hours.

### Background Service Scheduling (`background_service_runs`)

Every scheduled loop with an interval of **an hour or more** records its state in
the `background_service_runs` table — one row per loop name, holding both the
schedule (`last_completed_at`) and the run trace (`last_started_at`,
`last_success_at`, outcome, error, duration, rows processed,
`consecutive_failures`, `run_count`, the configured `interval_seconds`).

**Why it exists.** Scheduling used to live in a process-local ticker anchored at
process start, so a loop that did not run a cycle at boot waited a full interval
measured from that moment. On a continuously-deployed platform the process
restarts far more often than a daily sweep's interval, so the first cycle was
never reached: seven production sweeps ran exactly one cycle in the life of
production, two never ran at all, and nothing errored or alerted the whole time.
Reading the schedule from the database instead means a redeploy cannot reset it —
a restarted process asks "am I overdue?" rather than starting the clock again.

Operational notes:

- A loop that is overdue at boot runs a catch-up cycle, **staggered** against its
  siblings so several overdue sweeps do not hit MusicBrainz / Nominatim / Spotify
  at once. Tunable via `SWEEP_CATCHUP_BASE_SECONDS` (default `60`),
  `SWEEP_CATCHUP_SPACING_SECONDS` (default `90`) and `SWEEP_CATCHUP_MAX_SECONDS`
  (default `1800`).
- Cycles are claimed atomically, so a deploy's healthcheck-gated overlap (old and
  new instance briefly both running) cannot double-run a sweep. A claim left
  behind by a crashed process is reclaimable after its lease.
- A failed cycle still stamps `last_completed_at`, so it retries on the normal
  cadence rather than firing on every deploy; `last_success_at` and
  `consecutive_failures` carry the health signal instead.
- Loops with a sub-hour interval (`enrichment_worker` 30s, `image_enrich_outbox`
  60s, `radio_slot_fetch` 10m, `reminder` 30m) deliberately write no rows — their
  first cycle already fits inside any plausible uptime, so persistence would be
  pure overhead. **This list is also the coverage map for overdue alerting
  below: a loop with no row cannot be reported as stopped.** Lowering a monitored
  sweep's interval below one hour therefore removes its alerting — the loop
  retires its own row on the next boot, so it goes quiet rather than paging
  against its stale cadence.
- To answer "when did this last run?" for any loop:
  `SELECT name, last_completed_at, last_success_at, last_outcome, consecutive_failures FROM background_service_runs ORDER BY name;`

**Overdue-sweep alerting (PSY-1612).** A `sweep_health_check` loop runs every 15
minutes (`SWEEP_HEALTH_CHECK_INTERVAL_MINUTES`) and reports loops that have
stopped running to Sentry. Turn it off with `DISABLE_SWEEP_HEALTH_CHECK=1`.

- **Detection bound.** A loop is overdue once
  `max(2 × interval, interval + lease + margin)` has passed with no completed
  cycle, where `margin = SWEEP_CATCHUP_MAX_SECONDS (30m) + 15m jitter`. Reporting
  then happens on the next check pass, so add up to one `SWEEP_HEALTH_CHECK_INTERVAL_MINUTES`.
  Worked examples with the defaults (lease 1h, margin 45m):
  **1h sweep → 2h45m threshold, reported within ~3h**; **24h sweep → 48h
  threshold, reported within ~48h15m**. The floor exists because a process killed
  mid-cycle holds its claim for a full lease, so a perfectly healthy recovery can
  take `interval + lease` — without it every SIGKILL was a coin-flip false page.
  Note `SWEEP_CATCHUP_MAX_SECONDS` feeds the margin, so raising it widens every
  threshold.
- **Throttling.** One report when a loop crosses healthy → overdue, then at most
  one re-assert per 24h (`SWEEP_OVERDUE_REALERT_HOURS`) while it stays overdue.
  Any completed cycle — success *or* failure — clears the throttle, so a later
  stall reports immediately instead of inheriting a half-spent window.
- **Retirement.** Rows are only reported while some live process still declares
  the loop (the health check re-stamps `last_registered_at` for the loops it owns
  on every pass). A row nobody re-registers for 7 days stops being reported —
  which covers renames, and rows carried in by `psy-deploy-prod --with-db-restore`
  from stage, where extra `ENABLE_*` sweeps run. **Consequences to plan for:**
  switching off a sweep that has previously run here keeps paging daily until it
  retires, and a `--with-db-restore` release carries stage's registration
  timestamps over, so stage-only sweeps read as stalled in prod for the window.
  To retire immediately:

  ```sql
  UPDATE background_service_runs SET last_registered_at = NULL WHERE name = '<loop name>';
  ```

  **Do not `DELETE` the row.** If any running process still owns that loop the
  health check re-creates it within one pass, with `created_at = NOW()` — which
  hands a genuinely stalled sweep a *fresh* grace window (so it stops paging
  exactly when it should not), makes it report as `never_ran` afterwards, and
  discards its run history. The `UPDATE` above retires without resetting the
  schedule, and is safe even if the loop is still wired (it re-registers on the
  next pass).
- **Not covered — the detector's own liveness.** Nothing reports that the health
  check itself stopped: it holds no run-state row by design (so it cannot monitor
  itself), which means `DISABLE_SWEEP_HEALTH_CHECK=1` left set, or its goroutine
  stopped by the loop-stopping outer recover, removes all coverage with only a
  boot log line as evidence. Tracked in PSY-1636.
- **Not covered — a process killed mid-report.** The throttle stamp commits before
  the alert is delivered (it must, or two replicas both report). A handler panic
  and an in-progress shutdown both release the claim for retry, but a SIGKILL or
  OOM between the commit and delivery does not — that sweep stays silent for up to
  one re-assert window. So the honest bound is "within the detection bound, or up
  to `SWEEP_OVERDUE_REALERT_HOURS` later if a process dies mid-report".
- **Not covered:** a loop that runs and *fails* every cycle. It keeps stamping
  `last_completed_at`, so it never reads as overdue and nothing pages. Tracked in
  PSY-1620.

**Opt-in (default OFF) — image enrichment sweep (PSY-1246).** Unlike the
`DISABLE_*` services above, the ongoing image-enrichment sweep is gated by an
**`ENABLE_*`** flag and does **not** run unless explicitly turned on: image
enrichment is paused at the hotlink tier pending a product signal and display is
gated on PSY-1242, so it must not auto-start in prod. (E2E / dispatch harnesses
that set the `DISABLE_*` flags need no change — it stays off by default.)

| Variable                            | Default     | Effect                                                          |
| ----------------------------------- | ----------- | --------------------------------------------------------------- |
| `ENABLE_IMAGE_ENRICH_SWEEP`         | unset (off) | `"1"` starts the sweep (fills missing artist photos + covers)   |
| `IMAGE_ENRICH_SWEEP_INTERVAL_HOURS` | `24`        | Tick cadence                                                    |
| `IMAGE_ENRICH_SWEEP_BATCH`          | `50`        | Entities per type processed per tick                            |
| `IMAGE_ENRICH_SWEEP_REATTEMPT_DAYS` | `90`        | Don't re-attempt an imageless entity for this many days         |

**Opt-in (default OFF) — artist-location sweep (PSY-1250).** Same `ENABLE_*`
posture as the image sweep, for the same reason: the resolver AUTO-WRITES a
name-matched location and the manual `backfill-artist-location` cmd's dry-run
review is the homonym backstop, so this runs only where explicitly enabled (enable
on stage first, watch the report, then prod). Location only — links are a follow-up
(PSY-1279). Keep `REATTEMPT_DAYS` ≫ `INTERVAL_HOURS × (locationless tail / batch)`
or the memo is defeated and the tail is re-queried. **This flag is the location-
enrichment FEATURE switch:** `=1` enables BOTH the nightly sweep AND PSY-1251's
eager on-create enrichment (per-create MusicBrainz calls, off the request goroutine,
for interactively-created artists — admin create + entity-request fulfillment).

| Variable                               | Default     | Effect                                                          |
| -------------------------------------- | ----------- | --------------------------------------------------------------- |
| `ENABLE_ARTIST_LOCATION_SWEEP`         | unset (off) | `"1"` enables location enrichment: the nightly sweep + on-create (PSY-1251) |
| `ARTIST_LOCATION_SWEEP_INTERVAL_HOURS` | `24`        | Tick cadence                                                    |
| `ARTIST_LOCATION_SWEEP_BATCH`          | `50`        | Artists processed per tick                                      |
| `ARTIST_LOCATION_SWEEP_REATTEMPT_DAYS` | `30`        | Don't re-attempt a locationless artist for this many days       |

### Security Notes

- **Never commit `.env.production`** to version control
- **Use strong passwords** in production
- **Rotate credentials** regularly
- **Use secrets management** in production (Docker Secrets, Kubernetes Secrets, etc.)

## Infrastructure Sizing and Monitoring

**Read this before provisioning a new environment.** A production outage was
caused by a volume that could never have survived a write burst, and nothing
alerted — the API was fully down while cached frontend pages kept serving 200s.

### The volume sizing rule

```
volume >= pg_database_size + max_wal_size + filesystem overhead,  with margin
```

`max_wal_size` is a **soft ceiling Postgres is entitled to reach** (and can
exceed under load), not a
high-water mark it works up to. If it exceeds free space, the volume is already
doomed at 0% used — the first sustained burst of `UPDATE`s fills it. A
percentage-usage alert cannot catch that: usage looks fine right up until
checkpoint churn claims the headroom the config always allowed it to claim.

So there are **two** checks, and the floor is the one that catches a
misconfiguration:

1. **Floor (catches misconfiguration).** Does `data + max_wal_size + overhead`
   fit the volume with margin? Verify at provisioning time, not from a graph.
2. **Percentage (catches gradual growth).** This is the second check, not the
   first.

**`scripts/check-volume-headroom.sh` is the canonical implementation of both** —
prefer it over doing this by hand, and treat its defaults as the numbers of
record rather than restating them here:

```bash
cd backend && bash ../scripts/check-volume-headroom.sh production
```

**Precondition:** the working directory, or one of its ancestors, must be
`railway link`ed to this project — the CLI resolves the project and environment
from that link, which is machine-local state in `~/.railway/config.json`, not
anything this repo carries. Run it from `backend/` if that is what you have
linked.

**It mutates the link while it runs.** `railway volume list` has no environment
flag, so the script links to the target environment, reads, and relinks on exit.
A hard kill skips the relink and leaves the directory pointed at whatever it was
inspecting — quite possibly production. If it is interrupted, run `railway
status` and re-link before any other `railway` command.

**Its exit codes are not fully trustworthy from an unlinked directory.** Measured:
from a directory with no link the script exits **1**, silently, rather than the
3 the table below implies — `set -euo pipefail` aborts on the first `railway`
call before the guard that would have exited 3. Since the table reads 1 as
"below the floor, STOP and resize", a check that never ran can present as a
capacity failure. Confirm the link before trusting any result.

| Exit | Meaning | Action |
| --- | --- | --- |
| `0` | clears the floor, under the usage threshold | proceed |
| `1` | **below the floor** — cannot hold the database plus a full WAL cycle | **STOP** — resize before deploying |
| `2` | clears the floor, usage past the threshold | proceed, but plan a resize |
| `3` | **could not determine** — missing `psql`/`jq`/`railway`, environment not linkable, volume name mismatch, no `DATABASE_PUBLIC_URL`, **or the database is unreachable** | **STOP** — an unrun check is not a passed check, and an unreachable database is itself a finding |

Tunable via `VOLUME_CHECK_OVERHEAD_PCT`, `VOLUME_CHECK_USAGE_PCT`,
`VOLUME_CHECK_VOLUME` (the volume name, the usual cause of a "not found" exit 3)
and `VOLUME_CHECK_DB_SERVICE`. On any run that reaches its summary the script
prints the effective overhead and the computed floor — but note that the exit-1
and exit-3 paths bail out *before* that summary, so a failing run gives you no
values to read. Exit 3 deserves the same respect as exit 1: it means the volume
is *unverified*, which is the state the environment was in before the outage.

**What exit 0 does not cover.** The floor is computed from
`pg_database_size(current_database())` and `max_wal_size` only. Three gaps:

- **`max_wal_size` is a *soft* limit.** Write load that outruns checkpointing can
  exceed it even with nothing pinning WAL. The overhead margin is what absorbs
  this, which is why you should not provision at exactly the floor.
- **Pinned WAL isn't counted.** A non-zero `wal_keep_size`, or any replication
  slot — especially an inactive one, which retains WAL indefinitely — lets WAL
  grow past `max_wal_size` entirely.
- **The floor counts one database, not the cluster.** The volume holds all of
  PGDATA: other databases, logs, temp files. Currently a small gap absorbed by
  the overhead margin; it stops being small the moment a second application
  database lands on the same Postgres.

Check these by hand when provisioning, with the query below.

To inspect the inputs by hand:

```sql
SELECT pg_size_pretty(sum(pg_database_size(datname))) AS cluster_data,
       current_setting('max_wal_size')                     AS max_wal,
       current_setting('min_wal_size')                     AS min_wal,
       current_setting('wal_keep_size')                    AS wal_keep,
       (SELECT count(*) FROM pg_replication_slots)         AS slots
  FROM pg_database;
```

`max_wal_size` is only the bound if nothing is **pinning** WAL. A non-zero
`wal_keep_size`, or any replication slot (especially an inactive one, which
retains WAL indefinitely), lets WAL grow past that ceiling — so the arithmetic
above no longer holds. Both must be checked, not assumed.

Note that WAL volume tracks **write churn, not row count**. A backfill that
rewrites existing rows generates far more WAL than one that inserts new ones,
and a mass `DELETE` is a write burst too — which is why large retention
operations should be a partition `DETACH` rather than a `DELETE`.

Postgres on Railway is a managed service with no config file in this repo, so
`max_wal_size` is set through the platform rather than here. Set it
**explicitly**; a default that happens to fit today is not the same as a value
chosen against the volume.

> **Open question — the mechanism is not recorded yet.** When
> `check-volume-headroom.sh` fails, it offers two remedies: grow the volume, or
> lower `max_wal_size`. Only the first is documented anywhere (Railway
> dashboard; `railway volume update` has no size flag). Nobody has written down
> how `max_wal_size` is actually set on this Postgres service, or what value it
> runs today versus what was chosen against the current volume. Until someone
> does, treat "grow the volume" as the only executable remedy — and be aware
> that leaves the original misconfiguration (a ceiling inherited by default
> rather than chosen against the disk) in place.

### What watches what

**Automatic — runs without anyone remembering:**

| Signal | Watched by | On failure |
| --- | --- | --- |
| New deployment can serve | Railway deploy healthcheck → `/health` (`railway.toml`) | Fails the deploy; previous version keeps serving |
| Background sweeps running | `sweep_health_check` → Sentry | Pages a human |

**Manual — in this repo, but only runs if a human runs it. Nothing in CI or any
deploy pipeline invokes this:**

| Signal | Tool | On failure |
| --- | --- | --- |
| Volume floor + usage | `scripts/check-volume-headroom.sh` | Blocks the deploy **only if someone ran it** |

**Configured OUTSIDE this repo — nothing here can verify these are live. If you
are reading this during an incident and wondering why nobody was paged, check
that these exist before assuming an alert broke:**

| Signal | Configured in | On failure |
| --- | --- | --- |
| Dependencies reachable | External uptime monitor → `/health/ready` | Pages a human; **no restart** |
| Volume usage % (continuous) | Railway native volume alerts | Pages a human |

The external monitor must live **outside the platform**. The frontend is served
from a CDN cache that keeps returning 200s through a total API outage, so
checking the site's homepage proves nothing about the backend.

Configure it as: **`GET https://<api host>/health/ready`, alert on any non-200.**

- Alert on *any* non-200, not `== 503` specifically. A 503 is the healthy way to
  report "cannot do my job", but a connection failure or a timeout is the same
  outage and produces no status code at all. A rule written only against 503
  stays silent for the worst cases.
- Set the monitor's own request timeout **above** the probe's internal 2s
  dependency deadline, so a slow database returns a 503 you can read rather than
  a client-side timeout you have to guess at.
- Use the exact path, no trailing slash. Either GET or HEAD works.
- Point it at the **API host** (`api.psychichomily.com`), *not*
  `psychichomily.com/api/…`. That path goes through the frontend deployment,
  which is the thing you are trying to test independently of — a probe routed
  through it can be answered by a cache rather than by the backend. The
  endpoints send `Cache-Control: no-store`, but do not rely on an intermediary
  honouring it when you can just avoid the intermediary.

Do not point it at `/health`: that endpoint returns 200 by design whenever the
process is alive, so it stays green during exactly the outage you want detected.

## Database Schema

The API uses PostgreSQL with the following main tables:

- **artists** - Artist/band information
- **venues** - Venue information
- **shows** - Concert/show information
- **show_artists** - Many-to-many relationship between shows and artists

### Schema Migration History

- `000001_create_initial_schema` - Initial tables (artists, venues, shows, show_artists)

## Development

## Venue Discovery

The project includes an automated venue discovery system that imports show data from venue calendars.

### Components

- **Node.js Discovery** (`discovery/`) - Playwright-based discovery tool for TicketWeb venues
- **Go Importer** (`cmd/discovery-import/`) - CLI tool to import discovered JSON into the database
- **Systemd Timer** (`deploy/discovery/`) - Weekly scheduled runs on the server

### Usage

```bash
# Run discovery and import (from project root)
cd discovery
./run-discovery.sh

# Dry run (no database changes)
./run-discovery.sh --dry-run

# Import only (if you have JSON files)
cd backend
go build -o ./discovery-import ./cmd/discovery-import
./discovery-import -input ../discovery/output/discovered-events-*.json -dry-run
```

### Server Deployment

```bash
# Install systemd timer
sudo cp deploy/discovery/discovery.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now discovery.timer

# Manual run
sudo systemctl start discovery.service
journalctl -u discovery.service -f
```

See `docs/venue-discovery-design.md` for detailed documentation.

---

### Project Structure

```
backend/
├── cmd/
│   ├── server/
│   │   └── main.go              # Application entry point
│   └── discovery-import/
│       └── main.go              # Venue discovery importer CLI
├── internal/
│   ├── api/
│   │   ├── handlers/            # HTTP handlers
│   │   │   └── health.go        # Health endpoint
│   │   └── routes/              # Route definitions
│   │       └── routes.go        # All routes setup
│   └── config/                  # Configuration management
│       └── config.go            # Environment variable handling
├── db/
│   └── migrations/              # Database migration files
│       ├── 000001_create_initial_schema.up.sql
│       └── 000001_create_initial_schema.down.sql
├── scripts/                     # Management scripts
│   ├── backup-to-gcs.sh         # Backup to Google Cloud Storage
│   ├── restore-from-gcs.sh      # Restore from GCS backup
│   ├── deploy-to-production.sh  # Deploy to production
│   ├── update-production.sh     # Update production
│   └── verify-gcs-backups.sh    # Verify backup integrity
├── docs/                        # Documentation
│   └── venue-discovery-design.md # Venue discovery architecture
├── deploy/                      # Deployment configurations
│   └── discovery/               # Systemd units for venue discovery
├── Dockerfile                   # Docker image definition
├── docker-compose.yml           # Docker Compose configuration
├── docker-compose.prod.yml      # Production Docker Compose
└── README.md                    # This file
```
