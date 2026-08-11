/**
 * The pure half of the monitor: given what was observed, decide whether the
 * sitemap is healthy.
 *
 * Split from the fetching deliberately. The decision logic is where a wrong
 * threshold or an inverted comparison hides, and it is only testable in
 * isolation if it does no I/O. fetch.ts gathers, this file judges.
 */

import { SITEMAP_FAMILIES, type Family } from '@/app/sitemap-shards'
import type { MonitorConfig } from './config'
import type { SitemapShape } from './parse'

export interface FamilyComparison {
  family: Family
  /** `<loc>` count for this family in the served sitemap. */
  observed: number
  /** Entry count the API reports for this family. */
  expected: number
  /** observed - expected. Negative means the sitemap is missing URLs. */
  delta: number
  /** The largest |delta| tolerated for this family under the current config. */
  allowed: number
  ok: boolean
  /** The API has entries but the sitemap serves none — a failure at any tolerance. */
  vanished: boolean
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

function compareFamilies(
  input: EvaluationInput,
  config: MonitorConfig
): FamilyComparison[] {
  return SITEMAP_FAMILIES.map(family => {
    const observed = input.observedByFamily[family] ?? 0
    const expected = input.expectedByFamily[family] ?? 0
    const delta = observed - expected
    const allowed = allowedDrift(expected, config)
    // The absolute floor exists to keep SMALL families quiet, but it also makes
    // them unmonitorable: `festivals` has 10 entries and the default floor is
    // 10, so the whole family disappearing would sit exactly inside budget and
    // report green. Total disappearance is the defect class this monitor exists
    // to catch, so it fails regardless of tolerance.
    const vanished = expected > 0 && observed === 0
    return {
      family,
      observed,
      expected,
      delta,
      allowed,
      ok: !vanished && Math.abs(delta) <= allowed,
      vanished,
    }
  })
}

function describeDrift(c: FamilyComparison): string {
  const direction = c.delta < 0 ? 'missing' : 'extra'
  return `${c.family}: sitemap ${c.observed} vs API ${c.expected} (${Math.abs(c.delta)} ${direction}, tolerance ±${c.allowed})`
}

export function evaluate(input: EvaluationInput, config: MonitorConfig): Report {
  const families = compareFamilies(input, config)
  const futureShows = {
    observed: input.futureShowCount,
    required: config.minFutureShows,
    ok: input.futureShowCount >= config.minFutureShows,
  }

  const failures: string[] = []

  for (const error of input.errors) {
    failures.push(`fetch: ${error}`)
  }

  for (const comparison of families) {
    if (comparison.vanished) {
      failures.push(
        `vanished — ${comparison.family}: the API has ${comparison.expected} entries and the sitemap serves NONE`
      )
    } else if (!comparison.ok) {
      failures.push(`drift — ${describeDrift(comparison)}`)
    }
  }

  if (!futureShows.ok) {
    failures.push(
      `stale — only ${futureShows.observed} upcoming show URLs (need ≥ ${futureShows.required}); ` +
        'a sitemap of purely historical shows is the signature of the original incident'
    )
  }

  const badSamples = input.samples.filter(sample => !sample.ok)
  for (const sample of badSamples) {
    failures.push(`unreachable — ${sample.url} → ${sample.error ?? sample.status}`)
  }

  return {
    ok: failures.length === 0,
    target: config.target,
    shape: input.shape,
    shardCount: input.shardCount,
    families,
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
 * probe a releases-only check: releases is ~20k of the ~33k URLs, so a 10-URL
 * sample expects 0.44 shows and would probe the highest-churn families — shows,
 * and the newest routes, scene weeks — essentially never. A broken
 * `/scenes/{city}/{iso-week}` route could ship and stay green for weeks.
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
