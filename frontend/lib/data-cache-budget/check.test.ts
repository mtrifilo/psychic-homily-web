import { describe, expect, it } from 'vitest'
import { findAllowlisted, findOverBudget, formatBudgetFailures } from './check'
import {
  DATA_CACHE_BUDGET_BYTES,
  DATA_CACHE_BUDGET_FRACTION,
  DATA_CACHE_ITEM_LIMIT_BYTES,
  DATA_CACHE_RAW_LIMIT_BYTES,
} from './budget'

// The measured `/artists` entry sizes from PSY-1674, as on-disk (base64) bytes.
const ARTISTS_CACHED_2026_07_26 = 1_533_430
const ARTISTS_OVER_2026_08_08 = 4_311_128

describe('the budget constants', () => {
  it('caps at the 2 MB Vercel documents for a single cache item', () => {
    expect(DATA_CACHE_ITEM_LIMIT_BYTES).toBe(2_097_152)
  })

  // Failing only at the cap means failing on the deploy that has already
  // shipped a silently-uncached route. The margin is the point of the gate.
  it('fails below the cap, leaving headroom to react', () => {
    expect(DATA_CACHE_BUDGET_FRACTION).toBeLessThan(1)
    expect(DATA_CACHE_BUDGET_BYTES).toBeLessThan(DATA_CACHE_ITEM_LIMIT_BYTES)
  })

  // The cap applies to the base64 envelope, so a raw body has ~3/4 of it. This
  // is the number the module header calls the "~1.5 MB effective raw budget",
  // and it is what a `curl` byte count should be compared against.
  it('derives a raw limit around 1.5 MB from the base64 inflation', () => {
    expect(DATA_CACHE_RAW_LIMIT_BYTES).toBe(1_572_864)
  })
})

describe('findOverBudget', () => {
  it('passes an entry under the budget', () => {
    expect(findOverBudget([{ key: 'a', bytes: 414_988 }])).toEqual([])
  })

  // The regression this gate exists to catch, replayed at its measured size.
  it('fails the /artists entry as it stood on 2026-08-08', () => {
    const [failure] = findOverBudget([
      { key: 'a', bytes: ARTISTS_OVER_2026_08_08, url: 'https://api/artists' },
    ])

    expect(failure).toBeDefined()
    expect(failure.url).toBe('https://api/artists')
    expect(failure.fraction).toBeCloseTo(2.06, 2)
  })

  // The entry that was still caching, six weeks of growth earlier, is the case
  // that must NOT fail — otherwise the gate is just a lower cap.
  it('passes the last /artists entry that actually cached', () => {
    expect(findOverBudget([{ key: 'a', bytes: ARTISTS_CACHED_2026_07_26 }])).toEqual([])
  })

  it('fails at the boundary rather than just past it', () => {
    expect(findOverBudget([{ key: 'a', bytes: DATA_CACHE_BUDGET_BYTES }])).toHaveLength(1)
    expect(findOverBudget([{ key: 'a', bytes: DATA_CACHE_BUDGET_BYTES - 1 }])).toEqual([])
  })

  // The recorded exception must not fail the build, but must still be visible.
  it('does not fail a recorded warn-band exception, but still surfaces it', () => {
    const entries = [
      {
        key: 'a',
        bytes: 2_028_910,
        url: 'https://api/sitemap/entries?family=releases',
      },
    ]
    expect(findOverBudget(entries)).toEqual([])
    expect(findAllowlisted(entries)).toHaveLength(1)
  })

  // An allowlist entry waives the WARN band only. Nothing waives the cap.
  it('does not allowlist a family that is not recorded', () => {
    expect(
      findOverBudget([
        { key: 'a', bytes: 2_028_910, url: 'https://api/sitemap/entries?family=shows' },
      ])
    ).toHaveLength(1)
  })

  it('reports the worst offender first', () => {
    const failures = findOverBudget([
      { key: 'small', bytes: DATA_CACHE_BUDGET_BYTES + 1 },
      { key: 'big', bytes: ARTISTS_OVER_2026_08_08 },
    ])

    expect(failures.map(f => f.key)).toEqual(['big', 'small'])
  })
})

describe('formatBudgetFailures', () => {
  const message = formatBudgetFailures(
    findOverBudget([
      { key: 'abc123', bytes: ARTISTS_OVER_2026_08_08, url: 'https://api/artists' },
    ])
  )

  it('names the offending URL and its share of the cap', () => {
    expect(message).toContain('https://api/artists')
    expect(message).toContain('4.11 MB')
    expect(message).toContain('206%')
  })

  // Whoever reads this is looking at a red build over a limit they have likely
  // never met, and the fix is a payload decision, not a code defect.
  it('explains the silence and points at the remedies', () => {
    // Next does log a console.warn — the failure is buried, not absent, and
    // saying "no signal at all" is the wrong claim to hand the next reader.
    expect(message).toContain('console.warn')
    expect(message.toLowerCase()).toContain('base64')
    expect(message.toLowerCase()).toContain('project the response')
  })

  // Falling back to the cache key keeps an entry reportable when the envelope
  // shape changes under a Next upgrade.
  it('falls back to the cache key when the url could not be read', () => {
    const withoutUrl = formatBudgetFailures(
      findOverBudget([{ key: 'abc123', bytes: ARTISTS_OVER_2026_08_08 }])
    )
    expect(withoutUrl).toContain('abc123')
  })
})
