import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Show detail', () => {
  test('displays show details with artist and venue links', async ({ page }) => {
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Navigate to first show detail
    await page
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Back navigation link
    await expect(
      page.getByRole('link', { name: /back to shows/i })
    ).toBeVisible()

    // H1 heading with artist name(s)
    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).not.toBeEmpty()

    // Venue link (points to /venues/...)
    await expect(page.locator('a[href^="/venues/"]').first()).toBeVisible()

    // Artist link(s) (points to /artists/...)
    await expect(page.locator('a[href^="/artists/"]').first()).toBeVisible()

    // Header element wraps the show info
    await expect(page.locator('header').first()).toBeVisible()
  })

  test('page title includes artist and venue', async ({ page }) => {
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    await page
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Wait for client-side data to load
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
      timeout: 10_000,
    })

    // SSR metadata: page title format is "{headliner} at {venue}"
    await expect(page).toHaveTitle(/.+ at .+/, { timeout: 10_000 })
  })

  test('back to shows link navigates to shows list', async ({ page }) => {
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    await page
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    await page.getByRole('link', { name: /back to shows/i }).click()
    await page.waitForURL(/\/shows$/, { timeout: 10_000 })

    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()
  })
})
