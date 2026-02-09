import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('City filter on shows list', () => {
  test('city filter chips are visible', async ({ page }) => {
    await page.goto('/shows')

    // Wait for shows to load first
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // "All Cities" button should be visible
    await expect(
      page.getByRole('button', { name: /all cities/i })
    ).toBeVisible({ timeout: 5_000 })

    // At least one city chip should be visible (shows count in parens)
    await expect(
      page.getByRole('button', { name: /Phoenix/i })
    ).toBeVisible()
  })

  test('clicking a city filter updates URL and filters shows', async ({
    page,
  }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Record initial count (should be 50, the pagination limit)
    const initialCount = await page.locator('article').count()
    expect(initialCount).toBe(50)

    // Click "Tucson" city filter (18 shows — fewer than the 50 page limit)
    await page.getByRole('button', { name: /Tucson/i }).click()

    // URL should update with city and state params
    await expect(page).toHaveURL(/city=Tucson/)
    await expect(page).toHaveURL(/state=AZ/)

    // Wait for the article count to decrease from 50
    await page.waitForFunction(
      (initial) => document.querySelectorAll('article').length < initial,
      initialCount,
      { timeout: 10_000 }
    )

    // Filtered count should be less than initial
    const filteredCount = await page.locator('article').count()
    expect(filteredCount).toBeGreaterThan(0)
    expect(filteredCount).toBeLessThan(initialCount)
  })

  test('All Cities button resets the filter', async ({ page }) => {
    // Start with Tucson filter (18 shows, under pagination limit)
    await page.goto('/shows?city=Tucson&state=AZ')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Tucson should not show "Load More" (only 18 shows)
    await expect(
      page.getByRole('button', { name: /load more/i })
    ).not.toBeVisible()

    // Click "All Cities" to reset and wait for the unfiltered API response
    const [response] = await Promise.all([
      page.waitForResponse(
        (resp) =>
          resp.url().includes('/shows/upcoming') &&
          !resp.url().includes('city='),
        { timeout: 10_000 }
      ),
      page.getByRole('button', { name: /all cities/i }).click(),
    ])
    expect(response.status()).toBeLessThan(400)

    // URL should no longer have city/state params
    expect(page.url()).not.toContain('city=')

    // Wait for "Load More" button to appear (unfiltered view has 50+ shows)
    await expect(
      page.getByRole('button', { name: /load more/i })
    ).toBeVisible({ timeout: 10_000 })
  })

  test('city filter preserves state across page navigation', async ({
    page,
  }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Apply Tucson filter
    await page.getByRole('button', { name: /Tucson/i }).click()
    await expect(page).toHaveURL(/city=Tucson/)

    // Navigate to a show detail and back
    await page
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Go back
    await page.goBack()
    await page.waitForURL(/\/shows\?/, { timeout: 10_000 })

    // Filter should still be in URL
    expect(page.url()).toContain('city=Tucson')
  })
})
