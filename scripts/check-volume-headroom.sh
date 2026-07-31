#!/usr/bin/env bash
#
# check-volume-headroom.sh — is the Postgres volume big enough to survive?
#
# PSY-1643. On 2026-07-29 production was down ~75 minutes because the Postgres
# volume filled and WAL could not be written during recovery. The volume was
# 500 MB. `max_wal_size` was 1024 MB. Postgres was configured to use up to twice
# the entire disk for WAL alone, before counting a 262 MB database — so the
# volume could never survive a full WAL cycle. It was undersized by construction,
# not by margin, and the crash was a matter of when.
#
# That is why this checks a FLOOR and not just a percentage. A "warn at 80% full"
# alert would have said nothing useful: the volume was already doomed at 0% used,
# because the configured ceiling exceeded its capacity. The percentage check is
# still here for gradual growth, but the floor is what catches a misconfiguration.
#
#   floor = pg_database_size + max_wal_size + overhead
#
# Usage:
#   scripts/check-volume-headroom.sh [environment]     # default: production
#
# Exit codes:
#   0  volume clears the floor and is under the usage threshold
#   1  volume is BELOW the floor — it cannot hold the database plus a full WAL cycle
#   2  volume clears the floor but usage is past the threshold
#   3  could not determine (missing tooling, unreachable database)
#
# Secrets: never prints the database URL. Only sizes and names.

set -euo pipefail

ENVIRONMENT="${1:-production}"
DB_SERVICE="${VOLUME_CHECK_DB_SERVICE:-Postgres}"
VOLUME_NAME="${VOLUME_CHECK_VOLUME:-postgres-volume}"
USAGE_THRESHOLD_PCT="${VOLUME_CHECK_USAGE_PCT:-75}"
# Filesystem + Postgres overhead beyond database and WAL: temp files, logs, the
# free-space margin the filesystem needs to not be pathological.
OVERHEAD_PCT="${VOLUME_CHECK_OVERHEAD_PCT:-25}"

for bin in railway jq psql; do
  command -v "$bin" >/dev/null 2>&1 || { echo "ERROR: $bin not found on PATH" >&2; exit 3; }
done

# `railway volume list` has no -e flag: it reads the LINKED environment. So this
# has to link, read, and relink — and must relink even if it dies partway, or it
# leaves a project pointed at production where a later bare railway command would
# act on the wrong environment.
ORIGINAL_ENV="$(railway status 2>/dev/null | awk -F': ' '/^Environment:/ {print $2}' | tr -d '[:space:]')"
restore_env() {
  if [ -n "${ORIGINAL_ENV:-}" ] && [ "$ORIGINAL_ENV" != "$ENVIRONMENT" ]; then
    railway environment "$ORIGINAL_ENV" >/dev/null 2>&1 || \
      echo "WARNING: could not relink environment '$ORIGINAL_ENV' — check 'railway status'" >&2
  fi
}
trap restore_env EXIT

railway environment "$ENVIRONMENT" >/dev/null 2>&1 || {
  echo "ERROR: could not link environment '$ENVIRONMENT'" >&2; exit 3; }

volume_json="$(railway volume list --json 2>/dev/null || true)"
used_mb="$(printf '%s' "$volume_json" | jq -r --arg n "$VOLUME_NAME" \
  '.volumes[]? | select(.name==$n) | .currentSizeMB' 2>/dev/null || true)"
size_mb="$(printf '%s' "$volume_json" | jq -r --arg n "$VOLUME_NAME" \
  '.volumes[]? | select(.name==$n) | .sizeMB' 2>/dev/null || true)"

if [ -z "$used_mb" ] || [ "$used_mb" = "null" ]; then
  echo "ERROR: volume '$VOLUME_NAME' not found in environment '$ENVIRONMENT'" >&2
  exit 3
fi

db_url="$(railway variables --json -s "$DB_SERVICE" -e "$ENVIRONMENT" 2>/dev/null \
  | jq -r '.DATABASE_PUBLIC_URL // empty')"
if [ -z "$db_url" ]; then
  echo "ERROR: no DATABASE_PUBLIC_URL for service '$DB_SERVICE' in '$ENVIRONMENT'" >&2
  exit 3
fi

# One round trip for both numbers. max_wal_size is reported in MB by pg_settings.
read -r db_mb max_wal_mb <<<"$(psql "$db_url" -tAF' ' -c \
  "SELECT ceil(pg_database_size(current_database())/1048576.0),
          (SELECT setting::bigint FROM pg_settings WHERE name='max_wal_size');" 2>/dev/null || true)"

if [ -z "${db_mb:-}" ] || [ -z "${max_wal_mb:-}" ]; then
  echo "ERROR: could not read database size / max_wal_size from '$ENVIRONMENT'" >&2
  exit 3
fi

floor_mb="$(awk -v d="$db_mb" -v w="$max_wal_mb" -v o="$OVERHEAD_PCT" \
  'BEGIN { printf "%.0f", (d + w) * (1 + o/100) }')"
used_pct="$(awk -v u="$used_mb" -v s="$size_mb" 'BEGIN { printf "%.1f", (u/s)*100 }')"

printf '\n  Postgres volume headroom — %s\n\n' "$ENVIRONMENT"
printf '    volume            %8.0f MB / %.0f MB  (%s%% used)\n' "$used_mb" "$size_mb" "$used_pct"
printf '    database          %8s MB\n' "$db_mb"
printf '    max_wal_size      %8s MB   <- Postgres may use this much WAL\n' "$max_wal_mb"
printf '    required floor    %8s MB   (database + max_wal_size + %s%% overhead)\n\n' \
  "$floor_mb" "$OVERHEAD_PCT"

if awk -v s="$size_mb" -v f="$floor_mb" 'BEGIN { exit !(s < f) }'; then
  cat >&2 <<EOF
  FAIL: the volume is SMALLER than the floor.

  It cannot hold the database plus a full WAL cycle, so it will fill regardless of
  how empty it looks right now. This is the shape of the PSY-1643 outage: a 500 MB
  volume with max_wal_size=1024 MB.

  Fix: grow the volume to at least ${floor_mb} MB (Railway dashboard — the CLI's
  'railway volume update' has no size flag), or lower max_wal_size.

EOF
  exit 1
fi

if awk -v p="$used_pct" -v t="$USAGE_THRESHOLD_PCT" 'BEGIN { exit !(p >= t) }'; then
  printf '  WARN: %s%% used, at or past the %s%% threshold. Volume clears the floor,\n' \
    "$used_pct" "$USAGE_THRESHOLD_PCT" >&2
  printf '  so this is gradual growth rather than a misconfiguration — but plan a resize.\n\n' >&2
  exit 2
fi

printf '  OK: clears the floor (%.1fx) and under the %s%% usage threshold.\n\n' \
  "$(awk -v s="$size_mb" -v f="$floor_mb" 'BEGIN { printf "%.1f", s/f }')" \
  "$USAGE_THRESHOLD_PCT"
