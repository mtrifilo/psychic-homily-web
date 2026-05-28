import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

// PSY-719: Google OAuth login.
//
// The E2E harness cannot complete a real Google OAuth handshake:
//   1. The Google provider is only registered when GOOGLE_CLIENT_ID *and*
//      GOOGLE_CLIENT_SECRET are set (backend/internal/auth/goth.go
//      SetupGoth). The E2E backend env (frontend/e2e/global-setup.ts) sets
//      neither, so goth has no "google" provider and BeginAuthHandler errors.
//      OAUTH_SECRET_KEY in that env is only the Gorilla session-store key —
//      not a callback mock.
//   2. Even with a provider registered, completing the flow requires a real
//      redirect to Google's consent screen and a valid authorization code
//      from Google — impossible without an external mock IdP, which does not
//      exist anywhere in backend/ or frontend/e2e/.
//
// Per the ticket, this spec is skipped rather than fabricating a mock. A
// follow-up OAuth-harness spike (a fake OAuth2 IdP + test-only provider
// registration) is needed before this flow can be exercised end-to-end.
test.describe('Google OAuth', () => {
  test.skip(
    true,
    'requires an OAuth IdP mock + test-only provider registration — see PSY-719 follow-up spike'
  )

  test('logs in via the Google button', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('button', { name: /continue with google/i }).click()
    // Would assert the post-callback logged-in state once a mock IdP exists.
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible()
  })
})
