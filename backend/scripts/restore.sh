#!/bin/bash

# Database Restore Script
# Usage: ./scripts/restore.sh <backup_file> [--from-gcs]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
FROM_GCS=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --from-gcs)
            FROM_GCS=true
            shift
            ;;
        *)
            BACKUP_FILE="$1"
            shift
            ;;
    esac
done

echo -e "${BLUE}🔄 Database Restore Script${NC}"

# Check arguments
if [ -z "$BACKUP_FILE" ]; then
    echo -e "${RED}❌ Error: Please specify a backup file${NC}"
    echo "Usage: ./scripts/restore.sh <backup_file> [--from-gcs]"
    echo ""
    echo "Available local backups:"
    ls -la backups/backup_*.sql 2>/dev/null || echo "No local backups found"
    echo ""
    echo "Available GCS backups:"
    if [ ! -z "$GCS_BUCKET" ]; then
        gsutil ls gs://$GCS_BUCKET/backups/ 2>/dev/null || echo "No GCS backups found"
    else
        echo "GCS_BUCKET not configured"
    fi
    exit 1
fi

# Check if we're in the right directory
if [ ! -f "docker-compose.prod.yml" ]; then
    echo -e "${RED}❌ Error: docker-compose.prod.yml not found. Please run this script from the backend directory.${NC}"
    exit 1
fi

# Load environment variables
if [ -f ".env.production" ]; then
    echo -e "${BLUE}📋 Loading production environment variables...${NC}"
    # Unquoted on purpose: word splitting turns each KEY=VALUE line into its own
    # export argument, and quoting would export one malformed variable.
    # shellcheck disable=SC2046
    export $(cat .env.production | grep -v '^#' | xargs)
else
    echo -e "${RED}❌ Error: .env.production not found!${NC}"
    exit 1
fi

# Handle GCS backup download
if [ "$FROM_GCS" = true ]; then
    if [ -z "$GCS_BUCKET" ]; then
        echo -e "${RED}❌ Error: GCS_BUCKET not configured!${NC}"
        exit 1
    fi
    
    echo -e "${BLUE}📥 Downloading backup from GCS...${NC}"
    LOCAL_BACKUP="backups/$(basename $BACKUP_FILE)"
    gsutil cp "gs://$GCS_BUCKET/backups/$BACKUP_FILE" "$LOCAL_BACKUP" || {
        echo -e "${RED}❌ Failed to download backup from GCS!${NC}"
        exit 1
    }
    BACKUP_FILE="$LOCAL_BACKUP"
    echo -e "${GREEN}✅ Backup downloaded: $BACKUP_FILE${NC}"
fi

# Check if backup file exists
if [ ! -f "$BACKUP_FILE" ]; then
    echo -e "${RED}❌ Error: Backup file '$BACKUP_FILE' not found!${NC}"
    exit 1
fi

# Check if database is running
if ! docker compose -f docker-compose.prod.yml ps | grep -q "db.*Up"; then
    echo -e "${RED}❌ Database is not running!${NC}"
    echo "Start the application first: docker compose -f docker-compose.prod.yml up -d"
    exit 1
fi

# Confirm restore
echo -e "${YELLOW}⚠️  WARNING: This will overwrite the current database!${NC}"
echo -e "${BLUE}📁 Backup file: $BACKUP_FILE${NC}"
echo -e "${BLUE}📊 File size: $(du -h "$BACKUP_FILE" | cut -f1)${NC}"
echo ""
read -p "Are you sure you want to continue? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo -e "${YELLOW}❌ Restore cancelled${NC}"
    exit 0
fi

# Create pre-restore backup
echo -e "${BLUE}💾 Creating pre-restore backup...${NC}"
PRE_RESTORE_BACKUP="backups/pre_restore_backup_$(date +%Y%m%d_%H%M%S).sql"
docker compose -f docker-compose.prod.yml exec -T db pg_dump -U $POSTGRES_USER $POSTGRES_DB > "$PRE_RESTORE_BACKUP"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Pre-restore backup created: $PRE_RESTORE_BACKUP${NC}"
else
    echo -e "${YELLOW}⚠️  Pre-restore backup failed (continuing anyway)${NC}"
fi

# Stop application to prevent data corruption
echo -e "${BLUE}🛑 Stopping application...${NC}"
docker compose -f docker-compose.prod.yml stop app

# Restore database
echo -e "${BLUE}🔄 Restoring database from: $BACKUP_FILE${NC}"
docker compose -f docker-compose.prod.yml exec -T db psql -U $POSTGRES_USER -d $POSTGRES_DB -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
docker compose -f docker-compose.prod.yml exec -T db psql -U $POSTGRES_USER -d $POSTGRES_DB < "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Database restored successfully!${NC}"
else
    echo -e "${RED}❌ Database restore failed!${NC}"
    echo -e "${YELLOW}⚠️  You may need to manually restore from: $PRE_RESTORE_BACKUP${NC}"
    exit 1
fi

# Start application
echo -e "${BLUE}🚀 Starting application...${NC}"
docker compose -f docker-compose.prod.yml start app

# Wait for application to be healthy
echo -e "${BLUE}⏳ Waiting for application to be healthy...${NC}"
for i in {1..30}; do
    if curl -f http://localhost:8080/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Application is healthy!${NC}"
        break
    fi
    echo -e "${YELLOW}⏳ Waiting for application to start... ($i/30)${NC}"
    sleep 2
done

if curl -f http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}🎉 Restore completed successfully!${NC}"
    echo -e "${BLUE}📊 Application is running at: http://localhost:8080${NC}"
else
    echo -e "${YELLOW}⚠️  Application may not be fully started${NC}"
    echo -e "${BLUE}📋 Check logs: docker compose -f docker-compose.prod.yml logs app${NC}"
fi 
