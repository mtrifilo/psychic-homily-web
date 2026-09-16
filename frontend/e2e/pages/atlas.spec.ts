import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

/**
 * `/atlas?city=City,ST` (PSY-2079) — the Atlas's one URL entry point.
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
    // The continental default, not a guessed city: far above city view.
    expect(view.zoom).toBeLessThan(11)
  })
})
