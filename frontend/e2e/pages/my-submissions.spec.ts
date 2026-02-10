import { test, expect } from '../fixtures'

test.describe('My Submissions tab', () => {
  test('displays user submissions in My Submissions tab', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/collection')

    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })

    // Click My Submissions tab
    await authenticatedPage
      .getByRole('tab', { name: /my submissions/i })
      .click()

    await expect(
      authenticatedPage.getByRole('tab', { name: /my submissions/i })
    ).toHaveAttribute('data-state', 'active')

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

  test('shows submission status and details', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/collection')

    await expect(
      authenticatedPage.getByRole('heading', { name: /my collection/i })
    ).toBeVisible({ timeout: 10_000 })

    await authenticatedPage
      .getByRole('tab', { name: /my submissions/i })
      .click()

    await expect(
      authenticatedPage.getByRole('tab', { name: /my submissions/i })
    ).toHaveAttribute('data-state', 'active')

    // Wait for submissions to load
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // At least one approved show should have a "Published" badge
    // (admin tests may approve additional shows, so use .first() to avoid strict mode)
    await expect(authenticatedPage.getByText('Published').first()).toBeVisible()

    // Venue and location info should be present (Phoenix, AZ)
    await expect(authenticatedPage.getByText('Phoenix, AZ').first()).toBeVisible()
  })
})
