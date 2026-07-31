/**
 * The pure half of the monitor: given what was observed, decide whether the
 * sitemap is healthy.
 *
 * Split from the fetching deliberately. The decision logic is where a wrong
 * threshold or an inverted comparison hides, and it is only testable in
 * isolation if it does no I/O. fetch.ts gathers, this file judges.
 */

import { FAMILY_SHARD_IDS, type Family } from '@/app/sitemap-shards'
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
  errors: string[]
  /** One line per failed assertion, with the numbers that failed it. */
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
  return FAMILY_SHARD_IDS.map(family => {
    const observed = input.observedByFamily[family] ?? 0
    const expected = input.expectedByFamily[family] ?? 0
    const delta = observed - expected
    const allowed = allowedDrift(expected, config)
    return { family, observed, expected, delta, allowed, ok: Math.abs(delta) <= allowed }
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
    if (!comparison.ok) failures.push(`drift — ${describeDrift(comparison)}`)
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
    errors: input.errors,
    failures,
  }
}

/**
 * Choose `size` distinct entries at random.
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
    const index = Math.floor(rng() * pool.length)
    // Guard against an rng returning exactly 1 (or a hair under, rounding up).
    const safeIndex = Math.min(Math.max(index, 0), pool.length - 1)
    picked.push(pool[safeIndex])
    pool.splice(safeIndex, 1)
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
