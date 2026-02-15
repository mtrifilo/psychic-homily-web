import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

const TEST_USER = {
  email: 'e2e-user@test.local',
  password: 'e2e-test-password-123',
}

test.describe('Login', () => {
  test('logs in with valid credentials and redirects to home', async ({
    page,
  }) => {
    await page.goto('/auth')

    // Auth page heading
    await expect(
      page.getByRole('heading', { name: /welcome to psychic homily/i })
    ).toBeVisible()

    // Fill and submit
    await page.getByLabel('Email').fill(TEST_USER.email)
    await page.locator('#password').fill(TEST_USER.password)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()

    // Redirects away from /auth
    await page.waitForURL((url) => !url.pathname.startsWith('/auth'), {
      timeout: 15_000,
    })

    // Nav shows avatar dropdown instead of login link
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible()
    await expect(page.getByRole('link', { name: /login/i })).not.toBeVisible()
  })

  test('shows error for invalid credentials', async ({ page }) => {
    await page.goto('/auth')

    await page.getByLabel('Email').fill('wrong@example.com')
    await page.locator('#password').fill('wrong-password-here')
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()

    // Error alert appears
    await expect(page.getByRole('alert')).toBeVisible({ timeout: 10_000 })
  })

  test('shows validation error for empty password', async ({ page }) => {
    await page.goto('/auth')

    await page.getByLabel('Email').fill(TEST_USER.email)
    // Leave password empty, submit
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()

    // Validation message appears
    await expect(page.getByText(/password is required/i)).toBeVisible()

    // Still on /auth
    expect(page.url()).toContain('/auth')
  })

  test('logout returns to unauthenticated state', async ({ page }) => {
    // Log in first
    await page.goto('/auth')
    await page.getByLabel('Email').fill(TEST_USER.email)
    await page.locator('#password').fill(TEST_USER.password)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await page.waitForURL((url) => !url.pathname.startsWith('/auth'), {
      timeout: 15_000,
    })

    // Open avatar dropdown and click Sign out
    await page.getByRole('button', { name: 'User menu' }).click()
    await page.getByRole('menuitem', { name: /sign out/i }).click()

    // Login link reappears
    await expect(page.getByRole('link', { name: /login/i })).toBeVisible({
      timeout: 5_000,
    })

    // Avatar button no longer visible
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).not.toBeVisible()
  })
})
