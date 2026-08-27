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
# the same window. Unlike Vercel's rolling retention window, the Search
# Analytics API serves roughly 16 months of history, so a missed month is
# recoverable here — the pairing discipline exists for comparability, not
# data-loss urgency.
#
# PAIRED RUNS MUST PASS EXPLICIT --since/--until TO BOTH SCRIPTS. The two
# DEFAULT windows differ — traffic-snapshot.sh ends at today, this script at
# today-2 (GSC data lag) — so bare paired runs silently cover windows offset
# by two days at both ends, and each doc's header looks self-consistent.
#
# OUT OF SCOPE: the Index Coverage report (indexed / not-indexed counts by
# reason) has no bulk public API — only the per-URL, quota-limited URL
# Inspection API. Index coverage stays a manual read in the GSC UI; the
# 2026-08-17 manual capture in docs/research/gsc-baseline-2026-08.md shows
# what to record.
#
# Usage:
#   scripts/gsc-snapshot.sh [--out DIR] [--days N] [--since DATE] [--until DATE]
#
#   --out DIR      Directory to write the snapshot into. Default: docs/research
#                  relative to the repo root. NOTE: docs/ is gitignored and is
#                  absent from fresh worktrees, so pass --out explicitly when
#                  running anywhere other than the main checkout.
#   --days N       Length of the trailing window in days. Default: 28.
#   --since DATE   Explicit window start (YYYY-MM-DD). Overrides --days.
#   --until DATE   Explicit window end (YYYY-MM-DD), inclusive. Default: two
#                  days before today (UTC) — GSC data lags 1-2 days, so ending
#                  at today would silently render empty tail rows.
#   --force        Overwrite an existing snapshot for today (refused by default).
#
# Requires: bash, curl, jq, python3, and the gcloud CLI holding an Application
# Default Credential with the webmasters.readonly scope (see the auth error
# message below for the exact login command).

set -euo pipefail

# --- Configuration ----------------------------------------------------------

# Overridable so the script can be pointed at another property without edits.
SITE="${GSC_SITE:-sc-domain:psychichomily.com}"
# ADC user credentials must name a quota project or the API 403s with
# SERVICE_DISABLED for the OAuth client's own project (verified 2026-08-26).
QUOTA_PROJECT="${GSC_QUOTA_PROJECT:-psychic-homily}"
API_BASE="${GSC_API:-https://searchconsole.googleapis.com/webmasters/v3}"

# One fetch feeds both the top-queries table and the zero-click derivation.
# The API sorts rows by clicks DESCENDING, so zero-click rows all sit at the
# tail — a small fetch truncates them alphabetically (an accident of the
# tie-break order), not by impressions. 25000 is the API's per-request max;
# fetch everything so zero-click membership is complete.
QUERY_FETCH_LIMIT=25000
QUERY_TABLE_LIMIT=25
PAGE_LIMIT=25
ZERO_CLICK_LIMIT=20
ZERO_CLICK_MIN_IMPRESSIONS=5

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
  scripts/gsc-snapshot.sh [--out DIR] [--days N] [--since DATE] [--until DATE]

  --out DIR      Directory to write the snapshot into. Default: docs/research
                 relative to the repo root. docs/ is gitignored and absent from
                 fresh worktrees, so pass this explicitly outside the main
                 checkout.
  --days N       Length of the trailing window in days. Default: 28.
  --since DATE   Explicit window start (YYYY-MM-DD). Overrides --days.
  --until DATE   Explicit window end (YYYY-MM-DD), inclusive. Default: two days
                 before today (UTC), because GSC data lags 1-2 days. NOTE this
                 differs from traffic-snapshot.sh's default (today): paired
                 runs must pass explicit --since/--until to both scripts.
  --force        Overwrite an existing snapshot for this window. Refused by
                 default, because the generated doc carries hand-written
                 analysis.

Requires curl, jq, python3, and a gcloud Application Default Credential with
the webmasters.readonly scope.
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

case "$WINDOW_DAYS" in
  ''|*[!0-9]*) die "--days must be a positive integer, got '$WINDOW_DAYS'" ;;
esac
[ "$WINDOW_DAYS" -gt 0 ] || die "--days must be greater than zero"

# GSC data lags 1-2 days behind real time; default the window end to two days
# ago so the tail of the daily series is real data, not silent zeros.
if [ -z "$UNTIL" ]; then
  UNTIL="$(shift_date "$(date -u +%Y-%m-%d)" -2)"
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

# Checked twice on purpose. Here so a mistaken re-run fails immediately instead
# of after six API calls, and again just before the move, which is the only
# check that actually closes the race.
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
if ! TOKEN="$("$GCLOUD" auth application-default print-access-token 2>"$WORK_DIR/token-err")"; then
  printf 'gsc-snapshot: could not mint an access token from the Application Default Credential.\n' >&2
  sed 's/^/gsc-snapshot:   /' "$WORK_DIR/token-err" >&2
  printf 'gsc-snapshot: (re)authenticate with:\n' >&2
  printf 'gsc-snapshot:   gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/webmasters.readonly\n' >&2
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
      printf 'gsc-snapshot: if the credential expired, re-authenticate with:\n' >&2
      printf 'gsc-snapshot:   gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/webmasters.readonly\n' >&2
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

DAY_LIMIT=$((WINDOW_DAYS + 2))

echo "gsc-snapshot: querying ${SINCE} → ${UNTIL} for ${SITE} ..." >&2

search_query "$WORK_DIR/totals.json"  ''                   1
search_query "$WORK_DIR/daily.json"   '["date"]'           "$DAY_LIMIT"
search_query "$WORK_DIR/queries.json" '["query"]'          "$QUERY_FETCH_LIMIT"
search_query "$WORK_DIR/pages.json"   '["page"]'           "$PAGE_LIMIT"
api_call     "$WORK_DIR/sitemaps.json" GET "sites/${SITE_ENC}/sitemaps"

# --- Rendering --------------------------------------------------------------

# Every rendered fragment is computed into a variable BEFORE the document is
# assembled, each guarded by `|| die`. Doing this inline in the heredoc instead
# would silently swallow failures: `set -e` does not propagate out of a command
# substitution, so a broken jq filter would emit an empty table and still exit
# zero, producing a partial snapshot that reads as a complete record.

# Cell hygiene, shared by every renderer below via JQ_DEFS: queries are
# arbitrary user-typed strings, so a literal `|` would split the markdown row
# (shifting every metric one column left in the durable record) and a literal
# backtick would break the code span. Pipes are escaped; a cell containing a
# backtick is rendered as plain text instead of a code span, preserving the
# string exactly rather than mangling it.
#
# metrics(): clicks, impressions, CTR, position — the four columns of every
# metric table. CTR arrives as a 0-1 fraction — multiply out explicitly (a
# formatting slip here already produced nonsense CTRs once during the
# 2026-08-26 credential verification). ONE definition on purpose: parallel
# copies of this formula would drift on the next format fix.
JQ_DEFS='
  def esc: tostring | gsub("\\|"; "\\|");
  def code: esc | if test("`") then . else "`" + . + "`" end;
  def metrics(r): "\(r.clicks) | \(r.impressions) | \((r.ctr * 10000 | round) / 100)% | \((r.position * 10 | round) / 10)";
'

render_metric_table() {
  # $1 = file, $2 = jq expression mapping a row to its display cell,
  # $3 = optional row cap (omit or 0 = all rows)
  jq -r --argjson limit "${3:-0}" "$JQ_DEFS"'
    (.rows // []) | (if $limit > 0 then .[:$limit] else . end)
    | if length == 0 then "| _no data in this window_ | | | | |"
      else (.[] | "| " + ('"$2"') + " | " + metrics(.) + " |")
      end
  ' "$1"
}

TOTAL_CLICKS="$(jq -re '(.rows // []) | (.[0].clicks // 0)' "$WORK_DIR/totals.json")" \
  || die "could not read the clicks total from the API response"
TOTAL_IMPRESSIONS="$(jq -re '(.rows // []) | (.[0].impressions // 0)' "$WORK_DIR/totals.json")" \
  || die "could not read the impressions total from the API response"
TOTAL_CTR="$(jq -re '(.rows // []) | (((.[0].ctr // 0) * 10000 | round) / 100)' "$WORK_DIR/totals.json")" \
  || die "could not read the CTR total from the API response"
TOTAL_POSITION="$(jq -re '(.rows // []) | (((.[0].position // 0) * 10 | round) / 10)' "$WORK_DIR/totals.json")" \
  || die "could not read the position total from the API response"

# The Search Analytics API does not echo the window it resolved (unlike the
# Vercel API), so the honest coverage signal is the last date that actually
# returned data. A LAST_DATA_DAY short of the requested end means the lag
# window was longer than the 2-day default assumption. Zero-filled rows
# (0 clicks / 0 impressions / position 0 — an unattainable rank) are
# placeholders the API emits for empty days, not data; counting them would
# make a zero-filled lag tail read as full coverage.
LAST_DATA_DAY="$(jq -re '
  (.rows // []) | map(select(.clicks > 0 or .impressions > 0) | .keys[0]) | max // "none"
' "$WORK_DIR/daily.json")" \
  || die "could not read the daily series"

# Page URLs come back absolute; strip the property's own origin for
# readability but leave foreign origins intact so they stay visible. The
# pattern is derived from $SITE (not hardcoded) so the GSC_SITE override keeps
# working: an sc-domain property covers every subdomain and both schemes,
# hence the permissive host prefix; a url-prefix property strips itself only.
if [ "${SITE#sc-domain:}" != "$SITE" ]; then
  ESCAPED="$(printf '%s' "${SITE#sc-domain:}" | sed 's/[][\.*^$()+?{|]/\\&/g')"
  STRIP_RE="^https?://([a-z0-9-]+\\.)*${ESCAPED}"
else
  ESCAPED="$(printf '%s' "$SITE" | sed 's/[][\.*^$()+?{|]/\\&/g')"
  STRIP_RE="^${ESCAPED}"
fi
export STRIP_RE

TBL_DAILY="$(render_metric_table "$WORK_DIR/daily.json" '(.keys[0] | esc)')" \
  || die "failed to render the daily series"

TBL_QUERIES="$(render_metric_table "$WORK_DIR/queries.json" '(.keys[0] | code)' "$QUERY_TABLE_LIMIT")" \
  || die "failed to render the top-queries table"

TBL_PAGES="$(render_metric_table "$WORK_DIR/pages.json" '(.keys[0] | sub($ENV.STRIP_RE; ""; "i") | code)' "$PAGE_LIMIT")" \
  || die "failed to render the top-pages table"

# Zero-click page-one demand: queries we rank on page one for (position < 11)
# that produced impressions but no clicks. Derived from the same fetch as the
# table above (complete because QUERY_FETCH_LIMIT is the API max); at these
# positions the lever is title/snippet appeal, not rank.
TBL_ZERO_CLICK="$(jq -r --argjson min "$ZERO_CLICK_MIN_IMPRESSIONS" --argjson limit "$ZERO_CLICK_LIMIT" "$JQ_DEFS"'
  [(.rows // [])[] | select(.clicks == 0 and .position < 11 and .impressions >= $min)]
  | sort_by(-.impressions)
  | if length == 0 then "| _none above the impression threshold_ | | |"
    else (.[:$limit][] | "| \(.keys[0] | code) | \(.impressions) | \((.position * 10 | round) / 10) |")
    end
' "$WORK_DIR/queries.json")" || die "failed to render the zero-click table"

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

1. **GSC data lags 1-2 days.** The script defaults the window end to two days
   before capture for this reason. Last date with data in this capture:
   **${LAST_DATA_DAY}** — if that is short of the window end, the tail days
   were not yet served and this snapshot under-counts them.
2. **The API does not echo the window it resolved.** Unlike the Vercel API
   there is no effective-window line to reconcile against; the daily series is
   the coverage check.
3. **"Position" is an impression-weighted average.** A page ranking #3 for one
   query and #40 for another shows a mid-range average that matches neither.
   Judge specific queries in the query table, not the headline average.
4. **Zero-click rows are the actionable demand.** Page-one rankings with
   impressions and no clicks (threshold: ≥${ZERO_CLICK_MIN_IMPRESSIONS} impressions) are where
   title/snippet appeal — not ranking — is the lever.
5. **Index coverage is NOT in this document.** The Index Coverage report has
   no bulk API; read it manually in the GSC UI and record it alongside this
   file (format precedent: \`gsc-baseline-2026-08.md\`).

---

## Headline totals

**${TOTAL_CLICKS} clicks · ${TOTAL_IMPRESSIONS} impressions · ${TOTAL_CTR}% CTR · avg position ${TOTAL_POSITION}**

## Daily series

| Date | Clicks | Impressions | CTR | Position |
| --- | ---: | ---: | ---: | ---: |
${TBL_DAILY}

## Top queries (by clicks, top ${QUERY_TABLE_LIMIT})

| Query | Clicks | Impressions | CTR | Position |
| --- | ---: | ---: | ---: | ---: |
${TBL_QUERIES}

## Zero-click page-one queries (≥${ZERO_CLICK_MIN_IMPRESSIONS} impressions, position < 11, top ${ZERO_CLICK_LIMIT} by impressions)

| Query | Impressions | Position |
| --- | ---: | ---: |
${TBL_ZERO_CLICK}

## Top pages (top ${PAGE_LIMIT})

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

Credential: gcloud Application Default Credential (webmasters.readonly scope,
quota project \`${QUOTA_PROJECT}\`). Index coverage is a manual UI read — see
trap 5 above.
EOF
} >"$DOC_PATH"

mkdir -p "$OUT_DIR" || die "could not create output directory: $OUT_DIR"

# The generated doc carries a hand-written Interpretation section. Silently
# overwriting it would destroy analysis that cannot be regenerated, which is the
# opposite of what a script whose whole purpose is durability should do.
if [ -e "$OUT_DIR/$DOC_NAME" ] && [ "$FORCE" -ne 1 ]; then
  die "$OUT_DIR/$DOC_NAME already exists; move it or pass --force to overwrite"
fi

mv "$DOC_PATH" "$OUT_DIR/$DOC_NAME" || die "could not write snapshot to $OUT_DIR"

echo "gsc-snapshot: wrote ${OUT_DIR}/${DOC_NAME}" >&2
echo "${OUT_DIR}/${DOC_NAME}"
