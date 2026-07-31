import { test, expect } from '../fixtures'
import { restoreUnverifiedVenueSeed } from '../fixtures/seed-restore'

test.describe('Admin: Verify Venue', () => {
  test.describe.configure({ mode: 'serial' })

  // PSY-1663: verifying a venue is one-way — no product surface un-verifies
  // one, and the fixture-reset endpoint only deletes rows owned by a seeded
  // worker user, so it cannot restore an admin fixture either. Without this
  // hook, a failure anywhere after the verify click left BOTH tests asserting
  // against a state the database could never return to, and every retry was
  // guaranteed to fail. Restoring at entry is a no-op on a clean database, so
  // the first attempt still exercises the real flow.
  test.beforeEach(() => {
    restoreUnverifiedVenueSeed()
  })

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
