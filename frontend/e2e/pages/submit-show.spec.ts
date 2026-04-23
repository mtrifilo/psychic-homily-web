import { test, expect } from '../fixtures'
import { test as unauthTest } from '../fixtures/error-detection'

test.describe('Submit a show', () => {
  // PSY-507: retries off. The mutating test below creates a real `shows`
  // row before its UI assertion runs; when PSY-437's Valley Bar autocomplete
  // flake trips the UI check, Playwright's default 2 CI retries resubmit the
  // same (headliner, venue, date) and the backend's duplicate-headliner guard
  // rejects them, turning a transient frontend flake into a hard failure.
  // `cleanBetweenRetries` (below) would normally reset between attempts, but
  // while PSY-437 is open we prefer to surface the real flake rate rather
  // than mask it with retries. Remove this line when PSY-437 lands.
  test.describe.configure({ retries: 0 })

  test('displays submission form for verified user', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/submissions')

    // Page heading
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Artists section
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Artists' })
    ).toBeVisible({ timeout: 5_000 })

    // Venue input
    await expect(
      authenticatedPage.locator('[id="venue.name"]')
    ).toBeVisible()

    // Date input
    await expect(authenticatedPage.locator('#date')).toBeVisible()

    // Submit button
    await expect(
      authenticatedPage.getByRole('button', { name: 'Submit Show' })
    ).toBeVisible()
  })

  // TODO(PSY-446/PSY-437): add `{ tag: '@smoke' }` once the Valley Bar
  // autocomplete flake is resolved. Classified as a Smoke candidate by
  // the PSY-445 audit (docs/strategy/testing-layers.md) — currently
  // deferred to keep PR CI green. Track in PSY-437.
  test('can submit a show with existing venue', async ({
    authenticatedPage,
    cleanBetweenRetries: _cleanup,
  }) => {
    await authenticatedPage.goto('/submissions')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Fill artist name
    await authenticatedPage
      .locator('[id="artists[0].name"]')
      .fill('E2E Submitted Artist')

    // Fill venue — type to trigger autocomplete, then select from dropdown.
    // The combobox input opens the dropdown when its value is non-empty
    // (VenueInput.tsx:70 setIsOpen). Debounce is 50ms, so a single fill
    // fires one search rather than the 10 pressSequentially fired.
    const venueInput = authenticatedPage.locator('[id="venue.name"]')
    await venueInput.focus()
    await venueInput.fill('Valley Bar')

    // Wait for the specific venue option to appear. The dropdown item
    // has `role="option"` explicitly set on a <button>; match by role to
    // stay aligned with the computed accessibility tree. Scope to the
    // listbox so we don't collide with any "Add a show at Valley Bar"
    // buttons elsewhere in the app.
    const listbox = authenticatedPage.getByRole('listbox')
    const valleyBarOption = listbox.getByRole('option', { name: /Valley Bar/ })
    await expect(valleyBarOption).toBeVisible({ timeout: 10_000 })

    // The option's onMouseDown directly calls handleVenueSelect(venue)
    // with the full venue object (including city/state), bypassing the
    // filteredVenues lookup in handleConfirm. It also sets justSelectedRef
    // which prevents a duplicate handleConfirm call on blur.
    await valleyBarOption.click()

    // Verify city auto-filled from selected venue
    await expect(
      authenticatedPage.locator('[id="venue.city"]')
    ).toHaveValue('Phoenix', { timeout: 10_000 })

    // Fill date with tomorrow's date
    const tomorrow = new Date()
    tomorrow.setDate(tomorrow.getDate() + 1)
    const dateStr = tomorrow.toISOString().split('T')[0] // YYYY-MM-DD
    await authenticatedPage.locator('#date').fill(dateStr)

    // Time defaults to 20:00 — leave as-is

    // Fill optional cost
    await authenticatedPage.locator('#cost').fill('$15')

    // Submit the form
    await authenticatedPage
      .getByRole('button', { name: 'Submit Show' })
      .click()

    // Wait for success message
    await expect(
      authenticatedPage.getByText('Show Submitted!')
    ).toBeVisible({ timeout: 10_000 })

    // Wait for redirect to /library (PSY-275: collection merged into library)
    await authenticatedPage.waitForURL(/\/library/, { timeout: 10_000 })

    // Verify we're on the library page
    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })
  })
})

unauthTest.describe('Submit show — unauthenticated', () => {
  unauthTest(
    'redirects unauthenticated user to login',
    async ({ page }) => {
      await page.goto('/submissions')

      // Should redirect to auth page
      await page.waitForURL(/\/auth/, { timeout: 10_000 })

      // Auth page content should be visible
      await expect(
        page.getByText('Sign in to your account')
      ).toBeVisible({ timeout: 5_000 })
    }
  )
})
