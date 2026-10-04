import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'
import { creditUncovered, jumpToPhoenix, stubAtlas, waitForMap } from '../helpers/atlas'

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
    await touchDrag(page, x, atPeek.y + atPeek.height / 2, atPeek.y - 330)
    await expect(list).toHaveAttribute('data-detent', 'half')
    await expect(list).not.toHaveAttribute('data-dragging', 'true')
    // Wait out the settle animation, then drag down from where the grabber
    // now is.
    await expect
      .poll(() => list.evaluate((el) => Math.round(el.getBoundingClientRect().height)))
      .toBe(400)
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
      await page.mouse.move(x, forMouse.y + forMouse.height / 2 - (330 * i) / 12)
    }
    await page.waitForTimeout(150)
    await page.mouse.up()
    await expect(list).toHaveAttribute('data-detent', 'half')

    // A tap on the counted marker opens the list at Half, scoped to the point.
    await page.getByTestId('atlas-venue-stack').tap()
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
