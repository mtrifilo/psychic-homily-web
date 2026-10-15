#!/bin/sh
set -e

echo "Starting entrypoint script (v3)..."
# Prints a database URL as scheme://***@host:port/db: the userinfo becomes ***
# and the query (which can carry password=) and any fragment are dropped.
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
# MIGRATE_LOCK_TIMEOUT overrides the 5s default; an invalid value or zero
# (which Postgres reads as "no limit") falls back to 5s, never to unbounded.
#
# MIGRATE_STATEMENT_TIMEOUT, when not 0, adds a statement_timeout to the same
# option string, which also bounds how long a granted lock is held. It
# defaults to 0, which adds nothing, so the role or server setting applies.
#
# scripts/check_migration_lock_timeout.sh reads the option string and the
# -database arguments below; keep them in step with it.

# Succeeds when $1 is a whole number with an optional Postgres time unit
# (us, ms, s, min, h, d); a bare number is milliseconds. This also keeps the
# value free of characters that would change the URL around it.
is_pg_duration() {
    pg_duration_unit=${1##*[[:digit:]]}
    case ${1%"$pg_duration_unit"} in
        ''|*[![:digit:]]*) return 1 ;;
    esac
    case $pg_duration_unit in
        ''|us|ms|s|min|h|d) return 0 ;;
    esac
    return 1
}

# Succeeds when the number part of valid duration $1 is zero.
is_zero_duration() {
    case ${1%%[![:digit:]]*} in
        *[!0]*) return 1 ;;
    esac
    return 0
}

MIGRATE_LOCK_TIMEOUT=${MIGRATE_LOCK_TIMEOUT:-5s}
if ! is_pg_duration "$MIGRATE_LOCK_TIMEOUT" || is_zero_duration "$MIGRATE_LOCK_TIMEOUT"; then
    echo "WARNING: MIGRATE_LOCK_TIMEOUT is not a positive whole number with an optional unit (us, ms, s, min, h, d); using 5s."
    MIGRATE_LOCK_TIMEOUT=5s
fi
MIGRATE_STATEMENT_TIMEOUT=${MIGRATE_STATEMENT_TIMEOUT:-0}
if ! is_pg_duration "$MIGRATE_STATEMENT_TIMEOUT"; then
    echo "WARNING: MIGRATE_STATEMENT_TIMEOUT is not a whole number with an optional unit (us, ms, s, min, h, d); using 0 (not set)."
    MIGRATE_STATEMENT_TIMEOUT=0
fi
# From here on an empty MIGRATE_STATEMENT_TIMEOUT means "not set".
if is_zero_duration "$MIGRATE_STATEMENT_TIMEOUT"; then
    MIGRATE_STATEMENT_TIMEOUT=""
fi
# A bare number is milliseconds; naming the unit keeps the boot lines unambiguous.
case $MIGRATE_LOCK_TIMEOUT in *[[:digit:]]) MIGRATE_LOCK_TIMEOUT="${MIGRATE_LOCK_TIMEOUT}ms" ;; esac
case $MIGRATE_STATEMENT_TIMEOUT in *[[:digit:]]) MIGRATE_STATEMENT_TIMEOUT="${MIGRATE_STATEMENT_TIMEOUT}ms" ;; esac

MIGRATE_OPTIONS_PARAM="options=-c%20lock_timeout%3D${MIGRATE_LOCK_TIMEOUT}"
if [ -n "$MIGRATE_STATEMENT_TIMEOUT" ]; then
    MIGRATE_OPTIONS_PARAM="${MIGRATE_OPTIONS_PARAM}%20-c%20statement_timeout%3D${MIGRATE_STATEMENT_TIMEOUT}"
fi
case "$DATABASE_URL" in
    *\?*) MIGRATE_DATABASE_URL="${DATABASE_URL}&${MIGRATE_OPTIONS_PARAM}" ;;
    *) MIGRATE_DATABASE_URL="${DATABASE_URL}?${MIGRATE_OPTIONS_PARAM}" ;;
esac
# The driver reads only the first `options` parameter, so one already in
# DATABASE_URL replaces both timeouts above.
case "$DATABASE_URL" in
    *options=*) echo "WARNING: DATABASE_URL carries its own 'options' parameter; migrations run WITHOUT the ${MIGRATE_LOCK_TIMEOUT} lock_timeout or any statement_timeout from MIGRATE_STATEMENT_TIMEOUT." ;;
    *)
        echo "Migrations wait at most ${MIGRATE_LOCK_TIMEOUT} for any lock (lock_timeout)."
        if [ -n "$MIGRATE_STATEMENT_TIMEOUT" ]; then
            echo "Each migration statement runs at most ${MIGRATE_STATEMENT_TIMEOUT} (statement_timeout)."
        else
            echo "Migrations get no statement_timeout from this script (MIGRATE_STATEMENT_TIMEOUT=0)."
        fi
        ;;
esac

echo "Running database migrations..."
if ! migrate -path /app/db/migrations -database "$MIGRATE_DATABASE_URL" up; then
    # A failed migration leaves its version marked dirty, and every later
    # `migrate up` refuses until the version is forced.
    echo "ERROR: Migration failed! Schema version now: $(migrate -path /app/db/migrations -database "$MIGRATE_DATABASE_URL" version 2>&1)"
    echo "ERROR: Boot refused. If that version is (dirty): confirm its DDL did not apply (drop any INVALID index a CONCURRENTLY build left), run 'migrate force <previous version>', then redeploy."
    exit 1
fi
echo "Migrations completed successfully!"

# Start the application
echo "Starting application..."
exec ./main
