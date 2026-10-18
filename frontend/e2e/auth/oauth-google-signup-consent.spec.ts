import { test } from '../fixtures/error-detection'
import { expect, type Request } from '@playwright/test'

// A Google sign-up start whose consent versions the backend no longer accepts
// is refused back to /auth with TERMS_ACCEPTANCE_REQUIRED, and the page retries
// it once with versions read again from GET /legal/versions.
//
// Every versions read before the first start is answered with a version no
// backend accepts, standing in for a cached answer older than the deployed
// backend; every later read reaches the real route. The real backend start refuses the stale
// start, and the retried start reaches the faux provider's AuthURL
// (example.com, see oauth-google.spec.ts), which is as far as a sign-up start
// goes without a consent screen.

const STALE_TERMS_VERSION = '2000-01-01'

test.describe('Google sign-up consent retry', () => {
  test('retries a start refused for stale consent versions once, with the published versions', async ({ page }) => {
    const starts: URL[] = []
    page.on('request', (request: Request) => {
      const url = new URL(request.url())
      if (url.pathname === '/auth/login/google') starts.push(url)
    })

    await page.route('**/legal/versions', async route => {
      if (starts.length > 0) {
        await route.continue()
        return
      }
      const response = await route.fetch()
      const body = await response.json()
      await route.fulfill({ response, json: { ...body, terms_version: STALE_TERMS_VERSION } })
    })
    const fauxRedirect = page.waitForRequest(
      request => new URL(request.url()).hostname === 'example.com',
      { timeout: 30_000 }
    )

    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await page.locator('#terms').check()
    await page.locator('#age-confirmation').check()
    await page.getByRole('button', { name: 'Sign up with Google' }).click()

    await fauxRedirect

    expect(starts).toHaveLength(2)
    expect(starts[0].searchParams.get('terms_version')).toBe(STALE_TERMS_VERSION)
    const retried = starts[1].searchParams
    expect(retried.get('signup_intent')).toBe('1')
    expect(retried.get('terms_version')).not.toBe(STALE_TERMS_VERSION)
    expect(retried.get('terms_version')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(retried.get('privacy_version')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})
