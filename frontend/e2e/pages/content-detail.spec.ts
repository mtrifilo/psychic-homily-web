import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

/**
 * PSY-723: SEO/render coverage for content detail pages.
 *
 * Each content detail route is a Next.js server component with a
 * `generateMetadata` (title + canonical) plus a JSON-LD `<script>` and a
 * level-1 heading. These are silent-regression surfaces: a broken
 * `generateMetadata`, a dropped canonical, or a missing JSON-LD block hurts
 * SEO and browsing without throwing a visible error. This spec asserts, per
 * covered content type:
 *   1. the page loads and its H1 (entity name) is visible;
 *   2. a `<link rel="canonical">` is present in <head> and points at the
 *      page's own URL;
 *   3. a `<script type="application/ld+json">` is present and parses.
 *
 * The `error-detection` fixture (no auth needed — these are public,
 * read-only pages) auto-fails the test on any console.error / failed
 * request / 5xx, so each assertion lands on real content rather than an
 * error state.
 *
 * Selectors prefer `getByRole`/`getByText` over class selectors (PSY-859
 * anti-false-coverage guidance). The success-state H1 is rendered by the
 * shared `EntityHeader` (components/shared/EntityHeader.tsx) as
 * `<h1>{title}</h1>` — for releases the release title, for labels the label
 * name. (Two other `<h1>`s exist in *Detail.tsx but only on the error/404
 * branch, which we never hit for a seeded slug.)
 *
 * ── SEED SCOPE (verified against frontend/e2e/setup-db.sh) ───────────────
 * Of the six content detail routes
 *   /blog/[slug]  /releases/[slug]  /festivals/[slug]
 *   /scenes/[slug]  /labels/[slug]  /dj-sets/[slug]
 * the E2E seed only INSERTs into `releases` and `labels`. There are no
 * seeded rows for blog_posts, festivals, scenes, or dj-sets/playlists, so
 * those four routes have NO stable slug to navigate to. Per PSY-722's
 * precedent (and PSY-859's anti-false-coverage rule) we cover ONLY the two
 * seeded types here and disclose the gap in the PR's scope-deviation note
 * rather than fabricating fixtures or writing `.skip`/Not-Found placeholders.
 * Recommended follow-up: a seed ticket adding one row per uncovered type so
 * the remaining four can be covered the same way.
 *
 * ── JSON-LD NOTE ─────────────────────────────────────────────────────────
 * The release/label *page* components do not emit a page-specific JSON-LD
 * block (unlike artists/shows/venues/blog/dj-sets). However the root layout
 * (app/layout.tsx) renders a global Organization schema
 * `<script type="application/ld+json">` (via generateOrganizationSchema) in
 * <head> on EVERY page. So the presence assertion below is satisfied by that
 * global block — which is the SEO signal these pages actually ship. The test
 * asserts presence + valid JSON + a `@type`, not a release/label-specific
 * schema, to avoid claiming coverage the page doesn't provide.
 *
 * Stable seeded slugs used below (from setup-db.sh):
 *   - release "Futures"               -> /releases/futures
 *   - label   "Run For Cover Records" -> /labels/run-for-cover-records
 */

interface CoveredType {
  /** Human label for the test title. */
  label: string
  /** Route path including the seeded slug. */
  path: string
  /** Exact H1 text the success-state EntityHeader renders. */
  heading: string
  /** Absolute canonical href set by the route's generateMetadata. */
  canonical: string
}

const COVERED_TYPES: CoveredType[] = [
  {
    label: 'release',
    path: '/releases/futures',
    heading: 'Futures',
    canonical: 'https://psychichomily.com/releases/futures',
  },
  {
    label: 'label',
    path: '/labels/run-for-cover-records',
    heading: 'Run For Cover Records',
    canonical: 'https://psychichomily.com/labels/run-for-cover-records',
  },
]

test.describe('Content detail pages — SEO + render', () => {
  for (const ct of COVERED_TYPES) {
    test(`${ct.label} detail: loads, canonical + JSON-LD present`, async ({
      page,
    }) => {
      await page.goto(ct.path)

      // 1. Page loaded: the entity-name H1 (EntityHeader) is visible. We scope
      //    to the page <main> so the persistent sidebar/topbar can't satisfy
      //    the query, and target level 1 to skip section sub-headings.
      await expect(
        page
          .getByRole('main')
          .getByRole('heading', { name: ct.heading, level: 1 })
      ).toBeVisible({ timeout: 10_000 })

      // 2. Canonical <link> in <head> points at this page's own URL. Next.js
      //    resolves `alternates.canonical` against metadataBase
      //    (https://psychichomily.com), so the rendered href is the absolute
      //    production URL — NOT the localhost test origin. `<link>` is not an
      //    ARIA role, so query the DOM attribute directly.
      const canonical = page.locator('link[rel="canonical"]')
      await expect(canonical).toHaveCount(1)
      await expect(canonical).toHaveAttribute('href', ct.canonical)

      // 3. JSON-LD structured-data block is present and parses. (Satisfied by
      //    the global Organization schema in app/layout.tsx — see file header.)
      const ldScripts = page.locator('script[type="application/ld+json"]')
      const ldCount = await ldScripts.count()
      expect(ldCount).toBeGreaterThan(0)

      const firstLd = await ldScripts.first().textContent()
      expect(firstLd, 'JSON-LD script should have content').toBeTruthy()
      const parsed = JSON.parse(firstLd ?? '{}')
      expect(parsed['@type'], 'JSON-LD should declare an @type').toBeTruthy()
    })
  }
})
