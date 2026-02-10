import { test, expect } from '../fixtures'

test.describe('Admin: Venue Edits', () => {
  test.describe.configure({ mode: 'serial' })

  test('displays pending venue edits', async ({ adminPage }) => {
    await adminPage.goto('/admin/venue-edits')

    // Page heading
    await expect(
      adminPage.getByRole('heading', { level: 2, name: 'Pending Venue Edits' })
    ).toBeVisible({ timeout: 10_000 })

    // Badge visible
    await expect(adminPage.getByText('Pending Edit').first()).toBeVisible()

    // Proposed address change visible (from ChangeDiff component)
    await expect(adminPage.getByText('123 Updated Address')).toBeVisible()

    // Submitter name visible
    await expect(adminPage.getByText('Test User').first()).toBeVisible()
  })

  test('can approve a venue edit', async ({ adminPage }) => {
    await adminPage.goto('/admin/venue-edits')

    // Wait for page to load
    await expect(
      adminPage.getByText('123 Updated Address')
    ).toBeVisible({ timeout: 10_000 })

    // Find the card with the address edit and click Approve
    const card = adminPage
      .locator('[class*="card"], article')
      .filter({ hasText: '123 Updated Address' })
    await card.getByRole('button', { name: 'Approve' }).click()

    // Dialog opens
    const dialog = adminPage.getByRole('dialog', { name: 'Approve Venue Edit' })
    await expect(
      dialog.getByRole('heading', { name: 'Approve Venue Edit' })
    ).toBeVisible({ timeout: 5_000 })

    // Click confirm
    await dialog.getByRole('button', { name: 'Approve Changes' }).click()

    // Edit disappears
    await expect(
      adminPage.getByText('123 Updated Address')
    ).not.toBeVisible({ timeout: 10_000 })

    // Other edit still visible
    await expect(
      adminPage.getByText('Renamed Venue E2E')
    ).toBeVisible()
  })

  test('can reject a venue edit with reason', async ({ adminPage }) => {
    await adminPage.goto('/admin/venue-edits')

    // Wait for page to load
    await expect(
      adminPage.getByText('Renamed Venue E2E')
    ).toBeVisible({ timeout: 10_000 })

    // Find the card and click Reject
    const card = adminPage
      .locator('[class*="card"], article')
      .filter({ hasText: 'Renamed Venue E2E' })
    await card.getByRole('button', { name: 'Reject' }).click()

    // Dialog opens
    const dialog = adminPage.getByRole('dialog', { name: 'Reject Venue Edit' })
    await expect(
      dialog.getByRole('heading', { name: 'Reject Venue Edit' })
    ).toBeVisible({ timeout: 5_000 })

    // Reject button disabled until reason provided
    const rejectButton = dialog.getByRole('button', { name: 'Reject Edit' })
    await expect(rejectButton).toBeDisabled()

    // Fill in rejection reason
    await dialog.locator('#rejection-reason').fill('Inaccurate information')

    // Reject button now enabled
    await expect(rejectButton).toBeEnabled()
    await rejectButton.click()

    // Edit disappears, empty state appears
    await expect(
      adminPage.getByText('Renamed Venue E2E')
    ).not.toBeVisible({ timeout: 10_000 })
    await expect(
      adminPage.getByRole('heading', { level: 3, name: 'No Pending Venue Edits' })
    ).toBeVisible()
  })
})
