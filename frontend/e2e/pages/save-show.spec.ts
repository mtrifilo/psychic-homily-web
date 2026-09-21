import { test, expect } from '../fixtures'

// PSY-430: pin to a reserved show seeded by setup-db.sh so parallel
// mutating tests in other files don't race on the same .first() row.
const RESERVED_SHOW_SLUG = 'e2e-save-show-test'
const RESERVED_SHOW_TITLE = 'E2E [save-show-test]'
const RESERVED_SHOW_URL = `/shows/${RESERVED_SHOW_SLUG}`

test.describe('Save/unsave a show', () => {
  // Tests share DB state (same user saving/unsaving the same show),
  // so they must not run in parallel
  test.describe.configure({ mode: 'serial' })
  test('save button is hidden when not authenticated', async ({ page }) => {
    await page.goto(RESERVED_SHOW_URL)

    // Wait for show detail to load (breadcrumb confirms the right show)
    await expect(
      page
        .getByRole('navigation', { name: 'Breadcrumb' })
        .getByText(RESERVED_SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })

    // Save button should NOT be visible when unauthenticated
    await expect(
      page.getByRole('button', { name: /save show|remove from saved shows/i })
    ).not.toBeVisible()
  })

  // PSY-1663: the unsave is this test's last step, so a failure between the
  // save and it leaves the show saved; the retry then finds "Remove from
  // saved shows" where it waits for "Save show". Opting into
  // `cleanBetweenRetries` clears the worker user's bookmarks between
  // attempts, which does not depend on the failing attempt reaching its own
  // cleanup.
  test('can save and unsave a show from detail page', { tag: '@smoke' }, async ({
    authenticatedPage,
    cleanBetweenRetries: _cleanup,
  }) => {
    await authenticatedPage.goto(RESERVED_SHOW_URL)

    // Wait for detail page to load
    // Breadcrumb shows the show title; the H1 is the headlining artist name,
    // so we verify the right show loaded via the breadcrumb.
    await expect(
      authenticatedPage
        .getByRole('navigation', { name: 'Breadcrumb' })
        .getByText(RESERVED_SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })

    // Save button should be visible and show "Save show"
    const saveButton = authenticatedPage.getByRole('button', {
      name: 'Save show',
    })
    await expect(saveButton).toBeVisible({ timeout: 5_000 })

    // Click to save
    await saveButton.click()

    // Button should change to "Remove from saved shows"
    await expect(
      authenticatedPage.getByRole('button', { name: 'Remove from saved shows' })
    ).toBeVisible({ timeout: 5_000 })

    // Click to unsave — wait for API response to ensure DB state is
    // updated before the next serial test starts (optimistic UI updates
    // flip the button text before the DELETE request completes)
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/saved-shows/') &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: 'Remove from saved shows' })
        .click(),
    ])

    // Button should revert to "Save show"
    await expect(
      authenticatedPage.getByRole('button', { name: 'Save show' })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('save state persists after navigation', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto(RESERVED_SHOW_URL)
    // Breadcrumb shows the show title; the H1 is the headlining artist name,
    // so we verify the right show loaded via the breadcrumb.
    await expect(
      authenticatedPage
        .getByRole('navigation', { name: 'Breadcrumb' })
        .getByText(RESERVED_SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })

    // Save the show and wait for the API response to complete
    const saveButton = authenticatedPage.getByRole('button', {
      name: 'Save show',
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
      authenticatedPage.getByRole('button', { name: 'Remove from saved shows' })
    ).toBeVisible({ timeout: 5_000 })

    // Navigate away via the breadcrumb link
    await authenticatedPage
      .locator('nav[aria-label="Breadcrumb"]')
      .getByRole('link', { name: 'Shows' })
      .click()
    await authenticatedPage.waitForURL(/\/shows$/, { timeout: 10_000 })

    // Navigate back to the same show
    await authenticatedPage.goto(RESERVED_SHOW_URL)
    // Breadcrumb shows the show title; the H1 is the headlining artist name,
    // so we verify the right show loaded via the breadcrumb.
    await expect(
      authenticatedPage
        .getByRole('navigation', { name: 'Breadcrumb' })
        .getByText(RESERVED_SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })

    // Should still be saved
    await expect(
      authenticatedPage.getByRole('button', { name: 'Remove from saved shows' })
    ).toBeVisible({ timeout: 10_000 })

    // Clean up: unsave the show — wait for API response
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          resp.url().includes('/saved-shows/') &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: 'Remove from saved shows' })
        .click(),
    ])
    await expect(
      authenticatedPage.getByRole('button', { name: 'Save show' })
    ).toBeVisible({ timeout: 5_000 })
  })
})
