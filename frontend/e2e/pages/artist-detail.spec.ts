import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import {
  CLIENT_NAVIGATION_TIMEOUT_MS,
  SEEDED_DETAIL_PATHS,
  warmRoutes,
} from '../helpers/navigation'

// The `/shows` -> click first card -> `waitForURL` leg below is shared with
// show-detail, venue-detail, and city-filter; see the note at the top of
// show-detail.spec.ts and keep the four in step.
test.describe('Artist detail', () => {
  test.beforeAll(async ({ browser }, testInfo) => {
    await warmRoutes(browser, testInfo, [
      '/shows',
      SEEDED_DETAIL_PATHS.show,
      SEEDED_DETAIL_PATHS.artist,
    ])
  })

  test('displays artist information with shows tabs', async ({ page }) => {
    // Navigate: shows list → show detail → artist link
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
    await page.waitForURL(/\/shows\//, {
      timeout: CLIENT_NAVIGATION_TIMEOUT_MS,
    })

    // Wait for show detail to load, then click artist link
    const artistLink = page.locator('a[href^="/artists/"]').first()
    await expect(artistLink).toBeVisible({ timeout: 10_000 })
    const artistName = await artistLink.textContent()
    await artistLink.click()

    await page.waitForURL(/\/artists\//, {
      timeout: CLIENT_NAVIGATION_TIMEOUT_MS,
    })

    // H1 with artist name
    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).toContainText(artistName!)

    // Breadcrumb link to Artists list
    const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]')
    await expect(breadcrumbNav.getByRole('link', { name: 'Artists' })).toBeVisible()

    // ArtistShowsList renders the upcoming-shows heading unconditionally, while
    // the past-shows `<section>` renders only once EITHER the year histogram
    // reports a year with shows OR, while that request is still in flight or
    // has failed, the first page request reports a non-zero total (PSY-1754).
    // The E2E seed (setup-db.sh) inserts only future-dated shows, so the
    // past-shows assertion would never resolve here; the archive's year strip,
    // pager and URL state are covered by ArtistShowsList.test.tsx instead.
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()
  })

  // "shows tabs switch between upcoming and past" moved to a component test
  // in features/artists/components/ArtistDetail.test.tsx per PSY-472.
  // See docs/research/e2e-layer-5-audit.md item #2.
})
