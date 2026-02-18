import { test as base, expect } from '@playwright/test'

/**
 * Error detection fixture that auto-fails tests on:
 * - Uncaught exceptions (pageerror)
 * - console.error calls
 * - Failed network requests
 * - 5xx server responses
 *
 * Filters out known acceptable errors.
 */

// Patterns to ignore (expected in normal operation)
const IGNORED_PATTERNS = [
  /401.*\/auth\/profile/, // Expected when not logged in
  /favicon/, // Favicon load failures
  /\/api\/auth\/profile.*401/, // Auth check on unauthenticated pages
  /verify-email\/confirm/, // Expected errors when testing invalid verification tokens
  /magic-link\/verify/, // Expected errors when testing invalid magic link tokens
]

function shouldIgnore(message: string): boolean {
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
        const msg = consoleMessage.text()
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
