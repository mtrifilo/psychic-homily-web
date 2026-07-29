import { test as base, expect } from '@playwright/test'

/**
 * Error detection fixture that auto-fails tests on:
 * - Uncaught exceptions (pageerror)
 * - console.error calls
 * - Failed network requests
 * - 5xx server responses
 *
 * Filters out known acceptable errors.
 *
 * STATUS: this gate is DORMANT — today it can fail nothing.
 *
 * Playwright builds a fixture only when something destructures it, and no
 * spec destructures `errors`. The auth fixtures in ./auth.ts
 * (`authenticatedPage`, `adminPage`) do destructure it, but they drive a page
 * they open themselves via `browser.newContext()`, whereas the listeners
 * below attach to the default `page` fixture — a different Page object that
 * those tests never navigate. The watcher therefore observes a blank page and
 * its end-of-test assertion passes trivially.
 *
 * Read this as a net that is rigged but not yet hung. Arming it means having
 * a spec destructure `errors` alongside the page it actually drives; moving
 * CI E2E onto a production build is the change that makes that worth doing.
 * Until then, the ignore list below is maintained ahead of need, so that
 * arming the gate is not also a debugging session.
 */

// Patterns to ignore (expected in normal operation).
// Keep these narrow: every entry here is an error the gate can never report
// again, so a pattern broader than the noise it targets silently buys back
// real failures.
const IGNORED_PATTERNS = [
  /401.*\/auth\/profile/, // Expected when not logged in
  /favicon/, // Favicon load failures
  /\/api\/auth\/profile.*401/, // Auth check on unauthenticated pages
  /verify-email\/confirm/, // Expected errors when testing invalid verification tokens
  /magic-link\/verify/, // Expected errors when testing invalid magic link tokens
  // Vercel Analytics and Speed Insights inject their client scripts only in
  // production builds, and only Vercel's edge serves those paths — so they
  // 404 on any other host (local `next start`, self-hosted). Each one emits
  // both a "Refused to execute script" console error and a "Failed to load
  // resource" one. `/_vercel/` is reserved by the platform and no application
  // route lives under it, so this cannot mask a failure of our own code.
  /\/_vercel\//,
]

export function shouldIgnore(message: string): boolean {
  return IGNORED_PATTERNS.some((pattern) => pattern.test(message))
}

type ErrorEntry = {
  type: 'pageerror' | 'console.error' | 'request-failed' | 'server-error'
  message: string
}

export const test = base.extend<{ errors: ErrorEntry[] }>({
  errors: async ({ page }, runFixture) => {
    const errors: ErrorEntry[] = []

    // Uncaught exceptions
    page.on('pageerror', (error) => {
      const msg = error.message
      if (!shouldIgnore(msg)) {
        errors.push({ type: 'pageerror', message: msg })
      }
    })

    // console.error
    page.on('console', (consoleMessage) => {
      if (consoleMessage.type() === 'error') {
        // Chromium reports a failed subresource as a bare "Failed to load
        // resource: …" text and carries the offending URL in `location()`
        // instead. Appending the URL is what lets a URL-targeted ignore
        // pattern match those at all, and it means a reported failure names
        // the resource rather than leaving the reader to guess which one.
        const text = consoleMessage.text()
        const url = consoleMessage.location()?.url
        const msg = url ? `${text} (${url})` : text
        if (!shouldIgnore(msg)) {
          errors.push({ type: 'console.error', message: msg })
        }
      }
    })

    // Failed network requests
    page.on('requestfailed', (request) => {
      const msg = `${request.method()} ${request.url()} - ${request.failure()?.errorText}`
      if (!shouldIgnore(msg)) {
        errors.push({ type: 'request-failed', message: msg })
      }
    })

    // 5xx server errors
    page.on('response', (response) => {
      if (response.status() >= 500) {
        const msg = `${response.status()} ${response.url()}`
        if (!shouldIgnore(msg)) {
          errors.push({ type: 'server-error', message: msg })
        }
      }
    })

    // Run the test
    await runFixture(errors)

    // After the test: assert no unexpected errors occurred
    if (errors.length > 0) {
      const summary = errors
        .map((e) => `  [${e.type}] ${e.message}`)
        .join('\n')
      expect(
        errors.length,
        `Test produced ${errors.length} unexpected error(s):\n${summary}`
      ).toBe(0)
    }
  },
})

export { expect } from '@playwright/test'
