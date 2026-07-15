import { test, expect } from '../fixtures'

// PSY-1438: show-owner controls live under Contribute, separate from Library.
test.describe('Show submissions console (Contribute)', () => {
  test("displays the signed-in user's submitted shows", async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/contribute/submissions')

    await expect(
      authenticatedPage.getByRole('heading', { name: /^show submissions$/i })
    ).toBeVisible({ timeout: 10_000 })

    // At least one submission should be visible
    // (ShowCard renders artist names + venue, not the show title)
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // The seeded show is in Phoenix, AZ — verify location text is rendered
    await expect(
      authenticatedPage.getByText('Phoenix, AZ').first()
    ).toBeVisible()
  })

  test('redirects the retired Library deep-link and preserves the console', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/library?tab=submissions')
    await authenticatedPage.waitForURL('/contribute/submissions')

    await expect(
      authenticatedPage.getByRole('heading', { name: /^show submissions$/i })
    ).toBeVisible({ timeout: 10_000 })

    // Wait for submissions to load
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // At least one approved show should have a "Published" badge
    // (admin tests may approve additional shows, so use .first() to avoid strict mode)
    await expect(authenticatedPage.getByText('Published').first()).toBeVisible()

    // Venue and location info should be present (Phoenix, AZ)
    await expect(
      authenticatedPage.getByText('Phoenix, AZ').first()
    ).toBeVisible()
  })

  test('shows and dismisses the private-submission success dialog', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/contribute/submissions?submitted=private')

    const dialog = authenticatedPage.getByRole('dialog', {
      name: 'Private Show Added',
    })
    await expect(dialog).toBeVisible({ timeout: 10_000 })
    await dialog.getByRole('button', { name: 'Got it' }).click()

    await expect(dialog).not.toBeVisible()
    await authenticatedPage.waitForURL('/contribute/submissions')
  })
})
