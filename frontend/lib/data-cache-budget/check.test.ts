import { afterEach, describe, expect, it, vi } from 'vitest'

/**
 * The allowlist itself is EMPTY, and empty is its intended steady state
 * (PSY-1763 removed the only entry it has ever held). Its BEHAVIOUR still has
 * to be tested, so the membership test is stubbed rather than a permanent entry
 * being parked in the real list to keep a test alive — which is exactly the
 * rot the list's own doc comment warns about.
 */
const { allowlistedUrls } = vi.hoisted(() => ({
  allowlistedUrls: new Set<string>(),
}))

vi.mock('./budget', async importOriginal => ({
  ...(await importOriginal<typeof import('./budget')>()),
  isWarnBandAllowlisted: (url?: string) => url !== undefined && allowlistedUrls.has(url),
}))

import { formatBudgetFailures, partitionOverBudget } from './check'
import {
  DATA_CACHE_BUDGET_BYTES,
  DATA_CACHE_BUDGET_FRACTION,
  DATA_CACHE_ITEM_LIMIT_BYTES,
  DATA_CACHE_RAW_LIMIT_BYTES,
  WARN_BAND_ALLOWLIST,
} from './budget'

afterEach(() => {
  allowlistedUrls.clear()
})

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

describe('the warn-band allowlist', () => {
  // Age is the signal that an entry has rotted into a permanent baseline, so
  // it has to be readable without parsing prose. Vacuous while the list is
  // empty, and deliberately kept: it is the shape guard for the next entry
  // anyone adds, and an empty list is the state that needs no guarding.
  it('dates every entry', () => {
    for (const entry of WARN_BAND_ALLOWLIST) {
      expect(entry.measuredAt, `${entry.match} has no measuredAt`).toMatch(/^\d{4}-\d{2}-\d{2}$/)
      expect(entry.reason.length).toBeGreaterThan(20)
      expect(entry.ticket, `${entry.match} has no ticket`).toMatch(/^PSY-\d+$/)
    }
  })
})

describe('partitionOverBudget', () => {
  it('passes an entry under the budget', () => {
    expect(partitionOverBudget([{ key: 'a', bytes: 414_988 }]).failures).toEqual([])
  })

  // The regression this gate exists to catch, replayed at its measured size.
  it('fails the /artists entry as it stood on 2026-08-08', () => {
    const [failure] = partitionOverBudget([
      { key: 'a', bytes: ARTISTS_OVER_2026_08_08, url: 'https://api/artists' },
    ]).failures

    expect(failure).toBeDefined()
    expect(failure.url).toBe('https://api/artists')
    expect(failure.fraction).toBeCloseTo(2.06, 2)
  })

  // The entry that was still caching, weeks of growth earlier, is the case that
  // must NOT fail — otherwise the gate is just a lower cap.
  it('passes the last /artists entry that actually cached', () => {
    expect(
      partitionOverBudget([{ key: 'a', bytes: ARTISTS_CACHED_2026_07_26 }]).failures
    ).toEqual([])
  })

  it('fails at the boundary rather than just past it', () => {
    expect(
      partitionOverBudget([{ key: 'a', bytes: DATA_CACHE_BUDGET_BYTES }]).failures
    ).toHaveLength(1)
    expect(
      partitionOverBudget([{ key: 'a', bytes: DATA_CACHE_BUDGET_BYTES - 1 }]).failures
    ).toEqual([])
  })

  // The recorded exception must not fail the build, but must still be visible.
  // Driven at the size the releases family actually reached before it was
  // sub-sharded (PSY-1674 measurement, 97% of the cap), because that is the
  // band this branch exists for.
  it('does not fail a recorded warn-band exception, but still surfaces it', () => {
    const url = 'https://api.psychichomily.com/sitemap/entries?family=releases'
    allowlistedUrls.add(url)

    const { failures, allowlisted } = partitionOverBudget([
      { key: 'a', bytes: 2_028_910, url },
    ])

    expect(failures).toEqual([])
    expect(allowlisted).toHaveLength(1)
    expect(allowlisted[0].fraction).toBeCloseTo(0.97, 2)
  })

  // An allowlist entry waives the WARN band for one URL only.
  it('does not allowlist a URL that is not recorded', () => {
    allowlistedUrls.add('https://api.psychichomily.com/sitemap/entries?family=releases')

    expect(
      partitionOverBudget([
        {
          key: 'a',
          bytes: 2_028_910,
          url: 'https://api.psychichomily.com/sitemap/entries?family=shows',
        },
      ]).failures
    ).toHaveLength(1)
  })

  // The hard cap is enforced independently of the list: an allowlisted URL that
  // actually breaches has already stopped being cached, and waving it through
  // would make the gate a lower cap with a bypass.
  it('still fails an allowlisted URL that is over the hard cap', () => {
    const url = 'https://api.psychichomily.com/sitemap/entries?family=releases'
    allowlistedUrls.add(url)

    const { failures, allowlisted } = partitionOverBudget([
      { key: 'a', bytes: DATA_CACHE_ITEM_LIMIT_BYTES + 1, url },
    ])

    expect(failures).toHaveLength(1)
    expect(allowlisted).toEqual([])
  })

  it('reports the worst offender first', () => {
    const { failures } = partitionOverBudget([
      { key: 'small', bytes: DATA_CACHE_BUDGET_BYTES + 1 },
      { key: 'big', bytes: ARTISTS_OVER_2026_08_08 },
    ])

    expect(failures.map(f => f.key)).toEqual(['big', 'small'])
  })
})

describe('formatBudgetFailures', () => {
  const message = formatBudgetFailures(
    partitionOverBudget([
      { key: 'abc123', bytes: ARTISTS_OVER_2026_08_08, url: 'https://api/artists' },
    ]).failures
  )

  it('names the offending URL and its share of the cap', () => {
    expect(message).toContain('https://api/artists')
    expect(message).toContain('4.11 MiB')
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

  // The gate fires at 80% of the cap, so MOST red builds are warn-band ones.
  // Telling that reader the payload "is not cached and re-pulls on every render"
  // describes a production regression that has not happened yet.
  it('does not claim a warn-band payload has stopped being cached', () => {
    const warnBand = formatBudgetFailures(
      partitionOverBudget([
        { key: 'a', bytes: DATA_CACHE_BUDGET_BYTES + 1, url: 'https://api/x' },
      ]).failures
    )

    expect(warnBand).toContain('WARN')
    expect(warnBand).toContain('still cached')
    expect(warnBand).not.toContain('NOT CACHED AT ALL')
  })

  it('says plainly that a breached payload is not cached at all', () => {
    expect(message).toContain('BREACH')
    expect(message).toContain('NOT CACHED AT ALL')
  })

  // The operator reads this under hotfix pressure; the override is the only
  // thing here they can act on immediately.
  it('names the break-glass and its cost', () => {
    expect(message).toContain('DATA_CACHE_BUDGET_ENFORCE=warn')
    expect(message).toContain('does not fix anything')
  })

  // Falling back to the cache key keeps an entry reportable when the envelope
  // shape changes under a Next upgrade.
  it('falls back to the cache key when the url could not be read', () => {
    const withoutUrl = formatBudgetFailures(
      partitionOverBudget([{ key: 'abc123', bytes: ARTISTS_OVER_2026_08_08 }]).failures
    )
    expect(withoutUrl).toContain('abc123')
  })
})
