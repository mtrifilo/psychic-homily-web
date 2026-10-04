import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

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
            // Two rooms at the city centroid share one pin.
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
              slug: 'sheet-night',
              title: 'Sheet Night',
              event_date: '2030-01-05T03:00:00Z',
              city: 'Phoenix',
              state: 'AZ',
              price: null,
              age_requirement: null,
              artists: [{ id: 901, slug: 'sheet-band', name: 'Sheet Band' }],
            },
          ],
        },
      }),
  )
}

async function waitForMap(page: Page) {
  await expect
    .poll(
      () =>
        page.evaluate(() => {
          const m = (
            window as unknown as {
              __atlasMap?: {
                isStyleLoaded: () => boolean
                areTilesLoaded: () => boolean
              } | null
            }
          ).__atlasMap
          return !!m && m.isStyleLoaded() && m.areTilesLoaded()
        }),
      { timeout: 60_000 },
    )
    .toBe(true)
}

/**
 * Whether three points across the credit all hit the credit itself. The
 * document is first scrolled back to the top: Playwright's actionability
 * scroll before a tap can move the page (the footer sits below the Atlas),
 * which says nothing about what the layout covers.
 */
function creditUncovered(page: Page) {
  return page.evaluate(() => {
    window.scrollTo(0, 0)
    const el = document.querySelector('.maplibregl-ctrl-attrib')
    if (!el) return false
    const r = el.getBoundingClientRect()
    const y = r.top + r.height / 2
    return [r.left + 4, r.left + r.width / 2, r.right - 4].every((x) => {
      const hit = document.elementFromPoint(x, y)
      return !!hit && el.contains(hit)
    })
  })
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
    await page.getByRole('combobox', { name: 'Search scenes' }).tap()
    await expect(page.getByPlaceholder('City or state…')).toBeVisible()
    expect(await creditUncovered(page)).toBe(true)
  })
})
