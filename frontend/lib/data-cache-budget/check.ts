/**
 * The policy half of the post-build fetch Data Cache scan.
 *
 * WHAT THIS CATCHES, AND WHAT IT CANNOT. Next drops an over-cap entry without
 * writing it, so this scan sees only payloads that still fit — it is the EARLY
 * WARNING half of the gate, catching a fetch on its way to the cap. The failure
 * itself is caught by ./assert.ts at the fetch site. Read ./budget.ts first;
 * it records Next's enforcement code and why the split exists.
 *
 * That limitation is also why this scan is worth having despite the assertion:
 * it needs no list of URLs and no cooperation from the caller, so every cached
 * fetch in the app is covered the day it is written, including ones that never
 * go through the shared list helpers. It found `/sitemap/entries?family=releases`
 * at 97% of the cap on its first run — a route already sharded by family
 * (PSY-1622) whose largest shard had quietly grown to the edge anyway.
 *
 * ONLY THIS BUILD'S ENTRIES ARE JUDGED. Vercel restores `.next/cache` between
 * builds, so an entry written before a fix would otherwise fail every later
 * build forever, including the build that fixes it. Entries older than the
 * current `BUILD_ID` are skipped, which is safe in the direction that matters:
 * a previous build's entry was already judged by that build's gate, and
 * anything this build fetched is fresh and judged now.
 */
import {
  DATA_CACHE_BUDGET_BYTES,
  DATA_CACHE_BUDGET_FRACTION,
  DATA_CACHE_ITEM_LIMIT_BYTES,
  formatMib,
  isWarnBandAllowlisted,
} from './budget'

export { DATA_CACHE_BUDGET_BYTES, DATA_CACHE_BUDGET_FRACTION, DATA_CACHE_ITEM_LIMIT_BYTES }

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
export function findOverBudget(entries: FetchCacheEntry[]): BudgetFailure[] {
  return entries
    .filter(entry => entry.bytes >= DATA_CACHE_BUDGET_BYTES)
    // Recorded warn-band exceptions are reported by the caller, not failed on.
    // Nothing on disk can be over the hard cap — Next never writes those — so
    // every hit here is by definition still inside the warn band.
    .filter(entry => !isWarnBandAllowlisted(entry.url))
    .map(entry => ({ ...entry, fraction: entry.bytes / DATA_CACHE_ITEM_LIMIT_BYTES }))
    .sort((a, b) => b.bytes - a.bytes)
}

/** Warn-band entries that are recorded exceptions, for reporting. */
export function findAllowlisted(entries: FetchCacheEntry[]): FetchCacheEntry[] {
  return entries
    .filter(entry => entry.bytes >= DATA_CACHE_BUDGET_BYTES && isWarnBandAllowlisted(entry.url))
    .sort((a, b) => b.bytes - a.bytes)
}

/**
 * The failure message. It has to teach, because the reader is looking at a red
 * build for a cache limit they have probably never heard of, and the fix is a
 * payload decision rather than a code defect.
 */
export function formatBudgetFailures(failures: BudgetFailure[]): string {
  const lines = failures.map(f => {
    const pct = (f.fraction * 100).toFixed(0)
    return `  ${f.url ?? `(url unreadable) ${f.key}`}\n      ${formatMib(f.bytes)} — ${pct}% of the 2 MB cache-item cap`
  })

  return [
    `Fetch Data Cache budget exceeded by ${failures.length} ${
      failures.length === 1 ? 'entry' : 'entries'
    }.`,
    '',
    ...lines,
    '',
    `Nothing over ${formatMib(DATA_CACHE_ITEM_LIMIT_BYTES)} is cached. Next logs one console.warn and carries on,`,
    'so the fetch keeps working and keeps re-pulling the whole body from origin on',
    'every render — which is how `/artists` went unnoticed for ten days. This gate',
    `fails at ${(DATA_CACHE_BUDGET_FRACTION * 100).toFixed(0)}% of the cap so there is room to react before that happens.`,
    '',
    'Sizes above are the on-disk cache entry, which holds the body BASE64-encoded',
    '(~1.334x). The raw response budget is therefore about 1.5 MB, not 2 MB.',
    '',
    'The fix is to shrink the payload, not to raise the threshold:',
    '  - project the response to the fields the caller actually reads',
    '    (see app/artists/artistsMetadata.ts and GET /artists/listing), or',
    '  - shard the fetch so each entry keys separately (see app/sitemap.ts), or',
    '  - bound it with a limit — but only where dropping rows is acceptable,',
    '    which for a JSON-LD ItemList it generally is not.',
  ].join('\n')
}
