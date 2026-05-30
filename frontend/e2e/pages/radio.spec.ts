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
 * SCOPING NOTE: the persistent left Sidebar (components/layout/Sidebar.tsx)
 * renders a `<Link href="/radio">Radio</Link>` that is visible at the
 * Playwright desktop viewport (the `<aside>` is `hidden md:flex`). To keep
 * the page-content assertions unambiguous under strict mode, link/heading
 * queries are scoped to the page's `<main>` (`page.getByRole('main')`),
 * which excludes the sidebar `<aside>` and the TopBar.
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

    // Station cards render as links. KEXP / WFMU / NTS are the three
    // index-visible stations (the 3 WFMU sub-channels are hidden by
    // isStationVisibleOnIndex per PSY-673).
    await expect(
      main.getByRole('link', { name: KEXP_STATION_NAME })
    ).toBeVisible({ timeout: 10_000 })
    await expect(main.getByRole('link', { name: 'WFMU' })).toBeVisible()
    await expect(main.getByRole('link', { name: 'NTS Radio' })).toBeVisible()
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
    await expect(
      main.getByRole('link', { name: KEXP_SHOW_NAME })
    ).toBeVisible({ timeout: 10_000 })
  })

  test('clicking a show opens show detail with its episodes section', async ({
    page,
  }) => {
    // Start at the station so the click target is the real rendered show
    // card link (not a hand-built URL).
    await page.goto(`/radio/${KEXP_SLUG}`)

    const showLink = page
      .getByRole('main')
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

    // Breadcrumb links back up the chain (Radio + the station name). The
    // station name appears in both the breadcrumb and the "on {station}"
    // line, so allow more than one match via `.first()`.
    await expect(main.getByRole('link', { name: 'Radio' })).toBeVisible()
    await expect(
      main.getByRole('link', { name: KEXP_STATION_NAME }).first()
    ).toBeVisible()

    // "Recent Episodes" section renders. PSY-899 seeds one KEXP episode for
    // this show (air date 2025-01-15), so this asserts the populated path:
    // the section heading shows + the `RadioEpisodeRow` link into the dated
    // episode route resolves. The episode list is CLIENT-fetched
    // (RadioShowDetail.tsx useRadioEpisodes), so the row appears
    // asynchronously — allow up to 10s. Target the row by its href to the
    // dated episode route (`/radio/kexp/the-morning-show/2025-01-15`): that
    // URL is the exact deep-chain link the seed makes reachable and is immune
    // to date-format / play-count text variation. (The row's accessible name
    // includes the title "The Morning Show — …", so a name query would be a
    // weaker assertion that also reads as colliding with the show H1.)
    await expect(
      main.getByRole('heading', { name: /recent episodes/i })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      main.locator(
        `a[href="/radio/${KEXP_SLUG}/${KEXP_SHOW_SLUG}/${KEXP_EPISODE_AIR_DATE}"]`
      )
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
