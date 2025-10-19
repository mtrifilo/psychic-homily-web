# PostgreSQL 18 Production Upgrade Guide

## Status

✅ **Stage Environment**: Successfully upgraded to PostgreSQL 18 on 2025-10-19
⏳ **Production Environment**: Ready to upgrade (currently PostgreSQL 17)

## What Was Done on Stage

1. Fixed docker-compose project naming issues
2. Updated upgrade script to use Docker Compose V2 syntax (`docker compose` instead of `docker-compose`)
3. Fixed volume naming to match existing volumes (`backend_ph_stage_data`)
4. Successfully upgraded from PostgreSQL 17 to 18
5. Verified all data integrity and indexes

## Production Upgrade Steps

### Prerequisites

1. **Schedule a maintenance window** - The upgrade will cause approximately 5-10 minutes of downtime
2. **Notify users** of the maintenance window
3. **SSH into your VPS** with production access
4. **Verify current backups** exist and are recent

### Step-by-Step Process

#### 1. SSH into Production Server

```bash
ssh <your-production-server>
cd /opt/psychic-homily-production  # Or wherever your production is deployed
```

#### 2. Verify Current State

```bash
# Check what's running
docker ps | grep production

# Check current PostgreSQL version
docker exec -it <your-pg-container> psql -U <your-user> -d <your-db> -c "SELECT version();"

# Check current data counts
docker exec -it <your-pg-container> psql -U <your-user> -d <your-db> -c "
SELECT 'users' as table_name, COUNT(*) as count FROM users
UNION ALL SELECT 'artists', COUNT(*) FROM artists
UNION ALL SELECT 'shows', COUNT(*) FROM shows
UNION ALL SELECT 'venues', COUNT(*) FROM venues;
"
```

#### 3. Pull Latest Code (with fixed scripts)

```bash
git fetch
git pull origin main
```

#### 4. Run the Upgrade Script

```bash
sudo ./backend/scripts/upgrade-postgres-to-18.sh production
```

The script will:

- Display pre-upgrade statistics
- Create SQL backup (compressed)
- Create filesystem backup of the volume
- Stop the database and application
- Remove old volume
- Start PostgreSQL 18 with fresh volume
- Restore all data
- Run migrations
- Verify data integrity
- Restart the application
- Perform health check

**Expected Duration**: 5-10 minutes depending on database size

#### 5. Verify Production After Upgrade

```bash
# Check application health
curl -f http://localhost:8080/health

# Check external endpoint
curl -f https://api.psychichomily.com/health

# Verify PostgreSQL version
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production exec -T db psql -U <user> -d <db> -c "SELECT version();"

# Verify data counts match pre-upgrade
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production exec -T db psql -U <user> -d <db> -c "
SELECT 'users' as table_name, COUNT(*) as count FROM users
UNION ALL SELECT 'artists', COUNT(*) FROM artists
UNION ALL SELECT 'shows', COUNT(*) FROM shows
UNION ALL SELECT 'venues', COUNT(*) FROM venues;
"

# Test API endpoints
curl https://api.psychichomily.com/artists
curl https://api.psychichomily.com/shows
```

#### 6. Update Production docker-compose File

Once verified working on PostgreSQL 18, update the config:

```bash
# Edit locally
# Change backend/docker-compose.prod.yml: image: postgres:17 → postgres:18

git add backend/docker-compose.prod.yml
git commit -m "Upgrade production to PostgreSQL 18"
git push
```

### Rollback Plan (If Needed)

If something goes wrong, you can rollback:

```bash
# Stop everything
sudo systemctl stop psychic-homily-production
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production down

# Remove the new volume
docker volume rm backend_postgres_data  # Or whatever the production volume name is

# Restore from filesystem backup
# The backup is at: /opt/psychic-homily-production/backend/backups/pg17_volume_<timestamp>.tar.gz

# Recreate volume and restore
docker volume create backend_postgres_data
docker run --rm \
    -v backend_postgres_data:/data \
    -v /opt/psychic-homily-production/backend/backups:/backup \
    alpine tar -xzf /backup/pg17_volume_<timestamp>.tar.gz -C /data

# Change docker-compose back to postgres:17
# Then restart
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production up -d db
sudo systemctl start psychic-homily-production
```

## Production Deployment Script Considerations

**Note**: Your production deployment script may need similar updates that were made to stage:

1. Check if it uses `docker-compose` (needs to be `docker compose`)
2. Check if it uses explicit project naming with `-p` flag
3. Verify volume names match what docker-compose actually creates

You may need to inspect `/opt/psychic-homily-production/backend/scripts/deploy-production.sh` and apply similar fixes.

## Post-Upgrade

After successful upgrade:

1. Monitor application logs for any issues
2. Check performance metrics
3. Keep backups for at least 30 days
4. Update documentation with new PostgreSQL version

## Backup Locations

The upgrade script creates backups at:

- **SQL Backup**: `/opt/psychic-homily-production/backend/backups/pg17_to_pg18_<timestamp>.sql.gz`
- **Volume Backup**: `/opt/psychic-homily-production/backend/backups/pg17_volume_<timestamp>.tar.gz`

Keep these for at least 30 days after successful upgrade.

## Benefits of PostgreSQL 18

- Performance improvements for parallel queries
- Better indexing performance
- Improved JSON handling
- Enhanced full-text search capabilities
- Bug fixes and security patches
