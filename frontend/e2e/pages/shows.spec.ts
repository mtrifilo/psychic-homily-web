import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Shows list', () => {
  test('loads and displays upcoming shows', async ({ page }) => {
    await page.goto('/shows')

    await expect(page).toHaveTitle(/Upcoming Shows/)

    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()

    // Wait for show cards to render (client-side fetch)
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Multiple shows visible
    const showCount = await page.locator('article').count()
    expect(showCount).toBeGreaterThanOrEqual(5)
  })

  test('show cards contain artist links, venue, and details link', async ({
    page,
  }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    const firstShow = page.locator('article').first()

    // Has at least one link (artist or venue)
    await expect(firstShow.locator('a').first()).toBeVisible()

    // Has a "Details" link pointing to /shows/...
    await expect(
      firstShow.getByRole('link', { name: 'Details' })
    ).toBeVisible()
  })

  test('pagination loads more shows', async ({ page }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Wait for "Load More" to appear (API returns has_more: true with >10 shows)
    const loadMoreButton = page.getByRole('button', { name: /load more/i })
    await expect(loadMoreButton).toBeVisible({ timeout: 5_000 })

    const initialCount = await page.locator('article').count()
    expect(initialCount).toBe(50) // Backend default limit

    await loadMoreButton.click()

    // Wait for additional shows to load
    await page.waitForFunction(
      (initial) => document.querySelectorAll('article').length > initial,
      initialCount,
      { timeout: 10_000 }
    )

    const newCount = await page.locator('article').count()
    expect(newCount).toBeGreaterThan(initialCount)
  })

  test('show detail link navigates correctly', async ({ page }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Click the "Details" link on the first show
    const detailsLink = page
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })

    const href = await detailsLink.getAttribute('href')
    expect(href).toMatch(/^\/shows\//)

    await detailsLink.click()

    // Should navigate to /shows/<slug-or-id>
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })
  })
})
