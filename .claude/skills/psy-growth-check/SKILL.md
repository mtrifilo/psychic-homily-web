---
name: psy-growth-check
description: Read how Psychic Homily's search growth and analytics are trending, the same way every time. Use when the user asks "how is growth trending", "check analytics", "did the SEO work move anything", "run the monthly read", or before and after a promote that touches indexing, sitemaps, limiters or server rendering. Runs the paired Search Console and Vercel snapshots over one window, reads the per-family breadth table and the zero-click queue diff, compares the URL Inspection sample, does the Search Console UI reads that have no API, then writes a dated record and says what moved against the last read. Encodes the traps: raw Vercel totals are crawler-dominated, Search Console lags three days, PostHog is consent-gated, records are gitignored and must never be overwritten, and the scripts must run from a checkout at origin/main.
argument-hint: "[--since YYYY-MM-DD --until YYYY-MM-DD] [--sample docs/research/gsc-inspect-<date>/sample.tsv]"
---

# psy-growth-check: the growth and analytics read

One recipe, so two reads a month apart are comparable. The canonical measures are **Search Console** (impressions, clicks, distinct pages shown per family) and the **Google organic** figure of the Vercel snapshot (google.com-referred visitor-days summed from its daily table). Never quote raw Vercel totals as growth: they are dominated by credential probing on `/auth` and long-tail crawlers (`pattern_analytics_consent_blind_spots.md`, `pattern_vercel_analytics_api.md` in the memory directory `~/.claude/projects/-Users-mtrifilo-dev-psychic-homily-web/memory/`). Strategy context: `project_growth_and_return.md` there and `docs/research/growth-and-return-2026-09.md`. Previous records: `docs/research/growth-trends-<date>.md`.

## 0. Pick the window

- Default: the last 28 served days. Search Console serves data two to three days late, so `--until` is today minus 3. Both scripts default to that, but pass `--since` and `--until` explicitly so the pair is reproducible and survives a date rollover between the two runs.
- Keep windows comparable with the previous record: same length, and read weeks Monday to Sunday (the breadth table only counts complete weeks; your weekly sums must drop partial weeks the same way).
- Reporting changes that invalidate comparisons: Search Console impressions were misreported 2025-05-13 to 2026-04-27 (Google's Data anomalies page); never compare across that boundary. Google's Search Status Dashboard lists ranking updates; name any that overlap the window.

## 1. Run the paired snapshots from a checkout at origin/main

The main checkout on this machine is often stale and dirty, and the scripts gain columns over time (the per-family breadth table shipped 2026-10-24), so run them from a throwaway detached worktree at `origin/main`, with `--out` pointing at the main checkout's `docs/research` (gitignored, absent from fresh worktrees).

```bash
REPO=/Users/mtrifilo/dev/psychic-homily-web
OUT=$REPO/docs/research
WT=$REPO/.claude/worktrees/growth-check-$(date +%Y%m%d)
git -C "$REPO" fetch -q origin && git -C "$REPO" worktree add --detach -q "$WT" origin/main

bash "$WT/scripts/gsc-snapshot.sh"     --since <since> --until <until> --out "$OUT"
zsh -lc "source ~/.zshrc >/dev/null 2>&1; bash '$WT/scripts/traffic-snapshot.sh' --since <since> --until <until> --out '$OUT'"

git -C "$REPO" worktree remove "$WT"     # after the read
```

- `gsc-snapshot.sh` needs the gcloud Application Default Credential with the `webmasters.readonly` scope and the quota project `psychic-homily` (its header prints the two login commands on failure). It writes `gsc-snapshot-<since>_<until>.md` and the zero-click queue `gsc-zero-click-<since>_<until>.tsv`, and diffs that queue against the most recent same-length queue in `--out` ending before `--until` (it must have its paired doc; windows of a different length produce no diff).
- `traffic-snapshot.sh` needs `VERCEL_API_KEY` in the environment (it lives in `~/.zshrc`, hence the `zsh -lc` wrapper). It writes `traffic-snapshot-<since>_<until>.md`.
- **Never pass `--force` by habit.** Both scripts refuse to overwrite an existing window because the records are gitignored and exist only on this machine; `gsc-snapshot.sh` overwrites in place, taking the hand-filled Interpretation and Page indexing sections and the zero-click TSV with it. If a window already exists, read it instead. Re-run with `--force` only when the existing doc's Interpretation section is still the empty placeholder and you have said so in your notes.
- Run both in the background and read the docs after; each takes a few minutes.

## 2. What to read in the two docs

From `gsc-snapshot-<since>_<until>.md`:
- **Headline totals** (clicks, impressions, CTR, position) against the previous window's headline.
- **Breadth by family** (one table per complete Monday week). Rows are named `show: dated AFTER window` (shows dated after that week: the upcoming-show family), `show: dated IN window`, `show: dated BEFORE window`, `show: numeric id`, `/artists/`, `/releases/`, `/venues/`, `/scenes/`, `/radio`, `/festivals/`, `/labels/`, and `other: <segment>`. Pages is distinct URLs with at least one impression; quote each family's page count beside its advertised URL count from the sitemap index (the doc's trap 7), and do not add breadth impressions up as site totals (trap 8). Breadth is the leading indicator: in October 2026 the fall was breadth, not CTR.
- **Zero-click queries** (average position under 11, at least 5 impressions, 0 clicks) and the **queue diff**: which pairs entered or left since the previous same-length window. City-intent queries landing on dated show pages were the 2026-10 finding behind PSY-2529.
- **Top pages**: whether the landing pages are evergreen (scene pages, home, indexes) or dated show pages that will die.
- Fill the doc's **Interpretation** and **Page indexing (manual read)** sections by hand (section 4 below supplies the numbers); the script leaves placeholders.

From `traffic-snapshot-<since>_<until>.md`:
- **Google organic**: the headline there ("N google.com-referred visitors", summed from its daily table) is the number to quote, and its daily table is what you sum into Monday weeks. The referrer table's `google.com` row can differ by a few visitors; do not mix the two series between reads.
- `duckduckgo.com` and `bing.com` in the referrer table are the other engines; `accounts.google.com` is sign-in, not search.
- Read the doc's own traps before quoting anything: country segments at exactly 1.0 pages per visitor are crawlers, a stealth Android crawler sweeps entity pages around the clock, and per-visit pageview fingerprints above the PSY-1863 cap are automation.
- Fill its **Interpretation** section.

Compute the weekly series with Python over the two daily tables rather than by eye, cutting weeks Monday to Sunday and reporting partial weeks as "(n days)":

```python
import re, datetime, collections
def daily(path, heading):
    rows, on = [], False
    for line in open(path):
        if line.startswith(heading): on = True; continue
        if on and line.startswith('## '): break
        m = re.match(r'\|\s*(\d{4}-\d{2}-\d{2})\s*\|(.*)', line)
        if on and m: rows.append((m.group(1), [c.strip() for c in m.group(2).strip().strip('|').split('|')]))
    return rows
def monday(d): dt = datetime.date.fromisoformat(d); return (dt - datetime.timedelta(days=dt.weekday())).isoformat()
# traffic: daily('traffic-snapshot-...md', '## Google organic') -> cells [visitors, pageviews]
# gsc:     daily('gsc-snapshot-...md', '## Daily series')      -> cells [clicks, impressions, ctr, position]
```

The 2026-10-31 record (`growth-trends-2026-10-31.md`) shows the resulting table shape.

## 3. Index state: the URL Inspection sample

`scripts/gsc-inspect-sample.sh` inspects 100 sitemap URLs per family through the URL Inspection API (quota 2,000 a day, so at most three runs a day; about 70 minutes for 600 URLs; results are kept on interrupt) and writes `gsc-inspect-<date>.md` plus `gsc-inspect-<date>/sample.tsv` and `raw/`. It needs the same gcloud credential as the snapshot, so it runs in the session that holds it.

- **Baseline:** `docs/research/gsc-inspect-2026-10-26.md` (207 of 600 indexed; upcoming shows 22 of 100; 23 to 36 per family "URL is unknown to Google").
- **Comparison:** re-inspect the baseline's URLs so membership churn does not masquerade as movement:
  ```bash
  bash "$WT/scripts/gsc-inspect-sample.sh" --sample "$OUT/gsc-inspect-2026-10-26/sample.tsv" --out "$OUT"
  ```
  Read the show families of an old sample as ageing URLs: within weeks every "upcoming-shows" URL in it is a past show, and once the sitemap diet drops past shows older than 30 days many leave the sitemap entirely, so a fall in that family's indexed count is ageing, not a regression. Set a new baseline (a fresh draw, no `--sample`) when the diet flags flip and at least quarterly, and say which baseline a comparison used.
- A fresh draw on the same UTC day as a comparison would claim the same `gsc-inspect-<date>` name; give it its own `--out` directory or run it another day.
- Timing: two weeks after a promote that changes indexing or crawling, and before the next such promote.

## 4. Search Console UI reads that have no API

Read these in the owner's logged-in Chrome (the claude-in-chrome tools; one tab, close it after). Page indexing numbers go into the snapshot doc's "Page indexing (manual read)" section with the report's own "Last update" date, not the day you read it; the rest go into the record.

| Report | URL | Record |
| --- | --- | --- |
| Page indexing | `https://search.google.com/search-console/index?resource_id=sc-domain%3Apsychichomily.com` | Indexed, Not indexed, "Discovered, currently not indexed", "Crawled, currently not indexed", and the report's Last update date |
| Crawl stats | `https://search.google.com/search-console/settings/crawl-stats?resource_id=sc-domain%3Apsychichomily.com` | total requests, per host (`psychichomily.com`, `api.psychichomily.com`), by purpose (refresh versus discovery), by file type, by Googlebot type; drill into a host or file type for examples and the time chart |
| Enhancements | overview page, Enhancements block | Events valid and invalid counts |
| Manual actions and security | left nav | must be empty |

Search appearance by API (which rich results earn impressions), with the same credential the snapshot uses:

```bash
TOKEN="$(gcloud auth application-default print-access-token)"
curl -sS -H "Authorization: Bearer $TOKEN" -H "x-goog-user-project: psychic-homily" -H "Content-Type: application/json" \
  -X POST "https://searchconsole.googleapis.com/webmasters/v3/sites/sc-domain%3Apsychichomily.com/searchAnalytics/query" \
  -d '{"startDate":"<since>","endDate":"<until>","dimensions":["searchAppearance"],"rowLimit":50}'
```

Do not write the token into a file that outlives the command.

## 5. PostHog (consent-gated, human slice)

PostHog project 308106 (`growth-trends-2026-10-23.md` section 7) is readable only through the owner's logged-in Chrome session; no personal API key exists. Only visitors who accepted cookies appear, so quote it as behaviour of the consenting slice (bounce, session length, paths), never as traffic. "Filter test accounts" is usually off; the owner's own devices inflate promote-and-verify days.

## 6. Write the record and move the memory

- Record: `docs/research/growth-trends-<YYYY-MM-DD>.md` with the weekly tables, the comparison against the previous record, what was promoted inside the window (the release log memory `project_release_log.md` has every promote), and a "checks owed" list. Pair files are the two snapshot docs and the inspection doc.
- Memory: append a dated paragraph to `project_growth_and_return.md` with the numbers that change a decision, and point `MEMORY.md`'s growth line at the new record.
- Linear: offer the owner a project update on "Growth Measurement & Activation (Aug 2026)" and post it only on their word (an update is a message to shared state): `linear project-update create bdfaf914-4acd-4e28-b5b6-55a8b62aacc2 --body-file <path> --health onTrack|atRisk|offTrack`.

## 7. Interpretation rules that have held

- Breadth before CTR: count pages shown per family per week before reading snippets.
- A dated show page that ranks for a city query is a landing page that will die; the evergreen scene pages (PSY-2529) are where that intent should land.
- Depth near 1.0 means the landing page does not carry the second click; it is a layout and link-graph question, not a content one.
- Position moving while CTR is flat means the set of pages ranking changed, not the titles.
- Attribute nothing to a promote until two weekly reads after it, and name every Google update that overlaps.
- Numbers go in tables; speculation is marked as such; sources and capture dates sit next to every figure.
