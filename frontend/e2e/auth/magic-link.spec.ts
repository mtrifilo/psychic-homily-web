import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { createMagicLinkToken } from '../helpers/jwt'
import { execSync } from 'child_process'

const TEST_USER_EMAIL = 'e2e-user@test.local'
const E2E_DB_URL =
  'postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable'

/** Look up a user's ID directly from the database (avoids rate-limited auth endpoints). */
function getUserId(email: string): number {
  const result = execSync(
    `psql "${E2E_DB_URL}" -tAc "SELECT id FROM users WHERE email = '${email}'"`,
    { encoding: 'utf-8' }
  ).trim()
  return parseInt(result, 10)
}

test.describe('Magic Link Authentication', () => {
  test('authenticates user with valid magic link', { tag: '@smoke' }, async ({ page }) => {
    const userId = getUserId(TEST_USER_EMAIL)

    // Generate a valid magic link JWT
    const token = await createMagicLinkToken(userId, TEST_USER_EMAIL)

    // Navigate to magic link page (the page fixture has no auth)
    await page.goto(`/auth/magic-link?token=${token}`)

    // Assert success
    await expect(page.getByText('Welcome back!')).toBeVisible({
      timeout: 15_000,
    })

    // Wait for redirect to homepage (happens after 1.5s)
    await page.waitForURL('/', { timeout: 10_000 })

    // Assert homepage loaded
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible({ timeout: 10_000 })
  })

  test('completes sign-in: session persists past the redirect', { tag: '@smoke' }, async ({
    page,
  }) => {
    // PSY-719: the success-message + redirect assertion above covers the
    // *verification* leg. This asserts *completion* — that the authenticated
    // session actually took effect: after the redirect to home, the logged-in
    // marker (avatar dropdown) is present and the login link is gone, mirroring
    // the email/password login spec's post-login assertion.
    const userId = getUserId(TEST_USER_EMAIL)
    const token = await createMagicLinkToken(userId, TEST_USER_EMAIL)

    await page.goto(`/auth/magic-link?token=${token}`)
    await page.waitForURL('/', { timeout: 15_000 })

    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('link', { name: /login/i })
    ).not.toBeVisible()
  })

  // The link is asked for from a page that needs sign-in and opened in a
  // browser that never saw the request, so the only thing that can carry the
  // destination is the link itself. The E2E backend has no email provider, so
  // the send is answered here, and the outbox row the backend's send writes
  // and the token its poller mints are made the same way: the row holds the
  // destination the request carried and the token names the row.
  test('returns a link asked for from a deep page to that page, opened in another browser', async ({
    page,
    browser,
  }) => {
    const userId = getUserId(TEST_USER_EMAIL)
    let sendBody: { email?: string; return_to?: string } | null = null
    await page.route('**/auth/magic-link/send', async route => {
      sendBody = route.request().postDataJSON()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          message: 'If an account exists with this email, a magic link has been sent.',
        }),
      })
    })

    await page.goto('/library')
    await page.waitForURL(/\/auth\?returnTo=%2Flibrary/, { timeout: 15_000 })
    await page.locator('#email').fill(TEST_USER_EMAIL)
    await page.getByRole('button', { name: 'Email me a sign-in link instead' }).click()
    await expect(page.getByText(/Check your email for a sign-in link/)).toBeVisible({
      timeout: 10_000,
    })
    expect(sendBody).toEqual({ email: TEST_USER_EMAIL, return_to: '/library' })

    const destinationRef = parseInt(
      execSync(
        `psql "${E2E_DB_URL}" -tAc "INSERT INTO auth_email_outbox (kind, user_id, status, return_to) VALUES ('magic_link', ${userId}, 'sent', '/library') RETURNING id"`,
        { encoding: 'utf-8' }
      ).trim(),
      10
    )
    const token = await createMagicLinkToken(userId, TEST_USER_EMAIL, destinationRef)
    const otherBrowser = await browser.newContext()
    try {
      const opened = await otherBrowser.newPage()
      // A destination added to the emailed URL is ignored: the link's own wins.
      await opened.goto(`/auth/magic-link?token=${token}&returnTo=%2Fvenues`)
      await expect(opened.getByText('Welcome back!')).toBeVisible({ timeout: 15_000 })
      await opened.waitForURL(url => url.pathname === '/library', { timeout: 10_000 })
      await expect(opened.getByRole('button', { name: 'User menu' })).toBeVisible({
        timeout: 10_000,
      })
    } finally {
      await otherBrowser.close()
    }

    // The tab that asked keeps its sent state; it shares no session with the
    // browser that opened the link.
    await expect(page.getByText(/Check your email for a sign-in link/)).toBeVisible()
  })

  test('shows error for expired/invalid magic link', async ({ page }) => {
    await page.goto('/auth/magic-link?token=expired-invalid-token')

    await expect(page.getByText('Link Expired')).toBeVisible({
      timeout: 15_000,
    })
    await expect(
      page.getByRole('button', { name: 'Back to Sign In' })
    ).toBeVisible()
  })

  test('shows invalid link when no token provided', async ({ page }) => {
    await page.goto('/auth/magic-link')

    await expect(page.getByText('Invalid Link')).toBeVisible({
      timeout: 10_000,
    })
    await expect(
      page.getByRole('button', { name: 'Back to Sign In' })
    ).toBeVisible()
  })
})
