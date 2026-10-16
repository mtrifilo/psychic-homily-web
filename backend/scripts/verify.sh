#!/bin/bash

# System Verification Script
# Usage: ./scripts/verify.sh [--backups] [--system] [--all]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
CHECK_BACKUPS=false
CHECK_SYSTEM=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --backups)
            CHECK_BACKUPS=true
            shift
            ;;
        --system)
            CHECK_SYSTEM=true
            shift
            ;;
        --all)
            CHECK_BACKUPS=true
            CHECK_SYSTEM=true
            shift
            ;;
        *)
            echo -e "${RED}❌ Unknown option: $1${NC}"
            echo "Usage: ./scripts/verify.sh [--backups] [--system] [--all]"
            exit 1
            ;;
    esac
done

# If no options specified, check everything
if [ "$CHECK_BACKUPS" = false ] && [ "$CHECK_SYSTEM" = false ]; then
    CHECK_BACKUPS=true
    CHECK_SYSTEM=true
fi

echo -e "${BLUE}🔍 System Verification Report${NC}"
echo -e "${BLUE}Date: $(date)${NC}"
echo ""

# Load environment variables
if [ -f ".env.production" ]; then
    # Unquoted on purpose: word splitting makes each KEY=VALUE its own export
    # argument, so a value cannot contain whitespace.
    # shellcheck disable=SC2046
    export $(cat .env.production | grep -v '^#' | xargs)
fi

# Check system health
if [ "$CHECK_SYSTEM" = true ]; then
    echo -e "${BLUE}📊 System Health Check${NC}"
    echo "================================"
    
    # Check if Docker is running
    if docker info >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Docker is running${NC}"
    else
        echo -e "${RED}❌ Docker is not running${NC}"
    fi
    
    # Check if containers are running
    if [ -f "docker-compose.prod.yml" ]; then
        if docker compose -f docker-compose.prod.yml ps | grep -q "Up"; then
            echo -e "${GREEN}✅ Application containers are running${NC}"
            
            # Check API health
            if curl -f http://localhost:8080/health >/dev/null 2>&1; then
                echo -e "${GREEN}✅ API is responding${NC}"
            else
                echo -e "${RED}❌ API is not responding${NC}"
            fi
            
            # Check database health
            if docker compose -f docker-compose.prod.yml exec -T db pg_isready -U $POSTGRES_USER >/dev/null 2>&1; then
                echo -e "${GREEN}✅ Database is healthy${NC}"
            else
                echo -e "${RED}❌ Database is not healthy${NC}"
            fi
        else
            echo -e "${RED}❌ Application containers are not running${NC}"
        fi
    else
        echo -e "${YELLOW}⚠️  docker-compose.prod.yml not found${NC}"
    fi
    
    # Check disk space
    DISK_USAGE=$(df -h . | tail -1 | awk '{print $5}' | sed 's/%//')
    if [ "$DISK_USAGE" -lt 80 ]; then
        echo -e "${GREEN}✅ Disk space: ${DISK_USAGE}% used${NC}"
    elif [ "$DISK_USAGE" -lt 90 ]; then
        echo -e "${YELLOW}⚠️  Disk space: ${DISK_USAGE}% used${NC}"
    else
        echo -e "${RED}❌ Disk space: ${DISK_USAGE}% used${NC}"
    fi
    
    # Check memory usage
    MEMORY_USAGE=$(free | grep Mem | awk '{printf "%.0f", $3/$2 * 100.0}')
    if [ "$MEMORY_USAGE" -lt 80 ]; then
        echo -e "${GREEN}✅ Memory usage: ${MEMORY_USAGE}%${NC}"
    elif [ "$MEMORY_USAGE" -lt 90 ]; then
        echo -e "${YELLOW}⚠️  Memory usage: ${MEMORY_USAGE}%${NC}"
    else
        echo -e "${RED}❌ Memory usage: ${MEMORY_USAGE}%${NC}"
    fi
    
    echo ""
fi

# Check backups
if [ "$CHECK_BACKUPS" = true ]; then
    echo -e "${BLUE}💾 Backup Verification${NC}"
    echo "================================"
    
    # Check local backups
    LOCAL_BACKUP_COUNT=$(ls -1 backups/backup_*.sql 2>/dev/null | wc -l)
    if [ "$LOCAL_BACKUP_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✅ Local backups: $LOCAL_BACKUP_COUNT found${NC}"
        
        # Show latest local backup
        LATEST_LOCAL=$(ls -t backups/backup_*.sql 2>/dev/null | head -1)
        if [ -n "$LATEST_LOCAL" ]; then
            BACKUP_SIZE=$(du -h "$LATEST_LOCAL" | cut -f1)
            BACKUP_DATE=$(stat -c %y "$LATEST_LOCAL" | cut -d' ' -f1)
            echo -e "${BLUE}   Latest: $(basename $LATEST_LOCAL) (${BACKUP_SIZE}, ${BACKUP_DATE})${NC}"
        fi
    else
        echo -e "${YELLOW}⚠️  No local backups found${NC}"
    fi
    
    # Check GCS backups
    if [ ! -z "$GCS_BUCKET" ]; then
        if command -v gsutil >/dev/null 2>&1; then
            GCS_BACKUP_COUNT=$(gsutil ls gs://$GCS_BUCKET/backups/ 2>/dev/null | wc -l)
            if [ "$GCS_BACKUP_COUNT" -gt 0 ]; then
                echo -e "${GREEN}✅ GCS backups: $GCS_BACKUP_COUNT found${NC}"
                
                # Show latest GCS backup
                LATEST_GCS=$(gsutil ls gs://$GCS_BUCKET/backups/ | tail -1)
                if [ -n "$LATEST_GCS" ]; then
                    echo -e "${BLUE}   Latest: $(basename $LATEST_GCS)${NC}"
                    
                    # Test download of latest backup
                    echo -e "${BLUE}   Testing download...${NC}"
                    if gsutil cp "$LATEST_GCS" /tmp/test_backup.sql >/dev/null 2>&1; then
                        echo -e "${GREEN}   ✅ Latest backup is accessible${NC}"
                        rm -f /tmp/test_backup.sql
                    else
                        echo -e "${RED}   ❌ Latest backup download failed${NC}"
                    fi
                fi
            else
                echo -e "${YELLOW}⚠️  No GCS backups found${NC}"
            fi
        else
            echo -e "${YELLOW}⚠️  gsutil not installed${NC}"
        fi
    else
        echo -e "${YELLOW}⚠️  GCS_BUCKET not configured${NC}"
    fi
    
    echo ""
fi

echo -e "${GREEN}🎉 Verification complete!${NC}" 
