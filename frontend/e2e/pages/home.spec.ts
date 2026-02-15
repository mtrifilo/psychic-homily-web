import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Homepage', () => {
  test('loads and displays upcoming shows', async ({ page }) => {
    await page.goto('/')

    // Page title
    await expect(page).toHaveTitle(/Psychic Homily/)

    // "Upcoming Shows" section heading
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()

    // "View all" link to /shows
    await expect(
      page.getByRole('link', { name: /view all/i }).first()
    ).toBeVisible()

    // Wait for show cards to load (client-side fetch via TanStack Query)
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Verify at least one show card has a link (artist or venue)
    const firstShow = page.locator('article').first()
    await expect(firstShow.locator('a').first()).toBeVisible()
    await expect(firstShow.getByRole('link', { name: 'Details' })).toBeVisible()
  })

  test('displays navigation links', async ({ page }) => {
    await page.goto('/')

    // Desktop nav links (visible on default 1280x720 viewport)
    await expect(page.getByRole('link', { name: 'Shows' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Venues' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Blog' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'DJ Sets' })).toBeVisible()

    // Login link visible when not authenticated
    await expect(page.getByRole('link', { name: /login/i })).toBeVisible()
  })

  test('displays blog and DJ set sections', async ({ page }) => {
    await page.goto('/')

    // Blog section (server-rendered from markdown files)
    await expect(
      page.getByRole('heading', { name: /latest from the blog/i })
    ).toBeVisible()

    // DJ Set section (server-rendered from markdown files)
    await expect(
      page.getByRole('heading', { name: /latest dj set/i })
    ).toBeVisible()
  })
})
