import { test, expect } from '../fixtures'

test.describe('Admin: Verify Venue', () => {
  test.describe.configure({ mode: 'serial' })

  test('displays unverified venues list', async ({ adminPage }) => {
    await adminPage.goto('/admin/unverified-venues')

    // Page heading
    await expect(
      adminPage.getByRole('heading', { level: 2, name: 'Unverified Venues' })
    ).toBeVisible({ timeout: 10_000 })

    // Seeded unverified venue visible
    await expect(
      adminPage.getByText('E2E Unverified Venue')
    ).toBeVisible()

    // Badge, location, and verify button visible
    await expect(
      adminPage.getByText('Unverified', { exact: true })
    ).toBeVisible()
    await expect(adminPage.getByText('Phoenix, AZ').first()).toBeVisible()
    await expect(
      adminPage.getByRole('button', { name: 'Verify Venue' })
    ).toBeVisible()
  })

  test('can verify an unverified venue', async ({ adminPage }) => {
    await adminPage.goto('/admin/unverified-venues')

    // Wait for venue to load
    await expect(
      adminPage.getByText('E2E Unverified Venue')
    ).toBeVisible({ timeout: 10_000 })

    // Click Verify Venue button
    await adminPage
      .getByRole('button', { name: 'Verify Venue' })
      .click()

    // Dialog opens
    const dialog = adminPage.getByRole('dialog', { name: 'Verify Venue' })
    await expect(
      dialog.getByRole('heading', { name: 'Verify Venue' })
    ).toBeVisible({ timeout: 5_000 })

    // Dialog shows the address
    await expect(dialog.getByText('999 Test Street')).toBeVisible()

    // Click confirm button (scoped to dialog to avoid ambiguity)
    await dialog.getByRole('button', { name: 'Verify Venue' }).click()

    // Venue disappears, empty state appears
    await expect(
      adminPage.getByRole('heading', { name: 'E2E Unverified Venue' })
    ).not.toBeVisible({ timeout: 10_000 })
    await expect(
      adminPage.getByRole('heading', { level: 3, name: 'All Venues Verified' })
    ).toBeVisible()
  })
})
