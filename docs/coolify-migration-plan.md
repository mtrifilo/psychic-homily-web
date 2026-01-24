# Coolify Migration Plan

## Overview

This document outlines the plan to migrate the Psychic Homily backend deployment from the current GitHub Actions + SCP + Systemd workflow to Coolify.

---

## Current Infrastructure Assessment

### Server Specifications

| Resource       | Current Value    | Notes                  |
| -------------- | ---------------- | ---------------------- |
| OS             | Ubuntu 24.10 x64 | DigitalOcean Droplet   |
| RAM            | 2GB              | ✅ Upgraded from 960MB |
| Disk           | 25GB             | Sufficient for Coolify |
| Docker         | 28.4.0           | ✅ Already installed   |
| Docker Compose | v2.39.2          | ✅ Already installed   |

### Current Deployment Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        DigitalOcean Droplet                        │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                         Nginx                                │   │
│  │   api.psychichomily.com → :8080 (production backend)       │   │
│  │   stage.api.psychichomily.com → :8081 (stage backend)      │   │
│  │   SSL via Let's Encrypt (Certbot)                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              │                                      │
│         ┌────────────────────┴────────────────────┐                │
│         ▼                                         ▼                │
│  ┌──────────────────┐                  ┌──────────────────┐        │
│  │ Systemd Service  │                  │ Systemd Service  │        │
│  │ (Go Binary)      │                  │ (Go Binary)      │        │
│  │ Port 8080        │                  │ Port 8081        │        │
│  └────────┬─────────┘                  └────────┬─────────┘        │
│           │                                     │                  │
│           ▼                                     ▼                  │
│  ┌──────────────────┐                  ┌──────────────────┐        │
│  │ Docker: Postgres │                  │ Docker: Postgres │        │
│  │ Docker: Redis    │                  │ Docker: Redis    │        │
│  │ (Production)     │                  │ (Stage)          │        │
│  └──────────────────┘                  └──────────────────┘        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

                              ▲
                              │ GitHub Actions
                              │ - Build Go binary
                              │ - SCP upload
                              │ - Run migrations
                              │ - Restart systemd
                              │
┌─────────────────────────────┴──────────────────────────────────────┐
│                        GitHub Repository                           │
└────────────────────────────────────────────────────────────────────┘
```

### Services to Migrate

| Service               | Current Setup           | Docker Port | Notes                                             |
| --------------------- | ----------------------- | ----------- | ------------------------------------------------- |
| Production Backend    | Go binary + systemd     | 8080        | api.psychichomily.com                             |
| Stage Backend         | Go binary + systemd     | 8081        | stage.api.psychichomily.com                       |
| Production PostgreSQL | Docker (postgres:17.5)  | 5432        | Volume: `psychic-homily-production_postgres_data` |
| Stage PostgreSQL      | Docker (postgres:18)    | 5433        | Volume: `psychic-homily-stage_ph_stage_data`      |
| Production Redis      | Docker (redis:7-alpine) | 6379        | Volume: `psychic-homily-production_redis_data`    |
| Stage Redis           | Docker (redis:7-alpine) | 6380        | Volume: `psychic-homily-stage_ph_stage_redis`     |

### Files to Deprecate After Migration

- `.github/workflows/deploy-production.yml`
- `.github/workflows/deploy-stage.yml` (if exists)
- `backend/scripts/deploy-production.sh`
- `backend/scripts/deploy-stage.sh` (if exists)
- `backend/scripts/sync-env-files.sh`
- `backend/systemd/psychic-homily-production.service`
- `backend/systemd/psychic-homily-stage.service`

---

## Pre-Migration Checklist

### ⚠️ Critical: RAM Upgrade Recommended

Coolify requires a minimum of **2GB RAM** (4GB recommended). Your current droplet has only **960MB**.

**Options:**

1. **Upgrade Droplet** (Recommended): Resize to 2GB+ RAM (~$12/month for 2GB)
2. **Proceed with 1GB** (Risky): May work with swap, but expect performance issues

To upgrade on DigitalOcean:

1. Power off droplet
2. Resize → Choose 2GB+ plan
3. Power on

### Database Backups

Before any migration, create full database backups:

```bash
# SSH into server
ssh mattcom

# Backup production database
docker exec psychic-homily-production-db-1 pg_dump -U <POSTGRES_USER> <POSTGRES_DB> > /opt/psychic-homily-production/backend/backups/pre-coolify-backup-$(date +%Y%m%d).sql

# Backup stage database
docker exec ph_stage_db pg_dump -U <POSTGRES_USER> <POSTGRES_DB> > /opt/psychic-homily-stage/backend/backups/pre-coolify-backup-$(date +%Y%m%d).sql
```

---

## Migration Plan

### Phase 1: Coolify Installation

#### Step 1.1: Install Coolify

```bash
ssh mattcom

# Run the official Coolify installation script
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash
```

This will:

- Install Coolify at `/data/coolify`
- Set up a Traefik reverse proxy (will replace Nginx)
- Create the Coolify dashboard on port 8000

#### Step 1.2: Initial Coolify Setup

1. Access Coolify at `http://<your-droplet-ip>:8000`
2. Create admin account
3. Configure the "localhost" server as your deployment target

#### Step 1.3: Configure Domain for Coolify (Optional but Recommended)

Add a DNS record for Coolify dashboard:

- `coolify.psychichomily.com` → Your droplet IP

Then configure in Coolify settings.

---

### Phase 2: Migrate Databases to Coolify

**Strategy: Create new Coolify-managed database services and restore from backups**

#### Step 2.1: Create PostgreSQL Services in Coolify

> **Note:** Coolify v4 UI uses Projects, not "Resources". Create a Project first, then add resources within it.

1. In Coolify → **Projects** → Create Project (e.g., `psychic-homily-production`)
2. Inside the project, click **+ Add** → Database → PostgreSQL
3. Create two PostgreSQL instances:
   - **Production DB**: `postgres:18.1-alpine`
   - **Stage DB**: `postgres:18.1-alpine`

4. Configure credentials. **Important notes:**
   - **Do NOT set Port Mappings** - internal Docker networking handles this
   - **Disable SSL** for internal connections (unless specifically needed)
   - The "Initial Database" field may not auto-create the database - you may need to create it manually

#### Step 2.2: Migrate Data

**Restore from backup** (Recommended approach)

```bash
# 1. Find the Coolify container IDs
sudo docker ps --format '{{.Names}}\t{{.Image}}' | grep postgres

# 2. Create the database if it doesn't exist (Coolify may not auto-create it)
sudo docker exec -i <container-id> psql -U <user> -d postgres -c 'CREATE DATABASE <dbname>;'

# 3. Restore from backup
cat /path/to/backup.sql | sudo docker exec -i <container-id> psql -U <user> -d <dbname>

# 4. Verify tables were created
sudo docker exec -i <container-id> psql -U <user> -d <dbname> -c '\dt'
```

Stage:

```
DB_HOST=
DB_PORT=5432
DB_USER=ph_stage_user
DB_NAME=psychic_homily_stage
```

Production:

```
DB_HOST=
DB_PORT=5432
DB_USER=psychic-db
DB_NAME=psychic-db-production
DB_PASSWORD=
```

#### Step 2.3: Create Redis Services in Coolify

1. In Coolify → Resources → New → Database → Redis
2. Create two Redis instances (production and stage)

Stage:

```
DB_HOST=
DB_PORT=5432
DB_USER=ph_stage_user
DB_NAME=psychic_homily_stage
REDIS_ADDR=ssowscg0oswg4kgck4s08wko:6379
```

Production

```
DB_HOST=
DB_PORT=5432
DB_USER=psychic-db
DB_NAME=psychic-db-production
DB_PASSWORD=
REDIS_ADDR=v00wooowo0c44sc0400ckg00:6379
```

---

### Phase 3: Deploy Backend Applications

#### Step 3.1: Connect GitHub Repository

1. Coolify → Sources → New → GitHub
2. Authorize Coolify GitHub App
3. Select `psychic-homily-web` repository

#### Step 3.2: Create Production Backend Application

1. Coolify → Resources → New → Application
2. Select GitHub as source
3. Configure:
   - **Repository**: `psychic-homily-web`
   - **Branch**: `main`
   - **Build Pack**: Dockerfile
   - **Dockerfile Location**: `backend/Dockerfile`
   - **Docker Context**: `backend`

4. Environment Variables (copy from `.env.production`):

   ```
   ENVIRONMENT=production
   API_ADDR=0.0.0.0:8080
   DB_HOST=<coolify-postgres-production-hostname>
   DB_PORT=5432
   POSTGRES_USER=<your_user>
   POSTGRES_PASSWORD=<your_password>
   POSTGRES_DB=<your_db>
   REDIS_ADDR=<coolify-redis-production-hostname>:6379
   # ... other env vars
   ```

5. Domain Configuration:
   - **Domain**: `api.psychichomily.com`
   - **SSL**: Let's Encrypt (Coolify handles this automatically via Traefik)

6. Health Check:
   - **Path**: `/health`
   - **Port**: `8080`

#### Step 3.3: Create Stage Backend Application

Repeat Step 3.2 with:

- **Branch**: `stage` (or `main` with different config)
- **Domain**: `stage.api.psychichomily.com`
- **Environment Variables**: Copy from `.env.stage`
- **Port**: `8081` (or 8080 since Coolify isolates containers)

#### Step 3.4: Configure Auto-Deploy

In each application settings:

1. Enable **Auto Deploy** on push to configured branch
2. Set up deployment webhooks (Coolify provides these)

---

### Phase 4: Database Migrations Strategy

#### Option A: Build-time Migrations (Recommended for Coolify)

Modify `backend/Dockerfile` to run migrations on startup:

```dockerfile
# Add to Dockerfile
COPY --from=builder /app/db/migrations ./db/migrations

# Create entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["./main"]
```

Create `backend/docker-entrypoint.sh`:

```bash
#!/bin/sh
set -e

# Run migrations
echo "Running database migrations..."
/app/migrate -path /app/db/migrations -database "$DATABASE_URL" up

# Start the application
exec "$@"
```

#### Option B: Coolify Pre-Deploy Commands

Configure in Coolify application settings:

- **Pre-deploy Command**: Run migration container before deployment

---

### Phase 5: Transition & Cleanup

#### Step 5.1: DNS Cutover

Once Coolify applications are tested:

1. Coolify's Traefik will handle SSL automatically
2. DNS remains the same (pointing to droplet IP)
3. Traefik will route based on hostname

#### Step 5.2: Remove Old Infrastructure

```bash
ssh mattcom

# Stop and disable old systemd services
sudo systemctl stop psychic-homily-production psychic-homily-stage
sudo systemctl disable psychic-homily-production psychic-homily-stage
sudo rm /etc/systemd/system/psychic-homily-*.service
sudo systemctl daemon-reload

# Stop old Docker containers (after verifying Coolify works)
docker compose -f /opt/psychic-homily-production/backend/docker-compose.prod.yml down
docker compose -f /opt/psychic-homily-stage/backend/docker-compose.stage.yml down

# Remove old Nginx configs (Traefik replaces Nginx)
sudo rm /etc/nginx/sites-enabled/psychic-homily-api
sudo rm /etc/nginx/sites-enabled/stage-api-psychichomily
sudo rm /etc/nginx/sites-available/psychic-homily-api
sudo rm /etc/nginx/sites-available/stage-api-psychichomily
sudo systemctl stop nginx
sudo systemctl disable nginx

# Archive old deployment directories
sudo mv /opt/psychic-homily-production /opt/archive-psychic-homily-production
sudo mv /opt/psychic-homily-stage /opt/archive-psychic-homily-stage

# Clean up orphaned Docker volumes (CAREFUL - verify first!)
docker volume prune
```

#### Step 5.3: Update GitHub Workflows

Either:

1. **Delete** `.github/workflows/deploy-production.yml` and related files
2. **Or** Update them to trigger Coolify webhooks instead

---

## Post-Migration Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        DigitalOcean Droplet                        │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                   Coolify + Traefik                          │   │
│  │   api.psychichomily.com → production-backend container      │   │
│  │   stage.api.psychichomily.com → stage-backend container     │   │
│  │   coolify.psychichomily.com → Coolify dashboard             │   │
│  │   SSL via Let's Encrypt (automatic)                         │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              │                                      │
│         ┌────────────────────┴────────────────────┐                │
│         ▼                                         ▼                │
│  ┌──────────────────┐                  ┌──────────────────┐        │
│  │ Docker Container │                  │ Docker Container │        │
│  │ (Go Backend)     │                  │ (Go Backend)     │        │
│  │ Production       │                  │ Stage            │        │
│  └────────┬─────────┘                  └────────┬─────────┘        │
│           │                                     │                  │
│           ▼                                     ▼                  │
│  ┌──────────────────┐                  ┌──────────────────┐        │
│  │ Coolify-managed  │                  │ Coolify-managed  │        │
│  │ PostgreSQL+Redis │                  │ PostgreSQL+Redis │        │
│  │ (Production)     │                  │ (Stage)          │        │
│  └──────────────────┘                  └──────────────────┘        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

                              ▲
                              │ GitHub Webhook (on push)
                              │ Coolify auto-deploys
                              │
┌─────────────────────────────┴──────────────────────────────────────┐
│                        GitHub Repository                           │
└────────────────────────────────────────────────────────────────────┘
```

---

## Rollback Plan

If Coolify migration fails:

1. **Re-enable old systemd services**:

   ```bash
   sudo systemctl enable psychic-homily-production psychic-homily-stage
   sudo systemctl start psychic-homily-production psychic-homily-stage
   ```

2. **Re-enable Nginx**:

   ```bash
   sudo systemctl enable nginx
   sudo systemctl start nginx
   ```

3. **Stop Coolify's Traefik** (to free port 80/443):

   ```bash
   docker stop coolify-proxy
   ```

4. **Restore database from backup** if needed

---

## Benefits of Coolify

| Feature              | Current (GitHub Actions) | Coolify                 |
| -------------------- | ------------------------ | ----------------------- |
| Deployment Trigger   | Manual workflow dispatch | Auto-deploy on push     |
| SSL Management       | Certbot (manual renewal) | Automatic via Traefik   |
| Rollback             | Manual (backup restore)  | One-click in UI         |
| Monitoring           | None                     | Built-in container logs |
| Database Management  | Manual Docker commands   | UI-based management     |
| Secrets Management   | `.env` files on server   | Encrypted in Coolify    |
| Multi-environment    | Separate directories     | Projects & Environments |
| Zero-downtime Deploy | Custom script            | Built-in                |

---

## Estimated Timeline

| Phase                         | Duration      | Notes                                 |
| ----------------------------- | ------------- | ------------------------------------- |
| RAM Upgrade                   | 15 mins       | DigitalOcean resize (requires reboot) |
| Phase 1: Install Coolify      | 30 mins       | Script-based installation             |
| Phase 2: Database Migration   | 1-2 hours     | Backup, restore, verify               |
| Phase 3: Backend Applications | 1 hour        | Configure both environments           |
| Phase 4: Migration Strategy   | 30 mins       | Update Dockerfile                     |
| Phase 5: Cleanup              | 30 mins       | Remove old infrastructure             |
| **Total**                     | **4-5 hours** | Plus testing time                     |

---

## Appendix A: Environment Variables Reference

### Production Environment Variables

```env
ENVIRONMENT=production
API_ADDR=0.0.0.0:8080
DB_HOST=<coolify-postgres-hostname>
DB_PORT=5432
POSTGRES_USER=
POSTGRES_PASSWORD=
POSTGRES_DB=
REDIS_ADDR=<coolify-redis-hostname>:6379
JWT_SECRET=
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
OAUTH_REDIRECT_URL=https://api.psychichomily.com/auth/google/callback
FRONTEND_URL=https://www.psychichomily.com
CORS_ALLOWED_ORIGINS=https://www.psychichomily.com,https://psychichomily.com
```

### Stage Environment Variables

```env
ENVIRONMENT=stage
API_ADDR=0.0.0.0:8080
DB_HOST=<coolify-postgres-hostname>
DB_PORT=5432
POSTGRES_USER=
POSTGRES_PASSWORD=
POSTGRES_DB=
REDIS_ADDR=<coolify-redis-hostname>:6379
JWT_SECRET=
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
OAUTH_REDIRECT_URL=https://stage.api.psychichomily.com/auth/google/callback
FRONTEND_URL=https://stage.psychichomily.com
CORS_ALLOWED_ORIGINS=https://stage.psychichomily.com,https://stage-psychic-homily.netlify.app
```

---

## Appendix B: Coolify Docker Compose Equivalent

For reference, this is approximately what Coolify generates:

```yaml
# This is managed by Coolify - do not edit directly
version: '3.8'

services:
  psychic-homily-backend-production:
    build:
      context: ./backend
      dockerfile: Dockerfile
    environment:
      - ENVIRONMENT=production
      # ... other env vars
    networks:
      - coolify
    labels:
      - 'traefik.enable=true'
      - 'traefik.http.routers.backend-prod.rule=Host(`api.psychichomily.com`)'
      - 'traefik.http.routers.backend-prod.tls.certresolver=letsencrypt'
    healthcheck:
      test: ['CMD', 'wget', '--spider', 'http://localhost:8080/health']
      interval: 30s
      timeout: 10s
      retries: 3

  postgres-production:
    image: postgres:17.5
    volumes:
      - postgres-prod-data:/var/lib/postgresql/data
    environment:
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
      - POSTGRES_DB=${POSTGRES_DB}
    networks:
      - coolify

  redis-production:
    image: redis:7-alpine
    volumes:
      - redis-prod-data:/data
    networks:
      - coolify

networks:
  coolify:
    external: true

volumes:
  postgres-prod-data:
  redis-prod-data:
```

---

## Next Steps

1. [ ] **Decide on RAM upgrade** - Strongly recommended before proceeding
2. [ ] **Create database backups** - Critical before any migration
3. [ ] **Schedule maintenance window** - 4-5 hours with potential downtime
4. [ ] **Test Coolify locally first** (optional) - Spin up a test VM
5. [ ] **Execute Phase 1** - Install Coolify
6. [ ] **Proceed through phases** - Follow this document

---

## Troubleshooting & Lessons Learned

### Permission Denied Error on Database Startup

If you see an error like:

```
bash: line 11: /data/coolify/databases/<id>/README.md: Permission denied
```

This is a known Coolify bug ([GitHub Issue #5199](https://github.com/coollabsio/coolify/issues/5199)). Fix with:

```bash
sudo chmod -R 775 /data/coolify
```

### Port Already Allocated Error

If you see:

```
Bind for 0.0.0.0:5433 failed: port is already allocated
```

This means the old database is still running on that port. **Solution:** Remove the Port Mappings field in Coolify - internal Docker networking doesn't need external port exposure.

### Database Not Auto-Created

Coolify's "Initial Database" field sometimes doesn't create the database. Manually create it:

```bash
sudo docker exec -i <container-id> psql -U <user> -d postgres -c 'CREATE DATABASE <dbname>;'
```

### Finding Container Hostnames

In Coolify, services connect via container ID as hostname. Find them with:

```bash
sudo docker ps --format '{{.Names}}\t{{.Image}}'
```

The container name (e.g., `z88gow84ogw00oc4c4c8808w`) is the hostname for `DB_HOST` or `REDIS_ADDR`.

### PostgreSQL Version Upgrades

SQL dumps from `pg_dump` are forward-compatible. Restoring a PostgreSQL 17 backup to PostgreSQL 18 works fine.

---

## Migration Progress Checklist

- [x] RAM Upgrade (960MB → 2GB)
- [x] Phase 1: Install Coolify
- [x] Phase 2: Database Migration
  - [x] Stage PostgreSQL created and data restored
  - [x] Production PostgreSQL created and data restored
  - [x] Stage Redis created
  - [x] Production Redis created
- [ ] Phase 3: Deploy Backend Applications
  - [x] Stage backend configured and deployed in Coolify
  - [ ] Production backend configured in Coolify
- [x] Phase 4: Configure Migrations
  - [x] Updated `backend/Dockerfile` with golang-migrate CLI (v4.19.1)
  - [x] Created `backend/docker-entrypoint.sh` for database wait + migrations
- [ ] Phase 5: Cleanup Old Infrastructure
  - [x] Stop old stage systemd service
  - [x] Stop old stage Docker containers (`ph_stage_db`, `ph_stage_redis`)
  - [x] Remove old stage Nginx config
  - [x] Archive old stage directory
  - [ ] Stop old production systemd service
  - [ ] Stop old production Docker containers
  - [ ] Remove old production Nginx config
  - [ ] Archive old production directory
  - [ ] Disable Nginx service entirely

---

## Detailed Next Steps

### Stage Backend — Post-Deployment Cleanup

The stage backend is now running on Coolify. Complete the following cleanup steps:

#### 1. Verify Stage is Working on Coolify ✅ COMPLETE

```bash
# Test health endpoint
curl https://stage.api.psychichomily.com/health
# Result: {"status":"ok"}

# Test API endpoints
curl https://stage.api.psychichomily.com/api/venues
# Result: Returns venue data successfully
```

**Status:** Stage API is fully functional on Coolify.

#### 2. Stop Old Stage Infrastructure ✅ COMPLETE

**Systemd service:** ✅ Stopped and disabled

**Docker containers:** ✅ Stopped and removed (`ph_stage_db`, `ph_stage_redis`)

#### 3. Remove Old Stage Nginx Config ✅ COMPLETE

Nginx config files removed:

- `/etc/nginx/sites-enabled/stage-api-psychichomily`
- `/etc/nginx/sites-available/stage-api-psychichomily`

#### 4. Archive Old Stage Directory ✅ COMPLETE

Old stage directory archived to `/opt/archive-psychic-homily-stage`

---

### ⚠️ Production API Status

**Current Status: DOWN (Expected)**

The production API (`api.psychichomily.com`) is currently offline. This is expected during the transition:

- Old systemd service (`psychic-homily-production`) has been disabled
- Coolify production backend has not yet been deployed
- Production will be restored once the Coolify backend deployment is complete

**To restore production:** Complete the "Production Backend — Deployment Steps" section below.

---

### Production Backend — Deployment Steps

#### 1. Create Production Backend Application in Coolify

1. In Coolify dashboard → **Projects** → Select/create `psychic-homily-production`
2. Click **+ Add** → **Application** → **GitHub**
3. Select your repository and configure:

| Setting             | Value                |
| ------------------- | -------------------- |
| Repository          | `psychic-homily-web` |
| Branch              | `main`               |
| Build Pack          | Dockerfile           |
| Dockerfile Location | `backend/Dockerfile` |
| Docker Context      | `backend`            |

#### 2. Configure Environment Variables

Add these environment variables in Coolify (reference `.env.production` for secrets):

```env
ENVIRONMENT=production
API_ADDR=0.0.0.0:8080

# Database (Coolify-managed PostgreSQL)
DB_HOST=nkg0wo808kcosog4004ow8cc
DB_PORT=5432
POSTGRES_USER=psychic-db
POSTGRES_PASSWORD=<from .env.production>
POSTGRES_DB=psychic-db-production
DATABASE_URL=postgres://psychic-db:<password>@nkg0wo808kcosog4004ow8cc:5432/psychic-db-production?sslmode=disable

# Redis (Coolify-managed)
REDIS_ADDR=v00wooowo0c44sc0400ckg00:6379

# Authentication
JWT_SECRET=<from .env.production>
JWT_EXPIRY_HOURS=24
SESSION_SECURE=true
SESSION_DOMAIN=psychichomily.com

# Google OAuth
GOOGLE_CLIENT_ID=<from .env.production>
GOOGLE_CLIENT_SECRET=<from .env.production>
OAUTH_REDIRECT_URL=https://api.psychichomily.com/auth/google/callback

# CORS & Frontend
FRONTEND_URL=https://www.psychichomily.com
CORS_ALLOWED_ORIGINS=https://www.psychichomily.com,https://psychichomily.com

# Logging
LOG_LEVEL=info
```

#### 3. Configure Domain & Health Check

In Coolify application settings:

| Setting           | Value                                 |
| ----------------- | ------------------------------------- |
| Domain            | `api.psychichomily.com`               |
| SSL               | Automatic (Let's Encrypt via Traefik) |
| Health Check Path | `/health`                             |
| Health Check Port | `8080`                                |

#### 4. Deploy and Verify

1. Click **Deploy** in Coolify
2. Monitor the build logs for any errors
3. Once deployed, verify:

```bash
# Test health endpoint
curl https://api.psychichomily.com/health

# Test API functionality
curl https://api.psychichomily.com/api/venues

# Test OAuth flow by logging in via the production frontend
```

#### 5. Stop Old Production Infrastructure

**Only after verifying production is working on Coolify:**

```bash
ssh mattcom

# Stop and disable the old systemd service
sudo systemctl stop psychic-homily-production
sudo systemctl disable psychic-homily-production

# Stop old Docker containers
docker compose -f /opt/psychic-homily-production/backend/docker-compose.prod.yml down

# Verify the old service is stopped
sudo systemctl status psychic-homily-production
```

#### 6. Remove Old Production Nginx Config

```bash
# Remove Nginx site config for production API
sudo rm /etc/nginx/sites-enabled/psychic-homily-api
sudo rm /etc/nginx/sites-available/psychic-homily-api

# If no other sites use Nginx, stop it entirely
sudo systemctl stop nginx
sudo systemctl disable nginx
```

#### 7. Archive Old Production Directory (Optional)

```bash
sudo mv /opt/psychic-homily-production /opt/archive-psychic-homily-production
```

---

### Final Cleanup

After both stage and production are running on Coolify:

#### 1. Clean Up Orphaned Docker Resources

```bash
ssh mattcom

# Remove unused Docker volumes (CAREFUL - verify nothing important first)
docker volume ls  # List all volumes
docker volume prune  # Remove unused volumes

# Remove unused Docker images
docker image prune -a
```

#### 2. Update/Remove GitHub Workflows

The following files can be removed or updated since Coolify handles deployments:

- `.github/workflows/deploy-production.yml` — Remove or convert to Coolify webhook trigger
- `.github/workflows/deploy-stage.yml` — Remove or convert to Coolify webhook trigger
- `.github/workflows/daily-deployment.yml` — Keep if needed for frontend Netlify builds

#### 3. Remove Old Deployment Scripts (Optional)

These files are no longer needed but can be kept for reference:

- `backend/scripts/deploy-production.sh`
- `backend/scripts/deploy-stage.sh`
- `backend/systemd/psychic-homily-production.service`
- `backend/systemd/psychic-homily-stage.service`

---

_Document created: January 12, 2026_
_Last updated: January 24, 2026_
