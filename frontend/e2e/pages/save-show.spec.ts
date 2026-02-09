import { test, expect } from '../fixtures'

test.describe('Save/unsave a show', () => {
  // Tests share DB state (same user saving/unsaving the same show),
  // so they must not run in parallel
  test.describe.configure({ mode: 'serial' })
  test('save button is hidden when not authenticated', async ({ page }) => {
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

    // Wait for show detail to load
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
      timeout: 10_000,
    })

    // Save button should NOT be visible when unauthenticated
    await expect(
      page.getByRole('button', { name: /add to my list|remove from my list/i })
    ).not.toBeVisible()
  })

  test('can save and unsave a show from detail page', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/shows')
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    await authenticatedPage
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })
      .click()
    await authenticatedPage.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Wait for detail page to load
    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // Save button should be visible and show "Add to My List"
    const saveButton = authenticatedPage.getByRole('button', {
      name: 'Add to My List',
    })
    await expect(saveButton).toBeVisible({ timeout: 5_000 })

    // Click to save
    await saveButton.click()

    // Button should change to "Remove from My List"
    await expect(
      authenticatedPage.getByRole('button', { name: 'Remove from My List' })
    ).toBeVisible({ timeout: 5_000 })

    // Click to unsave
    await authenticatedPage
      .getByRole('button', { name: 'Remove from My List' })
      .click()

    // Button should revert to "Add to My List"
    await expect(
      authenticatedPage.getByRole('button', { name: 'Add to My List' })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('save state persists after navigation', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/shows')
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Navigate to first show detail
    await authenticatedPage
      .locator('article')
      .first()
      .getByRole('link', { name: 'Details' })
      .click()
    await authenticatedPage.waitForURL(/\/shows\//, { timeout: 10_000 })

    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // Save the show and wait for the API response to complete
    const saveButton = authenticatedPage.getByRole('button', {
      name: 'Add to My List',
    })
    await expect(saveButton).toBeVisible({ timeout: 5_000 })

    const [saveResponse] = await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) => resp.url().includes('/saved-shows/') && resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      saveButton.click(),
    ])
    expect(saveResponse.status()).toBeLessThan(400)

    await expect(
      authenticatedPage.getByRole('button', { name: 'Remove from My List' })
    ).toBeVisible({ timeout: 5_000 })

    // Remember the URL so we can come back
    const showUrl = authenticatedPage.url()

    // Navigate away
    await authenticatedPage.getByRole('link', { name: /back to shows/i }).click()
    await authenticatedPage.waitForURL(/\/shows$/, { timeout: 10_000 })

    // Navigate back to the same show
    await authenticatedPage.goto(showUrl)
    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // Should still be saved
    await expect(
      authenticatedPage.getByRole('button', { name: 'Remove from My List' })
    ).toBeVisible({ timeout: 10_000 })

    // Clean up: unsave the show
    await authenticatedPage
      .getByRole('button', { name: 'Remove from My List' })
      .click()
    await expect(
      authenticatedPage.getByRole('button', { name: 'Add to My List' })
    ).toBeVisible({ timeout: 5_000 })
  })
})
