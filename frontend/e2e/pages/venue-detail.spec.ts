import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

// The `/shows` -> click first card -> `waitForURL` leg below is shared with
// show-detail, artist-detail, and city-filter. It has flaked in CI under shard
// load. See the FLAKE NOTE at the top of show-detail.spec.ts before changing
// it, and change all four together if you do.
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

    // VenueShowsList renders the upcoming-shows heading unconditionally, while
    // the past-shows `<section>` renders only when the year histogram reports a
    // year with shows (PSY-1753). The E2E seed (setup-db.sh) inserts only
    // future-dated shows, so the past-shows assertion would never resolve here;
    // the archive's year strip, pager and URL state are covered by
    // VenueShowsList.test.tsx instead.
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()
  })

  // "shows tabs switch between upcoming and past" moved to a component test
  // in features/venues/components/VenueDetail.test.tsx per PSY-472.
  // See docs/research/e2e-layer-5-audit.md item #2.
})
