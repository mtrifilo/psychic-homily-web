#!/bin/bash

# Pre-flight Check for PostgreSQL Upgrade
# Verifies system is ready for upgrade
# Usage: ./preflight-check-postgres-upgrade.sh <environment>

set -e

ENVIRONMENT=${1:-stage}
COMPOSE_FILE="backend/docker-compose.${ENVIRONMENT}.yml"
ENV_FILE="backend/.env.${ENVIRONMENT}"

echo "🔍 Pre-flight Check for PostgreSQL Upgrade"
echo "Environment: $ENVIRONMENT"
echo "=========================================="
echo ""

PASSED=0
FAILED=0

# Check 1: Database is accessible
echo "✓ Check 1: Database Connectivity"
if docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T db psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -c "SELECT 1;" > /dev/null 2>&1; then
    echo "  ✅ Database is accessible"
    ((PASSED++))
else
    echo "  ❌ Cannot connect to database"
    ((FAILED++))
fi

# Check 2: Verify current version
echo ""
echo "✓ Check 2: Current PostgreSQL Version"
CURRENT_VERSION=$(docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T db psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -t -c "SHOW server_version;" | tr -d ' ')
echo "  Current version: $CURRENT_VERSION"
if [[ "$CURRENT_VERSION" == 17* ]]; then
    echo "  ✅ PostgreSQL 17 confirmed"
    ((PASSED++))
else
    echo "  ⚠️  Unexpected version: $CURRENT_VERSION"
    ((FAILED++))
fi

# Check 3: Disk space
echo ""
echo "✓ Check 3: Available Disk Space"
AVAILABLE_SPACE=$(df -h /opt | tail -1 | awk '{print $4}')
USED_PERCENT=$(df -h /opt | tail -1 | awk '{print $5}' | tr -d '%')
echo "  Available: $AVAILABLE_SPACE"
echo "  Used: $USED_PERCENT%"
if [[ $USED_PERCENT -lt 80 ]]; then
    echo "  ✅ Sufficient disk space"
    ((PASSED++))
else
    echo "  ❌ Disk usage > 80% - free up space before upgrade"
    ((FAILED++))
fi

# Check 4: Backup directory exists and is writable
echo ""
echo "✓ Check 4: Backup Directory"
BACKUP_DIR="/opt/psychic-homily-${ENVIRONMENT}/backend/backups"
if [[ -d "$BACKUP_DIR" && -w "$BACKUP_DIR" ]]; then
    echo "  ✅ Backup directory exists and is writable"
    echo "  Path: $BACKUP_DIR"
    ((PASSED++))
else
    echo "  ❌ Backup directory not accessible: $BACKUP_DIR"
    ((FAILED++))
fi

# Check 5: Data counts
echo ""
echo "✓ Check 5: Current Data Counts"
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T db psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -c "
SELECT table_name, count 
FROM (
    SELECT 'users' as table_name, COUNT(*) as count FROM users
    UNION ALL SELECT 'artists', COUNT(*) FROM artists
    UNION ALL SELECT 'shows', COUNT(*) FROM shows
    UNION ALL SELECT 'venues', COUNT(*) FROM venues
) t;
"
echo "  ✅ Data counts retrieved"
((PASSED++))

# Check 6: No active connections (for production)
if [[ "$ENVIRONMENT" == "production" ]]; then
    echo ""
    echo "✓ Check 6: Active Database Connections"
    ACTIVE_CONNS=$(docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T db \
        psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -t -c \
        "SELECT count(*) FROM pg_stat_activity WHERE datname='${POSTGRES_DB}' AND pid != pg_backend_pid();" | tr -d ' ')
    echo "  Active connections: $ACTIVE_CONNS"
    if [[ $ACTIVE_CONNS -lt 5 ]]; then
        echo "  ✅ Low connection count"
        ((PASSED++))
    else
        echo "  ⚠️  High connection count - consider scheduling maintenance window"
        ((FAILED++))
    fi
fi

# Check 7: Docker version
echo ""
echo "✓ Check 7: Docker Version"
DOCKER_VERSION=$(docker --version | awk '{print $3}' | tr -d ',')
echo "  Docker version: $DOCKER_VERSION"
echo "  ✅ Docker is available"
((PASSED++))

# Summary
echo ""
echo "========================================"
echo "Pre-flight Check Summary"
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo ""

if [[ $FAILED -eq 0 ]]; then
    echo "✅ All checks passed! System is ready for upgrade."
    echo ""
    echo "Next steps:"
    echo "  1. Schedule maintenance window (if production)"
    echo "  2. Notify users of downtime"
    echo "  3. Run: ./upgrade-postgres-to-18.sh $ENVIRONMENT"
    exit 0
else
    echo "❌ Some checks failed. Fix issues before proceeding."
    exit 1
fi
