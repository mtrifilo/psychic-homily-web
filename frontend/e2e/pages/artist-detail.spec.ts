import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Artist detail', () => {
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
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Wait for show detail to load, then click artist link
    const artistLink = page.locator('a[href^="/artists/"]').first()
    await expect(artistLink).toBeVisible({ timeout: 10_000 })
    const artistName = await artistLink.textContent()
    await artistLink.click()

    await page.waitForURL(/\/artists\//, { timeout: 10_000 })

    // H1 with artist name
    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).toContainText(artistName!)

    // Breadcrumb link to Artists list
    const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]')
    await expect(breadcrumbNav.getByRole('link', { name: 'Artists' })).toBeVisible()

    // Upcoming and Past Shows tabs (nested inside the Overview tab content)
    await expect(page.getByRole('tab', { name: /upcoming/i })).toBeVisible()
    await expect(
      page.getByRole('tab', { name: /past shows/i })
    ).toBeVisible()
  })

  // "shows tabs switch between upcoming and past" moved to a component test
  // in features/artists/components/ArtistDetail.test.tsx per PSY-472.
  // See docs/research/e2e-layer-5-audit.md item #2.
})
