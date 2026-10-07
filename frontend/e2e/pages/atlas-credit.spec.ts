import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'
import {
  creditReport,
  dismissBanner,
  jumpToPhoenix,
  phoenixDotPoint,
  stubAtlas,
  waitForMap,
  type AtlasMapSeam,
} from '../helpers/atlas'

/**
 * The map credit (the OpenStreetMap attribution the ODbL requires) across
 * Atlas states and layouts, on a first visit with the cookie banner up and,
 * where a test says so, after it is dismissed. Where a credit is due, its
 * whole text is on screen and every sample point across it hits the credit
 * itself. At globe zoom the compact globe (below `lg`) draws no OpenStreetMap
 * data, so there the check is that nothing OSM-derived renders and the
 * control lists nothing.
 *
 * SwiftShader is required: MapLibre needs a WebGL2 context headless Chromium
 * otherwise lacks.
 */
test.use({
  launchOptions: {
    args: ['--enable-unsafe-swiftshader', '--use-angle=swiftshader'],
  },
})

const PHONE_VIEWPORTS = [
  { width: 390, height: 844 },
  { width: 390, height: 664 },
  { width: 360, height: 780 },
] as const

const OSM_CREDIT = ['OpenFreeMap', 'OpenStreetMap contributors']
const NASA_CREDIT = ['NASA GIBS']

async function expectCreditVisible(
  page: Page,
  state: string,
  credits: string[] = OSM_CREDIT,
) {
  // MapLibre lists a source's credit once that source's tiles load, which can
  // trail the layout reaching this state (a sheet at its detent, the rail).
  for (const credit of credits) {
    await expect(page.locator('.maplibregl-ctrl-attrib-inner')).toContainText(credit, {
      timeout: 30_000,
    })
  }
  const report = await creditReport(page)
  test.info().annotations.push({ type: state, description: JSON.stringify(report) })
  expect(report, `${state}: credit present`).not.toBeNull()
  expect(report!.empty, `${state}: credit lists a source`).toBe(false)
  for (const credit of credits) {
    expect(report!.text, `${state}: credit complete`).toContain(credit)
  }
  expect(report!.clipped, `${state}: credit text clipped`).toBe(false)
  expect(report!.covered, `${state}: points that miss the credit`).toEqual([])
  expect(report!.underSheet, `${state}: sheets reaching the credit`).toEqual([])
}

/**
 * At globe zoom the compact globe draws ocean, Natural Earth land and PH's own
 * scene dots; no layer reading the OpenStreetMap-derived vector source
 * renders, so MapLibre lists no credit and hides its control.
 */
async function expectNoCreditDue(page: Page, state: string) {
  const facts = await page.evaluate(() => {
    const m = (window as unknown as { __atlasMap: AtlasMapSeam }).__atlasMap
    const osmLayers = m
      .getStyle()
      .layers.filter((l) => l.source === 'openmaptiles')
      .map((l) => l.id)
    return {
      zoom: m.getZoom(),
      osmLayerCount: osmLayers.length,
      osmFeatures: m.queryRenderedFeatures({ layers: osmLayers }).length,
      empty:
        document
          .querySelector('.maplibregl-ctrl-attrib')
          ?.classList.contains('maplibregl-attrib-empty') ?? null,
    }
  })
  test.info().annotations.push({ type: state, description: JSON.stringify(facts) })
  expect(facts.zoom, `${state}: at globe zoom`).toBeLessThan(5)
  expect(facts.osmLayerCount, `${state}: the style has OSM layers`).toBeGreaterThan(0)
  expect(facts.osmFeatures, `${state}: OSM-derived features rendered`).toBe(0)
  expect(facts.empty, `${state}: credit control lists nothing`).toBe(true)
}

/**
 * Past the street basemap's first zoom but short of city view, so the
 * OpenStreetMap credit is due while the globe's chrome is still up.
 */
async function jumpToStreetBasemap(page: Page) {
  await jumpToPhoenix(page, 8)
  await expect(page.locator('.maplibregl-ctrl-attrib-inner')).toContainText(
    'OpenStreetMap',
    { timeout: 30_000 },
  )
  // Tiles settled, so a later camera move cancels no request in flight.
  await waitForMap(page)
}

/** A tap or click on the Phoenix scene dot, through the map's own projection. */
async function pressSceneDot(page: Page, how: 'tap' | 'click') {
  const point = await phoenixDotPoint(page)
  if (how === 'tap') await page.touchscreen.tap(point.x, point.y)
  else await page.mouse.click(point.x, point.y)
}

async function stepDetent(page: Page, sheetTestId: string, to: string) {
  const sheet = page.getByTestId(sheetTestId)
  await sheet.locator('[data-bottom-sheet-grabber]').tap()
  await expect(sheet).toHaveAttribute('data-detent', to)
  // Wait out the settle animation before measuring. The detent attribute is
  // already committed, and getAnimations() flushes style first, so the height
  // transition it starts is listed until it finishes.
  await expect
    .poll(() => sheet.evaluate((el) => el.getAnimations().length))
    .toBe(0)
}

for (const viewport of PHONE_VIEWPORTS) {
  for (const colorScheme of ['dark', 'light'] as const) {
    test.describe(`Atlas credit at ${viewport.width}x${viewport.height} ${colorScheme}`, () => {
      test.use({
        viewport,
        colorScheme,
        hasTouch: true,
        isMobile: true,
        deviceScaleFactor: 2,
      })
      test.setTimeout(120_000)

      test('first visit, city view, venue and artist sheets', async ({ page }) => {
        await stubAtlas(page)
        await page.goto('/atlas?city=Phoenix%2CAZ')
        await waitForMap(page)
        await expect(page.getByRole('dialog', { name: 'Cookie consent' })).toBeVisible()

        const list = page.getByTestId('atlas-venue-sheet')
        await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
        await expectCreditVisible(page, 'first visit, list at peek, banner up')
        await stepDetent(page, 'atlas-venue-sheet', 'half')
        await expectCreditVisible(page, 'list at half, banner up')

        await dismissBanner(page)
        await expectCreditVisible(page, 'list at half')
        await stepDetent(page, 'atlas-venue-sheet', 'full')
        await expectCreditVisible(page, 'list at full')

        await list.getByRole('button', { name: /Street Room/ }).tap()
        const venueSheet = page.getByTestId('atlas-venue-panel')
        await expect(venueSheet).toHaveAttribute('data-detent', 'half')
        await expectCreditVisible(page, 'venue sheet at half')
        await stepDetent(page, 'atlas-venue-panel', 'full')
        await expectCreditVisible(page, 'venue sheet at full')

        await venueSheet.getByRole('button', { name: /Sheet Night/ }).tap()
        const artistSheet = page.getByTestId('atlas-artist-panel')
        await expect(artistSheet).toHaveAttribute('data-detent', 'full')
        await expect(artistSheet.getByRole('heading', { name: 'Sheet Band' })).toBeVisible()
        await expectCreditVisible(page, 'artist sheet at full')
      })

      test('globe entry and scene preview', async ({ page }) => {
        await stubAtlas(page)
        await page.goto('/atlas')
        await waitForMap(page)
        await expect(page.getByRole('dialog', { name: 'Cookie consent' })).toBeVisible()
        await expectNoCreditDue(page, 'globe entry, banner up')

        await pressSceneDot(page, 'tap')
        const preview = page.getByTestId('atlas-scene-preview-sheet')
        await expect(preview).toHaveAttribute('data-detent', 'half')
        await expectNoCreditDue(page, 'scene preview at globe zoom')

        await jumpToStreetBasemap(page)
        await expect(preview).toBeVisible()
        await expectCreditVisible(page, 'scene preview at half, z8, banner up')
        await dismissBanner(page)
        await stepDetent(page, 'atlas-scene-preview-sheet', 'full')
        await expectCreditVisible(page, 'scene preview at full, z8')
      })
    })
  }
}

test.describe('Atlas credit on a compact tablet pane', () => {
  test.use({ viewport: { width: 820, height: 1000 }, hasTouch: true })
  test.setTimeout(120_000)

  test('globe entry owes no credit; city view keeps the list sheet above the banner', async ({
    page,
  }) => {
    await stubAtlas(page)
    await page.goto('/atlas')
    await waitForMap(page)
    const banner = page.getByRole('dialog', { name: 'Cookie consent' })
    await expect(banner).toBeVisible()
    await expectNoCreditDue(page, 'globe entry')
    await pressSceneDot(page, 'tap')
    const preview = page.getByTestId('atlas-scene-preview-sheet')
    await expect(preview).toBeVisible()
    await expectNoCreditDue(page, 'scene preview at globe zoom')
    await page.getByRole('button', { name: 'Close scene preview' }).tap()

    // City view is camera-derived: street zoom over the scene engages it.
    await jumpToPhoenix(page, 12.5)
    const list = page.getByTestId('atlas-venue-sheet')
    await expect(list).toHaveAttribute('data-detent', 'peek', { timeout: 30_000 })
    await expectCreditVisible(page, 'city view, list at peek, banner up')
    const listBottom = await list.evaluate((el) => el.getBoundingClientRect().bottom)
    const bannerTop = await banner.evaluate((el) => el.getBoundingClientRect().top)
    expect(listBottom, 'list sheet ends above the banner').toBeLessThanOrEqual(bannerTop + 0.5)
  })
})

// The panel layout below `xl`, where the banner sits above the tab bar, and a
// short `xl` window: in both the credit is docked bottom-left, at the frame's
// bottom edge, with the banner up.
for (const viewport of [
  { width: 1024, height: 768 },
  { width: 1280, height: 560 },
] as const) {
  test.describe(`Atlas credit beside the rail at ${viewport.width}x${viewport.height}`, () => {
    test.use({ viewport })
    test.setTimeout(120_000)

    test('city view rail, banner up', async ({ page }) => {
      await stubAtlas(page)
      await page.goto('/atlas?city=Phoenix%2CAZ')
      await waitForMap(page)
      await expect(page.getByRole('dialog', { name: 'Cookie consent' })).toBeVisible()
      await expect(page.getByTestId('atlas-venue-rail')).toBeVisible({ timeout: 30_000 })
      await expectCreditVisible(page, 'city view rail, banner up')
    })
  })
}

test.describe('Atlas credit on desktop', () => {
  test.use({ viewport: { width: 1440, height: 900 } })
  test.setTimeout(120_000)

  test('globe, scene preview, rail and panels, banner up then dismissed', async ({
    page,
  }) => {
    await stubAtlas(page)
    await page.goto('/atlas')
    await waitForMap(page)
    const banner = page.getByRole('dialog', { name: 'Cookie consent' })
    await expect(banner).toBeVisible()
    // The full globe's night raster carries the NASA credit at globe zoom.
    await expectCreditVisible(page, 'globe entry, banner up', NASA_CREDIT)

    await pressSceneDot(page, 'click')
    await expect(page.getByRole('complementary', { name: 'Phoenix, AZ scene' })).toBeVisible()
    await expectCreditVisible(page, 'scene preview, banner up', NASA_CREDIT)
    await jumpToStreetBasemap(page)
    await expectCreditVisible(page, 'scene preview at z8, banner up')

    // City view is camera-derived: street zoom over the scene engages it.
    await jumpToPhoenix(page, 12.5)
    const rail = page.getByTestId('atlas-venue-rail')
    await expect(rail).toBeVisible({ timeout: 30_000 })
    await expectCreditVisible(page, 'city view rail, banner up')

    await rail.getByRole('button', { name: /Street Room/ }).click()
    const venuePanel = page.getByTestId('atlas-venue-panel')
    await expect(venuePanel.getByRole('button', { name: /Sheet Night/ })).toBeVisible()
    await expectCreditVisible(page, 'venue panel, banner up')

    await venuePanel.getByRole('button', { name: /Sheet Night/ }).click()
    const artistPanel = page.getByTestId('atlas-artist-panel')
    await expect(artistPanel.getByRole('heading', { name: 'Sheet Band' })).toBeVisible()
    await expectCreditVisible(page, 'artist panel, banner up')

    await page.getByRole('button', { name: 'Reject All' }).click()
    await expect(banner).toHaveCount(0)
    await expectCreditVisible(page, 'artist panel')
  })
})
