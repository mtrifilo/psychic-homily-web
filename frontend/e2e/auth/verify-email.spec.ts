import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { createVerificationToken } from '../helpers/jwt'
import { execSync } from 'child_process'

const UNVERIFIED_USER_EMAIL = 'e2e-unverified@test.local'
const E2E_DB_URL =
  'postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable'

/** Look up the unverified user's ID directly from the database (avoids rate-limited auth endpoints). */
function getUnverifiedUserId(): number {
  const result = execSync(
    `psql "${E2E_DB_URL}" -tAc "SELECT id FROM users WHERE email = '${UNVERIFIED_USER_EMAIL}'"`,
    { encoding: 'utf-8' }
  ).trim()
  return parseInt(result, 10)
}

test.describe('Email Verification', () => {
  test('verifies email with valid token', async ({ page }) => {
    const userId = getUnverifiedUserId()

    // Generate a valid verification JWT
    const token = await createVerificationToken(userId, UNVERIFIED_USER_EMAIL)

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
