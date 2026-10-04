import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

/**
 * `/atlas?city=City,ST` (PSY-2079): the Atlas's one URL entry point.
 *
 * The scenes payload is synthesized so the city this asserts on has known
 * coordinates regardless of what the seed geocoded. The SwiftShader flags are
 * required: MapLibre needs a WebGL2 context, and headless Chromium has none
 * without them.
 */
test.use({
  launchOptions: {
    args: ['--enable-unsafe-swiftshader', '--use-angle=swiftshader'],
  },
  viewport: { width: 1440, height: 900 },
})

test.describe('Atlas city entry', () => {
  const PHOENIX = { lat: 33.4484, lng: -112.074 }

  async function stubScenes(page: Page) {
    await page.route(
      url => url.pathname.endsWith('/scenes'),
      route =>
        route.fulfill({
          json: {
            scenes: [
              {
                city: 'Phoenix',
                state: 'AZ',
                slug: 'phoenix-az',
                venue_count: 9,
                upcoming_show_count: 40,
                total_show_count: 100,
                shows_this_week: 2,
                shows_calendar_week: 2,
                latitude: PHOENIX.lat,
                longitude: PHOENIX.lng,
              },
              {
                city: 'Chicago',
                state: 'IL',
                slug: 'chicago-il',
                venue_count: 12,
                upcoming_show_count: 80,
                total_show_count: 200,
                shows_this_week: 5,
                shows_calendar_week: 5,
                latitude: 41.88,
                longitude: -87.63,
              },
            ],
            count: 2,
          },
        })
    )
  }

  /** The camera, through the seam the Atlas already keeps for verification. */
  function camera(page: Page) {
    return page.evaluate(() => {
      const map = (
        window as unknown as {
          __atlasMap?: {
            getCenter: () => { lng: number; lat: number }
            getZoom: () => number
          } | null
        }
      ).__atlasMap
      if (!map) return null
      const center = map.getCenter()
      return { lng: center.lng, lat: center.lat, zoom: map.getZoom() }
    })
  }

  test('opens the globe on the city the link names', async ({ page }) => {
    await stubScenes(page)
    await page.goto('/atlas?city=Phoenix%2CAZ')

    await expect
      .poll(() => camera(page), { timeout: 30_000 })
      .not.toBeNull()

    const view = (await camera(page))!
    expect(view.lat).toBeCloseTo(PHOENIX.lat, 1)
    expect(view.lng).toBeCloseTo(PHOENIX.lng, 1)
    // City view engages at zoom 11; the entry has to land past it or the link
    // drops the visitor at continental altitude over roughly the right place.
    expect(view.zoom).toBeGreaterThanOrEqual(11)
  })

  test('a city no scene knows leaves the globe where it opens without one', async ({
    page,
  }) => {
    await stubScenes(page)
    await page.goto('/atlas?city=Atlantis%2CZZ')

    await expect
      .poll(() => camera(page), { timeout: 30_000 })
      .not.toBeNull()

    const view = (await camera(page))!
    // The continental default, not a guessed city: the harness serves no geo,
    // so this is North America, far above city view.
    expect(view.zoom).toBeLessThan(11)
    expect(view.lat).toBeCloseTo(39.5, 0)
    expect(view.lng).toBeCloseTo(-98.35, 0)
  })
})

/**
 * The light globe's boundary lines and place labels on a compact viewport
 * (narrower than `lg`), on an 800px tablet pane.
 */
test.describe('Atlas light globe outlines and place labels', () => {
  test.use({ viewport: { width: 800, height: 1000 } })

  const LINE_LAYERS = ['globe-state-lines', 'globe-country-lines']

  type MapSeam = {
    getLayoutProperty: (layer: string, name: string) => unknown
    queryRenderedFeatures: (options: { layers: string[] }) => unknown[]
    jumpTo: (options: { center: [number, number]; zoom: number }) => void
    getZoom: () => number
  }

  function renderedLines(page: Page) {
    return page.evaluate((layers) => {
      const map = (window as unknown as { __atlasMap?: MapSeam | null }).__atlasMap
      if (!map) return null
      return Object.fromEntries(
        layers.map((id) => [id, map.queryRenderedFeatures({ layers: [id] }).length]),
      )
    }, LINE_LAYERS)
  }

  test('draws the outlines and place labels at globe zoom, clear of the scene labels, and neither at street zoom', async ({
    page,
  }) => {
    await page.route(
      url => url.pathname.endsWith('/scenes'),
      route =>
        route.fulfill({
          json: {
            scenes: [
              {
                city: 'Chicago',
                state: 'IL',
                slug: 'chicago-il',
                venue_count: 12,
                upcoming_show_count: 200,
                total_show_count: 300,
                shows_this_week: 0,
                shows_calendar_week: 0,
                latitude: 41.88,
                longitude: -87.63,
              },
              {
                city: 'Phoenix',
                state: 'AZ',
                slug: 'phoenix-az',
                venue_count: 9,
                upcoming_show_count: 180,
                total_show_count: 250,
                shows_this_week: 0,
                shows_calendar_week: 0,
                latitude: 33.4484,
                longitude: -112.074,
              },
            ],
            count: 2,
          },
        })
    )
    await page.goto('/atlas')

    // Globe zoom: both line layers are shown, and the country lines render.
    await expect
      .poll(async () => (await renderedLines(page))?.['globe-country-lines'] ?? 0, {
        timeout: 30_000,
      })
      .toBeGreaterThan(0)
    const visibility = await page.evaluate((layers) => {
      const map = (window as unknown as { __atlasMap: MapSeam }).__atlasMap
      return layers.map((id) => map.getLayoutProperty(id, 'visibility'))
    }, LINE_LAYERS)
    expect(visibility).toEqual(['visible', 'visible'])

    // Place labels: at least one, none over a scene label.
    const placeLabels = page.getByTestId('atlas-place-label')
    await expect(placeLabels.first()).toBeAttached({ timeout: 30_000 })
    const overlaps = await page.evaluate(() => {
      const rect = (el: Element) => el.getBoundingClientRect()
      const scenes = [...document.querySelectorAll<HTMLElement>('[data-testid="atlas-scene-label"]')]
        .filter((el) => el.style.opacity !== '0')
        .map(rect)
      const hits: string[] = []
      for (const label of document.querySelectorAll('[data-testid="atlas-place-label"]')) {
        const a = rect(label)
        for (const b of scenes) {
          if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) {
            hits.push(label.textContent ?? '')
          }
        }
      }
      return { hits, scenes: scenes.length }
    })
    expect(overlaps.scenes).toBeGreaterThan(0)
    expect(overlaps.hits).toEqual([])

    // Street zoom: past the globe surface's cutoff, nothing of either draws.
    await page.evaluate(() => {
      const map = (window as unknown as { __atlasMap: MapSeam }).__atlasMap
      map.jumpTo({ center: [-87.63, 41.88], zoom: 12.5 })
    })
    await expect
      .poll(() => renderedLines(page), { timeout: 30_000 })
      .toEqual({ 'globe-state-lines': 0, 'globe-country-lines': 0 })
    await expect(placeLabels).toHaveCount(0)
  })
})

test.describe('Atlas on a phone that prefers reduced motion', () => {
  test.use({
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
  })

  test('lists the scenes, with their links, instead of the map', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto('/atlas')
    const list = page.getByTestId('atlas-scene-list')
    await expect(list).toBeVisible({ timeout: 30_000 })
    await expect(page.locator('canvas.maplibregl-canvas')).toHaveCount(0)
    const firstRow = list.getByRole('button', { expanded: false }).first()
    await firstRow.tap()
    await expect(list.getByRole('link', { name: /Open scene/ })).toHaveAttribute(
      'href',
      /^\/scenes\/[a-z0-9-]+$/
    )
  })
})
