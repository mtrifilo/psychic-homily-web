#!/bin/sh
set -e

echo "Starting entrypoint script (v3)..."
echo "DEBUG: DATABASE_URL=${DATABASE_URL:-NOT SET}"

# Check if DATABASE_URL is set
if [ -z "$DATABASE_URL" ]; then
    echo "ERROR: DATABASE_URL is not set!"
    echo "In Railway, add a Variable Reference from the Postgres service"
    exit 1
fi

# Check if running on Railway (uses internal hostnames)
if echo "$DATABASE_URL" | grep -q "railway.internal"; then
    echo "Detected Railway environment - skipping netcat health check"
    echo "Railway manages database availability internally"
else
    # Extract DB_HOST and DB_PORT from DATABASE_URL if not explicitly set
    # DATABASE_URL format: postgres://user:pass@host:port/dbname?params
    if [ -z "$DB_HOST" ] && [ -n "$DATABASE_URL" ]; then
        DB_HOST=$(echo "$DATABASE_URL" | sed -n 's|.*@\([^:/]*\).*|\1|p')
        DB_PORT=$(echo "$DATABASE_URL" | sed -n 's|.*@[^:]*:\([0-9]*\)/.*|\1|p')
        DB_PORT=${DB_PORT:-5432}
        echo "Extracted from DATABASE_URL: DB_HOST=$DB_HOST, DB_PORT=$DB_PORT"
    fi

    # Wait for database to be ready
    echo "Waiting for database at ${DB_HOST}:${DB_PORT}..."
    max_attempts=30
    attempt=1

    while ! nc -z "$DB_HOST" "$DB_PORT" 2>/dev/null; do
        if [ $attempt -ge $max_attempts ]; then
            echo "ERROR: Database not available after $max_attempts attempts. Exiting."
            exit 1
        fi
        echo "Attempt $attempt/$max_attempts: Database not ready, waiting..."
        sleep 2
        attempt=$((attempt + 1))
    done

    echo "Database is ready!"
fi

# Run migrations
echo "Running database migrations..."
migrate -path /app/db/migrations -database "$DATABASE_URL" up

if [ $? -eq 0 ]; then
    echo "Migrations completed successfully!"
else
    echo "ERROR: Migration failed!"
    exit 1
fi

# Start the application
echo "Starting application..."
exec ./main
