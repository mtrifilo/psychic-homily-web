import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'
import { BACKEND_BASE_URL } from '../backend-url'

// A new Google identity, end to end through the faux "google" provider (see
// oauth-google.spec.ts for how the handshake is driven): one trip to Google,
// the "Create your account" step, a signed-in session.
//
// The faux provider takes the identity for one handshake from faux_user_id
// and faux_email on the callback query (backend/internal/auth/
// oauth_test_provider.go), so this spec presents an identity nothing in the
// seed holds without restarting the backend.

const BACKEND_ORIGIN = BACKEND_BASE_URL

async function continueWithGoogleAs(page: Page, subject: string, email: string) {
  const fauxRedirect = page.waitForRequest(
    req => new URL(req.url()).hostname === 'example.com',
    { timeout: 20_000 }
  )
  await page.getByRole('button', { name: 'Continue with Google' }).click()
  const state = new URL((await fauxRedirect).url()).searchParams.get('state')
  expect(state, 'goth should have produced a state nonce').toBeTruthy()

  const callbackUrl = new URL(`${BACKEND_ORIGIN}/auth/callback/google`)
  callbackUrl.searchParams.set('state', state!)
  callbackUrl.searchParams.set('code', 'e2e-faux-auth-code')
  callbackUrl.searchParams.set('faux_user_id', subject)
  callbackUrl.searchParams.set('faux_email', email)
  await page.goto(callbackUrl.toString())
}

test.describe('Google sign-up for a new identity', () => {
  test('finishes on the Create your account step and signs in', async ({ page }) => {
    const unique = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`
    const email = `e2e-new-google-${unique}@test.local`

    await page.goto('/auth')
    await continueWithGoogleAs(page, `e2e-new-google-${unique}`, email)

    await page.waitForURL(url => url.pathname === '/auth/complete', { timeout: 20_000 })
    await expect(
      page.getByRole('heading', { name: 'Create your Psychic Homily account' })
    ).toBeVisible()
    await expect(page.getByText(email)).toBeVisible()

    await page.getByLabel('Month').fill('4')
    await page.getByLabel('Day').fill('27')
    await page.getByLabel('Year').fill('1998')
    await page.getByRole('checkbox', { name: /I agree to the Terms of Service/ }).click()
    await page.getByRole('button', { name: 'Create account' }).click()

    await page.waitForURL(url => !url.pathname.startsWith('/auth'), { timeout: 20_000 })
    await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({
      timeout: 10_000,
    })
  })
})
