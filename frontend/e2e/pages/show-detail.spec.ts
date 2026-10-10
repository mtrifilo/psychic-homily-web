import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { BACKEND_BASE_URL } from '../backend-url'
import {
  CLIENT_NAVIGATION_TIMEOUT_MS,
  SEEDED_DETAIL_PATHS,
  warmRoutes,
} from '../helpers/navigation'

/**
 * The list-to-detail leg below (`/shows` -> click the first card's show link
 * -> `waitForURL(/\/shows\//)`) is shared with artist-detail, venue-detail
 * and city-filter. Each of the four warms the routes it visits in
 * `beforeAll` and gives the URL wait CLIENT_NAVIGATION_TIMEOUT_MS
 * (helpers/navigation.ts), so a route's first compile on the dev server runs
 * in the hook instead of the wait. Change the four together.
 */
test.describe('Show detail', () => {
  test.beforeAll(async ({ browser }, testInfo) => {
    await warmRoutes(browser, testInfo, ['/shows', SEEDED_DETAIL_PATHS.show])
  })

  test('displays show details with artist and venue links', async ({ page }) => {
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Navigate to first show detail via the show link in the card
    await page
      .locator('article')
      .first()
      .locator('a[href^="/shows/"]')
      .first()
      .click()
    await page.waitForURL(/\/shows\//, {
      timeout: CLIENT_NAVIGATION_TIMEOUT_MS,
    })

    // H1 heading with artist name(s)
    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).not.toBeEmpty()

    // Breadcrumb navigation link to Shows list
    const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]')
    await expect(breadcrumbNav.getByRole('link', { name: 'Shows' })).toBeVisible()

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
      .locator('a[href^="/shows/"]')
      .first()
      .click()
    await page.waitForURL(/\/shows\//, {
      timeout: CLIENT_NAVIGATION_TIMEOUT_MS,
    })

    // Wait for client-side data to load
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
      timeout: 10_000,
    })

    // SSR metadata: page title format is "{headliner} at {venue}"
    await expect(page).toHaveTitle(/.+ at .+/, { timeout: 10_000 })
  })

  test('a numeric show URL permanently redirects to its slug URL', async ({
    page,
    request,
  }) => {
    // Read-only use of the reserved seeded show, so parallel mutating specs
    // cannot move it.
    const slug = 'e2e-attendance-test'
    const show = await request.get(`${BACKEND_BASE_URL}/shows/${slug}`)
    expect(show.ok()).toBe(true)
    const { id } = (await show.json()) as { id: number }

    const numeric = await request.get(`/shows/${id}?ref=e2e`, { maxRedirects: 0 })
    expect(numeric.status()).toBe(308)
    const location = new URL(numeric.headers()['location'], 'http://origin')
    expect(`${location.pathname}${location.search}`).toBe(
      `/shows/${slug}?ref=e2e`
    )

    // The slug URL is the end of the chain: no second redirect.
    const canonical = await request.get(`/shows/${slug}`, { maxRedirects: 0 })
    expect(canonical.status()).toBe(200)

    await page.goto(`/shows/${id}`)
    await expect(page).toHaveURL(new RegExp(`/shows/${slug}$`))
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute(
      'href',
      `https://psychichomily.com/shows/${slug}`
    )
  })
})
