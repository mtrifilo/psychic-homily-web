import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

/**
 * The `/venues` mini Atlas pane (PSY-2079).
 *
 * Its own spec file rather than cases inside `venues.spec.ts`: MapLibre needs a
 * WebGL2 context, headless Chromium has none without the SwiftShader flags, and
 * Playwright only accepts `launchOptions` at file scope (it forces a new
 * worker). The rest of the directory's behaviour stays in `venues.spec.ts`.
 *
 * The seeded rooms carry no coordinates, so the rows are synthesized at the
 * response shape, which also pins the city's rooms somewhere predictable.
 */
test.use({
  launchOptions: {
    args: ['--enable-unsafe-swiftshader', '--use-angle=swiftshader'],
  },
})

const PHOENIX = '/venues?cities=Phoenix%2CAZ'

const ROOMS = [
  { id: 9001, name: 'Mapped Room One', lat: 33.4484, lng: -112.074, upcoming: 12 },
  { id: 9002, name: 'Mapped Room Two', lat: 33.4942, lng: -112.0299, upcoming: 3 },
  { id: 9003, name: 'Mapped Quiet Room', lat: 33.4152, lng: -111.9315, upcoming: 0 },
]

async function tableIsUp(page: Page) {
  await expect(page.getByRole('table')).toBeVisible({ timeout: 10_000 })
}

/**
 * A URL PREDICATE, not a glob: the list request is the one whose path ENDS at
 * `/venues`, which is what keeps `/venues/cities` on the real backend. The PAGE
 * is at that path too, so the document navigation has to fall through.
 */
async function stubRooms(page: Page) {
  await page.route(
    url => url.pathname.endsWith('/venues'),
    async route => {
      if (route.request().resourceType() === 'document') {
        return route.fallback()
      }
      await route.fulfill({
        json: {
          venues: ROOMS.map(room => ({
            id: room.id,
            slug: `mapped-room-${room.id}`,
            name: room.name,
            address: '1 Test St',
            city: 'Phoenix',
            state: 'AZ',
            timezone: 'America/Phoenix',
            verified: true,
            latitude: room.lat,
            longitude: room.lng,
            upcoming_show_count: room.upcoming,
            social: {},
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          })),
          total: ROOMS.length,
          limit: 50,
          offset: 0,
        },
      })
    }
  )
}

test.describe('Venues mini Atlas', () => {
  test('is beside the table at 1440 and draws a live map', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await stubRooms(page)
    await page.goto(PHOENIX)
    await tableIsUp(page)

    const pane = page.getByTestId('venue-mini-atlas')
    await expect(pane).toBeVisible()

    // The frame's box, reserved before the library lands.
    const box = await pane.boundingBox()
    expect(box?.width).toBe(368)

    // A real MapLibre instance, not an empty div: it paints its own canvas at
    // the box's size and renders the credit its license requires.
    const canvas = pane.locator('canvas.maplibregl-canvas')
    await expect(canvas).toBeVisible({ timeout: 30_000 })

    // The canvas fills the reserved box, and its BACKING STORE is the assertion
    // that matters: maplibre-gl.css restyles the container at map init, and a
    // container it collapses leaves the canvas at MapLibre's 300px default
    // while the box around it still measures right.
    const drawn = await canvas.evaluate((el: HTMLCanvasElement) => ({
      width: el.width / (window.devicePixelRatio || 1),
      height: el.height / (window.devicePixelRatio || 1),
    }))
    expect(drawn.width).toBeGreaterThan(350)
    expect(drawn.height).toBeGreaterThan(350)

    // The skeleton is handed over once the style has painted, not left on top.
    await expect(pane.locator('.animate-pulse')).toHaveCount(0, {
      timeout: 30_000,
    })

    // The ODbL credit is inside the pane, not running past its edge.
    const credit = pane.locator('.maplibregl-ctrl-attrib')
    await expect(credit).toContainText(/OpenStreetMap/i)
    const creditBox = await credit.boundingBox()
    expect(creditBox).not.toBeNull()
    expect(creditBox!.x).toBeGreaterThanOrEqual(box!.x - 1)
    expect(creditBox!.x + creditBox!.width).toBeLessThanOrEqual(
      box!.x + box!.width + 1
    )
  })

  test('links into the Atlas opened on this city', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await stubRooms(page)
    await page.goto(PHOENIX)
    await tableIsUp(page)

    await expect(page.getByTestId('venue-mini-atlas-open')).toHaveAttribute(
      'href',
      '/atlas?city=Phoenix%2CAZ'
    )
  })

  test('is absent at 1279, and the rows carry nothing for it', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1279, height: 900 })
    await stubRooms(page)
    await page.goto(PHOENIX)
    await tableIsUp(page)

    await expect(page.getByTestId('venue-mini-atlas')).toHaveCount(0)
    await expect(page.locator('canvas.maplibregl-canvas')).toHaveCount(0)
    await expect(page.locator('tbody tr[data-venue-row]')).toHaveCount(0)
  })

  test('the zoom controls a keyboard can reach carry their own names', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await stubRooms(page)
    await page.goto(PHOENIX)
    await tableIsUp(page)

    const pane = page.getByTestId('venue-mini-atlas')
    await expect(pane.locator('canvas.maplibregl-canvas')).toBeVisible({
      timeout: 30_000,
    })

    // The canvas is hidden from assistive tech and cannot be tabbed into, so
    // MapLibre's zoom buttons are the pane's whole keyboard surface. They have
    // to say what they do.
    await expect(pane.locator('canvas.maplibregl-canvas')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
    await expect(pane.locator('canvas.maplibregl-canvas')).toHaveAttribute(
      'tabindex',
      '-1'
    )
    const zoomButtons = pane.locator('button.maplibregl-ctrl-zoom-in, button.maplibregl-ctrl-zoom-out')
    await expect(zoomButtons).toHaveCount(2)
    for (const name of [/zoom in/i, /zoom out/i]) {
      await expect(pane.getByRole('button', { name })).toBeVisible()
    }
  })

  test('a row hover marks that room and no other', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await stubRooms(page)
    await page.goto(PHOENIX)
    await tableIsUp(page)
    await expect(page.getByTestId('venue-mini-atlas')).toBeVisible()

    const rows = page.locator('tbody tr[data-venue-row]')
    await rows.first().hover()

    await expect(rows.first()).toHaveClass(/outline-primary/)
    await expect(rows.nth(1)).not.toHaveClass(/outline-primary/)
  })
})
