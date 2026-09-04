import { describe, expect, it } from 'vitest'
import { shardIdsFor, SITEMAP_FAMILIES, type Family } from '@/app/sitemap-shards'
import { resolveConfig, type MonitorConfig } from './config'
import {
  allowedDrift,
  countFutureShows,
  evaluate,
  isoDate,
  pickSample,
  pickStratifiedSample,
  type EvaluationInput,
} from './evaluate'

const config: MonitorConfig = resolveConfig({})

function counts(overrides: Partial<Record<Family, number>> = {}): Record<Family, number> {
  const base = Object.fromEntries(SITEMAP_FAMILIES.map(f => [f, 0])) as Record<Family, number>
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
    observedByShard: new Map(),
    expectedByShard: new Map(),
    unservedShards: [],
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

  /**
   * The absolute floor keeps small families quiet, but it also made them
   * unmonitorable: `festivals` has 10 entries and the default floor is 10, so
   * the entire family disappearing sat exactly inside budget and reported
   * green. Total disappearance is the defect class this monitor exists to
   * catch, so it must fail at any tolerance.
   */
  it('fails a family that vanished entirely, even inside the drift budget', () => {
    const report = evaluate(
      input({
        observedByFamily: counts({ festivals: 0 }),
        expectedByFamily: counts({ festivals: 10 }),
      }),
      config
    )
    expect(report.ok).toBe(false)
    expect(report.failures[0]).toContain('vanished — festivals')
    expect(report.failures[0]).toContain('the API has 10 entries and the sitemap serves NONE')
  })

  it('does not cry vanished when the API itself has no entries', () => {
    const report = evaluate(
      input({ observedByFamily: counts({ festivals: 0 }), expectedByFamily: counts({ festivals: 0 }) }),
      config
    )
    expect(report.ok).toBe(true)
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

/**
 * The per-document half of the verdict, and the reason it exists: a family
 * comparison cannot see one document of a sub-sharded family going dark,
 * because a bucket is a fraction of its family and that fraction sits inside
 * the drift tolerance.
 */
describe('evaluate, per shard', () => {
  const SHOWS_BUCKETS = shardIdsFor('shows')

  /** Every shows bucket healthy at `each` rows, with `overrides` applied. */
  function showBuckets(each: number, overrides: Record<string, number> = {}) {
    const observed = new Map<string, number>()
    const expected = new Map<string, number>()
    for (const shard of SHOWS_BUCKETS) {
      expected.set(shard, each)
      observed.set(shard, overrides[shard] ?? each)
    }
    const total = SHOWS_BUCKETS.length * each
    const served = [...observed.values()].reduce((sum, n) => sum + n, 0)
    return input({
      observedByShard: observed,
      expectedByShard: expected,
      observedByFamily: counts({ shows: served }),
      expectedByFamily: counts({ shows: total }),
    })
  }

  it('passes when every document matches the feed', () => {
    const report = evaluate(showBuckets(1600), config)

    expect(report.ok).toBe(true)
    expect(report.shards.filter(s => s.family === 'shows')).toHaveLength(SHOWS_BUCKETS.length)
  })

  /**
   * The whole reason for the per-document check. One bucket of eight is 12.5%
   * of its family, which clears the default 20% family tolerance, so the family
   * comparison passes and without this the loss would be reported nowhere.
   */
  it('names a document that serves nothing while its family passes', () => {
    const dark = SHOWS_BUCKETS[2]
    const report = evaluate(showBuckets(1600, { [dark]: 0 }), config)

    expect(report.families.find(f => f.family === 'shows')?.ok).toBe(true)
    expect(report.ok).toBe(false)
    expect(report.failures).toContainEqual(
      `vanished — shard ${dark}: the API has 1600 entries and the document serves NONE`
    )
    expect(report.shards.find(s => s.shard === dark)?.vanished).toBe(true)
  })

  it('reports a document that drifted past its own tolerance', () => {
    const drifted = SHOWS_BUCKETS[1]
    const report = evaluate(showBuckets(1600, { [drifted]: 1000 }), config)

    expect(report.failures).toContainEqual(
      `drift — shard ${drifted}: sitemap 1000 vs API 1600 (600 missing, tolerance ±320)`
    )
  })

  // A document that never answered is already reported as a fetch error. Scoring
  // it as zero observed would blame the sitemap for a transport failure.
  it('stays silent about a document that was never fetched', () => {
    const missing = SHOWS_BUCKETS[3]
    const base = showBuckets(1600)
    const observed = new Map(base.observedByShard)
    observed.delete(missing)
    const report = evaluate({ ...base, observedByShard: observed }, config)

    expect(report.failures).toEqual([])
    expect(report.shards.map(s => s.shard)).not.toContain(missing)
  })

  // A document with no expected count is not scored: inventing a zero would
  // print a failure whose own numbers say nothing failed. The reason it has no
  // count is reported by its own class, below.
  it('does not score a document the feed reported no count for', () => {
    const unexpected = SHOWS_BUCKETS[4]
    const base = showBuckets(1600)
    const expected = new Map(base.expectedByShard)
    expected.delete(unexpected)
    const report = evaluate({ ...base, expectedByShard: expected }, config)

    expect(report.shards.map(s => s.shard)).not.toContain(unexpected)
  })

  /**
   * The deploy window this whole scheme is built to tolerate: the frontend
   * lists an id the deployed backend does not recognise yet. It has to be a
   * red report naming the ids, not a crash alert, because the sitemap really is
   * short and "the monitor could not run" says the opposite.
   */
  it('fails, naming the ids, when the API does not serve a shard', () => {
    const unserved = SHOWS_BUCKETS[5]
    const report = evaluate({ ...showBuckets(1600), unservedShards: [unserved] }, config)

    expect(report.ok).toBe(false)
    expect(report.failures).toContainEqual(
      `unserved — shard ${unserved}: the API does not serve this id, so its URLs are announced by nobody`
    )
  })

  /**
   * Ordering, not wording: the Discord field truncates, so the one-line classes
   * have to precede the class that arrives eight or twenty-four lines at a time.
   */
  it('reports a vanished document before proportional drift', () => {
    const dark = SHOWS_BUCKETS[0]
    const drifted = SHOWS_BUCKETS[1]
    const report = evaluate(showBuckets(1600, { [dark]: 0, [drifted]: 800 }), config)
    const vanishedAt = report.failures.findIndex(f => f.startsWith('vanished — shard'))
    const driftAt = report.failures.findIndex(f => f.startsWith('drift — shard'))

    expect(vanishedAt).toBeGreaterThanOrEqual(0)
    expect(driftAt).toBeGreaterThan(vanishedAt)
  })

  // A single-document family is already covered by its FamilyComparison, and
  // comparing it twice would print one problem as two failures.
  it('does not compare a family served by one document', () => {
    const report = evaluate(showBuckets(1600), config)

    expect(report.shards.map(s => s.shard)).not.toContain('venues')
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

/**
 * Uniform sampling over the pooled URLs would make the reachability probe a
 * releases-only check: releases is ~20k of the ~33k URLs, so a 10-URL sample
 * expects 0.44 shows. Every family must be represented.
 */
describe('pickStratifiedSample', () => {
  const skewed = new Map<string, string[]>([
    ['releases', Array.from({ length: 20_000 }, (_, i) => `https://x/releases/${i}`)],
    ['shows', Array.from({ length: 1_458 }, (_, i) => `https://x/shows/${i}`)],
    ['festivals', ['https://x/festivals/only']],
  ])

  it('draws from every non-empty bucket', () => {
    const picked = pickStratifiedSample(skewed, 10, () => 0)
    expect(picked.some(u => u.includes('/releases/'))).toBe(true)
    expect(picked.some(u => u.includes('/shows/'))).toBe(true)
    expect(picked.some(u => u.includes('/festivals/'))).toBe(true)
  })

  it('still probes a family that has only one URL', () => {
    expect(pickStratifiedSample(skewed, 3, () => 0)).toContain('https://x/festivals/only')
  })

  it('skips empty buckets rather than padding with nothing', () => {
    const withEmpty = new Map<string, string[]>([
      ['shows', ['https://x/shows/1']],
      ['tags', []],
    ])
    expect(pickStratifiedSample(withEmpty, 4, () => 0)).toEqual(['https://x/shows/1'])
  })

  it('returns nothing when there is nothing to sample', () => {
    expect(pickStratifiedSample(new Map(), 10)).toEqual([])
    expect(pickStratifiedSample(skewed, 0)).toEqual([])
  })
})
