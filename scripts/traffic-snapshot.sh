#!/usr/bin/env bash
#
# Capture a durable traffic snapshot from the Vercel Web Analytics REST API.
#
# WHY THIS EXISTS
# ---------------
# Vercel serves only a bounded trailing window of Web Analytics data —
# ~12 months on the current Pro plan, but the plan tier has changed under
# this project before, so re-probe before relying on the limit. Anything
# older than the window is gone: no export, no archive, no support path.
# A month that nobody captured within the window is a month that never
# happened. This script turns what used to be a hand-run curl sequence into
# one reproducible command so the capture actually gets done on schedule.
#
# Run it monthly, BEFORE the retention window rolls past the month you care
# about. The generated markdown (in gitignored docs/research/) is the durable
# record — and it lives ONLY on this machine: docs/ is deliberately
# gitignored and no off-machine copy exists as of 2026-08, so preservation
# beyond this machine is an open question, not a solved one.
#
# The monthly capture is a PAIR: run scripts/gsc-snapshot.sh over the same
# window for the Search Console side (impressions, queries, CTR, position) —
# Vercel sees only the clicks that became visits. Both scripts default their
# window end to today-3 (GSC data lags 2-3 days), so bare same-day paired
# runs cover the same window and produce name-linked files; explicit
# --since/--until on both is still preferred for reproducibility.
#
# Usage:
#   scripts/traffic-snapshot.sh [--out DIR] [--days N] [--since DATE] [--until DATE] [--force]
#
#   --out DIR      Directory to write the snapshot into. Default: docs/research
#                  relative to the repo root. NOTE: docs/ is gitignored and is
#                  absent from fresh worktrees, so pass --out explicitly when
#                  running anywhere other than the main checkout.
#   --days N       Length of the trailing window in days. Default: 28.
#   --since DATE   Explicit window start (YYYY-MM-DD). Overrides --days.
#   --until DATE   Explicit window end (YYYY-MM-DD), inclusive. Default: three
#                  days before today (UTC), matching gsc-snapshot.sh so bare
#                  paired runs cover the same window. (Vercel itself serves
#                  data through today; pass --until explicitly for a
#                  right-up-to-now unpaired read.)
#   --force        Overwrite an existing snapshot for this window (refused by
#                  default).
#
# Requires: bash, curl, jq, python3, and VERCEL_API_KEY in the environment
# (`source ~/.zshrc`).

set -euo pipefail

# --- Configuration ----------------------------------------------------------

# Overridable so the script can be pointed at another project without edits.
PROJECT_ID="${VERCEL_PROJECT_ID:-prj_5Nl7trgVEwUg9pqYL14IeOLjEXHl}"
TEAM_ID="${VERCEL_TEAM_ID:-team_TwamRjiOUoFPT99apHURROD4}"
API_BASE="${VERCEL_ANALYTICS_API:-https://api.vercel.com/v1/query/web-analytics}"

REFERRER_LIMIT=15
PAGE_LIMIT=30
SEGMENT_LIMIT=10

WINDOW_DAYS=28
SINCE=""
UNTIL=""
OUT_DIR=""
FORCE=0

# --- Helpers ----------------------------------------------------------------

die() {
  printf 'traffic-snapshot: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Capture a durable traffic snapshot from the Vercel Web Analytics REST API.

Usage:
  scripts/traffic-snapshot.sh [--out DIR] [--days N] [--since DATE] [--until DATE] [--force]

  --out DIR      Directory to write the snapshot into. Default: docs/research
                 relative to the repo root. docs/ is gitignored and absent from
                 fresh worktrees, so pass this explicitly outside the main
                 checkout.
  --days N       Length of the trailing window in days. Default: 28.
  --since DATE   Explicit window start (YYYY-MM-DD). Overrides --days.
  --until DATE   Explicit window end (YYYY-MM-DD), inclusive. Default: three
                 days before today (UTC), matching gsc-snapshot.sh so bare
                 paired runs cover the same window.
  --force        Overwrite an existing snapshot for this window. Refused by
                 default, because the generated doc carries hand-written
                 analysis.

Requires VERCEL_API_KEY in the environment (source ~/.zshrc), plus curl, jq,
and python3.
USAGE
}

# The final stdout line is this script's machine-readable contract (the path it
# wrote), so usage text on the error path must go to stderr or a caller doing
# `path="$(traffic-snapshot.sh --typo)"` captures the whole help block as a path.
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
  # pass validation and then flow verbatim into the query string and the doc's
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
    *) printf 'traffic-snapshot: unknown argument %s\n\n' "$1" >&2; usage_and_exit 1 ;;
  esac
done

command -v curl    >/dev/null 2>&1 || die "curl is required"
command -v jq      >/dev/null 2>&1 || die "jq is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

[ -n "${VERCEL_API_KEY:-}" ] || die "VERCEL_API_KEY is not set (try: source ~/.zshrc)"

# Pin the API base to Vercel's host over HTTPS: the bearer token is attached
# to every request, so a stray override (typo, inherited env) would otherwise
# ship the credential in cleartext or to an arbitrary host — silently, since
# the script would just report an HTTP error.
case "$API_BASE" in
  https://api.vercel.com/*) ;;
  *) die "VERCEL_ANALYTICS_API must be an https://api.vercel.com endpoint (got: ${API_BASE})" ;;
esac

case "$WINDOW_DAYS" in
  ''|*[!0-9]*) die "--days must be a positive integer, got '$WINDOW_DAYS'" ;;
esac
# Normalize before any $(( )) sees it: a leading zero ("08") would otherwise
# be read as octal — an arithmetic error, or a silently smaller window.
WINDOW_DAYS=$((10#$WINDOW_DAYS))
[ "$WINDOW_DAYS" -gt 0 ] || die "--days must be greater than zero"

# Default matches gsc-snapshot.sh (GSC data lags 2-3 days), so bare same-day
# paired runs land on the same window and the same filename stem. Vercel
# itself serves data through today; override --until for an unpaired read.
[ -n "$UNTIL" ] || UNTIL="$(shift_date "$(date -u +%Y-%m-%d)" -3)"
require_iso_date "$UNTIL" "--until"

if [ -n "$SINCE" ]; then
  require_iso_date "$SINCE" "--since"
else
  SINCE="$(shift_date "$UNTIL" "-$((WINDOW_DAYS - 1))")"
fi

# Recompute the span from the resolved dates rather than trusting --days, which
# is meaningless once --since is given explicitly. The daily-series limit and
# the window label in the generated doc both depend on this being the real span.
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

# Vercel resolves `until` differently depending on the grouping dimension.
# Verified 2026-08-18 against the live API:
#   - visits/count and time groupings (by=day) read `until=D` as the END of day
#     D, echoing an effective end of D+1T00:00.
#   - Non-time groupings (by=requestPath, by=referrerHostname, by=country, ...)
#     read `until=D` as D at 01:00, which drops nearly all of day D.
# Left uncorrected, the breakdown sections silently cover one day less than the
# headline totals and the sections do not reconcile. Sending the day AFTER the
# window end to the non-time groupings realigns them, at the cost of a one-hour
# overhang past midnight. That trade is recorded in the generated doc.
DIM_UNTIL="$(shift_date "$UNTIL" 1)"

CAPTURED_ON="$(date -u +%Y-%m-%d)"
# Keyed on the WINDOW, not the capture date, matching gsc-snapshot.sh: the
# paired artifacts link by name, several same-day runs over different windows
# (e.g. a monthly 28-day plus a quarterly 90-day) cannot collide, and --force
# can no longer destroy a DIFFERENT window's hand-annotated snapshot. Files
# from before this change keep their capture-date names; they are historical.
DOC_NAME="traffic-snapshot-${SINCE}_${UNTIL}.md"

# Early check so a mistaken re-run fails immediately instead of after nine
# API calls; the publication step's `mv -n` is what actually closes the
# overwrite race.
if [ -e "$OUT_DIR/$DOC_NAME" ] && [ "$FORCE" -ne 1 ]; then
  die "$OUT_DIR/$DOC_NAME already exists; move it or pass --force to overwrite"
fi

# --- API access -------------------------------------------------------------

# Everything is staged in a scratch directory and moved into place only after
# every request has succeeded, so a mid-run API failure can never leave a
# half-written snapshot that later reads as a complete record.
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/traffic-snapshot.XXXXXX")"
cleanup() { rm -rf "$WORK_DIR"; }
trap cleanup EXIT

# api_get <outfile> <endpoint> <query-string> [odata-filter]
#
# Fails the whole script on any transport error, any non-200 status, or any
# response carrying an `error` object. Vercel returns 200-with-error-body in
# some cases, so the status code alone is not a sufficient check.
api_get() {
  local out_file endpoint query filter url status message
  out_file="$1"
  endpoint="$2"
  query="$3"
  filter="${4:-}"

  url="${API_BASE}/${endpoint}?teamId=${TEAM_ID}&projectId=${PROJECT_ID}&${query}"

  set -- -sS --max-time 60 -o "$out_file" -w '%{http_code}' \
    --proto '=https' --proto-redir '=https' -K -
  if [ -n "$filter" ]; then
    set -- "$@" -G --data-urlencode "filter=${filter}"
  fi

  # The credential goes in over stdin as a curl config rather than as a -H
  # argument, which would expose it in `ps` output to every process on the box
  # for the life of the request.
  if ! status="$(printf 'header = "Authorization: Bearer %s"\n' "$VERCEL_API_KEY" \
      | curl "$@" "$url")"; then
    die "request to ${endpoint} failed (curl transport error)"
  fi

  if [ "$status" != "200" ]; then
    message="$(jq -r '.error.message // "no error message in body"' "$out_file" 2>/dev/null \
      || echo "unparseable response body")"
    die "${endpoint} returned HTTP ${status}: ${message}"
  fi

  if ! jq -e 'type == "object"' "$out_file" >/dev/null 2>&1; then
    die "${endpoint} returned a non-JSON response"
  fi

  if jq -e 'has("error")' "$out_file" >/dev/null 2>&1; then
    message="$(jq -r '.error.message // "unspecified error"' "$out_file")"
    die "${endpoint} returned an error payload: ${message}"
  fi
}

# Report the window the API resolved the request to, which is not always the one
# asked for: this echo is what exposed the per-dimension `until` quirk handled
# above. It is the resolved REQUEST, not proof of what was served, so it cannot
# confirm the data is complete.
#
# The explicit null check is load-bearing. String interpolation happily renders a
# missing `.query` as the literal "null → null" and exits 0, so without it every
# caller's `|| die` guard would be unreachable and a schema change on Vercel's
# side would durably record "null → null" in all six sections of a snapshot the
# script reported as successful.
effective_window() {
  jq -re '
    if (.query.since == null) or (.query.until == null)
    then error("response is missing query.since/query.until")
    else "\(.query.since) → \(.query.until)"
    end
  ' "$1"
}

TIME_QUERY="since=${SINCE}&until=${UNTIL}"
DIM_QUERY="since=${SINCE}&until=${DIM_UNTIL}"
GOOGLE_FILTER="referrerHostname eq 'google.com'"

# Series length is bounded by the window, plus slack for the exclusive end.
DAY_LIMIT=$((WINDOW_DAYS + 2))

echo "traffic-snapshot: querying ${SINCE} → ${UNTIL} ..." >&2

api_get "$WORK_DIR/count.json"      "visits/count"     "$TIME_QUERY"
api_get "$WORK_DIR/referrers.json"  "visits/aggregate" "${DIM_QUERY}&by=referrerHostname&limit=${REFERRER_LIMIT}"
api_get "$WORK_DIR/pages.json"      "visits/aggregate" "${DIM_QUERY}&by=requestPath&limit=${PAGE_LIMIT}"
api_get "$WORK_DIR/daily.json"      "visits/aggregate" "${TIME_QUERY}&by=day&limit=${DAY_LIMIT}"
api_get "$WORK_DIR/google-daily.json" "visits/aggregate" "${TIME_QUERY}&by=day&limit=${DAY_LIMIT}" "$GOOGLE_FILTER"
api_get "$WORK_DIR/google-pages.json" "visits/aggregate" "${DIM_QUERY}&by=requestPath&limit=${PAGE_LIMIT}" "$GOOGLE_FILTER"

# Automation segments. The interpretation traps below tell the reader to check
# browser, OS, and country mix for crawler signatures; capturing them here means
# that check stays possible after the retention window has rolled past.
api_get "$WORK_DIR/browsers.json" "visits/aggregate" "${DIM_QUERY}&by=browserName&limit=${SEGMENT_LIMIT}"
api_get "$WORK_DIR/os.json"       "visits/aggregate" "${DIM_QUERY}&by=osName&limit=${SEGMENT_LIMIT}"
api_get "$WORK_DIR/countries.json" "visits/aggregate" "${DIM_QUERY}&by=country&limit=${SEGMENT_LIMIT}"

# --- Rendering --------------------------------------------------------------

# Every rendered fragment is computed into a variable BEFORE the document is
# assembled, each guarded by `|| die`. Doing this inline in the heredoc instead
# would silently swallow failures: `set -e` does not propagate out of a command
# substitution, so a broken jq filter would emit an empty table and still exit
# zero, producing a partial snapshot that reads as a complete record.

# Render `.data` as a markdown table keyed on <dimension>. `label` is a jq
# reserved word, hence `pretty`.
render_dimension_table() {
  jq -r --arg dim "$2" '
    def pretty($v):
      if $v == null then "*(unknown)*"
      elif $v == "" then "*(direct)*"
      elif $v == "Others" then "*(others, aggregated by Vercel)*"
      else "`" + $v + "`" end;
    if (.data | length) == 0 then "| _no data in this window_ | | |"
    else
      (.data[] | "| \(pretty(.[$dim])) | \(.visitors) | \(.pageviews) |")
    end
  ' "$1"
}

render_day_table() {
  jq -r '
    if (.data | length) == 0 then "| _no data in this window_ | | |"
    else
      (.data[] | "| \(.timestamp | split("T")[0]) | \(.visitors) | \(.pageviews) |")
    end
  ' "$1"
}

VISITORS="$(jq -re '.data.visitors' "$WORK_DIR/count.json")" \
  || die "could not read visitor total from the API response"
PAGEVIEWS="$(jq -re '.data.pageviews' "$WORK_DIR/count.json")" \
  || die "could not read pageview total from the API response"
GOOGLE_VISITORS="$(jq -re '[.data[].visitors] | add // 0' "$WORK_DIR/google-daily.json")" \
  || die "could not read the google.com-referred visitor series"

WIN_COUNT="$(effective_window "$WORK_DIR/count.json")"      || die "unreadable window: count"
WIN_REFERRERS="$(effective_window "$WORK_DIR/referrers.json")" || die "unreadable window: referrers"
WIN_PAGES="$(effective_window "$WORK_DIR/pages.json")"      || die "unreadable window: pages"
WIN_DAILY="$(effective_window "$WORK_DIR/daily.json")"      || die "unreadable window: daily"
WIN_GDAILY="$(effective_window "$WORK_DIR/google-daily.json")" || die "unreadable window: google daily"
WIN_GPAGES="$(effective_window "$WORK_DIR/google-pages.json")" || die "unreadable window: google pages"

TBL_REFERRERS="$(render_dimension_table "$WORK_DIR/referrers.json" referrerHostname)" \
  || die "failed to render the referrer table"
TBL_PAGES="$(render_dimension_table "$WORK_DIR/pages.json" requestPath)" \
  || die "failed to render the top-pages table"
TBL_DAILY="$(render_day_table "$WORK_DIR/daily.json")" \
  || die "failed to render the daily series"
TBL_GDAILY="$(render_day_table "$WORK_DIR/google-daily.json")" \
  || die "failed to render the google.com daily series"
TBL_GPAGES="$(render_dimension_table "$WORK_DIR/google-pages.json" requestPath)" \
  || die "failed to render the google.com landing-page table"
TBL_BROWSERS="$(render_dimension_table "$WORK_DIR/browsers.json" browserName)" \
  || die "failed to render the browser table"
TBL_OS="$(render_dimension_table "$WORK_DIR/os.json" osName)" \
  || die "failed to render the OS table"
TBL_COUNTRIES="$(render_dimension_table "$WORK_DIR/countries.json" country)" \
  || die "failed to render the country table"

DOC_PATH="$WORK_DIR/$DOC_NAME"

{
  cat <<EOF
# Traffic snapshot — captured ${CAPTURED_ON}

**Window:** ${SINCE} → ${UNTIL} (${WINDOW_DAYS} days, inclusive).
Source: Vercel Web Analytics REST API, captured by \`scripts/traffic-snapshot.sh\`.
The default window ends three days before capture to align with the paired
GSC capture's data lag — Vercel itself serves data through the capture date,
so a gap before "captured" above is deliberate, not a failed run (pass an
explicit \`--until\` for an unpaired right-up-to-now read).

Vercel serves only a bounded trailing window of analytics data, so this file is
the durable record. Re-run the script monthly, before the window rolls past the
month you want to keep.

---

## Read this before quoting any number

1. **"Visitors" dedupe per DAY, not per window.** One person visiting daily
   contributes up to one "visitor" per day to the window total. No visitor
   count here is a count of unique humans, and the longer the window, the more
   inflated it is.
2. **Check the automation segments before trusting anything.** The browser and
   OS tables below exist for this. A single browser or OS row holding a large
   share of pageviews against a small visitor count is a crawler, not an
   audience.
3. **Country segments at exactly 1.0 pageviews per visitor are crawlers.**
   Datacenter traffic that executes JS registers as a visitor, loads one page,
   and leaves. An exact 1.0 ratio across dozens of visitors is a signature, not
   a coincidence.
4. **A stealth crawler with an Android / Chrome Mobile user agent has been
   active since 2026-08.** It runs real Chrome with \`navigator.webdriver\`
   false and no referrer, sweeping long-tail entity pages around the clock, so
   Vercel's bot suppression does not catch it. Expect it to dominate raw
   pageviews. Subtract it before quoting growth.
5. **A client-side pageview cap (PSY-1863, PR #1917) suppresses events past
   150 per browser per UTC day.** Where it is live, no browser can contribute
   more than 150 pageviews in a day, so per-visit fingerprints above that
   ceiling cannot appear and heavy-crawler pageview totals are floored rather
   than measured. It had NOT shipped to production when this script was written
   (2026-08-18), so check whether it deployed during this window before
   comparing against a snapshot from the other side of that change.
6. **Sections do not all cover an identical window.** Vercel reads \`until\`
   as end-of-day for totals and daily series, but as 01:00 on the \`until\` day
   for breakdowns by page, referrer, browser, OS, and country. This script
   compensates by sending the following day to the breakdown queries, which
   leaves them with a one-hour overhang past the window end. Each section
   records the window the API actually used; trust that line over the header
   when reconciling.

---

## Headline totals

**${VISITORS} visitors · ${PAGEVIEWS} pageviews** (raw, uncorrected).

Effective window: \`${WIN_COUNT}\`

## By referrer

Effective window: \`${WIN_REFERRERS}\`

| Referrer | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_REFERRERS}

## Top pages

**This table is the least complete thing in this document.** Vercel caps
\`limit\` at 100 and folds everything else into one "Others" row, and the
crawler above sweeps thousands of distinct long-tail paths, so the named rows
routinely account for under 1% of window pageviews. Read this as "which pages
rank", never as a distribution of traffic, and do not compute shares from it.

Effective window: \`${WIN_PAGES}\`

| Path | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_PAGES}

## Daily series

Effective window: \`${WIN_DAILY}\`

| Date | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_DAILY}

## Google organic

**${GOOGLE_VISITORS} google.com-referred visitors**, summed from the daily
series below. The \`google.com\` row in the referrer table above is queried over
the slightly wider breakdown window, so the two can differ by any referral
landing in that one-hour overhang. Cite this figure with the daily series, not
against the referrer table.

Filter: \`${GOOGLE_FILTER}\`

### Daily

Effective window: \`${WIN_GDAILY}\`

| Date | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_GDAILY}

### Landing pages

Effective window: \`${WIN_GPAGES}\`

| Path | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_GPAGES}

## Automation segments

Captured so trap 2 and trap 3 above stay checkable after the window expires.

### By browser

| Browser | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_BROWSERS}

### By OS

| OS | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_OS}

### By country

| Country | Visitors | Pageviews |
| --- | ---: | ---: |
${TBL_COUNTRIES}

---

## Interpretation

_Fill this in by hand. The tables above are the durable machine-captured part;_
_what the numbers mean is not something the script can know. Record at minimum:_
_which segments were excluded as bots and why, which channels grew, and what_
_changed since the previous snapshot._

## How this was captured

\`\`\`bash
source ~/.zshrc   # provides VERCEL_API_KEY
scripts/traffic-snapshot.sh --since ${SINCE} --until ${UNTIL}
\`\`\`

Google Search Console data (impressions, queries, CTR, position) is NOT in
this file: capture it with
\`scripts/gsc-snapshot.sh --since ${SINCE} --until ${UNTIL}\`, which writes
\`gsc-snapshot-${SINCE}_${UNTIL}.md\` alongside this file. Both scripts
default their window end to today-3 (GSC data lag), so bare same-day paired
runs align; explicit dates are still preferred for reproducibility.
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

echo "traffic-snapshot: wrote ${OUT_DIR}/${DOC_NAME}" >&2
echo "${OUT_DIR}/${DOC_NAME}"
