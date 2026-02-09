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
      .getByRole('link', { name: 'Details' })
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

    // Back to Shows link
    await expect(
      page.getByRole('link', { name: /back to shows/i })
    ).toBeVisible()

    // Upcoming and Past Shows tabs
    await expect(page.getByRole('tab', { name: /upcoming/i })).toBeVisible()
    await expect(
      page.getByRole('tab', { name: /past shows/i })
    ).toBeVisible()
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

    const artistLink = page.locator('a[href^="/artists/"]').first()
    await expect(artistLink).toBeVisible({ timeout: 10_000 })
    await artistLink.click()
    await page.waitForURL(/\/artists\//, { timeout: 10_000 })

    await page.getByRole('link', { name: /back to shows/i }).click()
    await page.waitForURL(/\/shows$/, { timeout: 10_000 })

    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()
  })

  test('shows tabs switch between upcoming and past', async ({ page }) => {
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

    const artistLink = page.locator('a[href^="/artists/"]').first()
    await expect(artistLink).toBeVisible({ timeout: 10_000 })
    await artistLink.click()
    await page.waitForURL(/\/artists\//, { timeout: 10_000 })

    // Upcoming tab should be active by default
    const upcomingTab = page.getByRole('tab', { name: /upcoming/i })
    await expect(upcomingTab).toBeVisible({ timeout: 10_000 })

    // Click Past Shows tab
    const pastTab = page.getByRole('tab', { name: /past shows/i })
    await pastTab.click()
    await expect(pastTab).toHaveAttribute('aria-selected', 'true')

    // Click back to Upcoming
    await upcomingTab.click()
    await expect(upcomingTab).toHaveAttribute('aria-selected', 'true')
  })
})
