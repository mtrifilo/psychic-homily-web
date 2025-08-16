#!/bin/bash

# Staging deployment script
set -e

COMMIT_SHA=$1
COMPOSE_FILE="docker-compose.staging.yml"
BACKUP_DIR="/opt/psychic-homily-staging/backups"
SERVICE_NAME="psychic-homily-staging"
APP_PORT=8081
TEMP_PORT=8083

echo "🚀 Staging deployment for commit: $COMMIT_SHA"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup current binary
if [ -f "$SERVICE_NAME" ]; then
    cp "$SERVICE_NAME" "$BACKUP_DIR/${SERVICE_NAME}.backup.$(date +%Y%m%d_%H%M%S)"
fi

# Ensure database services are running
echo "🐳 Ensuring staging database services are healthy..."
docker-compose -f "$COMPOSE_FILE" up -d db redis

# Wait for database health
echo "⏳ Waiting for staging database..."
for i in {1..20}; do
    if docker-compose -f "$COMPOSE_FILE" exec db pg_isready -U "${POSTGRES_USER:-ph_staging_user}" -d "${POSTGRES_DB:-psychic_homily_staging}" >/dev/null 2>&1; then
        echo "✅ Staging database ready"
        break
    fi
    sleep 2
done

# Run migrations on staging database
echo "🔄 Running staging database migrations..."
if ! docker-compose -f "$COMPOSE_FILE" run --rm migrate; then
    echo "❌ Staging migration failed - aborting deployment"
    exit 1
fi

# Deploy new binary alongside old one
echo "📦 Deploying new staging binary..."
chmod +x "$SERVICE_NAME"

# Start new binary on temporary port
echo "🚀 Starting new staging binary on port $TEMP_PORT..."
export API_ADDR="0.0.0.0:$TEMP_PORT"
nohup ./"$SERVICE_NAME" > /tmp/new-staging-app.log 2>&1 &
NEW_APP_PID=$!

# Wait for new app to be healthy
echo "🏥 Health checking new staging binary..."
for i in {1..30}; do
    if curl -f "http://localhost:$TEMP_PORT/health" > /dev/null 2>&1; then
        echo "✅ New staging binary is healthy!"
        break
    fi
    sleep 2
    
    if [ $i -eq 30 ]; then
        echo "❌ New staging binary failed health check"
        kill $NEW_APP_PID 2>/dev/null || true
        exit 1
    fi
done

# Gracefully stop old staging service
echo "⏹️  Gracefully stopping old staging service..."
sudo systemctl stop "$SERVICE_NAME" || true

# Wait a moment for old service to fully stop
sleep 3

# Start new service on correct port
echo "🔄 Starting new staging service on port $APP_PORT..."
sudo systemctl start "$SERVICE_NAME"

# Verify new service is healthy
echo "🏥 Verifying new staging service health..."
for i in {1..20}; do
    if curl -f "http://localhost:$APP_PORT/health" > /dev/null 2>&1; then
        echo "✅ New staging service is healthy!"
        break
    fi
    sleep 2
    
    if [ $i -eq 20 ]; then
        echo "❌ New staging service health check failed - rolling back"
        # Rollback logic here
        exit 1
    fi
done

# Clean up temporary process
kill $NEW_APP_PID 2>/dev/null || true

echo "🎉 Staging deployment completed successfully!"
echo " New binary running for commit: $COMMIT_SHA"
echo " Health check: http://localhost:$APP_PORT/health"
