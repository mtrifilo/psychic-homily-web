import { test, expect } from '../fixtures'
import { test as unauthTest } from '../fixtures/error-detection'

test.describe('Submit a show', () => {
  test('displays submission form for verified user', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/submissions')

    // Page heading
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Artists section
    await expect(
      authenticatedPage.getByText('Artists', { exact: true })
    ).toBeVisible({ timeout: 5_000 })

    // Venue input
    await expect(
      authenticatedPage.locator('[id="venue.name"]')
    ).toBeVisible()

    // Date input
    await expect(authenticatedPage.locator('#date')).toBeVisible()

    // Submit button
    await expect(
      authenticatedPage.getByRole('button', { name: 'Submit Show' })
    ).toBeVisible()
  })

  test('can submit a show with existing venue', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/submissions')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Fill artist name
    await authenticatedPage
      .locator('[id="artists[0].name"]')
      .fill('E2E Submitted Artist')

    // Fill venue — type to trigger autocomplete, then select from dropdown
    // pressSequentially keeps focus on the input while typing
    const venueInput = authenticatedPage.locator('[id="venue.name"]')
    await venueInput.click()
    await venueInput.pressSequentially('Valley Bar', { delay: 30 })

    // Wait for autocomplete results to load (confirms API responded)
    await expect(
      authenticatedPage.getByText('Existing Venues')
    ).toBeVisible({ timeout: 5_000 })

    // Click the Valley Bar venue in the autocomplete dropdown.
    // The dropdown button's onMouseDown directly calls handleVenueSelect(venue)
    // with the full venue object (including city/state), bypassing the
    // filteredVenues lookup in handleConfirm. It also sets justSelectedRef
    // which prevents a duplicate handleConfirm call on blur.
    await authenticatedPage
      .getByRole('button', { name: /Valley Bar/ })
      .click()

    // Verify city auto-filled from selected venue
    await expect(
      authenticatedPage.locator('[id="venue.city"]')
    ).toHaveValue('Phoenix', { timeout: 10_000 })

    // Fill date with tomorrow's date
    const tomorrow = new Date()
    tomorrow.setDate(tomorrow.getDate() + 1)
    const dateStr = tomorrow.toISOString().split('T')[0] // YYYY-MM-DD
    await authenticatedPage.locator('#date').fill(dateStr)

    // Time defaults to 20:00 — leave as-is

    // Fill optional cost
    await authenticatedPage.locator('#cost').fill('$15')

    // Submit the form
    await authenticatedPage
      .getByRole('button', { name: 'Submit Show' })
      .click()

    // Wait for success message
    await expect(
      authenticatedPage.getByText('Show Submitted!')
    ).toBeVisible({ timeout: 10_000 })

    // Wait for redirect to /collection
    await authenticatedPage.waitForURL(/\/collection/, { timeout: 10_000 })

    // Verify we're on the collection page
    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })
  })
})

unauthTest.describe('Submit show — unauthenticated', () => {
  unauthTest(
    'redirects unauthenticated user to login',
    async ({ page }) => {
      await page.goto('/submissions')

      // Should redirect to auth page
      await page.waitForURL(/\/auth/, { timeout: 10_000 })

      // Auth page content should be visible
      await expect(
        page.getByText('Sign in to your account')
      ).toBeVisible({ timeout: 5_000 })
    }
  )
})
