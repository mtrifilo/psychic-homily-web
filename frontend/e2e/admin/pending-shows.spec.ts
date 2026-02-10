import { test, expect } from '../fixtures'

test.describe('Admin: Pending Shows', () => {
  test.describe.configure({ mode: 'serial' })

  test('displays pending shows for admin review', async ({ adminPage }) => {
    await adminPage.goto('/admin/pending-shows')

    // Page heading
    await expect(
      adminPage.getByRole('heading', { level: 2, name: 'Show Review' })
    ).toBeVisible({ timeout: 10_000 })

    // Both seeded pending shows visible
    await expect(
      adminPage.getByText('E2E Pending Show Approve')
    ).toBeVisible()
    await expect(
      adminPage.getByText('E2E Pending Show Reject')
    ).toBeVisible()

    // Badge and action buttons visible
    await expect(adminPage.getByText('Pending Review').first()).toBeVisible()
    await expect(
      adminPage.getByRole('button', { name: 'Approve' }).first()
    ).toBeVisible()
    await expect(
      adminPage.getByRole('button', { name: 'Reject' }).first()
    ).toBeVisible()
  })

  test('can approve a pending show', async ({ adminPage }) => {
    await adminPage.goto('/admin/pending-shows')

    // Wait for page to load
    await expect(
      adminPage.getByRole('heading', { name: 'E2E Pending Show Approve' })
    ).toBeVisible({ timeout: 10_000 })

    // Find the card containing the approve show and click its Approve button
    const card = adminPage
      .locator('[class*="card"], article')
      .filter({ hasText: 'E2E Pending Show Approve' })
    await card.getByRole('button', { name: 'Approve' }).click()

    // Dialog opens
    const dialog = adminPage.getByRole('dialog', { name: 'Approve Show' })
    await expect(
      dialog.getByRole('heading', { name: 'Approve Show' })
    ).toBeVisible({ timeout: 5_000 })

    // All venues are verified, so we see the "already verified" message
    await expect(
      dialog.getByText('already verified')
    ).toBeVisible()

    // Click dialog's Approve button
    await dialog.getByRole('button', { name: 'Approve' }).click()

    // Show disappears from list
    await expect(
      adminPage.getByRole('heading', { name: 'E2E Pending Show Approve' })
    ).not.toBeVisible({ timeout: 10_000 })

    // Other show still visible
    await expect(
      adminPage.getByRole('heading', { name: 'E2E Pending Show Reject' })
    ).toBeVisible()
  })

  test('can reject a pending show with reason', async ({ adminPage }) => {
    await adminPage.goto('/admin/pending-shows')

    // Wait for page to load
    await expect(
      adminPage.getByRole('heading', { name: 'E2E Pending Show Reject' })
    ).toBeVisible({ timeout: 10_000 })

    // Find the card and click Reject
    const card = adminPage
      .locator('[class*="card"], article')
      .filter({ hasText: 'E2E Pending Show Reject' })
    await card.getByRole('button', { name: 'Reject' }).click()

    // Dialog opens
    const dialog = adminPage.getByRole('dialog', { name: 'Reject Show' })
    await expect(
      dialog.getByRole('heading', { name: 'Reject Show' })
    ).toBeVisible({ timeout: 5_000 })

    // Reject button is disabled until a reason is provided
    const rejectButton = dialog.getByRole('button', { name: 'Reject Show' })
    await expect(rejectButton).toBeDisabled()

    // Fill in rejection reason
    await dialog.locator('#rejection-reason').fill('Duplicate show submission')

    // Reject button now enabled
    await expect(rejectButton).toBeEnabled()
    await rejectButton.click()

    // Show disappears, empty state appears
    await expect(
      adminPage.getByRole('heading', { name: 'E2E Pending Show Reject' })
    ).not.toBeVisible({ timeout: 10_000 })
    await expect(
      adminPage.getByRole('heading', { level: 3, name: 'No Pending Shows' })
    ).toBeVisible()
  })
})
