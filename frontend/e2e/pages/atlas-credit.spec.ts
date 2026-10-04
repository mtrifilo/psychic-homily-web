import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

/**
 * The map credit (the OpenStreetMap attribution the ODbL requires) in every
 * Atlas state where one is shown, with the cookie banner up and after it is
 * dismissed. In each, the credit's whole text is on screen and every sample
 * point across it hits the credit itself. At globe zoom the compact globe
 * (below `lg`) draws no OpenStreetMap data, so there the check is that nothing
 * OSM-derived renders and the control lists nothing.
 *
 * Scenes, venues and shows are synthesized so the city, its pins and the bill
 * are known regardless of the seed. SwiftShader is required: MapLibre needs a
 * WebGL2 context headless Chromium otherwise lacks.
 */

test.use({
  launchOptions: {
    args: ['--enable-unsafe-swiftshader', '--use-angle=swiftshader'],
  },
})

const VIEWPORTS = [
  { width: 390, height: 844 },
  { width: 390, height: 664 },
  { width: 360, height: 780 },
] as const

const PHOENIX = { lat: 33.4484, lng: -112.074 }

function venue(id: number, name: string, extra: Record<string, unknown> = {}) {
  return {
    id,
    slug: `venue-${id}`,
    name,
    address: null,
    city: 'Phoenix',
    state: 'AZ',
    latitude: PHOENIX.lat,
    longitude: PHOENIX.lng,
    verified: true,
    upcoming_show_count: 3,
    shows_this_week: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
    ...extra,
  }
}

async function stubAtlas(page: Page) {
  await page.route(
    (url) => url.pathname.endsWith('/scenes'),
    (route) =>
      route.fulfill({
        json: {
          scenes: [
            {
              city: 'Phoenix',
              state: 'AZ',
              slug: 'phoenix-az',
              venue_count: 3,
              upcoming_show_count: 9,
              total_show_count: 9,
              shows_this_week: 0,
              shows_calendar_week: 0,
              latitude: PHOENIX.lat,
              longitude: PHOENIX.lng,
            },
          ],
          count: 1,
        },
      }),
  )
  await page.route(
    (url) => /\/venues$/.test(url.pathname),
    (route) =>
      route.fulfill({
        json: {
          venues: [
            venue(1, 'Centroid Room One'),
            venue(2, 'Centroid Room Two'),
            venue(3, 'Street Room', {
              street_latitude: 33.4943,
              street_longitude: -112.0326,
            }),
          ],
          total: 3,
          limit: 100,
          offset: 0,
        },
      }),
  )
  await page.route(
    (url) => /\/venues\/\d+\/shows$/.test(url.pathname),
    (route) =>
      route.fulfill({
        json: {
          venue_id: 3,
          total: 1,
          shows: [
            {
              id: 501,
              slug: 'credit-night',
              title: 'Credit Night',
              event_date: '2030-01-05T03:00:00Z',
              city: 'Phoenix',
              state: 'AZ',
              price: null,
              age_requirement: null,
              artists: [{ id: 901, slug: 'credit-band', name: 'Credit Band' }],
            },
          ],
        },
      }),
  )
}

type AtlasMapSeam = {
  isStyleLoaded: () => boolean
  areTilesLoaded: () => boolean
  getZoom: () => number
  getStyle: () => { layers: { id: string; source?: string }[] }
  queryRenderedFeatures: (opts: { layers: string[] }) => unknown[]
  project: (lngLat: [number, number]) => { x: number; y: number }
  getCanvas: () => HTMLCanvasElement
  jumpTo: (o: { center: [number, number]; zoom: number }) => void
}

async function waitForMap(page: Page) {
  await expect
    .poll(
      () =>
        page.evaluate(() => {
          const m = (window as unknown as { __atlasMap?: AtlasMapSeam | null })
            .__atlasMap
          return !!m && m.isStyleLoaded() && m.areTilesLoaded()
        }),
      { timeout: 60_000 },
    )
    .toBe(true)
}

/**
 * Where the credit is and what covers it. The document is first scrolled
 * back to the top: a tap's actionability scroll can move the page (the footer
 * sits below the Atlas), which says nothing about what the layout covers.
 *
 * `covered` lists every sample point (three across, at three heights) whose
 * topmost element is not the credit; `clipped` is true when the text overflows
 * its box or the box leaves the viewport; `underSheet` names any sheet whose
 * top edge sits above the credit's bottom edge.
 */
function creditReport(page: Page) {
  return page.evaluate(() => {
    window.scrollTo(0, 0)
    const el = document.querySelector('.maplibregl-ctrl-attrib')
    const inner = document.querySelector('.maplibregl-ctrl-attrib-inner')
    if (!el || !inner) return null
    const r = el.getBoundingClientRect()
    const covered: string[] = []
    for (const fy of [0.25, 0.5, 0.75]) {
      for (const x of [r.left + 4, r.left + r.width / 2, r.right - 4]) {
        const y = r.top + r.height * fy
        // The Next.js dev indicator, a dev-server-only overlay, is looked
        // through rather than counted as covering.
        const hit = document
          .elementsFromPoint(x, y)
          .find((h) => h.tagName !== 'NEXTJS-PORTAL' && !h.closest('nextjs-portal'))
        if (!hit || !el.contains(hit)) {
          const who = hit
            ? `${hit.tagName.toLowerCase()}${hit.getAttribute('data-testid') ? `[${hit.getAttribute('data-testid')}]` : ''}${hit.getAttribute('aria-label') ? `(${hit.getAttribute('aria-label')})` : ''}`
            : 'nothing'
          covered.push(`${Math.round(x)},${Math.round(y)}:${who}`)
        }
      }
    }
    const clipped =
      inner.scrollWidth > inner.clientWidth + 1 ||
      inner.scrollHeight > inner.clientHeight + 1 ||
      r.left < 0 ||
      r.top < 0 ||
      r.right > window.innerWidth ||
      r.bottom > window.innerHeight
    const sheets = Array.from(
      document.querySelectorAll<HTMLElement>('[data-slot="bottom-sheet"]'),
    )
      .filter((s) => s.getBoundingClientRect().height > 0)
      .map((s) => ({
        id: s.getAttribute('data-testid') ?? 'sheet',
        top: Math.round(s.getBoundingClientRect().top),
      }))
    const underSheet = sheets.filter((s) => s.top < r.bottom).map((s) => s.id)
    return {
      sheets,
      text: inner.textContent ?? '',
      empty: el.classList.contains('maplibregl-attrib-empty'),
      rect: {
        top: Math.round(r.top),
        bottom: Math.round(r.bottom),
        left: Math.round(r.left),
        right: Math.round(r.right),
      },
      covered,
      clipped,
      underSheet,
    }
  })
}

const OSM_CREDIT = ['OpenFreeMap', 'OpenStreetMap contributors']
const NASA_CREDIT = ['NASA GIBS']

async function expectCreditVisible(
  page: Page,
  state: string,
  credits: string[] = OSM_CREDIT,
) {
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

async function jumpTo(page: Page, zoom: number) {
  await page.evaluate(
    ({ lng, lat, zoom }) => {
      const m = (window as unknown as { __atlasMap: AtlasMapSeam }).__atlasMap
      m.jumpTo({ center: [lng, lat], zoom })
    },
    { ...PHOENIX, zoom },
  )
}

/** A tap or click on the Phoenix scene dot, through the map's own projection. */
async function pressSceneDot(page: Page, how: 'tap' | 'click') {
  const point = await page.evaluate(({ lng, lat }) => {
    const m = (window as unknown as { __atlasMap: AtlasMapSeam }).__atlasMap
    const p = m.project([lng, lat])
    const r = m.getCanvas().getBoundingClientRect()
    return { x: r.left + p.x, y: r.top + p.y }
  }, PHOENIX)
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

// Fixme while AtlasGlobe's 640px gate renders MobileSceneList at these widths
// (no map exists to measure); PSY-1560 removes the gate and un-fixmes these.
for (const viewport of VIEWPORTS) {
  for (const colorScheme of ['dark', 'light'] as const) {
    test.describe.fixme(`Atlas credit at ${viewport.width}x${viewport.height} ${colorScheme}`, () => {
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
        await stepDetent(page, 'atlas-venue-sheet', 'full')
        await expectCreditVisible(page, 'list at full, banner up')

        await page.getByRole('button', { name: 'Reject All' }).tap()
        await expect(page.getByRole('dialog', { name: 'Cookie consent' })).toHaveCount(0)
        await expectCreditVisible(page, 'list at full')

        await list.getByRole('button', { name: /Street Room/ }).tap()
        const venueSheet = page.getByTestId('atlas-venue-panel')
        await expect(venueSheet).toHaveAttribute('data-detent', 'half')
        await expectCreditVisible(page, 'venue sheet at half')
        await stepDetent(page, 'atlas-venue-panel', 'full')
        await expectCreditVisible(page, 'venue sheet at full')

        await venueSheet.getByRole('button', { name: /Credit Night/ }).tap()
        const artistSheet = page.getByTestId('atlas-artist-panel')
        await expect(artistSheet).toHaveAttribute('data-detent', 'full')
        await expect(artistSheet.getByRole('heading', { name: 'Credit Band' })).toBeVisible()
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

        // Past the street basemap's first zoom with the preview still open,
        // so the OpenStreetMap credit is due while the preview is up.
        await jumpTo(page, 8)
        await expect(page.locator('.maplibregl-ctrl-attrib-inner')).toContainText(
          'OpenStreetMap',
          { timeout: 30_000 },
        )
        await expect(preview).toBeVisible()
        await expectCreditVisible(page, 'scene preview at half, z8, banner up')
        await stepDetent(page, 'atlas-scene-preview-sheet', 'full')
        await expectCreditVisible(page, 'scene preview at full, z8, banner up')
      })
    })
  }
}

test.describe('Atlas credit on a compact pane above the mobile gate', () => {
  test.use({ viewport: { width: 820, height: 1000 }, hasTouch: true })

  test('globe entry and scene preview owe no credit', async ({ page }) => {
    await stubAtlas(page)
    await page.goto('/atlas')
    await waitForMap(page)
    await expectNoCreditDue(page, 'globe entry')
    await pressSceneDot(page, 'tap')
    await expect(page.getByTestId('atlas-scene-preview-sheet')).toBeVisible()
    await expectNoCreditDue(page, 'scene preview at globe zoom')
  })
})

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
    await jumpTo(page, 8)
    await expect(page.locator('.maplibregl-ctrl-attrib-inner')).toContainText(
      'OpenStreetMap',
      { timeout: 30_000 },
    )
    await expectCreditVisible(page, 'scene preview at z8, banner up')

    await page.goto('/atlas?city=Phoenix%2CAZ')
    await waitForMap(page)
    const rail = page.getByTestId('atlas-venue-rail')
    await expect(rail).toBeVisible({ timeout: 30_000 })
    await expectCreditVisible(page, 'city view rail, banner up')

    await rail.getByRole('button', { name: /Street Room/ }).click()
    const venuePanel = page.getByTestId('atlas-venue-panel')
    await expect(venuePanel.getByRole('button', { name: /Credit Night/ })).toBeVisible()
    await expectCreditVisible(page, 'venue panel, banner up')

    await venuePanel.getByRole('button', { name: /Credit Night/ }).click()
    const artistPanel = page.getByTestId('atlas-artist-panel')
    await expect(artistPanel.getByRole('heading', { name: 'Credit Band' })).toBeVisible()
    await expectCreditVisible(page, 'artist panel, banner up')

    await page.getByRole('button', { name: 'Reject All' }).click()
    await expect(banner).toHaveCount(0)
    await expectCreditVisible(page, 'artist panel')
  })
})
