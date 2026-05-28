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
