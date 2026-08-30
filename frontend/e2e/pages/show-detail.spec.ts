import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

/**
 * FLAKE NOTE on the list-to-detail leg below (`/shows` -> click the first
 * card's show link -> `waitForURL(/\/shows\//)`).
 *
 * Both tests here have timed out on that `waitForURL` in CI while the product
 * was fine. Recorded so the next occurrence is not re-investigated from zero.
 * Evidence, from main CI run 33282546005 (shard 4/4, 2026-08-29):
 *
 *  - Both timed out on the initial attempt and PASSED ON RETRY, in a job whose
 *    Playwright summary read "1 failed / 2 flaky / 29 passed". The `1 failed`
 *    was a different spec with a genuine strict-mode defect, which failed the
 *    initial attempt AND both retries. Deterministic defect versus
 *    retry-recoverable flake is exactly that difference.
 *  - This leg is not unique to this file. `artist-detail.spec.ts` and
 *    `venue-detail.spec.ts` open with a byte-identical construct, and
 *    `city-filter.spec.ts` with a near-identical one. They sit on other shards
 *    and were green in the same run, which is the evidence against a real
 *    defect in the shared shape.
 *  - Ten local executions across five independent stack bring-ups, including
 *    under parallel workers, produced zero failures.
 *
 * So this is treated as shard load, and the spec is deliberately left alone
 * rather than absorbing a timeout bump that would hide a future real
 * regression.
 *
 * There is a THIRD option, and this repo has already used it: navigation.spec.ts
 * hit flakiness on this same shape and fixed it under PSY-430 by navigating
 * straight to a reserved seeded slug instead of clicking `.first()`, on the
 * reasoning that unreserved first-card rows race parallel mutating tests
 * (`fullyParallel`, 3 workers in CI). Not adopted here, for one reason worth
 * stating plainly: this spec's `.first()` card is the only place the
 * list-to-detail CLICK is exercised end to end, and swapping it for a `goto`
 * trades a real coverage leg for a flake that currently self-recovers. That
 * trade is worth making the moment it stops self-recovering, and the reserved
 * row (`e2e-attendance-test`) already exists to make it a small change.
 *
 * If it recurs and stops recovering on retry, take that option, and take it in
 * ALL FOUR specs together, not just this one: they are twins, and a fix that
 * lands on a subset is how the next red goes unconnected to this note.
 */
test.describe('Show detail', () => {
  test('displays show details with artist and venue links', async ({ page }) => {
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Navigate to first show detail via the show link in the card
    await page
      .locator('article')
      .first()
      .locator('a[href^="/shows/"]')
      .first()
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // H1 heading with artist name(s)
    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).not.toBeEmpty()

    // Breadcrumb navigation link to Shows list
    const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]')
    await expect(breadcrumbNav.getByRole('link', { name: 'Shows' })).toBeVisible()

    // Venue link (points to /venues/...)
    await expect(page.locator('a[href^="/venues/"]').first()).toBeVisible()

    // Artist link(s) (points to /artists/...)
    await expect(page.locator('a[href^="/artists/"]').first()).toBeVisible()

    // Header element wraps the show info
    await expect(page.locator('header').first()).toBeVisible()
  })

  test('page title includes artist and venue', async ({ page }) => {
    await page.goto('/shows')
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    await page
      .locator('article')
      .first()
      .locator('a[href^="/shows/"]')
      .first()
      .click()
    await page.waitForURL(/\/shows\//, { timeout: 10_000 })

    // Wait for client-side data to load
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
      timeout: 10_000,
    })

    // SSR metadata: page title format is "{headliner} at {venue}"
    await expect(page).toHaveTitle(/.+ at .+/, { timeout: 10_000 })
  })

})
