import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'
import {
  creditUncovered,
  dismissBanner,
  jumpToPhoenix,
  phoenixDotPoint,
  stubAtlas,
  waitForMap,
} from '../helpers/atlas'

/**
 * The Atlas sheet layout under touch: panes narrower than the rail's 900px
 * get the venue list and every panel as bottom sheets over a full-width map.
 *
 * 820px wide, not a phone width: the map only renders above the 640px mobile
 * gate, and this layout is what every pane between that gate and 900px gets.
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
 * A tap on the Phoenix scene dot, through the map's own projection, once the
 * dot has stopped moving on screen (the map resizes when the banner leaves).
 */
async function tapSceneDot(page: Page) {
  const dotPoint = async () => {
    const p = await phoenixDotPoint(page)
    return { x: Math.round(p.x), y: Math.round(p.y) }
  }
  let point = await dotPoint()
  await expect
    .poll(async () => {
      const previous = point
      await page.waitForTimeout(250)
      point = await dotPoint()
      return point.x === previous.x && point.y === previous.y
    })
    .toBe(true)
  await page.touchscreen.tap(point.x, point.y)
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
  // Each test boots a SwiftShader map and walks several animated steps; the
  // first test's back-to-globe flight renders the whole globe again.
  test.setTimeout(120_000)

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
for (const viewport of [
  { width: 820, height: 480 },
  { width: 820, height: 664 },
  { width: 820, height: 844 },
]) {
  test.describe(`Atlas sheet detents at ${viewport.width}x${viewport.height}`, () => {
    test.use({ viewport })
    test.setTimeout(120_000)

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

    test('a scene dot opens its preview at Peek; Half is one pull up', async ({ page }) => {
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
      const preview = await expectDetentHeight(page, 'atlas-scene-preview-sheet', 'peek')
      await pullToHalf(page, 'atlas-scene-preview-sheet')
      const half = await expectDetentHeight(page, 'atlas-scene-preview-sheet', 'half')
      test.info().annotations.push({
        type: 'scene preview',
        description: JSON.stringify({
          host: preview.host,
          peek: preview.height,
          half: half.height,
          mapAreaAtPeek: preview.host - preview.height,
          mapAreaAtHalf: half.host - half.height,
        }),
      })
    })
  })
}
