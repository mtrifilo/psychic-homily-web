import { describe, expect, it } from 'vitest'
import {
  DATA_CACHE_BUDGET_BYTES,
  DATA_CACHE_ITEM_LIMIT_BYTES,
  DATA_CACHE_RAW_LIMIT_BYTES,
  encodedSize,
  formatMiB,
  isWarnBandAllowlisted,
  WARN_BAND_ALLOWLIST,
} from './budget'

/**
 * The allowlist itself is empty, and empty is the intended steady state — so
 * these drive the MATCHING RULE against injected fixtures rather than against
 * the real list. Without that, every case below would pass vacuously
 * (`[].some()` is false for any input) and the rule would ship unverified.
 *
 * It is worth verifying because the rule is a scoping control on a build-gate
 * waiver: an entry must excuse exactly the fetch it was measured against. The
 * sub-shard ids (`releases-b0`, and its siblings) are precisely the shape the
 * doc warns about: names that EXTEND an existing family name, so a regression
 * to substring or prefix matching would waive a whole family at once.
 */
describe('isWarnBandAllowlisted', () => {
  const allowlist = [{ match: '/sitemap/entries?family=releases' }]
  const at = (url: string) => isWarnBandAllowlisted(url, allowlist)

  it('matches the exact fetch the entry was measured against', () => {
    expect(at('https://api.psychichomily.com/sitemap/entries?family=releases')).toBe(true)
  })

  it('matches regardless of origin — the entry names a path, not a host', () => {
    expect(at('http://localhost:8080/sitemap/entries?family=releases')).toBe(true)
  })

  /**
   * The property the doc calls out by name: `releases-b0` is not `releases`, so
   * an entry for the family must not excuse a sub-shard of it, or vice versa.
   */
  it.each([
    'https://api.psychichomily.com/sitemap/entries?family=releases-b0',
    'https://api.psychichomily.com/sitemap/entries?family=releases-b1',
    'https://api.psychichomily.com/sitemap/entries?family=releases-b7',
    'https://api.psychichomily.com/sitemap/entries?family=releases_v2',
  ])('does not excuse %s', url => {
    expect(at(url)).toBe(false)
  })

  it('is not a bare substring match', () => {
    // The string "/sitemap/entries?family=releases" appears in this URL's query
    // VALUE. A substring implementation would excuse it; identity must not.
    expect(at('https://evil.example/x?q=/sitemap/entries?family=releases')).toBe(false)
  })

  it('ignores query parameters other than family', () => {
    expect(at('https://api.psychichomily.com/sitemap/entries?family=releases&v=2')).toBe(true)
  })

  it('does not excuse the same path with no family at all', () => {
    expect(at('https://api.psychichomily.com/sitemap/entries')).toBe(false)
  })

  it('matches a path-only entry only when no family is present', () => {
    const pathOnly = [{ match: '/artists' }]
    expect(isWarnBandAllowlisted('https://api/artists', pathOnly)).toBe(true)
    expect(isWarnBandAllowlisted('https://api/artists?family=shows', pathOnly)).toBe(false)
  })

  it('refuses anything it cannot parse, rather than guessing', () => {
    expect(at('not a url')).toBe(false)
    expect(at('/sitemap/entries?family=releases')).toBe(false)
    expect(isWarnBandAllowlisted(undefined, allowlist)).toBe(false)
  })

  it('excuses nothing against the real list, which is empty', () => {
    expect(WARN_BAND_ALLOWLIST).toEqual([])
    expect(
      isWarnBandAllowlisted('https://api.psychichomily.com/sitemap/entries?family=releases')
    ).toBe(false)
  })
})

/**
 * The unit rule this module writes down for itself: the cap is 2 x 1024², so
 * everything is reported in MEBIbytes. Pinned because the surrounding docs cite
 * these figures in prose, and a decimal-MB reading of the same bytes is what
 * made an earlier comment claim "2.04 MB, i.e. 97% of the 2 MB cap".
 */
describe('the reported units', () => {
  it('formats against 1024-based mebibytes', () => {
    expect(formatMiB(DATA_CACHE_ITEM_LIMIT_BYTES)).toBe('2.00 MiB')
    expect(formatMiB(2_040_275)).toBe('1.95 MiB')
  })

  it('keeps the raw limit and the encoded cap consistent', () => {
    expect(encodedSize(DATA_CACHE_RAW_LIMIT_BYTES)).toBeLessThanOrEqual(
      DATA_CACHE_ITEM_LIMIT_BYTES
    )
    expect(DATA_CACHE_BUDGET_BYTES).toBeLessThan(DATA_CACHE_ITEM_LIMIT_BYTES)
  })

  /** The measured pre-sharding reading, as a share of the cap. */
  it('puts the un-sharded releases family at 97.3% of the cap', () => {
    expect((100 * 2_040_275) / DATA_CACHE_ITEM_LIMIT_BYTES).toBeCloseTo(97.3, 1)
  })
})
