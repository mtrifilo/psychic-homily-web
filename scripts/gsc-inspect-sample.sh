#!/usr/bin/env bash
#
# Inspect a deterministic per-family sample of sitemap URLs with the Google
# Search Console URL Inspection API and record a dated index-state baseline.
#
# WHY THIS EXISTS
# ---------------
# The Page indexing report has no bulk API and no per-family breakdown, and the
# breadth tables in scripts/gsc-snapshot.sh count pages that earned
# impressions, not pages Google has indexed. This script draws a fixed sample
# from each URL family in the live sitemap, inspects every sampled URL, and
# writes per-family counts of Google's verdict, coverage state, indexing
# state, fetch state, robots.txt state, canonical agreement, and last crawl
# month, so an index-state change can be attributed to a family.
#
# SAMPLING
# --------
# The sitemap index and every shard it lists are fetched (public, no
# credential). Each URL is placed in at most one family by its path:
#
#   upcoming-shows  /shows/YYYY-MM-DD-...   slug date on or after today (UTC)
#   past-shows      /shows/YYYY-MM-DD-...   slug date before today (UTC)
#   artists         /artists/<slug>
#   releases        /releases/<slug>
#   venues          /venues/<slug>          (not /venues/<slug>/shows/<year>)
#   scene-weeks     /scenes/<slug>/YYYY-Www
#
# A slug date that is not a real calendar day, a URL with a query string or a
# fragment, and every other path belong to no family. Each family is ordered
# by sha256(seed + url) (hex, then URL) and the first N are kept.
#
# MEMBERSHIP: the same seed over the same sitemap always yields the same
# sample. The seed is the only input that re-draws a whole family; membership
# otherwise moves only with the sitemap itself: a new URL whose hash sorts
# ahead of the cut enters, a URL removed from the sitemap leaves, and an
# upcoming show moves to past-shows once its date passes. sample.tsv is
# written beside the results so a later run can diff membership before its
# counts are compared.
#
# QUOTA
# -----
# The URL Inspection API allows 2000 calls a day and 600 a minute per
# property. A run that would plan more than 2000 inspections is refused. Each
# request is followed by a pause, so request starts are at least
# REQUEST_INTERVAL_SECONDS apart. A 429, a 5xx, or a transport error is retried
# once after a backoff. A 429 on the retry, or a 401 or 403, stops the run:
# the results so far are written and marked incomplete, or nothing is written
# when no inspection had succeeded. Other runs on the same day share the
# daily quota.
#
# OUTPUT
# ------
#   <out>/gsc-inspect-<date>.md                       the summary
#   <out>/gsc-inspect-<date>/sample.tsv               metadata line, header,
#                                                     then family, url, hash
#   <out>/gsc-inspect-<date>/raw/<family>/<hash>.json every response body
#   <out>/gsc-inspect-<date>/failures.tsv             every URL that failed
#
# <date> is the UTC capture date. The final stdout line is the summary's path.
#
# Usage:
#   scripts/gsc-inspect-sample.sh [--out DIR] [--per-family N] [--families LIST]
#                                 [--seed STRING] [--force]
#   scripts/gsc-inspect-sample.sh --sample-only [--per-family N] [--families LIST] [--seed STRING]
#   scripts/gsc-inspect-sample.sh --fixture DIR [--out DIR] [--force]
#
#   --out DIR         Directory to write into. Default: docs/research relative
#                     to the repo root. docs/ is gitignored and absent from
#                     fresh worktrees, so pass --out explicitly elsewhere.
#   --per-family N    URLs to sample per family. Default: 100.
#   --families LIST   Comma-separated subset of the six families above.
#                     Default: all six. Output keeps the order above.
#   --seed STRING     Sampling seed, [A-Za-z0-9._:-], up to 64 characters.
#                     Default: gsc-inspect-v1. Recorded in sample.tsv and in
#                     the summary.
#   --sample-only     Print the sample as TSV on stdout and exit. Fetches the
#                     sitemap only: no credential, no API call, no files.
#   --fixture DIR     Offline render: read DIR/sample.tsv, DIR/raw/<family>/
#                     <hash>.json and DIR/failures.tsv (optional) instead of
#                     fetching and inspecting, then write the summary and a
#                     copy of those files as a normal run would. Takes no
#                     sampling flags. scripts/testdata/gsc-inspect/ is a
#                     hand-written example.
#   --force           Replace an existing summary and run directory for this
#                     date (refused by default).
#
# Exit status: 0 on a complete run, 2 when the run stopped early and partial
# results were written, 1 on any other error (including a stop before any
# inspection succeeded).
#
# Environment:
#   GSC_SITE                   Property. Default: sc-domain:psychichomily.com
#   GSC_QUOTA_PROJECT          x-goog-user-project quota header. Default:
#                              psychic-homily
#   GSC_INSPECT_SITEMAP_INDEX  Sitemap index URL, https:// or file://. Default:
#                              https://psychichomily.com/sitemap-index. Shards
#                              must share the index's scheme and host.
#
# Requires: bash, curl, jq, python3; for a live run, the gcloud CLI holding an
# Application Default Credential that includes the webmasters.readonly scope
# (the same credential scripts/gsc-snapshot.sh uses). scripts/
# gsc-inspect-sample_test.sh is the self-test.

set -euo pipefail

# --- Configuration ----------------------------------------------------------

SITE="${GSC_SITE:-sc-domain:psychichomily.com}"
# ADC user credentials must name a quota project or the API rejects them.
QUOTA_PROJECT="${GSC_QUOTA_PROJECT:-psychic-homily}"
SITEMAP_INDEX="${GSC_INSPECT_SITEMAP_INDEX:-https://psychichomily.com/sitemap-index}"
# Not overridable: the bearer token is attached to every request to this URL.
INSPECT_URL="https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"

ALL_FAMILIES="upcoming-shows past-shows artists releases venues scene-weeks"
DEFAULT_SEED="gsc-inspect-v1"
DAILY_QUOTA=2000
# 60 / 0.11 < 600: at most 545 request starts fit in any minute.
REQUEST_INTERVAL_SECONDS=0.11
# Longer than a minute, so a 429 on the retry is not the per-minute quota.
BACKOFF_429_SECONDS=61
BACKOFF_RETRY_SECONDS=5

PER_FAMILY=100
FAMILIES_ARG=""
SEED="$DEFAULT_SEED"
OUT_DIR=""
FIXTURE_DIR=""
SAMPLE_ONLY=0
FORCE=0
SAMPLING_FLAG_GIVEN=""

# --- Helpers ----------------------------------------------------------------

die() {
  printf 'gsc-inspect-sample: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Inspect a deterministic per-family sample of sitemap URLs with the Search
Console URL Inspection API and write a dated index-state summary.

Usage:
  scripts/gsc-inspect-sample.sh [--out DIR] [--per-family N] [--families LIST]
                                [--seed STRING] [--force]
  scripts/gsc-inspect-sample.sh --sample-only [--per-family N] [--families LIST] [--seed STRING]
  scripts/gsc-inspect-sample.sh --fixture DIR [--out DIR] [--force]

  --out DIR         Output directory. Default: docs/research in the repo root.
  --per-family N    URLs sampled per family. Default: 100.
  --families LIST   Comma-separated subset of: upcoming-shows, past-shows,
                    artists, releases, venues, scene-weeks. Default: all six.
  --seed STRING     Sampling seed ([A-Za-z0-9._:-], up to 64). Default:
                    gsc-inspect-v1. The seed is the only input that re-draws a
                    whole family; sitemap changes move membership at the
                    margin (see the script header).
  --sample-only     Print the sample TSV on stdout; no credential, no API call.
  --fixture DIR     Render offline from DIR/sample.tsv and DIR/raw/.
  --force           Replace an existing summary and run directory for today.

Exit status: 0 complete, 2 stopped early with partial results written,
1 any other error. The script header documents the families, the quota
handling and the environment overrides.
USAGE
}

# The final stdout line is the summary's path, so usage on the error path goes
# to stderr.
usage_and_exit() {
  if [ "${1:-0}" -eq 0 ]; then usage; else usage >&2; fi
  exit "${1:-0}"
}

require_value() {
  case "${2:-}" in
    ''|--*) die "$1 requires a value" ;;
  esac
}

# --- Argument parsing -------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --out)         require_value --out "${2:-}"; OUT_DIR="$2"; shift 2 ;;
    --per-family)  require_value --per-family "${2:-}"; PER_FAMILY="$2"; SAMPLING_FLAG_GIVEN="$1"; shift 2 ;;
    --families)    require_value --families "${2:-}"; FAMILIES_ARG="$2"; SAMPLING_FLAG_GIVEN="$1"; shift 2 ;;
    --seed)        require_value --seed "${2:-}"; SEED="$2"; SAMPLING_FLAG_GIVEN="$1"; shift 2 ;;
    --fixture)     require_value --fixture "${2:-}"; FIXTURE_DIR="$2"; shift 2 ;;
    --sample-only) SAMPLE_ONLY=1; SAMPLING_FLAG_GIVEN="$1"; shift ;;
    --force)       FORCE=1; shift ;;
    -h|--help)     usage_and_exit 0 ;;
    *) printf 'gsc-inspect-sample: unknown argument %s\n\n' "$1" >&2; usage_and_exit 1 ;;
  esac
done

command -v curl    >/dev/null 2>&1 || die "curl is required"
command -v jq      >/dev/null 2>&1 || die "jq is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

if [ -n "$FIXTURE_DIR" ] && [ -n "$SAMPLING_FLAG_GIVEN" ]; then
  die "--fixture renders an existing sample and takes no sampling flags (got $SAMPLING_FLAG_GIVEN)"
fi

case "$PER_FAMILY" in
  ''|*[!0-9]*) die "--per-family must be a positive integer, got '$PER_FAMILY'" ;;
esac
# Leading zeros would otherwise be read as octal by $(( )).
PER_FAMILY=$((10#$PER_FAMILY))
[ "$PER_FAMILY" -gt 0 ] || die "--per-family must be greater than zero"

case "$SEED" in
  *[!A-Za-z0-9._:-]*) die "--seed may contain only letters, digits, '.', '_', ':' and '-'" ;;
esac
[ "${#SEED}" -le 64 ] || die "--seed must be at most 64 characters"

# FAMILIES holds the selected families in ALL_FAMILIES order.
FAMILIES="$ALL_FAMILIES"
if [ -n "$FAMILIES_ARG" ]; then
  requested=" "
  for name in $(printf '%s' "$FAMILIES_ARG" | tr ',' ' '); do
    case " $ALL_FAMILIES " in
      *" $name "*) ;;
      *) die "unknown family '$name' (known: $(printf '%s' "$ALL_FAMILIES" | tr ' ' ','))" ;;
    esac
    case "$requested" in
      *" $name "*) die "family '$name' is listed twice in --families" ;;
    esac
    requested="${requested}${name} "
  done
  FAMILIES=""
  for name in $ALL_FAMILIES; do
    case "$requested" in
      *" $name "*) FAMILIES="${FAMILIES:+$FAMILIES }$name" ;;
    esac
  done
  [ -n "$FAMILIES" ] || die "--families selected no family"
fi
FAMILY_COUNT=0
for name in $FAMILIES; do FAMILY_COUNT=$((FAMILY_COUNT + 1)); done
FAMILIES_CSV="$(printf '%s' "$FAMILIES" | tr ' ' ',')"

PLANNED=$((PER_FAMILY * FAMILY_COUNT))
if [ -z "$FIXTURE_DIR" ] && [ "$SAMPLE_ONLY" -ne 1 ] && [ "$PLANNED" -gt "$DAILY_QUOTA" ]; then
  die "${FAMILY_COUNT} families x ${PER_FAMILY} URLs plans ${PLANNED} inspections, above the API's ${DAILY_QUOTA}-a-day quota per property"
fi

# --- Python programs ----------------------------------------------------------

# Sitemap documents are read with a regex over <loc> elements, entity-decoded,
# rather than with xml.etree, so the script does not depend on pyexpat, which
# is not loadable in every Python build.
#
# Prints the <loc> of every entry in a sitemap index, one per line. Exits 3
# when the document is not a sitemap index, 4 when a shard is not on the
# index's own scheme and host.
IFS= read -r -d '' PY_INDEX_LOCS <<'PY' || true
import html, re, sys, urllib.parse
index_url, path = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as handle:
    document = handle.read()
if not re.search(r"<sitemapindex[\s>]", document):
    sys.exit(3)
def origin(url):
    parts = urllib.parse.urlsplit(url)
    return (parts.scheme.lower(), parts.netloc.lower())
for raw in re.findall(r"<loc>([^<]*)</loc>", document):
    loc = html.unescape(raw).strip()
    if not loc:
        continue
    if origin(loc) != origin(index_url):
        sys.stderr.write(loc + "\n")
        sys.exit(4)
    print(loc)
PY

# Classifies every URL in the shard files and prints the sample as TSV: the
# metadata line, the column header, then family, url, hash rows in family
# order and hash order within a family. Per-family population counts go into
# the metadata line.
IFS= read -r -d '' PY_SAMPLE <<'PY' || true
import datetime, hashlib, html, re, sys, urllib.parse
today, seed, per_family, families, index_url = sys.argv[1:6]
shards = sys.argv[6:]
per_family = int(per_family)
families = families.split(",")
SHOW = re.compile(r"/shows/([0-9]{4}-[0-9]{2}-[0-9]{2})-[^/]+")
SINGLE = re.compile(r"/(artists|releases|venues)/[^/]+")
SCENE_WEEK = re.compile(r"/scenes/[^/]+/[0-9]{4}-W[0-9]{2}")

def family_of(url):
    parts = urllib.parse.urlsplit(url)
    if parts.scheme not in ("http", "https") or parts.query or parts.fragment:
        return None
    path = parts.path
    match = SHOW.fullmatch(path)
    if match:
        try:
            datetime.date.fromisoformat(match.group(1))
        except ValueError:
            return None
        return "upcoming-shows" if match.group(1) >= today else "past-shows"
    match = SINGLE.fullmatch(path)
    if match:
        return match.group(1)
    if SCENE_WEEK.fullmatch(path):
        return "scene-weeks"
    return None

members = {name: set() for name in families}
for shard in shards:
    with open(shard, encoding="utf-8") as handle:
        document = handle.read()
    if not re.search(r"<urlset[\s>]", document):
        sys.stderr.write("not a sitemap urlset: " + shard + "\n")
        sys.exit(3)
    for raw in re.findall(r"<loc>([^<]*)</loc>", document):
        url = html.unescape(raw).strip()
        name = family_of(url)
        if name in members:
            members[name].add(url)

def sample_hash(url):
    return hashlib.sha256((seed + url).encode("utf-8")).hexdigest()

population = ",".join(f"{name}:{len(members[name])}" for name in families)
print(f"# gsc-inspect-sample v1 seed={seed} per_family={per_family} today_utc={today} sitemap={index_url} population={population}")
print("family\turl\thash")
for name in families:
    ranked = sorted((sample_hash(url), url) for url in members[name])
    for digest, url in ranked[:per_family]:
        print(f"{name}\t{url}\t{digest}")
PY

# Renders the summary markdown from a run directory (sample.tsv, raw/,
# failures.tsv). Exits 3 on a malformed sample.tsv or an unparseable raw file.
IFS= read -r -d '' PY_RENDER <<'PY' || true
import collections, json, os, re, sys
run_dir, doc_date, site, quota_project, mode, fixture_dir, stop_reason, all_families = sys.argv[1:9]
FAMILIES = all_families.split()
VERDICTS = ["PASS", "PARTIAL", "FAIL", "NEUTRAL"]
FIELDS = ["verdict", "coverageState", "indexingState", "pageFetchState", "robotsTxtState"]
INVISIBLE = re.compile("[\x00-\x1f\x7f-\x9f\u200b-\u200f\u2028\u2029\u202a-\u202e\u2066-\u2069\ufeff]")

def fail(message):
    sys.stderr.write(message + "\n")
    sys.exit(3)

def esc(value):
    value = INVISIBLE.sub(" ", str(value))
    return value.replace("\\", "\\\\").replace("|", "\\|")

def code(value):
    value = esc(value)
    longest = max([len(run) for run in re.findall("`+", value)] or [0])
    fence = "`" * (longest + 1)
    pad = " " if longest else ""
    return fence + pad + value + pad + fence

with open(os.path.join(run_dir, "sample.tsv"), encoding="utf-8") as handle:
    lines = handle.read().splitlines()
if len(lines) < 2 or not lines[0].startswith("# gsc-inspect-sample v1 ") or lines[1] != "family\turl\thash":
    fail("sample.tsv must start with the gsc-inspect-sample v1 metadata line and the family/url/hash header")
meta = dict(field.split("=", 1) for field in lines[0][len("# gsc-inspect-sample v1 "):].split(" ") if "=" in field)
population = {}
for item in filter(None, meta.get("population", "").split(",")):
    name, _, count = item.partition(":")
    if count.isdigit():
        population[name] = int(count)
rows = []
for number, line in enumerate(lines[2:], start=3):
    cells = line.split("\t")
    if len(cells) != 3 or cells[0] not in FAMILIES or not re.fullmatch("[0-9a-f]{64}", cells[2]):
        fail(f"sample.tsv line {number} is not family, url, sha256 hash")
    rows.append(cells)
if len({row[2] for row in rows}) != len(rows):
    fail("sample.tsv lists a hash twice")
families = [name for name in FAMILIES if name in population or any(row[0] == name for row in rows)]

failures = {}
failures_path = os.path.join(run_dir, "failures.tsv")
if os.path.exists(failures_path):
    with open(failures_path, encoding="utf-8") as handle:
        failure_lines = handle.read().splitlines()
    for line in failure_lines[1:]:
        cells = line.split("\t")
        if len(cells) == 5:
            failures[cells[2]] = {"status": cells[3], "message": cells[4]}

expected = set()
stats = {}
for name in families:
    stats[name] = {"sampled": 0, "inspected": 0, "failed": [], "fields": {f: collections.Counter() for f in FIELDS},
                   "both_canonical": 0, "canonical_differs": 0, "crawl": collections.Counter()}
for name, url, digest in rows:
    family = stats[name]
    family["sampled"] += 1
    path = os.path.join(run_dir, "raw", name, digest + ".json")
    expected.add(os.path.normpath(path))
    result = None
    if os.path.exists(path):
        try:
            with open(path, encoding="utf-8") as handle:
                body = json.load(handle)
        except (OSError, ValueError):
            fail(f"raw response {path} is not readable JSON")
        if isinstance(body, dict) and isinstance(body.get("inspectionResult"), dict):
            result = body["inspectionResult"].get("indexStatusResult") or {}
        elif digest not in failures:
            error = body.get("error") if isinstance(body, dict) else None
            message = error.get("message") if isinstance(error, dict) else None
            failures[digest] = {"status": "response", "message": str(message or "no inspectionResult")}
    if result is None:
        if digest in failures:
            family["failed"].append((url, failures[digest]))
        continue
    family["inspected"] += 1
    for field in FIELDS:
        value = result.get(field)
        family["fields"][field]["(absent)" if value in (None, "") else str(value)] += 1
    google, user = result.get("googleCanonical"), result.get("userCanonical")
    if google and user:
        family["both_canonical"] += 1
        if google != user:
            family["canonical_differs"] += 1
    crawl = result.get("lastCrawlTime")
    family["crawl"][crawl[:7] if isinstance(crawl, str) and re.match("[0-9]{4}-[0-9]{2}", crawl) else "never"] += 1

raw_root = os.path.join(run_dir, "raw")
stray = 0
for directory, _, names in os.walk(raw_root):
    for file_name in names:
        if file_name.endswith(".json") and os.path.normpath(os.path.join(directory, file_name)) not in expected:
            stray += 1
if stray:
    sys.stderr.write(f"gsc-inspect-sample: WARNING: {stray} raw file(s) match no sample row and were not counted\n")

def ordered(counter):
    return sorted(counter.items(), key=lambda item: (-item[1], item[0]))

def advertised(name):
    return str(population[name]) if name in population else "unknown"

out = []
emit = out.append
total_sampled = sum(s["sampled"] for s in stats.values())
total_inspected = sum(s["inspected"] for s in stats.values())
emit(f"# GSC URL Inspection sample, captured {doc_date}")
emit("")
emit(f"**Property:** {code(site)}")
emit(f"**Seed:** {code(meta.get('seed', 'unknown'))} · **Requested per family:** {esc(meta.get('per_family', 'unknown'))} · **Families:** {', '.join(families)}")
emit(f"**Sitemap:** {code(meta.get('sitemap', 'unknown'))}, read {esc(meta.get('today_utc', 'unknown'))}. Shows are split on their slug date against that UTC date; a show dated that day is upcoming.")
emit(f"Source: Search Console URL Inspection API (`urlInspection.index.inspect`), captured by `scripts/gsc-inspect-sample.sh`. The sample is `gsc-inspect-{doc_date}/sample.tsv`; every response body is in `gsc-inspect-{doc_date}/raw/<family>/<hash>.json`.")
emit("")
if mode == "fixture":
    emit(f"> **Fixture render.** Built offline from {code(fixture_dir)}; no API call was made, and these numbers are not production data.")
    emit("")
if stop_reason:
    emit(f"> **INCOMPLETE: the run stopped early ({esc(stop_reason)}).** {total_inspected} of {total_sampled} sampled URLs were inspected; the rest count under Failed or Not inspected. Do not compare these counts with a complete run.")
    emit("")
emit("---")
emit("")
emit("## Read this before quoting any number")
emit("")
emit("1. **A sample, not a census.** Each family table describes the sampled URLs only. At 100 URLs a share carries a sampling margin of up to about 10 percentage points (95% confidence); smaller samples carry more.")
emit("2. **Membership.** The same seed over the same sitemap yields the same URLs. Between runs, membership still moves at the margin: a new sitemap URL whose hash sorts ahead of the cut enters, a removed URL leaves, and an upcoming show becomes a past show when its date passes. Diff the two `sample.tsv` files before attributing a change in counts.")
emit("3. **Requested vs served.** Sampled is the requested size or the family's advertised (sitemap) URL count, whichever is smaller. Inspected + Failed + Not inspected = Sampled. Not inspected means no response was recorded (the run stopped first).")
emit("4. **Field values are Google's own.** `verdict` is the overall index-status verdict; `coverageState` is the reason as the Search Console UI words it, and its wording can change. `(absent)` means the response omitted the field.")
emit("5. **Canonical.** A canonical difference is counted only where the response carries both `googleCanonical` and `userCanonical`; unindexed URLs often carry neither.")
emit("6. **Last crawl.** `lastCrawlTime` is bucketed by UTC month; `never` means the field is absent.")
emit("")
emit("## Roll-up")
emit("")
emit("| Family | Advertised | Sampled | Inspected | Failed | Not inspected | PASS | PARTIAL | FAIL | NEUTRAL | Other verdict | Canonical differs |")
emit("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
def rollup_row(label, advertised_cell, counts):
    # counts: sampled, inspected, failed, missing, the VERDICTS, other,
    # canonical differs, canonical both set.
    cells = [label, advertised_cell] + [str(c) for c in counts[:-2]] + [f"{counts[-2]} of {counts[-1]}"]
    return "| " + " | ".join(cells) + " |"

family_counts = []
for name in families:
    s = stats[name]
    verdicts = s["fields"]["verdict"]
    s["failed_count"] = len(s["failed"])
    s["missing"] = s["sampled"] - s["inspected"] - s["failed_count"]
    other = s["inspected"] - sum(verdicts[v] for v in VERDICTS)
    counts = ([s["sampled"], s["inspected"], s["failed_count"], s["missing"]] + [verdicts[v] for v in VERDICTS]
              + [other, s["canonical_differs"], s["both_canonical"]])
    family_counts.append(counts)
    emit(rollup_row(name, advertised(name), counts))
total_population = str(sum(population[n] for n in families)) if all(n in population for n in families) else "unknown"
emit(rollup_row("**All families**", total_population, [sum(column) for column in zip(*family_counts)] or [0] * (len(VERDICTS) + 7)))
for name in families:
    s = stats[name]
    emit("")
    emit(f"## {name}")
    emit("")
    emit(f"Sampled {s['sampled']} of {advertised(name)} advertised (requested {esc(meta.get('per_family', 'unknown'))}): {s['inspected']} inspected, {s['failed_count']} failed, {s['missing']} not inspected.")
    emit("")
    emit("| Field | Value | URLs |")
    emit("| --- | --- | ---: |")
    if s["inspected"] == 0:
        emit("| _no inspected URLs_ | | |")
    else:
        for field in FIELDS:
            for value, count in ordered(s["fields"][field]):
                emit(f"| {field} | {code(value)} | {count} |")
        emit(f"| canonical | googleCanonical differs from userCanonical | {s['canonical_differs']} of {s['both_canonical']} with both set |")
        for month, count in sorted(s["crawl"].items(), key=lambda item: (item[0] == "never", item[0])):
            emit(f"| lastCrawlTime | {code(month)} | {count} |")
    if s["failed"]:
        emit("")
        emit("| Failed URL | Status | Message |")
        emit("| --- | --- | --- |")
        for url, failure in s["failed"]:
            emit(f"| {code(url)} | {code(failure['status'])} | {esc(failure['message']) or '-'} |")
emit("")
emit("---")
emit("")
emit("## Interpretation")
emit("")
emit("_Fill this in by hand: what moved against the previous run (after diffing sample.tsv), and which family the change belongs to._")
emit("")
emit("## How this was captured")
emit("")
emit("```bash")
if mode == "fixture":
    emit(f"scripts/gsc-inspect-sample.sh --fixture {fixture_dir}")
else:
    emit(f"scripts/gsc-inspect-sample.sh --seed {meta.get('seed', '')} --per-family {meta.get('per_family', '')} --families {','.join(families)}")
emit("```")
if mode != "fixture":
    emit("")
    emit(f"Credential: gcloud Application Default Credential with the webmasters.readonly scope, quota project {code(quota_project)}; the same credential as `scripts/gsc-snapshot.sh`.")
print("\n".join(out))
PY

# --- Sampling -----------------------------------------------------------------

case "$SITEMAP_INDEX" in
  https://*|file://*) ;;
  *) die "GSC_INSPECT_SITEMAP_INDEX must be an https:// or file:// URL (got: ${SITEMAP_INDEX})" ;;
esac

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gsc-inspect-sample.XXXXXX")"
cleanup() { rm -rf "$WORK_DIR"; }
trap cleanup EXIT
RUN_STAGE="$WORK_DIR/run"
mkdir -p "$RUN_STAGE"

TODAY_UTC="$(date -u +%Y-%m-%d)"

# fetch_public <outfile> <url>: the sitemap carries no credential; redirects
# stay on https.
fetch_public() {
  curl -sS --fail --max-time 120 --proto '=https,file' --proto-redir '=https' -o "$1" "$2" \
    || die "could not fetch $2"
}

draw_sample() {
  # $1 = output TSV path
  local shard_list shard index=0
  local shard_files=()
  mkdir -p "$WORK_DIR/sitemap"
  fetch_public "$WORK_DIR/sitemap/index.xml" "$SITEMAP_INDEX"
  shard_list="$WORK_DIR/sitemap/shards.txt"
  python3 -I -c "$PY_INDEX_LOCS" "$SITEMAP_INDEX" "$WORK_DIR/sitemap/index.xml" >"$shard_list" \
    2>"$WORK_DIR/sitemap/index-err" || {
      case $? in
        3) die "$SITEMAP_INDEX is not a sitemap index" ;;
        4) die "sitemap index lists a shard on another scheme or host: $(cat "$WORK_DIR/sitemap/index-err")" ;;
        *) die "could not parse the sitemap index: $(tail -n 1 "$WORK_DIR/sitemap/index-err")" ;;
      esac
    }
  [ -s "$shard_list" ] || die "the sitemap index at $SITEMAP_INDEX lists no shards"
  while IFS= read -r shard <&3; do
    index=$((index + 1))
    fetch_public "$WORK_DIR/sitemap/shard-${index}.xml" "$shard"
    shard_files+=("$WORK_DIR/sitemap/shard-${index}.xml")
  done 3<"$shard_list"
  echo "gsc-inspect-sample: read ${index} sitemap shard(s) from ${SITEMAP_INDEX}" >&2
  python3 -I -c "$PY_SAMPLE" "$TODAY_UTC" "$SEED" "$PER_FAMILY" "$FAMILIES_CSV" "$SITEMAP_INDEX" \
    "${shard_files[@]}" >"$1" || die "could not classify the sitemap URLs"
}

if [ "$SAMPLE_ONLY" -eq 1 ]; then
  draw_sample "$WORK_DIR/sample.tsv"
  cat "$WORK_DIR/sample.tsv"
  exit 0
fi

# --- Output location ------------------------------------------------------------

while [ "${OUT_DIR}" != "${OUT_DIR%/}" ] && [ -n "${OUT_DIR%/}" ]; do
  OUT_DIR="${OUT_DIR%/}"
done
if [ -z "$OUT_DIR" ]; then
  REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" \
    || die "not inside a git repo; pass --out explicitly"
  OUT_DIR="$REPO_ROOT/docs/research"
fi

RUN_NAME="gsc-inspect-${TODAY_UTC}"
DOC_NAME="${RUN_NAME}.md"
# Early check so a mistaken re-run fails before any quota is spent; the
# publication step's mkdir is what closes the race.
for name in "$DOC_NAME" "$RUN_NAME"; do
  if [ -e "$OUT_DIR/$name" ] && [ "$FORCE" -ne 1 ]; then
    die "$OUT_DIR/$name already exists; move it or pass --force to replace it"
  fi
done

# --- Sample and inspect, or load the fixture ------------------------------------

STOP_REASON=""
MODE="live"

if [ -n "$FIXTURE_DIR" ]; then
  MODE="fixture"
  [ -f "$FIXTURE_DIR/sample.tsv" ] || die "fixture $FIXTURE_DIR has no sample.tsv"
  cp "$FIXTURE_DIR/sample.tsv" "$RUN_STAGE/sample.tsv"
  if [ -d "$FIXTURE_DIR/raw" ]; then
    cp -R "$FIXTURE_DIR/raw" "$RUN_STAGE/raw"
  fi
  if [ -f "$FIXTURE_DIR/failures.tsv" ]; then
    cp "$FIXTURE_DIR/failures.tsv" "$RUN_STAGE/failures.tsv"
  fi
else
  # The brew cask installs gcloud outside the default PATH; check its home
  # before giving up.
  GCLOUD="$(command -v gcloud || true)"
  if [ -z "$GCLOUD" ] && [ -x /opt/homebrew/share/google-cloud-sdk/bin/gcloud ]; then
    GCLOUD=/opt/homebrew/share/google-cloud-sdk/bin/gcloud
  fi
  [ -n "$GCLOUD" ] || die "gcloud CLI not found. Install: brew install --cask google-cloud-sdk"

  print_auth_help() {
    printf 'gsc-inspect-sample: (re)authenticate with (narrow read-only scope; try this first):\n' >&2
    printf 'gsc-inspect-sample:   gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly\n' >&2
    printf 'gsc-inspect-sample: if the API then 403s on the quota project, fall back to the broad variant:\n' >&2
    printf 'gsc-inspect-sample:   gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/webmasters.readonly\n' >&2
  }

  draw_sample "$RUN_STAGE/sample.tsv"
  SAMPLED="$(($(wc -l <"$RUN_STAGE/sample.tsv") - 2))"
  echo "gsc-inspect-sample: sampled ${SAMPLED} URL(s) with seed ${SEED} (families: ${FAMILIES_CSV})" >&2

  if ! TOKEN="$("$GCLOUD" auth application-default print-access-token 2>"$WORK_DIR/token-err")"; then
    printf 'gsc-inspect-sample: could not mint an access token from the Application Default Credential.\n' >&2
    sed 's/^/gsc-inspect-sample:   /' "$WORK_DIR/token-err" >&2
    print_auth_help
    exit 1
  fi

  FAILURES_PATH="$RUN_STAGE/failures.tsv"
  printf 'family\turl\thash\tstatus\tmessage\n' >"$FAILURES_PATH"

  # post_inspection <outfile> <json-body>: prints the HTTP status, or nothing
  # and a non-zero exit on a transport error. The credential goes in over
  # stdin as a curl config so it never appears in `ps` output.
  post_inspection() {
    printf 'header = "Authorization: Bearer %s"\n' "$TOKEN" \
      | curl -sS --max-time 60 -o "$1" -w '%{http_code}' \
          --proto '=https' --proto-redir '=https' \
          -X POST -H "x-goog-user-project: ${QUOTA_PROJECT}" \
          -H 'Content-Type: application/json' --data "$2" -K - "$INSPECT_URL"
  }

  # record_failure <family> <url> <hash> <status> <response-file>
  record_failure() {
    local message
    message="$(jq -r '.error.message // empty' "$5" 2>/dev/null || true)"
    [ -n "$message" ] || message="no error message in the response"
    message="$(printf '%s' "$message" | tr '\t\r\n' '   ')"
    printf '%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$message" >>"$FAILURES_PATH"
  }

  # inspect_url <family> <url> <hash>: one inspection with at most one retry.
  # A JSON object returned by the final attempt is kept at raw/<family>/
  # <hash>.json whatever its status. Counts a result in SUCCEEDED; sets
  # STOP_REASON when the run must stop.
  inspect_url() {
    local family url digest body raw part status shape delay attempt=1
    family="$1"
    url="$2"
    digest="$3"
    raw="$RUN_STAGE/raw/$family/$digest.json"
    part="$WORK_DIR/response.json"
    body="$(jq -cn --arg u "$url" --arg s "$SITE" '{inspectionUrl: $u, siteUrl: $s, languageCode: "en-US"}')"
    while :; do
      rm -f "$part" "$raw"
      status="$(post_inspection "$part" "$body")" || status="transport"
      sleep "$REQUEST_INTERVAL_SECONDS"
      # result: an object carrying inspectionResult; object: any other JSON
      # object; none: no body, or not a JSON object.
      shape="$(jq -r 'if type != "object" then "none" elif (.inspectionResult | type) == "object" then "result" else "object" end' \
        "$part" 2>/dev/null || echo none)"
      if [ "$shape" != "none" ]; then
        mv "$part" "$raw"
      fi
      case "$status" in
        200)
          if [ "$shape" = "result" ]; then
            SUCCEEDED=$((SUCCEEDED + 1))
          else
            record_failure "$family" "$url" "$digest" "200 without inspectionResult" "$raw"
          fi
          return 0
          ;;
        401|403)
          record_failure "$family" "$url" "$digest" "$status" "$raw"
          STOP_REASON="HTTP $status from the API: the credential was refused"
          print_auth_help
          return 0
          ;;
        429|5[0-9][0-9]|transport)
          if [ "$attempt" -eq 1 ]; then
            attempt=2
            delay="$BACKOFF_RETRY_SECONDS"
            if [ "$status" = "429" ]; then delay="$BACKOFF_429_SECONDS"; fi
            echo "gsc-inspect-sample: ${status} on ${url}; retrying once in ${delay}s" >&2
            sleep "$delay"
            continue
          fi
          record_failure "$family" "$url" "$digest" "$status" "$raw"
          if [ "$status" = "429" ]; then
            STOP_REASON="a second 429 from the API: the quota is exhausted"
          fi
          return 0
          ;;
        *)
          record_failure "$family" "$url" "$digest" "$status" "$raw"
          return 0
          ;;
      esac
    done
  }

  for family in $FAMILIES; do
    mkdir -p "$RUN_STAGE/raw/$family"
  done

  DONE=0
  SUCCEEDED=0
  while IFS=$'\t' read -r family url digest <&3; do
    inspect_url "$family" "$url" "$digest"
    DONE=$((DONE + 1))
    if [ -n "$STOP_REASON" ]; then
      break
    fi
    if [ $((DONE % 25)) -eq 0 ]; then
      echo "gsc-inspect-sample: ${DONE}/${SAMPLED} inspected" >&2
    fi
  done 3< <(tail -n +3 "$RUN_STAGE/sample.tsv")

  # A stop before any result has nothing worth publishing, and a published
  # empty run would make the next attempt today need --force.
  if [ -n "$STOP_REASON" ] && [ "$SUCCEEDED" -eq 0 ]; then
    die "stopped before any inspection succeeded: ${STOP_REASON}. Nothing was written."
  fi
fi

# --- Render and publish ----------------------------------------------------------

DOC_PATH="$WORK_DIR/$DOC_NAME"
python3 -I -c "$PY_RENDER" "$RUN_STAGE" "$TODAY_UTC" "$SITE" "$QUOTA_PROJECT" "$MODE" "$FIXTURE_DIR" "$STOP_REASON" "$ALL_FAMILIES" \
  >"$DOC_PATH" || die "failed to render the summary"

mkdir -p "$OUT_DIR" || die "could not create output directory: $OUT_DIR"

# The run directory is published before the doc, so a published doc never
# names a missing run directory. mkdir without -p fails when the directory
# appeared mid-run, which refuses the overwrite the early check only narrows.
if [ "$FORCE" -eq 1 ]; then
  rm -rf "${OUT_DIR:?}/${RUN_NAME:?}"
fi
mkdir "$OUT_DIR/$RUN_NAME" 2>/dev/null \
  || die "$OUT_DIR/$RUN_NAME appeared during the run; refusing to overwrite it"
for entry in sample.tsv failures.tsv raw; do
  if [ -e "$RUN_STAGE/$entry" ]; then
    mv "$RUN_STAGE/$entry" "$OUT_DIR/$RUN_NAME/$entry" || die "could not write $entry to $OUT_DIR/$RUN_NAME"
  fi
done
if [ "$FORCE" -eq 1 ]; then
  mv "$DOC_PATH" "$OUT_DIR/$DOC_NAME" || die "could not write the summary to $OUT_DIR"
else
  mv -n "$DOC_PATH" "$OUT_DIR/$DOC_NAME" || die "could not write the summary to $OUT_DIR"
  [ -e "$DOC_PATH" ] && die "$OUT_DIR/$DOC_NAME appeared during the run; refusing to overwrite it"
fi

echo "gsc-inspect-sample: wrote ${OUT_DIR}/${RUN_NAME}/ and ${OUT_DIR}/${DOC_NAME}" >&2
if [ -n "$STOP_REASON" ]; then
  printf 'gsc-inspect-sample: STOPPED EARLY: %s. Partial results were written and marked incomplete.\n' "$STOP_REASON" >&2
  echo "${OUT_DIR}/${DOC_NAME}"
  exit 2
fi
if [ "$MODE" = "live" ]; then
  echo "gsc-inspect-sample: to drop the ADC credential until the next run: gcloud auth application-default revoke" >&2
fi
echo "${OUT_DIR}/${DOC_NAME}"
