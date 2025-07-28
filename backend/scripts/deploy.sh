#!/bin/bash

# Production Deployment Script for DigitalOcean
# Handles both initial deployment and subsequent updates
# Usage: ./scripts/deploy.sh [--initial|--update]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
DEPLOYMENT_TYPE="update"
if [[ "$1" == "--initial" ]]; then
    DEPLOYMENT_TYPE="initial"
elif [[ "$1" == "--update" ]]; then
    DEPLOYMENT_TYPE="update"
fi

echo -e "${BLUE}🚀 Psychic Homily Backend Deployment${NC}"
echo -e "${BLUE}Type: ${DEPLOYMENT_TYPE}${NC}"

# Check if we're in the right directory
if [ ! -f "docker-compose.prod.yml" ]; then
    echo -e "${RED}❌ Error: docker-compose.prod.yml not found. Please run this script from the backend directory.${NC}"
    exit 1
fi

# Load environment variables (FIXED)
if [ -f ".env.production" ]; then
    echo -e "${BLUE}📋 Loading production environment variables...${NC}"
    set -a  # automatically export all variables
    source .env.production
    set +a  # disable automatic export
else
    echo -e "${RED}❌ Error: .env.production not found!${NC}"
    echo "Please create .env.production with your production settings"
    exit 1
fi

# Validate required environment variables (NEW)
REQUIRED_VARS=("POSTGRES_USER" "POSTGRES_PASSWORD" "POSTGRES_DB" "JWT_SECRET_KEY")
for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        echo -e "${RED}❌ Error: Required environment variable $var is not set!${NC}"
        exit 1
    fi
done

echo -e "${GREEN}✅ All required environment variables are set${NC}"

# Create backup directory
mkdir -p backups

# Pre-deployment backup (for updates)
if [[ "$DEPLOYMENT_TYPE" == "update" ]]; then
    echo -e "${BLUE}🔍 Checking for existing data...${NC}"
    
    # Check if database is running and has data
    if docker compose -f docker-compose.prod.yml ps | grep -q "db.*Up"; then
        echo -e "${YELLOW}⚠️  Database is running. Creating pre-deployment backup...${NC}"
        
        # Create timestamped backup
        BACKUP_FILE="backups/pre_deployment_backup_$(date +%Y%m%d_%H%M%S).sql"
        
        # Check if database has data (IMPROVED ERROR HANDLING)
        TABLE_COUNT=$(docker compose -f docker-compose.prod.yml exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ' || echo "0")
        
        if [ "$TABLE_COUNT" -gt 0 ] 2>/dev/null; then
            echo -e "${YELLOW}⚠️  Database contains $TABLE_COUNT tables. Creating backup...${NC}"
            
            # Create backup
            docker compose -f docker-compose.prod.yml exec -T db pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > "$BACKUP_FILE"
            
            if [ $? -eq 0 ] && [ -s "$BACKUP_FILE" ]; then
                echo -e "${GREEN}✅ Pre-deployment backup created: $BACKUP_FILE${NC}"
                
                # Upload to GCS if configured
                if [ ! -z "$GCS_BUCKET" ] && command -v gsutil &> /dev/null; then
                    echo -e "${BLUE}📤 Uploading backup to Google Cloud Storage...${NC}"
                    gsutil cp "$BACKUP_FILE" "gs://$GCS_BUCKET/backups/" || echo -e "${YELLOW}⚠️  GCS upload failed (continuing anyway)${NC}"
                fi
            else
                echo -e "${RED}❌ Backup failed! Aborting deployment.${NC}"
                exit 1
            fi
        else
            echo -e "${GREEN}✅ Database is empty, no backup needed${NC}"
        fi
    fi
fi

# Clean up old containers and images (NEW)
echo -e "${BLUE}🧹 Cleaning up old containers and images...${NC}"
docker compose -f docker-compose.prod.yml down --remove-orphans
docker system prune -f

# Build and start services
echo -e "${BLUE}🔨 Building and starting services...${NC}"
docker compose -f docker-compose.prod.yml up -d --build

# Wait for database to be ready (IMPROVED)
echo -e "${BLUE}⏳ Waiting for database to be ready...${NC}"
for i in {1..20}; do
    if docker compose -f docker-compose.prod.yml exec db pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Database is ready${NC}"
        break
    else
        echo -e "${YELLOW}⏳ Waiting for database... (attempt $i/20)${NC}"
        sleep 5
    fi
    
    if [ $i -eq 20 ]; then
        echo -e "${RED}❌ Database failed to start!${NC}"
        echo -e "${BLUE}📋 Database logs:${NC}"
        docker compose -f docker-compose.prod.yml logs db
        exit 1
    fi
done

# Run migrations
echo -e "${BLUE}🔄 Running database migrations...${NC}"

# Check current migration version (IMPROVED)
CURRENT_VERSION=$(docker compose -f docker-compose.prod.yml run --rm migrate -path /migrations -database "postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@db:5432/$POSTGRES_DB?sslmode=disable" version 2>/dev/null | tail -1 || echo "0")

echo -e "${BLUE}📊 Current migration version: $CURRENT_VERSION${NC}"

# Run migrations
if ! docker compose -f docker-compose.prod.yml run --rm migrate; then
    echo -e "${RED}❌ Migration failed!${NC}"
    exit 1
fi

# Check if migrations succeeded
NEW_VERSION=$(docker compose -f docker-compose.prod.yml run --rm migrate -path /migrations -database "postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@db:5432/$POSTGRES_DB?sslmode=disable" version 2>/dev/null | tail -1 || echo "0")

echo -e "${GREEN}✅ Migrations completed successfully (version $NEW_VERSION)${NC}"

# Wait for application to be healthy (IMPROVED)
echo -e "${BLUE}⏳ Waiting for application to be healthy...${NC}"
for i in {1..60}; do
    if curl -f http://127.0.0.1:8080/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Application is healthy!${NC}"
        break
    fi
    echo -e "${YELLOW}⏳ Waiting for application to start... ($i/60)${NC}"
    sleep 2
    
    # Show logs if taking too long
    if [ $i -eq 30 ]; then
        echo -e "${YELLOW}⚠️  Application taking longer than expected. Showing recent logs:${NC}"
        docker compose -f docker-compose.prod.yml logs --tail=20 app
    fi
done

# Check if application is running
if curl -f http://127.0.0.1:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}🎉 Deployment successful!${NC}"
    echo -e "${BLUE}📊 Application is running at: http://127.0.0.1:8080${NC}"
    echo -e "${BLUE}🔍 Health check: http://127.0.0.1:8080/health${NC}"
else
    echo -e "${RED}❌ Deployment failed! Application is not responding.${NC}"
    echo -e "${BLUE}📋 Checking logs...${NC}"
    docker compose -f docker-compose.prod.yml logs app
    exit 1
fi

# Create post-deployment backup
echo -e "${BLUE}💾 Creating post-deployment backup...${NC}"
POST_BACKUP_FILE="backups/post_deployment_backup_$(date +%Y%m%d_%H%M%S).sql"
docker compose -f docker-compose.prod.yml exec -T db pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > "$POST_BACKUP_FILE"

if [ $? -eq 0 ] && [ -s "$POST_BACKUP_FILE" ]; then
    echo -e "${GREEN}✅ Post-deployment backup created: $POST_BACKUP_FILE${NC}"
    
    # Upload to GCS if configured
    if [ ! -z "$GCS_BUCKET" ] && command -v gsutil &> /dev/null; then
        echo -e "${BLUE}📤 Uploading post-deployment backup to GCS...${NC}"
        gsutil cp "$POST_BACKUP_FILE" "gs://$GCS_BUCKET/backups/" || echo -e "${YELLOW}⚠️  GCS upload failed${NC}"
    fi
fi

# Final status check (NEW)
echo -e "${BLUE}🔍 Final deployment verification...${NC}"
docker compose -f docker-compose.prod.yml ps

echo -e "${GREEN}🎉 Deployment complete!${NC}"
echo -e "${BLUE}🌐 API available at: https://api.psychichomily.com${NC}"
echo -e "${BLUE}💾 Backups created:${NC}"
if [[ "$DEPLOYMENT_TYPE" == "update" && -n "$BACKUP_FILE" ]]; then
    echo -e "   - Pre-deployment: $BACKUP_FILE"
fi
echo -e "   - Post-deployment: $POST_BACKUP_FILE"
echo ""
echo -e "${YELLOW}📋 Next steps:${NC}"
echo -e "   1. Test the API endpoints"
echo -e "   2. Verify OAuth providers are working"
echo -e "   3. Check application logs: docker compose -f docker-compose.prod.yml logs -f"
echo -e "   4. Monitor application: docker compose -f docker-compose.prod.yml ps"

# Reload nginx to ensure proper configuration
echo -e "${BLUE}🔄 Reloading nginx configuration...${NC}"
if sudo nginx -t; then
    sudo systemctl reload nginx
    echo -e "${GREEN}✅ Nginx reloaded successfully${NC}"
else
    echo -e "${YELLOW}⚠️  Nginx configuration test failed, please check manually${NC}"
fi
