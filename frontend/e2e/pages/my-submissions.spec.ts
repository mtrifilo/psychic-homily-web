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
    await expect(authenticatedPage.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // The stable approved show should be present
    await expect(
      authenticatedPage.getByText('E2E My Submitted Show')
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

    // The approved show should have a "Published" badge
    await expect(authenticatedPage.getByText('Published')).toBeVisible()

    // Venue and location info should be present (Phoenix, AZ)
    await expect(authenticatedPage.getByText('Phoenix, AZ')).toBeVisible()
  })
})
