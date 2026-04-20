import { test, expect } from '../fixtures'

test.describe('Show list actions', () => {
  test('hide save buttons for unauthenticated users', async ({ page }) => {
    await page.goto('/shows')

    const firstShow = page.locator('article').first()
    await expect(firstShow).toBeVisible({ timeout: 10_000 })

    await expect(
      firstShow.getByRole('button', {
        name: /add to my list|remove from my list/i,
      })
    ).toHaveCount(0)
  })

  test('toggle save state from list cards for authenticated users', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/shows')

    // PSY-430: pin to a reserved show seeded by setup-db.sh so parallel
    // mutating tests in other files don't race on the same .first() row.
    // The aria-label on ShowCard's <article> exposes show.title as the
    // accessible name, so getByRole('article', { name }) finds it directly.
    const reservedShow = authenticatedPage.getByRole('article', {
      name: 'E2E [list-actions-test]',
    })
    await expect(reservedShow).toBeVisible({ timeout: 10_000 })

    const saveButton = reservedShow.locator(
      'button[aria-label="Add to My List"], button[aria-label="Remove from My List"]'
    )
    await expect(saveButton).toBeVisible()

    const initialLabel = await saveButton.getAttribute('aria-label')
    expect(
      initialLabel === 'Add to My List' ||
        initialLabel === 'Remove from My List'
    ).toBeTruthy()

    const firstToggleMethod =
      initialLabel === 'Add to My List' ? 'POST' : 'DELETE'
    const firstExpectedLabel =
      initialLabel === 'Add to My List'
        ? 'Remove from My List'
        : 'Add to My List'

    const [firstToggleResponse] = await Promise.all([
      authenticatedPage.waitForResponse(
        resp =>
          resp.url().includes('/saved-shows/') &&
          resp.request().method() === firstToggleMethod,
        { timeout: 10_000 }
      ),
      saveButton.click(),
    ])
    expect(firstToggleResponse.status()).toBeLessThan(400)
    await expect(
      reservedShow.getByRole('button', { name: firstExpectedLabel })
    ).toBeVisible({ timeout: 5_000 })

    // Cleanup: toggle back so test state is stable.
    const secondToggleMethod = firstToggleMethod === 'POST' ? 'DELETE' : 'POST'
    const [secondToggleResponse] = await Promise.all([
      authenticatedPage.waitForResponse(
        resp =>
          resp.url().includes('/saved-shows/') &&
          resp.request().method() === secondToggleMethod,
        { timeout: 10_000 }
      ),
      reservedShow.getByRole('button', { name: firstExpectedLabel }).click(),
    ])
    expect(secondToggleResponse.status()).toBeLessThan(400)
    await expect(
      reservedShow.getByRole('button', { name: initialLabel || 'Add to My List' })
    ).toBeVisible({ timeout: 5_000 })
  })

  // PSY-473 / Layer-5 audit item #5: the role-based conditional render
  // for the admin edit control is already covered by
  // `features/shows/components/ShowCard.test.tsx:226-234` (both `isAdmin`
  // branches asserted via `screen.queryByTitle('Edit show')`). The E2E
  // version spun up two full Playwright contexts for the same assertion.
})
