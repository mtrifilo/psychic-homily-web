#!/bin/bash

# Deploy script for Psychic Homily Backend (Binary + Docker Services)
# Usage: ./deploy.sh <commit-sha>

set -e

if [ $# -eq 0 ]; then
    echo "Usage: $0 <commit-sha>"
    echo "Example: $0 abc1234"
    exit 1
fi

COMMIT_SHA=$1
COMPOSE_FILE="docker-compose.prod.yml"
BACKUP_DIR="/opt/psychic-homily-backend/backups"
SERVICE_NAME="psychic-homily-backend"

echo "🚀 Deploying Psychic Homily Backend (Binary + Docker Services)"
echo "📱 Commit SHA: $COMMIT_SHA"

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Backup current binary if it exists
if [ -f "$SERVICE_NAME" ]; then
    cp "$SERVICE_NAME" "$BACKUP_DIR/${SERVICE_NAME}.backup.$(date +%Y%m%d_%H%M%S)"
    echo "💾 Backed up current binary"
fi

# Stop the Go application service
echo "⏹️  Stopping Go application service..."
sudo systemctl stop "$SERVICE_NAME" || true

# Ensure database services are running
echo "🐳 Starting/updating Docker services..."
docker-compose -f "$COMPOSE_FILE" up -d db redis

# Wait for database to be healthy
echo "⏳ Waiting for database to be ready..."
for i in {1..20}; do
    if docker-compose -f "$COMPOSE_FILE" exec db pg_isready -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-postgres}" >/dev/null 2>&1; then
        echo "✅ Database is ready"
        break
    else
        echo "⏳ Waiting for database... (attempt $i/20)"
        sleep 5
    fi
    
    if [ $i -eq 20 ]; then
        echo "❌ Database failed to start!"
        docker-compose -f "$COMPOSE_FILE" logs db
        exit 1
    fi
done

# Run migrations
echo "🔄 Running database migrations..."
if ! docker-compose -f "$COMPOSE_FILE" run --rm migrate; then
    echo "❌ Migration failed!"
    exit 1
fi

# Make binary executable
echo "🔧 Making binary executable..."
chmod +x "$SERVICE_NAME"

# Start the Go application service
echo "▶️  Starting Go application service..."
sudo systemctl start "$SERVICE_NAME"

# Wait for application to be healthy
echo "🏥 Waiting for application health check..."
for i in {1..30}; do
    if curl -f http://localhost:8080/health > /dev/null 2>&1; then
        echo "✅ Application is healthy!"
        break
    else
        echo "⏳ Waiting for application to start... ($i/30)"
        sleep 2
        
        # Show logs if taking too long
        if [ $i -eq 15 ]; then
            echo "⚠️  Application taking longer than expected. Checking logs:"
            sudo journalctl -u "$SERVICE_NAME" --no-pager -n 20
        fi
    fi
    
    if [ $i -eq 30 ]; then
        echo "❌ Application health check failed!"
        echo "📋 Recent logs:"
        sudo journalctl -u "$SERVICE_NAME" --no-pager -n 50
        exit 1
    fi
done

# Check service status
echo "📊 Service status:"
sudo systemctl status "$SERVICE_NAME" --no-pager

# Check Docker services status
echo "🐳 Docker services status:"
docker-compose -f "$COMPOSE_FILE" ps

# Clean up old Docker images
echo "🧹 Cleaning up old Docker images..."
docker image prune -f

echo "🎉 Deployment completed successfully!"
echo "📱 New binary deployed for commit: $COMMIT_SHA"
echo "�� Health check: http://localhost:8080/health"
echo "📊 Service status: sudo systemctl status $SERVICE_NAME"
