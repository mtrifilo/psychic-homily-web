import { test, expect } from '../fixtures'

// PSY-275: the old /collection page was merged into /library.
// These tests now target /library and the consolidated tabs (Shows, Venues, Submissions).
test.describe('Library page (formerly /collection)', () => {
  test('displays Library heading and tabs', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/library')

    // Heading
    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })

    // Key tabs present in the consolidated Library
    await expect(
      authenticatedPage.getByRole('tab', { name: /shows/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('tab', { name: /venues/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('tab', { name: /submissions/i })
    ).toBeVisible()

    // Shows tab is selected by default
    await expect(
      authenticatedPage.getByRole('tab', { name: /shows/i })
    ).toHaveAttribute('data-state', 'active')
  })

  test('shows empty state when no shows are saved', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/library')

    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })

    // Empty state for shows
    await expect(
      authenticatedPage.getByText('No shows saved yet')
    ).toBeVisible({ timeout: 5_000 })
    await expect(
      authenticatedPage.getByRole('link', { name: 'Browse Shows' })
    ).toBeVisible()
  })

  test('falls back to shows tab when tab query is invalid', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/library?tab=invalid')

    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })

    await expect(
      authenticatedPage.getByRole('tab', { name: /shows/i })
    ).toHaveAttribute('data-state', 'active')
    await authenticatedPage.waitForURL('/library')
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

    // Navigate to library
    await authenticatedPage.goto('/library')
    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
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

    // Clean up: go back to the show and unsave it (wait for API response
    // so the DELETE completes before the test ends and the page closes)
    await authenticatedPage.goto(showUrl)
    await expect(
      authenticatedPage.getByRole('heading', { level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/saved-shows/') &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: 'Remove from My List' })
        .click(),
    ])
    await expect(
      authenticatedPage.getByRole('button', { name: 'Add to My List' })
    ).toBeVisible({ timeout: 5_000 })
  })
})
