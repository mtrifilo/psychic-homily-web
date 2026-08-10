/**
 * The policy half of the post-build fetch Data Cache scan.
 *
 * WHAT THIS CATCHES, AND WHAT IT CANNOT. Next drops an over-cap entry without
 * writing it, so this scan sees only payloads that still fit — it is the EARLY
 * WARNING half of the gate, catching a fetch on its way to the cap. The failure
 * itself is caught by ./assert.ts at the fetch site. Read ./budget.ts first;
 * it records Next's enforcement code and why the split exists.
 *
 * WHAT IT COVERS, STATED HONESTLY. It needs no list of URLs and no cooperation
 * from the caller, so it sees fetches that never go through the shared list
 * helpers — that is its advantage over the assertion. But it only sees fetches
 * that RUN DURING `next build`, because that is when entries are written. Every
 * entity detail route (`app/artists/[slug]`, `app/tags/[slug]`, `app/venues/[slug]`,
 * `app/scenes/[slug]`, the label / release / festival / collection / radio pages)
 * fetches with `next: { revalidate }` and declares no `generateStaticParams`, so
 * it renders on demand and is invisible here — and none of them call the
 * assertion either. Those routes are UNGUARDED by both halves. They carry the
 * same cap and grow with the catalogue exactly as `/artists` did; `/tags/{slug}`
 * and `/scenes/{slug}` are the plausible next one. Closing that needs a single
 * cached-fetch wrapper every route goes through, which is a change of its own.
 *
 * It found `/sitemap/entries?family=releases` at 97% of the cap on its first
 * run — a route already sharded by family (PSY-1622) whose largest shard had
 * quietly grown to the edge anyway.
 *
 * A SECOND HOLE, ALSO STATED HONESTLY. Vercel restores `.next/cache` between
 * builds and every fetch here has a 1 h window, so a redeploy inside that hour
 * serves every fetch from the restored cache. A Data Cache HIT does not rewrite
 * the entry file, so nothing is fresh and this half weighs nothing at all. It
 * cannot be fixed by measuring differently — the data simply is not regenerated
 * — so ./cli.ts says "MEASURED NOTHING" rather than "OK", and reports every
 * entry on disk regardless of age. What it will not do on such a build is FAIL,
 * which is the deliberate half of the split below.
 *
 * REPORT EVERYTHING, FAIL ON WHAT THIS BUILD WROTE. Because the cache is
 * restored, an entry written before this gate existed can be over the warn line;
 * failing on it would be unfixable, including on the build that fixes it. So age
 * gates the FAILURE, never the REPORT. ./cli.ts takes the age baseline from
 * ./stamp.ts rather than from `.next/BUILD_ID`, whose mtime lands partway
 * through the build and silently aged out everything fetched before it.
 */
import {
  DATA_CACHE_BUDGET_BYTES,
  DATA_CACHE_BUDGET_FRACTION,
  DATA_CACHE_ITEM_LIMIT_BYTES,
  formatMiB,
  isWarnBandAllowlisted,
} from './budget'

export interface FetchCacheEntry {
  /** Cache-key filename, the only handle the build artifact gives us. */
  key: string
  /** Size of the on-disk envelope: the base64 body plus its JSON wrapper. */
  bytes: number
  /** `data.url` from the envelope, when it could be read. */
  url?: string
}

export interface BudgetFailure extends FetchCacheEntry {
  /** Share of the 2 MB cap this entry occupies, e.g. 2.06 for 206%. */
  fraction: number
}

/**
 * Entries at or above the budget, worst first.
 *
 * Exported separately from the I/O so the policy is unit-testable without a
 * build directory — the same split `lib/sitemap-prerender` uses.
 */
export function partitionOverBudget(entries: FetchCacheEntry[]): {
  /** Over the warn line and not excused: these fail the build. */
  failures: BudgetFailure[]
  /** Over the warn line but recorded in WARN_BAND_ALLOWLIST: reported only. */
  allowlisted: BudgetFailure[]
} {
  const overBudget = entries
    .filter(entry => entry.bytes >= DATA_CACHE_BUDGET_BYTES)
    .map(entry => ({ ...entry, fraction: entry.bytes / DATA_CACHE_ITEM_LIMIT_BYTES }))
    .sort((a, b) => b.bytes - a.bytes)

  // The hard cap is enforced here INDEPENDENTLY of the allowlist, mirroring the
  // fetch-site assertion. It would be tempting to assume nothing on disk can be
  // over the cap, since Next refuses to write those — but that is not true:
  // `patch-fetch.js` marks a bare build-time-prerendered fetch
  // `isImplicitBuildTimeCache`, and `incremental-cache/index.js` skips the size
  // check entirely for those, so an over-cap entry CAN land on disk. Waiving
  // that for an allowlisted URL would wave through a route that has already
  // stopped caching — the exact thing the gate exists to catch.
  const breached = (entry: BudgetFailure) => entry.bytes >= DATA_CACHE_ITEM_LIMIT_BYTES

  return {
    failures: overBudget.filter(entry => breached(entry) || !isWarnBandAllowlisted(entry.url)),
    allowlisted: overBudget.filter(entry => !breached(entry) && isWarnBandAllowlisted(entry.url)),
  }
}

/**
 * The failure message. It has to teach, because the reader is looking at a red
 * build for a cache limit they have probably never heard of, and the fix is a
 * payload decision rather than a code defect.
 */
export function formatBudgetFailures(failures: BudgetFailure[]): string {
  // Most red builds are WARN-band, because the gate fires at 80% of the cap.
  // Telling that reader their payload "is not cached and re-pulls on every
  // render" describes a production regression that has not happened yet — and
  // makes the break-glass look far costlier than it is. Each line says which it
  // is, and the prose below follows the worst one present.
  const anyBreached = failures.some(f => f.bytes >= DATA_CACHE_ITEM_LIMIT_BYTES)

  const lines = failures.map(f => {
    const pct = (f.fraction * 100).toFixed(0)
    const label = f.bytes >= DATA_CACHE_ITEM_LIMIT_BYTES ? 'BREACH' : 'WARN  '
    return `  ${label} ${f.url ?? `(url unreadable) ${f.key}`}\n      ${formatMiB(f.bytes)} — ${pct}% of the 2 MB cache-item cap`
  })

  const explanation = anyBreached
    ? [
        `BREACH means over ${formatMiB(DATA_CACHE_ITEM_LIMIT_BYTES)}, which is NOT CACHED AT ALL. Next logs one`,
        'console.warn and carries on, so the fetch keeps working and keeps re-pulling the',
        'whole body from origin on every render — which is how `/artists` went unnoticed',
        'for ten days.',
      ]
    : [
        `WARN means still cached, but past ${(DATA_CACHE_BUDGET_FRACTION * 100).toFixed(0)}% of the ${formatMiB(DATA_CACHE_ITEM_LIMIT_BYTES)} cap. Nothing is`,
        'broken in production yet. This gate fires early ON PURPOSE: past the cap an entry',
        'stops being cached silently, and `/artists` went from 73% to 206% in under two',
        'weeks of ordinary catalogue growth.',
      ]

  return [
    `Fetch Data Cache budget exceeded by ${failures.length} ${
      failures.length === 1 ? 'entry' : 'entries'
    }.`,
    '',
    ...lines,
    '',
    ...explanation,
    '',
    'Sizes above are the on-disk cache entry, which holds the body BASE64-encoded',
    '(~1.334x). The raw response budget is therefore about 1.5 MB, not 2 MB.',
    '',
    'The fix is to shrink the payload, not to raise the threshold:',
    '  - project the response to the fields the caller actually reads',
    '    (see app/artists/artistsMetadata.ts and GET /artists/listing), or',
    '  - shard the fetch so each entry keys separately (see app/sitemap.ts), or',
    '  - bound it with a limit — but only where dropping rows is acceptable.',
    '    For a JSON-LD ItemList that depends on whether the dropped URLs are',
    '    reachable another way: /artists bounds its block at 100 because every',
    '    artist is in the /sitemap/artists.xml shard regardless (PSY-1773), and',
    '    what a bound costs there is enrichment, not discoverability. Where no',
    '    sitemap shard covers the family, a bound DOES drop URLs — check before',
    '    reaching for one.',
    '',
    // None of the remedies above can be done under hotfix pressure, and this
    // condition is usually triggered by DATA rather than by the commit being
    // deployed. Naming the override here is the difference between a gate and
    // a deadlock; the cost is stated so it is never the quiet default.
    'BREAK-GLASS, if you need to ship something unrelated right now:',
    '  DATA_CACHE_BUDGET_ENFORCE=warn bun run build',
    anyBreached
      ? 'That ships a route which is NOT cached and re-pulls this payload from origin on every render.'
      : 'The payloads above are still cached today, so this mainly spends the warning margin.',
    'It buys time; it does not fix anything. Shrink the payload.',
  ].join('\n')
}
