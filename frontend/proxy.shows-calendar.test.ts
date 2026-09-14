import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import {
  isAddressableShowsYear,
  isRealShowsCalendarDay,
  proxy,
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

/**
 * `/shows/{yyyy}/{mm}` and `/shows/{yyyy}/{mm}/{dd}` sit one level below the
 * entity-detail shape the generic check handles, so they need their own branch
 * here or every malformed date soft-404s — a 404 BODY committed at HTTP 200
 * once the shell has streamed (the PSY-897 arc, the same failure the scene
 * periods and the venue year archives hit).
 *
 * The branch is SHAPE-only by decision. Whether a well-formed month HAS shows
 * is a question about the upcoming partition under the reader's own filters,
 * and the route answers it from the month histogram it already reads.
 */
describe('proxy — shows month and day routes', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  /**
   * The load-bearing property: no backend round trip on these paths at all.
   * A probe per request would be two backend calls for every month page, on
   * the site's busiest prefix, to answer a question the page re-asks anyway.
   */
  it('passes a well-formed month through without touching the backend', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')

    const response = await proxy(requestFor('/shows/2026/11'))

    expect(fetchMock).not.toHaveBeenCalled()
    expect(response.status).toBe(200)
  })

  it('passes a well-formed day through without touching the backend', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')

    const response = await proxy(requestFor('/shows/2026/11/14'))

    expect(fetchMock).not.toHaveBeenCalled()
    expect(response.status).toBe(200)
  })

  it('passes a leap day in a leap year and 404s one in a common year', async () => {
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
    const fetchMock = vi.spyOn(globalThis, 'fetch')

    const response = await proxy(requestFor(pathname))

    expect(response.status).toBe(404)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * The show detail page's file-convention OG card reaches the same
   * four-segment shape a month does. A static segment outranks a dynamic one in
   * the router, so the card is what actually renders — and this branch has to
   * agree with the router rather than 404 it as a malformed month.
   */
  it('leaves the show OG card route alone', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')

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
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await proxy(requestFor('/shows/2026-03-20-a-show'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/shows/2026-03-20-a-show/exists',
      expect.objectContaining({ method: 'HEAD' })
    )
  })

  it('still lets the reserved static sub-routes through', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')

    expect((await proxy(requestFor('/shows/submit'))).status).toBe(200)
    expect((await proxy(requestFor('/shows/saved'))).status).toBe(200)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * The proxy keeps its OWN copy of the segment shapes because it must not
   * import `features/` — the constraint the scenes, charts and venue-year
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
