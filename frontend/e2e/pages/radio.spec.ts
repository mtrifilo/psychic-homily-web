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
 * EXACTNESS NOTE (PSY-1957): Playwright's `name:` option matches by SUBSTRING
 * with a string argument, so a locator naming an entity also matches every
 * OTHER control whose accessible name merely contains that entity's name.
 * The dial's outbound `[listen]` bracket announces "Listen to {channel}
 * (opens in a new tab)" (PSY-1865 gave it the channel-naming `ariaLabel`;
 * BracketLink appends the new-tab half), which contains the channel name and
 * so collided with the channel's own strip link under strict mode.
 *
 * SCOPE OF THE RULE: it governs LINK locators that name an entity (a station,
 * channel, or show). It does NOT govern the heading locators below, which
 * deliberately match a section-title PREFIX by substring: `SectionHeader`
 * renders "Shows — active first, sorted by latest playlist", so
 * `heading { name: 'Shows' }` would match NOTHING with `exact: true`. Do not
 * sweep `exact: true` across this file mechanically.
 *
 * Two DIFFERENT tools, for two different collisions. Do not reach for one
 * expecting it to cover the other:
 *
 *  - `exact: true` defeats a LONGER name that CONTAINS the target's name.
 *    That is the outbound-bracket case above. It does nothing about a second
 *    element whose name is identical.
 *  - A landmark/region SCOPE defeats an IDENTICAL name elsewhere on the page.
 *    `exact` cannot help there. `/radio` has exactly that case: the program
 *    guide links stations by bare name too.
 *
 * Where the target's name legitimately carries decoration, match the WHOLE
 * decorated name exactly ("← KEXP") rather than falling back to a substring
 * plus `.first()`: a substring plus `.first()` can pass while asserting
 * nothing, which is worse than the collision it works around.
 *
 * Honest accounting of what is load-bearing TODAY versus hardening: on the
 * current seed the only live collision is the dial's channel row (the
 * `[listen]` bracket named after its channel). The region scopes and the
 * remaining `exact: true`s are hardening against collisions the code already
 * contains but the seed does not yet reach — see the guide note in the first
 * test. That is a good reason to keep them; it is not the same as "required".
 *
 * One behavior change to know about: `exact: true` is case-SENSITIVE, while
 * substring matching is not. The link literals carrying `exact: true` match
 * backend/internal/seeddata/radio.go byte for byte, so a casing edit to a
 * seeded station/channel/show name will now fail those assertions. That
 * failure is loud, which is the point. (The heading and landmark literals
 * below are not seed data and are unaffected.)
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

// Landmark names this spec scopes to. Hoisted like the seed literals above so
// a rename is one edit here rather than a hunt through the file. These are
// `aria-label`s on production components, so they are a real coupling: see the
// note beside `<section aria-label="The dial">` in RadioHub.tsx.
const DIAL_REGION = 'The dial'
const SHOWS_REGION = 'Shows'

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
    //
    // Scoped to the dial landmark because `RadioGuide` renders its own station
    // link whose accessible name is EXACTLY the station name
    // (`<Link>{row.station.name}</Link>`, app/radio/_components/RadioGuide.tsx)
    // inside the same <main>. `exact` cannot separate two identical names, so
    // the scope is the only tool that works on that one.
    //
    // That collision is latent, not live: the guide renders nothing on the E2E
    // seed because `gen-e2e-seed` never writes `radio_shows.schedule` while the
    // guide query filters `schedule IS NOT NULL`. Seed one schedule slot and
    // these four assertions go ambiguous without the scope.
    const dial = main.getByRole('region', { name: DIAL_REGION, exact: true })
    await expect(
      dial.getByRole('link', { name: KEXP_STATION_NAME, exact: true })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      dial.getByRole('link', { name: 'WFMU', exact: true })
    ).toBeVisible()
    await expect(
      dial.getByRole('link', { name: 'NTS Radio', exact: true })
    ).toBeVisible()

    // WFMU's channels surface as underlined sub-row links on the flagship
    // strip (seed has 3 wfmu sub-channels; assert one stable example).
    //
    // This is the one LIVE collision on the current seed, and the reason this
    // branch exists: the same row's outbound bracket announces "Listen to Give
    // the Drummer Radio (opens in a new tab)", which a substring match also
    // selects. `exact: true` is load-bearing here, not hardening.
    await expect(
      dial.getByRole('link', { name: 'Give the Drummer Radio', exact: true })
    ).toBeVisible()
  })

  test('clicking a station opens station detail and lists its shows', async ({
    page,
  }) => {
    await page.goto('/radio')

    // Click into KEXP (network-less → 1-segment /radio/kexp URL). Scoped and
    // exact per the note in the test above. This one is a CLICK, so ambiguity
    // is a hard failure rather than a failed assertion.
    const stationLink = page
      .getByRole('main')
      .getByRole('region', { name: DIAL_REGION, exact: true })
      .getByRole('link', { name: KEXP_STATION_NAME, exact: true })
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
    //
    // `exact: true` per the exactness note above (hardening, not a live
    // collision): the card title is a bare `<Link>{show.name}</Link>`, so a
    // seeded "The Morning Show Weekend Edition" would go ambiguous INSIDE the
    // region.
    await expect(
      main
        .getByRole('region', { name: SHOWS_REGION, exact: true })
        .getByRole('link', { name: KEXP_SHOW_NAME, exact: true })
    ).toBeVisible({ timeout: 10_000 })
  })

  test('clicking a show opens show detail with its episodes section', async ({
    page,
  }) => {
    // Start at the station so the click target is the real rendered show
    // card link (not a hand-built URL). PSY-1072: scoped to the shows
    // directory region — the on-air box + playlists feed (PSY-1050) also
    // link the show name, tripping strict mode un-scoped. For `exact: true`,
    // see the EXACTNESS NOTE above.
    await page.goto(`/radio/${KEXP_SLUG}`)

    const showLink = page
      .getByRole('main')
      .getByRole('region', { name: SHOWS_REGION, exact: true })
      .getByRole('link', { name: KEXP_SHOW_NAME, exact: true })
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
    // PSY-1957: this used to be a substring match on the bare station name
    // plus `.first()`, which was the one construct in this file that could
    // pass while asserting NOTHING. If the back-link were renamed or removed
    // and any earlier-in-DOM link in <main> carried "KEXP" in its name,
    // `.first()` would silently retarget and stay green. The show meta line
    // already prints `station_name` as plain text, so linking it is an
    // obvious next edit.
    //
    // The decoration here is a hard-coded literal, not variable data
    // (`← {show.station_name}` in RadioShowDetail.tsx), so the whole
    // accessible name is knowable and can be matched exactly. Naming it in
    // full is both unambiguous and self-describing: the assertion now fails
    // loudly if the back-link changes, instead of quietly matching something
    // else.
    await expect(
      main.getByRole('link', {
        name: `← ${KEXP_STATION_NAME}`,
        exact: true,
      })
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
    //
    // `exact: true` per the EXACTNESS NOTE, and this is the site where the
    // rule pays for itself most: "Radio" is a SUFFIX of three seeded station
    // names ("NTS Radio", "Give the Drummer Radio", "Rock'n'Soul Radio"), so a
    // substring match here is one seed or routing change from ambiguity, on a
    // locator that CLICKS. It survives today only because KEXP is
    // network-less, so NetworkTabBar renders no sibling channel links.
    // StationDetail renders the breadcrumb as a lucide <ArrowLeft/> plus the
    // text "Radio"; lucide stamps aria-hidden on icons with no a11y prop, so
    // the accessible name is exactly "Radio" (unlike the SHOW page's
    // back-link, whose arrow is a literal character and part of the name).
    const breadcrumb = page
      .getByRole('main')
      .getByRole('link', { name: 'Radio', exact: true })
    await expect(breadcrumb).toBeVisible({ timeout: 10_000 })
    await breadcrumb.click()

    await page.waitForURL(/\/radio(\?|$)/, { timeout: 10_000 })
    await expect(
      page.getByRole('main').getByRole('heading', { name: 'Radio', level: 1 })
    ).toBeVisible({ timeout: 10_000 })
  })
})
