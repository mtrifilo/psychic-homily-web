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
#
# Every statement in the migrate run waits at most MIGRATE_LOCK_TIMEOUT for a
# lock: an ALTER TABLE's ACCESS EXCLUSIVE lock, a LOCK TABLE, golang-migrate's
# own advisory lock, and CREATE INDEX CONCURRENTLY's wait for older
# transactions (a CONCURRENTLY build that times out leaves an INVALID index
# behind, which IF NOT EXISTS then skips). Unbounded, a migration queued behind
# a long transaction waits forever, every later query on that table queues
# behind it, and stopping this container does not end the wait on the server.
# The bound is a libpq startup option on the migrate connection only, enforced
# by the server; the application server connects with DATABASE_URL unchanged.
# It bounds the wait to acquire a lock, not how long a granted lock is held.
#
# scripts/check_migration_lock_timeout.sh reads the option string and the
# -database arguments below; keep them in step with it.
MIGRATE_LOCK_TIMEOUT=5s
MIGRATE_LOCK_TIMEOUT_OPTION="options=-c%20lock_timeout%3D${MIGRATE_LOCK_TIMEOUT}"
case "$DATABASE_URL" in
    *\?*) MIGRATE_DATABASE_URL="${DATABASE_URL}&${MIGRATE_LOCK_TIMEOUT_OPTION}" ;;
    *) MIGRATE_DATABASE_URL="${DATABASE_URL}?${MIGRATE_LOCK_TIMEOUT_OPTION}" ;;
esac
# The driver reads only the first `options` parameter, so one already in
# DATABASE_URL replaces the lock_timeout above.
case "$DATABASE_URL" in
    *options=*) echo "WARNING: DATABASE_URL carries its own 'options' parameter; migrations run WITHOUT the ${MIGRATE_LOCK_TIMEOUT} lock_timeout." ;;
    *) echo "Migrations wait at most ${MIGRATE_LOCK_TIMEOUT} for any lock (lock_timeout)." ;;
esac

echo "Running database migrations..."
if ! migrate -path /app/db/migrations -database "$MIGRATE_DATABASE_URL" up; then
    # A failed migration leaves its version marked dirty, and every later
    # `migrate up` refuses until the version is forced.
    echo "ERROR: Migration failed! Schema version now: $(migrate -path /app/db/migrations -database "$MIGRATE_DATABASE_URL" version 2>&1)"
    echo "ERROR: Boot refused. If that version is (dirty): confirm its DDL did not apply, run 'migrate force <previous version>', then redeploy."
    exit 1
fi
echo "Migrations completed successfully!"

# Start the application
echo "Starting application..."
exec ./main
