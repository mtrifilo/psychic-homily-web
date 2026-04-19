import { test, expect } from '../fixtures'

// PSY-430: pin to a reserved venue seeded by setup-db.sh so parallel
// mutating tests in other files don't race on the same .first() row.
const RESERVED_VENUE_SLUG = 'e2e-favorite-venue-test'
const RESERVED_VENUE_NAME = 'E2E [favorite-venue-test]'
const RESERVED_VENUE_URL = `/venues/${RESERVED_VENUE_SLUG}`

test.describe('Favorite venue', () => {
  // Tests share DB state (same user favoriting/unfavoriting the same venue),
  // so they must not run in parallel
  test.describe.configure({ mode: 'serial' })
  test('favorite button is hidden when not authenticated', async ({
    page,
  }) => {
    await page.goto(RESERVED_VENUE_URL)

    // Wait for venue detail to load
    await expect(
      page.getByRole('heading', { level: 1, name: RESERVED_VENUE_NAME })
    ).toBeVisible({ timeout: 10_000 })

    // Favorite button should NOT be visible when unauthenticated
    await expect(
      page.getByRole('button', {
        name: /add to favorites|remove from favorites/i,
      })
    ).not.toBeVisible()
  })

  test('can favorite and unfavorite a venue from detail page', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto(RESERVED_VENUE_URL)

    // Wait for venue detail to load
    await expect(
      authenticatedPage.getByRole('heading', {
        level: 1,
        name: RESERVED_VENUE_NAME,
      })
    ).toBeVisible({ timeout: 10_000 })

    // Favorite button should be visible
    const favoriteButton = authenticatedPage.getByRole('button', {
      name: 'Add to Favorites',
    })
    await expect(favoriteButton).toBeVisible({ timeout: 5_000 })

    // Click to favorite — wait for API response (optimistic UI updates
    // flip the button text before the POST request completes)
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/favorite-venues/') &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      favoriteButton.click(),
    ])

    // Button should change to "Remove from Favorites"
    await expect(
      authenticatedPage.getByRole('button', {
        name: 'Remove from Favorites',
      })
    ).toBeVisible({ timeout: 5_000 })

    // Click to unfavorite (cleanup) — wait for API response
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/favorite-venues/') &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: 'Remove from Favorites' })
        .click(),
    ])

    // Button should revert to "Add to Favorites"
    await expect(
      authenticatedPage.getByRole('button', { name: 'Add to Favorites' })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('favorited venue appears in library venues tab', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto(RESERVED_VENUE_URL)
    await expect(
      authenticatedPage.getByRole('heading', {
        level: 1,
        name: RESERVED_VENUE_NAME,
      })
    ).toBeVisible({ timeout: 10_000 })

    // Favorite the venue and wait for API response
    const favoriteButton = authenticatedPage.getByRole('button', {
      name: 'Add to Favorites',
    })
    await expect(favoriteButton).toBeVisible({ timeout: 5_000 })

    const [favoriteResponse] = await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/favorite-venues/') &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      favoriteButton.click(),
    ])
    expect(favoriteResponse.status()).toBeLessThan(400)

    await expect(
      authenticatedPage.getByRole('button', {
        name: 'Remove from Favorites',
      })
    ).toBeVisible({ timeout: 5_000 })

    // Navigate to library venues tab (PSY-275: favorites merged into venues tab on /library)
    await authenticatedPage.goto('/library?tab=venues')
    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })

    // The reserved venue name should appear (not the empty state).
    // .first() because the venue may render in both Favorite and Followed
    // sections of the venues tab — either is sufficient evidence the favorite
    // landed.
    await expect(
      authenticatedPage
        .getByRole('link', { name: RESERVED_VENUE_NAME })
        .first()
    ).toBeVisible({ timeout: 5_000 })

    // Clean up: navigate back to venue and unfavorite
    await authenticatedPage.goto(RESERVED_VENUE_URL)
    await expect(
      authenticatedPage.getByRole('heading', {
        level: 1,
        name: RESERVED_VENUE_NAME,
      })
    ).toBeVisible({ timeout: 10_000 })

    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/favorite-venues/') &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: 'Remove from Favorites' })
        .click(),
    ])
    await expect(
      authenticatedPage.getByRole('button', { name: 'Add to Favorites' })
    ).toBeVisible({ timeout: 5_000 })
  })
})
