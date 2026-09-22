import { test, expect } from '../fixtures'
import { USER_COUNT, userAuthFileForWorker } from '../global-setup'
import {
  lookupWorkerUserId,
  resetTestFixtures,
} from '../fixtures/test-fixtures-reset'

// The homepage has two server-picked viewer variants (PSY-2103). These are the
// ANONYMOUS one; the signed-in variant has its own describe below.
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
    // Scope to <main>: the footer brand column carries this same tagline
    // (PSY-1533), so an unscoped match resolves to two elements and
    // strict-mode-violates. The footer is a sibling of <main> in the root
    // layout, so this isolates the hero copy.
    await expect(
      page.getByRole('main').getByText('Your music knowledge graph.')
    ).toBeVisible()

    // Current stats band (PSY-1431) — global stats under the hero.
    await expect(
      page.getByRole('region', { name: /current stats/i })
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('shows in the next 7 days')).toBeVisible()
    await expect(page.getByText('entities in the graph')).toBeVisible()

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
    // the heading names whichever seeded scene is liveliest, so match the whole
    // heading with the city as the wildcard. Fully anchored on purpose: the
    // section's copy must carry no time window, and an unanchored pattern would
    // still pass on a reintroduced one. The e2e seed carries a metro'd
    // Phoenix scene (PSY-1319), so the section must not self-hide. Scroll
    // deterministically to the section BELOW it (radio renders immediately)
    // rather than a magic wheel delta, so content growth can't strand the
    // observer.
    await page
      .getByRole('heading', { name: /latest radio shows/i })
      .scrollIntoViewIfNeeded()
    await expect(
      page.getByRole('heading', { name: /^the .+ scene graph$/i })
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
    const emptyGraph = page.getByText(/not enough connected names/i)
    await expect(graph.or(emptyGraph)).toBeVisible({ timeout: 15_000 })
    // Count copy is truthful only when a settled canvas roster exists; the
    // fallback deliberately carries no synthetic "0 artists" caption.
    // `[^.]*` rather than `.*` for the city: a greedy wildcard would happily
    // swallow a reintroduced "… tied to Phoenix this month." and still pass.
    const graphCaption = page.getByText(
      /of the most connected artists tied to [^.]*\. Ties like shared bills and labels link them/i
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
      page.getByRole('contentinfo').getByText('Your music knowledge graph.')
    ).toBeVisible()
  })

  test('never offers the customize-home toolbar (PSY-2104)', async ({ page }) => {
    await page.goto('/')

    await expect(
      page.getByRole('button', { name: 'Customize home' })
    ).toHaveCount(0)
    await expect(page.getByText(/your layout/i)).toHaveCount(0)
  })
})

test.describe('Homepage (signed in)', () => {
  // PSY-2103: the saved-shows module replaces the wordmark hero, and the
  // general "Upcoming shows" section is gone for signed-in viewers.
  //
  // These tests assert the ZERO state on a worker user that every other spec
  // on this worker shares, and save-show.spec unsaves only as its last step. A
  // reset at SETUP is what protects the first attempt from a sibling's
  // leftovers; `cleanBetweenRetries` runs at teardown and only protects a
  // retry, so both are used.
  test.beforeEach(async ({}, testInfo) => {
    const authFile = userAuthFileForWorker(testInfo.workerIndex % USER_COUNT)
    await resetTestFixtures(await lookupWorkerUserId(authFile))
  })
  test('replaces the hero with the saved-shows module', async ({
    authenticatedPage,
    cleanBetweenRetries: _cleanup,
  }) => {
    await authenticatedPage.goto('/')

    await expect(
      authenticatedPage.getByRole('heading', {
        name: 'Your upcoming shows',
        level: 1,
      })
    ).toBeVisible()

    // The logged-out hero and its sign-up nudge are gone.
    await expect(
      authenticatedPage.getByRole('heading', {
        name: 'Psychic Homily',
        level: 1,
      })
    ).toHaveCount(0)

    // The general list is gone, and /shows is still one click away.
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Upcoming shows', exact: true })
    ).toHaveCount(0)
    await expect(
      authenticatedPage.getByRole('link', { name: /view all shows/i })
    ).toHaveCount(0)
    await expect(
      authenticatedPage.getByRole('link', { name: 'Find a show' })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('link', { name: /view all in library/i })
    ).toBeVisible()

    // Zero-saves prompt row.
    await expect(
      authenticatedPage.getByText('Save a show and it shows up here.')
    ).toBeVisible()

    // The nearby list renders for every signed-in viewer, saves or none.
    await expect(
      authenticatedPage.getByRole('heading', {
        name: /Shows (near you )?this week/,
      })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('link', { name: /all upcoming shows/i })
    ).toBeVisible()
    await expect(
      authenticatedPage.locator('article').first()
    ).toBeVisible({ timeout: 10_000 })
  })

  // AC3 (second half) and AC4: the save loop this ticket exists to close.
  // Saving from the nearby list must move the row into the module with no
  // reload, and unsaving from the module must send it back and reach /library.
  test('moves a show between the nearby list and the module without a reload', async ({
    authenticatedPage,
    cleanBetweenRetries: _cleanup,
  }) => {
    await authenticatedPage.goto('/')

    // The heading drops "near you" when the city was only a guess (no geo in
    // the e2e stack), so match either form.
    const nearby = authenticatedPage.getByRole('region', {
      name: /Shows (near you )?this week/,
    })
    await expect(nearby.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Pin the row by its own accessible name so the assertions below cannot
    // silently follow a different show when the list reorders.
    const firstRow = nearby.locator('article').first()
    const savedTitle = await firstRow.getAttribute('aria-label')
    expect(savedTitle).toBeTruthy()

    const savedModule = authenticatedPage.getByRole('region', {
      name: 'Your upcoming shows',
    })

    await firstRow.getByRole('button', { name: 'Save show' }).click()

    // No goto(): the row must cross over on invalidation alone.
    const savedRow = savedModule.getByRole('article', {
      name: savedTitle!,
      exact: true,
    })
    await expect(savedRow).toBeVisible({ timeout: 10_000 })
    await expect(
      nearby.getByRole('article', { name: savedTitle!, exact: true })
    ).toHaveCount(0)
    await expect(savedModule.getByText(/1 saved/)).toBeVisible()

    // Unsave from the module: the row leaves it and returns to the nearby list.
    await savedRow
      .getByRole('button', { name: /remove from saved shows/i })
      .click()

    await expect(savedRow).toHaveCount(0, { timeout: 10_000 })
    await expect(
      authenticatedPage.getByText('Save a show and it shows up here.')
    ).toBeVisible()

    // ...and /library agrees on the next visit.
    await authenticatedPage.goto('/library')
    await expect(
      authenticatedPage.getByRole('main').getByText('Nothing saved yet.')
    ).toBeVisible({ timeout: 10_000 })
  })

  test('keeps the discovery sections below the module', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/')

    await expect(
      authenticatedPage.getByRole('link', { name: 'Shows in any city' })
    ).toBeVisible()

    await expect(
      authenticatedPage.getByRole('region', { name: /current stats/i })
    ).toBeVisible({ timeout: 10_000 })

    await authenticatedPage
      .getByRole('heading', { name: /latest radio shows/i })
      .scrollIntoViewIfNeeded()
    await expect(
      authenticatedPage.getByRole('heading', { name: /^the .+ scene graph$/i })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      authenticatedPage.getByRole('heading', { name: /latest radio shows/i })
    ).toBeVisible()
  })

  // PSY-2104. One test, not four: each step depends on the one before it, and
  // the layout it writes is stored on the worker user that every other
  // signed-in spec shares. It therefore ENDS at the shipped default, and the
  // reset is asserted rather than assumed.
  test('hides, reorders and resets home sections, and remembers across a reload', async ({
    authenticatedPage: page,
  }) => {
    await page.goto('/')

    // Scoped to the list: the page carries other lists (nav, cards), and an
    // unscoped listitem query would resolve against whichever came first.
    const sectionRows = page
      .getByRole('list', { name: 'Home sections' })
      .getByRole('listitem')
    const toolbar = page.getByRole('button', { name: 'Customize home' })
    await expect(toolbar).toBeVisible()
    await expect(page.getByText(/HOME · 5 SECTIONS · YOUR LAYOUT/i)).toBeVisible()

    const radioHeading = page.getByRole('heading', {
      name: /latest radio shows/i,
    })
    await expect(radioHeading).toBeVisible()

    await toolbar.click()
    const radioRow = page.getByRole('checkbox', { name: 'Latest radio shows' })
    await expect(radioRow).toBeChecked()

    // Hide it: the section goes, and the row says so.
    await radioRow.uncheck()
    await expect(radioRow).not.toBeChecked()
    await expect(radioHeading).toHaveCount(0)

    // Reorder: community stats one step up, which is a swap with the nearby
    // section rather than a re-sort.
    await page.getByRole('button', { name: 'Move Community stats up' }).click()
    await expect(sectionRows.nth(1)).toContainText(
      'Community stats'
    )

    // Persisted on the account, not in this tab.
    await page.reload()
    await expect(radioHeading).toHaveCount(0)
    await page.getByRole('button', { name: 'Customize home' }).click()
    await expect(
      page.getByRole('checkbox', { name: 'Latest radio shows' })
    ).not.toBeChecked()
    await expect(sectionRows.nth(1)).toContainText(
      'Community stats'
    )

    // Back to the shipped layout, so the rest of this worker's specs see the
    // page they were written against.
    await page.getByRole('button', { name: 'Reset to default' }).click()
    await expect(
      page.getByRole('checkbox', { name: 'Latest radio shows' })
    ).toBeChecked()
    await expect(sectionRows.nth(1)).toContainText(
      'Shows near you this week'
    )
    await page.keyboard.press('Escape')
    await expect(radioHeading).toBeVisible()

    await page.reload()
    await expect(radioHeading).toBeVisible()
  })
})
