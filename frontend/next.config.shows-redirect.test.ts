import { describe, it, expect, vi } from 'vitest'

// The global mock in test/setup omits `withSentryConfig`, which next.config
// calls at module scope. Identity is the right stand-in here: this file asserts
// the redirect table, and Sentry's wrapper does not touch it.
vi.mock('@sentry/nextjs', async importOriginal => {
  const actual = await importOriginal<Record<string, unknown>>()
  return {
    ...actual,
    withSentryConfig: <T,>(config: T): T => config,
  }
})

import nextConfig from './next.config'
import { SHOWS_CALENDAR_DAY_SEGMENT } from './proxy'

/**
 * The legacy Hugo flatten rule (`/shows/{yyyy}/{mm}/{slug}` -> `/shows/{slug}`)
 * is evaluated BEFORE the proxy and before the router, so it is the FIRST thing
 * a day URL meets. It carries its own copy of the day grammar as a negative
 * lookahead, and that copy is the one nothing else pins: widen or narrow
 * `DAY_SEGMENT` and this rule silently starts eating day URLs again, with the
 * proxy tests and the route tests both still green.
 *
 * So this walks the rule's own source pattern against the day SHAPE over every
 * one- and two-digit final segment plus real slugs, and asserts the two
 * partition the space exactly: the redirect claims a segment if and only if the
 * day route's shape does not.
 *
 * The shape, not `parseDaySegments`: the redirect cannot know how long a month
 * is, so `/shows/2026/11/31` is claimed by neither (`proxy.ts` 404s it on the
 * calendar test). `SHOWS_CALENDAR_DAY_SEGMENT` is the proxy's copy, which
 * `proxy.shows-calendar.test.ts` already pins to the route's — so asserting
 * against it chains all three copies together.
 */
describe('the legacy Hugo shows redirect', () => {
  async function flattenRule() {
    const redirects = await nextConfig.redirects!()
    const rule = redirects.find(
      entry =>
        entry.source.startsWith('/shows/:year') &&
        entry.destination === '/shows/:slug'
    )
    expect(rule, 'the Hugo flatten rule is still in next.config').toBeDefined()
    return rule!
  }

  /**
   * The rule's `source` is path-to-regexp, and the only part this test needs to
   * evaluate is the custom pattern on `:slug`. Extracting it keeps the test
   * honest about WHICH text it is checking rather than re-stating the lookahead.
   */
  function slugPattern(source: string): RegExp {
    const match = source.match(/:slug\((.*)\)$/)
    expect(match, `no custom :slug pattern in ${source}`).not.toBeNull()
    return new RegExp(`^(?:${match![1]})$`)
  }

  it('claims a final segment exactly when the day route does not', async () => {
    const rule = await flattenRule()
    const pattern = slugPattern(rule.source)

    const segments: string[] = ['a-real-show-slug', '2026-03-20-a-show', 'x', '0']
    for (let value = 0; value < 100; value += 1) {
      segments.push(String(value).padStart(2, '0'))
      segments.push(String(value))
    }

    for (const segment of segments) {
      const isDayShaped = SHOWS_CALENDAR_DAY_SEGMENT.test(segment)
      expect(
        pattern.test(segment),
        `redirect and day shape disagree about /shows/2026/11/${segment}`
      ).toBe(!isDayShaped)
    }
  })

  // The rule's own reason for existing: a legacy show URL still flattens.
  it('still claims a word slug', async () => {
    const rule = await flattenRule()
    expect(slugPattern(rule.source).test('bright-eyes-at-the-rebel-lounge')).toBe(
      true
    )
    expect(rule.permanent).toBe(true)
  })

  // The month route is two segments under /shows; this rule needs three, so it
  // cannot reach a month URL at all.
  it('requires a third segment, so it cannot reach a month URL', async () => {
    const rule = await flattenRule()
    expect(rule.source).toMatch(/^\/shows\/:year\([^)]*\)\/:month\([^)]*\)\/:slug\(/)
  })
})
