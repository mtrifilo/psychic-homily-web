#!/bin/sh
set -e

echo "Starting entrypoint script..."

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
