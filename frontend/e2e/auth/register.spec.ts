import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

const REGISTER_USER = {
  email: 'e2e-register@test.local',
  password: 'Xq9!mzPh2wLk_e2e',
}

test.describe('Registration', () => {
  test('registers a new account and lands on the check-your-inbox interstitial', { tag: '@smoke' }, async ({ page }) => {
    await page.goto('/auth')

    // Switch to signup tab
    await page.getByRole('tab', { name: 'Create account' }).click()

    // Wait for signup form to render
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    // Fill registration form
    await page.locator('#signup-email').fill(REGISTER_USER.email)
    await page.locator('#signup-password').fill(REGISTER_USER.password)
    await page.locator('#terms').check()
    // PSY-1023: signup now requires a 16+ age confirmation alongside terms.
    await page.locator('#age-confirmation').check()

    // Submit
    await page.getByRole('button', { name: 'Create account' }).click()

    // PSY-1900: signup no longer navigates on success. The card is replaced
    // in place by the check-your-inbox interstitial, which names the address
    // the verification link went to.
    await expect(
      page.getByRole('heading', { name: 'Check your inbox.' })
    ).toBeVisible({ timeout: 15_000 })
    // `exact` keeps this off the surrounding sentence, which also contains the
    // address, so strict mode sees one node rather than two.
    await expect(
      page.getByText(REGISTER_USER.email, { exact: true })
    ).toBeVisible()
    await expect(page.getByText(/It expires in 24 hours/)).toBeVisible()
    await expect(
      page.getByRole('link', { name: 'Browse upcoming shows' })
    ).toBeVisible()

    // The session is live even though the user has not verified yet.
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('shows password strength requirements', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(page.locator('#signup-password')).toBeVisible({
      timeout: 5_000,
    })

    // Type a short password to trigger the strength meter
    await page.locator('#signup-password').fill('short')

    // Password strength meter should show unmet requirements
    await expect(
      page.getByText('At least 12 characters')
    ).toBeVisible({ timeout: 5_000 })

    // Submit button should be disabled with invalid password
    await expect(
      page.getByRole('button', { name: 'Create account' })
    ).toBeDisabled()

    // Still on /auth
    expect(page.url()).toContain('/auth')
  })

  test('shows error for breached password', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    // Fill form with a commonly breached password
    await page.locator('#signup-email').fill('breach-test@test.local')
    await page.locator('#signup-password').fill('TestPassword123!')
    await page.locator('#terms').check()
    // PSY-1023: signup now requires a 16+ age confirmation alongside terms.
    await page.locator('#age-confirmation').check()

    // Submit — server will reject the breached password
    await page.getByRole('button', { name: 'Create account' }).click()

    // PSY-474: can't use `page.getByRole('alert')` unscoped — Next.js's
    // RouteAnnouncer renders a permanent empty `role="alert"` live region
    // at the page root for route narration, so any form-level alert makes
    // the selector match 2 elements and trip strict-mode. The text check
    // below is content-specific and sufficient on its own.
    await expect(
      page.getByText(/password has been exposed in a data breach/i)
    ).toBeVisible({ timeout: 10_000 })

    // Still on /auth
    expect(page.url()).toContain('/auth')
  })
})
