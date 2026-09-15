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
 * is, so `/shows/2026/11/31` is claimed by neither and the proxy 404s it on the
 * calendar test. The shape asserted against is the proxy's exported copy, which
 * is itself asserted equal to the route grammar's, so this chains onto that.
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
   * The rule's `source` compiled the way Next compiles it, through its own
   * bundled matcher.
   *
   * Compiling the WHOLE source rather than extracting the `:slug` sub-pattern
   * is what makes this test able to see the optional `[/#?]` suffix
   * path-to-regexp appends: a lookahead anchored on `$` stops applying the
   * moment anything follows the day segment, and a sub-pattern lifted out of
   * its context cannot show that.
   */
  async function ruleMatcher() {
    const rule = await flattenRule()
    // Next bundles its own copy and ships no types for it. The cast is the
    // narrowest statement of what this test needs: the compiler Next itself
    // applies to a redirect source.
    const compiled = (await import(
      // @ts-expect-error - no type declarations ship with the bundled copy
      'next/dist/compiled/path-to-regexp/index.js'
    )) as { pathToRegexp: (source: string) => RegExp }
    return compiled.pathToRegexp(rule.source)
  }

  it('claims a final segment exactly when the day route does not', async () => {
    const matcher = await ruleMatcher()

    const segments: string[] = [
      'a-real-show-slug',
      '2026-03-20-a-show',
      'x',
      '0',
      '001',
      '100',
      '011',
    ]
    for (let value = 0; value < 100; value += 1) {
      segments.push(String(value).padStart(2, '0'))
      segments.push(String(value))
    }

    // A bare path, and every suffix path-to-regexp's own trailing group can
    // absorb. The suffixed forms are the ones a lookahead anchored on `$` gets
    // wrong, and they are exactly the shape the legacy Hugo URLs had.
    const suffixes = ['', '/', '#frag', '?page=2']

    for (const segment of segments) {
      const isDayShaped = SHOWS_CALENDAR_DAY_SEGMENT.test(segment)
      for (const suffix of suffixes) {
        const path = `/shows/2026/11/${segment}${suffix}`
        expect(
          matcher.test(path),
          `redirect and day shape disagree about ${path}`
        ).toBe(!isDayShaped)
      }
    }
  })

  // The rule's own reason for existing: a legacy show URL still flattens,
  // trailing slash and all, which is the form those URLs were written in.
  it('still claims a word slug, with or without a trailing slash', async () => {
    const matcher = await ruleMatcher()
    const rule = await flattenRule()

    expect(matcher.test('/shows/2026/11/bright-eyes-at-the-rebel-lounge')).toBe(
      true
    )
    expect(matcher.test('/shows/2026/11/bright-eyes-at-the-rebel-lounge/')).toBe(
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
