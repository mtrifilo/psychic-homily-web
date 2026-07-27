import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Venue list page', () => {
  test('loads and displays venues', async ({ page }) => {
    await page.goto('/venues')

    // `name` is a case-insensitive SUBSTRING match unless `exact` is set, so a
    // bare `name: 'Venues'` also matched the tag facet panel's "Filter venues
    // by tag" <h2> (VenueList.tsx). That <h2> is client-only, so the collision
    // only appeared once the page had hydrated — which is why this passed in
    // isolation and hit a strict-mode violation in full runs. Keep it pinned.
    await expect(
      page.getByRole('heading', { level: 1, name: 'Venues', exact: true })
    ).toBeVisible()

    // Wait for venue cards to render
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Multiple venues should be visible
    const venueCount = await page.locator('article').count()
    expect(venueCount).toBeGreaterThanOrEqual(3)
  })

  test('venue cards show name, location, and show count', async ({ page }) => {
    await page.goto('/venues')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    const firstVenue = page.locator('article').first()

    // Venue name as heading
    await expect(firstVenue.locator('h2')).toBeVisible()
    await expect(firstVenue.locator('h2')).not.toBeEmpty()

    // Location text (city, state)
    await expect(firstVenue.getByText(/,\s*[A-Z]{2}/)).toBeVisible()

    // Show count badge
    await expect(firstVenue.getByText(/\d+\s+shows?/)).toBeVisible()
  })

})
