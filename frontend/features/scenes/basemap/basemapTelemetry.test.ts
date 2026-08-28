import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { ErrorEvent } from 'maplibre-gl'
import { PH_BASEMAP_SOURCE_ID } from './phBasemap'

/**
 * Guards for the Atlas basemap failure signal (PSY-1568).
 *
 * The two properties worth pinning are the ones that fail silently in
 * production: the THROTTLE (an outage is one error per tile per pan, so a
 * regression here turns one incident into hundreds of events and burns the
 * Sentry quota) and the FILTER (reporting unrelated MapLibre errors through
 * this path would make the signal unactionable, and reporting the GIBS raster
 * would quietly expand a scope that was deliberately deferred).
 *
 * `@sentry/nextjs` is globally mocked in test/setup.ts, so `captureMessage` is
 * already a spy here.
 */

// The module holds its once-per-session guard at module scope, which is the
// point of the design — so a "fresh session" in a test is a fresh module
// registry, not a reset function. Importing dynamically after
// `vi.resetModules()` exercises the real guard rather than a test-only seam.
// The two imports are awaited IN SEQUENCE, not with Promise.all: concurrent
// dynamic imports straight after a reset race the vite module runner, and the
// test can end up holding a different mock instance than the one the module
// under test bound to (silently zero recorded calls).
async function freshSession() {
  vi.resetModules()
  const telemetry = await import('./basemapTelemetry')
  const sentry = await import('@sentry/nextjs')
  return {
    handleBasemapError: telemetry.handleBasemapError,
    captureMessage: vi.mocked(sentry.captureMessage),
  }
}

/**
 * A MapLibre error event as the map actually receives it: MapLibre merges the
 * owning source's `sourceId` into the event as it bubbles from the source's
 * tile manager up to the map.
 */
function sourceErrorEvent(
  sourceId: string | undefined,
  error: unknown
): ErrorEvent {
  return { type: 'error', error, ...(sourceId ? { sourceId } : {}) } as unknown as ErrorEvent
}

/** MapLibre's AJAXError shape: an Error carrying the HTTP status and the URL. */
function ajaxError(status: number, url: string): Error {
  return Object.assign(new Error(`AJAXError: Server Error (${status}): ${url}`), {
    status,
    statusText: 'Server Error',
    url,
    name: 'AJAXError',
  })
}

const TILE_URL = 'https://tiles.openfreemap.org/planet/14/4207/6101.pbf'

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

describe('handleBasemapError', () => {
  it('reports the OpenFreeMap vector source once and stays quiet after that', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(429, TILE_URL))
    )

    expect(captureMessage).toHaveBeenCalledTimes(1)
  })

  it('reports again in a new session', async () => {
    const first = await freshSession()
    first.handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    expect(first.captureMessage).toHaveBeenCalledTimes(1)

    // `vi.resetModules()` resets the MODULE registry, not the mock's recorded
    // calls — the same spy instance is handed back — so the history is cleared
    // by hand to keep the next assertion about the second session alone.
    first.captureMessage.mockClear()

    const second = await freshSession()
    second.handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    expect(second.captureMessage).toHaveBeenCalledTimes(1)
  })

  it('attaches the source, host and status as searchable tags', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )

    expect(captureMessage).toHaveBeenCalledWith(
      'Atlas basemap tile source failed',
      expect.objectContaining({
        level: 'error',
        tags: expect.objectContaining({
          service: 'atlas-basemap',
          error_type: 'basemap_source_failed',
          basemap_source: PH_BASEMAP_SOURCE_ID,
          basemap_host: 'tiles.openfreemap.org',
          basemap_status: 503,
        }),
      })
    )
  })

  it('ships no full URL — not in the title, the tags, or the extras', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(500, TILE_URL))
    )

    // AJAXError puts the whole URL in its message; the event must not carry it
    // anywhere, however deeply nested.
    expect(JSON.stringify(captureMessage.mock.calls[0])).not.toContain(
      'https://'
    )
    const options = captureMessage.mock.calls[0][1] as {
      extra: { tilePath: string; errorMessage: string }
    }
    // toTelemetryPath placeholders the purely-numeric segments; `6101.pbf`
    // survives because it is not one. Both are public tile coordinates — the
    // load-bearing part is that the origin and any query string are gone.
    expect(options.extra.tilePath).toBe('/planet/:id/:id/6101.pbf')
    expect(options.extra.errorMessage).toContain('<url>')
  })

  it('still reports when MapLibre attaches no HTTP fields', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, new Error('style load failed'))
    )

    const options = captureMessage.mock.calls[0][1] as {
      tags: { basemap_status: string; basemap_host: string }
      extra: { tilePath: string | undefined }
    }
    // No status and no URL: the event degrades to the configured host and an
    // explicit "none" rather than throwing or silently dropping the signal.
    expect(options.tags.basemap_status).toBe('none')
    expect(options.tags.basemap_host).toBe('tiles.openfreemap.org')
    expect(options.extra.tilePath).toBeUndefined()
  })

  it('ignores errors from other sources and from the map itself', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    // The NASA GIBS raster is explicitly out of scope (a follow-up), and a
    // map-level error carries no sourceId at all.
    handleBasemapError(sourceErrorEvent('nightEarth', ajaxError(503, TILE_URL)))
    handleBasemapError(sourceErrorEvent('scenes', new Error('bad geojson')))
    handleBasemapError(sourceErrorEvent(undefined, new Error('WebGL context lost')))

    expect(captureMessage).not.toHaveBeenCalled()
  })

  it('restores the console.error MapLibre suppresses once a listener exists', async () => {
    const { handleBasemapError } = await freshSession()
    const error = new Error('WebGL context lost')

    handleBasemapError(sourceErrorEvent(undefined, error))

    // Attaching ANY error listener disables MapLibre's built-in console.error,
    // so every error — reported or not — must still reach the console.
    expect(console.error).toHaveBeenCalledWith(error)
  })

  it('never throws out of the handler when reporting fails', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()
    captureMessage.mockImplementationOnce(() => {
      throw new Error('sentry transport down')
    })

    // A throw here would surface inside MapLibre's event dispatch as an error
    // about the thing that was trying to report an error.
    expect(() =>
      handleBasemapError(
        sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
      )
    ).not.toThrow()
  })
})
