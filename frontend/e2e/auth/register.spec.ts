import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

const REGISTER_USER = {
  email: 'e2e-register@test.local',
  password: 'Xq9!mzPh2wLk_e2e',
}

test.describe('Registration', () => {
  test('registers a new account and redirects to home', async ({ page }) => {
    await page.goto('/auth')

    // Switch to signup tab
    await page.getByRole('tab', { name: 'Create account' }).click()

    // Wait for signup form to render
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    // Fill registration form
    await page.locator('#signup-email').fill(REGISTER_USER.email)
    await page.locator('#signup-password').fill(REGISTER_USER.password)
    await page.locator('#terms').check()

    // Submit
    await page.getByRole('button', { name: 'Create account' }).click()

    // Redirects away from /auth on success
    await page.waitForURL((url) => !url.pathname.startsWith('/auth'), {
      timeout: 15_000,
    })

    // Nav shows avatar dropdown
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

    // Submit — server will reject the breached password
    await page.getByRole('button', { name: 'Create account' }).click()

    // Error alert should appear with breach message
    await expect(page.getByRole('alert')).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByText(/password has been exposed in a data breach/i)
    ).toBeVisible()

    // Still on /auth
    expect(page.url()).toContain('/auth')
  })
})
