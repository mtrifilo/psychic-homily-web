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
// The two imports are awaited IN SEQUENCE. Observed with `Promise.all`: the
// second and later `freshSession()` calls in a file saw zero captures, because
// the concurrent import resolved the basemapTelemetry module from cache rather
// than re-instantiating it — so `reportedSources` still held the previous
// test's entry and every call returned early at the throttle. The mock
// instance is NOT the variable here (vitest keeps factory mocks across
// `resetModules`, see the note further down); the module under test is.
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

/**
 * MapLibre's AJAXError shape: an Error carrying the HTTP status and the URL.
 * Deliberately does NOT set `name` — MapLibre's AJAXError never does either,
 * and a fixture that invents one would let the module claim triage value it
 * does not have in production.
 */
function ajaxError(status: number, url: string): Error {
  return Object.assign(new Error(`AJAXError: Server Error (${status}): ${url}`), {
    status,
    statusText: 'Server Error',
    url,
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
    // anywhere, however deeply nested. Asserted on the HOST, not on
    // `'https://'`: a scrub that replaces the scheme but stops early at a
    // query-legal character leaves the credential-bearing tail behind, and a
    // scheme assertion goes green on exactly that leak.
    const serialized = JSON.stringify(captureMessage.mock.calls[0])
    // The bare hostname survives as a deliberate tag; the URL must not. Both
    // halves are checked: the scheme (so nothing ships a whole URL) AND the
    // path (so a scrub that stopped early cannot go green on the tail alone).
    expect(serialized).not.toContain('://')
    expect(serialized).not.toContain('/planet/')
    const options = captureMessage.mock.calls[0][1] as {
      extra: { errorMessage: string }
    }
    expect(options.extra.errorMessage).not.toContain(TILE_URL)
    expect(options.extra.errorMessage).toContain('<url>')
  })

  it('caps the message it ships', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()
    const long = Object.assign(new Error('y'.repeat(500)), {
      status: 500,
      url: TILE_URL,
    })

    handleBasemapError(sourceErrorEvent(PH_BASEMAP_SOURCE_ID, long))

    const options = captureMessage.mock.calls[0][1] as {
      extra: { errorMessage: string }
    }
    expect(options.extra.errorMessage.length).toBeLessThanOrEqual(203)
  })

  it('still reports when MapLibre attaches no HTTP fields', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, new Error('style load failed'))
    )

    const options = captureMessage.mock.calls[0][1] as {
      tags: { basemap_status: string | number; basemap_host: string }
      extra: { tilePath: string | undefined }
    }
    // No status and no URL: the event degrades to the configured host and an
    // explicit "none" rather than throwing or silently dropping the signal.
    expect(options.tags.basemap_status).toBe('none')
    expect(options.tags.basemap_host).toBe('tiles.openfreemap.org')
    expect(options.extra.tilePath).toBeUndefined()
  })

  it('tags the event with the basemap source id', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )

    const options = captureMessage.mock.calls[0][1] as {
      tags: { basemap_source: string }
    }
    expect(options.tags.basemap_source).toBe(PH_BASEMAP_SOURCE_ID)

    // NOT covered, and deliberately not pretended otherwise: that the throttle
    // and the tag read the EVENT's sourceId rather than the module constant.
    // Past the filter the two are provably equal, so no test here can tell
    // them apart -- the property only becomes observable once a second source
    // is in scope. Whoever adds the GIBS raster must extract the accepted-id
    // set and add a test that two sources get two independent throttle slots;
    // keying on the constant would let the first failure silence the other.
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

  it('never hands the raw error to console.error when Sentry is collecting', async () => {
    vi.stubEnv('NEXT_PUBLIC_SENTRY_DSN', 'https://k@sentry.example.test/1')
    try {
      const { handleBasemapError } = await freshSession()

      handleBasemapError(
        sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
      )

      // This is the load-bearing privacy property, and it is NOT about the
      // console. Sentry's console breadcrumb keeps the raw `data.arguments`
      // alongside its joined message, and normalizeEvent expands an Error
      // argument into its own properties -- so passing the AJAXError object
      // here would ship `.url` and the unscrubbed `.message` on the very
      // event captured a line later, whatever the breadcrumb message said.
      const logged = vi.mocked(console.error).mock.calls[0]
      expect(logged).toHaveLength(1)
      expect(typeof logged[0]).toBe('string')
      expect(logged[0]).not.toContain('tiles.openfreemap.org')
      expect(logged[0]).toContain('<url>')
    } finally {
      vi.unstubAllEnvs()
    }
  })

  it('logs the raw error when no DSN is set, where the stack is read', async () => {
    const { handleBasemapError } = await freshSession()
    const error = ajaxError(503, TILE_URL)

    handleBasemapError(sourceErrorEvent(PH_BASEMAP_SOURCE_ID, error))

    // No DSN means no breadcrumb ships, and the object carries the stack.
    expect(console.error).toHaveBeenCalledWith(error)
  })

  it('falls back to the error itself when the message is empty', async () => {
    vi.stubEnv('NEXT_PUBLIC_SENTRY_DSN', 'https://k@sentry.example.test/1')
    try {
      const { handleBasemapError } = await freshSession()
      // MapLibre's XHR path rejects with `new Error(xhr.statusText)`, and
      // statusText is '' on a network error. A nullish-only fallback would
      // log a blank line, having already discarded the type and stack.
      const blank = Object.assign(new Error(''), { status: 0 })

      handleBasemapError(sourceErrorEvent(PH_BASEMAP_SOURCE_ID, blank))

      const logged = vi.mocked(console.error).mock.calls[0][0] as string
      expect(logged).not.toBe('')
      expect(logged).toContain('Error')
    } finally {
      vi.unstubAllEnvs()
    }
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

  it('stays quiet when the browser knows it is offline', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()
    const onLine = vi
      .spyOn(navigator, 'onLine', 'get')
      .mockReturnValue(false)

    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(0, TILE_URL))
    )
    expect(captureMessage).not.toHaveBeenCalled()

    // ...and the slot is NOT spent, so a real outage once connectivity is back
    // still reports. Otherwise one tunnel would silence the whole session.
    onLine.mockReturnValue(true)
    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    expect(captureMessage).toHaveBeenCalledTimes(1)

    // Restored explicitly: the config sets no `restoreMocks`, and the global
    // afterEach only clears call history, so a getter spy left installed here
    // would leak a mocked navigator into every test that follows.
    onLine.mockRestore()
  })

  it('scrubs a whole URL whose query contains commas, brackets or semicolons', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()
    // Every one of these characters is LEGAL in a query string. A terminator
    // set that treats them as the end of the URL leaves the tail — including
    // anything credential-shaped — in the shipped message.
    const nasty =
      'https://tiles.example.com/x?BBOX=1,2,3,4&filter[]=a;jsessionid=SECRET123'
    const error = Object.assign(
      new Error(`AJAXError: Not Found (404): ${nasty}`),
      { status: 404, url: nasty }
    )

    handleBasemapError(sourceErrorEvent(PH_BASEMAP_SOURCE_ID, error))

    const serialized = JSON.stringify(captureMessage.mock.calls[0])
    // The bare HOSTNAME is a deliberate tag (low cardinality, and the whole
    // triage question) -- what must never ship is the path and query.
    expect(serialized).not.toContain('SECRET123')
    expect(serialized).not.toContain('jsessionid')
    expect(serialized).not.toContain('BBOX')
    expect(serialized).not.toContain('filter')
    expect(serialized).not.toContain('://')

    const options = captureMessage.mock.calls[0][1] as {
      extra: { errorMessage: string }
    }
    expect(options.extra.errorMessage).toBe(
      'AJAXError: Not Found (404): <url>'
    )
  })

  it('keeps a URL embedded in a braced or quoted token from eating the rest', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()
    const jsonish = Object.assign(
      new Error(`{"url":"${TILE_URL}","status":503,"note":"keep me"}`),
      { status: 503, url: TILE_URL }
    )

    handleBasemapError(sourceErrorEvent(PH_BASEMAP_SOURCE_ID, jsonish))

    const options = captureMessage.mock.calls[0][1] as {
      extra: { errorMessage: string }
    }
    // A terminator of "anything up to whitespace" would have collapsed this to
    // `{"url":"<url>`, destroying the status and note alongside the URL.
    expect(options.extra.errorMessage).not.toContain('https://')
    expect(options.extra.errorMessage).toContain('keep me')
    expect(options.extra.errorMessage).toContain('503')
  })

  it('does not burn the one report slot on a send that threw', async () => {
    const { handleBasemapError, captureMessage } = await freshSession()
    captureMessage.mockImplementationOnce(() => {
      throw new Error('sentry transport down')
    })

    // A failed send must not count as "reported". Stamping the throttle before
    // the capture would let one transport hiccup silence the source for the
    // whole session, so the outage would never surface -- exactly the silent
    // degradation this module exists to catch.
    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )

    expect(captureMessage).toHaveBeenCalledTimes(2)

    // ...and once one lands, the throttle engages as normal.
    handleBasemapError(
      sourceErrorEvent(PH_BASEMAP_SOURCE_ID, ajaxError(503, TILE_URL))
    )
    expect(captureMessage).toHaveBeenCalledTimes(2)
  })
})
