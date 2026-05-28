import { test, expect } from '../fixtures'

/**
 * PSY-720: end-to-end coverage for the role-based admin route guard.
 *
 * AdminGuard lives at `app/admin/admin-guard.tsx` and is wired into every
 * admin route via `app/admin/layout.tsx`. Vitest covers the component in
 * isolation (`admin-guard.test.tsx`); this spec is the integration guard
 * that catches regressions in the routing wiring itself — e.g. if the
 * layout-level guard were ever removed or wired only to a subset of
 * sub-routes, the unit test would still pass while the redirect contract
 * silently broke.
 *
 * The contract under test (from admin-guard.tsx):
 * - unauthenticated → `/auth?returnTo=%2Fadmin`
 * - authenticated non-admin → `/` (intermediate "Access Denied" card)
 * - authenticated admin → children render
 *
 * Sub-routes asserted: /admin/dashboard, /admin/reports, /admin/users
 * (all three confirmed to exist as standalone routes under `app/admin/`
 * with their own page.tsx). The /admin root itself is intentionally
 * excluded — its page.tsx carries its own redundant redirect that races
 * with AdminGuard and complicates the contract.
 */

const AUTH_URL_REGEX = /\/auth(\?|$)/
const ADMIN_RETURN_TO = 'returnTo=%2Fadmin'
const ADMIN_SUBROUTES = ['/admin/dashboard', '/admin/reports', '/admin/users']

test.describe('AdminGuard: unauthenticated access', () => {
  for (const route of ADMIN_SUBROUTES) {
    test(`unauthenticated visitor at ${route} is redirected to /auth with returnTo`, async ({
      page,
    }) => {
      await page.goto(route)

      // Redirects to /auth
      await page.waitForURL(AUTH_URL_REGEX, { timeout: 10_000 })

      // returnTo query param preserves the original admin path so post-login
      // navigation can resume where the user intended.
      expect(page.url()).toContain(ADMIN_RETURN_TO)

      // Auth page rendered (sanity that we landed on the real auth surface,
      // not an error boundary).
      await expect(
        page.getByRole('heading', { name: /welcome to psychic homily/i })
      ).toBeVisible({ timeout: 5_000 })
    })
  }
})

test.describe('AdminGuard: authenticated non-admin access', () => {
  for (const route of ADMIN_SUBROUTES) {
    test(`authenticated non-admin user at ${route} is redirected to /`, async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto(route)

      // Redirect destination is the home page (NOT /auth — they're already
      // signed in; the guard demotes them to home).
      await authenticatedPage.waitForURL(
        (url) => url.pathname === '/',
        { timeout: 10_000 }
      )

      // Home page rendered (the "upcoming shows" heading is the same marker
      // home.spec.ts uses).
      await expect(
        authenticatedPage.getByRole('heading', { name: /upcoming shows/i })
      ).toBeVisible({ timeout: 10_000 })
    })
  }
})

test.describe('AdminGuard: authenticated admin access', () => {
  for (const route of ADMIN_SUBROUTES) {
    test(`admin user at ${route} sees the page without redirect`, async ({
      adminPage,
    }) => {
      await adminPage.goto(route)

      // No redirect away from the requested admin route.
      await expect(adminPage).toHaveURL(new RegExp(`${route}(\\?|$)`), {
        timeout: 10_000,
      })

      // The page rendered something — the loader spinner from AdminGuard
      // resolved into actual content. We don't assert page-specific markers
      // here because that would couple this guard test to each sub-page's
      // implementation; the per-page specs (admin/pending-shows.spec.ts,
      // admin/verify-venue.spec.ts) cover that. What we DO assert is that
      // the AdminGuard "Access Denied" card is NOT showing — the only
      // state that proves the guard let us through.
      await expect(
        adminPage.getByRole('heading', { name: 'Access Denied' })
      ).not.toBeVisible({ timeout: 10_000 })
    })
  }
})
