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
      .getByRole('link', { name: 'Details' })
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

    // Back to Venues link
    await expect(
      page.getByRole('link', { name: /back to venues/i })
    ).toBeVisible()

    // Upcoming and Past Shows tabs
    await expect(page.getByRole('tab', { name: /upcoming/i })).toBeVisible()
    await expect(
      page.getByRole('tab', { name: /past shows/i })
    ).toBeVisible()
  })

  test('back to venues link navigates to venues list', async ({ page }) => {
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

    const venueLink = page.locator('a[href^="/venues/"]').first()
    await expect(venueLink).toBeVisible({ timeout: 10_000 })
    await venueLink.click()
    await page.waitForURL(/\/venues\//, { timeout: 10_000 })

    await page.getByRole('link', { name: /back to venues/i }).click()
    await page.waitForURL(/\/venues$/, { timeout: 10_000 })
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

    const venueLink = page.locator('a[href^="/venues/"]').first()
    await expect(venueLink).toBeVisible({ timeout: 10_000 })
    await venueLink.click()
    await page.waitForURL(/\/venues\//, { timeout: 10_000 })

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
