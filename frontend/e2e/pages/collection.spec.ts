import { test, expect } from '../fixtures'

test.describe('Collection page', () => {
  test('displays My Collection heading and tabs', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/collection')

    // Heading
    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })

    // Three tabs
    await expect(
      authenticatedPage.getByRole('tab', { name: /saved shows/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('tab', { name: /favorite venues/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('tab', { name: /my submissions/i })
    ).toBeVisible()

    // Saved Shows tab is selected by default
    await expect(
      authenticatedPage.getByRole('tab', { name: /saved shows/i })
    ).toHaveAttribute('data-state', 'active')
  })

  test('shows empty state when no shows are saved', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/collection')

    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })

    // Empty state for saved shows
    await expect(
      authenticatedPage.getByText('No saved shows yet')
    ).toBeVisible({ timeout: 5_000 })
    await expect(
      authenticatedPage.getByRole('link', { name: 'Browse Shows' })
    ).toBeVisible()
  })

  test('shows saved show after saving one', async ({
    authenticatedPage,
  }) => {
    // Navigate to shows list and open a show detail
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

    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // Save the show and wait for API response
    const saveButton = authenticatedPage.getByRole('button', {
      name: 'Add to My List',
    })
    await expect(saveButton).toBeVisible({ timeout: 5_000 })

    const [saveResponse] = await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/saved-shows/') &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      saveButton.click(),
    ])
    expect(saveResponse.status()).toBeLessThan(400)

    // Confirm button changed
    await expect(
      authenticatedPage.getByRole('button', { name: 'Remove from My List' })
    ).toBeVisible({ timeout: 5_000 })

    // Remember the show URL for cleanup
    const showUrl = authenticatedPage.url()

    // Navigate to collection
    await authenticatedPage.goto('/collection')
    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })

    // At least one show card should be visible
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 5_000,
    })
    await expect(
      authenticatedPage
        .locator('article')
        .first()
        .getByRole('link', { name: 'Details' })
    ).toBeVisible()

    // Clean up: go back to the show and unsave it
    await authenticatedPage.goto(showUrl)
    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    await authenticatedPage
      .getByRole('button', { name: 'Remove from My List' })
      .click()
    await expect(
      authenticatedPage.getByRole('button', { name: 'Add to My List' })
    ).toBeVisible({ timeout: 5_000 })
  })
})
