import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { createMagicLinkToken } from '../helpers/jwt'

const TEST_USER = {
  email: 'e2e-user@test.local',
  password: 'e2e-test-password-123',
}

test.describe('Magic Link Authentication', () => {
  test('authenticates user with valid magic link', async ({ page }) => {
    // Get user ID by logging in via API
    const loginResponse = await page.request.post(
      'http://localhost:8080/auth/login',
      {
        data: {
          email: TEST_USER.email,
          password: TEST_USER.password,
        },
      }
    )
    expect(loginResponse.ok()).toBe(true)

    const profileResponse = await page.request.get(
      'http://localhost:8080/auth/profile'
    )
    expect(profileResponse.ok()).toBe(true)
    const profile = await profileResponse.json()
    const userId = profile.user.id

    // Generate a valid magic link JWT
    const token = await createMagicLinkToken(userId, TEST_USER.email)

    // Navigate to magic link page in a fresh context (the page fixture has no auth)
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
