import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

/**
 * PSY-722: end-to-end coverage for the public radio browse flow.
 *
 * Phase 2d shipped a full provider integration (KEXP / WFMU / NTS) with no
 * E2E signal. This spec walks the read-only navigation chain a visitor
 * follows from the radio hub down into a single show:
 *
 *   /radio  →  /radio/{station-slug}  →  /radio/{station-slug}/{show-slug}
 *
 * Selectors prefer `getByRole`/`getByText` over class selectors (PSY-859
 * anti-false-coverage guidance). Card titles are single outer `<Link>`s via
 * `EntityCardTitle`, so `getByRole('link', { name })` resolves cleanly under
 * Playwright strict mode.
 *
 * SCOPING NOTE: chrome outside `<main>` renders its own `Radio` links — the
 * top-bar primary nav links straight to /radio (PSY-1057) and the Footer
 * carries one too. To keep the page-content assertions unambiguous under
 * strict mode, link/heading queries are scoped to the page's `<main>`
 * (`page.getByRole('main')`), which excludes the TopBar and Footer.
 *
 * SEED SCOPE (verified against backend/internal/seeddata/radio.go, rendered
 * by cmd/gen-e2e-seed into frontend/e2e/setup-db.sh):
 *   - radio_networks: 1 (wfmu)
 *   - radio_stations: 6 (kexp, wfmu + 3 wfmu sub-channels, nts-radio)
 *   - radio_shows:   13 (6 KEXP, 4 WFMU, 3 NTS)
 *   - radio_episodes: 1 (the-morning-show, air date 2025-01-15)  [PSY-899]
 *   - radio_plays:    2 (Calexico "Crystal Frontier" matched +
 *                        Beach House "Space Song" unmatched)     [PSY-899]
 *
 * PSY-899 seeds one KEXP episode + two plays so the deep radio browse chain
 * is E2E-reachable. The show-detail test below therefore asserts the
 * populated "Recent Episodes" state (a row linking into the dated episode
 * route), not an empty state.
 *
 * Out of scope here (deliberate — see the PR scope note): the deeper chain
 * the seed now also enables — episode-detail navigation, tracklist render,
 * and the artist "As Heard On" → station cross-link — is left to a follow-up
 * so this spec stays focused on the radio → station → show browse path.
 *
 * Stable seeded slugs used below:
 *   - station "KEXP"             -> /radio/kexp        (network-less, 1-segment URL)
 *   - show    "The Morning Show" -> slug the-morning-show, host John Richards
 */

const KEXP_STATION_NAME = 'KEXP'
const KEXP_SLUG = 'kexp'
const KEXP_SHOW_NAME = 'The Morning Show'
const KEXP_SHOW_SLUG = 'the-morning-show'
// PSY-899 seeds exactly one episode for the-morning-show, keyed by air date.
// Episodes are addressed by air date, so this is also the deep-chain route.
const KEXP_EPISODE_AIR_DATE = '2025-01-15'

test.describe('Radio browse flow', () => {
  test('/radio loads and lists seeded stations', async ({ page }) => {
    await page.goto('/radio')
    const main = page.getByRole('main')

    // Page-level identity heading.
    await expect(
      main.getByRole('heading', { name: 'Radio', level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // PSY-1049: the /radio index is The Dial — every index-visible station
    // renders as a full-width strip whose underlined station name is a link
    // to the station page (no clicks needed to see the whole dial). KEXP /
    // WFMU / NTS are the three index-visible stations (the 3 WFMU
    // sub-channels are hidden by isStationVisibleOnIndex per PSY-673; they
    // appear as channel sub-rows under the WFMU strip instead).
    await expect(
      main.getByRole('link', { name: KEXP_STATION_NAME })
    ).toBeVisible({ timeout: 10_000 })
    await expect(main.getByRole('link', { name: 'WFMU' })).toBeVisible()
    await expect(main.getByRole('link', { name: 'NTS Radio' })).toBeVisible()

    // WFMU's channels surface as underlined sub-row links on the flagship
    // strip (seed has 3 wfmu sub-channels; assert one stable example).
    await expect(
      main.getByRole('link', { name: 'Give the Drummer Radio' })
    ).toBeVisible()
  })

  test('clicking a station opens station detail and lists its shows', async ({
    page,
  }) => {
    await page.goto('/radio')

    // Click into KEXP (network-less → 1-segment /radio/kexp URL).
    const stationLink = page
      .getByRole('main')
      .getByRole('link', { name: KEXP_STATION_NAME })
    await expect(stationLink).toBeVisible({ timeout: 10_000 })
    await stationLink.click()

    await page.waitForURL(new RegExp(`/radio/${KEXP_SLUG}(\\?|$)`), {
      timeout: 10_000,
    })

    const main = page.getByRole('main')

    // Station H1 carries the station name (network-less stations render the
    // station name as the page H1; networked stations render the network
    // name there instead — KEXP has no network so this holds).
    await expect(
      main.getByRole('heading', { name: KEXP_STATION_NAME, level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // "Shows" section heading is rendered unconditionally on station detail.
    await expect(main.getByRole('heading', { name: 'Shows' })).toBeVisible()

    // At least one seeded show card link is present (KEXP seeds 6 shows).
    // PSY-1072: scope to the shows directory's landmark — the PSY-1050
    // station-page rebuild also links the show name from the on-air box and
    // the playlists feed (client-fetched, so the un-scoped match count varies
    // 2–3 and trips strict mode). StationShowsDirectory renders
    // `<section aria-label="Shows">` → role=region.
    await expect(
      main
        .getByRole('region', { name: 'Shows' })
        .getByRole('link', { name: KEXP_SHOW_NAME })
    ).toBeVisible({ timeout: 10_000 })
  })

  test('clicking a show opens show detail with its episodes section', async ({
    page,
  }) => {
    // Start at the station so the click target is the real rendered show
    // card link (not a hand-built URL). PSY-1072: scoped to the shows
    // directory region — the on-air box + playlists feed (PSY-1050) also
    // link the show name, tripping strict mode un-scoped.
    await page.goto(`/radio/${KEXP_SLUG}`)

    const showLink = page
      .getByRole('main')
      .getByRole('region', { name: 'Shows' })
      .getByRole('link', { name: KEXP_SHOW_NAME })
    await expect(showLink).toBeVisible({ timeout: 10_000 })
    await showLink.click()

    await page.waitForURL(
      new RegExp(`/radio/${KEXP_SLUG}/${KEXP_SHOW_SLUG}(\\?|$)`),
      { timeout: 10_000 }
    )

    const main = page.getByRole('main')

    // Show H1 is the show name.
    await expect(
      main.getByRole('heading', { name: KEXP_SHOW_NAME, level: 1 })
    ).toBeVisible({ timeout: 10_000 })

    // PSY-1072: the PSY-1051 show-page rebuild replaced the Radio + station
    // breadcrumb chain with a single "← {station}" back-link (the hub stays
    // reachable via the top-bar Radio link, which lives outside <main>).
    // `name` matching is substring, so KEXP_STATION_NAME matches "← KEXP";
    // `.first()` guards against the station name also appearing in body copy.
    await expect(
      main.getByRole('link', { name: KEXP_STATION_NAME }).first()
    ).toBeVisible()

    // Episode archive renders. PSY-899 seeds one KEXP episode for this show
    // (air date 2025-01-15), so this asserts the populated path: the section
    // heading shows + the archive-table row links into the dated episode
    // route. PSY-1072: the PSY-1051 rebuild renamed the section from
    // "Recent Episodes" to "Playlists — N episode(s)" and renders it as the
    // archive table. The row link is CLIENT-fetched, so allow up to 10s.
    // Target the row by its href to the dated episode route
    // (`/radio/kexp/the-morning-show/2025-01-15`): that URL is the exact
    // deep-chain link the seed makes reachable and is immune to date-format /
    // play-count text variation.
    await expect(
      main.getByRole('heading', { name: /playlists/i })
    ).toBeVisible({ timeout: 10_000 })
    // `.first()`: the archive table links the dated route from several cells
    // (date, episode title, "Open latest playlist", playlist row) — the
    // assertion's intent is "the dated episode route is linked", not "exactly
    // once".
    await expect(
      main
        .locator(
          `a[href="/radio/${KEXP_SLUG}/${KEXP_SHOW_SLUG}/${KEXP_EPISODE_AIR_DATE}"]`
        )
        .first()
    ).toBeVisible({ timeout: 10_000 })
  })

  test('station detail breadcrumb returns to the radio hub', async ({
    page,
  }) => {
    await page.goto(`/radio/${KEXP_SLUG}`)

    // The in-page breadcrumb "Radio" link (scoped to <main> to avoid the
    // sidebar's own /radio nav link).
    const breadcrumb = page.getByRole('main').getByRole('link', { name: 'Radio' })
    await expect(breadcrumb).toBeVisible({ timeout: 10_000 })
    await breadcrumb.click()

    await page.waitForURL(/\/radio(\?|$)/, { timeout: 10_000 })
    await expect(
      page.getByRole('main').getByRole('heading', { name: 'Radio', level: 1 })
    ).toBeVisible({ timeout: 10_000 })
  })
})
