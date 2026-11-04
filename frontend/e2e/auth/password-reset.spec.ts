import { test } from '../fixtures/error-detection'
import { expect, request as playwrightRequest } from '@playwright/test'
import { createPasswordResetToken } from '../helpers/jwt'
import { runE2ESql } from '../e2e-db'
import { BACKEND_BASE_URL } from '../backend-url'

// The emailed password reset, from the link on: no email is delivered in the
// E2E env, so the link is minted the way the outbox mints it
// (createPasswordResetToken), against the account's password hash as it
// stands. The account is a dedicated row this spec creates and resets, so
// changing its password cannot disturb the shared login users.

const RESET_EMAIL = 'e2e-password-reset@test.local'
const SEED_PASSWORD = 'e2e-test-password-123'
const NEW_PASSWORD = 'e2e-reset-new-password-456!'

/**
 * Put the reset account back to the seed password and no revocation, so every
 * attempt (and every CI retry) starts from the same state.
 */
function resetAccount(): { id: number; hash: string } {
  runE2ESql(
    `INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, created_at, updated_at)
     SELECT '${RESET_EMAIL}', password_hash, 'Test', 'Reset', true, false, true, NOW(), NOW()
       FROM users WHERE email = 'e2e-user@test.local'
     ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, sessions_valid_after = NULL`
  )
  const [id, hash] = runE2ESql(`SELECT id || '|' || password_hash FROM users WHERE email = '${RESET_EMAIL}'`).split('|')
  return { id: parseInt(id, 10), hash }
}

async function resetLink(): Promise<string> {
  const { id, hash } = resetAccount()
  const token = await createPasswordResetToken(id, RESET_EMAIL, hash)
  return `/auth/reset-password?token=${token}`
}

test.describe('Password reset', () => {
  test('sets a new password once, signs this browser in and every other session out', async ({ page }) => {
    const link = await resetLink()

    // An earlier session for the account, as another device holds it.
    const otherDevice = await playwrightRequest.newContext({ baseURL: BACKEND_BASE_URL })
    const login = await otherDevice.post('/auth/login', { data: { email: RESET_EMAIL, password: SEED_PASSWORD } })
    expect(login.ok()).toBe(true)
    expect((await otherDevice.get('/auth/profile')).status()).toBe(200)

    await page.goto(link)
    await expect(page.getByRole('heading', { name: 'Reset your password' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(`For ${RESET_EMAIL}`)).toBeVisible()

    await page.getByLabel('New password').fill(NEW_PASSWORD)
    await page.getByLabel('Confirm password').fill(NEW_PASSWORD)
    await page.getByRole('button', { name: 'Set new password' }).click()

    await expect(page.getByRole('heading', { name: 'New password set.' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({ timeout: 10_000 })

    expect((await otherDevice.get('/auth/profile')).status()).toBe(401)
    await otherDevice.dispose()

    await page.getByRole('link', { name: 'Continue' }).click()
    await page.waitForURL('/', { timeout: 10_000 })
    await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({ timeout: 10_000 })

    // The same link a second time.
    await page.goto(link)
    await expect(page.getByRole('heading', { name: 'That link no longer works.' })).toBeVisible({ timeout: 15_000 })
  })

  test('refuses a link that is not a live reset link', async ({ page }) => {
    await page.goto('/auth/reset-password?token=expired-invalid-token')

    await expect(page.getByRole('heading', { name: 'That link no longer works.' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('link', { name: 'Request a new link' })).toHaveAttribute('href', '/auth')
  })
})
