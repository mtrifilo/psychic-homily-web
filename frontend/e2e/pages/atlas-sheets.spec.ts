import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'
import {
  ATLAS_TEST_TIMEOUT_MS,
  type AtlasMapSeam,
  creditUncovered,
  dismissBanner,
  jumpTo,
  jumpToPhoenix,
  mapPoint,
  PHOENIX,
  PHONE_CONTEXT,
  STREET_ROOM,
  stubAtlas,
  waitForMap,
} from '../helpers/atlas'

/**
 * The Atlas sheet layout under touch: panes narrower than the rail's 900px
 * get the venue list and every panel as bottom sheets over a full-width map.
 *
 * The full journey runs on an 820px tablet pane; the phone panes at the end
 * step through the detents and the sheet chain at phone sizes.
 * Scenes, venues and the venue's shows are synthesized so the stacked point
 * and the bill are known regardless of what the seed geocoded. SwiftShader is
 * required: MapLibre needs a WebGL2 context headless Chromium otherwise lacks.
 */
test.use({
  launchOptions: {
    args: ['--enable-unsafe-swiftshader', '--use-angle=swiftshader'],
  },
  viewport: { width: 820, height: 1000 },
  hasTouch: true,
})

test.describe.configure({ timeout: ATLAS_TEST_TIMEOUT_MS })

/**
 * A one-finger drag through CDP touch events, which (unlike Playwright's
 * tap) carry the browser's implicit pointer capture: the press lands on the
 * grabber and the handle takes the capture once the drag starts.
 */
async function touchDrag(page: Page, x: number, fromY: number, toY: number) {
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x, y: fromY, id: 1 }],
  })
  const steps = 12
  for (let i = 1; i <= steps; i++) {
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x, y: fromY + ((toY - fromY) * i) / steps, id: 1 }],
    })
  }
  // A pause before release so the drag settles by position, not as a fling.
  await page.waitForTimeout(150)
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
}

type Detent = 'peek' | 'half' | 'full'

// The detent rule (bottomSheetHeightPx and its constants in
// components/ui/bottom-sheet.tsx) with the Atlas's top inset
// (ATLAS_SHEET_TOP_INSET_PX in features/scenes/cityView.ts): every detent is
// capped at the host minus the inset; Peek and Full sit at their DS heights;
// Half is a share of the host up to its DS ceiling, or Full's height when the
// share is less than a minimum step above Peek.
const PEEK_PX = 120
const HALF_MAX_PX = 400
const HALF_HOST_PERCENT = 45
const HALF_MIN_STEP_PX = 48
const FULL_PX = 660
const ATLAS_SHEET_TOP_INSET_PX = 112
function detentPx(detent: Detent, hostPx: number) {
  const cap = Math.max(0, hostPx - ATLAS_SHEET_TOP_INSET_PX)
  if (detent === 'peek') return Math.min(PEEK_PX, cap)
  if (detent === 'full') return Math.min(FULL_PX, cap)
  const sharePx = (hostPx * HALF_HOST_PERCENT) / 100
  if (sharePx < PEEK_PX + HALF_MIN_STEP_PX) return Math.min(FULL_PX, cap)
  return Math.min(HALF_MAX_PX, sharePx, cap)
}

/** How far a drag from Peek travels to release exactly at Half's height. */
async function peekToHalfPx(page: Page, testId: string) {
  const host = await hostHeight(page, testId)
  return detentPx('half', host) - detentPx('peek', host)
}

/** The sheet's host (its parent, the map pane) height in CSS px. */
function hostHeight(page: Page, testId: string) {
  return page.getByTestId(testId).evaluate((el) => el.parentElement!.clientHeight)
}

/**
 * Waits until the sheet has settled at `detent` and its height matches the
 * rule for its host, to the layout's subpixel rounding. Returns the measured
 * host and sheet heights.
 */
async function expectDetentHeight(page: Page, testId: string, detent: Detent) {
  const sheet = page.getByTestId(testId)
  await expect(sheet).toHaveAttribute('data-detent', detent)
  await expect(sheet).not.toHaveAttribute('data-dragging', 'true')
  const host = await hostHeight(page, testId)
  const expected = detentPx(detent, host)
  let height = 0
  await expect
    .poll(async () => {
      height = await sheet.evaluate((el) => el.getBoundingClientRect().height)
      return height
    })
    .toBeCloseTo(expected, 0)
  return { host, height }
}

/**
 * Where `at` falls in the viewport, through the map's own projection, once it
 * has stopped moving on screen (the map resizes when the banner leaves).
 */
async function settledMapPoint(page: Page, at: { lat: number; lng: number }) {
  const rounded = async () => {
    const p = await mapPoint(page, at)
    return { x: Math.round(p.x), y: Math.round(p.y) }
  }
  let point = await rounded()
  await expect
    .poll(async () => {
      const previous = point
      await page.waitForTimeout(250)
      point = await rounded()
      return point.x === previous.x && point.y === previous.y
    })
    .toBe(true)
  return point
}

/** A tap on the Phoenix scene dot once it has stopped moving on screen. */
async function tapSceneDot(page: Page) {
  const point = await settledMapPoint(page, PHOENIX)
  await page.touchscreen.tap(point.x, point.y)
}

/**
 * Counts, from now on, every time the element with `testId` is shown (its
 * inline display set to anything but none), so a check can say a tap never
 * showed it, even for a frame. Returns a reader for the count.
 */
async function countShows(page: Page, testId: string) {
  type ShowsWindow = { __shows?: Record<string, number> }
  await page.getByTestId(testId).evaluate((el) => {
    const w = window as unknown as ShowsWindow
    const id = el.getAttribute('data-testid')!
    w.__shows = { ...w.__shows, [id]: 0 }
    new MutationObserver(() => {
      if ((el as HTMLElement).style.display !== 'none') w.__shows![id] += 1
    }).observe(el, { attributes: true, attributeFilter: ['style'] })
  })
  return () =>
    page.evaluate((id) => (window as unknown as ShowsWindow).__shows?.[id] ?? 0, testId)
}

/** Whether the primary pointer is coarse, as the page's media queries see it. */
function pointerIsCoarse(page: Page) {
  return page.evaluate(() => window.matchMedia('(pointer: coarse)').matches)
}

/**
 * Frames the stacked centroid pin and the Street Room pin together at street
 * zoom, and waits until both are drawn.
 */
async function frameBothPins(page: Page) {
  const midpoint = {
    lat: (PHOENIX.lat + STREET_ROOM.lat) / 2,
    lng: (PHOENIX.lng + STREET_ROOM.lng) / 2,
  }
  await jumpTo(page, midpoint, 12)
  await waitForMap(page)
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as unknown as { __atlasMap: AtlasMapSeam }).__atlasMap.queryRenderedFeatures(
            { layers: ['venue-pins'] },
          ).length,
      ),
    )
    .toBeGreaterThanOrEqual(2)
}

/** A touch drag up from the sheet's grabber at Peek to Half's height. */
async function pullToHalf(page: Page, testId: string) {
  const distance = await peekToHalfPx(page, testId)
  const grabber = page.getByTestId(testId).locator('[data-bottom-sheet-grabber]')
  const box = (await grabber.boundingBox())!
  const y = box.y + box.height / 2
  await touchDrag(page, box.x + box.width / 2, y, y - distance)
}

test.describe('Atlas sheet layout under touch', () => {
  test('stacked pin, list, venue and artist sheets, back to globe', async ({ page }) => {
    await stubAtlas(page)
    await page.goto('/atlas?city=Phoenix%2CAZ')
    await waitForMap(page)

    const list = page.getByTestId('atlas-venue-sheet')
    await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
    await expect(page.getByTestId('atlas-venue-rail')).toHaveCount(0)
    // The credit sits top-left on this layout, and nothing covers it.
    await expect(
      page.locator('.maplibregl-ctrl-top-left .maplibregl-ctrl-attrib'),
    ).toBeVisible()
    expect(await creditUncovered(page)).toBe(true)

    // A drag that starts on the grabber moves the sheet, and settles it.
    const grabber = list.locator('[data-bottom-sheet-grabber]')
    const atPeek = (await grabber.boundingBox())!
    const x = atPeek.x + atPeek.width / 2
    const toHalf = await peekToHalfPx(page, 'atlas-venue-sheet')
    await touchDrag(page, x, atPeek.y + atPeek.height / 2, atPeek.y + atPeek.height / 2 - toHalf)
    // Wait out the settle animation, then drag down from where the grabber
    // now is.
    await expectDetentHeight(page, 'atlas-venue-sheet', 'half')
    const atHalf = (await grabber.boundingBox())!
    await touchDrag(page, x, atHalf.y + atHalf.height / 2, atHalf.y + 330)
    await expect(list).toHaveAttribute('data-detent', 'peek')

    // A mouse drag works too, although every move after the press leaves the
    // handle (a mouse has no implicit capture).
    await expect
      .poll(() => list.evaluate((el) => Math.round(el.getBoundingClientRect().height)))
      .toBe(120)
    const forMouse = (await grabber.boundingBox())!
    await page.mouse.move(x, forMouse.y + forMouse.height / 2)
    await page.mouse.down()
    for (let i = 1; i <= 12; i++) {
      await page.mouse.move(x, forMouse.y + forMouse.height / 2 - (toHalf * i) / 12)
    }
    await page.waitForTimeout(150)
    await page.mouse.up()
    await expect(list).toHaveAttribute('data-detent', 'half')

    // A tap on the counted marker opens the list at Peek, scoped to the
    // point, and Peek names the scope; the rows are one step up, at Half.
    await page.getByTestId('atlas-venue-stack').tap()
    await expect(list).toHaveAttribute('data-detent', 'peek')
    await expect(page.getByTestId('venue-sheet-peek-line')).toHaveText(
      '2 venues at the city centre point',
    )
    await grabber.tap()
    await expect(list).toHaveAttribute('data-detent', 'half')
    await expect(page.getByTestId('venue-sheet-scope-line')).toHaveText(
      '2 venues at the city centre point',
    )
    await expect(list.getByRole('button', { name: /Street Room/ })).toHaveCount(0)
    await list.getByRole('button', { name: 'Show all' }).tap()
    await expect(list.getByRole('button', { name: /Street Room/ })).toBeVisible()

    // Row -> venue sheet at Half; the list steps aside.
    await list.getByRole('button', { name: /Street Room/ }).tap()
    const venueSheet = page.getByTestId('atlas-venue-panel')
    await expect(venueSheet).toHaveAttribute('data-detent', 'half')
    await expect(list).toBeHidden()
    expect(await creditUncovered(page)).toBe(true)

    // Show -> artist sheet at Full.
    await venueSheet.getByRole('button', { name: /Sheet Night/ }).tap()
    const artistSheet = page.getByTestId('atlas-artist-panel')
    await expect(artistSheet).toHaveAttribute('data-detent', 'full')
    await expect(artistSheet.getByRole('heading', { name: 'Sheet Band' })).toBeVisible()
    expect(await creditUncovered(page)).toBe(true)

    // Escape pops one level at a time, back to the list.
    await page.keyboard.press('Escape')
    await expect(venueSheet).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(list).toBeVisible()

    // Back to the globe from the status row.
    await page.getByRole('button', { name: 'Back to globe' }).tap()
    await expect(list).toHaveCount(0, { timeout: 15_000 })
  })

  test('the scene search opens below the top-left credit', async ({ page }) => {
    await stubAtlas(page)
    await page.goto('/atlas')
    await waitForMap(page)
    // Below city view but past the street basemap's first zoom, so the
    // OpenStreetMap credit is showing while the globe's search is too.
    await jumpToPhoenix(page, 8)
    await expect(page.locator('.maplibregl-ctrl-attrib-inner')).toContainText(
      'OpenStreetMap',
      { timeout: 30_000 },
    )
    await page.getByRole('combobox', { name: 'Search scenes' }).tap()
    await expect(page.getByPlaceholder('City or state…')).toBeVisible()
    expect(await creditUncovered(page)).toBe(true)
  })
})

// Viewport heights of a landscape phone (where Half takes Full's height), a
// short phone and a tall phone, at a width where the map renders in the sheet
// layout. The rule is checked against each host as measured, and the host and
// map area are recorded as annotations.
for (const { mergedHalf, ...viewport } of [
  { width: 820, height: 480, mergedHalf: true },
  { width: 820, height: 664, mergedHalf: false },
  { width: 820, height: 844, mergedHalf: false },
]) {
  test.describe(`Atlas sheet detents at ${viewport.width}x${viewport.height}`, () => {
    test.use({ viewport })

    test('a stacked pin opens the list at Peek; Half is one pull up and sized by the host', async ({
      page,
    }) => {
      await stubAtlas(page)
      await page.goto('/atlas?city=Phoenix%2CAZ')
      await waitForMap(page)
      // The banner shortens the host while it is up.
      await dismissBanner(page)

      const list = page.getByTestId('atlas-venue-sheet')
      await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
      await page.getByTestId('atlas-venue-stack').tap()
      const peek = await expectDetentHeight(page, 'atlas-venue-sheet', 'peek')
      await expect(page.getByTestId('venue-sheet-peek-line')).toHaveText(
        '2 venues at the city centre point',
      )

      await pullToHalf(page, 'atlas-venue-sheet')
      const half = await expectDetentHeight(page, 'atlas-venue-sheet', 'half')
      expect(
        detentPx('half', half.host) === detentPx('full', half.host),
        `Half ${mergedHalf ? 'takes' : 'stays under'} Full's height on this host`,
      ).toBe(mergedHalf)
      await expect(page.getByTestId('venue-sheet-scope-line')).toBeVisible()
      expect(await creditUncovered(page)).toBe(true)
      test.info().annotations.push({
        type: 'venue list',
        description: JSON.stringify({
          host: half.host,
          peek: peek.height,
          half: half.height,
          full: detentPx('full', half.host),
          mapAreaAtPeek: half.host - peek.height,
          mapAreaAtHalf: half.host - half.height,
        }),
      })

      // A venue sheet opens at Half by the same rule.
      await list.getByRole('button', { name: /Centroid Room One/ }).tap()
      await expectDetentHeight(page, 'atlas-venue-panel', 'half')
      expect(await creditUncovered(page)).toBe(true)
    })

    test('a scene dot opens its preview at Half, sized by the host', async ({ page }) => {
      await stubAtlas(page)
      await page.goto('/atlas')
      await waitForMap(page)
      await dismissBanner(page)

      // A tap can land on a frame where the dot layer has not yet redrawn
      // after the resize; a missed tap on the globe does nothing, so it is
      // repeated until the preview opens. Tapping an open scene's dot again
      // keeps the preview as it is.
      await expect(async () => {
        await tapSceneDot(page)
        await expect(page.getByTestId('atlas-scene-preview-sheet')).toBeVisible({
          timeout: 2_000,
        })
      }).toPass({ timeout: 30_000 })
      const half = await expectDetentHeight(page, 'atlas-scene-preview-sheet', 'half')
      test.info().annotations.push({
        type: 'scene preview',
        description: JSON.stringify({
          host: half.host,
          half: half.height,
          mapAreaAtHalf: half.host - half.height,
        }),
      })
    })
  })
}

for (const viewport of [
  { width: 390, height: 844 },
  { width: 360, height: 780 },
]) {
  test.describe(`Atlas sheet detents on a ${viewport.width}x${viewport.height} phone`, () => {
    test.use({ viewport, isMobile: true, deviceScaleFactor: 2 })

    test('Peek, Half and a capped Full, through the list, venue and artist sheets', async ({
      page,
    }) => {
      await stubAtlas(page)
      await page.goto('/atlas?city=Phoenix%2CAZ')
      await waitForMap(page)
      await expect(page.getByTestId('atlas-scene-list')).toHaveCount(0)
      // The banner shortens the host while it is up; dismissed, the host is
      // the phone's whole map area.
      await dismissBanner(page)

      const list = page.getByTestId('atlas-venue-sheet')
      await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
      const { host } = await expectDetentHeight(page, 'atlas-venue-sheet', 'peek')
      // On a phone the host caps Full below its DS height, and Half stays a
      // separate step under it.
      expect(detentPx('full', host), 'Full is capped by the host').toBeLessThan(FULL_PX)
      expect(detentPx('half', host), 'Half stays under Full').toBeLessThan(
        detentPx('full', host),
      )
      expect(await creditUncovered(page)).toBe(true)

      // The grabber steps Peek to Half, then to Full, which stops below the
      // credit. (Drags to Half are walked on the 820px panes above.)
      const grabber = list.locator('[data-bottom-sheet-grabber]')
      await grabber.tap()
      await expectDetentHeight(page, 'atlas-venue-sheet', 'half')
      await grabber.tap()
      await expectDetentHeight(page, 'atlas-venue-sheet', 'full')
      expect(await creditUncovered(page)).toBe(true)

      // From the tallest detent the grabber returns to Peek, which leaves the
      // stacked marker at the map's centre in view. The marker opens the list
      // at Peek, scoped; the rows are one step up.
      await grabber.tap()
      await expectDetentHeight(page, 'atlas-venue-sheet', 'peek')
      await page.getByTestId('atlas-venue-stack').tap()
      await expect(page.getByTestId('venue-sheet-peek-line')).toHaveText(
        '2 venues at the city centre point',
      )
      await grabber.tap()
      await expectDetentHeight(page, 'atlas-venue-sheet', 'half')
      await list.getByRole('button', { name: 'Show all' }).tap()

      // Row -> venue sheet at Half, then Full.
      await list.getByRole('button', { name: /Street Room/ }).tap()
      const venueSheet = page.getByTestId('atlas-venue-panel')
      await expectDetentHeight(page, 'atlas-venue-panel', 'half')
      await venueSheet.locator('[data-bottom-sheet-grabber]').tap()
      await expectDetentHeight(page, 'atlas-venue-panel', 'full')
      expect(await creditUncovered(page)).toBe(true)

      // Show -> artist sheet, which opens at Full.
      await venueSheet.getByRole('button', { name: /Sheet Night/ }).tap()
      const artistSheet = page.getByTestId('atlas-artist-panel')
      await expect(artistSheet.getByRole('heading', { name: 'Sheet Band' })).toBeVisible()
      await expectDetentHeight(page, 'atlas-artist-panel', 'full')
      expect(await creditUncovered(page)).toBe(true)

      await page.keyboard.press('Escape')
      await expect(venueSheet).toBeVisible()
      await page.keyboard.press('Escape')
      await expect(list).toBeVisible()
      await page.getByRole('button', { name: 'Back to globe' }).tap()
      await expect(list).toHaveCount(0, { timeout: 15_000 })
    })
  })
}

test.describe('Atlas pin and dot taps on a 390x844 phone', () => {
  test.use({ viewport: { width: 390, height: 844 }, ...PHONE_CONTEXT })

  test('a tap on a lone pin opens its venue and a tap on a stacked pin opens the list, with no tooltip', async ({
    page,
  }) => {
    await stubAtlas(page)
    await page.goto('/atlas?city=Phoenix%2CAZ')
    await waitForMap(page)
    await dismissBanner(page)
    expect(await pointerIsCoarse(page), 'the phone context has a coarse pointer').toBe(true)

    const list = page.getByTestId('atlas-venue-sheet')
    await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
    await frameBothPins(page)
    const tooltip = page.getByTestId('atlas-venue-tooltip')
    const tooltipShows = await countShows(page, 'atlas-venue-tooltip')

    // One tap on the lone pin opens its venue sheet.
    const street = await settledMapPoint(page, STREET_ROOM)
    await page.touchscreen.tap(street.x, street.y)
    const venueSheet = page.getByTestId('atlas-venue-panel')
    await expect(venueSheet).toBeVisible()
    await expect(venueSheet).toContainText('Street Room')
    await expect(tooltip).toBeHidden()
    expect(await tooltipShows(), 'the tooltip never showed for the lone pin').toBe(0)

    // Back to the list, then one tap on the stacked pin's point opens the
    // list at Peek, scoped to the venues that share it.
    await page.keyboard.press('Escape')
    await expect(list).toBeVisible()
    const stacked = await settledMapPoint(page, PHOENIX)
    await page.touchscreen.tap(stacked.x, stacked.y)
    await expect(list).toHaveAttribute('data-detent', 'peek')
    await expect(page.getByTestId('venue-sheet-peek-line')).toHaveText(
      '2 venues at the city centre point',
    )
    await expect(tooltip).toBeHidden()
    expect(await tooltipShows(), 'the tooltip never showed for the stacked pin').toBe(0)
  })

  test('a tap on a scene dot opens its preview with no tooltip', async ({ page }) => {
    await stubAtlas(page)
    await page.goto('/atlas')
    await waitForMap(page)
    await dismissBanner(page)
    const tooltipShows = await countShows(page, 'atlas-scene-tooltip')

    // Repeated for the reason the scene preview detent test gives.
    await expect(async () => {
      await tapSceneDot(page)
      await expect(page.getByTestId('atlas-scene-preview-sheet')).toBeVisible({
        timeout: 2_000,
      })
    }).toPass({ timeout: 30_000 })
    await expect(page.getByTestId('atlas-scene-tooltip')).toBeHidden()
    expect(await tooltipShows(), 'the scene tooltip never showed').toBe(0)
  })
})

test.describe('Atlas pin hover under a mouse at 820x844', () => {
  test.use({ viewport: { width: 820, height: 844 }, hasTouch: false })

  test('a stacked pin is named by its stack and a lone pin by its venue', async ({ page }) => {
    await stubAtlas(page)
    await page.goto('/atlas?city=Phoenix%2CAZ')
    await waitForMap(page)
    await dismissBanner(page, { via: 'click' })
    expect(await pointerIsCoarse(page), 'the mouse context has a fine pointer').toBe(false)

    const list = page.getByTestId('atlas-venue-sheet')
    await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
    await frameBothPins(page)
    const tooltip = page.getByTestId('atlas-venue-tooltip')

    const stacked = await settledMapPoint(page, PHOENIX)
    await page.mouse.move(stacked.x, stacked.y)
    await expect(tooltip).toBeVisible()
    await expect(tooltip).toContainText('2 venues at the city centre point')
    await expect(tooltip).toContainText('click for shows')
    await expect(tooltip).not.toContainText('Centroid Room')
    await expect(tooltip).not.toContainText('upcoming')

    // A mouse move leaves the camera where it is, so the settled frame holds.
    const street = await mapPoint(page, STREET_ROOM)
    await page.mouse.move(street.x, street.y)
    await expect(tooltip).toContainText('Street Room')
    await expect(tooltip).toContainText('3 upcoming')
    await expect(tooltip).toContainText('click for shows')

    // A click on the lone pin still opens its venue.
    await page.mouse.click(street.x, street.y)
    await expect(page.getByTestId('atlas-venue-panel')).toContainText('Street Room')
  })
})
