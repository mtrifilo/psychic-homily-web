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

    // Assert success state. Scope CTA assertions to <main> so sidebar nav links
    // (PSY-600) don't trip strict-mode resolution.
    await expect(
      page.getByRole('heading', { name: 'Welcome to the index.' })
    ).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('ALERTS', { exact: true })).toBeVisible()
    await expect(
      page.getByRole('main').getByRole('link', { name: 'Browse shows near you' })
    ).toBeVisible()
    await expect(
      page.getByRole('main').getByRole('link', { name: 'Explore artists' })
    ).toBeVisible()
  })

  test('shows the expired card for an invalid token', async ({ page }) => {
    await page.goto('/verify-email?token=invalid-garbage-token')

    await expect(
      page.getByRole('heading', { name: 'That link has expired.' })
    ).toBeVisible({ timeout: 15_000 })
    // Signed out, so the fresh-link action routes through sign-in rather than
    // offering a resend the API would only 401.
    await expect(
      page.getByRole('main').getByRole('link', {
        name: 'Sign in to send a fresh link',
      })
    ).toBeVisible()
  })

  test('does not claim expiry when no token is provided', async ({ page }) => {
    await page.goto('/verify-email')

    await expect(
      page.getByRole('heading', { name: 'That link is not valid.' })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('main').getByRole('link', {
        name: 'Sign in to send a fresh link',
      })
    ).toBeVisible()
  })
})
