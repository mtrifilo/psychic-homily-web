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

    const firstShow = authenticatedPage.locator('article').first()
    await expect(firstShow).toBeVisible({ timeout: 10_000 })

    const saveButton = firstShow.locator(
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
      firstShow.getByRole('button', { name: firstExpectedLabel })
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
      firstShow.getByRole('button', { name: firstExpectedLabel }).click(),
    ])
    expect(secondToggleResponse.status()).toBeLessThan(400)
    await expect(
      firstShow.getByRole('button', { name: initialLabel || 'Add to My List' })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('show admin edit controls only for admins', async ({
    authenticatedPage,
    adminPage,
  }) => {
    await authenticatedPage.goto('/shows')
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })
    await expect(
      authenticatedPage.locator('[title="Edit show"]').first()
    ).toHaveCount(0)

    await adminPage.goto('/shows')
    await expect(adminPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })
    await expect(adminPage.locator('[title="Edit show"]').first()).toBeVisible()
  })
})
