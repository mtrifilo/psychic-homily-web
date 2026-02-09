import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { createVerificationToken } from '../helpers/jwt'

const UNVERIFIED_USER = {
  email: 'e2e-unverified@test.local',
  password: 'e2e-test-password-123',
}

test.describe('Email Verification', () => {
  test('verifies email with valid token', async ({ page }) => {
    // Log in as unverified user via API to get their user ID
    const loginResponse = await page.request.post(
      'http://localhost:8080/auth/login',
      {
        data: {
          email: UNVERIFIED_USER.email,
          password: UNVERIFIED_USER.password,
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

    // Generate a valid verification JWT
    const token = await createVerificationToken(userId, UNVERIFIED_USER.email)

    // Navigate to the verify-email page with the token
    await page.goto(`/verify-email?token=${token}`)

    // Assert success state
    await expect(page.getByText('Email Verified!')).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByRole('link', { name: 'Submit a Show' })).toBeVisible()
    await expect(
      page.getByRole('link', { name: 'Go to My Collection' })
    ).toBeVisible()
  })

  test('shows error for invalid token', async ({ page }) => {
    await page.goto('/verify-email?token=invalid-garbage-token')

    await expect(page.getByText('Verification Failed')).toBeVisible({
      timeout: 15_000,
    })
    await expect(
      page.getByRole('link', { name: 'Request New Verification Email' })
    ).toBeVisible()
  })

  test('shows invalid link when no token provided', async ({ page }) => {
    await page.goto('/verify-email')

    await expect(page.getByText('Invalid Verification Link')).toBeVisible({
      timeout: 10_000,
    })
    await expect(
      page.getByRole('link', { name: 'Go to Settings' })
    ).toBeVisible()
  })
})
