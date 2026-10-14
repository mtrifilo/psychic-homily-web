#!/bin/sh
set -e

echo "Starting entrypoint script (v3)..."
# Prints a database URL as scheme://***@host:port/db: the userinfo becomes ***
# and the query and fragment are dropped, since both can carry a password.
# The host part is the text after the LAST '@', so a password holding a literal
# '@' or '/' stays hidden. Prints "NOT SET" for an empty URL. Returns 1 without
# printing when the input does not reduce to that shape: no scheme, a '?' or
# '#' before the last '@', or a host part with characters outside [:alnum:]
# and ._:/-.
redact_database_url() {
    redact_url=$1
    if [ -z "$redact_url" ]; then
        echo "NOT SET"
        return 0
    fi
    case $redact_url in
        *://*) ;;
        *) return 1 ;;
    esac
    redact_scheme=${redact_url%%://*}
    case $redact_scheme in
        ''|*[![:alnum:]+.-]*) return 1 ;;
    esac
    redact_rest=${redact_url#*://}
    redact_userinfo=""
    case $redact_rest in
        *@*)
            case ${redact_rest%@*} in
                *[?#]*) return 1 ;;
            esac
            redact_rest=${redact_rest##*@}
            redact_userinfo="***@"
            ;;
    esac
    redact_rest=${redact_rest%%[?#]*}
    case $redact_rest in
        ''|*[![:alnum:]._:/-]*) return 1 ;;
    esac
    printf '%s://%s%s\n' "$redact_scheme" "$redact_userinfo" "$redact_rest"
}

echo "DATABASE_URL: $(redact_database_url "$DATABASE_URL" || echo "set (not shown)")"

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
