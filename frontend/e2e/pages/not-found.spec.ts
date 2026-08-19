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
 * `ENTITY_CHECKS` maps exactly EIGHT types — shows, venues, artists, releases,
 * labels, festivals, tags, scenes — each `config.matcher`-intercepted. Seven
 * fetch the backend `/<type>/<slug>`; tags is the lone exception, fetching the
 * enriched `/tags/<slug>/detail`, so the invalid-tag case exercises that
 * distinct path. `scenes` (PSY-906) is a derived city/state aggregation whose
 * `/scenes/<slug>` backend call 404s for an unresolvable or below-threshold
 * slug. All eight are covered below.
 *
 * Intentionally EXCLUDED (still soft-404 / 200 — asserting 404 would FAIL):
 *   - collections (auth-gated soft-404)
 *   - blog, dj-sets (local MDX soft-404)
 * These get coverage when their fixes land.
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
 * The eight entity types `proxy.ts` existence-checks. An invalid slug on each
 * must yield a TRUE HTTP 404 (backend 404 -> proxy rewrite -> no-route 404).
 * `tags` is listed because it alone takes the `/tags/<slug>/detail` backend
 * path — covering it guards that branch specifically. `scenes` (PSY-906) is a
 * derived aggregation; an unresolvable slug 404s at `/scenes/<slug>`.
 */
const PROXY_FIXED_ENTITIES = [
  'shows',
  'venues',
  'artists',
  'releases',
  'labels',
  'festivals',
  'tags',
  'scenes',
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
  { label: 'venue', path: '/venues/e2e-reserved-venue' },
  // Phoenix, AZ qualifies as a scene: setup-db.sh seeds 3 verified Phoenix
  // venues (Rebel Lounge / Crescent Ballroom / Valley Bar) — above the ≥2
  // verified-venue threshold — plus approved Phoenix shows. The slug is derived
  // (city+state), so no single row a parallel worker could delete gates it.
  { label: 'scene', path: '/scenes/phoenix-az' },
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
  /**
   * Scene-week routes (PSY-1577) — the proxy branch that handles a THIRD path
   * segment under `/scenes/`.
   *
   * The generic entity check only inspects the 3-segment `/<entity>/<slug>`
   * shape, so `/scenes/<slug>/<week>` fell straight through to
   * `NextResponse.next()` and every bad week key soft-404'd: the global 404 body
   * committed at HTTP 200. These pin both directions of the new branch, because
   * the proxy is the only thing that can produce a real 404 here — the page's
   * own `notFound()` fires after the shell has already streamed.
   */
  test.describe('Scene week routes (PSY-1577)', () => {
    test('rolling /scenes/<slug>/week returns HTTP 200 (proxy does not over-404)', async ({
      page,
    }) => {
      const response = await page.goto('/scenes/phoenix-az/week')
      expect(
        response?.status(),
        '/scenes/phoenix-az/week must return 200 — proxy.ts must not over-404 the rolling week route'
      ).toBe(200)
      await expect(
        page.getByRole('main').getByRole('heading', { level: 1 })
      ).toBeVisible({ timeout: 10_000 })
    })

    test('a week key that does not exist returns HTTP 404', async ({ page }) => {
      // 2025 has 52 ISO weeks, so 2025-W53 is well-formed but unreal. Only the
      // backend can say so, which is why the proxy existence-checks the week
      // endpoint rather than re-deriving the calendar in TS.
      const response = await page.goto('/scenes/phoenix-az/2025-W53')
      expect(
        response?.status(),
        '/scenes/phoenix-az/2025-W53 must return 404 — a well-formed but nonexistent week must not soft-404'
      ).toBe(404)
    })

    test('a non-week segment under a scene returns HTTP 404', async ({ page }) => {
      const response = await page.goto('/scenes/phoenix-az/garbage')
      expect(
        response?.status(),
        '/scenes/phoenix-az/garbage must return 404 — no route can serve it'
      ).toBe(404)
    })

    test('a week under an unresolvable scene returns HTTP 404', async ({ page }) => {
      const response = await page.goto('/scenes/nowhere-zz/week')
      expect(
        response?.status(),
        '/scenes/nowhere-zz/week must return 404 — the scene itself does not resolve'
      ).toBe(404)
    })
  })

  /**
   * Scene WINDOW routes (PSY-1849) — the same third-segment branch, for the two
   * multi-day rolling windows.
   *
   * These are the reason the branch needed extending at all: `proxy.ts` 404s any
   * segment under `/scenes/<slug>/` that it does not recognise, so both routes
   * were hard-404ed before Next ever saw them. Nothing short of reading the HTTP
   * status can tell that from a page choosing not to exist, which is why it is
   * pinned here rather than in the unit suite.
   */
  test.describe('Scene window routes (PSY-1849)', () => {
    for (const segment of ['this-weekend', 'next-4-weeks']) {
      test(`rolling /scenes/<slug>/${segment} returns HTTP 200 (proxy does not over-404)`, async ({
        page,
      }) => {
        const response = await page.goto(`/scenes/phoenix-az/${segment}`)
        expect(
          response?.status(),
          `/scenes/phoenix-az/${segment} must return 200 — proxy.ts must not over-404 the rolling window route`
        ).toBe(200)
        await expect(
          page.getByRole('main').getByRole('heading', { level: 1 })
        ).toBeVisible({ timeout: 10_000 })
      })

      test(`/${segment} under an unresolvable scene returns HTTP 404`, async ({ page }) => {
        const response = await page.goto(`/scenes/nowhere-zz/${segment}`)
        expect(
          response?.status(),
          `/scenes/nowhere-zz/${segment} must return 404 — the scene itself does not resolve`
        ).toBe(404)
      })
    }
  })

  /**
   * Scene-day routes (PSY-1627) — the same third-segment branch, for the
   * nightly pages.
   *
   * Same failure mode as the week routes above and the same reason it can only
   * be caught here: the proxy is the only thing that can produce a real 404 for
   * these paths, and nothing short of reading the HTTP status can tell a 404
   * from a 404 body served at 200.
   */
  test.describe('Scene day routes (PSY-1627)', () => {
    test('rolling /scenes/<slug>/tonight returns HTTP 200 (proxy does not over-404)', async ({
      page,
    }) => {
      const response = await page.goto('/scenes/phoenix-az/tonight')
      expect(
        response?.status(),
        '/scenes/phoenix-az/tonight must return 200 — proxy.ts must not over-404 the rolling day route'
      ).toBe(200)
      // The SCENE, not merely "an h1 rendered". `app/not-found.tsx` puts its
      // own <h1>404</h1> inside the same <main>, so a presence check passes on
      // a soft-404 — which is the one failure this whole block exists to
      // catch, and the one the cheap scene probe could reintroduce.
      await expect(
        page.getByRole('main').getByRole('heading', { level: 1 })
      ).toContainText('Phoenix', { timeout: 10_000 })
    })

    // The `[period]` segment now dispatches BOTH shapes from one route. Nothing
    // else asserts the week branch still resolves to the week view: the week
    // block above is decided entirely by proxy.ts before the page component
    // runs, so a branch-order regression there would ship green.
    test('the shared period route still renders the WEEK view for a week key', async ({
      page,
    }) => {
      const response = await page.goto('/scenes/phoenix-az/2026-W31')
      expect(response?.status()).toBe(200)
      // A marker only the week view emits — the day view has no week range.
      await expect(page.getByRole('main')).toContainText(/Mon,.*–.*Sun,/, {
        timeout: 10_000,
      })
    })

    test('the permalink /tonight declares canonical returns HTTP 200', async ({ page }) => {
      // A canonical that 404s is worse than no canonical at all. Read the
      // target OFF the page rather than hardcoding this week's key: a fixed key
      // would keep passing while quietly testing nothing, since any valid week
      // 200s. This way the assertion follows the real declared relationship.
      await page.goto('/scenes/phoenix-az/tonight')
      const href = await page.locator('link[rel="canonical"]').getAttribute('href')

      expect(href, '/tonight must declare a canonical').toBeTruthy()
      // The WEEK permalink, not the dated day permalink: day permalinks are
      // announced in no sitemap, so canonicalizing to one aimed crawlers at a
      // URL the site never tells them about.
      expect(
        href,
        'the canonical must be the WEEK permalink, never the rolling URL or a dated day'
      ).toMatch(/\/scenes\/phoenix-az\/\d{4}-W\d{2}$/)

      // The PATH, against the server under test. The canonical is absolute and
      // points at the production origin, so navigating to it verbatim would
      // leave the branch entirely and assert a fact about the live site.
      const { pathname } = new URL(href!)
      const response = await page.goto(pathname)
      expect(
        response?.status(),
        `${pathname} must return 200 — it is what /tonight points every crawler at`
      ).toBe(200)
    })

    /**
     * A real DATED day permalink still resolves.
     *
     * Kept as its own test now that the canonical probe above follows the week
     * key. A dated permalink is no longer any rolling page's canonical, but it
     * is still linked from every day page's prev/next chips, and this is the
     * only place the assembled path (proxy.ts's date branch, then `[period]`'s
     * `looksLikeCalendarDate` dispatch to the DAY view) is exercised against a
     * date the server agrees exists. Without it, a change that 404s or
     * soft-404s every dated permalink ships green.
     */
    test('a real dated day permalink returns HTTP 200 and renders the DAY view', async ({
      page,
    }) => {
      // Read the date off the page rather than hardcoding today's: a fixed date
      // would keep passing while testing nothing, since any valid date 200s.
      //
      // Parsed in the page rather than via a Playwright text filter because the
      // day route emits SEVERAL ld+json blocks and one of them is a top-level
      // ARRAY (the MusicEvent graph), so the block has to be identified by its
      // parsed @type, not by matching source text.
      await page.goto('/scenes/phoenix-az/tonight')
      const leaf = await page.evaluate(() => {
        const blocks = document.querySelectorAll('script[type="application/ld+json"]')
        for (const block of Array.from(blocks)) {
          const parsed = JSON.parse(block.textContent ?? 'null')
          if (parsed && !Array.isArray(parsed) && parsed['@type'] === 'BreadcrumbList') {
            return parsed.itemListElement[parsed.itemListElement.length - 1].item as string
          }
        }
        return null
      })

      expect(leaf, '/tonight must emit a BreadcrumbList').toBeTruthy()
      expect(
        leaf,
        'the breadcrumb leaf must name the dated day permalink'
      ).toMatch(/\/scenes\/phoenix-az\/\d{4}-\d{2}-\d{2}$/)

      const { pathname } = new URL(leaf!)
      const response = await page.goto(pathname)
      expect(
        response?.status(),
        `${pathname} must return 200 — it is linked from every day page`
      ).toBe(200)
      await expect(
        page.getByRole('main').getByRole('heading', { level: 1 })
      ).toContainText('Phoenix', { timeout: 10_000 })

      // The DAY view, not the week view. `[period]` dispatches both shapes, so
      // a branch-order regression there would otherwise ship green. Asserted on
      // the canonical rather than on rendered prose: a dated day permalink is
      // its own canonical, while the week view would declare a `-W##` key.
      const served = await page.locator('link[rel="canonical"]').getAttribute('href')
      expect(
        served,
        'a dated day permalink must self-canonicalize, proving the DAY branch ran'
      ).toBe(`https://psychichomily.com${pathname}`)
    })

    test('a date that does not exist returns HTTP 404', async ({ page }) => {
      // February never has 30 days. Go's date parser NORMALIZES this to March
      // 2nd rather than rejecting it, so only the backend's round-trip check
      // stands between an impossible date and a duplicate URL for a real day.
      const response = await page.goto('/scenes/phoenix-az/2026-02-30')
      expect(
        response?.status(),
        '/scenes/phoenix-az/2026-02-30 must return 404 — an impossible date must not soft-404'
      ).toBe(404)
    })

    test('tonight under an unresolvable scene returns HTTP 404', async ({ page }) => {
      const response = await page.goto('/scenes/nowhere-zz/tonight')
      expect(
        response?.status(),
        '/scenes/nowhere-zz/tonight must return 404 — the scene itself does not resolve'
      ).toBe(404)
    })
  })
})
