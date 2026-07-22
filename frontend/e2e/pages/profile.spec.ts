import { test, expect } from '../fixtures'

test.describe('Profile page', () => {
  test('displays profile information for authenticated user', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/profile')

    // Heading
    await expect(
      authenticatedPage.getByRole('heading', { name: /edit profile & settings/i })
    ).toBeVisible({ timeout: 10_000 })

    // Profile tab is visible and active by default
    await expect(
      authenticatedPage.getByRole('tab', { name: /profile/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('tab', { name: /profile/i })
    ).toHaveAttribute('data-state', 'active')

    // Settings tab is visible
    await expect(
      authenticatedPage.getByRole('tab', { name: /settings/i })
    ).toBeVisible()

    // Email lives on Settings → Account (board J / PSY-1414), not Profile tab.
    // First name displayed in profile header (use first() — also appears in form)
    await expect(
      authenticatedPage.getByText('Test', { exact: true }).first()
    ).toBeVisible()
  })

  test('settings tab shows board J account sections', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/profile?tab=settings')

    await expect(
      authenticatedPage.getByRole('heading', { name: /edit profile & settings/i })
    ).toBeVisible({ timeout: 10_000 })

    // Account card (email + verification fold) — not a standalone "Email Verification"
    await expect(
      authenticatedPage.getByText('Account', { exact: true })
    ).toBeVisible({ timeout: 5_000 })
    await expect(
      authenticatedPage.getByText(/^e2e-user(-[1-4])?@test\.local$/).first()
    ).toBeVisible()
    await expect(
      authenticatedPage.getByText('Verified', { exact: true })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByText('Email Verification')
    ).toHaveCount(0)

    // Passkeys card (PSY-1508)
    await expect(
      authenticatedPage.getByText('Passkeys', { exact: true })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByText(
        'Sign in with Touch ID, Face ID, or a security key.'
      )
    ).toBeVisible()

    // Change password section (first() — title + button both match)
    await expect(
      authenticatedPage.getByText('Change password').first()
    ).toBeVisible()

    // Export your data section
    await expect(
      authenticatedPage.getByText('Export your data')
    ).toBeVisible()

    // Danger zone with Delete account
    await expect(authenticatedPage.getByText('Danger zone')).toBeVisible()
    await expect(
      authenticatedPage.getByRole('button', { name: /delete account/i })
    ).toBeVisible()
  })

  test('admin user sees admin-only sections', async ({ adminPage }) => {
    await adminPage.goto('/profile?tab=settings')

    await expect(
      adminPage.getByRole('heading', { name: /edit profile & settings/i })
    ).toBeVisible({ timeout: 10_000 })

    // API tokens section (admin-only)
    await expect(
      adminPage.getByText('API tokens', { exact: true })
    ).toBeVisible({ timeout: 5_000 })

    // CLI authentication section (admin-only)
    await expect(adminPage.getByText('CLI authentication')).toBeVisible()
  })

  test('hash deep-link focuses the Username field', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/profile#username')

    await expect(
      authenticatedPage.getByRole('heading', { name: /edit profile & settings/i })
    ).toBeVisible({ timeout: 10_000 })

    const username = authenticatedPage.getByLabel(/^username$/i)
    await expect(username).toBeFocused({ timeout: 5_000 })
  })

  test('hash deep-link focuses the Bio field', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/profile#bio')

    await expect(
      authenticatedPage.getByRole('heading', { name: /edit profile & settings/i })
    ).toBeVisible({ timeout: 10_000 })

    const bio = authenticatedPage.getByLabel(/^bio$/i)
    await expect(bio).toBeFocused({ timeout: 5_000 })
  })
})
