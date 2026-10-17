# shellcheck shell=bash
# scripts/lib/cic_if_not_exists.sh
#
# Sourced by check_migration_lock_timeout.sh and its self-test so both apply
# one rule for CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS. Defines
# constants and functions only; sourcing it runs nothing.

# The newest migration version merged before CREATE INDEX CONCURRENTLY IF NOT
# EXISTS became an error. Fixed: raising it would grandfather new migrations.
CIC_IF_NOT_EXISTS_GRANDFATHERED_THROUGH=20261015210000

CIC_IF_NOT_EXISTS='create[[:space:]]+(unique[[:space:]]+)?index[[:space:]]+concurrently[[:space:]]+if[[:space:]]+not[[:space:]]+exists'

# cic_if_not_exists_in <sql>: true when <sql> (comments stripped, lowercase)
# contains CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS. A here-string, not
# a pipe, feeds grep -q: under pipefail an early-exiting grep can SIGPIPE its
# writer and turn a match into a miss.
cic_if_not_exists_in() {
  grep -qE "$CIC_IF_NOT_EXISTS" <<<"$1"
}

# cic_grandfathered <file>: true when the file's version (the digits before the
# first '_' of its name) is at or below CIC_IF_NOT_EXISTS_GRANDFATHERED_THROUGH.
# A version that is empty, not all digits, or longer than a timestamp is not.
cic_grandfathered() {
  local version="${1##*/}"
  version="${version%%_*}"
  case "$version" in
    '' | *[!0-9]*) return 1 ;;
  esac
  [ "${#version}" -le 14 ] || return 1
  [ "$((10#$version))" -le "$((10#$CIC_IF_NOT_EXISTS_GRANDFATHERED_THROUGH))" ]
}
