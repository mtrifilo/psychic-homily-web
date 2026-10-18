import { test } from '../fixtures/error-detection'
import { expect, type Request } from '@playwright/test'

// The Google sign-up start sends the consent versions only when GET
// /legal/versions publishes the ones this build's legal pages show.
//
// Retry: the first start is answered the way the backend answers a start whose
// versions it does not accept (a 307 back to /auth with
// TERMS_ACCEPTANCE_REQUIRED), standing in for a backend instance that had not
// yet taken this build's versions. The page reads the versions again and
// sends the start once more; that one reaches the real backend, which sends
// it on to the faux provider's AuthURL (example.com, see
// oauth-google.spec.ts), as far as a sign-up start goes without a consent
// screen.
//
// Changed versions: the route publishes versions no build names, so the form
// asks for a reload and sends nothing.

const REFUSAL_MESSAGE =
  'Please accept the Terms of Service and Privacy Policy before creating an account.'

function startsOf(page: import('@playwright/test').Page): URL[] {
  const starts: URL[] = []
  page.on('request', (request: Request) => {
    const url = new URL(request.url())
    if (url.pathname === '/auth/login/google') starts.push(url)
  })
  return starts
}

async function acceptConsent(page: import('@playwright/test').Page) {
  await page.goto('/auth')
  await page.getByRole('tab', { name: 'Create account' }).click()
  await page.locator('#terms').check()
  await page.locator('#age-confirmation').check()
}

test.describe('Google sign-up consent versions', () => {
  test('retries a start refused for its consent versions once', async ({ page }) => {
    const starts = startsOf(page)
    let refused = false
    await page.route('**/auth/login/google**', async route => {
      if (refused) {
        await route.continue()
        return
      }
      refused = true
      const arrival = new URL('/auth', page.url())
      arrival.searchParams.set('error', REFUSAL_MESSAGE)
      arrival.searchParams.set('error_code', 'TERMS_ACCEPTANCE_REQUIRED')
      await route.fulfill({ status: 307, headers: { location: arrival.toString() } })
    })
    const fauxRedirect = page.waitForRequest(
      request => new URL(request.url()).hostname === 'example.com',
      { timeout: 30_000 }
    )

    await acceptConsent(page)
    await page.getByRole('button', { name: 'Sign up with Google' }).click()

    await fauxRedirect

    expect(starts).toHaveLength(2)
    for (const start of starts) {
      expect(start.searchParams.get('signup_intent')).toBe('1')
      expect(start.searchParams.get('terms_version')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    }
    expect(starts[1].searchParams.get('terms_version')).toBe(
      starts[0].searchParams.get('terms_version')
    )
  })

  test('asks for a reload, and starts nothing, when the published versions changed', async ({ page }) => {
    const starts = startsOf(page)
    await page.route('**/legal/versions', async route => {
      const response = await route.fetch()
      const body = await response.json()
      await route.fulfill({
        response,
        json: { ...body, terms_version: '2999-01-01' },
      })
    })

    await acceptConsent(page)

    await expect(
      page.getByText('The Terms of Service or Privacy Policy were updated.')
    ).toBeVisible()
    await expect(page.getByRole('button', { name: 'Sign up with Google' })).toBeDisabled()
    await expect(page.getByRole('button', { name: 'Create account' })).toBeDisabled()
    expect(starts).toHaveLength(0)
  })
})
