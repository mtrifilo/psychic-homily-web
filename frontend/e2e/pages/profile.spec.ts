import { test, expect } from '../fixtures'

test.describe('Profile page', () => {
  test('displays profile information for authenticated user', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/profile')

    // Heading
    await expect(
      authenticatedPage.getByRole('heading', { name: /my profile/i })
    ).toBeVisible({ timeout: 10_000 })

    // Two tabs
    await expect(
      authenticatedPage.getByRole('tab', { name: /profile/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('tab', { name: /settings/i })
    ).toBeVisible()

    // Profile tab is active by default
    await expect(
      authenticatedPage.getByRole('tab', { name: /profile/i })
    ).toHaveAttribute('data-state', 'active')

    // User email displayed (use first() — also appears in nav link)
    await expect(
      authenticatedPage.getByText('e2e-user@test.local').first()
    ).toBeVisible()

    // First name displayed
    await expect(
      authenticatedPage.getByText('Test', { exact: true })
    ).toBeVisible()
  })

  test('settings tab shows account sections', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/profile?tab=settings')

    await expect(
      authenticatedPage.getByRole('heading', { name: /my profile/i })
    ).toBeVisible({ timeout: 10_000 })

    // Email Verification section with Verified badge
    await expect(
      authenticatedPage.getByText('Email Verification')
    ).toBeVisible({ timeout: 5_000 })
    await expect(
      authenticatedPage.getByText('Verified', { exact: true })
    ).toBeVisible()

    // Change Password section (first() — title + button both match)
    await expect(
      authenticatedPage.getByText('Change Password').first()
    ).toBeVisible()

    // Export Your Data section
    await expect(
      authenticatedPage.getByText('Export Your Data')
    ).toBeVisible()

    // Danger Zone with Delete Account
    await expect(authenticatedPage.getByText('Danger Zone')).toBeVisible()
    await expect(
      authenticatedPage.getByRole('button', { name: /delete account/i })
    ).toBeVisible()
  })

  test('admin user sees admin-only sections', async ({ adminPage }) => {
    await adminPage.goto('/profile?tab=settings')

    await expect(
      adminPage.getByRole('heading', { name: /my profile/i })
    ).toBeVisible({ timeout: 10_000 })

    // API Tokens section (admin-only)
    await expect(
      adminPage.getByText('API Tokens', { exact: true })
    ).toBeVisible({ timeout: 5_000 })

    // CLI Authentication section (admin-only)
    await expect(adminPage.getByText('CLI Authentication')).toBeVisible()
  })
})
