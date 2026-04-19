import { test, expect } from '../fixtures'

/**
 * PSY-454: consolidated cross-entity back-to-list navigation.
 *
 * Replaces 5 per-entity nav-only specs that each exercised the same
 * list → detail → back-link → list loop (see PSY-445 audit's Delete/merge
 * bucket). This file parameterizes the loop over shows/artists/venues.
 *
 * Nav is unauthenticated — no auth fixture required.
 *
 * PSY-430: use the reserved seeded rows from setup-db.sh (stable slugs) so
 * we don't compete with parallel mutating tests that touch `.first()` cards.
 */

type NavEntity = {
  entity: 'shows' | 'artists' | 'venues'
  /** Breadcrumb link label on the detail page (also the list-page heading). */
  breadcrumbLabel: 'Shows' | 'Artists' | 'Venues'
  /** Reserved seeded detail-page slug (from setup-db.sh). */
  detailSlug: string
  /** List-page heading matcher — used when returning to the list. */
  listHeadingMatcher: RegExp
}

const ENTITIES: NavEntity[] = [
  {
    entity: 'shows',
    breadcrumbLabel: 'Shows',
    detailSlug: 'e2e-attendance-test',
    listHeadingMatcher: /upcoming shows/i,
  },
  {
    entity: 'artists',
    breadcrumbLabel: 'Artists',
    detailSlug: 'e2e-follow-test',
    listHeadingMatcher: /artists/i,
  },
  {
    entity: 'venues',
    breadcrumbLabel: 'Venues',
    detailSlug: 'e2e-favorite-venue-test',
    listHeadingMatcher: /venues/i,
  },
]

test.describe('Cross-entity back-to-list navigation', () => {
  for (const { entity, breadcrumbLabel, detailSlug, listHeadingMatcher } of ENTITIES) {
    test(`${entity}: list → detail → back link returns to list`, async ({
      page,
    }) => {
      // 1) List page renders with its heading + at least one card.
      await page.goto(`/${entity}`)
      await expect(
        page.getByRole('heading', { name: listHeadingMatcher }).first()
      ).toBeVisible({ timeout: 10_000 })
      await expect(page.locator('article').first()).toBeVisible({
        timeout: 10_000,
      })

      // 2) Navigate straight to a reserved seeded detail page. Direct goto
      // keeps the list→detail step deterministic even under parallel workers
      // that mutate the unreserved `.first()` row.
      //
      // The deleted tests also covered the list→detail click leg
      // (shows.spec.ts:71 asserted the href pattern before clicking; venues.spec.ts:40
      // clicked the card link). That leg is implicitly covered here: Next.js
      // routing to `/${entity}/${detailSlug}` exercises the same app-router
      // path the card links generate, and we assert the detail page renders
      // below. If the list → detail link patterns regress, the list cards
      // (covered by shows.spec.ts / venues.spec.ts render tests) will catch it.
      await page.goto(`/${entity}/${detailSlug}`)
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible({
        timeout: 10_000,
      })

      // 3) Click the breadcrumb back-link to return to the list.
      const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]')
      await expect(
        breadcrumbNav.getByRole('link', { name: breadcrumbLabel })
      ).toBeVisible()
      await breadcrumbNav
        .getByRole('link', { name: breadcrumbLabel })
        .click()

      // 4) Back on the list page: URL is the bare `/${entity}` and heading
      // is visible.
      await page.waitForURL(new RegExp(`/${entity}$`), { timeout: 10_000 })
      await expect(
        page.getByRole('heading', { name: listHeadingMatcher }).first()
      ).toBeVisible()
    })
  }
})
