#!/usr/bin/env bash
#
# Capture a durable search-demand snapshot from the Google Search Console API.
#
# WHY THIS EXISTS
# ---------------
# Vercel Web Analytics (scripts/traffic-snapshot.sh) sees only search clicks
# that became visits. Impressions, queries, CTR, and position — the demand we
# rank for but fail to convert — live in Search Console. This script captures
# that side of the funnel into a markdown record next to the Vercel snapshot.
#
# The monthly capture is a PAIR: run traffic-snapshot.sh and this script over
# the same window. Retention differs by side — the Search Analytics API
# serves ~16 months of history, Vercel ~12 months on the current Pro plan —
# so a missed month is recoverable on both sides for a while, but capture on
# schedule anyway: the pairing discipline exists for comparability.
#
# Both scripts default their window end to today-3 (GSC data lags 2-3 days),
# so bare same-day paired runs cover the same window and produce name-linked
# files. Explicit --since/--until on both is still preferred: it makes the
# capture reproducible and immune to a date rollover between the two runs.
#
# OUT OF SCOPE: the Index Coverage report (indexed / not-indexed counts by
# reason) has no bulk public API — only the per-URL, quota-limited URL
# Inspection API. Index coverage stays a manual read in the GSC UI; the
# 2026-08-17 manual capture in docs/research/gsc-baseline-2026-08.md shows
# what to record.
#
# Usage:
#   scripts/gsc-snapshot.sh [--out DIR] [--days N] [--since DATE] [--until DATE] [--force]
#
#   --out DIR      Directory to write the snapshot into. Default: docs/research
#                  relative to the repo root. NOTE: docs/ is gitignored and is
#                  absent from fresh worktrees, so pass --out explicitly when
#                  running anywhere other than the main checkout.
#   --days N       Length of the trailing window in days. Default: 28.
#   --since DATE   Explicit window start (YYYY-MM-DD). Overrides --days.
#   --until DATE   Explicit window end (YYYY-MM-DD), inclusive. Default: three
#                  days before today (UTC) — GSC data lags 2-3 days (a 3-day
#                  lag was observed on the first real capture), so ending near
#                  today under-serves the tail. The generated doc reconciles
#                  the served range against the requested one either way.
#   --force        Overwrite an existing snapshot for this window (refused by
#                  default).
#
# Requires: bash, curl, jq, python3, and the gcloud CLI holding an Application
# Default Credential that includes the webmasters.readonly scope. The script
# prints TWO login commands on auth failure: a narrow webmasters.readonly one
# to try first, and a broad cloud-platform fallback (the only combination
# verified 2026-08-26) — see the scope note beside the token mint below.

set -euo pipefail

# --- Configuration ----------------------------------------------------------

# Overridable so the script can be pointed at another property without edits.
SITE="${GSC_SITE:-sc-domain:psychichomily.com}"
# ADC user credentials must name a quota project or the API 403s with
# SERVICE_DISABLED for the OAuth client's own project (verified 2026-08-26).
QUOTA_PROJECT="${GSC_QUOTA_PROJECT:-psychic-homily}"
API_BASE="${GSC_API:-https://searchconsole.googleapis.com/webmasters/v3}"

# Fetch limits are the API's per-request maximum, deliberately decoupled from
# the display caps below. The API sorts rows by clicks DESCENDING and breaks
# ties ALPHABETICALLY, so a small fetch is an alphabet accident, not a
# ranking: zero-click rows all sit at the tail and get cut mid-letter, and
# the low-click tie blocks of the queries/pages tables get cut the same way.
# Fetching at the cap keeps membership complete only while the window stays
# under 25000 rows per dimension — a cap hit is detected after each fetch and
# stamped into the doc rather than silently dropping the tail (there is no
# startRow pagination here).
QUERY_FETCH_LIMIT=25000
PAGE_FETCH_LIMIT=25000
QUERY_TABLE_LIMIT=25
PAGE_TABLE_LIMIT=25
ZERO_CLICK_LIMIT=20
ZERO_CLICK_MIN_IMPRESSIONS=5
ZERO_CLICK_MAX_POSITION=11

WINDOW_DAYS=28
SINCE=""
UNTIL=""
OUT_DIR=""
FORCE=0

# --- Helpers ----------------------------------------------------------------

die() {
  printf 'gsc-snapshot: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Capture a durable search-demand snapshot from the Google Search Console API.

Usage:
  scripts/gsc-snapshot.sh [--out DIR] [--days N] [--since DATE] [--until DATE] [--force]

  --out DIR      Directory to write the snapshot into. Default: docs/research
                 relative to the repo root. docs/ is gitignored and absent from
                 fresh worktrees, so pass this explicitly outside the main
                 checkout.
  --days N       Length of the trailing window in days. Default: 28.
  --since DATE   Explicit window start (YYYY-MM-DD). Overrides --days.
  --until DATE   Explicit window end (YYYY-MM-DD), inclusive. Default: three
                 days before today (UTC), because GSC data lags 2-3 days —
                 matching traffic-snapshot.sh's default, so bare paired runs
                 cover the same window. Explicit dates on both are still
                 preferred for reproducibility.
  --force        Overwrite an existing snapshot for this window. Refused by
                 default, because the generated doc carries hand-written
                 analysis.

Requires curl, jq, python3, and a gcloud Application Default Credential that
includes the webmasters.readonly scope. On auth failure the script prints a
narrow webmasters.readonly login to try first and a broad cloud-platform
fallback (the only combination verified so far) — see the scope note in the
script.
USAGE
}

# The final stdout line is this script's machine-readable contract (the path it
# wrote), so usage text on the error path must go to stderr or a caller doing
# `path="$(gsc-snapshot.sh --typo)"` captures the whole help block as a path.
usage_and_exit() {
  if [ "${1:-0}" -eq 0 ]; then usage; else usage >&2; fi
  exit "${1:-0}"
}

# Reject a missing value that would otherwise swallow the next flag, so
# `--out --days 7` reports the real mistake instead of dying on a stray `7`.
require_value() {
  case "${2:-}" in
    ''|--*) die "$1 requires a value" ;;
  esac
}

# Shift an ISO date by N days. python3 rather than `date -d` / `date -v`, which
# are mutually incompatible between GNU and BSD userlands.
shift_date() {
  python3 -c '
import datetime, sys
base = datetime.date.fromisoformat(sys.argv[1])
print(base + datetime.timedelta(days=int(sys.argv[2])))
' "$1" "$2"
}

require_iso_date() {
  # The glob is not redundant with the Python parse below. `date.fromisoformat`
  # widened in Python 3.11 to accept `20260813` and `2026-W33-1`, which would
  # pass validation and then flow verbatim into the request body and the doc's
  # window header. Pinning the shape here keeps behavior identical across
  # interpreter versions.
  case "$1" in
    [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;;
    *) die "invalid date '$1' for $2 (expected YYYY-MM-DD)" ;;
  esac
  python3 -c '
import datetime, sys
try:
    datetime.date.fromisoformat(sys.argv[1])
except ValueError:
    sys.exit(1)
' "$1" || die "invalid date '$1' for $2 (expected YYYY-MM-DD)"
}

# --- Argument parsing -------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --out)   require_value --out "${2:-}";   OUT_DIR="$2"; shift 2 ;;
    --days)  require_value --days "${2:-}";  WINDOW_DAYS="$2"; shift 2 ;;
    --since) require_value --since "${2:-}"; SINCE="$2"; shift 2 ;;
    --until) require_value --until "${2:-}"; UNTIL="$2"; shift 2 ;;
    --force) FORCE=1; shift ;;
    -h|--help) usage_and_exit 0 ;;
    *) printf 'gsc-snapshot: unknown argument %s\n\n' "$1" >&2; usage_and_exit 1 ;;
  esac
done

command -v curl    >/dev/null 2>&1 || die "curl is required"
command -v jq      >/dev/null 2>&1 || die "jq is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

# The brew cask installs gcloud outside the default PATH; check its home before
# giving up so a fresh shell that hasn't re-sourced ~/.zshrc still works.
GCLOUD="$(command -v gcloud || true)"
if [ -z "$GCLOUD" ] && [ -x /opt/homebrew/share/google-cloud-sdk/bin/gcloud ]; then
  GCLOUD=/opt/homebrew/share/google-cloud-sdk/bin/gcloud
fi
[ -n "$GCLOUD" ] || die "gcloud CLI not found. Install: brew install --cask google-cloud-sdk"

# Pin the API base to Google's own hosts over HTTPS: the bearer token is
# attached to every request, so a stray override (typo, inherited env in an
# agent-dispatch repo) would otherwise ship the credential in cleartext or to
# an arbitrary host — silently, since the script would just report an HTTP
# error. This deliberately sacrifices mock-server overrides; unit-test the jq
# and shell fragments instead.
case "$API_BASE" in
  https://searchconsole.googleapis.com/*|https://www.googleapis.com/*) ;;
  *) die "GSC_API must be an https:// googleapis.com endpoint (got: ${API_BASE})" ;;
esac

case "$WINDOW_DAYS" in
  ''|*[!0-9]*) die "--days must be a positive integer, got '$WINDOW_DAYS'" ;;
esac
# Normalize before any $(( )) sees it: a leading zero ("08") would otherwise
# be read as octal — an arithmetic error, or a silently smaller window.
WINDOW_DAYS=$((10#$WINDOW_DAYS))
[ "$WINDOW_DAYS" -gt 0 ] || die "--days must be greater than zero"

# GSC data lags 2-3 days behind real time (a 3-day lag was observed on the
# first real capture); default the window end to three days ago so the tail
# of the daily series is real data. This is best effort — the generated doc
# reconciles the served range against the requested one regardless.
if [ -z "$UNTIL" ]; then
  UNTIL="$(shift_date "$(date -u +%Y-%m-%d)" -3)"
fi
require_iso_date "$UNTIL" "--until"

if [ -n "$SINCE" ]; then
  require_iso_date "$SINCE" "--since"
else
  SINCE="$(shift_date "$UNTIL" "-$((WINDOW_DAYS - 1))")"
fi

# Recompute the span from the resolved dates rather than trusting --days, which
# is meaningless once --since is given explicitly.
WINDOW_DAYS="$(python3 -c '
import datetime, sys
start = datetime.date.fromisoformat(sys.argv[1])
end = datetime.date.fromisoformat(sys.argv[2])
if start > end:
    sys.exit(1)
print((end - start).days + 1)
' "$SINCE" "$UNTIL")" || die "--since ($SINCE) must not be after --until ($UNTIL)"

# Strip trailing slashes so the reported path does not come out doubled.
while [ "${OUT_DIR}" != "${OUT_DIR%/}" ] && [ -n "${OUT_DIR%/}" ]; do
  OUT_DIR="${OUT_DIR%/}"
done

if [ -z "$OUT_DIR" ]; then
  REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" \
    || die "not inside a git repo; pass --out explicitly"
  OUT_DIR="$REPO_ROOT/docs/research"
fi

CAPTURED_ON="$(date -u +%Y-%m-%d)"
# Keyed on the WINDOW, not the capture date: the header advertises multi-month
# backfill (the API serves ~16 months), and several same-day backfill runs
# would otherwise collide on one filename — with --force then silently
# overwriting a DIFFERENT window's snapshot and its hand-written analysis.
DOC_NAME="gsc-snapshot-${SINCE}_${UNTIL}.md"

# Early check so a mistaken re-run fails immediately instead of after seven
# API calls; the publication step's `mv -n` is what actually closes the
# overwrite race.
if [ -e "$OUT_DIR/$DOC_NAME" ] && [ "$FORCE" -ne 1 ]; then
  die "$OUT_DIR/$DOC_NAME already exists; move it or pass --force to overwrite"
fi

# --- API access -------------------------------------------------------------

# Everything is staged in a scratch directory and moved into place only after
# every request has succeeded, so a mid-run API failure can never leave a
# half-written snapshot that later reads as a complete record.
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gsc-snapshot.XXXXXX")"
cleanup() { rm -rf "$WORK_DIR"; }
trap cleanup EXIT

# Mint an access token from the Application Default Credential. This is the
# single auth touchpoint; every failure mode lands here, so the error message
# carries the exact re-auth command instead of a pointer to documentation.
#
# SCOPE NOTE: the 2026-08-26 verification granted BOTH cloud-platform (broad
# — full GCP authority for the signed-in account, not just Search Console)
# and webmasters.readonly, and that combination is known to pass the
# x-goog-user-project quota check. Whether webmasters.readonly ALONE would
# suffice has NOT been verified, so the auth guidance below suggests the
# narrow scope first and the broad one only as a fallback — whichever works,
# record the outcome here and close this open question. The login command
# OVERWRITES the machine-wide ADC file every Google client library reads; the
# success path prints the revoke command for dropping the credential between
# monthly runs.
print_auth_help() {
  printf 'gsc-snapshot: (re)authenticate with (narrow read-only scope — try this first):\n' >&2
  printf 'gsc-snapshot:   gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly\n' >&2
  printf 'gsc-snapshot: if the API then 403s on the quota project, fall back to the broad variant:\n' >&2
  printf 'gsc-snapshot:   gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/webmasters.readonly\n' >&2
  printf 'gsc-snapshot: the ADC is per-machine/per-user, not repo-local — worktrees inherit it; a new machine needs this one-time login.\n' >&2
}

if ! TOKEN="$("$GCLOUD" auth application-default print-access-token 2>"$WORK_DIR/token-err")"; then
  printf 'gsc-snapshot: could not mint an access token from the Application Default Credential.\n' >&2
  sed 's/^/gsc-snapshot:   /' "$WORK_DIR/token-err" >&2
  print_auth_help
  exit 1
fi

SITE_ENC="$(jq -rn --arg s "$SITE" '$s | @uri')"

# api_call <outfile> <method> <url-path> [json-body]
#
# Fails the whole script on any transport error, any non-200 status, or any
# response carrying an `error` object. Sends the quota project on every call —
# without it the API rejects ADC user credentials outright.
api_call() {
  local out_file method path body url status message
  out_file="$1"
  method="$2"
  path="$3"
  body="${4:-}"

  url="${API_BASE}/${path}"

  set -- -sS --max-time 60 -o "$out_file" -w '%{http_code}' \
    --proto '=https' --proto-redir '=https' \
    -X "$method" -H "x-goog-user-project: ${QUOTA_PROJECT}" -K -
  if [ -n "$body" ]; then
    set -- "$@" -H 'Content-Type: application/json' --data "$body"
  fi

  # The credential goes in over stdin as a curl config rather than as a -H
  # argument, which would expose it in `ps` output to every process on the box
  # for the life of the request.
  if ! status="$(printf 'header = "Authorization: Bearer %s"\n' "$TOKEN" \
      | curl "$@" "$url")"; then
    die "request to ${path} failed (curl transport error)"
  fi

  if [ "$status" != "200" ]; then
    message="$(jq -r '.error.message // "no error message in body"' "$out_file" 2>/dev/null \
      || echo "unparseable response body")"
    if [ "$status" = "401" ] || [ "$status" = "403" ]; then
      printf 'gsc-snapshot: %s returned HTTP %s: %s\n' "$path" "$status" "$message" >&2
      print_auth_help
      exit 1
    fi
    die "${path} returned HTTP ${status}: ${message}"
  fi

  if ! jq -e 'type == "object"' "$out_file" >/dev/null 2>&1; then
    die "${path} returned a non-JSON response"
  fi

  if jq -e 'has("error")' "$out_file" >/dev/null 2>&1; then
    message="$(jq -r '.error.message // "unspecified error"' "$out_file")"
    die "${path} returned an error payload: ${message}"
  fi
}

# search_query <outfile> <dimensions-json-or-empty> <row-limit>
search_query() {
  local out_file dims limit body
  out_file="$1"
  dims="$2"
  limit="$3"
  body="$(jq -cn --arg s "$SINCE" --arg u "$UNTIL" --argjson limit "$limit" \
    --argjson dims "${dims:-null}" \
    '{startDate: $s, endDate: $u, rowLimit: $limit}
     + (if $dims then {dimensions: $dims} else {} end)')"
  api_call "$out_file" POST "sites/${SITE_ENC}/searchAnalytics/query" "$body"
}

# Upper bound only: endDate is inclusive, so the series has at most
# WINDOW_DAYS rows (the API omits unserved tail days rather than padding).
DAY_LIMIT=$WINDOW_DAYS

echo "gsc-snapshot: querying ${SINCE} → ${UNTIL} for ${SITE} ..." >&2

search_query "$WORK_DIR/totals.json"     ''                  1
search_query "$WORK_DIR/daily.json"      '["date"]'          "$DAY_LIMIT"
search_query "$WORK_DIR/queries.json"    '["query"]'         "$QUERY_FETCH_LIMIT"
search_query "$WORK_DIR/query-page.json" '["query","page"]'  "$QUERY_FETCH_LIMIT"
search_query "$WORK_DIR/pages.json"      '["page"]'          "$PAGE_FETCH_LIMIT"
api_call     "$WORK_DIR/sitemaps.json" GET "sites/${SITE_ENC}/sitemaps"

row_count() { jq -r '(.rows // []) | length' "$1"; }

# A response exactly at the requested cap means the tail was cut — and because
# rows arrive clicks-descending with an alphabetical tie-break, the retained
# tail is an ARBITRARY ALPHABETICAL SLICE of the zero/low-click block, not a
# ranking of it. Warn on stderr, stamp the doc, and record which fetches were
# capped so downstream sections can refuse to rank a biased sample.
TRUNCATION_NOTE=""
CAPPED_FETCHES=""
check_cap() {
  # $1 = file, $2 = human label, $3 = the row limit that was requested
  local n
  n="$(row_count "$1")"
  if [ "$n" -ge "$3" ]; then
    echo "gsc-snapshot: WARNING: the $2 fetch hit the API's $3-row cap; the retained low-click tail is an arbitrary alphabetical slice, not a ranking" >&2
    CAPPED_FETCHES="${CAPPED_FETCHES} $2"
    TRUNCATION_NOTE="${TRUNCATION_NOTE}> **⚠ The $2 fetch hit the API's $3-row per-request cap.** The rows kept below the clicked block are an arbitrary alphabetical slice — wrong rows, not just fewer rows. Treat the affected tables as biased samples.
"
  fi
}
check_cap "$WORK_DIR/queries.json"    "query"      "$QUERY_FETCH_LIMIT"
check_cap "$WORK_DIR/query-page.json" "query+page" "$QUERY_FETCH_LIMIT"
check_cap "$WORK_DIR/pages.json"      "page"       "$PAGE_FETCH_LIMIT"

QUERY_ROW_COUNT="$(row_count "$WORK_DIR/queries.json")"
QUERY_PAGE_ROW_COUNT="$(row_count "$WORK_DIR/query-page.json")"
PAGE_ROW_COUNT="$(row_count "$WORK_DIR/pages.json")"

# Displayed-row counts for the section headings: "top 25 of 12 fetched" reads
# as a defect, so show the smaller number when the fetch is under the cap.
min() { if [ "$1" -lt "$2" ]; then echo "$1"; else echo "$2"; fi; }
QUERY_SHOWN="$(min "$QUERY_TABLE_LIMIT" "$QUERY_ROW_COUNT")"
PAGE_SHOWN="$(min "$PAGE_TABLE_LIMIT" "$PAGE_ROW_COUNT")"

# --- Rendering --------------------------------------------------------------

# Every rendered fragment is computed into a variable BEFORE the document is
# assembled, each guarded by `|| die`. Doing this inline in the heredoc instead
# would silently swallow failures: `set -e` does not propagate out of a command
# substitution, so a broken jq filter would emit an empty table and still exit
# zero, producing a partial snapshot that reads as a complete record.

# Cell hygiene, shared by every renderer below via JQ_DEFS: queries are
# arbitrary user-typed strings from the public internet. esc neutralizes the
# three things that corrupt or spoof a markdown table row — control and
# invisible-format characters incl. bidi overrides (row splitting, visual
# spoofing), backslashes (a "\|" payload would otherwise eat the pipe escape
# and split the cell — doubling keeps every pipe behind an odd backslash
# run; exact round-trip is unreachable, so rendered cells may show doubled
# backslashes and the doc's provenance trap says so), and literal pipes
# (column shifting). code always fences the cell, sizing the fence one
# backtick longer than the longest backtick run in the payload (CommonMark),
# so there is no unfenced fallback for a string an outsider crafted.
#
# metrics(): clicks, impressions, CTR, position — the four columns of every
# metric table. CTR arrives as a 0-1 fraction — multiply out explicitly (a
# formatting slip here already produced nonsense CTRs once during the
# 2026-08-26 credential verification). ONE definition on purpose: parallel
# copies of this formula would drift on the next format fix.
JQ_DEFS='
  def esc: tostring
    | gsub("[[:cntrl:]\u200b-\u200f\u2028\u2029\u202a-\u202e\u2066-\u2069\ufeff]"; " ")
    | gsub("\\\\"; "\\\\")
    | gsub("\\|"; "\\|");
  def code: esc | . as $s
    | ([ $s | match("`+"; "g").string | length ] | (max // 0) + 1) as $n
    | ("`" * $n) as $fence
    | (if $n > 1 then " " else "" end) as $pad
    | $fence + $pad + $s + $pad + $fence;
  def metrics(r):
    if r.clicks == 0 and r.impressions == 0
    then "0 | 0 | — | —"
    else "\(r.clicks) | \(r.impressions) | \((r.ctr * 10000 | round) / 100)% | \((r.position * 10 | round) / 10)"
    end;
'

render_metric_table() {
  # $1 = file, $2 = jq expression mapping a row to its display cell,
  # $3 = optional row cap (omit or 0 = all rows), $4 = "ranked" to re-sort by
  #      clicks then impressions before slicing. The API's own order breaks
  #      click ties ALPHABETICALLY, so slicing it directly turns a "top N"
  #      table into an alphabet accident past the first few rows; ranked
  #      re-sorting makes the tie-break meaningful. The daily series must
  #      NOT be ranked — it is date-ordered.
  local order='.'
  if [ "${4:-}" = "ranked" ]; then
    order='sort_by([-.clicks, -.impressions])'
  fi
  jq -r --argjson limit "${3:-0}" "$JQ_DEFS"'
    (.rows // []) | '"$order"'
    | (if $limit > 0 then .[:$limit] else . end)
    | if length == 0 then "| _no data in this window_ | | | | |"
      else (.[] | "| " + ('"$2"') + " | " + metrics(.) + " |")
      end
  ' "$1"
}

TOTAL_CLICKS="$(jq -re '(.rows // []) | (.[0].clicks // 0)' "$WORK_DIR/totals.json")" \
  || die "could not read the clicks total from the API response"
TOTAL_IMPRESSIONS="$(jq -re '(.rows // []) | (.[0].impressions // 0)' "$WORK_DIR/totals.json")" \
  || die "could not read the impressions total from the API response"
# Same zero-activity guard as metrics(): CTR over zero impressions is
# undefined and position 0 is not a possible SERP rank, so an all-zero
# window (a quiet backfilled month) renders "—", matching the daily table.
TOTAL_CTR="$(jq -re '(.rows // []) | if ((.[0].impressions // 0) == 0) then "—" else "\(((.[0].ctr // 0) * 10000 | round) / 100)" end' "$WORK_DIR/totals.json")" \
  || die "could not read the CTR total from the API response"
TOTAL_POSITION="$(jq -re '(.rows // []) | if ((.[0].impressions // 0) == 0) then "—" else "\(((.[0].position // 0) * 10 | round) / 10)" end' "$WORK_DIR/totals.json")" \
  || die "could not read the position total from the API response"

# The Search Analytics API does not echo the window it resolved (unlike the
# Vercel API), so coverage must be derived from the daily series itself. Row
# PRESENCE is the served-ness signal: the API OMITS unserved tail days
# (observed live: a 28-day request returned 26 rows, the two lag days simply
# absent) and EMITS zero rows for served days with no recorded activity (the
# pre-ramp week renders as genuine zeros). Do NOT filter on metric values
# here — a quiet backfilled month is all-zero rows and still fully served,
# and filtering would make the doc falsely claim the API served nothing.
LAST_SERVED_DAY="$(jq -re '(.rows // []) | map(.keys[0]) | max // "none"' "$WORK_DIR/daily.json")" \
  || die "could not read the daily series"
SERVED_ROWS="$(row_count "$WORK_DIR/daily.json")"
ACTIVE_DAYS="$(jq -re '(.rows // []) | map(select(.clicks > 0 or .impressions > 0)) | length' "$WORK_DIR/daily.json")" \
  || die "could not count active days in the daily series"

# Reconcile the served range against the requested one IN the doc, next to the
# totals, rather than leaving the reader to notice a short daily series. The
# first real capture shipped a 28-day header over 25 served days and its
# hand-written analysis quoted the totals as 28-day figures — this line exists
# so that cannot happen silently again. ISO dates compare lexicographically.
COVERAGE_LINE="Coverage: ${SERVED_ROWS} of the ${WINDOW_DAYS} requested days were served; ${ACTIVE_DAYS} of those show activity."
if [ "$LAST_SERVED_DAY" = "none" ]; then
  COVERAGE_LINE="**Coverage: the API returned no daily rows for this window.**"
  echo "gsc-snapshot: WARNING: no daily rows returned for ${SINCE} → ${UNTIL}" >&2
elif [ "$SERVED_ROWS" -lt "$WINDOW_DAYS" ] || [ "$LAST_SERVED_DAY" \< "$UNTIL" ]; then
  COVERAGE_LINE="**Coverage: ${SERVED_ROWS} of the ${WINDOW_DAYS} requested days were served (series ends ${LAST_SERVED_DAY}); ${ACTIVE_DAYS} served days show activity.** Days absent from the series were not served by the API (typically the recent lag tail) — totals cover served days only; do not quote them as ${WINDOW_DAYS}-day figures."
  echo "gsc-snapshot: WARNING: ${SERVED_ROWS}/${WINDOW_DAYS} days served; series ends ${LAST_SERVED_DAY} (requested ${UNTIL})" >&2
fi

# Page URLs come back absolute; strip the property's own origin for
# readability but leave foreign origins intact so they stay visible. The
# pattern is derived from $SITE (not hardcoded) so the GSC_SITE override keeps
# working: an sc-domain property covers every subdomain and both schemes,
# hence the permissive host prefix; a url-prefix property strips itself only.
if [ "${SITE#sc-domain:}" != "$SITE" ]; then
  ESCAPED="$(printf '%s' "${SITE#sc-domain:}" | sed 's/[][\.*^$()+?{|]/\\&/g')"
  STRIP_RE="^https?://([a-z0-9-]+\\.)*${ESCAPED}"
else
  # url-prefix siteUrl values carry a trailing slash; strip it before escaping
  # so page paths keep their leading "/" after the sub().
  ESCAPED="$(printf '%s' "${SITE%/}" | sed 's/[][\.*^$()+?{|]/\\&/g')"
  STRIP_RE="^${ESCAPED}"
fi
export STRIP_RE

TBL_DAILY="$(render_metric_table "$WORK_DIR/daily.json" '(.keys[0] | esc)')" \
  || die "failed to render the daily series"

TBL_QUERIES="$(render_metric_table "$WORK_DIR/queries.json" '(.keys[0] | code)' "$QUERY_TABLE_LIMIT" ranked)" \
  || die "failed to render the top-queries table"

TBL_PAGES="$(render_metric_table "$WORK_DIR/pages.json" '(.keys[0] | sub($ENV.STRIP_RE; ""; "i") | code)' "$PAGE_TABLE_LIMIT" ranked)" \
  || die "failed to render the top-pages table"

# Zero-click demand: (query, page) pairs with impressions, no clicks, and an
# average position under the page-one threshold. The query+page granularity
# names the URL whose title/snippet needs the work — a query ranking with two
# URLs appears twice. If the query+page fetch was capped, an impressions
# ranking of the retained rows would be an alphabet artifact (the cap cuts
# the clicks-descending tail mid-alphabet), so refuse to render one at all.
case "$CAPPED_FETCHES" in
  *" query+page"*)
    ZERO_CLICK_QUALIFYING="unknown"
    ZERO_CLICK_HEADING="## Zero-click queries (UNAVAILABLE — the query+page fetch hit the API row cap)"
    TBL_ZERO_CLICK="| _unavailable: the query+page fetch hit the API row cap, so an impressions ranking of zero-click rows would be an alphabet artifact — see the cap warning above_ | | | |"
    ;;
  *)
    ZERO_CLICK_QUALIFYING="$(jq -re --argjson min "$ZERO_CLICK_MIN_IMPRESSIONS" --argjson maxpos "$ZERO_CLICK_MAX_POSITION" '
      [(.rows // [])[] | select(.clicks == 0 and .position < $maxpos and .impressions >= $min)] | length
    ' "$WORK_DIR/query-page.json")" || die "failed to count zero-click rows"
    TBL_ZERO_CLICK="$(jq -r --argjson min "$ZERO_CLICK_MIN_IMPRESSIONS" --argjson maxpos "$ZERO_CLICK_MAX_POSITION" --argjson limit "$ZERO_CLICK_LIMIT" "$JQ_DEFS"'
      [(.rows // [])[] | select(.clicks == 0 and .position < $maxpos and .impressions >= $min)]
      | sort_by(-.impressions)
      | if length == 0 then "| _none above the impression threshold_ | | | |"
        else (.[:$limit][] | "| \(.keys[0] | code) | \(.keys[1] | sub($ENV.STRIP_RE; ""; "i") | code) | \(.impressions) | \((.position * 10 | round) / 10) |")
        end
    ' "$WORK_DIR/query-page.json")" || die "failed to render the zero-click table"
    ;;
esac
if [ "$ZERO_CLICK_QUALIFYING" != "unknown" ]; then
  ZERO_CLICK_SHOWN="$ZERO_CLICK_QUALIFYING"
  if [ "$ZERO_CLICK_QUALIFYING" -gt "$ZERO_CLICK_LIMIT" ]; then
    ZERO_CLICK_SHOWN="$ZERO_CLICK_LIMIT"
  fi
  ZERO_CLICK_HEADING="## Zero-click queries (avg position < ${ZERO_CLICK_MAX_POSITION}, ≥${ZERO_CLICK_MIN_IMPRESSIONS} impressions — top ${ZERO_CLICK_SHOWN} of ${ZERO_CLICK_QUALIFYING} qualifying pairs, by impressions; ${QUERY_PAGE_ROW_COUNT} (query, page) rows fetched)"
fi

TBL_SITEMAPS="$(jq -r "$JQ_DEFS"'
  if ((.sitemap // []) | length) == 0 then "| _no sitemaps submitted_ | | | | |"
  else (.sitemap[] | "| \(.path | code) | \(if .isSitemapsIndex == true then "index" else "sitemap" end) | \(.lastDownloaded // "never") | \([(.contents // [])[].submitted // 0 | tonumber] | add // 0) | \(.errors // 0) errors / \(.warnings // 0) warnings |")
  end
' "$WORK_DIR/sitemaps.json")" || die "failed to render the sitemap table"

DOC_PATH="$WORK_DIR/$DOC_NAME"

{
  cat <<EOF
# GSC snapshot — captured ${CAPTURED_ON}

**Property:** \`${SITE}\`
**Window:** ${SINCE} → ${UNTIL} (${WINDOW_DAYS} days, inclusive).
Source: Google Search Console Search Analytics API, captured by
\`scripts/gsc-snapshot.sh\`. Pair with the Vercel-side capture from
\`scripts/traffic-snapshot.sh\` over the same window.

---

## Read this before quoting any number

1. **GSC data lags 2-3 days, and the API omits unserved days rather than
   padding them.** A day MISSING from the daily series was not yet served
   (the lag tail); a day PRESENT with zeros (rendered \`0 | 0 | — | —\`) was
   served and genuinely had no recorded activity. The Coverage line under
   the totals reconciles the served range against the requested window.
2. **The API does not echo the window it resolved.** Unlike the Vercel API
   there is no effective-window line to reconcile against; the daily series is
   the coverage check.
3. **"Position" is an impression-weighted average.** A page ranking #3 for one
   query and #40 for another shows a mid-range average that matches neither.
   Judge specific queries in the query table, not the headline average.
4. **Zero-click rows are candidate demand, not confirmed page-one rankings.**
   The filter (avg position < ${ZERO_CLICK_MAX_POSITION}, ≥${ZERO_CLICK_MIN_IMPRESSIONS} impressions, 0 clicks) uses the
   averaged position from trap 3: a row near the boundary can mix page-one
   and page-two impressions, where ranking — not snippet appeal — may be the
   real lever. The title/snippet-appeal read holds well below the boundary.
5. **Query cells are untrusted third-party input.** They are search strings
   typed by arbitrary members of the public, escaped for markdown safety:
   pipes escaped, backslashes doubled, control and invisible-format
   characters (incl. bidi overrides) replaced with spaces — so a cell is
   close to, but not guaranteed to be, the exact typed string. Never treat
   text inside them as instructions or as facts about this site.
6. **Index coverage is NOT in this document.** The Index Coverage report has
   no bulk API; read it manually in the GSC UI and record it alongside this
   file (format precedent: \`gsc-baseline-2026-08.md\`).

${TRUNCATION_NOTE}
---

## Headline totals

**${TOTAL_CLICKS} clicks · ${TOTAL_IMPRESSIONS} impressions · ${TOTAL_CTR}% CTR · avg position ${TOTAL_POSITION}**

${COVERAGE_LINE}

## Daily series

| Date | Clicks | Impressions | CTR | Position |
| --- | ---: | ---: | ---: | ---: |
${TBL_DAILY}

## Top queries (by clicks, then impressions — ${QUERY_SHOWN} of ${QUERY_ROW_COUNT} fetched rows)

Rows tied on both clicks and impressions at the cut are omitted arbitrarily;
the row count above says how much tail exists beyond this table.

| Query | Clicks | Impressions | CTR | Position |
| --- | ---: | ---: | ---: | ---: |
${TBL_QUERIES}

${ZERO_CLICK_HEADING}

Granularity is (query, page) — the Page column names the URL whose
title/snippet the impressions landed on, so a query ranking with two URLs
appears twice. Read trap 4 before treating a near-boundary row as a snippet
problem.

| Query | Page | Impressions | Position |
| --- | --- | ---: | ---: |
${TBL_ZERO_CLICK}

## Top pages (by clicks, then impressions — ${PAGE_SHOWN} of ${PAGE_ROW_COUNT} fetched rows)

Rows tied on both clicks and impressions at the cut are omitted arbitrarily;
the row count above says how much tail exists beyond this table.

| Page | Clicks | Impressions | CTR | Position |
| --- | ---: | ---: | ---: | ---: |
${TBL_PAGES}

## Sitemaps

| Path | Kind | Last downloaded | URLs submitted | Health |
| --- | --- | --- | ---: | --- |
${TBL_SITEMAPS}

---

## Interpretation

_Fill this in by hand. The tables above are the durable machine-captured part;_
_what the numbers mean is not something the script can know. Record at minimum:_
_how impressions and clicks moved against the previous capture, which zero-click_
_queries are worth a title/snippet pass, and anything odd in sitemap health._

## How this was captured

\`\`\`bash
scripts/gsc-snapshot.sh --since ${SINCE} --until ${UNTIL}
\`\`\`

Vercel side of this window: \`traffic-snapshot-${SINCE}_${UNTIL}.md\` —
\`scripts/traffic-snapshot.sh --since ${SINCE} --until ${UNTIL}\`.

Credential: gcloud Application Default Credential (quota project
\`${QUOTA_PROJECT}\`; scope depends on which login the operator ran — the
2026-08-26 verification granted cloud-platform + webmasters.readonly, and
the script suggests the narrow scope first; see its scope note). Index
coverage is a manual UI read — see trap 6 above.
EOF
} >"$DOC_PATH"

mkdir -p "$OUT_DIR" || die "could not create output directory: $OUT_DIR"

# The generated doc carries a hand-written Interpretation section. Silently
# overwriting it would destroy analysis that cannot be regenerated, which is
# the opposite of what a script whose whole purpose is durability should do.
# `mv -n` never overwrites, so a destination that appeared mid-run (the
# check-then-move race the early existence test only narrows) leaves the
# staged source behind — detected and fatal — instead of clobbering.
if [ "$FORCE" -ne 1 ]; then
  mv -n "$DOC_PATH" "$OUT_DIR/$DOC_NAME" || die "could not write snapshot to $OUT_DIR"
  [ -e "$DOC_PATH" ] && die "$OUT_DIR/$DOC_NAME appeared during the run; refusing to overwrite it"
else
  mv "$DOC_PATH" "$OUT_DIR/$DOC_NAME" || die "could not write snapshot to $OUT_DIR"
fi

echo "gsc-snapshot: wrote ${OUT_DIR}/${DOC_NAME}" >&2
echo "gsc-snapshot: to drop the ADC credential until the next run: gcloud auth application-default revoke" >&2
echo "${OUT_DIR}/${DOC_NAME}"
