import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import {
  isAddressableShowsYear,
  isRealShowsCalendarDay,
  SHOWS_CALENDAR_DAY_SEGMENT,
  SHOWS_CALENDAR_MAX_YEAR,
  SHOWS_CALENDAR_MIN_YEAR,
  SHOWS_CALENDAR_MONTH_SEGMENT,
} from './proxy'
import {
  parseDaySegments,
  parseMonthSegments,
  SHOWS_CALENDAR_MAX_YEAR as ROUTE_MAX_YEAR,
  SHOWS_CALENDAR_MIN_YEAR as ROUTE_MIN_YEAR,
} from '@/features/shows/showsCalendarRoute'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

const RANGE_URL = 'http://localhost:8080/shows/calendar/range'

/**
 * A month relative to the CURRENT one, as the endpoint spells an edge.
 *
 * Every span below is built from the clock rather than written down, for the
 * reason the backend suite gives about its own fixtures: the proxy refuses a
 * span that does not hold today, so a span pinned to literal months would start
 * failing on a date nobody chose.
 */
function monthFromNow(delta: number): { year: number; month: number } {
  const now = new Date()
  const at = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + delta, 1))
  return { year: at.getUTCFullYear(), month: at.getUTCMonth() + 1 }
}

/** The dated path of a month relative to this one, with an optional day. */
function pathFromNow(delta: number, day?: number): string {
  const { year, month } = monthFromNow(delta)
  const base = `/shows/${year}/${String(month).padStart(2, '0')}`
  return day === undefined ? base : `${base}/${String(day).padStart(2, '0')}`
}

/** The span most cases below are read against: last month through four out. */
const SPAN = {
  first_month: monthFromNow(-1),
  last_month: monthFromNow(4),
}

function rangeResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

/**
 * A proxy whose span cache is empty.
 *
 * The cache is module state that lives for the life of the instance, which is
 * the whole point of it in production and a shared fixture between test cases.
 * Re-importing per case is what keeps one case's span out of the next one's.
 */
async function freshProxy(): Promise<(request: NextRequest) => Promise<Response>> {
  vi.resetModules()
  const proxyModule = await import('./proxy')
  return proxyModule.proxy as unknown as (
    request: NextRequest
  ) => Promise<Response>
}

/**
 * `/shows/{yyyy}/{mm}` and `/shows/{yyyy}/{mm}/{dd}` sit one level below the
 * entity-detail shape the generic check handles, so they need their own branch
 * here or every malformed date soft-404s, a 404 BODY committed at HTTP 200
 * once the shell has streamed (the PSY-897 arc, the same failure the scene
 * periods and the venue year archives hit).
 *
 * SHAPE is decided here without a backend. MEMBERSHIP is decided against the
 * addressable span: a month outside it is a real 404, and a month inside it
 * with nothing on is the list's quiet state at 200.
 */
describe('proxy, shows month and day routes', () => {
  let fetchMock: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    fetchMock = vi.spyOn(globalThis, 'fetch')
    fetchMock.mockResolvedValue(rangeResponse(SPAN))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('passes a month inside the addressable span', async () => {
    const proxy = await freshProxy()

    const response = await proxy(requestFor(pathFromNow(1)))

    expect(response.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledWith(RANGE_URL, expect.anything())
  })

  it('passes a day inside the addressable span', async () => {
    const proxy = await freshProxy()

    const response = await proxy(requestFor(pathFromNow(1, 14)))

    expect(response.status).toBe(200)
  })

  it('404s a month before the span and a month after it', async () => {
    const proxy = await freshProxy()

    expect((await proxy(requestFor(pathFromNow(-2)))).status).toBe(404)
    expect((await proxy(requestFor(pathFromNow(5)))).status).toBe(404)
  })

  /** A day is addressable exactly when its month is. */
  it('404s a day whose month is outside the span', async () => {
    const proxy = await freshProxy()

    expect((await proxy(requestFor(pathFromNow(-2, 28)))).status).toBe(404)
    expect((await proxy(requestFor(pathFromNow(5, 1)))).status).toBe(404)
  })

  /**
   * The edges are INCLUSIVE. The first edge is the current month, which is
   * where every Tonight link lands, and the last is the month holding the last
   * upcoming show.
   */
  it('passes both edge months', async () => {
    const proxy = await freshProxy()

    expect((await proxy(requestFor(pathFromNow(-1)))).status).toBe(200)
    expect((await proxy(requestFor(pathFromNow(4)))).status).toBe(200)
  })

  /**
   * A run keeps its own address. The proxy passes it through rather than
   * rewriting, so the day route still reads the span it was asked for.
   */
  it('leaves a run parameter untouched on the way through', async () => {
    const proxy = await freshProxy()
    const run = `${pathFromNow(1, 11)}?days=3`
    const request = {
      nextUrl: new URL(`http://localhost:3000${run}`),
      url: `http://localhost:3000${run}`,
    } as unknown as NextRequest

    const response = await proxy(request)

    expect(response.status).toBe(200)
    expect(response.headers.get('x-middleware-rewrite')).toBeNull()
  })

  /**
   * One probe per instance, not one per URL. A crawler walking a family of
   * dated URLs would otherwise spend a backend call on each, against an
   * anonymous per-IP budget every reader behind one address shares.
   */
  it('reads the span once and reuses it', async () => {
    const proxy = await freshProxy()

    await Promise.all([
      proxy(requestFor(pathFromNow(1))),
      proxy(requestFor(pathFromNow(2))),
      proxy(requestFor(pathFromNow(3, 2))),
    ])
    await proxy(requestFor(pathFromNow(4)))

    const rangeCalls = fetchMock.mock.calls.filter(
      (call: unknown[]) => call[0] === RANGE_URL
    )
    expect(rangeCalls).toHaveLength(1)
  })

  /**
   * FAIL OPEN, on every answer that is not a span. A 404 produced from a span
   * nobody answered for would take out every dated URL at once, the ones the
   * site's own chips link included; a quiet month rendered at 200 is transient.
   *
   * The 404 row is the deploy skew: the frontend goes live before the backend
   * carries the route, and chi answers an unrouted path with a 404.
   */
  it.each([
    ['a 5xx', () => Promise.resolve(rangeResponse({}, 500))],
    ['a 404 from an API that does not carry the route yet', () =>
      Promise.resolve(new Response('404 page not found', { status: 404 }))],
    ['a 429', () => Promise.resolve(rangeResponse({}, 429))],
    ['a network error', () => Promise.reject(new Error('ECONNREFUSED'))],
    ['a body that is not a span', () => Promise.resolve(rangeResponse({ months: [] }))],
    ['an inverted span', () =>
      Promise.resolve(
        rangeResponse({ first_month: monthFromNow(4), last_month: monthFromNow(-1) })
      )],
    ['a span that does not hold today', () =>
      Promise.resolve(
        rangeResponse({ first_month: monthFromNow(6), last_month: monthFromNow(9) })
      )],
    ['an edge outside the addressable years', () =>
      Promise.resolve(
        rangeResponse({ first_month: { year: 0, month: 1 }, last_month: monthFromNow(4) })
      )],
  ])('serves the page when the probe answers %s', async (_label, answer) => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    fetchMock.mockImplementation(() => answer() as Promise<Response>)
    const proxy = await freshProxy()

    // A month that would be 404ed under the span above.
    expect((await proxy(requestFor(pathFromNow(-2)))).status).toBe(200)
  })

  /**
   * A refresh that fails keeps the span it had, rather than falling open on
   * every dated URL at the moment the backend is least able to serve them: a
   * rendered window costs three backend reads where a 404 costs none.
   */
  it('keeps the last good span when a later probe fails', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.useFakeTimers()
    try {
      const proxy = await freshProxy()
      expect((await proxy(requestFor(pathFromNow(-2)))).status).toBe(404)

      fetchMock.mockResolvedValue(rangeResponse({}, 503))
      vi.advanceTimersByTime(300_001)

      expect((await proxy(requestFor(pathFromNow(-2)))).status).toBe(404)
      expect((await proxy(requestFor(pathFromNow(1)))).status).toBe(200)
      // The refresh must actually have been attempted, or the assertions above
      // would be reading the first probe's entry and proving nothing.
      const rangeCalls = fetchMock.mock.calls.filter(
        (call: unknown[]) => call[0] === RANGE_URL
      )
      expect(rangeCalls.length).toBeGreaterThan(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('passes a leap day in a leap year and 404s one in a common year', async () => {
    fetchMock.mockResolvedValue(
      rangeResponse({ first_month: monthFromNow(-1), last_month: { year: 2028, month: 3 } })
    )
    const proxy = await freshProxy()

    expect((await proxy(requestFor('/shows/2028/02/29'))).status).toBe(200)
    expect((await proxy(requestFor('/shows/2027/02/29'))).status).toBe(404)
  })

  /**
   * The year bound is the crawl bound, and the two copies of it have to agree
   * or one side 404s a URL the other serves.
   */
  it('keeps the proxy and route year bounds in lockstep', () => {
    expect(SHOWS_CALENDAR_MIN_YEAR).toBe(ROUTE_MIN_YEAR)
    expect(SHOWS_CALENDAR_MAX_YEAR).toBe(ROUTE_MAX_YEAR)
  })

  it.each([
    '/shows/2026/9',
    '/shows/0026/11',
    '/shows/0000/01',
    '/shows/1999/11',
    '/shows/2101/11',
    '/shows/9999/11',
    '/shows/2026/13',
    '/shows/2026/00',
    '/shows/26/11',
    '/shows/not-a-year/11',
    '/shows/2026/november',
    '/shows/2026/11/32',
    '/shows/2026/11/00',
    '/shows/2026/11/4',
    '/shows/2026/11/31',
    '/shows/some-show-slug/edit',
  ])('404s %s before anything renders', async pathname => {
    const proxy = await freshProxy()

    const response = await proxy(requestFor(pathname))

    expect(response.status).toBe(404)
    // Shape is settled without a backend, which is what keeps the larger half
    // of the crawlable space off the probe entirely.
    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * The show detail page's file-convention OG card reaches the same
   * four-segment shape a month does. A static segment outranks a dynamic one in
   * the router, so the card is what actually renders, and this branch has to
   * agree with the router rather than 404 it as a malformed month.
   */
  it('leaves the show OG card route alone', async () => {
    const proxy = await freshProxy()

    const response = await proxy(
      requestFor('/shows/2026-03-20-a-show/opengraph-image')
    )

    expect(response.status).toBe(200)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * The detail shape is untouched by this branch: `/shows/<slug>` is three
   * segments and still goes to the existence probe.
   */
  it('still existence-checks the show detail shape', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 200 }))
    const proxy = await freshProxy()

    await proxy(requestFor('/shows/2026-03-20-a-show'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/shows/2026-03-20-a-show/exists',
      expect.objectContaining({ method: 'HEAD' })
    )
  })

  it('still lets the reserved static sub-routes through', async () => {
    const proxy = await freshProxy()

    expect((await proxy(requestFor('/shows/submit'))).status).toBe(200)
    expect((await proxy(requestFor('/shows/saved'))).status).toBe(200)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * The proxy keeps its OWN copy of the segment shapes because it must not
   * import `features/`, the constraint the scenes, charts and venue-year
   * branches work under too. This is what keeps the two copies honest: the
   * proxy's verdict and the route's parse must agree on every input, or a URL
   * the proxy waves through renders a not-found at HTTP 200 (or worse, a URL
   * the route would serve is hard-404ed before it renders).
   */
  it('accepts exactly the segments the route grammar accepts', () => {
    const years = [
      '2026',
      '2000',
      '2100',
      '1999',
      '2101',
      '0000',
      '0026',
      '9999',
      '26',
      '20261',
      'abcd',
      '',
      ' 2026',
    ]
    const months = ['01', '09', '11', '12', '00', '13', '1', '001', 'ab', '']
    const days = ['01', '09', '28', '31', '00', '32', '1', 'xx', '']

    for (const year of years) {
      for (const month of months) {
        const proxySaysMonth =
          isAddressableShowsYear(year) &&
          SHOWS_CALENDAR_MONTH_SEGMENT.test(month)
        expect(
          proxySaysMonth,
          `month shape disagreement on /shows/${year}/${month}`
        ).toBe(parseMonthSegments(year, month) !== null)

        for (const day of days) {
          const proxySaysDay =
            proxySaysMonth &&
            SHOWS_CALENDAR_DAY_SEGMENT.test(day) &&
            isRealShowsCalendarDay(year, month, day)
          expect(
            proxySaysDay,
            `day shape disagreement on /shows/${year}/${month}/${day}`
          ).toBe(parseDaySegments(year, month, day) !== null)
        }
      }
    }
  })
})
