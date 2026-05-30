import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

// PSY-914: Google OAuth login, end-to-end through a faux "google" provider.
//
// The E2E harness cannot complete a REAL Google handshake (no client ID/secret,
// no live consent screen). Instead the backend registers a faux clone of goth's
// test provider under the name "google", gated behind ENABLE_OAUTH_TEST_PROVIDER
// (set in global-setup.ts) plus a default-deny ENVIRONMENT guard that refuses to
// boot the server with the flag on outside {test,ci,development}. See
// backend/internal/auth/oauth_test_provider.go.
//
// What this exercises (~95% of the real path): the button does a full-page nav
// to the backend's /auth/login/google, which runs the genuine
// gothic.BeginAuthHandler — it mints the _gothic_session cookie (carrying the
// state nonce) and 307s to the faux AuthURL http://example.com/auth?state=<nonce>.
// We let the browser follow that 307 natively so it actually stores the session
// cookie, capture the state nonce off the example.com request, then drive the
// browser straight to the real backend callback
// /auth/callback/google?state=<nonce>&code=... — echoing the state, the real
// OAuth contract. The callback runs the genuine gothic.CompleteUserAuth ->
// FindOrCreateUserWithConsent -> CreateToken -> auth_token cookie -> redirect to
// the frontend. Only the identity (a fixed e2e-oauth@test.local user) is faked.
//
// Why capture-then-navigate rather than page.route intercept: Playwright only
// re-applies route handlers to a redirect hop if the *initiating* request was
// itself intercepted, and example.com is a real reachable page, so routing it
// directly does nothing. Observing the redirect request and re-navigating is the
// robust path and keeps the genuine cookie-setting begin-auth leg intact.
//
// The fixed email matches a pre-seeded user (setup-db.sh), so the first faux
// login resolves to an EXISTING user (a login via linkOAuthAccount), NOT a new
// signup — keeping this spec off the terms/consent path. The signup-consent
// flow is a separate follow-up.

const BACKEND_ORIGIN = 'http://localhost:8080'

test.describe('Google OAuth', () => {
  test('logs in via the faux Google provider', { tag: '@smoke' }, async ({ page }) => {
    await page.goto('/auth')

    // The faux AuthURL host. Capturing this request gives us the state nonce
    // goth generated; the _gothic_session cookie is set on the 307 that points
    // here, so by the time this fires the browser already holds it.
    const fauxRedirect = page.waitForRequest(
      (req) => new URL(req.url()).hostname === 'example.com',
      { timeout: 20_000 }
    )

    await page.getByRole('button', { name: /continue with google/i }).click()

    const fauxReq = await fauxRedirect
    const state = new URL(fauxReq.url()).searchParams.get('state')
    expect(state, 'goth should have produced a state nonce on the faux AuthURL').toBeTruthy()

    // Complete the handshake: hit the real callback with the state echoed back
    // plus an arbitrary code. The _gothic_session cookie (host-scoped to
    // localhost, port-agnostic) rides along, so CompleteUserAuth validates the
    // state, the faux provider returns the fixed user, and the real
    // login + token + auth cookie path runs. The callback 307s to the frontend.
    const callbackUrl = new URL(`${BACKEND_ORIGIN}/auth/callback/google`)
    callbackUrl.searchParams.set('state', state!)
    callbackUrl.searchParams.set('code', 'e2e-faux-auth-code')
    await page.goto(callbackUrl.toString())

    // Lands back on the frontend, off the /auth page.
    await page.waitForURL((url) => !url.pathname.startsWith('/auth'), {
      timeout: 20_000,
    })

    // Logged-in marker present, login link gone — mirrors login.spec.ts.
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('link', { name: /login/i })).not.toBeVisible()
  })
})
