#!/usr/bin/env bash
# scripts/gsc-snapshot_test.sh
#
# Self-test for scripts/gsc-snapshot.sh. Puts a stub `gcloud` (mints a fake
# token) and a stub `curl` (answers Search Analytics requests from fixture
# files and logs each request body) first on PATH, runs the real script, and
# asserts on the generated doc and the logged requests: the complete-week
# list, the page-family classifier including the slug-date split and the
# label tie-break, startRow pagination past the 25000-row request cap, the
# short-week flag, the per-week query-row split (its dated-show and 1.5
# position boundaries, a week with no qualifying row, and no query cell in the
# doc), traps 6 to 9, the Page indexing slot, and the request bodies of the
# window-wide fetches.
#
# Runs locally:
#   bash scripts/gsc-snapshot_test.sh
#
# It touches nothing outside its own mktemp -d and needs no network and no
# Google credential.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/gsc-snapshot.sh"

FIXTURE="$(mktemp -d)" || exit 1
[ -d "$FIXTURE" ] || exit 1
trap 'rm -rf "$FIXTURE"' EXIT

STUB_BIN="$FIXTURE/bin"
STUB_FIXTURES="$FIXTURE/responses"
STUB_LOG="$FIXTURE/requests.log"
mkdir -p "$STUB_BIN" "$STUB_FIXTURES"

FAILURES=0
CASE=""

cat >"$STUB_BIN/gcloud" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  "auth application-default print-access-token") echo stub-token ;;
  *) echo "stub gcloud: unexpected call: $*" >&2; exit 9 ;;
esac
STUB

# The stub curl writes the response body to the -o file and prints the status
# the script reads through -w. A POST body is answered by its dimensions:
#   ["date"]          one row per day of the range, ending at STUB_LAST_SERVED
#                     when set
#   ["page"]          responses/pages_<start>_<end>_<startRow>.json when
#                     present, else responses/generate_<start>_<end>_<startRow>
#                     holding a row count of synthetic one-impression
#                     /artists/ rows at position 5, else no rows
#   ["query","page"]  the same, from responses/query-pages_<stem>.json or
#                     responses/generate-query-pages_<stem>
#   other             no rows
# A GET (the sitemap listing) is answered with no sitemaps.
cat >"$STUB_BIN/curl" <<'STUB'
#!/usr/bin/env python3
import datetime, json, os, sys

args = sys.argv[1:]
out_file = body = None
for flag, value in zip(args, args[1:]):
    if flag == "-o":
        out_file = value
    elif flag == "--data":
        body = value
sys.stdin.read()
with open(os.environ["STUB_LOG"], "a") as log:
    log.write((body if body is not None else "GET " + args[-1]) + "\n")

responses = os.environ["STUB_FIXTURES"]
response = {}
if body is None:
    response = {"sitemap": []}
else:
    request = json.loads(body)
    dims = request.get("dimensions")
    start, end = request["startDate"], request["endDate"]
    start_row = request.get("startRow", 0)
    if dims == ["date"]:
        last = min(end, os.environ.get("STUB_LAST_SERVED") or end)
        day = datetime.date.fromisoformat(start)
        rows = []
        while str(day) <= last:
            rows.append({"keys": [str(day)], "clicks": 1, "impressions": 10, "ctr": 0.1, "position": 5})
            day += datetime.timedelta(days=1)
        response = {"rows": rows}
    elif dims in (["page"], ["query", "page"]):
        stem = f"{start}_{end}_{start_row}"
        listed_prefix, generated_prefix = ("pages", "generate") if dims == ["page"] else ("query-pages", "generate-query-pages")
        listed = os.path.join(responses, f"{listed_prefix}_{stem}.json")
        generated = os.path.join(responses, f"{generated_prefix}_{stem}")
        if os.path.exists(listed):
            with open(listed) as f:
                response = json.load(f)
        elif os.path.exists(generated):
            with open(generated) as f:
                count = int(f.read())
            response = {"rows": [
                {"keys": ([f"generated query {start_row + i}"] if dims == ["query", "page"] else [])
                         + [f"https://psychichomily.com/artists/generated-{start_row + i}"],
                 "clicks": 0, "impressions": 1, "ctr": 0, "position": 5}
                for i in range(count)]}
with open(out_file, "w") as f:
    json.dump(response, f)
sys.stdout.write("200")
STUB
chmod +x "$STUB_BIN/gcloud" "$STUB_BIN/curl"
export PATH="$STUB_BIN:$PATH"
export STUB_FIXTURES STUB_LOG

# Week of 2026-09-28: one URL per classifier rule. Families differ in
# impressions except /collections/ and /labels/ (14 each), which the label
# breaks.
cat >"$STUB_FIXTURES/pages_2026-09-28_2026-10-04_0.json" <<'JSON'
{"rows": [
  {"keys": ["https://psychichomily.com/shows/2026-09-27-day-before"], "clicks": 0, "impressions": 2, "ctr": 0, "position": 3},
  {"keys": ["https://www.psychichomily.com/shows/2026-09-28-first-day"], "clicks": 1, "impressions": 30, "ctr": 0.03, "position": 4},
  {"keys": ["https://psychichomily.com/shows/2026-10-04-last-day"], "clicks": 1, "impressions": 10, "ctr": 0.1, "position": 6},
  {"keys": ["https://psychichomily.com/shows/2026-10-05-day-after"], "clicks": 2, "impressions": 50, "ctr": 0.04, "position": 2},
  {"keys": ["https://psychichomily.com/shows/2026-02-30-not-a-real-day"], "clicks": 0, "impressions": 1, "ctr": 0, "position": 9},
  {"keys": ["https://psychichomily.com/shows/2026-10-01"], "clicks": 0, "impressions": 1, "ctr": 0, "position": 9},
  {"keys": ["https://psychichomily.com/shows/2026/10/03/extra"], "clicks": 0, "impressions": 1, "ctr": 0, "position": 9},
  {"keys": ["https://psychichomily.com/shows/123"], "clicks": 0, "impressions": 7, "ctr": 0, "position": 8},
  {"keys": ["https://psychichomily.com/shows/2026/10"], "clicks": 0, "impressions": 3, "ctr": 0, "position": 5},
  {"keys": ["https://psychichomily.com/shows/2026/10/03"], "clicks": 0, "impressions": 3, "ctr": 0, "position": 7},
  {"keys": ["https://psychichomily.com/artists/a?ref=x"], "clicks": 1, "impressions": 1, "ctr": 1, "position": 2},
  {"keys": ["https://psychichomily.com/artists/b"], "clicks": 0, "impressions": 3, "ctr": 0, "position": 10},
  {"keys": ["https://psychichomily.com/artists/zero-impressions"], "clicks": 0, "impressions": 0, "ctr": 0, "position": 0},
  {"keys": ["https://psychichomily.com/venues/v"], "clicks": 0, "impressions": 12, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/releases/r"], "clicks": 0, "impressions": 13, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/labels/l"], "clicks": 0, "impressions": 14, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/scenes/s"], "clicks": 0, "impressions": 15, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/collections/c"], "clicks": 0, "impressions": 14, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/festivals/f"], "clicks": 0, "impressions": 17, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/radio"], "clicks": 0, "impressions": 9, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/radio/kexp"], "clicks": 0, "impressions": 9, "ctr": 0, "position": 3},
  {"keys": ["https://psychichomily.com/"], "clicks": 0, "impressions": 19, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/about"], "clicks": 0, "impressions": 20, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/blog/post"], "clicks": 0, "impressions": 21, "ctr": 0, "position": 1},
  {"keys": ["https://psychichomily.com/x|y"], "clicks": 0, "impressions": 22, "ctr": 0, "position": 1}
]}
JSON

# Week of 2026-10-05: a first response exactly at the cap, then a short one.
echo 25000 >"$STUB_FIXTURES/generate_2026-10-05_2026-10-11_0"
cat >"$STUB_FIXTURES/pages_2026-10-05_2026-10-11_25000.json" <<'JSON'
{"rows": [
  {"keys": ["https://psychichomily.com/artists/tail"], "clicks": 0, "impressions": 1, "ctr": 0, "position": 5},
  {"keys": ["https://psychichomily.com/releases/tail"], "clicks": 0, "impressions": 1, "ctr": 0, "position": 5}
]}
JSON

# Query rows, week of 2026-09-28. First group: a dated show page at position
# 1.0 and at exactly 1.5 (one page, two queries), and a dated show page on the
# www host with a query string, dated after the week. Everything else: the
# same dated page at 1.6, the numeric, calendar and not-a-real-day /shows/
# shapes at position 1, and an artist page whose query cell carries a pipe,
# backticks and a marker that must never reach the doc. The zero-impression
# row counts in neither group.
cat >"$STUB_FIXTURES/query-pages_2026-09-28_2026-10-04_0.json" <<'JSON'
{"rows": [
  {"keys": ["q1", "https://psychichomily.com/shows/2026-09-30-show-one"], "clicks": 1, "impressions": 40, "ctr": 0.025, "position": 1},
  {"keys": ["q2", "https://psychichomily.com/shows/2026-09-30-show-one"], "clicks": 0, "impressions": 30, "ctr": 0, "position": 1.5},
  {"keys": ["q3", "https://www.psychichomily.com/shows/2026-10-10-show-two?ref=x"], "clicks": 0, "impressions": 30, "ctr": 0, "position": 1.2},
  {"keys": ["q4", "https://psychichomily.com/shows/2020-01-01-no-impressions"], "clicks": 0, "impressions": 0, "ctr": 0, "position": 1},
  {"keys": ["q5", "https://psychichomily.com/shows/2026-09-30-show-one"], "clicks": 0, "impressions": 10, "ctr": 0, "position": 1.6},
  {"keys": ["q6", "https://psychichomily.com/shows/123"], "clicks": 1, "impressions": 5, "ctr": 0.2, "position": 1},
  {"keys": ["q7", "https://psychichomily.com/shows/2026/10/03"], "clicks": 0, "impressions": 5, "ctr": 0, "position": 1},
  {"keys": ["q8", "https://psychichomily.com/shows/2026-02-30-not-a-real-day"], "clicks": 0, "impressions": 5, "ctr": 0, "position": 1},
  {"keys": ["UNTRUSTED-QUERY-MARKER | `x`", "https://psychichomily.com/artists/a"], "clicks": 2, "impressions": 25, "ctr": 0.08, "position": 1}
]}
JSON

# Query rows, week of 2026-10-05: a first response exactly at the cap of
# non-qualifying rows, then a short one holding one qualifying row.
echo 25000 >"$STUB_FIXTURES/generate-query-pages_2026-10-05_2026-10-11_0"
cat >"$STUB_FIXTURES/query-pages_2026-10-05_2026-10-11_25000.json" <<'JSON'
{"rows": [
  {"keys": ["tail", "https://psychichomily.com/shows/2026-10-06-tail-show"], "clicks": 0, "impressions": 4, "ctr": 0, "position": 1}
]}
JSON

# Query rows, week of 2026-10-12: rows, but none qualifying.
cat >"$STUB_FIXTURES/query-pages_2026-10-12_2026-10-18_0.json" <<'JSON'
{"rows": [
  {"keys": ["q9", "https://psychichomily.com/shows/2026-10-13-below-the-line"], "clicks": 0, "impressions": 3, "ctr": 0, "position": 2},
  {"keys": ["q10", "https://psychichomily.com/artists/z"], "clicks": 1, "impressions": 2, "ctr": 0.5, "position": 1}
]}
JSON

run() {
  # run <since> <until>: runs the script into a fresh output dir
  : >"$STUB_LOG"
  OUT="$(mktemp -d "$FIXTURE/out.XXXXXX")"
  OUTPUT="$(bash "$UNDER_TEST" --since "$1" --until "$2" --out "$OUT" 2>&1)"
  STATUS=$?
  DOC="$OUT/gsc-snapshot-$1_$2.md"
}

fail() {
  FAILURES=$((FAILURES + 1))
  echo "FAIL [$CASE]: $*"
}

expect_status() {
  [ "$STATUS" -eq "$1" ] || fail "expected exit $1, got $STATUS; output: $OUTPUT"
}

expect_line() {
  grep -Fxq -- "$1" "$DOC" 2>/dev/null || fail "expected the doc to contain the line: $1"
}

expect_no_text() {
  if grep -Fq -- "$1" "$DOC" 2>/dev/null; then fail "expected the doc not to contain: $1"; fi
}

# query_split_lines: the week headings, short-week flags and table rows of the
# query-row split section, in order.
query_split_lines() {
  awk '/^## Query rows split/{on=1; next} /^## /{on=0} on && /^(### |\| |\*\*Only )/' "$DOC"
}

expect_requests() {
  # expect_requests <count> <fixed string>: request bodies containing it
  local n
  n="$(grep -Fc -- "$2" "$STUB_LOG" || true)"
  [ "${n:-0}" -eq "$1" ] || fail "expected $1 request(s) containing '$2', got ${n:-0}"
}

# ---------------------------------------------------------------- cases

CASE="window 2026-09-26..2026-10-20: complete weeks, classifier, pagination, short week"
export STUB_LAST_SERVED=2026-10-16
run 2026-09-26 2026-10-20
unset STUB_LAST_SERVED
expect_status 0
expect_line "### Week of 2026-09-28 (Monday 2026-09-28 to Sunday 2026-10-04)"
expect_line "### Week of 2026-10-05 (Monday 2026-10-05 to Sunday 2026-10-11)"
expect_line "### Week of 2026-10-12 (Monday 2026-10-12 to Sunday 2026-10-18)"
expect_no_text "### Week of 2026-09-21"
expect_no_text "### Week of 2026-10-19"
# The zero-impression row is not a page with an impression, so 24 of 25 count.
expect_line "24 pages with impressions · 5 clicks · 297 impressions · impression-weighted position 2.2"
# Every family row of the week, in order (impressions descending).
expected_rows="$(cat <<'ROWS'
| `show: dated AFTER window` | 1 | 2 | 50 | 2 |
| `show: dated IN window` | 2 | 2 | 40 | 4.5 |
| `other: /x\|y` | 1 | 0 | 22 | 1 |
| `other: blog` | 1 | 0 | 21 | 1 |
| `other: /about` | 1 | 0 | 20 | 1 |
| `/` | 1 | 0 | 19 | 1 |
| `/radio` | 2 | 0 | 18 | 2 |
| `/festivals/` | 1 | 0 | 17 | 1 |
| `/scenes/` | 1 | 0 | 15 | 1 |
| `/collections/` | 1 | 0 | 14 | 1 |
| `/labels/` | 1 | 0 | 14 | 1 |
| `/releases/` | 1 | 0 | 13 | 1 |
| `/venues/` | 1 | 0 | 12 | 1 |
| `show: numeric id` | 1 | 0 | 7 | 8 |
| `shows calendar YYYY/MM(/DD)` | 2 | 0 | 6 | 6 |
| `/artists/` | 2 | 1 | 4 | 8 |
| `other: shows` | 3 | 0 | 3 | 9 |
| `show: dated BEFORE window` | 1 | 0 | 2 | 3 |
ROWS
)"
week_rows="$(awk '/^### Week of 2026-09-28/{on=1; next} /^### /{on=0} on && /^\| `/' "$DOC")"
[ "$week_rows" = "$expected_rows" ] \
  || fail "family rows for the week of 2026-09-28 differ:$(diff <(printf '%s\n' "$expected_rows") <(printf '%s\n' "$week_rows"))"
# Pagination: 25000 generated rows plus the short second response.
expect_line "| \`/artists/\` | 25001 | 0 | 25001 | 5 |"
expect_line "| \`/releases/\` | 1 | 0 | 1 | 5 |"
expect_requests 1 '"startDate":"2026-10-05","endDate":"2026-10-11","rowLimit":25000,"dimensions":["page"],"startRow":25000'
expect_requests 0 '"startRow":50000'
expect_requests 1 '"startDate":"2026-09-28","endDate":"2026-10-04","rowLimit":25000,"dimensions":["page"]}'
# The week of 2026-10-12 has no page rows and only 5 served days.
expect_line "| _no pages with impressions in this week_ | | | | |"
expect_line "0 pages with impressions · 0 clicks · 0 impressions"
expect_line "**Only 5 of the 7 days in this week were served (see the Coverage line); do not compare it with a full week.**"
# One flag in the breadth section, one in the query-row split.
[ "$(grep -c '^\*\*Only ' "$DOC" 2>/dev/null)" = "2" ] || fail "expected exactly two short-week flags"
expect_line "6. **Page indexing is not machine-captured.** The Page indexing report has"
expect_line "7. **Breadth counts distinct pages with at least one impression.** A family's"
expect_line "8. **Breadth totals are not the daily totals.** Rows grouped by page are"
expect_line "9. **Dated show pages at average position 1.5 or better are"
expect_line "   are a floor: Google withholds rare queries, so they cover only part of the"
# The query-row split: the classifier and the 1.5 boundary (week of 09-28),
# pagination past the cap (10-05), and a week with no qualifying row (10-12).
expected_split="$(cat <<'ROWS'
### Week of 2026-09-28 (Monday 2026-09-28 to Sunday 2026-10-04)
| Query rows | Rows | Pages | Impressions | Clicks | CTR |
| --- | ---: | ---: | ---: | ---: | ---: |
| Dated show pages at average position 1.5 or better | 3 | 2 | 100 | 1 | 1% |
| Everything else | 5 | 5 | 50 | 3 | 6% |
### Week of 2026-10-05 (Monday 2026-10-05 to Sunday 2026-10-11)
| Query rows | Rows | Pages | Impressions | Clicks | CTR |
| --- | ---: | ---: | ---: | ---: | ---: |
| Dated show pages at average position 1.5 or better | 1 | 1 | 4 | 0 | 0% |
| Everything else | 25000 | 25000 | 25000 | 0 | 0% |
### Week of 2026-10-12 (Monday 2026-10-12 to Sunday 2026-10-18)
**Only 5 of the 7 days in this week were served (see the Coverage line); do not compare it with a full week.**
| Query rows | Rows | Pages | Impressions | Clicks | CTR |
| --- | ---: | ---: | ---: | ---: | ---: |
| Dated show pages at average position 1.5 or better | 0 | 0 | 0 | 0 | — |
| Everything else | 2 | 2 | 5 | 1 | 20% |
ROWS
)"
split_lines="$(query_split_lines)"
[ "$split_lines" = "$expected_split" ] \
  || fail "query-row split differs:$(diff <(printf '%s\n' "$expected_split") <(printf '%s\n' "$split_lines"))"
expect_no_text "UNTRUSTED-QUERY-MARKER"
expect_requests 1 '"startDate":"2026-10-05","endDate":"2026-10-11","rowLimit":25000,"dimensions":["query","page"],"startRow":25000'
expect_requests 1 '"startDate":"2026-09-28","endDate":"2026-10-04","rowLimit":25000,"dimensions":["query","page"]}'
expect_line "### Page indexing (manual read)"
expect_line "| Indexed | |"
expect_line "| Discovered - currently not indexed | |"
expect_line "| Crawled - currently not indexed | |"
expect_line "Report last update:"
expect_line "_Read these in the Search Console UI under Indexing > Pages (<https://search.google.com/search-console/index?resource_id=sc-domain%3Apsychichomily.com>) and copy the report's own \"Last update\" date, not the day of the read._"

CASE="window that is exactly two weeks, Monday to Sunday"
run 2026-09-28 2026-10-11
expect_status 0
expect_line "### Week of 2026-09-28 (Monday 2026-09-28 to Sunday 2026-10-04)"
expect_line "### Week of 2026-10-05 (Monday 2026-10-05 to Sunday 2026-10-11)"
expect_no_text "**Only "

CASE="window holding no complete week"
run 2026-10-01 2026-10-05
expect_status 0
expect_line "_No complete Monday-to-Sunday week lies inside this window, so there is no breadth table._"
expect_line "_No complete Monday-to-Sunday week lies inside this window, so there is no query-row split._"
expect_no_text "### Week of"
# Only the window-wide fetches run, with the request bodies the doc's other
# tables are built from.
expect_requests 1 '"dimensions":["page"]'
expect_requests 1 '"dimensions":["query","page"]'
expect_requests 0 '"startRow"'
expect_requests 1 '{"startDate":"2026-10-01","endDate":"2026-10-05","rowLimit":1}'
expect_requests 1 '{"startDate":"2026-10-01","endDate":"2026-10-05","rowLimit":5,"dimensions":["date"]}'
expect_requests 1 '{"startDate":"2026-10-01","endDate":"2026-10-05","rowLimit":25000,"dimensions":["query"]}'
expect_requests 1 '{"startDate":"2026-10-01","endDate":"2026-10-05","rowLimit":25000,"dimensions":["query","page"]}'
expect_requests 1 '{"startDate":"2026-10-01","endDate":"2026-10-05","rowLimit":25000,"dimensions":["page"]}'

# ---------------------------------------------------------------- result

if [ "$FAILURES" -gt 0 ]; then
  echo "gsc-snapshot_test: $FAILURES failure(s)"
  exit 1
fi
echo "gsc-snapshot_test: all cases passed"
