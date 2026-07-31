import { describe, expect, it } from 'vitest'
import { FAMILY_SHARD_IDS, type Family } from '@/app/sitemap-shards'
import { resolveConfig, type MonitorConfig } from './config'
import {
  allowedDrift,
  countFutureShows,
  evaluate,
  isoDate,
  pickSample,
  type EvaluationInput,
} from './evaluate'

const config: MonitorConfig = resolveConfig({})

function counts(overrides: Partial<Record<Family, number>> = {}): Record<Family, number> {
  const base = Object.fromEntries(FAMILY_SHARD_IDS.map(f => [f, 0])) as Record<Family, number>
  return { ...base, ...overrides }
}

function input(overrides: Partial<EvaluationInput> = {}): EvaluationInput {
  return {
    shape: 'index',
    shardCount: 10,
    observedByFamily: counts(),
    observedPages: 12,
    observedOther: 0,
    expectedByFamily: counts(),
    futureShowCount: 500,
    samples: [],
    errors: [],
    ...overrides,
  }
}

describe('allowedDrift', () => {
  // The floor governs small families, the ratio governs large ones. Taking the
  // larger of the two is what keeps both ends of the range quiet.
  it('uses the absolute floor for a small family', () => {
    expect(allowedDrift(10, config)).toBe(10)
  })

  it('uses the ratio for a large family', () => {
    expect(allowedDrift(20000, config)).toBe(4000)
  })

  it('never returns a fractional budget', () => {
    expect(allowedDrift(1457, config)).toBe(292)
  })
})

describe('evaluate', () => {
  it('passes when every family is within tolerance', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ shows: 1458, artists: 3476 }),
        expectedByFamily: counts({ shows: 1458, artists: 3476 }),
      }),
      config
    )
    expect(report.ok).toBe(true)
    expect(report.failures).toEqual([])
  })

  it('tolerates small drift inside the budget', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ shows: 1400 }),
        expectedByFamily: counts({ shows: 1458 }),
      }),
      config
    )
    expect(report.ok).toBe(true)
  })

  it('reports the drift numbers when a family is short', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ shows: 900 }),
        expectedByFamily: counts({ shows: 1458 }),
      }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures[0]).toContain('shows: sitemap 900 vs API 1458')
    expect(report.failures[0]).toContain('558 missing')
    expect(report.failures[0]).toContain('tolerance ±292')
  })

  it('flags over-coverage too, not only missing URLs', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ venues: 400 }),
        expectedByFamily: counts({ venues: 161 }),
      }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures[0]).toContain('239 extra')
  })

  /**
   * The regression test for the actual incident. The served sitemap had 114
   * correctly-formed show URLs, all historical, against 3498 slugged shows.
   * Both halves of this must fail — and the freshness half must fail even when
   * the count check is satisfied (the next case).
   */
  it('fails the known-stale production fixture', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ shows: 114 }),
        expectedByFamily: counts({ shows: 3498 }),
        futureShowCount: 0,
      }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures.some(f => f.includes('shows: sitemap 114 vs API 3498'))).toBe(true)
    expect(report.failures.some(f => f.includes('only 0 upcoming show URLs'))).toBe(true)
  })

  /**
   * The case a pure count check cannot catch: the right NUMBER of shows, all
   * of them in the past. This is why the freshness assertion is not optional.
   */
  it('fails a sitemap with a plausible count but no upcoming shows', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ shows: 1458 }),
        expectedByFamily: counts({ shows: 1458 }),
        futureShowCount: 0,
      }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures).toHaveLength(1)
    expect(report.failures[0]).toContain('only 0 upcoming show URLs')
  })

  it('fails when a sampled URL does not resolve', () => {
    const report = evaluate(
      input({
        samples: [
          { url: 'https://psychichomily.com/shows/a', status: 200, ok: true },
          { url: 'https://psychichomily.com/shows/b', status: 404, ok: false },
        ],
      }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures[0]).toContain('unreachable — https://psychichomily.com/shows/b → 404')
  })

  it('fails on a gathering error even when the counts line up', () => {
    const report = evaluate(
      input({ errors: ['sitemap index is missing the "releases" shard'] }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures[0]).toContain('missing the "releases" shard')
  })

  it('totals observed URLs across families, pages and unclassified', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ shows: 100, artists: 50 }),
        observedPages: 12,
        observedOther: 3,
      }),
      config
    )
    expect(report.observedTotal).toBe(165)
  })

  it('never fails merely because unclassified URLs exist', () => {
    const report = evaluate(input({ observedOther: 99 }), config)
    expect(report.ok).toBe(true)
  })

  it('honours a tightened threshold from config', () => {
    const strict = resolveConfig({
      SITEMAP_MONITOR_DRIFT_RATIO: '0',
      SITEMAP_MONITOR_DRIFT_FLOOR: '0',
    })
    const observed = { observedByFamily: counts({ shows: 1457 }), expectedByFamily: counts({ shows: 1458 }) }
    expect(evaluate(input(observed), config).ok).toBe(true)
    expect(evaluate(input(observed), strict).ok).toBe(false)
  })
})

describe('countFutureShows', () => {
  it('counts today as upcoming', () => {
    expect(countFutureShows(['2026-07-29', '2026-07-30', '2026-07-31'], '2026-07-30')).toBe(2)
  })

  it('returns zero for an entirely historical catalogue', () => {
    expect(countFutureShows(['2025-01-01', '2025-06-30'], '2026-07-30')).toBe(0)
  })
})

describe('isoDate', () => {
  // Compared as strings in UTC so the runner's timezone cannot flip a verdict.
  it('formats in UTC regardless of local zone', () => {
    expect(isoDate(new Date('2026-07-30T23:30:00Z'))).toBe('2026-07-30')
    expect(isoDate(new Date('2026-07-31T00:30:00Z'))).toBe('2026-07-31')
  })
})

describe('pickSample', () => {
  const items = ['a', 'b', 'c', 'd', 'e']

  it('returns the requested number of distinct entries', () => {
    const picked = pickSample(items, 3, () => 0.5)
    expect(picked).toHaveLength(3)
    expect(new Set(picked).size).toBe(3)
  })

  it('returns everything when asked for more than exists', () => {
    expect(pickSample(items, 99, () => 0)).toEqual(items)
  })

  it('returns nothing when sampling is disabled', () => {
    expect(pickSample(items, 0)).toEqual([])
    expect(pickSample([], 5)).toEqual([])
  })

  // An rng returning exactly 1 would index past the end and push undefined
  // into the sample, which would then be probed as the string "undefined".
  it('stays in bounds for a degenerate rng', () => {
    const picked = pickSample(items, 5, () => 1)
    expect(picked).toHaveLength(5)
    expect(picked.every(entry => typeof entry === 'string')).toBe(true)
  })
})
