import { test, expect } from '../fixtures'

// PSY-275: the old /collection page was merged into /library.
// These tests now target /library and the consolidated tabs (Shows, Venues, Submissions).
test.describe('Library page (formerly /collection)', () => {
  test('displays Library heading and tabs', async ({ authenticatedPage }) => {
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

    // Empty state for shows (PSY-1440 dense editorial copy)
    await expect(authenticatedPage.getByText('Nothing saved yet.')).toBeVisible(
      { timeout: 5_000 }
    )
    await expect(
      authenticatedPage.getByRole('link', { name: 'Browse shows' })
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

  test('lists and unfollows a followed scene', async ({ authenticatedPage }) => {
    const followResponse = await authenticatedPage.request.post(
      '/api/scenes/phoenix-az/follow'
    )
    expect(followResponse.ok()).toBe(true)

    await authenticatedPage.goto('/library?tab=scenes')
    const sceneRow = authenticatedPage
      .getByRole('article')
      .filter({ hasText: 'Phoenix, AZ' })
    await expect(sceneRow).toBeVisible({ timeout: 10_000 })

    const [unfollowResponse] = await Promise.all([
      authenticatedPage.waitForResponse(
        response =>
          response.url().includes('/scenes/phoenix-az/follow') &&
          response.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      sceneRow.getByRole('button', { name: 'Unfollow Phoenix, AZ' }).click(),
    ])
    expect(unfollowResponse.ok()).toBe(true)
    await expect(sceneRow).not.toBeVisible({ timeout: 5_000 })
  })

  test('saves a release and removes it from the Releases tab', async ({
    authenticatedPage,
    cleanBetweenRetries,
  }) => {
    void cleanBetweenRetries
    await authenticatedPage.goto('/releases/futures')
    await expect(
      authenticatedPage.getByRole('heading', { level: 1, name: 'Futures' })
    ).toBeVisible({ timeout: 10_000 })

    const saveButton = authenticatedPage.getByRole('button', {
      name: 'Save release',
    })
    await expect(saveButton).toBeVisible({ timeout: 5_000 })

    await Promise.all([
      authenticatedPage.waitForResponse(
        response =>
          /\/saved-releases\/\d+$/.test(response.url()) &&
          response.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      saveButton.click(),
    ])

    await authenticatedPage.goto('/library?tab=releases')
    await expect(
      authenticatedPage.getByRole('tab', { name: 'Releases, 1 saved' })
    ).toHaveAttribute('data-state', 'active')

    const releaseRow = authenticatedPage
      .getByRole('article')
      .filter({ hasText: 'Futures' })
    await expect(releaseRow).toBeVisible({ timeout: 10_000 })
    await expect(releaseRow.getByText(/2004/)).toBeVisible()
    await expect(
      releaseRow.getByRole('link', { name: 'Run For Cover Records' })
    ).toBeVisible()
    await expect(releaseRow.getByText(/^saved /)).toBeVisible()

    await Promise.all([
      authenticatedPage.waitForResponse(
        response =>
          /\/saved-releases\/\d+$/.test(response.url()) &&
          response.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      releaseRow
        .getByRole('button', {
          name: 'Remove Futures from saved releases',
        })
        .click(),
    ])

    await expect(releaseRow).not.toBeVisible({ timeout: 5_000 })
    await expect(
      authenticatedPage.getByRole('tab', { name: 'Releases, 0 saved' })
    ).toBeVisible()
  })

  test(
    'shows saved show after saving one',
    { tag: '@smoke' },
    async ({ authenticatedPage }) => {
      // PSY-430: pin to a reserved show seeded by setup-db.sh so parallel
      // mutating tests in other files don't race on the same .first() row.
      const reservedShowSlug = 'e2e-collection-saved-show'
      const reservedShowTitle = 'E2E [collection-saved-show]'
      const showUrl = `/shows/${reservedShowSlug}`

      await authenticatedPage.goto(showUrl)
      // Breadcrumb shows the show title; the H1 is the headlining artist name,
      // so we verify the right show loaded via the breadcrumb.
      await expect(
        authenticatedPage
          .getByRole('navigation', { name: 'Breadcrumb' })
          .getByText(reservedShowTitle)
      ).toBeVisible({ timeout: 10_000 })

      // Save the show and wait for API response
      const saveButton = authenticatedPage.getByRole('button', {
        name: 'Add to My List',
      })
      await expect(saveButton).toBeVisible({ timeout: 5_000 })

      const [saveResponse] = await Promise.all([
        authenticatedPage.waitForResponse(
          resp =>
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

      // Navigate to library
      await authenticatedPage.goto('/library')
      await expect(
        authenticatedPage.getByRole('heading', { name: /^library$/i })
      ).toBeVisible({ timeout: 10_000 })

      // The reserved show we just saved should appear in the library.
      // Uses aria-label on <article> (added in this PR for testability + a11y).
      const savedCard = authenticatedPage.getByRole('article', {
        name: reservedShowTitle,
      })
      await expect(savedCard).toBeVisible({ timeout: 5_000 })
      await expect(
        authenticatedPage.getByRole('heading', { name: 'Upcoming' })
      ).toBeVisible()
      // The card links to the show detail page via the artist name.
      await expect(
        savedCard.locator(`a[href="/shows/${reservedShowSlug}"]`)
      ).toBeVisible()

      // Remove from the new Library row itself; this both exercises the ticket's
      // removal affordance and cleans up the reserved fixture.
      await Promise.all([
        authenticatedPage.waitForResponse(
          resp =>
            resp.url().includes('/saved-shows/') &&
            resp.request().method() === 'DELETE',
          { timeout: 10_000 }
        ),
        savedCard
          .getByRole('button', {
            name: `Remove ${reservedShowTitle} from saved shows`,
          })
          .click(),
      ])
      await expect(savedCard).not.toBeVisible({ timeout: 5_000 })
    }
  )
})
