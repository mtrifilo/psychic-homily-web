import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Venue detail', () => {
  test('displays venue information with shows tabs', async ({ page }) => {
    // Navigate: shows list → show detail → venue link
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    await page
      .locator('article')
      .first()
      .locator('a[href^="/shows/"]')
      .first()
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Wait for show detail to load, then click venue link
    const venueLink = page.locator('a[href^="/venues/"]').first()
    await expect(venueLink).toBeVisible({ timeout: 10_000 })
    const venueName = await venueLink.textContent()
    await venueLink.click()

    await page.waitForURL(/\/venues\//, { timeout: 10_000 })

    // H1 with venue name
    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).toContainText(venueName!)

    // Breadcrumb link to Venues list
    const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]')
    await expect(breadcrumbNav.getByRole('link', { name: 'Venues' })).toBeVisible()

    // VenueShowsList renders the upcoming-shows heading unconditionally and
    // the past-shows `<section>` only when `pastShows.length > 0`. The E2E
    // seed (setup-db.sh) inserts only future-dated shows, so the past-shows
    // assertion would never resolve here; collapsible-trigger behaviour is
    // covered by VenueShowsList.test.tsx instead.
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()
  })

  // "shows tabs switch between upcoming and past" moved to a component test
  // in features/venues/components/VenueDetail.test.tsx per PSY-472.
  // See docs/research/e2e-layer-5-audit.md item #2.
})
