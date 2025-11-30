# PostgreSQL 18 Volume Migration Runbook

## Overview

PostgreSQL 18 changed how it stores data. The Docker image now expects volumes mounted at `/var/lib/postgresql` (not `/var/lib/postgresql/data`). Data is stored in version-specific subdirectories like `/var/lib/postgresql/18/docker`.

**Impact:** Existing databases with the old volume mount will fail to start with:

```
Error: in 18+, these Docker images are configured to store database data in a
       format which is compatible with "pg_ctlcluster"
```

## Pre-Migration Checklist

- [ ] Confirm you have SSH access to the server
- [ ] Verify backup scripts are working
- [ ] Schedule maintenance window (expect 10-15 minutes downtime)
- [ ] Notify stakeholders of planned downtime

---

## Stage Environment Migration

### 1. SSH into Stage Server

```bash
ssh user@stage-server
cd /opt/psychic-homily-stage
```

### 2. Create Database Backup

```bash
# Create backup directory if needed
mkdir -p backend/backups

# Dump the database
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage \
  exec -T db pg_dump -U ${POSTGRES_USER:-ph_stage_user} ${POSTGRES_DB:-psychic_homily_stage} \
  > backend/backups/stage_backup_$(date +%Y%m%d_%H%M%S).sql

# Verify backup was created
ls -la backend/backups/
```

### 3. Stop Services

```bash
# Stop the Go application
sudo systemctl stop psychic-homily-stage

# Stop Docker containers (preserving volumes for now)
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage down
```

### 4. Remove Old Volume

```bash
# List volumes to confirm name
docker volume ls | grep stage

# Remove the old postgres data volume
docker volume rm backend_ph_stage_data
```

### 5. Pull Latest Code (with updated docker-compose.stage.yml)

```bash
git pull origin main
```

### 6. Start Fresh Database

```bash
# Start database with new volume mount
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage up -d db

# Wait for it to initialize
sleep 10

# Verify it's running
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage logs db
```

### 7. Restore Database

```bash
# Run migrations first (creates schema)
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage run --rm migrate

# Restore data from backup
cat backend/backups/stage_backup_YYYYMMDD_HHMMSS.sql | \
  docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage \
  exec -T db psql -U ${POSTGRES_USER:-ph_stage_user} ${POSTGRES_DB:-psychic_homily_stage}
```

### 8. Start Application

```bash
# Start Redis
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage up -d redis

# Start the Go application
sudo systemctl start psychic-homily-stage

# Verify health
curl http://localhost:8081/health
```

### 9. Verify Stage Migration

```bash
# Check database connectivity
docker compose -p backend -f backend/docker-compose.stage.yml --env-file backend/.env.stage \
  exec -T db psql -U ${POSTGRES_USER:-ph_stage_user} -d ${POSTGRES_DB:-psychic_homily_stage} \
  -c "SELECT COUNT(*) FROM shows;"

# Check application logs
sudo journalctl -u psychic-homily-stage -n 50 --no-pager
```

---

## Production Environment Migration

> ⚠️ **Only proceed after successful Stage migration**

### 1. SSH into Production Server

```bash
ssh user@production-server
cd /opt/psychic-homily-production
```

### 2. Create Database Backup

```bash
# Create backup directory if needed
mkdir -p backend/backups

# Dump the database
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production \
  exec -T db pg_dump -U ${POSTGRES_USER:-postgres} ${POSTGRES_DB:-postgres} \
  > backend/backups/prod_backup_$(date +%Y%m%d_%H%M%S).sql

# Verify backup size (should be > 0)
ls -lh backend/backups/prod_backup_*.sql

# Optional: Copy backup to a safe location
scp backend/backups/prod_backup_*.sql user@backup-server:/path/to/backups/
```

### 3. Stop Services

```bash
# Stop the Go application
sudo systemctl stop psychic-homily-production

# Stop Docker containers
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production down
```

### 4. Remove Old Volume

```bash
# List volumes to confirm name
docker volume ls | grep postgres

# Remove the old postgres data volume
# WARNING: This destroys the database - ensure backup is valid first!
docker volume rm psychic-homily-backend_postgres_data
# or it might be named:
docker volume rm backend_postgres_data
```

### 5. Pull Latest Code

```bash
git pull origin main
```

### 6. Start Fresh Database

```bash
# Start database with new volume mount
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production up -d db

# Wait for initialization
sleep 15

# Verify it's healthy
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production logs db | tail -20
```

### 7. Restore Database

```bash
# Run migrations (creates schema)
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production run --rm migrate

# Restore data from backup
cat backend/backups/prod_backup_YYYYMMDD_HHMMSS.sql | \
  docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production \
  exec -T db psql -U ${POSTGRES_USER:-postgres} ${POSTGRES_DB:-postgres}
```

### 8. Start Application

```bash
# Start Redis
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production up -d redis

# Start the Go application
sudo systemctl start psychic-homily-production

# Verify health
curl http://localhost:8080/health
```

### 9. Verify Production Migration

```bash
# Check database has data
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production \
  exec -T db psql -U ${POSTGRES_USER:-postgres} -d ${POSTGRES_DB:-postgres} \
  -c "SELECT COUNT(*) FROM shows;"

# Check application logs
sudo journalctl -u psychic-homily-production -n 50 --no-pager

# Test a few API endpoints
curl http://localhost:8080/api/shows | head
```

---

## Rollback Procedure

If migration fails and you need to restore the old setup:

### Option A: Restore from Backup (Recommended)

The backup SQL file can be restored to any PostgreSQL version:

```bash
# Start a fresh database (even with old docker-compose if needed)
# Then restore:
cat backend/backups/prod_backup_YYYYMMDD_HHMMSS.sql | docker exec -i container_name psql -U user -d database
```

### Option B: Revert docker-compose Changes

If you haven't deleted the old volume:

```bash
# Revert the docker-compose file
git checkout HEAD~1 -- backend/docker-compose.prod.yml

# Restart with old config
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production up -d
```

---

## Post-Migration Cleanup

After confirming everything works (wait at least 24 hours in production):

```bash
# Remove old backup files older than 7 days
find backend/backups -name "*.sql" -mtime +7 -delete

# Check disk space
df -h
```

---

## Future Deployments

After this migration, the normal deployment scripts (`deploy-stage.sh`, `deploy-production.sh`) will work as expected. The volume mount change is transparent to the deployment process.

The key changes made:

- `docker-compose.stage.yml`: Volume mount changed from `/var/lib/postgresql/data` to `/var/lib/postgresql`
- `docker-compose.prod.yml`: Volume mount changed from `/var/lib/postgresql/data` to `/var/lib/postgresql`

PostgreSQL 18 will now store data in `/var/lib/postgresql/18/docker` inside the container.
