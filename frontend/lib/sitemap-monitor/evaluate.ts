/**
 * The pure half of the monitor: given what was observed, decide whether the
 * sitemap is healthy.
 *
 * Split from the fetching deliberately. The decision logic is where a wrong
 * threshold or an inverted comparison hides, and it is only testable in
 * isolation if it does no I/O. fetch.ts gathers, this file judges.
 */

import { shardIdsFor, SITEMAP_FAMILIES, type Family } from '@/app/sitemap-shards'
import type { MonitorConfig } from './config'
import type { SitemapShape } from './parse'

/** One served count against the count the API reports for the same thing. */
export interface Comparison {
  /** `<loc>` count in the served sitemap. */
  observed: number
  /** Entry count the API reports. */
  expected: number
  /** observed - expected. Negative means the sitemap is missing URLs. */
  delta: number
  /** The largest |delta| tolerated under the current config. */
  allowed: number
  ok: boolean
  /** The API has entries but the sitemap serves none — a failure at any tolerance. */
  vanished: boolean
}

export interface FamilyComparison extends Comparison {
  family: Family
}

/**
 * One sub-shard's served count against what the API says it should hold.
 *
 * Only sub-shards are compared here. A family served by a single document is
 * already covered by its FamilyComparison, and comparing it twice would print
 * the same drift as two failures. A document that was never fetched is absent
 * from this list, because the walk already reported why.
 */
export interface ShardComparison extends Comparison {
  shard: string
  family: Family
}

export interface SampleResult {
  url: string
  /** Final status after redirects, or null when the request never completed. */
  status: number | null
  ok: boolean
  error?: string
}

export interface EvaluationInput {
  shape: SitemapShape
  /** How many child documents the index listed. Zero for the single-doc shape. */
  shardCount: number
  observedByFamily: Record<Family, number>
  observedPages: number
  /** `<loc>`s matching no known family — informational, never a failure. */
  observedOther: number
  expectedByFamily: Record<Family, number>
  /** `<loc>` count per shard document actually fetched from the sitemap. */
  observedByShard: ReadonlyMap<string, number>
  /** Entry count the API reports for each entity shard document. */
  expectedByShard: ReadonlyMap<string, number>
  /** Shard ids the API says it does not serve. */
  unservedShards: readonly string[]
  /** Shows dated today or later, by slug date. */
  futureShowCount: number
  samples: SampleResult[]
  /** Transport/shape problems gathered while fetching. Any entry fails the run. */
  errors: string[]
}

export interface Report {
  ok: boolean
  target: string
  shape: SitemapShape
  shardCount: number
  families: FamilyComparison[]
  /** Per-document comparisons, for the sub-sharded families only. */
  shards: ShardComparison[]
  observedPages: number
  observedOther: number
  observedTotal: number
  expectedTotal: number
  futureShows: { observed: number; required: number; ok: boolean }
  samples: SampleResult[]
  /**
   * One line per failed assertion, with the numbers that failed it.
   *
   * Gathering errors from `EvaluationInput` are folded in here rather than kept
   * as a separate field, so a formatter cannot render the same problem twice or
   * miss half of it.
   */
  failures: string[]
}

/**
 * The drift budget for one family.
 *
 * `max` of the two bounds, not `min`: the ratio governs large families where a
 * fixed count is meaninglessly tight, and the floor governs small ones where a
 * percentage is meaninglessly tight. Taking the larger is what keeps both ends
 * of the range quiet.
 */
export function allowedDrift(expected: number, config: MonitorConfig): number {
  return Math.max(config.driftFloor, Math.ceil(expected * config.driftRatio))
}

/**
 * Score one served count against one expected count.
 *
 * `vanished` is separate from the tolerance because the absolute floor exists
 * to keep SMALL sets quiet and that also makes them unmonitorable: `festivals`
 * has 10 entries and the default floor is 10, so the whole family disappearing
 * would sit exactly inside budget and report green. Total disappearance is the
 * defect class this monitor exists to catch, so it fails at any tolerance.
 */
function compare(observed: number, expected: number, config: MonitorConfig): Comparison {
  const delta = observed - expected
  const allowed = allowedDrift(expected, config)
  const vanished = expected > 0 && observed === 0
  return {
    observed,
    expected,
    delta,
    allowed,
    ok: !vanished && Math.abs(delta) <= allowed,
    vanished,
  }
}

function compareFamilies(
  input: EvaluationInput,
  config: MonitorConfig
): FamilyComparison[] {
  return SITEMAP_FAMILIES.map(family => ({
    family,
    ...compare(input.observedByFamily[family] ?? 0, input.expectedByFamily[family] ?? 0, config),
  }))
}

/**
 * Compare every sub-shard document against the count the API reports for it.
 *
 * This is what covers the loss a family comparison cannot see: one document of
 * a sub-sharded family going dark costs a fraction of its family, and whether
 * that fraction clears `driftRatio` is an accident of the bucket count rather
 * than something a check should depend on. Per document, an empty answer where
 * the API has rows is a failure at any tolerance and names which document.
 *
 * Single-document families are deliberately skipped: their FamilyComparison is
 * the same comparison, and running both would report one problem twice.
 *
 * A document missing from either side is skipped rather than scored: the walk
 * reports why it was not fetched, and an absent expected count is reported by
 * `unservedShards` or by fetchExpectedCounts throwing. Inventing a number for
 * either would produce a failure line whose own figures say nothing failed.
 */
function compareShards(
  input: EvaluationInput,
  config: MonitorConfig
): ShardComparison[] {
  const comparisons: ShardComparison[] = []
  for (const family of SITEMAP_FAMILIES) {
    const ids = shardIdsFor(family)
    if (ids.length < 2) continue
    for (const shard of ids) {
      const observed = input.observedByShard.get(shard)
      if (observed === undefined) continue
      const expected = input.expectedByShard.get(shard)
      if (expected === undefined) continue
      comparisons.push({ shard, family, ...compare(observed, expected, config) })
    }
  }
  return comparisons
}

/** `shows: sitemap 1458 vs API 1462 (4 missing, tolerance ±292)`. */
function describeDrift(label: string, c: Comparison): string {
  const direction = c.delta < 0 ? 'missing' : 'extra'
  return `${label}: sitemap ${c.observed} vs API ${c.expected} (${Math.abs(c.delta)} ${direction}, tolerance ±${c.allowed})`
}

export function evaluate(input: EvaluationInput, config: MonitorConfig): Report {
  const families = compareFamilies(input, config)
  const shards = compareShards(input, config)
  const futureShows = {
    observed: input.futureShowCount,
    required: config.minFutureShows,
    ok: input.futureShowCount >= config.minFutureShows,
  }

  // ORDERED BY CLASS, WORST FIRST, because this list is rendered into a Discord
  // field that truncates at MAX_FIELD_VALUE characters (characters, not bytes:
  // these lines carry em dashes and ± signs). Proportional drift
  // trips a family and every one of its documents at once (buckets hold equal
  // shares), so the drift classes are the repetitive ones and go last; a
  // vanished document is one line and names thousands of missing URLs, so it
  // must not be the line that gets cut.
  const failures: string[] = []

  for (const error of input.errors) {
    failures.push(`fetch: ${error}`)
  }

  for (const shard of input.unservedShards) {
    failures.push(
      `unserved — shard ${shard}: the API does not serve this id, so its URLs are announced by nobody`
    )
  }

  for (const comparison of families) {
    if (comparison.vanished) {
      failures.push(
        `vanished — ${comparison.family}: the API has ${comparison.expected} entries and the sitemap serves NONE`
      )
    }
  }

  for (const comparison of shards) {
    if (comparison.vanished) {
      failures.push(
        `vanished — shard ${comparison.shard}: the API has ${comparison.expected} entries and the document serves NONE`
      )
    }
  }

  if (!futureShows.ok) {
    failures.push(
      `stale — only ${futureShows.observed} upcoming show URLs (need ≥ ${futureShows.required}); ` +
        'a sitemap of purely historical shows is the signature of the original incident'
    )
  }

  for (const comparison of families) {
    if (!comparison.vanished && !comparison.ok) {
      failures.push(`drift — ${describeDrift(comparison.family, comparison)}`)
    }
  }

  const badSamples = input.samples.filter(sample => !sample.ok)
  for (const sample of badSamples) {
    failures.push(`unreachable — ${sample.url} → ${sample.error ?? sample.status}`)
  }

  for (const comparison of shards) {
    if (!comparison.vanished && !comparison.ok) {
      failures.push(`drift — ${describeDrift(`shard ${comparison.shard}`, comparison)}`)
    }
  }

  return {
    ok: failures.length === 0,
    target: config.target,
    shape: input.shape,
    shardCount: input.shardCount,
    families,
    shards,
    observedPages: input.observedPages,
    observedOther: input.observedOther,
    observedTotal:
      families.reduce((sum, c) => sum + c.observed, 0) +
      input.observedPages +
      input.observedOther,
    expectedTotal: families.reduce((sum, c) => sum + c.expected, 0),
    futureShows,
    samples: input.samples,
    failures,
  }
}

/**
 * Draw at least one URL from EVERY bucket, `size` in total where possible.
 *
 * Sampling uniformly from all URLs pooled together would make the reachability
 * probe a releases-only check: measured on 2026-09-03, releases is 28,720 emitted slugs of
 * the ~57k entity URLs, and the small families are hundreds each, so a 10-URL
 * sample expects 0.05 scene weeks and would probe the newest routes
 * essentially never. A broken `/scenes/{city}/{iso-week}` route could ship and
 * stay green for weeks.
 *
 * Guarantees one probe per non-empty bucket, so the per-bucket quota rises
 * above one only when `size` exceeds the bucket count.
 */
export function pickStratifiedSample(
  locsByBucket: ReadonlyMap<string, string[]>,
  size: number,
  rng: () => number = Math.random
): string[] {
  const buckets = [...locsByBucket.values()].filter(locs => locs.length > 0)
  if (buckets.length === 0 || size <= 0) return []

  const perBucket = Math.max(1, Math.floor(size / buckets.length))
  return buckets.flatMap(locs => pickSample(locs, perBucket, rng))
}

/**
 * Choose `size` entries at random, without replacement.
 *
 * Distinct by POSITION, not by value: if the same URL appears twice in the
 * pool it can be drawn twice. Deduping is not worth the pass — a duplicate loc
 * is itself a sitemap defect the count check would surface.
 *
 * The RNG is injected so the sampling is deterministic under test. Sampling
 * randomly rather than taking the first N matters: the first entries of every
 * shard are the oldest rows, so a fixed head would probe the same handful of
 * long-lived URLs forever and never notice a newly-broken one.
 */
export function pickSample<T>(items: readonly T[], size: number, rng: () => number = Math.random): T[] {
  if (size <= 0 || items.length === 0) return []
  if (size >= items.length) return [...items]

  const pool = [...items]
  const picked: T[] = []
  for (let i = 0; i < size; i++) {
    // Guard against an rng returning exactly 1, which would index past the end
    // and push undefined into the sample.
    const index = Math.min(Math.floor(rng() * pool.length), pool.length - 1)
    picked.push(pool[index])
    // Swap-with-last then pop, rather than splice: same draw-without-replacement
    // semantics in O(1) instead of an O(n) memmove over a 33k-element array on
    // every pick.
    pool[index] = pool[pool.length - 1]
    pool.pop()
  }
  return picked
}

/**
 * Count shows dated on or after `today`.
 *
 * Compared as ISO date strings, which sorts correctly for `YYYY-MM-DD` and
 * sidesteps timezone drift entirely — a Date-based comparison would flip
 * results depending on the runner's zone for shows dated today.
 */
export function countFutureShows(showDates: readonly string[], today: string): number {
  return showDates.filter(date => date >= today).length
}

/** `YYYY-MM-DD` in UTC. The runner's local zone must not change the verdict. */
export function isoDate(now: Date): string {
  return now.toISOString().slice(0, 10)
}
