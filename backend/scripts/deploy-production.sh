#!/bin/bash

# Zero-downtime deployment script for production environment
set -e

COMMIT_SHA=$1
COMPOSE_FILE="backend/docker-compose.prod.yml"
BACKUP_DIR="/opt/psychic-homily-production/backend/backups"
SERVICE_NAME="psychic-homily-production"
APP_PORT=8080
TEMP_PORT=8081
MAX_RETRIES=30
HEALTH_ENDPOINT="/health"

echo "🚀 Zero-downtime production deployment for commit: $COMMIT_SHA"

# Validate required parameters
if [ -z "$COMMIT_SHA" ]; then
    echo "❌ Error: COMMIT_SHA parameter is required"
    echo "Usage: $0 <commit_sha>"
    exit 1
fi

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup current binary if it exists
if [ -f "$SERVICE_NAME" ]; then
    BACKUP_FILE="$BACKUP_DIR/${SERVICE_NAME}.backup.$(date +%Y%m%d_%H%M%S)"
    cp "$SERVICE_NAME" "$BACKUP_FILE"
    echo "📦 Current binary backed up to: $BACKUP_FILE"
fi

# Clean up any orphaned containers (PRESERVE VOLUMES FOR DATA SAFETY)
echo "🧹 Cleaning up orphaned containers (preserving data volumes)..."
cd backend && docker compose -f "docker-compose.prod.yml" down --remove-orphans 2>/dev/null && cd .. || true

# Remove any containers with old naming conventions (NOT VOLUMES)
docker container rm -f ph_production_redis ph_production_migrate 2>/dev/null || true

# Only remove Redis cache volume (safe to delete), but preserve database volume
docker volume rm -f psychic-homily-backend_ph_production_redis 2>/dev/null || true

# NOTE: We deliberately DO NOT remove ph_production_data to preserve database data

# Ensure database services are running
echo "🐳 Ensuring production database services are healthy..."
cd backend && docker compose -f "docker-compose.prod.yml" up -d db redis && cd ..

# Wait for database health with better error handling
echo "⏳ Waiting for production database..."
DB_READY=false
for i in {1..20}; do
    if cd backend && docker compose -f "docker-compose.prod.yml" exec -T db pg_isready -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-postgres}" >/dev/null 2>&1 && cd ..; then
        echo "✅ Production database ready"
        DB_READY=true
        break
    fi
    echo "🔍 Database attempt $i/20..."
    sleep 2
done

if [ "$DB_READY" = false ]; then
    echo "❌ Database failed to become ready - aborting deployment"
    docker compose -f "$COMPOSE_FILE" logs db
    exit 1
fi

# Run migrations BEFORE deploying new binary
echo "🔄 Running production database migrations..."
if ! (cd backend && docker compose -f "docker-compose.prod.yml" run --rm migrate); then
    echo "❌ Production migration failed - aborting deployment"
    cd backend && docker compose -f "docker-compose.prod.yml" logs migrate && cd ..
    exit 1
fi

# Deploy new binary alongside old one
echo "📦 Deploying new production binary..."

# Check if binary exists in backend subdirectory and move it
if [ -f "backend/$SERVICE_NAME" ]; then
    echo "📦 Moving binary from backend/ to root directory"
    mv "backend/$SERVICE_NAME" "$SERVICE_NAME"
elif [ ! -f "$SERVICE_NAME" ]; then
    echo "❌ Binary $SERVICE_NAME not found in $(pwd) or backend/"
    echo "📁 Files in current directory:"
    ls -la
    echo "📁 Files in backend directory:"
    ls -la backend/ 2>/dev/null || echo "backend/ directory not found"
    exit 1
fi

chmod +x "$SERVICE_NAME"

# Check if temporary port is already in use
if netstat -tlnp | grep ":$TEMP_PORT " > /dev/null; then
    echo "⚠️  Port $TEMP_PORT is already in use, killing process..."
    TEMP_PID=$(netstat -tlnp | grep ":$TEMP_PORT " | awk '{print $7}' | cut -d'/' -f1)
    kill -9 $TEMP_PID 2>/dev/null || true
    sleep 2
fi

# Start new binary on temporary port
echo "🚀 Starting new production binary on port $TEMP_PORT..."

# Set ENVIRONMENT first so the Go app loads the right .env file
export ENVIRONMENT=production

# Override API_ADDR for temporary port
export API_ADDR="0.0.0.0:$TEMP_PORT"

echo "🔍 Environment config:"
echo "  ENVIRONMENT=$ENVIRONMENT"
echo "  API_ADDR=$API_ADDR"
echo "  Will load: .env.$ENVIRONMENT"

# Start the new binary in background
nohup ./"$SERVICE_NAME" > /tmp/new-production-app.log 2>&1 &
NEW_APP_PID=$!

echo "🔍 Started new binary with PID: $NEW_APP_PID"

# Function to check if process is still running
check_process_running() {
    if ! ps -p $NEW_APP_PID > /dev/null 2>&1; then
        echo "❌ New binary process died unexpectedly"
        echo "📋 Application logs:"
        cat /tmp/new-production-app.log 2>/dev/null || echo "No log file found"
        return 1
    fi
    return 0
}

# Wait for new app to be healthy
echo "🏥 Health checking new production binary..."
HEALTH_CHECK_PASSED=false

for i in $(seq 1 $MAX_RETRIES); do
    # First check if process is still running
    if ! check_process_running; then
        exit 1
    fi
    
    # Then check health endpoint
    if curl -f --connect-timeout 5 --max-time 10 "http://localhost:$TEMP_PORT$HEALTH_ENDPOINT" > /dev/null 2>&1; then
        echo "✅ New production binary is healthy!"
        HEALTH_CHECK_PASSED=true
        break
    fi
    
    # Debug: Show what's happening at intervals
    if [ $((i % 5)) -eq 0 ]; then
        echo "🔍 Debug attempt $i/$MAX_RETRIES: Checking status..."
        echo "  Process running: $(ps -p $NEW_APP_PID > /dev/null 2>&1 && echo "Yes" || echo "No")"
        echo "  Port listening: $(netstat -tlnp | grep ":$TEMP_PORT " > /dev/null && echo "Yes" || echo "No")"
        echo "  Recent logs:"
        tail -5 /tmp/new-production-app.log 2>/dev/null || echo "    No log file found"
    fi
    
    sleep 2
done

if [ "$HEALTH_CHECK_PASSED" = false ]; then
    echo "❌ New production binary failed health check after $MAX_RETRIES attempts"
    echo "🔍 Final debug info:"
    echo "  Process status: $(ps -p $NEW_APP_PID > /dev/null 2>&1 && echo "Running" || echo "Dead")"
    echo "  Port status: $(netstat -tlnp | grep ":$TEMP_PORT " || echo "Not listening")"
    echo "📋 Full application logs:"
    cat /tmp/new-production-app.log 2>/dev/null || echo "No log file found"
    
    # Cleanup
    kill -9 $NEW_APP_PID 2>/dev/null || true
    exit 1
fi

# Gracefully stop old production service
echo "⏹️  Gracefully stopping old production service..."
sudo systemctl stop "$SERVICE_NAME" || true

# Wait for old service to fully stop
sleep 3

# Kill the temporary process before starting systemd service
echo "🔄 Cleaning up temporary process..."
kill $NEW_APP_PID 2>/dev/null || true
sleep 2

# Start new service on correct port via systemd
echo "🔄 Starting new production service on port $APP_PORT via systemd..."
sudo systemctl daemon-reload
sudo systemctl start "$SERVICE_NAME"

# Verify new service is healthy
echo "🏥 Verifying new production service health..."
SYSTEMD_HEALTH_PASSED=false

for i in {1..20}; do
    if curl -f --connect-timeout 5 --max-time 10 "http://localhost:$APP_PORT$HEALTH_ENDPOINT" > /dev/null 2>&1; then
        echo "✅ New production service is healthy!"
        SYSTEMD_HEALTH_PASSED=true
        break
    fi
    
    # Check systemd status
    if [ $((i % 5)) -eq 0 ]; then
        echo "🔍 Systemd service status:"
        sudo systemctl status "$SERVICE_NAME" --no-pager -l || true
    fi
    
    sleep 2
done

if [ "$SYSTEMD_HEALTH_PASSED" = false ]; then
    echo "❌ New production service health check failed - attempting rollback"
    
    # Show systemd logs for debugging
    echo "🔍 Systemd service logs:"
    sudo journalctl -u "$SERVICE_NAME" --no-pager -l -n 50 || true
    
    # Attempt rollback if backup exists
    if [ -f "$BACKUP_FILE" ]; then
        echo "🔄 Rolling back to previous version..."
        sudo systemctl stop "$SERVICE_NAME" || true
        cp "$BACKUP_FILE" "$SERVICE_NAME"
        chmod +x "$SERVICE_NAME"
        sudo systemctl start "$SERVICE_NAME"
        
        # Quick health check of rollback
        sleep 5
        if curl -f "http://localhost:$APP_PORT$HEALTH_ENDPOINT" > /dev/null 2>&1; then
            echo "✅ Rollback successful"
        else
            echo "❌ Rollback also failed - manual intervention required"
        fi
    else
        echo "❌ No backup available for rollback"
    fi
    
    exit 1
fi

# Final cleanup
rm -f /tmp/new-production-app.log

echo "🎉 Zero-downtime production deployment completed successfully!"
echo "📱 New binary running for commit: $COMMIT_SHA"
echo "🌐 Health check: http://localhost:$APP_PORT$HEALTH_ENDPOINT"
echo "📊 Deployment completed at: $(date)"

# Optional: Record deployment in log
echo "$(date): Deployed commit $COMMIT_SHA successfully" >> "$BACKUP_DIR/deployment.log"
