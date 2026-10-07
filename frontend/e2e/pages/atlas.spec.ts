import { test } from '../fixtures'
import { expect, type Page } from '@playwright/test'
import {
  type AtlasMapSeam,
  dismissBanner,
  phoenixDotPoint,
  stubAtlas,
  waitForMap,
} from '../helpers/atlas'

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

/**
 * The light globe's place labels against the controls drawn over the map, on
 * a 390x844 phone signed in, so the My Scenes strip shows beside the search
 * pill, Drift, the Genres chip and the credit. These are the real controls
 * with their real classes: the label pass finds them by walking the pane, so
 * only a browser shows whether each wrapper lets it through to the control.
 */
for (const colorScheme of ['light', 'dark'] as const) {
  test.describe(`Atlas place labels clear of the chrome at 390x844 ${colorScheme}`, () => {
    test.use({
      viewport: { width: 390, height: 844 },
      colorScheme,
      hasTouch: true,
      isMobile: true,
      deviceScaleFactor: 2,
    })
    // The test boots a SwiftShader map signed in, waits out the banner's
    // resize, and moves the camera twice, each step waiting on the labels'
    // relayout.
    test.setTimeout(60_000)

    // The place set's Houston (public/atlas/globe-places-110m.geojson).
    const HOUSTON: [number, number] = [-95.34, 29.82]
    const DRIFT = 'button[aria-label="Drift to a random scene"]'

    type PaneWindow = {
      __atlasMap: AtlasMapSeam & {
        getContainer: () => HTMLElement
        unproject: (point: [number, number]) => { lng: number; lat: number }
      }
    }

    /**
     * Waits until the map's canvas fills the Atlas frame: the frame grows when
     * the banner leaves, and the measured pane, the map's container and then
     * the canvas follow it.
     */
    async function waitForMapSize(page: Page) {
      await expect
        .poll(() =>
          page.evaluate(() => {
            const map = (window as unknown as PaneWindow).__atlasMap
            const canvas = map.getCanvas().getBoundingClientRect()
            const frame = document.querySelector('[data-testid="atlas-pane-frame"]')!.getBoundingClientRect()
            return Math.abs(canvas.width - frame.width) < 1 && Math.abs(canvas.height - frame.height) < 1
          }),
        )
        .toBe(true)
    }

    type ChromeReport = {
      labels: string[]
      /** Each control's name and whether it has an area on screen. */
      controls: { name: string; drawn: boolean }[]
      /** `label under control` for every label box that meets a control's. */
      hits: string[]
    }

    /**
     * Every place label and the box of each control: the search pill, Drift,
     * the Genres chip, the My Scenes strip's star and chips, and the credit.
     */
    function chromeReport(page: Page): Promise<ChromeReport> {
      return page.evaluate((drift) => {
        const controls: { name: string; el: Element }[] = []
        const add = (name: string, el: Element | null | undefined) => {
          if (el) controls.push({ name, el })
        }
        add('search pill', document.querySelector('button[aria-label="Search scenes"]'))
        add('Drift', document.querySelector(drift))
        add('Genres chip', document.querySelector('[aria-controls="atlas-genre-legend"]')?.parentElement)
        for (const el of document.querySelectorAll('nav[aria-label="My scenes"] > *')) {
          add('My Scenes', el)
        }
        add('credit', document.querySelector('.maplibregl-ctrl-attrib'))

        const labels = [...document.querySelectorAll('[data-testid="atlas-place-label"]')]
        const hits: string[] = []
        for (const label of labels) {
          const a = label.getBoundingClientRect()
          for (const { name, el } of controls) {
            const b = el.getBoundingClientRect()
            if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) {
              hits.push(`${label.textContent} under ${name}`)
            }
          }
        }
        return {
          labels: labels.map((label) => label.textContent ?? ''),
          controls: controls.map(({ name, el }) => {
            const r = el.getBoundingClientRect()
            return { name, drawn: r.width > 0 && r.height > 0 }
          }),
          hits,
        }
      }, DRIFT)
    }

    /**
     * Waits until no place label meets a control (a control that has just
     * moved holds its old spot until the chrome has settled), with labels
     * drawn and every named control but the credit on screen. The compact
     * globe owes no credit at globe zoom, so the credit is checked only when
     * it is drawn.
     */
    async function expectLabelsClearOfChrome(page: Page, state: string) {
      let report: ChromeReport | undefined
      await expect
        .poll(
          async () => {
            report = await chromeReport(page)
            return report.hits
          },
          { message: `${state}: place labels under a control`, timeout: 10_000 },
        )
        .toEqual([])
      test.info().annotations.push({ type: state, description: JSON.stringify(report) })
      expect(report!.labels.length, `${state}: place labels drawn`).toBeGreaterThan(0)
      const drawn = report!.controls.filter((c) => c.drawn).map((c) => c.name)
      for (const name of ['search pill', 'Drift', 'Genres chip', 'My Scenes']) {
        expect(drawn, `${state}: ${name} on screen`).toContain(name)
      }
    }

    function placeLabelTexts(page: Page) {
      return page
        .getByTestId('atlas-place-label')
        .evaluateAll((labels) => labels.map((label) => label.textContent))
    }

    test('no place label sits under a control at the entry camera, and one panned under Drift is dropped', async ({
      authenticatedPage: page,
    }) => {
      await stubAtlas(page)
      // A followed scene, so the My Scenes strip shows.
      await page.route(
        (url) => url.pathname.endsWith('/me/following'),
        (route) =>
          route.fulfill({
            json: {
              following: [
                {
                  entity_type: 'scene',
                  entity_id: 1,
                  name: 'Phoenix',
                  slug: 'phoenix-az',
                  followed_at: '2026-09-01T00:00:00Z',
                },
              ],
              total: 1,
              limit: 100,
              offset: 0,
            },
          }),
      )
      await page.goto('/atlas')
      await waitForMap(page)
      await expect(page.getByTestId('atlas-place-label').first()).toBeAttached({ timeout: 30_000 })
      await expect(page.getByRole('navigation', { name: 'My scenes' })).toBeVisible()

      await expectLabelsClearOfChrome(page, 'entry camera, banner up')
      await dismissBanner(page)
      await waitForMapSize(page)
      await expectLabelsClearOfChrome(page, 'entry camera')

      // Houston on the map's centre at z4, clear of every control: drawn.
      await page.evaluate((center) => {
        const map = (window as unknown as PaneWindow).__atlasMap
        map.jumpTo({ center, zoom: 4 })
      }, HOUSTON)
      await expect.poll(() => placeLabelTexts(page)).toContain('Houston')

      // The same zoom with Houston's point on Drift's centre: dropped.
      const underDrift = await page.evaluate(
        ({ center, drift }) => {
          const map = (window as unknown as PaneWindow).__atlasMap
          const pane = map.getContainer().getBoundingClientRect()
          const button = document.querySelector(drift)!.getBoundingClientRect()
          const x = button.left + button.width / 2 - pane.left
          const y = button.top + button.height / 2 - pane.top
          // On the globe a screen offset is not a fixed shift in lng/lat, so
          // the camera steps until Houston projects onto the target. Each
          // step re-centres on the point that sits where the camera must move
          // Houston from.
          let p = map.project(center)
          for (let step = 0; step < 10 && Math.hypot(x - p.x, y - p.y) > 1; step++) {
            const next = map.unproject([pane.width / 2 - (x - p.x), pane.height / 2 - (y - p.y)])
            map.jumpTo({ center: [next.lng, next.lat], zoom: 4 })
            p = map.project(center)
          }
          return {
            houston: { x: p.x, y: p.y },
            drift: {
              left: button.left - pane.left,
              top: button.top - pane.top,
              right: button.right - pane.left,
              bottom: button.bottom - pane.top,
            },
          }
        },
        { center: HOUSTON, drift: DRIFT },
      )
      const { houston, drift } = underDrift
      expect(
        houston.x > drift.left && houston.x < drift.right && houston.y > drift.top && houston.y < drift.bottom,
        `Houston's point ${JSON.stringify(houston)} is under Drift ${JSON.stringify(drift)}`,
      ).toBe(true)
      await expectLabelsClearOfChrome(page, 'Houston under Drift at z4')
      expect(await placeLabelTexts(page)).not.toContain('Houston')
    })
  })
}

test.describe('Atlas on a phone that prefers reduced motion', () => {
  test.use({
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
  })

  test('lists the scenes, with their links, instead of the map', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto('/atlas')
    // The browser has WebGL2, so the list is reduced motion's doing.
    expect(
      await page.evaluate(() => !!document.createElement('canvas').getContext('webgl2'))
    ).toBe(true)
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

/**
 * A lost WebGL context, forced through the `WEBGL_lose_context` extension on
 * the map's own canvas. The extension never restores on its own, so a loss
 * without `restoreContext()` is a loss the browser never restores.
 */
test.describe('Atlas when the map loses its WebGL context', () => {
  // ATLAS_CONTEXT_RESTORE_DEADLINE_MS in features/scenes/components/atlasMapHealth.ts.
  const RESTORE_DEADLINE_MS = 3_000

  type ContextWindow = {
    __atlasMap?: AtlasMapSeam | null
    __atlasLoseContext?: WEBGL_lose_context | null
  }

  // Uncaught exceptions only. The fixture's `errors` also fails on any
  // console.error, and React logs every error a boundary catches, which is
  // how the fallback works.
  function collectPageErrors(page: Page) {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    return errors
  }

  /** Loses the context and resolves once the canvas has reported it. */
  function loseContext(page: Page) {
    return page.evaluate(
      () =>
        new Promise<void>((resolve, reject) => {
          const w = window as unknown as ContextWindow
          const canvas = w.__atlasMap?.getCanvas()
          const extension = canvas?.getContext('webgl2')?.getExtension('WEBGL_lose_context')
          if (!canvas || !extension) {
            reject(new Error('no map canvas with WEBGL_lose_context'))
            return
          }
          w.__atlasLoseContext = extension
          canvas.addEventListener('webglcontextlost', () => resolve(), { once: true })
          extension.loseContext()
        })
    )
  }

  test.beforeEach(async ({ page }) => {
    // A pulse ring, so the map repaints every frame through the loss.
    await stubAtlas(page, { showsThisWeek: 1 })
  })

  test('falls back to the scene list when the context is not restored', async ({ page }) => {
    const errors = collectPageErrors(page)
    await page.goto('/atlas')
    await waitForMap(page)

    await loseContext(page)
    const list = page.getByTestId('atlas-scene-list')
    // Inside the deadline the map waits for a restore.
    await page.waitForTimeout(1_000)
    await expect(list).toHaveCount(0)
    await expect(page.locator('canvas.maplibregl-canvas')).toHaveCount(1)
    await expect(list).toBeVisible({ timeout: RESTORE_DEADLINE_MS + 10_000 })
    await expect(list.getByText('Phoenix')).toBeVisible()
    await expect(page.locator('canvas.maplibregl-canvas')).toHaveCount(0)
    expect(errors).toEqual([])
  })

  test('keeps the map when the context is restored, with the scene dot hovered', async ({
    page,
  }) => {
    const errors = collectPageErrors(page)
    await page.goto('/atlas')
    await waitForMap(page)

    // The pointer rests on the scene dot through the loss, then leaves it.
    const dot = await phoenixDotPoint(page)
    await page.mouse.move(dot.x, dot.y)
    await expect
      .poll(() =>
        page.evaluate(
          () => (window as unknown as ContextWindow).__atlasMap!.getCanvas().style.cursor
        )
      )
      .toBe('pointer')
    await loseContext(page)
    await page.mouse.move(dot.x + 120, dot.y + 120)
    await page.evaluate(
      () =>
        new Promise<void>((resolve) => {
          const w = window as unknown as ContextWindow
          w.__atlasMap!.getCanvas().addEventListener('webglcontextrestored', () => resolve(), {
            once: true,
          })
          w.__atlasLoseContext!.restoreContext()
        })
    )
    // MapLibre re-creates the style it saved at the loss.
    await waitForMap(page)

    // Past the deadline, the map is still the map.
    await page.waitForTimeout(RESTORE_DEADLINE_MS + 1_000)
    await expect(page.getByTestId('atlas-scene-list')).toHaveCount(0)
    await expect(page.locator('canvas.maplibregl-canvas')).toHaveCount(1)
    // The canvas's own effects ran again on the restored style: the credit
    // control they remove at the loss is back.
    await expect(page.locator('.maplibregl-ctrl-attrib')).toHaveCount(1)

    // And it still draws: the scene came back with the style, and a camera
    // move renders a frame.
    const restored = await page.evaluate(
      () =>
        new Promise<{ scenes: number; rendered: boolean }>((resolve) => {
          const map = (window as unknown as ContextWindow).__atlasMap!
          const scenes = map.getStyle().sources.scenes?.data?.features?.length ?? 0
          map.once('render', () => resolve({ scenes, rendered: true }))
          setTimeout(() => resolve({ scenes, rendered: false }), 5_000)
          const center = map.getCenter()
          map.jumpTo({ center: [center.lng, center.lat], zoom: map.getZoom() + 0.5 })
        })
    )
    expect(restored).toEqual({ scenes: 1, rendered: true })
    expect(errors).toEqual([])
  })
})
