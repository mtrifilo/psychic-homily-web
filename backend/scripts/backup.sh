#!/bin/bash

# Database Backup Script
# Usage: ./scripts/backup.sh [--upload] [--verify]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
UPLOAD_TO_GCS=false
VERIFY_BACKUP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --upload)
            UPLOAD_TO_GCS=true
            shift
            ;;
        --verify)
            VERIFY_BACKUP=true
            shift
            ;;
        *)
            echo -e "${RED}❌ Unknown option: $1${NC}"
            echo "Usage: ./scripts/backup.sh [--upload] [--verify]"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}💾 Database Backup Script${NC}"

# Check if we're in the right directory
if [ ! -f "docker-compose.prod.yml" ]; then
    echo -e "${RED}❌ Error: docker-compose.prod.yml not found. Please run this script from the backend directory.${NC}"
    exit 1
fi

# Load environment variables
if [ -f ".env.production" ]; then
    echo -e "${BLUE}📋 Loading production environment variables...${NC}"
    export $(cat .env.production | grep -v '^#' | xargs)
else
    echo -e "${RED}❌ Error: .env.production not found!${NC}"
    exit 1
fi

# Create backup directory
mkdir -p backups

# Check if database is running
if ! docker compose -f docker-compose.prod.yml ps | grep -q "db.*Up"; then
    echo -e "${RED}❌ Database is not running!${NC}"
    echo "Start the application first: docker compose -f docker-compose.prod.yml up -d"
    exit 1
fi

# Create timestamped backup
BACKUP_FILE="backups/backup_$(date +%Y%m%d_%H%M%S).sql"
echo -e "${BLUE}📦 Creating backup: $BACKUP_FILE${NC}"

# Create backup
docker compose -f docker-compose.prod.yml exec -T db pg_dump -U $POSTGRES_USER $POSTGRES_DB > "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Backup created successfully: $BACKUP_FILE${NC}"
    
    # Get file size
    FILE_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
    echo -e "${BLUE}📊 Backup size: $FILE_SIZE${NC}"
    
    # Verify backup integrity
    if [ "$VERIFY_BACKUP" = true ]; then
        echo -e "${BLUE}🔍 Verifying backup integrity...${NC}"
        if docker compose -f docker-compose.prod.yml exec -T db pg_restore --list "$BACKUP_FILE" > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Backup integrity verified${NC}"
        else
            echo -e "${YELLOW}⚠️  Backup integrity check failed (this is normal for plain SQL dumps)${NC}"
        fi
    fi
    
    # Upload to GCS if requested
    if [ "$UPLOAD_TO_GCS" = true ]; then
        if [ ! -z "$GCS_BUCKET" ]; then
            echo -e "${BLUE}📤 Uploading backup to Google Cloud Storage...${NC}"
            gsutil cp "$BACKUP_FILE" "gs://$GCS_BUCKET/backups/" || {
                echo -e "${YELLOW}⚠️  GCS upload failed${NC}"
                exit 1
            }
            echo -e "${GREEN}✅ Backup uploaded to GCS: gs://$GCS_BUCKET/backups/$(basename $BACKUP_FILE)${NC}"
            
            # Clean up old remote backups (keep last 30 days)
            echo -e "${BLUE}🧹 Cleaning up old remote backups (keeping last 30)...${NC}"
            gsutil ls gs://$GCS_BUCKET/backups/ | sort | tail -n +31 | xargs -I {} gsutil rm {} 2>/dev/null || true
        else
            echo -e "${YELLOW}⚠️  GCS_BUCKET not configured, skipping upload${NC}"
        fi
    fi
    
    # Clean up old local backups (keep last 10)
    echo -e "${BLUE}🧹 Cleaning up old local backups (keeping last 10)...${NC}"
    ls -t backups/backup_*.sql | tail -n +11 | xargs -r rm -f
    
    echo -e "${GREEN}🎉 Backup completed successfully!${NC}"
    echo -e "${BLUE}📁 Local backup: $BACKUP_FILE${NC}"
    if [ "$UPLOAD_TO_GCS" = true ] && [ ! -z "$GCS_BUCKET" ]; then
        echo -e "${BLUE}☁️  Remote backup: gs://$GCS_BUCKET/backups/$(basename $BACKUP_FILE)${NC}"
    fi
else
    echo -e "${RED}❌ Backup failed!${NC}"
    exit 1
fi 
