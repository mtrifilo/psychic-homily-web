import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Homepage', () => {
  test('loads and displays upcoming shows', { tag: '@smoke' }, async ({ page }) => {
    await page.goto('/')

    // Page title (PSY-389: global "%s | Psychic Homily" template, no
    // "Arizona Music Community").
    await expect(page).toHaveTitle(/Psychic Homily/)

    // Discovery hero (PSY-389)
    await expect(
      page.getByRole('heading', { name: 'This is not a mirage.' })
    ).toBeVisible()

    // "Upcoming shows" section heading
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()

    // "View all shows" link to /shows (quiet link above the list)
    await expect(
      page.getByRole('link', { name: /view all shows/i }).first()
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

    // Top-bar primary nav (PSY-1013): explicit links + the menu triggers,
    // visible on the default 1280x720 viewport (>= the lg breakpoint).
    await expect(page.getByRole('link', { name: 'Shows' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Artists' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Browse the catalog' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Contribute' })).toBeVisible()

    // Login link visible when not authenticated
    await expect(page.getByRole('link', { name: /login/i })).toBeVisible()

    // Destinations the retired sidebar exposed directly are now reachable
    // inside the menus (no discoverability regression — PSY-1013).
    await page.getByRole('button', { name: 'Browse the catalog' }).click()
    await expect(page.getByRole('menuitem', { name: 'Venues' })).toBeVisible()
    await page.keyboard.press('Escape')

    await page.getByRole('button', { name: 'Contribute' }).click()
    await expect(page.getByRole('menuitem', { name: 'Blog' })).toBeVisible()
    await expect(page.getByRole('menuitem', { name: 'DJ Sets' })).toBeVisible()
  })

  test('displays discovery sections and footer (PSY-389)', async ({ page }) => {
    await page.goto('/')

    // Discover quick-links row in the hero
    await expect(
      page.getByRole('link', { name: 'Shows in any city' })
    ).toBeVisible()

    // Latest radio shows section + cards that link to /radio
    await expect(
      page.getByRole('heading', { name: /latest radio shows/i })
    ).toBeVisible()
    await expect(
      page.getByRole('link', { name: /KEXP.*Variety Mix/i })
    ).toHaveAttribute('href', '/radio')

    // Across the scene — reserved activity-feed placeholder
    await expect(
      page.getByRole('heading', { name: /across the scene/i })
    ).toBeVisible()
    await expect(
      page.getByText('Reserved for the Activity feed')
    ).toBeVisible()

    // Editorial footer columns (PSY-389)
    await expect(
      page.getByRole('navigation', { name: 'Discover' })
    ).toBeVisible()
    await expect(
      page.getByText('Made by the scene, for the scene.')
    ).toBeVisible()
  })
})
