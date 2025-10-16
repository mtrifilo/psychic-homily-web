#!/bin/bash

# Rollback PostgreSQL 18 to 17
# Emergency rollback script if upgrade fails
# Usage: ./rollback-postgres-upgrade.sh <environment> <backup-timestamp>

set -e

ENVIRONMENT=${1}
BACKUP_TIMESTAMP=${2}

if [[ -z "$ENVIRONMENT" || -z "$BACKUP_TIMESTAMP" ]]; then
    echo "Usage: $0 <environment> <backup-timestamp>"
    echo "Example: $0 stage 20250929_195433"
    exit 1
fi

BACKUP_DIR="/opt/psychic-homily-${ENVIRONMENT}/backend/backups"
COMPOSE_FILE="backend/docker-compose.${ENVIRONMENT}.yml"
ENV_FILE="backend/.env.${ENVIRONMENT}"
VOLUME_NAME="${ENVIRONMENT}_postgres_data"

echo "🔙 PostgreSQL Rollback Script"
echo "Environment: $ENVIRONMENT"
echo "Backup timestamp: $BACKUP_TIMESTAMP"
echo "================================"
echo ""

# Confirmation
echo "⚠️  WARNING: This will:"
echo "  1. Stop PostgreSQL 18"
echo "  2. Restore PostgreSQL 17 data"
echo "  3. Revert docker-compose.yml to PostgreSQL 17"
echo ""
read -p "Type 'ROLLBACK' to continue: " confirmation
if [[ "$confirmation" != "ROLLBACK" ]]; then
    echo "❌ Rollback cancelled"
    exit 1
fi

echo ""
echo "⏹️  Step 1: Stopping Services"
echo "============================="
if [[ "$ENVIRONMENT" == "production" ]]; then
    sudo systemctl stop psychic-homily-production || true
fi
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" stop db
echo "✅ Services stopped"

echo ""
echo "🗑️  Step 2: Removing PostgreSQL 18 Volume"
echo "=========================================="
docker volume rm ${VOLUME_NAME}
echo "✅ Volume removed"

echo ""
echo "📥 Step 3: Restoring PostgreSQL 17 Volume"
echo "=========================================="
docker volume create ${VOLUME_NAME}
docker run --rm \
    -v ${VOLUME_NAME}:/data \
    -v "$BACKUP_DIR":/backup \
    alpine sh -c "cd /data && tar -xzf /backup/pg17_volume_${BACKUP_TIMESTAMP}.tar.gz"
echo "✅ PostgreSQL 17 data restored"

echo ""
echo "📝 Step 4: Updating docker-compose.yml to PostgreSQL 17"
echo "========================================================"
sed -i.bak 's/postgres:18/postgres:17/g' "$COMPOSE_FILE"
echo "✅ docker-compose.yml updated (backup saved as .bak)"

echo ""
echo "🚀 Step 5: Starting PostgreSQL 17"
echo "=================================="
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d db
sleep 10
echo "✅ PostgreSQL 17 started"

echo ""
echo "🔍 Step 6: Verifying Rollback"
echo "=============================="
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T db psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -c "
SELECT 'PostgreSQL Version' as info, version() as value
UNION ALL
SELECT 'users', COUNT(*)::text FROM users
UNION ALL
SELECT 'artists', COUNT(*)::text FROM artists
UNION ALL
SELECT 'shows', COUNT(*)::text FROM shows
UNION ALL
SELECT 'venues', COUNT(*)::text FROM venues;
"

if [[ "$ENVIRONMENT" == "production" ]]; then
    echo ""
    echo "🚀 Step 7: Starting Application"
    echo "================================"
    sudo systemctl start psychic-homily-production
    sleep 5
    
    curl -f http://localhost:8080/health || {
        echo "❌ Health check failed!"
        exit 1
    }
    echo "✅ Application is healthy"
fi

echo ""
echo "✅ Rollback Complete!"
echo "====================="
echo ""
echo "⚠️  Remember to:"
echo "  1. Investigate why the upgrade failed"
echo "  2. Keep the PostgreSQL 18 backup for analysis"
echo "  3. Update docker-compose.yml in git if needed"
echo ""
