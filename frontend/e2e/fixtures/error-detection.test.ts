import { describe, expect, it } from 'vitest'
import { shouldIgnore } from './error-detection'

/**
 * These strings are captured verbatim from a local production build
 * (`next build` && `next start`) loading `/`, `/shows` and `/artists`. A dev
 * server does not emit them: the analytics scripts are injected only in
 * production output, and only Vercel's edge serves `/_vercel/`, so every
 * other host 404s them.
 *
 * Each script produces a pair of console errors, and only one of the two
 * carries the URL in its text — the other one is a bare "Failed to load
 * resource" whose URL the fixture appends from `location()`. Both forms are
 * asserted here, because a pattern that matched only the chatty one would
 * still leave the gate red.
 */
const VERCEL_ANALYTICS_NOISE = [
  // console.error — URL is in the message text
  "Refused to execute script from 'http://localhost:3457/_vercel/insights/script.js' because its MIME type ('text/html') is not executable, and strict MIME type checking is enabled.",
  "Refused to execute script from 'http://localhost:3457/_vercel/speed-insights/script.js' because its MIME type ('text/html') is not executable, and strict MIME type checking is enabled.",
  // console.error — bare text, URL appended by the fixture from location()
  'Failed to load resource: the server responded with a status of 404 (Not Found) (http://localhost:3457/_vercel/insights/script.js)',
  'Failed to load resource: the server responded with a status of 404 (Not Found) (http://localhost:3457/_vercel/speed-insights/script.js)',
  // requestfailed
  'GET http://localhost:3457/_vercel/insights/script.js - net::ERR_ABORTED',
  'GET http://localhost:3457/_vercel/speed-insights/script.js - net::ERR_ABORTED',
]

describe('error-detection ignore patterns', () => {
  it.each(VERCEL_ANALYTICS_NOISE)('ignores Vercel analytics noise: %s', (message) => {
    expect(shouldIgnore(message)).toBe(true)
  })

  it('ignores the noise regardless of host and port', () => {
    expect(
      shouldIgnore(
        'Failed to load resource: the server responded with a status of 404 (Not Found) (http://127.0.0.1:3000/_vercel/insights/script.js)',
      ),
    ).toBe(true)
  })

  /**
   * The guard rail on the entry above. Suppressing a whole error *shape*
   * ("Failed to load resource", "404") would hide genuine broken assets and
   * dead endpoints, which is precisely the class of bug this gate exists to
   * catch. Only the `/_vercel/` path is spent.
   */
  it('still reports a 404 that is not a Vercel analytics path', () => {
    expect(
      shouldIgnore(
        'Failed to load resource: the server responded with a status of 404 (Not Found) (http://localhost:3000/api/shows)',
      ),
    ).toBe(false)
  })

  it('still reports a missing application asset', () => {
    expect(
      shouldIgnore(
        'Failed to load resource: the server responded with a status of 404 (Not Found) (http://localhost:3000/_next/static/chunks/main.js)',
      ),
    ).toBe(false)
  })

  it('still reports application exceptions and server errors', () => {
    expect(shouldIgnore('TypeError: Cannot read properties of undefined')).toBe(
      false,
    )
    expect(shouldIgnore('500 http://localhost:3000/api/artists')).toBe(false)
  })

  /**
   * Appending the URL is what makes the bare "Failed to load resource" form
   * matchable, but it necessarily widens every OTHER entry too: a pattern
   * naming a route now also matches a browser-generated error that merely
   * happened on that route, because `location()` reports the page URL for
   * those. Pinned here so the reach is a decision rather than a surprise —
   * the affected entries all name flows whose errors are already excused.
   */
  it('matches an ignore pattern against the appended URL, not just the text', () => {
    expect(
      shouldIgnore(
        "Refused to execute script from 'http://localhost:3000/x.js' because its MIME type ('text/html') is not executable. (http://localhost:3000/verify-email/confirm)",
      ),
    ).toBe(true)
  })

  it('does not ignore a path that merely mentions vercel', () => {
    expect(
      shouldIgnore(
        'Failed to load resource: the server responded with a status of 404 (Not Found) (http://localhost:3000/blog/vercel-migration)',
      ),
    ).toBe(false)
  })
})
