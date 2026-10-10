import { test } from '../fixtures'
import { expect, type Page } from '@playwright/test'
import {
  ATLAS_TEST_TIMEOUT_MS,
  type AtlasMapSeam,
  dismissBanner,
  phoenixDotPoint,
  PHONE_CONTEXT,
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

test.describe.configure({ timeout: ATLAS_TEST_TIMEOUT_MS })

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
 * The compact globe draws its credit only from the street basemap's first
 * zoom (5), and place labels show below 5.5, so the credit is checked at 5.2.
 */
for (const colorScheme of ['light', 'dark'] as const) {
  test.describe(`Atlas place labels clear of the chrome at 390x844 ${colorScheme}`, () => {
    test.use({ viewport: { width: 390, height: 844 }, colorScheme, ...PHONE_CONTEXT })

    // The place set's Houston (public/atlas/globe-places-110m.geojson).
    const HOUSTON: [number, number] = [-95.34, 29.82]
    const DRIFT = 'button[aria-label="Drift to a random scene"]'
    const CREDIT = '.maplibregl-ctrl-attrib'
    const CONTROLS_AT_GLOBE_ZOOM = ['search pill', 'Drift', 'Genres chip', 'My Scenes']

    type SeamWindow = { __atlasMap: AtlasMapSeam }

    /**
     * Waits until the map's canvas fills the Atlas frame: the frame grows when
     * the banner leaves, and the measured pane, the map's container and then
     * the canvas follow it.
     */
    async function waitForMapSize(page: Page) {
      await expect
        .poll(() =>
          page.evaluate(() => {
            const map = (window as unknown as SeamWindow).__atlasMap
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
      return page.evaluate(({ drift, credit }) => {
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
        add('credit', document.querySelector(credit))

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
      }, { drift: DRIFT, credit: CREDIT })
    }

    /**
     * Waits until no place label meets a control (a control that has just
     * moved holds its old spot until the chrome has settled), then checks
     * that every control in `drawn` is on screen and, unless `labelsDrawn` is
     * false, that place labels are drawn. Returns the report that passed.
     */
    async function expectLabelsClearOfChrome(
      page: Page,
      state: string,
      { drawn: required = CONTROLS_AT_GLOBE_ZOOM, labelsDrawn = true } = {},
    ): Promise<ChromeReport> {
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
      if (labelsDrawn) {
        expect(report!.labels.length, `${state}: place labels drawn`).toBeGreaterThan(0)
      }
      const drawn = report!.controls.filter((c) => c.drawn).map((c) => c.name)
      for (const name of required) {
        expect(drawn, `${state}: ${name} on screen`).toContain(name)
      }
      return report!
    }

    /** Moves the camera to `center` at `zoom`, synchronously. */
    function jumpTo(page: Page, center: [number, number], zoom: number) {
      return page.evaluate(
        ({ center, zoom }) => (window as unknown as SeamWindow).__atlasMap.jumpTo({ center, zoom }),
        { center, zoom },
      )
    }

    /**
     * Keeps `zoom` and steps the camera until `place` projects onto the
     * centre of the control `selector` matches, then checks that it does.
     */
    async function moveUnder(page: Page, place: [number, number], selector: string, zoom: number) {
      const { point, box } = await page.evaluate(
        ({ place, selector, zoom }) => {
          const map = (window as unknown as SeamWindow).__atlasMap
          const pane = map.getContainer().getBoundingClientRect()
          const control = document.querySelector(selector)!.getBoundingClientRect()
          const x = control.left + control.width / 2 - pane.left
          const y = control.top + control.height / 2 - pane.top
          // On the globe a screen offset is not a fixed shift in lng/lat, so
          // the camera steps until the place projects onto the target. Each
          // step re-centres on the point that sits where the camera must move
          // the place from.
          let p = map.project(place)
          for (let step = 0; step < 10 && Math.hypot(x - p.x, y - p.y) > 1; step++) {
            const next = map.unproject([pane.width / 2 - (x - p.x), pane.height / 2 - (y - p.y)])
            map.jumpTo({ center: [next.lng, next.lat], zoom })
            p = map.project(place)
          }
          return {
            point: { x: p.x, y: p.y },
            box: {
              left: control.left - pane.left,
              top: control.top - pane.top,
              right: control.right - pane.left,
              bottom: control.bottom - pane.top,
            },
          }
        },
        { place, selector, zoom },
      )
      expect(
        point.x > box.left && point.x < box.right && point.y > box.top && point.y < box.bottom,
        `the place's point ${JSON.stringify(point)} is under ${selector} ${JSON.stringify(box)}`,
      ).toBe(true)
    }

    function placeLabelTexts(page: Page) {
      return page
        .getByTestId('atlas-place-label')
        .evaluateAll((labels) => labels.map((label) => label.textContent))
    }

    test('no place label sits under a control at the entry camera, and one panned under Drift or the credit is dropped', async ({
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
      await jumpTo(page, HOUSTON, 4)
      await expect.poll(() => placeLabelTexts(page)).toContain('Houston')

      // The same zoom with Houston's point on Drift's centre: dropped.
      await moveUnder(page, HOUSTON, DRIFT, 4)
      const underDrift = await expectLabelsClearOfChrome(page, 'Houston under Drift at z4')
      expect(underDrift.labels).not.toContain('Houston')

      // Houston on the map's centre at z5.2, where the credit is drawn too.
      await jumpTo(page, HOUSTON, 5.2)
      await expect(page.locator('.maplibregl-ctrl-attrib-inner')).toContainText('OpenStreetMap', {
        timeout: 30_000,
      })
      await expect.poll(() => placeLabelTexts(page)).toContain('Houston')
      const withCredit = [...CONTROLS_AT_GLOBE_ZOOM, 'credit']
      await expectLabelsClearOfChrome(page, 'Houston at z5.2, credit drawn', { drawn: withCredit })

      // The same zoom with Houston's point on the credit's centre: dropped.
      // Houston may be the only label in that view.
      await moveUnder(page, HOUSTON, CREDIT, 5.2)
      const underCredit = await expectLabelsClearOfChrome(page, 'Houston under the credit at z5.2', {
        drawn: withCredit,
        labelsDrawn: false,
      })
      expect(underCredit.labels).not.toContain('Houston')
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
  // Equal to ATLAS_CONTEXT_RESTORE_DEADLINE_MS in
  // features/scenes/components/atlasMapHealth.ts. A lower value here fails
  // restores the Atlas keeps.
  const RESTORE_DEADLINE_MS = 3_000

  type ContextWindow = { __atlasMap?: AtlasMapSeam | null }

  /** The canvas keeps its `WEBGL_lose_context` from before a loss for the restore. */
  type LosableCanvas = HTMLCanvasElement & { __loseContext?: WEBGL_lose_context | null }

  // Uncaught exceptions only. The fixture's `errors` also fails on any
  // console.error, and React logs every error a boundary catches, which is
  // how the fallback works.
  function collectPageErrors(page: Page) {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    return errors
  }

  type ContextChange = {
    method: 'loseContext' | 'restoreContext'
    event: 'webglcontextlost' | 'webglcontextrestored'
    timeoutMs: number
    /** The loss time the bound counts from, or null to count from the call. */
    since: number | null
  }

  /**
   * Makes a context change through the map canvas's `WEBGL_lose_context` and
   * resolves with the page's `performance.now()` when the canvas fired the
   * change's event; rejects naming the event when it has not fired within the
   * bound. A lost context hands out no extensions, so the canvas keeps the
   * one it gave before the loss and a restore reuses it.
   */
  function changeContext(page: Page, change: ContextChange) {
    return page.evaluate(
      ({ method, event, timeoutMs, since }) =>
        new Promise<number>((resolve, reject) => {
          const start = performance.now()
          const deadline = (since ?? start) + timeoutMs
          if (deadline <= start) {
            reject(
              new Error(
                `${method} was called ${Math.round(start - (since ?? start))} ms after the loss, past the ${timeoutMs} ms deadline`
              )
            )
            return
          }
          const canvas = (window as unknown as ContextWindow).__atlasMap?.getCanvas() as
            | LosableCanvas
            | undefined
          if (!canvas) {
            reject(new Error('window.__atlasMap holds no map'))
            return
          }
          canvas.__loseContext ??= canvas.getContext('webgl2')?.getExtension('WEBGL_lose_context')
          const extension = canvas.__loseContext
          if (!extension) {
            reject(new Error('the map canvas has no WEBGL_lose_context'))
            return
          }
          // Rounded up, so the timer never ends before `deadline`.
          const timer = setTimeout(
            () =>
              reject(
                new Error(
                  `${event} did not fire within ${timeoutMs} ms${since === null ? '' : ' of the loss'}`
                )
              ),
            Math.ceil(deadline - start)
          )
          canvas.addEventListener(
            event,
            () => {
              clearTimeout(timer)
              resolve(performance.now())
            },
            { once: true }
          )
          extension[method]()
        }),
      change
    )
  }

  /**
   * Loses the context and resolves with when the canvas reported it. The wait
   * fails by name about twice the slowest loss step measured on the CI runner
   * (9.2 s) after the call, timed in the page: it bounds the event, not the
   * round trip to the page.
   */
  function loseContext(page: Page) {
    return changeContext(page, {
      method: 'loseContext',
      event: 'webglcontextlost',
      timeoutMs: 20_000,
      since: null,
    })
  }

  /**
   * Restores the context lost at `lostAt`. The wait fails by name once the
   * Atlas's restore deadline has passed since the loss, counting the runner's
   * steps between the loss and this call. The Atlas starts its own clock in
   * the loss event, before the listener that records `lostAt`, so by then
   * the Atlas has given the map up to the scene list.
   */
  function restoreContext(page: Page, lostAt: number) {
    return changeContext(page, {
      method: 'restoreContext',
      event: 'webglcontextrestored',
      timeoutMs: RESTORE_DEADLINE_MS,
      since: lostAt,
    })
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
    const lostAt = await loseContext(page)
    await page.mouse.move(dot.x + 120, dot.y + 120)
    await restoreContext(page, lostAt)
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
