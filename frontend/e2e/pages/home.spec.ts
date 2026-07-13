import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Homepage', () => {
  test('loads and displays upcoming shows', { tag: '@smoke' }, async ({ page }) => {
    await page.goto('/')

    // Page title (PSY-389: global "%s | Psychic Homily" template, no
    // "Arizona Music Community").
    await expect(page).toHaveTitle(/Psychic Homily/)

    // Discovery hero (PSY-389; PSY-1137 animated wordmark). The <h1> is the
    // "Psychic Homily" wordmark (an sr-only heading backs the decorative canvas
    // for SEO/a11y); the tagline below it is supporting copy, not a heading.
    await expect(
      page.getByRole('heading', { name: 'Psychic Homily', level: 1 })
    ).toBeVisible()
    await expect(page.getByText('Your music knowledge graph.')).toBeVisible()

    // "Upcoming shows" section heading
    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()

    // "View all shows" link to /shows (quiet link above the list)
    await expect(
      page.getByRole('link', { name: /view all shows/i }).first()
    ).toBeVisible()

    // Wait for show cards to load (client-side fetch via TanStack Query)
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Verify at least one show card has a link (artist or venue)
    const firstShow = page.locator('article').first()
    await expect(firstShow.locator('a').first()).toBeVisible()
    await expect(firstShow.getByRole('link', { name: 'Details' })).toBeVisible()
  })

  test('displays navigation links', async ({ page }) => {
    await page.goto('/')

    // Top-bar primary nav (PSY-1013): explicit links + the menu triggers,
    // visible on the default 1280x720 viewport (>= the lg breakpoint). Scope to
    // the Primary nav landmark — the editorial footer (PSY-389) and the hero
    // discovery row also expose "Shows"/"Artists" links, so an unscoped
    // getByRole would strict-mode-violate.
    const primaryNav = page.getByRole('navigation', { name: 'Primary' })
    await expect(primaryNav.getByRole('link', { name: 'Shows' })).toBeVisible()
    await expect(primaryNav.getByRole('link', { name: 'Artists' })).toBeVisible()
    await expect(primaryNav.getByRole('button', { name: 'Browse the catalog' })).toBeVisible()
    await expect(primaryNav.getByRole('button', { name: 'Contribute' })).toBeVisible()

    // Login link visible when not authenticated
    await expect(page.getByRole('link', { name: /login/i })).toBeVisible()

    // Destinations the retired sidebar exposed directly are now reachable
    // inside the menus (no discoverability regression — PSY-1013).
    await primaryNav.getByRole('button', { name: 'Browse the catalog' }).click()
    await expect(page.getByRole('menuitem', { name: 'Venues' })).toBeVisible()
    await page.keyboard.press('Escape')

    await primaryNav.getByRole('button', { name: 'Contribute' }).click()
    await expect(page.getByRole('menuitem', { name: 'Blog' })).toBeVisible()
    await expect(page.getByRole('menuitem', { name: 'DJ Sets' })).toBeVisible()
  })

  test('displays discovery sections and footer (PSY-389)', async ({ page }) => {
    await page.goto('/')

    // Discover quick-links row in the hero
    await expect(
      page.getByRole('link', { name: 'Shows in any city' })
    ).toBeVisible()

    // Scene-graph section (PSY-1344/1450). Lazy-mounted on scroll intent, and
    // the heading names whichever seeded scene is liveliest — assert the
    // show-anchored suffix. The e2e seed carries a metro'd
    // Phoenix scene (PSY-1319), so the section must not self-hide. Scroll
    // deterministically to the section BELOW it (radio renders immediately)
    // rather than a magic wheel delta, so content growth can't strand the
    // observer.
    await page
      .getByRole('heading', { name: /latest radio shows/i })
      .scrollIntoViewIfNeeded()
    await expect(
      page.getByRole('heading', { name: /, this week$/i })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('link', { name: /open the graph/i })
    ).toBeVisible()
    // The graph area must settle to a real state — canvas (aria-label) or
    // the honest empty-roster fallback — and never the error card. This
    // catches a 500ing graph endpoint or a failed ForceGraphView chunk,
    // which the heading/CTA assertions alone would miss. (The seed stamps
    // venue metros but not artist home metros, so an empty based-here
    // roster is a legitimate settled state here.)
    const graph = page.getByLabel(/knowledge graph of the .* scene/i)
    const emptyGraph = page.getByText(/not enough connected artists/i)
    await expect(graph.or(emptyGraph)).toBeVisible({ timeout: 15_000 })
    // Count copy is truthful only when a settled canvas roster exists; the
    // fallback deliberately carries no synthetic "0 artists" caption.
    const graphCaption = page.getByText(
      /most connected artists playing or tied to .* this month/i
    )
    if (await graph.isVisible()) await expect(graphCaption).toBeVisible()
    else await expect(graphCaption).toHaveCount(0)
    await expect(page.getByText(/the graph couldn’t load/i)).toHaveCount(0)

    // Latest radio shows section + station cards deep-linking to their /radio
    // tabs. Card content is real data now (PSY-1329), so assert only the
    // stable editorial aria-label prefix (call sign · city) — the seeded e2e
    // DB may or may not carry radio episodes.
    await expect(
      page.getByRole('heading', { name: /latest radio shows/i })
    ).toBeVisible()
    await expect(
      page.getByRole('link', { name: /KEXP · Seattle/i })
    ).toHaveAttribute('href', '/radio/kexp')

    // Editorial footer columns (PSY-389). Scope to the footer landmark — the
    // hero also renders a "Discover" quick-links nav, so an unscoped match is
    // a strict-mode violation (two `navigation[name="Discover"]` on the page).
    await expect(
      page.getByRole('contentinfo').getByRole('navigation', { name: 'Discover' })
    ).toBeVisible()
    await expect(
      page.getByText('Made by the scene, for the scene.')
    ).toBeVisible()
  })
})
