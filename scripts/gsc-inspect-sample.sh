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
#   scene-weeks     /scenes/<slug>/YYYY-Www (not the /scenes/<slug> landing)
#
# A slug date that is not a real calendar day, a URL with a query string, a
# fragment, whitespace or a control character, and every other path belong to
# no family. These families are narrower than the breadth families of
# gsc-snapshot.sh, whose /venues/ and /scenes/ rows count every path under the
# prefix. The path prefixes track FAMILY_URL_PREFIXES in
# frontend/app/sitemap-shards.ts; a selected family that matches no sitemap URL
# stops the run, because that means the URL shapes moved. Each family is
# ordered by sha256(seed + url) (hex, then URL) and the first N are kept.
#
# MEMBERSHIP: a fresh draw over the same sitemap with the same seed always
# yields the same sample, and a different seed re-draws every family. A fresh
# draw on a later day also moves with the sitemap: a new URL whose hash sorts
# ahead of the cut enters, a removed URL leaves, and an upcoming show becomes
# a past show once its date passes. To re-inspect exactly the URLs of an
# earlier run, pass that run's sample.tsv with --sample: membership and family
# labels are then fixed until a fresh draw is made on purpose.
#
# QUOTA
# -----
# The URL Inspection API allows 2000 calls a day and 600 a minute per
# property; other runs on the same day share the daily quota. A plan above
# 2000 inspections is refused. Each request is followed by a pause, so request
# starts are at least REQUEST_INTERVAL_SECONDS apart. A 429, a 5xx, or a
# transport error is retried once after a backoff, and a 401 once after the
# access token is minted again. The run stops on a 429 to both attempts for
# one URL, a 401 to a fresh token, a 403, CONSECUTIVE_FAILURE_LIMIT failed URLs
# in a row, or SIGINT, SIGTERM or SIGHUP. A stopped run writes what it has and
# is marked incomplete. A run in which no inspection succeeded writes nothing. If the
# script dies after an inspection succeeded but before publishing, the staged
# results are kept and their directory is printed.
#
# OUTPUT
# ------
#   <out>/gsc-inspect-<date>.md                       the summary
#   <out>/gsc-inspect-<date>/sample.tsv               metadata line, header,
#                                                     then family, url, hash
#   <out>/gsc-inspect-<date>/raw/<family>/<hash>.json every JSON response body
#   <out>/gsc-inspect-<date>/raw/<family>/<hash>.body a non-JSON response body
#   <out>/gsc-inspect-<date>/failures.tsv             every URL that failed
#
# Each raw file holds the final attempt's body. <date> is the UTC capture date.
# The final stdout line is the summary's path.
#
# Usage:
#   scripts/gsc-inspect-sample.sh [--out DIR] [--per-family N] [--families LIST]
#                                 [--seed STRING] [--force]
#   scripts/gsc-inspect-sample.sh --sample FILE [--out DIR] [--force]
#   scripts/gsc-inspect-sample.sh --sample-only [--per-family N] [--families LIST] [--seed STRING]
#   scripts/gsc-inspect-sample.sh --fixture DIR --out DIR [--force]
#
#   --out DIR         Directory to write into. Default: docs/research relative
#                     to the repo root. docs/ is gitignored and absent from
#                     fresh worktrees, so pass --out explicitly elsewhere.
#                     Required with --fixture, so a fixture render can never
#                     take the dated name of a real run.
#   --per-family N    URLs to sample per family, 1 to 2000. Default: 100.
#   --families LIST   Comma-separated subset of the six families above.
#                     Default: all six. Output keeps the order above.
#   --seed STRING     Sampling seed, [A-Za-z0-9._:-], up to 64 characters.
#                     Default: gsc-inspect-v1. Recorded in sample.tsv and in
#                     the summary.
#   --sample FILE     Inspect the URLs of an earlier run's sample.tsv instead
#                     of drawing from the sitemap. Takes no sampling flags.
#   --sample-only     Print a fresh sample as TSV on stdout and exit. Fetches
#                     the sitemap only: no credential, no API call, no files.
#   --fixture DIR     Offline render: read DIR/sample.tsv, DIR/raw/<family>/
#                     <hash>.json and DIR/failures.tsv (optional) instead of
#                     fetching and inspecting, then write the summary and a
#                     copy of those files as a normal run would. Takes no
#                     sampling flags. scripts/testdata/gsc-inspect/ is a
#                     hand-written example.
#   --force           Replace an existing summary and run directory for this
#                     date (refused by default). The replaced pair is moved
#                     aside with a .replaced-<UTC time> suffix, not deleted.
#
# Exit status: 0 on a complete run, 2 when the run stopped early and partial
# results were written, 1 on any other error (including a run in which no
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
# Requires: bash, curl, jq, python3; for an inspecting run, the gcloud CLI
# holding an Application Default Credential that includes the
# webmasters.readonly scope (the same credential scripts/gsc-snapshot.sh
# uses). scripts/gsc-inspect-sample_test.sh is the self-test.

set -euo pipefail

# --- Configuration ----------------------------------------------------------

SITE="${GSC_SITE:-sc-domain:psychichomily.com}"
# ADC user credentials must name a quota project or the API rejects them.
QUOTA_PROJECT="${GSC_QUOTA_PROJECT:-psychic-homily}"
SITEMAP_INDEX="${GSC_INSPECT_SITEMAP_INDEX:-https://psychichomily.com/sitemap-index}"
# Not overridable: the bearer token is attached to every request to this URL.
INSPECT_URL="https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"

DEFAULT_SEED="gsc-inspect-v1"
DAILY_QUOTA=2000
# 60 / 0.11 < 600: at most 545 request starts fit in any minute.
REQUEST_INTERVAL_SECONDS=0.11
# Longer than a minute, so a 429 on the retry is not the per-minute quota.
BACKOFF_429_SECONDS=61
BACKOFF_RETRY_SECONDS=5
CONSECUTIVE_FAILURE_LIMIT=10

PER_FAMILY=100
FAMILIES_ARG=""
SEED="$DEFAULT_SEED"
OUT_DIR=""
FIXTURE_DIR=""
SAMPLE_FILE=""
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
  scripts/gsc-inspect-sample.sh --sample FILE [--out DIR] [--force]
  scripts/gsc-inspect-sample.sh --sample-only [--per-family N] [--families LIST] [--seed STRING]
  scripts/gsc-inspect-sample.sh --fixture DIR --out DIR [--force]

  --out DIR         Output directory. Default: docs/research in the repo root.
                    Required with --fixture.
  --per-family N    URLs sampled per family, 1 to 2000. Default: 100.
  --families LIST   Comma-separated subset of: upcoming-shows, past-shows,
                    artists, releases, venues, scene-weeks. Default: all six.
  --seed STRING     Sampling seed ([A-Za-z0-9._:-], up to 64). Default:
                    gsc-inspect-v1.
  --sample FILE     Re-inspect the URLs of an earlier run's sample.tsv; a
                    fresh draw moves with the sitemap (see the script header).
  --sample-only     Print a fresh sample TSV on stdout; no credential, no API
                    call.
  --fixture DIR     Render offline from DIR/sample.tsv and DIR/raw/.
  --force           Replace today's summary and run directory; the old pair is
                    moved aside with a .replaced-<UTC time> suffix.

Exit status: 0 complete, 2 stopped early with partial results written,
1 any other error. The script header documents the families, membership,
the quota handling and the environment overrides.
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

# --- Python programs ----------------------------------------------------------

# Shared by every Python program below, which are run as "$PY_LIB$PY_<name>".
#
# FAMILY_ORDER is the one list of family names; the shell reads it from here.
# family_of is the classifier. A URL with whitespace or a control character
# anywhere belongs to no family, because urlsplit silently drops tab, CR and LF
# and the URL would then be inspected in a different form than it was hashed.
#
# load_sample validates a sample.tsv as a whole (metadata line, header, three
# cells per row, a known family, a 64-hex hash, no hash twice, no unsafe
# character in a URL) and returns (meta, population, rows), or exits 3 with
# the reason.
#
# Sitemap documents are read with a regex over <loc> elements, entity-decoded,
# rather than with xml.etree, so the script does not depend on pyexpat, which
# is not loadable in every Python build.
IFS= read -r -d '' PY_LIB <<'PY' || true
import datetime, hashlib, html, re, sys, urllib.parse
FAMILY_ORDER = ["upcoming-shows", "past-shows", "artists", "releases", "venues", "scene-weeks"]
UNSAFE = re.compile("[\x00-\x20\x7f-\x9f\u2028\u2029]")
LOC = re.compile(r"<loc>([^<]*)</loc>")
SHOW = re.compile(r"/shows/([0-9]{4}-[0-9]{2}-[0-9]{2})-[^/]+")
ENTITY = re.compile(r"/(artists|releases|venues)/[^/]+")
SCENE_WEEK = re.compile(r"/scenes/[^/]+/[0-9]{4}-W[0-9]{2}")
SAMPLE_META = "# gsc-inspect-sample v1 "
SAMPLE_HEADER = "family\turl\thash"

def family_of(url, today):
    if UNSAFE.search(url):
        return None
    parts = urllib.parse.urlsplit(url)
    if parts.scheme not in ("http", "https") or parts.query or parts.fragment:
        return None
    match = SHOW.fullmatch(parts.path)
    if match:
        try:
            datetime.date.fromisoformat(match.group(1))
        except ValueError:
            return None
        return "upcoming-shows" if match.group(1) >= today else "past-shows"
    match = ENTITY.fullmatch(parts.path)
    if match:
        return match.group(1)
    if SCENE_WEEK.fullmatch(parts.path):
        return "scene-weeks"
    return None

def locs(document):
    return [html.unescape(raw).strip() for raw in LOC.findall(document)]

def load_sample(path):
    def fail(reason):
        sys.stderr.write(f"{path}: {reason}\n")
        sys.exit(3)
    try:
        with open(path, encoding="utf-8", newline="") as handle:
            text = handle.read()
    except (OSError, UnicodeDecodeError) as error:
        fail(f"cannot be read ({error})")
    if "\r" in text:
        fail("has CR line endings; convert it to LF")
    lines = text.split("\n")
    if lines and lines[-1] == "":
        lines.pop()
    if len(lines) < 2 or not lines[0].startswith(SAMPLE_META) or lines[1] != SAMPLE_HEADER:
        fail("must start with the gsc-inspect-sample v1 metadata line and the family/url/hash header")
    meta = dict(f.split("=", 1) for f in lines[0][len(SAMPLE_META):].split(" ") if "=" in f)
    population = {}
    for item in filter(None, meta.get("population", "").split(",")):
        name, _, count = item.partition(":")
        if count.isdigit():
            population[name] = int(count)
    rows = []
    for number, line in enumerate(lines[2:], start=3):
        cells = line.split("\t")
        if (len(cells) != 3 or cells[0] not in FAMILY_ORDER or UNSAFE.search(cells[1])
                or not re.fullmatch("[0-9a-f]{64}", cells[2])):
            fail(f"line {number} is not a known family, a URL without whitespace, and a sha256 hash")
        rows.append(cells)
    if len({row[2] for row in rows}) != len(rows):
        fail("lists a hash twice")
    return meta, population, rows
PY

# Prints the family names, space-separated, in output order.
IFS= read -r -d '' PY_FAMILIES <<'PY' || true
print(" ".join(FAMILY_ORDER))
PY

# Prints the <loc> of every entry in a sitemap index, one per line. Exits 3
# when the document is not a sitemap index, 4 when a shard is not on the
# index's own scheme and host or carries an unsafe character.
IFS= read -r -d '' PY_INDEX_LOCS <<'PY' || true
index_url, path = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as handle:
    document = handle.read()
if not re.search(r"<sitemapindex[\s>]", document):
    sys.exit(3)
def origin(url):
    parts = urllib.parse.urlsplit(url)
    return (parts.scheme.lower(), parts.netloc.lower())
for loc in locs(document):
    if not loc:
        continue
    if UNSAFE.search(loc) or origin(loc) != origin(index_url):
        sys.stderr.write(repr(loc) + "\n")
        sys.exit(4)
    print(loc)
PY

# Classifies every URL in the shard files and prints the sample as TSV: the
# metadata line, the column header, then family, url, hash rows in family
# order and hash order within a family. Exits 5 naming any selected family
# that matched no URL.
IFS= read -r -d '' PY_SAMPLE <<'PY' || true
today, seed, per_family, families, index_url = sys.argv[1:6]
shards = sys.argv[6:]
per_family = int(per_family)
families = families.split(",")
members = {name: set() for name in families}
unsafe = 0
for shard in shards:
    with open(shard, encoding="utf-8") as handle:
        document = handle.read()
    if not re.search(r"<urlset[\s>]", document):
        sys.stderr.write("not a sitemap urlset: " + shard + "\n")
        sys.exit(3)
    for url in locs(document):
        if UNSAFE.search(url):
            unsafe += 1
            continue
        name = family_of(url, today)
        if name in members:
            members[name].add(url)
if unsafe:
    sys.stderr.write(f"gsc-inspect-sample: skipped {unsafe} sitemap URL(s) holding whitespace or a control character\n")
empty = [name for name in families if not members[name]]
if empty:
    sys.stderr.write(", ".join(empty) + "\n")
    sys.exit(5)

def sample_hash(url):
    return hashlib.sha256((seed + url).encode("utf-8")).hexdigest()

population = ",".join(f"{name}:{len(members[name])}" for name in families)
print(f"{SAMPLE_META}seed={seed} per_family={per_family} today_utc={today} sitemap={index_url} population={population}")
print(SAMPLE_HEADER)
for name in families:
    ranked = sorted((sample_hash(url), url) for url in members[name])
    for digest, url in ranked[:per_family]:
        print(f"{name}\t{url}\t{digest}")
PY

# Validates a sample.tsv, writes its rows as LF-terminated family, url, hash
# lines to the second argument, and prints the row count.
IFS= read -r -d '' PY_CHECK_SAMPLE <<'PY' || true
rows = load_sample(sys.argv[1])[2]
with open(sys.argv[2], "w", encoding="utf-8") as out:
    for row in rows:
        out.write("\t".join(row) + "\n")
print(len(rows))
PY

# Renders the summary markdown from a run directory (sample.tsv, raw/,
# failures.tsv). Exits 3 on a malformed sample.tsv or an unparseable raw JSON
# file.
IFS= read -r -d '' PY_RENDER <<'PY' || true
import collections, json, os
run_dir, doc_date, site, quota_project, mode, source, stop_reason = sys.argv[1:8]
VERDICTS = ["PASS", "PARTIAL", "FAIL", "NEUTRAL"]
FIELDS = ["verdict", "coverageState", "indexingState", "pageFetchState", "robotsTxtState"]
INVISIBLE = re.compile("[\x00-\x1f\x7f-\x9f\u200b-\u200f\u2028\u2029\u202a-\u202e\u2066-\u2069\ufeff]")

def esc(value):
    value = INVISIBLE.sub(" ", str(value))
    return value.replace("\\", "\\\\").replace("|", "\\|")

def code(value):
    value = esc(value)
    longest = max([len(run) for run in re.findall("`+", value)] or [0])
    fence = "`" * (longest + 1)
    pad = " " if longest else ""
    return fence + pad + value + pad + fence

meta, population, rows = load_sample(os.path.join(run_dir, "sample.tsv"))
families = [name for name in FAMILY_ORDER if name in population or any(row[0] == name for row in rows)]

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
            sys.stderr.write(f"raw response {path} is not readable JSON\n")
            sys.exit(3)
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

stray = 0
for directory, _, names in os.walk(os.path.join(run_dir, "raw")):
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
emit(f"**Sitemap:** {code(meta.get('sitemap', 'unknown'))}, read {esc(meta.get('today_utc', 'unknown'))}. Shows were split on their slug date against that UTC date; a show dated that day is upcoming.")
emit(f"Source: Search Console URL Inspection API (`urlInspection.index.inspect`), captured by `scripts/gsc-inspect-sample.sh`. The sample is `gsc-inspect-{doc_date}/sample.tsv`; every response body is in `gsc-inspect-{doc_date}/raw/<family>/`.")
emit("")
if mode == "fixture":
    emit(f"> **Fixture render.** Built offline from {code(source)}; no API call was made, and these numbers are not production data.")
    emit("")
if mode == "reuse":
    emit(f"> **Re-inspected sample.** The URLs and family labels are those of {code(source)}, drawn on {esc(meta.get('today_utc', 'unknown'))}, not a fresh draw from today's sitemap; Advertised is that draw's count.")
    emit("")
if stop_reason:
    emit(f"> **INCOMPLETE: the run stopped early ({esc(stop_reason)}).** {total_inspected} of {total_sampled} sampled URLs were inspected; the rest count under Failed or Not inspected. Do not compare these counts with a complete run.")
    emit("")
emit("---")
emit("")
emit("## Read this before quoting any number")
emit("")
emit("1. **A sample, not a census.** Each family table describes the sampled URLs only. At 100 URLs a share carries a sampling margin of up to about 10 percentage points (95% confidence); smaller samples carry more.")
emit("2. **Membership.** A fresh draw with the same seed over the same sitemap yields the same URLs, but between days it moves at the margin: a new sitemap URL whose hash sorts ahead of the cut enters, a removed URL leaves, and an upcoming show becomes a past show when its date passes. To compare like with like, re-inspect an earlier run's URLs with `--sample <that run>/sample.tsv`, or diff the two `sample.tsv` files before attributing a change in counts.")
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
    # counts: sampled, inspected, failed, not inspected, the VERDICTS, other,
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
    emit(f"scripts/gsc-inspect-sample.sh --fixture {source} --out <dir>")
elif mode == "reuse":
    emit(f"scripts/gsc-inspect-sample.sh --sample {source}")
else:
    emit(f"scripts/gsc-inspect-sample.sh --seed {meta.get('seed', '')} --per-family {meta.get('per_family', '')} --families {','.join(families)}")
emit("```")
if mode != "fixture":
    emit("")
    emit(f"Credential: gcloud Application Default Credential with the webmasters.readonly scope, quota project {code(quota_project)}; the same credential as `scripts/gsc-snapshot.sh`.")
print("\n".join(out))
PY

# --- Argument parsing -------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --out)         require_value --out "${2:-}"; OUT_DIR="$2"; shift 2 ;;
    --per-family)  require_value --per-family "${2:-}"; PER_FAMILY="$2"; SAMPLING_FLAG_GIVEN="$1"; shift 2 ;;
    --families)    require_value --families "${2:-}"; FAMILIES_ARG="$2"; SAMPLING_FLAG_GIVEN="$1"; shift 2 ;;
    --seed)        require_value --seed "${2:-}"; SEED="$2"; SAMPLING_FLAG_GIVEN="$1"; shift 2 ;;
    --sample)      require_value --sample "${2:-}"; SAMPLE_FILE="$2"; shift 2 ;;
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

if [ -n "$FIXTURE_DIR" ] && [ -n "$SAMPLE_FILE" ]; then
  die "--fixture and --sample cannot be combined"
fi
if [ -n "$FIXTURE_DIR" ] || [ -n "$SAMPLE_FILE" ]; then
  if [ -n "$SAMPLING_FLAG_GIVEN" ]; then
    die "--fixture and --sample use an existing sample and take no sampling flags (got $SAMPLING_FLAG_GIVEN)"
  fi
fi
if [ -n "$FIXTURE_DIR" ] && [ -z "$OUT_DIR" ]; then
  die "--fixture requires --out, so a fixture render cannot take the name of a real run"
fi

# The length test comes first so no arithmetic sees an overflowing value.
case "$PER_FAMILY" in
  ''|*[!0-9]*) die "--per-family must be a positive integer, got '$PER_FAMILY'" ;;
esac
[ "${#PER_FAMILY}" -le 4 ] || die "--per-family must be at most ${DAILY_QUOTA}"
# Leading zeros would otherwise be read as octal by $(( )).
PER_FAMILY=$((10#$PER_FAMILY))
[ "$PER_FAMILY" -gt 0 ] || die "--per-family must be greater than zero"
[ "$PER_FAMILY" -le "$DAILY_QUOTA" ] || die "--per-family must be at most ${DAILY_QUOTA}"

case "$SEED" in
  *[!A-Za-z0-9._:-]*) die "--seed may contain only letters, digits, '.', '_', ':' and '-'" ;;
esac
[ "${#SEED}" -le 64 ] || die "--seed must be at most 64 characters"

ALL_FAMILIES="$(python3 -I -c "$PY_LIB$PY_FAMILIES")" || die "could not read the family list"

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
if [ -z "$FIXTURE_DIR" ] && [ -z "$SAMPLE_FILE" ] && [ "$SAMPLE_ONLY" -ne 1 ] && [ "$PLANNED" -gt "$DAILY_QUOTA" ]; then
  die "${FAMILY_COUNT} families x ${PER_FAMILY} URLs plans ${PLANNED} inspections, above the API's ${DAILY_QUOTA}-a-day quota per property"
fi

case "$SITEMAP_INDEX" in
  https://*) SITEMAP_PROTO='=https' ;;
  file://*)  SITEMAP_PROTO='=file' ;;
  *) die "GSC_INSPECT_SITEMAP_INDEX must be an https:// or file:// URL (got: ${SITEMAP_INDEX})" ;;
esac

# --- Staging ------------------------------------------------------------------

# Everything is staged here and moved into OUT_DIR only at the end. Once an
# inspection has succeeded, PRESERVE_STAGE is set and an exit that has not
# published keeps the staged results instead of deleting quota already spent.
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gsc-inspect-sample.XXXXXX")"
RUN_STAGE="$WORK_DIR/run"
mkdir -p "$RUN_STAGE"
PRESERVE_STAGE=0
cleanup() {
  if [ "$PRESERVE_STAGE" -eq 1 ]; then
    printf 'gsc-inspect-sample: the run did not publish; its staged results are kept in %s\n' "$RUN_STAGE" >&2
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

TODAY_UTC="$(date -u +%Y-%m-%d)"

# fetch_public <outfile> <url>: the sitemap carries no credential. The protocol
# is the index's own, redirects stay on https, and -g keeps curl from
# expanding {} and [] in a URL into many requests.
fetch_public() {
  curl -sS --fail -g --max-time 120 --proto "$SITEMAP_PROTO" --proto-redir '=https' -o "$1" "$2" \
    || die "could not fetch $2"
}

draw_sample() {
  # $1 = output TSV path
  local shard_list shard index=0 status=0
  local shard_files=()
  mkdir -p "$WORK_DIR/sitemap"
  fetch_public "$WORK_DIR/sitemap/index.xml" "$SITEMAP_INDEX"
  shard_list="$WORK_DIR/sitemap/shards.txt"
  python3 -I -c "$PY_LIB$PY_INDEX_LOCS" "$SITEMAP_INDEX" "$WORK_DIR/sitemap/index.xml" >"$shard_list" \
    2>"$WORK_DIR/sitemap/index-err" || status=$?
  case "$status" in
    0) ;;
    3) die "$SITEMAP_INDEX is not a sitemap index" ;;
    4) die "the sitemap index lists a shard on another scheme or host, or with whitespace or a control character: $(cat "$WORK_DIR/sitemap/index-err")" ;;
    *) die "could not parse the sitemap index: $(tail -n 1 "$WORK_DIR/sitemap/index-err")" ;;
  esac
  [ -s "$shard_list" ] || die "the sitemap index at $SITEMAP_INDEX lists no shards"
  while IFS= read -r shard <&3; do
    index=$((index + 1))
    fetch_public "$WORK_DIR/sitemap/shard-${index}.xml" "$shard"
    shard_files+=("$WORK_DIR/sitemap/shard-${index}.xml")
  done 3<"$shard_list"
  echo "gsc-inspect-sample: read ${index} sitemap shard(s) from ${SITEMAP_INDEX}" >&2
  status=0
  python3 -I -c "$PY_LIB$PY_SAMPLE" "$TODAY_UTC" "$SEED" "$PER_FAMILY" "$FAMILIES_CSV" "$SITEMAP_INDEX" \
    "${shard_files[@]}" >"$1" 2>"$WORK_DIR/sitemap/sample-err" || status=$?
  grep -v '^gsc-inspect-sample: ' "$WORK_DIR/sitemap/sample-err" >"$WORK_DIR/sitemap/sample-reason" || true
  grep '^gsc-inspect-sample: ' "$WORK_DIR/sitemap/sample-err" >&2 || true
  case "$status" in
    0) ;;
    5) die "no sitemap URL matched these families, so the URL shapes may have moved (compare FAMILY_URL_PREFIXES in frontend/app/sitemap-shards.ts): $(tr '\n' ' ' <"$WORK_DIR/sitemap/sample-reason")" ;;
    *) die "could not classify the sitemap URLs: $(tail -n 1 "$WORK_DIR/sitemap/sample-reason")" ;;
  esac
}

# check_sample <file> <rows-out>: prints the row count of a valid sample.tsv
# and writes its rows to <rows-out>, or dies with the reason.
check_sample() {
  python3 -I -c "$PY_LIB$PY_CHECK_SAMPLE" "$1" "$2" 2>"$WORK_DIR/check-err" \
    || die "invalid sample: $(tail -n 1 "$WORK_DIR/check-err")"
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
# Checked before any quota is spent; the publication step's mkdir is what
# closes the race.
for name in "$DOC_NAME" "$RUN_NAME"; do
  if [ -e "$OUT_DIR/$name" ] && [ "$FORCE" -ne 1 ]; then
    die "$OUT_DIR/$name already exists; move it or pass --force to replace it"
  fi
done
if ! mkdir -p -- "$OUT_DIR" 2>/dev/null || [ ! -w "$OUT_DIR" ]; then
  die "cannot write to the output directory $OUT_DIR"
fi

# --- Sample and inspect, or load the fixture ------------------------------------

STOP_REASON=""
MODE="live"
SOURCE=""

if [ -n "$FIXTURE_DIR" ]; then
  MODE="fixture"
  SOURCE="$FIXTURE_DIR"
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

  if [ -n "$SAMPLE_FILE" ]; then
    MODE="reuse"
    SOURCE="$SAMPLE_FILE"
    [ -f "$SAMPLE_FILE" ] || die "--sample $SAMPLE_FILE is not a file"
    cp "$SAMPLE_FILE" "$RUN_STAGE/sample.tsv"
  else
    draw_sample "$RUN_STAGE/sample.tsv"
  fi
  # Every row is validated before the first request, so a malformed sample
  # never spends quota.
  ROWS_PATH="$WORK_DIR/rows.tsv"
  SAMPLED="$(check_sample "$RUN_STAGE/sample.tsv" "$ROWS_PATH")"
  [ "$SAMPLED" -le "$DAILY_QUOTA" ] \
    || die "the sample holds ${SAMPLED} URLs, above the API's ${DAILY_QUOTA}-a-day quota per property"
  echo "gsc-inspect-sample: ${SAMPLED} URL(s) to inspect ($([ "$MODE" = reuse ] && echo "reused from $SAMPLE_FILE" || echo "seed $SEED, families $FAMILIES_CSV"))" >&2

  # mint_token: sets TOKEN from the Application Default Credential.
  mint_token() {
    TOKEN="$("$GCLOUD" auth application-default print-access-token 2>"$WORK_DIR/token-err")"
  }
  if ! mint_token; then
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

  # record_failure <family> <url> <hash> <status>: the message comes from the
  # URL's raw JSON body when there is one.
  record_failure() {
    local raw_base message=""
    raw_base="$RUN_STAGE/raw/$1/$3"
    if [ -f "$raw_base.json" ]; then
      message="$(jq -r '.error.message // empty' "$raw_base.json" 2>/dev/null || true)"
    elif [ -f "$raw_base.body" ]; then
      message="non-JSON response body, kept as raw/$1/$3.body"
    fi
    [ -n "$message" ] || message="no error message in the response"
    message="$(printf '%s' "$message" | tr '\t\r\n' '   ')"
    printf '%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$message" >>"$FAILURES_PATH"
  }

  # inspect_url <family> <url> <hash>: one inspection with at most one retry
  # after a 429, 5xx or transport error and at most one fresh token after a
  # 401. The final attempt's body is kept at raw/<family>/<hash>.json when it
  # is a JSON object and at raw/<family>/<hash>.body otherwise. Counts a result
  # in SUCCEEDED and returns 0; returns 1 for a failure. Sets STOP_REASON when
  # the run must stop. An interrupt keeps a result already received; otherwise
  # it returns 1 with no raw file and nothing recorded, so the URL counts as
  # not inspected.
  inspect_url() {
    local family url digest body raw part status shape delay first="" attempt=1 reminted=0
    family="$1"
    url="$2"
    digest="$3"
    raw="$RUN_STAGE/raw/$family/$digest"
    part="$WORK_DIR/response"
    body="$(jq -cn --arg u "$url" --arg s "$SITE" '{inspectionUrl: $u, siteUrl: $s, languageCode: "en-US"}')"
    while :; do
      rm -f "$part" "$raw.json" "$raw.body"
      status="$(post_inspection "$part" "$body")" || status="transport"
      sleep "$REQUEST_INTERVAL_SECONDS"
      # result: an object carrying inspectionResult; object: any other JSON
      # object; none: anything else, including no body.
      shape="$(jq -r 'if type != "object" then "none" elif (.inspectionResult | type) == "object" then "result" else "object" end' \
        "$part" 2>/dev/null || echo none)"
      if [ "$INTERRUPTED" -ne 0 ]; then
        # A result already received is kept; anything else is dropped.
        if [ "$status" = "200" ] && [ "$shape" = "result" ] && mv "$part" "$raw.json"; then
          SUCCEEDED=$((SUCCEEDED + 1))
          return 0
        fi
        return 1
      fi
      if [ "$shape" = "result" ] || [ "$shape" = "object" ]; then
        mv "$part" "$raw.json" || shape="unsaved"
      elif [ -s "$part" ]; then
        mv "$part" "$raw.body" || true
      fi
      case "$status" in
        200)
          if [ "$shape" = "result" ]; then
            SUCCEEDED=$((SUCCEEDED + 1))
            return 0
          fi
          if [ "$shape" = "unsaved" ]; then
            record_failure "$family" "$url" "$digest" "200 but the body could not be saved"
          else
            record_failure "$family" "$url" "$digest" "200 without inspectionResult"
          fi
          return 1
          ;;
        401)
          # An access token outlives about an hour at most; a long run can
          # outlast it, so the first 401 for a URL earns a fresh token and a
          # retry.
          if [ "$reminted" -eq 0 ]; then
            reminted=1
            if mint_token; then
              echo "gsc-inspect-sample: 401 on ${url}; minted a fresh access token and retrying" >&2
              continue
            fi
            record_failure "$family" "$url" "$digest" "$status"
            STOP_REASON="HTTP 401 from the API, and a fresh access token could not be minted: $(tr '\n' ' ' <"$WORK_DIR/token-err")"
            print_auth_help
            return 1
          fi
          record_failure "$family" "$url" "$digest" "$status"
          STOP_REASON="HTTP 401 from the API with a freshly minted token: the credential was refused"
          print_auth_help
          return 1
          ;;
        403)
          record_failure "$family" "$url" "$digest" "$status"
          STOP_REASON="HTTP 403 from the API: the credential was refused"
          print_auth_help
          return 1
          ;;
        429|5[0-9][0-9]|transport)
          if [ "$attempt" -eq 1 ]; then
            attempt=2
            first="$status"
            delay="$BACKOFF_RETRY_SECONDS"
            if [ "$status" = "429" ]; then delay="$BACKOFF_429_SECONDS"; fi
            echo "gsc-inspect-sample: ${status} on ${url}; retrying once in ${delay}s" >&2
            sleep "$delay"
            if [ "$INTERRUPTED" -ne 0 ]; then
              rm -f "$raw.json" "$raw.body"
              return 1
            fi
            continue
          fi
          record_failure "$family" "$url" "$digest" "$status"
          if [ "$status" = "429" ] && [ "$first" = "429" ]; then
            STOP_REASON="429 on both attempts for one URL, a minute apart: the daily quota is exhausted"
          fi
          return 1
          ;;
        *)
          record_failure "$family" "$url" "$digest" "$status"
          return 1
          ;;
      esac
    done
  }

  for family in $ALL_FAMILIES; do
    mkdir -p "$RUN_STAGE/raw/$family"
  done

  # SIGINT, SIGTERM or SIGHUP ends the loop after the URL in flight; the run
  # then publishes what it has.
  INTERRUPTED=0
  trap 'INTERRUPTED=1' INT TERM HUP
  DONE=0
  SUCCEEDED=0
  FAILED_IN_A_ROW=0
  while IFS=$'\t' read -r family url digest <&3; do
    if inspect_url "$family" "$url" "$digest"; then
      FAILED_IN_A_ROW=0
      PRESERVE_STAGE=1
    else
      FAILED_IN_A_ROW=$((FAILED_IN_A_ROW + 1))
    fi
    DONE=$((DONE + 1))
    if [ "$INTERRUPTED" -eq 1 ] && [ -z "$STOP_REASON" ]; then
      STOP_REASON="interrupted by a signal"
    fi
    if [ -z "$STOP_REASON" ] && [ "$FAILED_IN_A_ROW" -ge "$CONSECUTIVE_FAILURE_LIMIT" ]; then
      STOP_REASON="${CONSECUTIVE_FAILURE_LIMIT} URLs in a row failed"
    fi
    if [ -n "$STOP_REASON" ]; then
      break
    fi
    if [ $((DONE % 25)) -eq 0 ]; then
      echo "gsc-inspect-sample: ${DONE}/${SAMPLED} attempted, ${SUCCEEDED} inspected" >&2
    fi
  done 3<"$ROWS_PATH"
  trap - INT TERM HUP
  if [ -z "$STOP_REASON" ] && [ "$DONE" -ne "$SAMPLED" ]; then
    STOP_REASON="the loop read ${DONE} of ${SAMPLED} sample rows"
  fi

  # A run with no result has nothing worth publishing, and a published empty
  # run would make the next attempt today need --force.
  if [ "$SUCCEEDED" -eq 0 ]; then
    printf 'gsc-inspect-sample: no inspection succeeded; the first failures:\n' >&2
    tail -n +2 "$FAILURES_PATH" | head -n 5 | cut -f2,4,5 | sed 's/^/gsc-inspect-sample:   /' >&2
    die "no inspection succeeded${STOP_REASON:+ (stopped: $STOP_REASON)}. Nothing was written."
  fi
fi

# --- Render and publish ----------------------------------------------------------

DOC_PATH="$WORK_DIR/$DOC_NAME"
python3 -I -c "$PY_LIB$PY_RENDER" "$RUN_STAGE" "$TODAY_UTC" "$SITE" "$QUOTA_PROJECT" "$MODE" "$SOURCE" "$STOP_REASON" \
  >"$DOC_PATH" 2>"$WORK_DIR/render-err" || die "failed to render the summary: $(tail -n 1 "$WORK_DIR/render-err")"
grep '^gsc-inspect-sample: ' "$WORK_DIR/render-err" >&2 || true

# The run directory is published before the doc, so a published doc never
# names a missing run directory. mkdir without -p fails when the directory
# appeared mid-run, which refuses the overwrite the early check only narrows.
# --force moves a replaced run and its doc aside rather than deleting them, so
# a stopped run never destroys a complete one.
if [ "$FORCE" -eq 1 ]; then
  REPLACED_SUFFIX="replaced-$(date -u +%Y%m%dT%H%M%SZ)"
  for name in "$RUN_NAME" "$DOC_NAME"; do
    if [ -e "$OUT_DIR/$name" ]; then
      mv "$OUT_DIR/$name" "$OUT_DIR/$name.$REPLACED_SUFFIX" \
        || die "could not move the existing $OUT_DIR/$name aside"
      echo "gsc-inspect-sample: moved the existing $name aside to $name.$REPLACED_SUFFIX" >&2
    fi
  done
fi
mkdir "$OUT_DIR/$RUN_NAME" 2>/dev/null \
  || die "could not create $OUT_DIR/$RUN_NAME: it appeared during the run, or $OUT_DIR stopped being writable"
for entry in sample.tsv failures.tsv raw; do
  if [ -e "$RUN_STAGE/$entry" ]; then
    mv "$RUN_STAGE/$entry" "$OUT_DIR/$RUN_NAME/$entry" || die "could not write $entry to $OUT_DIR/$RUN_NAME"
  fi
done
mv -n "$DOC_PATH" "$OUT_DIR/$DOC_NAME" || die "could not write the summary to $OUT_DIR"
[ -e "$DOC_PATH" ] && die "$OUT_DIR/$DOC_NAME appeared during the run; refusing to overwrite it"
PRESERVE_STAGE=0

echo "gsc-inspect-sample: wrote ${OUT_DIR}/${RUN_NAME}/ and ${OUT_DIR}/${DOC_NAME}" >&2
if [ -n "$STOP_REASON" ]; then
  printf 'gsc-inspect-sample: STOPPED EARLY: %s. Partial results were written and marked incomplete.\n' "$STOP_REASON" >&2
  echo "${OUT_DIR}/${DOC_NAME}"
  exit 2
fi
if [ "$MODE" != "fixture" ]; then
  echo "gsc-inspect-sample: to drop the ADC credential until the next run: gcloud auth application-default revoke" >&2
fi
echo "${OUT_DIR}/${DOC_NAME}"
