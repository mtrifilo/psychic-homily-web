import { expect, type Page } from '@playwright/test'

/**
 * Shared /atlas fixtures and probes for browser specs. Scenes, venues and the
 * venue's shows are synthesized so the city, its pins and the bill are known
 * regardless of what the seed geocoded: one Phoenix scene, two rooms sharing
 * the city-centroid pin, one street-geocoded room whose single show bills one
 * artist.
 */

export const PHOENIX = { lat: 33.4484, lng: -112.074 }

/** The slice of the `__atlasMap` window seam these specs call. */
export type AtlasMapSeam = {
  isStyleLoaded: () => boolean
  areTilesLoaded: () => boolean
  getZoom: () => number
  getStyle: () => { layers: { id: string; source?: string }[] }
  queryRenderedFeatures: (opts: { layers: string[] }) => unknown[]
  project: (lngLat: [number, number]) => { x: number; y: number }
  getCanvas: () => HTMLCanvasElement
  jumpTo: (o: { center: [number, number]; zoom: number }) => void
}

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

/** Routes the scenes, venues and venue-shows reads to the Phoenix fixture. */
export async function stubAtlas(page: Page) {
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

/**
 * Waits until the style and visible tiles are loaded. `idle` and `load` never
 * fire on /atlas (its pulse ring repaints every frame), so they are not used.
 */
export async function waitForMap(page: Page) {
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

/** Rejects the cookie banner by a tap (the context needs `hasTouch`) and waits for it to leave. */
export async function dismissBanner(page: Page) {
  await page.getByRole('button', { name: 'Reject All' }).tap()
  await expect(page.getByRole('dialog', { name: 'Cookie consent' })).toHaveCount(0)
}

/** The Phoenix scene dot's position in the viewport, through the map's own projection. */
export function phoenixDotPoint(page: Page) {
  return page.evaluate(({ lng, lat }) => {
    const m = (window as unknown as { __atlasMap: AtlasMapSeam }).__atlasMap
    const p = m.project([lng, lat])
    const r = m.getCanvas().getBoundingClientRect()
    return { x: r.left + p.x, y: r.top + p.y }
  }, PHOENIX)
}

/** Moves the camera over Phoenix at `zoom`, synchronously. */
export async function jumpToPhoenix(page: Page, zoom: number) {
  await page.evaluate(
    ({ lng, lat, zoom }) => {
      const m = (window as unknown as { __atlasMap: AtlasMapSeam }).__atlasMap
      m.jumpTo({ center: [lng, lat], zoom })
    },
    { ...PHOENIX, zoom },
  )
}

/**
 * Where the map credit is and what covers it. The document is first scrolled
 * back to the top: a tap's actionability scroll can move the page (the footer
 * sits below the Atlas), which says nothing about what the layout covers.
 *
 * `covered` lists every sample point (three across, at three heights) whose
 * topmost element is not the credit; `clipped` is true when the text overflows
 * its box or the box leaves the viewport; `underSheet` names any sheet whose
 * top edge sits above the credit's bottom edge; `lines` counts the text's
 * rendered lines; `corner` is the MapLibre control corner that holds it.
 * Null when no credit control is mounted.
 */
export function creditReport(page: Page) {
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
          const testId = hit?.getAttribute('data-testid')
          const label = hit?.getAttribute('aria-label')
          const who = hit
            ? `${hit.tagName.toLowerCase()}${testId ? `[${testId}]` : ''}${label ? `(${label})` : ''}`
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
    // Line boxes grouped by their top edge: inline runs on one line share a
    // top to within a pixel or two, and a wrapped line starts a line-height
    // lower.
    const range = document.createRange()
    range.selectNodeContents(inner)
    const tops = Array.from(range.getClientRects())
      .filter((b) => b.width > 0 && b.height > 0)
      .map((b) => b.top)
      .sort((a, b) => a - b)
    let lines = 0
    let lineTop = -Infinity
    for (const top of tops) {
      if (top - lineTop > 4) {
        lines += 1
        lineTop = top
      }
    }
    const corner =
      ['top-left', 'top-right', 'bottom-left', 'bottom-right'].find((c) =>
        el.closest(`.maplibregl-ctrl-${c}`),
      ) ?? null
    const sheets = Array.from(
      document.querySelectorAll<HTMLElement>('[data-slot="bottom-sheet"]'),
    )
      .filter((s) => s.getBoundingClientRect().height > 0)
      .map((s) => ({
        id: s.getAttribute('data-testid') ?? 'sheet',
        top: Math.round(s.getBoundingClientRect().top),
      }))
    return {
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
      lines,
      corner,
      sheets,
      underSheet: sheets.filter((s) => s.top < r.bottom).map((s) => s.id),
    }
  })
}

/** Whether every sample point across the credit hits the credit itself. */
export async function creditUncovered(page: Page) {
  const report = await creditReport(page)
  return !!report && report.covered.length === 0
}
