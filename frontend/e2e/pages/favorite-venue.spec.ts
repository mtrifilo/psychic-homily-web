import { test, expect } from '../fixtures'

test.describe('Favorite venue', () => {
  // Tests share DB state (same user favoriting/unfavoriting the same venue),
  // so they must not run in parallel
  test.describe.configure({ mode: 'serial' })
  test('favorite button is hidden when not authenticated', async ({
    page,
  }) => {
    // Navigate to a venue detail page
    await page.goto('/venues')
    await expect(
      page.locator('a[href^="/venues/"]').first()
    ).toBeVisible({ timeout: 10_000 })

    await page.locator('a[href^="/venues/"]').first().click()
    await page.waitForURL(/\/venues\//, { timeout: 10_000 })

    // Wait for venue detail to load
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
      timeout: 10_000,
    })

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
    // Navigate to a venue detail page
    await authenticatedPage.goto('/venues')
    await expect(
      authenticatedPage.locator('a[href^="/venues/"]').first()
    ).toBeVisible({ timeout: 10_000 })

    await authenticatedPage
      .locator('a[href^="/venues/"]')
      .first()
      .click()
    await authenticatedPage.waitForURL(/\/venues\//, { timeout: 10_000 })

    // Wait for venue detail to load
    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
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

  test('favorited venue appears in collection favorites tab', async ({
    authenticatedPage,
  }) => {
    // Navigate to a venue detail page
    await authenticatedPage.goto('/venues')
    await expect(
      authenticatedPage.locator('a[href^="/venues/"]').first()
    ).toBeVisible({ timeout: 10_000 })

    // Capture venue name for later assertion
    const venueName = await authenticatedPage
      .locator('a[href^="/venues/"]')
      .first()
      .textContent()

    await authenticatedPage
      .locator('a[href^="/venues/"]')
      .first()
      .click()
    await authenticatedPage.waitForURL(/\/venues\//, { timeout: 10_000 })

    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
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

    // Remember the venue URL for cleanup
    const venueUrl = authenticatedPage.url()

    // Navigate to collection favorites tab
    await authenticatedPage.goto('/collection?tab=favorites')
    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })

    // The venue name should appear (not the empty state)
    await expect(
      authenticatedPage.getByText(venueName!)
    ).toBeVisible({ timeout: 5_000 })

    // Clean up: navigate back to venue and unfavorite
    await authenticatedPage.goto(venueUrl)
    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
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
