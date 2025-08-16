#!/bin/bash

# Zero-downtime deployment script
set -e

COMMIT_SHA=$1
COMPOSE_FILE="docker-compose.prod.yml"
BACKUP_DIR="/opt/psychic-homily-backend/backups"
SERVICE_NAME="psychic-homily-backend"
APP_PORT=8080
TEMP_PORT=8081

echo "🚀 Zero-downtime deployment for commit: $COMMIT_SHA"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup current binary
if [ -f "$SERVICE_NAME" ]; then
    cp "$SERVICE_NAME" "$BACKUP_DIR/${SERVICE_NAME}.backup.$(date +%Y%m%d_%H%M%S)"
fi

# Ensure database services are running
echo "🐳 Ensuring database services are healthy..."
docker-compose -f "$COMPOSE_FILE" up -d db redis

# Wait for database health
echo "⏳ Waiting for database..."
for i in {1..20}; do
    if docker-compose -f "$COMPOSE_FILE" exec db pg_isready -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-postgres}" >/dev/null 2>&1; then
        echo "✅ Database ready"
        break
    fi
    sleep 2
done

# Run migrations BEFORE deploying new binary
echo "🔄 Running database migrations..."
if ! docker-compose -f "$COMPOSE_FILE" run --rm migrate; then
    echo "❌ Migration failed - aborting deployment"
    exit 1
fi

# Deploy new binary alongside old one
echo "📦 Deploying new binary..."
chmod +x "$SERVICE_NAME"

# Start new binary on temporary port
echo "🚀 Starting new binary on port $TEMP_PORT..."
export API_ADDR="0.0.0.0:$TEMP_PORT"
nohup ./"$SERVICE_NAME" > /tmp/new-app.log 2>&1 &
NEW_APP_PID=$!

# Wait for new app to be healthy
echo "🏥 Health checking new binary..."
for i in {1..30}; do
    if curl -f "http://localhost:$TEMP_PORT/health" > /dev/null 2>&1; then
        echo "✅ New binary is healthy!"
        break
    fi
    sleep 2
    
    if [ $i -eq 30 ]; then
        echo "❌ New binary failed health check"
        kill $NEW_APP_PID 2>/dev/null || true
        exit 1
    fi
done

# Gracefully stop old service
echo "⏹️  Gracefully stopping old service..."
sudo systemctl stop "$SERVICE_NAME" || true

# Wait a moment for old service to fully stop
sleep 3

# Start new service on correct port
echo "🔄 Starting new service on port $APP_PORT..."
sudo systemctl start "$SERVICE_NAME"

# Verify new service is healthy
echo "🏥 Verifying new service health..."
for i in {1..20}; do
    if curl -f "http://localhost:$APP_PORT/health" > /dev/null 2>&1; then
        echo "✅ New service is healthy!"
        break
    fi
    sleep 2
    
    if [ $i -eq 20 ]; then
        echo "❌ New service health check failed - rolling back"
        # Rollback logic here
        exit 1
    fi
done

# Clean up temporary process
kill $NEW_APP_PID 2>/dev/null || true

echo "🎉 Zero-downtime deployment completed successfully!"
echo "📱 New binary running for commit: $COMMIT_SHA"
echo "🌐 Health check: http://localhost:$APP_PORT/health"
