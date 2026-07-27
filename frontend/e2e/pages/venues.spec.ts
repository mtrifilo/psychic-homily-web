import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Venue list page', () => {
  test('loads and displays venues', async ({ page }) => {
    await page.goto('/venues')

    // `name` matches the accessible name case-insensitively AS A SUBSTRING
    // unless `exact` is set, so a bare `name: 'Venues'` also matches the tag
    // facet panel's "Filter venues by tag" <h2> (VenueList.tsx). That <h2> is
    // client-only — absent from the SSR HTML — so whether the assertion saw
    // one heading or two came down to whether hydration had landed yet, which
    // in turn came down to whether an earlier spec had already compiled
    // /venues in the dev server. Hence "passes alone, strict-mode violation in
    // a full run". Pin to the page <h1> so no hydration timing can match two.
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
