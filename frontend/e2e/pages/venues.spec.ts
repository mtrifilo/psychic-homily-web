import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Venue list page', () => {
  test('loads and displays venues', async ({ page }) => {
    await page.goto('/venues')

    await expect(page.getByRole('heading', { name: 'Venues' })).toBeVisible()

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

  test('venue name links to detail page', async ({ page }) => {
    await page.goto('/venues')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Find a venue with a link to its detail page
    const venueLink = page.locator('article').first().locator('a[href^="/venues/"]').first()
    await expect(venueLink).toBeVisible()

    await venueLink.click()
    await page.waitForURL(/\/venues\//, { timeout: 10_000 })

    // Should be on a venue detail page
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
      timeout: 10_000,
    })
  })
})
