import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

/**
 * PSY-721: HTTP-404 regression guard for not-found pages.
 *
 * Why the status assertion is the point
 * -------------------------------------
 * Under Next 16's `cacheComponents: true` (PPR), an entity slug page that
 * calls `notFound()` rendered the not-found UI but committed HTTP **200**, not
 * 404 — the streaming-200 trap documented in `frontend/proxy.ts`. PSY-897
 * fixed this with a slug-existence proxy (Phase 1 #885 shows-only; Phase 2 #890
 * the other six types) that rewrites an unknown slug to a synthetic no-route
 * path so the framework returns a TRUE 404 before the stream commits.
 *
 * `proxy.ts` had ZERO automated coverage. This spec is that guard. The KEY
 * assertion in every invalid-slug case is `response.status() === 404` —
 * asserting not-found *content* alone is what let the original soft-404 bug
 * hide (200-with-404-content). We assert content only where it adds signal
 * (the global not-found UI, via the generic no-route case and the tag case),
 * and assert status everywhere.
 *
 * NOTE on the not-found UI rendered: proxy.ts rewrites every unknown entity
 * slug (all 7 types) to ONE synthetic no-route path (`/_psy-not-found`), which
 * renders the GLOBAL `app/not-found.tsx`. The route-local
 * `app/tags/[slug]/not-found.tsx` ("Tag Not Found" / "Back to Tags") is NOT
 * reached on a direct navigation — the proxy short-circuits before the tag page
 * renders — so the invalid-tag case asserts the GLOBAL 404 UI (verified against
 * the captured page snapshot during this spec's e2e run), not the tag-local UI.
 *
 * Scope (verified against frontend/proxy.ts on this branch's base)
 * ---------------------------------------------------------------
 * `ENTITY_CHECKS` maps exactly SEVEN types — shows, venues, artists, releases,
 * labels, festivals, tags — each `config.matcher`-intercepted. Six fetch the
 * backend `/<type>/<slug>`; tags is the lone exception, fetching the enriched
 * `/tags/<slug>/detail`, so the invalid-tag case exercises that distinct path.
 * All seven are covered below.
 *
 * Intentionally EXCLUDED (still soft-404 / 200 — asserting 404 would FAIL):
 *   - collections (auth-gated soft-404)
 *   - blog, dj-sets (local MDX soft-404)
 *   - scenes (renders a fabricated page — separate bug PSY-906)
 * These are PSY-897 Phase 3 / PSY-906 and get coverage when their fixes land.
 *
 * The `error-detection` fixture (no auth — these are public, read-only routes)
 * auto-fails on console.error / failed browser request / 5xx. A clean 404 page
 * produces none of those: the proxy's backend existence check runs server-side
 * (inside `proxy()`), so the browser only ever sees the final 404 from Next —
 * not the proxy's internal backend lookup.
 *
 * Selectors prefer `getByRole`/`getByText` (PSY-859 anti-false-coverage) and
 * scope headings to `getByRole('main')` so the persistent topbar/sidebar
 * (app/layout.tsx wraps `{children}` in `<main className="flex-1">`) can't
 * satisfy a heading query.
 *
 * Per-run random suffix on every invalid slug so it can never collide with a
 * seeded row (which would resolve -> 200 and silently weaken the test).
 */

/** Unique-per-run token; appended to every invalid slug to avoid seed collision. */
const NONCE = `psy721-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

/**
 * The seven entity types `proxy.ts` existence-checks. An invalid slug on each
 * must yield a TRUE HTTP 404 (backend 404 -> proxy rewrite -> no-route 404).
 * `tags` is listed because it alone takes the `/tags/<slug>/detail` backend
 * path — covering it guards that branch specifically.
 */
const PROXY_FIXED_ENTITIES = [
  'shows',
  'venues',
  'artists',
  'releases',
  'labels',
  'festivals',
  'tags',
] as const

/**
 * Reserved, stable seeded slugs (from frontend/e2e/setup-db.sh) for the
 * over-404 regression guard. Both are proxy-intercepted types, so a 200 here
 * proves the proxy lets REAL pages through (it doesn't 404 valid slugs).
 * Reserved rows (not `.first()` cards) so parallel mutating workers can't
 * disturb them — same rationale as navigation.spec.ts.
 */
const VALID_SEEDED = [
  { label: 'show', path: '/shows/e2e-attendance-test' },
  { label: 'venue', path: '/venues/e2e-favorite-venue-test' },
] as const

test.describe('Not-found pages — HTTP 404 status', () => {
  test('generic no-route match returns HTTP 404 with global not-found UI', async ({
    page,
  }) => {
    const response = await page.goto(`/this-route-does-not-exist-${NONCE}`)

    // KEY assertion: real 404 status, not a soft-404 200.
    expect(response?.status()).toBe(404)

    // Global app/not-found.tsx content (rendered inside <main>).
    const main = page.getByRole('main')
    await expect(
      main.getByRole('heading', { name: '404', level: 1 })
    ).toBeVisible({ timeout: 10_000 })
    await expect(main.getByText('Page not found')).toBeVisible()
    // "Go home" link is the global not-found's primary CTA.
    await expect(main.getByRole('link', { name: 'Go home' })).toBeVisible()
  })

  for (const entity of PROXY_FIXED_ENTITIES) {
    test(`invalid /${entity}/<slug> returns HTTP 404`, async ({ page }) => {
      const response = await page.goto(`/${entity}/nonexistent-${NONCE}`)

      // KEY assertion: proxy.ts rewrote the unknown slug to a real 404.
      expect(
        response?.status(),
        `invalid /${entity} slug must return 404 (proxy.ts existence check); ` +
          `a 200 means the proxy regressed or this type isn't covered`
      ).toBe(404)
    })
  }

  test('invalid tag slug returns HTTP 404 with GLOBAL not-found UI (proxy bypasses route-local not-found)', async ({
    page,
  }) => {
    // The invalid-tag case exercises proxy.ts's lone non-uniform backend path
    // (`/tags/<slug>/detail`, vs `/<type>/<slug>` for the other six). The status
    // assertion below proves that branch 404s correctly.
    const response = await page.goto(`/tags/nonexistent-${NONCE}`)
    expect(response?.status()).toBe(404)

    // IMPORTANT — proxy.ts rewrites EVERY unknown entity slug (all 7 types) to
    // a SINGLE synthetic no-route path (`/_psy-not-found`). That path renders
    // the GLOBAL `app/not-found.tsx`, NOT the route-local
    // `app/tags/[slug]/not-found.tsx`. The tag page never renders on a direct
    // navigation (the proxy short-circuits before it), so its co-located
    // not-found.tsx ("Tag Not Found" / "Back to Tags") does NOT fire here.
    // (It would only fire on a client-side RSC prefetch, which proxy.ts's
    // `missing: [next-router-prefetch, purpose:prefetch]` excludes — not a
    // full `page.goto`.) So we assert the global 404 UI that actually renders.
    // Verified against the captured page snapshot during the PSY-721 e2e run.
    const main = page.getByRole('main')
    await expect(
      main.getByRole('heading', { name: '404', level: 1 })
    ).toBeVisible({ timeout: 10_000 })
    await expect(main.getByText('Page not found')).toBeVisible()
  })

  for (const { label, path } of VALID_SEEDED) {
    test(`valid seeded ${label} returns HTTP 200 (proxy does not over-404)`, async ({
      page,
    }) => {
      const response = await page.goto(path)

      // Over-404 guard: a real, seeded slug must still resolve to 200. If this
      // flips to 404, proxy.ts is incorrectly 404-ing valid pages.
      expect(
        response?.status(),
        `valid seeded ${label} (${path}) must return 200 — proxy.ts must not over-404 real pages`
      ).toBe(200)

      // Sanity: the success-state H1 (entity name) renders, not a 404 body.
      await expect(
        page.getByRole('main').getByRole('heading', { level: 1 })
      ).toBeVisible({ timeout: 10_000 })
    })
  }

  /**
   * PSY-913 over-404 guard for RESERVED static routes under a proxied prefix.
   *
   * `proxy.ts`'s `/shows/:path*` matcher intercepts `/shows/submit` and
   * `/shows/saved` — both are REAL Next routes (`app/shows/submit/page.tsx`,
   * `app/shows/saved/page.tsx`), NOT entity slugs. PSY-897's shows phase made
   * the proxy existence-check `submit`/`saved` as if they were slugs → backend
   * 404 → rewrote the genuine page to a 404, breaking the full E2E suite on
   * main (`submit-show.spec.ts`). `RESERVED_SEGMENTS` excludes them. These tests
   * pin that the proxy lets each real page through. (Distinct from the seeded-
   * slug guard above: a slug-existence backend call would 404 these; the
   * exclusion is what saves them.)
   */
  test.describe('Reserved static routes under proxied prefixes (PSY-913)', () => {
    test('/shows/submit is NOT 404-ed by proxy (real route renders, anon → client-redirect to /auth)', async ({
      page,
    }) => {
      // `/shows/submit` is a client component with no server `notFound()`, so the
      // INITIAL document is HTTP 200 (pre-fix the proxy rewrote it to the
      // synthetic no-route path → 404). The page then client-redirects anon
      // visitors to `/auth` (verified: app/shows/submit/page.tsx useEffect
      // router.push('/auth?returnTo=%2Fshows%2Fsubmit')). Both assertions
      // together prove the proxy let the real page render+hydrate, not 404.
      const response = await page.goto('/shows/submit')
      expect(
        response?.status(),
        '/shows/submit must return 200 — proxy.ts must not existence-check this static route (PSY-913)'
      ).toBe(200)

      // The anon client-redirect only fires if the real page hydrated. A 404
      // body would never reach this useEffect.
      await page.waitForURL(/\/auth/, { timeout: 10_000 })
      await expect(
        page.getByText('Sign in to your account')
      ).toBeVisible({ timeout: 5_000 })
    })

    test('/shows/saved is NOT 404-ed by proxy (server-redirects to /library → anon lands on /auth)', async ({
      page,
    }) => {
      // `/shows/saved` is a server component that unconditionally
      // `redirect('/library')` (no auth gate — verified app/shows/saved/page.tsx).
      // `page.goto` follows the server redirect; the final document is the
      // `/library` 200 (its own client-side anon-redirect to /auth fires after).
      // Pre-fix the proxy rewrote `/shows/saved` to the synthetic no-route path
      // → 404, so the redirect chain never ran. The 200 below is the load-bearing
      // proof the exclusion let the real route through (PSY-913).
      const response = await page.goto('/shows/saved')
      expect(
        response?.status(),
        '/shows/saved must resolve to 200 (server redirect to /library) — proxy.ts must not existence-check this static route (PSY-913)'
      ).toBe(200)

      // Settle point: /library is a client component that redirects anon
      // visitors to /auth (verified app/library/page.tsx LibraryContent). A
      // 404 body would never reach that redirect, so landing on /auth confirms
      // the real route rendered.
      await page.waitForURL(/\/auth/, { timeout: 10_000 })
    })
  })
})
