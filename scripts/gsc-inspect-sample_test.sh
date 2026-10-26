#!/usr/bin/env bash
# scripts/gsc-inspect-sample_test.sh
#
# Self-test for scripts/gsc-inspect-sample.sh. Serves a fixture sitemap from
# file:// URLs through GSC_INSPECT_SITEMAP_INDEX, pins "today" with a stub
# date, and asserts: the six-family classifier (including the past/upcoming
# split on today's UTC date, the shapes that belong to no family, and URLs
# with control characters), deterministic and seed-sensitive sampling pinned
# against hashes computed independently with shasum, argument validation, the
# fixture-mode tables over scripts/testdata/gsc-inspect/, re-inspection of an
# earlier sample with --sample, and the inspection loop against a stub gcloud,
# curl and sleep: request shape, credential kept off argv, the backoff chosen
# per status, retry once on 429, 5xx and transport errors, a fresh token on
# 401, no retry on other errors, the stops (429 twice, 401 twice, 403,
# consecutive failures), runs in which nothing succeeded, and the exit codes.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/gsc-inspect-sample_test.sh
#
# Needs bash, curl, jq, python3 and shasum or sha256sum. It touches nothing
# outside its own mktemp -d and needs no network and no Google credential.

set -uo pipefail
# Byte-order sorting, matching the order Python sorts the hashes in.
export LC_ALL=C

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/gsc-inspect-sample.sh"
SHIPPED_FIXTURE="$SCRIPT_DIR/testdata/gsc-inspect"

TMP="$(mktemp -d)" || exit 1
trap 'rm -rf "$TMP"' EXIT

REAL_CURL="$(command -v curl)" || { echo "curl is required"; exit 1; }
REAL_DATE="$(command -v date)" || { echo "date is required"; exit 1; }
# The script's "today" is pinned by a stub date (below), so a run that
# crosses midnight UTC cannot split the expectations.
TODAY="2026-10-25"
FAILURES=0
CASE=""

fail() {
  FAILURES=$((FAILURES + 1))
  echo "FAIL [$CASE]: $*"
}

sha256_hex() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | cut -c1-64
  else
    shasum -a 256 | cut -c1-64
  fi
}

# --- Stubs on PATH ---------------------------------------------------------------

# DATE_BIN is on PATH for every run; STUB_BIN joins it for inspecting runs.
DATE_BIN="$TMP/date-bin"
STUB_BIN="$TMP/bin"
mkdir -p "$DATE_BIN" "$STUB_BIN"
cat >"$DATE_BIN/date" <<'STUB'
#!/usr/bin/env bash
if [ "$*" = "-u +%Y-%m-%d" ]; then echo "$STUB_TODAY"; else exec "$REAL_DATE" "$@"; fi
STUB

# gcloud logs each call and mints stub-token-<n>.
cat >"$STUB_BIN/gcloud" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  "auth application-default print-access-token")
    echo call >>"$STUB_LOG_DIR/gcloud.log"
    echo "stub-token-$(wc -l <"$STUB_LOG_DIR/gcloud.log" | tr -d ' ')"
    ;;
  *) echo "stub gcloud: unexpected call: $*" >&2; exit 9 ;;
esac
STUB

# sleep logs its argument and returns at once.
cat >"$STUB_BIN/sleep" <<'STUB'
#!/bin/sh
echo "$1" >>"$STUB_LOG_DIR/sleeps"
STUB

# curl hands every non-API URL (the file:// sitemap) to the real curl. An API
# request is logged (argv, stdin config, body) and answered with the next
# entry of STUB_LOG_DIR/plan, one per line; past the end of the plan it answers
# 200. Entries: 200 (an indexed result), 200-empty ({} with status 200),
# transport (exit 7, no body), <code> (a JSON error body), <code>html (an HTML
# body).
cat >"$STUB_BIN/curl" <<'STUB'
#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
if not args[-1].startswith("https://searchconsole.googleapis.com/"):
    real = os.environ["REAL_CURL"]
    os.execv(real, [real] + args)
out_file = body = None
for flag, value in zip(args, args[1:]):
    if flag == "-o":
        out_file = value
    elif flag == "--data":
        body = value
log_dir = os.environ["STUB_LOG_DIR"]
with open(os.path.join(log_dir, "argv.log"), "a") as log:
    log.write(" ".join(args) + "\n")
with open(os.path.join(log_dir, "stdin.log"), "a") as log:
    log.write(sys.stdin.read())
with open(os.path.join(log_dir, "body.log"), "a") as log:
    log.write(body + "\n")
counter = os.path.join(log_dir, "count")
n = int(open(counter).read()) if os.path.exists(counter) else 0
with open(counter, "w") as handle:
    handle.write(str(n + 1))
plan = open(os.path.join(log_dir, "plan")).read().split()
entry = plan[n] if n < len(plan) else "200"
if entry == "transport":
    sys.stderr.write("curl: (7) stub transport failure\n")
    sys.exit(7)
url = json.loads(body)["inspectionUrl"]
status = entry[:3]
if entry == "200":
    text = json.dumps({"inspectionResult": {"indexStatusResult": {
        "verdict": "PASS", "coverageState": "Submitted and indexed", "indexingState": "INDEXING_ALLOWED",
        "pageFetchState": "SUCCESSFUL", "robotsTxtState": "ALLOWED", "lastCrawlTime": "2026-10-01T00:00:00Z",
        "googleCanonical": url, "userCanonical": url}}})
elif entry == "200-empty":
    text = "{}"
elif entry.endswith("html"):
    text = "<html><body>stub " + status + "</body></html>"
else:
    text = json.dumps({"error": {"code": int(status), "message": "stub " + status, "status": "STUB"}})
with open(out_file, "w") as handle:
    handle.write(text)
sys.stdout.write(status)
STUB
chmod +x "$DATE_BIN/date" "$STUB_BIN/gcloud" "$STUB_BIN/sleep" "$STUB_BIN/curl"
export REAL_CURL REAL_DATE
export STUB_TODAY="$TODAY"

# --- Fixture sitemap -----------------------------------------------------------

SITEMAP="$TMP/sitemap"
mkdir -p "$SITEMAP"

# write_urlset <file> <path>...: a sitemap shard of https://psychichomily.com URLs.
write_urlset() {
  local file="$1" path
  shift
  {
    echo '<?xml version="1.0" encoding="UTF-8"?>'
    echo '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'
    for path in "$@"; do
      echo "  <url><loc>https://psychichomily.com${path}</loc></url>"
    done
    echo '</urlset>'
  } >"$file"
}

# write_index <file> <loc>...
write_index() {
  local file="$1" loc
  shift
  {
    echo '<?xml version="1.0" encoding="UTF-8"?>'
    echo '<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'
    for loc in "$@"; do
      echo "  <sitemap><loc>${loc}</loc></sitemap>"
    done
    echo '</sitemapindex>'
  } >"$file"
}

write_urlset "$SITEMAP/events.xml" \
  /shows/2099-01-01-future-a /shows/2099-02-02-future-b /shows/2099-03-03-future-c \
  "/shows/${TODAY}-today-show" \
  /shows/2020-01-01-past-a /shows/2020-02-02-past-b /shows/2020-03-03-past-c \
  /shows/2026-02-30-not-a-real-day /shows/legacy-show-2026-03-14 /shows/2026/10 /shows/123 \
  /venues/venue-a /venues/venue-b /venues/venue-a/shows/2026 '/venues?cities=Akron%2COH' \
  /scenes/akron-oh /scenes/akron-oh/2026-W41 /scenes/akron-oh/2026-W43
# &#10; and &#9; decode to a newline and a tab inside an otherwise valid path.
write_urlset "$SITEMAP/catalog.xml" \
  /artists/artist-a /artists/artist-b /artists/artist-c /artists/artist-d /artists/artist-e \
  /artists/artist-a '/artists/artist-a?ref=x' /artists/artist-a/extra '/artists/artist-b#top' \
  '/artists/split&#10;line' '/artists/tab&#9;bed' \
  /releases/release-a /releases/release-b /labels/label-a /
write_index "$SITEMAP/index.xml" "file://$SITEMAP/events.xml" "file://$SITEMAP/catalog.xml"
export GSC_INSPECT_SITEMAP_INDEX="file://$SITEMAP/index.xml"

# run [args...]: runs the script with the stub date; sets STATUS, STDOUT, STDERR.
run() {
  PATH="$DATE_BIN:$PATH" bash "$UNDER_TEST" "$@" >"$TMP/stdout" 2>"$TMP/stderr"
  STATUS=$?
  STDOUT="$(cat "$TMP/stdout")"
  STDERR="$(cat "$TMP/stderr")"
}

expect_status() {
  [ "$STATUS" -eq "$1" ] || fail "expected exit $1, got $STATUS; stderr: $STDERR"
}

expect_stderr() {
  printf '%s' "$STDERR" | grep -Fq -- "$1" || fail "expected stderr to contain: $1 (stderr: $STDERR)"
}

expect_line() {
  # expect_line <file> <exact line>
  grep -Fxq -- "$2" "$1" 2>/dev/null || fail "expected $(basename "$1") to contain the line: $2"
}

expect_no_text() {
  if grep -Fq -- "$2" "$1" 2>/dev/null; then fail "expected $(basename "$1") not to contain: $2"; fi
}

# family_urls <tsv> <family>: the sampled URLs of one family, in file order.
family_urls() {
  awk -F '\t' -v f="$2" 'NR > 2 && $1 == f { print $2 }' "$1"
}

# expect_family <tsv> <family> <url>...: the family's sampled URLs, sorted,
# are exactly the given URLs (given in sorted order).
expect_family() {
  local tsv="$1" family="$2" got want
  shift 2
  got="$(family_urls "$tsv" "$family" | sort | tr '\n' ' ')"
  want="$(printf '%s ' "$@")"
  [ "$got" = "$want" ] || fail "$family membership: got '$got', wanted '$want'"
}

# oracle_top <seed> <n> <url>...: the first n URLs by sha256(seed + url),
# computed with the system sha256 tool rather than the script's Python.
oracle_top() {
  local seed="$1" n="$2" url
  shift 2
  for url in "$@"; do
    printf '%s\t%s\n' "$(printf '%s' "$seed$url" | sha256_hex)" "$url"
  done | sort | head -n "$n" | cut -f2
}

ARTISTS=(
  https://psychichomily.com/artists/artist-a https://psychichomily.com/artists/artist-b
  https://psychichomily.com/artists/artist-c https://psychichomily.com/artists/artist-d
  https://psychichomily.com/artists/artist-e
)

# --- Classification and sampling ------------------------------------------------

CASE="every family classified; past/upcoming split on today; other shapes excluded"
run --sample-only --per-family 100
expect_status 0
printf '%s\n' "$STDOUT" >"$TMP/all.tsv"
[ "$(sed -n 2p "$TMP/all.tsv")" = "$(printf 'family\turl\thash')" ] || fail "second line is not the column header"
expect_line "$TMP/all.tsv" "# gsc-inspect-sample v1 seed=gsc-inspect-v1 per_family=100 today_utc=${TODAY} sitemap=file://$SITEMAP/index.xml population=upcoming-shows:4,past-shows:3,artists:5,releases:2,venues:2,scene-weeks:2"
expect_family "$TMP/all.tsv" upcoming-shows "https://psychichomily.com/shows/${TODAY}-today-show" \
  https://psychichomily.com/shows/2099-01-01-future-a https://psychichomily.com/shows/2099-02-02-future-b https://psychichomily.com/shows/2099-03-03-future-c
expect_family "$TMP/all.tsv" past-shows \
  https://psychichomily.com/shows/2020-01-01-past-a https://psychichomily.com/shows/2020-02-02-past-b https://psychichomily.com/shows/2020-03-03-past-c
expect_family "$TMP/all.tsv" venues https://psychichomily.com/venues/venue-a https://psychichomily.com/venues/venue-b
expect_family "$TMP/all.tsv" scene-weeks https://psychichomily.com/scenes/akron-oh/2026-W41 https://psychichomily.com/scenes/akron-oh/2026-W43
expect_family "$TMP/all.tsv" releases https://psychichomily.com/releases/release-a https://psychichomily.com/releases/release-b
# The duplicate artist-a is listed once; its query, fragment and subpath forms
# and the URLs holding a newline or a tab are excluded.
expect_family "$TMP/all.tsv" artists "${ARTISTS[@]}"
for excluded in not-a-real-day legacy-show /shows/2026/10 /shows/123 /venues/venue-a/shows/2026 'cities=' /scenes/akron-oh$'\t' /labels/ '?ref=' '#top' /artist-a/extra split tab; do
  expect_no_text "$TMP/all.tsv" "$excluded"
done
[ "$(awk -F '\t' 'NR > 2' "$TMP/all.tsv" | wc -l | tr -d ' ')" = "18" ] || fail "expected 18 sampled rows in total"
[ "$(awk -F '\t' 'NR > 2 && NF != 3' "$TMP/all.tsv" | wc -l | tr -d ' ')" = "0" ] || fail "every row should have exactly three cells"
expect_stderr "skipped 2 sitemap URL(s) holding whitespace or a control character"

CASE="sampling is deterministic: same seed twice is identical, and matches the shasum oracle"
run --sample-only --per-family 2 --seed alpha
expect_status 0
printf '%s\n' "$STDOUT" >"$TMP/alpha-1.tsv"
run --sample-only --per-family 2 --seed alpha
printf '%s\n' "$STDOUT" >"$TMP/alpha-2.tsv"
cmp -s "$TMP/alpha-1.tsv" "$TMP/alpha-2.tsv" || fail "two runs with seed alpha differ: $(diff "$TMP/alpha-1.tsv" "$TMP/alpha-2.tsv")"
[ "$(family_urls "$TMP/alpha-1.tsv" artists | tr '\n' ' ')" = "$(oracle_top alpha 2 "${ARTISTS[@]}" | tr '\n' ' ')" ] \
  || fail "artists sample for seed alpha does not match the shasum oracle"
# Literal pins: a change to the hash input or the sort order fails here.
expect_line "$TMP/alpha-1.tsv" "$(printf 'artists\thttps://psychichomily.com/artists/artist-e\t039b4c2508790e332ba4340e2cf2e47e6da18969ed8618441a58068fd5022bef')"
expect_line "$TMP/alpha-1.tsv" "$(printf 'artists\thttps://psychichomily.com/artists/artist-b\t5dcc9fc63a771690ee02647d431f0665b0970b0a646abfbd887974e2d6bf12ca')"
[ "$(awk -F '\t' 'NR > 2 { print $1 }' "$TMP/alpha-1.tsv" | uniq | tr '\n' ' ')" = "upcoming-shows past-shows artists releases venues scene-weeks " ] \
  || fail "families are not in canonical order"

CASE="a different seed draws a different sample"
run --sample-only --per-family 2 --seed beta
expect_status 0
printf '%s\n' "$STDOUT" >"$TMP/beta.tsv"
[ "$(family_urls "$TMP/beta.tsv" artists | tr '\n' ' ')" = "https://psychichomily.com/artists/artist-c https://psychichomily.com/artists/artist-a " ] \
  || fail "artists sample for seed beta: $(family_urls "$TMP/beta.tsv" artists | tr '\n' ' ')"
[ "$(family_urls "$TMP/beta.tsv" artists | tr '\n' ' ')" = "$(oracle_top beta 2 "${ARTISTS[@]}" | tr '\n' ' ')" ] \
  || fail "artists sample for seed beta does not match the shasum oracle"
if cmp -s <(tail -n +2 "$TMP/alpha-1.tsv") <(tail -n +2 "$TMP/beta.tsv"); then
  fail "seeds alpha and beta drew the same sample"
fi

CASE="--families selects a subset and keeps canonical order"
run --sample-only --families venues,artists --per-family 1
expect_status 0
[ "$(printf '%s\n' "$STDOUT" | awk -F '\t' 'NR > 2 { print $1 }' | tr '\n' ' ')" = "artists venues " ] \
  || fail "expected one artists row then one venues row, got: $STDOUT"

CASE="a family that matches no sitemap URL stops the run"
write_urlset "$SITEMAP/no-venues.xml" /artists/artist-a '/venues/venue-a/'
write_index "$SITEMAP/no-venues-index.xml" "file://$SITEMAP/no-venues.xml"
GSC_INSPECT_SITEMAP_INDEX="file://$SITEMAP/no-venues-index.xml" run --sample-only --families artists,venues
expect_status 1
expect_stderr "no sitemap URL matched these families, so the URL shapes may have moved"
expect_stderr "venues"

# --- Argument and sitemap validation -----------------------------------------------

CASE="unknown family is refused"
run --sample-only --families artists,labels
expect_status 1
expect_stderr "unknown family 'labels'"

CASE="duplicate family is refused"
run --sample-only --families artists,artists
expect_status 1
expect_stderr "family 'artists' is listed twice"

CASE="zero per-family is refused"
run --sample-only --per-family 0
expect_status 1
expect_stderr "--per-family must be greater than zero"

CASE="per-family above the daily quota is refused, including a value that would overflow"
run --sample-only --per-family 2001
expect_status 1
expect_stderr "--per-family must be at most 2000"
run --sample-only --per-family 99999999999999999999
expect_status 1
expect_stderr "--per-family must be at most 2000"

CASE="seed with a space is refused"
run --sample-only --seed 'a b'
expect_status 1
expect_stderr "--seed may contain only"

CASE="a plan above the daily quota is refused before any fetch"
run --per-family 400 --out "$TMP/never"
expect_status 1
expect_stderr "plans 2400 inspections, above the API's 2000-a-day quota"
[ ! -e "$TMP/never" ] || fail "a refused plan created the output directory"

CASE="--fixture or --sample with a sampling flag is refused"
run --fixture "$SHIPPED_FIXTURE" --seed alpha --out "$TMP/never"
expect_status 1
expect_stderr "use an existing sample and take no sampling flags"
run --sample "$SHIPPED_FIXTURE/sample.tsv" --per-family 3 --out "$TMP/never"
expect_status 1
expect_stderr "use an existing sample and take no sampling flags"

CASE="--fixture requires --out"
run --fixture "$SHIPPED_FIXTURE"
expect_status 1
expect_stderr "--fixture requires --out"

CASE="a shard on another host is refused"
write_index "$SITEMAP/foreign-index.xml" "file://$SITEMAP/events.xml" "https://example.com/sitemap.xml"
GSC_INSPECT_SITEMAP_INDEX="file://$SITEMAP/foreign-index.xml" run --sample-only
expect_status 1
expect_stderr "lists a shard on another scheme or host"
expect_stderr "https://example.com/sitemap.xml"

CASE="a shard whose decoded URL holds a newline is refused, not split into two fetches"
write_index "$SITEMAP/newline-index.xml" "file://$SITEMAP/events.xml&#10;https://example.com/x.xml"
GSC_INSPECT_SITEMAP_INDEX="file://$SITEMAP/newline-index.xml" run --sample-only
expect_status 1
expect_stderr "with whitespace or a control character"

CASE="a plain-http sitemap index is refused"
GSC_INSPECT_SITEMAP_INDEX="http://psychichomily.com/sitemap-index" run --sample-only
expect_status 1
expect_stderr "must be an https:// or file:// URL"

# --- Fixture mode -------------------------------------------------------------------

OUT="$TMP/fixture-out"
DOC="$OUT/gsc-inspect-${TODAY}.md"

CASE="fixture mode renders the expected roll-up and family tables"
run --fixture "$SHIPPED_FIXTURE" --out "$OUT"
expect_status 0
[ "$STDOUT" = "$DOC" ] || fail "stdout should be the summary path, got: $STDOUT"
expected_rollup="$(cat <<'ROWS'
| Family | Advertised | Sampled | Inspected | Failed | Not inspected | PASS | PARTIAL | FAIL | NEUTRAL | Other verdict | Canonical differs |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| upcoming-shows | 2 | 2 | 2 | 0 | 0 | 1 | 0 | 0 | 1 | 0 | 0 of 1 |
| past-shows | 340 | 3 | 2 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 of 1 |
| artists | 2 | 2 | 2 | 0 | 0 | 1 | 0 | 0 | 1 | 0 | 1 of 2 |
| releases | 2 | 2 | 2 | 0 | 0 | 0 | 1 | 0 | 0 | 1 | 0 of 0 |
| venues | 2 | 2 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 of 0 |
| scene-weeks | 2 | 2 | 2 | 0 | 0 | 1 | 0 | 0 | 1 | 0 | 1 of 2 |
| **All families** | 350 | 13 | 11 | 1 | 1 | 4 | 1 | 2 | 3 | 1 | 2 of 6 |
ROWS
)"
rollup="$(awk '/^## Roll-up/{on=1; next} /^## /{on=0} on && /^\|/' "$DOC")"
[ "$rollup" = "$expected_rollup" ] \
  || fail "roll-up differs:$(diff <(printf '%s\n' "$expected_rollup") <(printf '%s\n' "$rollup"))"
expect_line "$DOC" "> **Fixture render.** Built offline from \`$SHIPPED_FIXTURE\`; no API call was made, and these numbers are not production data."
expect_line "$DOC" "Sampled 3 of 340 advertised (requested 3): 2 inspected, 1 failed, 0 not inspected."
expect_line "$DOC" "Sampled 2 of 2 advertised (requested 3): 1 inspected, 0 failed, 1 not inspected."
expect_line "$DOC" "| pageFetchState | \`SOFT_404\` | 1 |"
expect_line "$DOC" "| coverageState | \`Duplicate, Google chose different canonical than user\` | 1 |"
expect_line "$DOC" "| canonical | googleCanonical differs from userCanonical | 1 of 2 with both set |"
expect_line "$DOC" "| indexingState | \`(absent)\` | 1 |"
expect_line "$DOC" "| lastCrawlTime | \`2026-09\` | 1 |"
expect_line "$DOC" "| lastCrawlTime | \`never\` | 1 |"
expect_line "$DOC" "| \`https://psychichomily.com/shows/2020-05-01-fixture-quintet-at-fixture-cellar\` | \`500\` | Internal error encountered. |"
expect_no_text "$DOC" "INCOMPLETE"
expect_no_text "$DOC" "Credential:"
cmp -s "$SHIPPED_FIXTURE/sample.tsv" "$OUT/gsc-inspect-${TODAY}/sample.tsv" || fail "the run directory does not hold a copy of sample.tsv"
[ "$(find "$OUT/gsc-inspect-${TODAY}/raw" -name '*.json' | wc -l | tr -d ' ')" = "12" ] || fail "expected 12 raw files in the run directory"

CASE="an existing summary is refused without --force"
run --fixture "$SHIPPED_FIXTURE" --out "$OUT"
expect_status 1
expect_stderr "already exists; move it or pass --force"

CASE="--force replaces the summary and the run directory"
echo stale >"$OUT/gsc-inspect-${TODAY}/stale-file"
run --fixture "$SHIPPED_FIXTURE" --out "$OUT" --force
expect_status 0
[ ! -e "$OUT/gsc-inspect-${TODAY}/stale-file" ] || fail "--force kept a stale file in the run directory"
expect_line "$DOC" "| **All families** | 350 | 13 | 11 | 1 | 1 | 4 | 1 | 2 | 3 | 1 | 2 of 6 |"

CASE="a malformed fixture sample.tsv is refused"
mkdir -p "$TMP/bad-fixture"
printf 'family\turl\thash\n' >"$TMP/bad-fixture/sample.tsv"
run --fixture "$TMP/bad-fixture" --out "$TMP/bad-out"
expect_status 1
expect_stderr "metadata line"
[ ! -e "$TMP/bad-out/gsc-inspect-${TODAY}.md" ] || fail "a malformed fixture still produced a summary"

# --- Inspection loop against stubs -----------------------------------------------------

# live <plan...> -- [args...]: a stubbed inspecting run into a fresh output
# directory. Without --sample in args it samples venues and scene-weeks, two
# URLs each (4 inspections).
live() {
  local plan=() sampling=(--families "venues,scene-weeks" --per-family 2)
  while [ "$1" != "--" ]; do plan+=("$1"); shift; done
  shift
  case " $* " in *" --sample "*) sampling=() ;; esac
  STUB_LOG_DIR="$(mktemp -d "$TMP/log.XXXXXX")"
  printf '%s\n' ${plan[@]+"${plan[@]}"} >"$STUB_LOG_DIR/plan"
  : >"$STUB_LOG_DIR/sleeps"
  LIVE_OUT="$(mktemp -d "$TMP/live.XXXXXX")/out"
  LIVE_DOC="$LIVE_OUT/gsc-inspect-${TODAY}.md"
  LIVE_RUN="$LIVE_OUT/gsc-inspect-${TODAY}"
  export STUB_LOG_DIR
  PATH="$STUB_BIN:$PATH" run ${sampling[@]+"${sampling[@]}"} --out "$LIVE_OUT" "$@"
}

requests() { cat "$STUB_LOG_DIR/count" 2>/dev/null || echo 0; }
# backoffs: the sleeps longer than the request pacing, in order.
backoffs() { grep -vx '0.11' "$STUB_LOG_DIR/sleeps" | tr '\n' ' '; }
failure_statuses() { awk -F '\t' 'NR > 1 { print $4 }' "$LIVE_RUN/failures.tsv" | tr '\n' ' '; }

CASE="a complete run: request shape, credential off argv, retry once on 5xx after 5s"
live 200 500 200 200 200 --
expect_status 0
[ "$(requests)" = "5" ] || fail "expected 5 API requests (one retry), got $(requests)"
expect_line "$STUB_LOG_DIR/body.log" '{"inspectionUrl":"https://psychichomily.com/venues/venue-a","siteUrl":"sc-domain:psychichomily.com","languageCode":"en-US"}'
expect_line "$STUB_LOG_DIR/stdin.log" 'header = "Authorization: Bearer stub-token-1"'
expect_no_text "$STUB_LOG_DIR/argv.log" "stub-token"
[ "$(grep -c -- '-H x-goog-user-project: psychic-homily' "$STUB_LOG_DIR/argv.log")" = "5" ] || fail "every request should carry the quota-project header"
[ "$(grep -c -- '--proto =https' "$STUB_LOG_DIR/argv.log")" = "5" ] || fail "every request should pin the protocol to https"
[ "$(grep -cx '0.11' "$STUB_LOG_DIR/sleeps")" = "5" ] || fail "every request should be followed by the pacing pause"
[ "$(backoffs)" = "5 " ] || fail "a 5xx should back off 5s, got: $(backoffs)"
expect_line "$LIVE_DOC" "| venues | 2 | 2 | 2 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 of 2 |"
expect_line "$LIVE_DOC" "| scene-weeks | 2 | 2 | 2 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 of 2 |"
expect_line "$LIVE_DOC" "| **All families** | 4 | 4 | 4 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 of 4 |"
[ "$(find "$LIVE_RUN/raw" -name '*.json' | wc -l | tr -d ' ')" = "4" ] || fail "expected 4 raw responses"
[ "$(wc -l <"$LIVE_RUN/failures.tsv" | tr -d ' ')" = "1" ] || fail "expected a header-only failures.tsv"
expect_no_text "$LIVE_DOC" "INCOMPLETE"
expect_stderr "500 on https://psychichomily.com/venues/venue-"

CASE="a 429 backs off 61s and a successful retry carries on"
live 429 200 200 200 200 --
expect_status 0
[ "$(requests)" = "5" ] || fail "expected 5 API requests, got $(requests)"
[ "$(backoffs)" = "61 " ] || fail "a 429 should back off 61s, got: $(backoffs)"
expect_line "$LIVE_DOC" "| **All families** | 4 | 4 | 4 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 of 4 |"

CASE="other errors are not retried; a transport error is retried once then recorded"
live 200 404 transport transport 200 --
expect_status 0
[ "$(requests)" = "5" ] || fail "expected 5 API requests, got $(requests)"
expect_line "$LIVE_DOC" "| venues | 2 | 2 | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 of 1 |"
expect_line "$LIVE_DOC" "| scene-weeks | 2 | 2 | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 of 1 |"
[ "$(failure_statuses)" = "404 transport " ] || fail "failures.tsv statuses: $(failure_statuses)"
grep -Fq "| \`404\` | stub 404 |" "$LIVE_DOC" || fail "the 404 failure row is missing its message"

CASE="a 200 without inspectionResult and a non-JSON body are failures with their bodies kept"
live 200-empty 502html 502html 200 200 --
expect_status 0
[ "$(failure_statuses)" = "200 without inspectionResult 502 " ] || fail "failures.tsv statuses: $(failure_statuses)"
[ "$(find "$LIVE_RUN/raw" -name '*.body' | wc -l | tr -d ' ')" = "1" ] || fail "the HTML body should be kept as a .body file"
grep -Fq "non-JSON response body, kept as raw/venues/" "$LIVE_RUN/failures.tsv" || fail "the 502 failure should point at its kept body"

CASE="a 429 on both attempts for one URL stops the run, writes what it has, and exits 2"
live 200 500 200 429 429 --
expect_status 2
[ "$(requests)" = "5" ] || fail "expected 5 API requests before the stop, got $(requests)"
expect_stderr "STOPPED EARLY: 429 on both attempts for one URL, a minute apart: the daily quota is exhausted"
expect_stderr "429 on https://psychichomily.com/scenes/akron-oh/"
[ "$STDOUT" = "$LIVE_DOC" ] || fail "stdout should be the summary path, got: $STDOUT"
expect_line "$LIVE_DOC" "> **INCOMPLETE: the run stopped early (429 on both attempts for one URL, a minute apart: the daily quota is exhausted).** 2 of 4 sampled URLs were inspected; the rest count under Failed or Not inspected. Do not compare these counts with a complete run."
expect_line "$LIVE_DOC" "| venues | 2 | 2 | 2 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 of 2 |"
expect_line "$LIVE_DOC" "| scene-weeks | 2 | 2 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 of 0 |"
[ "$(failure_statuses)" = "429 " ] || fail "failures.tsv should hold the one 429"

CASE="a 429 after a 5xx is recorded without stopping the run"
live 200 500 429 200 200 --
expect_status 0
[ "$(failure_statuses)" = "429 " ] || fail "failures.tsv statuses: $(failure_statuses)"
expect_no_text "$LIVE_DOC" "INCOMPLETE"

CASE="a 401 mints a fresh token and retries once"
live 200 401 200 200 200 --
expect_status 0
[ "$(wc -l <"$STUB_LOG_DIR/gcloud.log" | tr -d ' ')" = "2" ] || fail "expected a second token mint after the 401"
expect_line "$STUB_LOG_DIR/stdin.log" 'header = "Authorization: Bearer stub-token-2"'
expect_line "$LIVE_DOC" "| **All families** | 4 | 4 | 4 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 of 4 |"

CASE="a 401 to a fresh token stops the run with the auth help"
live 200 401 401 --
expect_status 2
expect_stderr "STOPPED EARLY: HTTP 401 from the API with a freshly minted token: the credential was refused"
expect_stderr "gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly"

CASE="a 403 stops the run with the auth help"
live 200 403 --
expect_status 2
[ "$(requests)" = "2" ] || fail "expected 2 API requests before the stop, got $(requests)"
expect_stderr "STOPPED EARLY: HTTP 403 from the API: the credential was refused"
expect_stderr "gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly"

CASE="a stop before any success writes nothing and exits 1"
live 429 429 --
expect_status 1
[ "$(requests)" = "2" ] || fail "expected 2 API requests before the stop, got $(requests)"
expect_stderr "no inspection succeeded (stopped: 429 on both attempts for one URL, a minute apart: the daily quota is exhausted). Nothing was written."
[ ! -e "$LIVE_DOC" ] && [ ! -e "$LIVE_RUN" ] || fail "a stop before any success still wrote output"

CASE="a run in which every URL fails writes nothing and exits 1"
live 404 404 404 404 --
expect_status 1
[ "$(requests)" = "4" ] || fail "expected 4 API requests, got $(requests)"
expect_stderr "no inspection succeeded. Nothing was written."
expect_stderr "stub 404"
[ ! -e "$LIVE_DOC" ] && [ ! -e "$LIVE_RUN" ] || fail "an all-failed run still wrote output"

CASE="an unwritable output directory is refused before any request"
STUB_LOG_DIR="$(mktemp -d "$TMP/log.XXXXXX")"
printf '200\n' >"$STUB_LOG_DIR/plan"
export STUB_LOG_DIR
echo file >"$TMP/a-file"
PATH="$STUB_BIN:$PATH" run --families venues --per-family 1 --out "$TMP/a-file/sub"
expect_status 1
expect_stderr "cannot write to the output directory"
[ "$(requests)" = "0" ] || fail "expected no API request, got $(requests)"

# --- Re-inspecting an earlier sample -------------------------------------------------

# 12 artists in hash order, so the consecutive-failure case has room to trip.
REUSE="$TMP/reuse-sample.tsv"
{
  echo "# gsc-inspect-sample v1 seed=earlier per_family=12 today_utc=2026-09-01 sitemap=https://psychichomily.com/sitemap-index population=artists:500"
  printf 'family\turl\thash\n'
  for i in 01 02 03 04 05 06 07 08 09 10 11 12; do
    url="https://psychichomily.com/artists/earlier-$i"
    printf 'artists\t%s\t%s\n' "$url" "$(printf '%s' "earlier$url" | sha256_hex)"
  done
} >"$REUSE"

CASE="--sample re-inspects exactly the earlier URLs, labelled as a re-inspection"
live -- --sample "$REUSE"
expect_status 0
[ "$(requests)" = "12" ] || fail "expected 12 API requests, got $(requests)"
[ "$(jq -r '.inspectionUrl' "$STUB_LOG_DIR/body.log" | tr '\n' ' ')" = "$(awk -F '\t' 'NR > 2 { print $2 }' "$REUSE" | tr '\n' ' ')" ] \
  || fail "the inspected URLs should be the reused sample's, in its order"
cmp -s "$REUSE" "$LIVE_RUN/sample.tsv" || fail "the run should keep the reused sample.tsv unchanged"
expect_line "$LIVE_DOC" "> **Re-inspected sample.** The URLs and family labels are those of \`$REUSE\`, drawn on 2026-09-01, not a fresh draw from today's sitemap; Advertised is that draw's count."
expect_line "$LIVE_DOC" "| artists | 500 | 12 | 12 | 0 | 0 | 12 | 0 | 0 | 0 | 0 | 0 of 12 |"

CASE="a malformed --sample is refused before the credential is used"
printf '# gsc-inspect-sample v1 seed=x\nfamily\turl\thash\nartists\thttps://psychichomily.com/artists/a\tnot-a-hash\n' >"$TMP/bad-sample.tsv"
live -- --sample "$TMP/bad-sample.tsv"
expect_status 1
expect_stderr "invalid sample"
[ "$(requests)" = "0" ] || fail "expected no API request, got $(requests)"
[ ! -e "$STUB_LOG_DIR/gcloud.log" ] || fail "the token should not be minted for a malformed sample"

CASE="10 failed URLs in a row stop the run"
live 200 404 404 404 404 404 404 404 404 404 404 -- --sample "$REUSE"
expect_status 2
[ "$(requests)" = "11" ] || fail "expected 11 API requests before the stop, got $(requests)"
expect_stderr "STOPPED EARLY: 10 URLs in a row failed"
expect_line "$LIVE_DOC" "| artists | 500 | 12 | 1 | 10 | 1 | 1 | 0 | 0 | 0 | 0 | 0 of 1 |"

# --- Result -----------------------------------------------------------------------------

if [ "$FAILURES" -gt 0 ]; then
  echo "gsc-inspect-sample_test: $FAILURES failure(s)"
  exit 1
fi
echo "gsc-inspect-sample_test: all cases passed"
