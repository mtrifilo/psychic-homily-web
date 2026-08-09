import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { proxy } from './proxy'
import { ARCHIVE_YEAR_RANGE } from '@/features/shows/showArchive'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

function mockHistogram(years: Array<{ year: number; count: number }>) {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ venue_id: 7, time_filter: 'past', years }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  )
}

function mockStatus(status: number) {
  return vi
    .spyOn(globalThis, 'fetch')
    .mockResolvedValue(new Response(null, { status }))
}

const ARCHIVE = '/venues/the-van-buren/shows'

/**
 * `/venues/<slug>/shows/<year>` sits one level below the entity-detail shape
 * the generic check handles, so it needs its own branch here or every bad year
 * soft-404s — a 404 BODY committed at HTTP 200 once the shell has streamed
 * (PSY-1756, same failure the scene periods hit in the PSY-897 arc).
 *
 * It matters more here than anywhere else in this file: the space is 8,100
 * in-range years per venue and only a handful of them are documents.
 */
describe('proxy — venue year archives', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('lets a year the venue has past shows in through', async () => {
    const fetchMock = mockHistogram([
      { year: 2025, count: 14 },
      { year: 2024, count: 60 },
    ])

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/venues/the-van-buren/shows/years?time_filter=past',
      { redirect: 'manual' }
    )
    expect(response.status).toBe(200)
  })

  it('turns a year with no past shows into a real 404', async () => {
    mockHistogram([{ year: 2025, count: 14 }])

    const response = await proxy(requestFor(`${ARCHIVE}/1999`))

    expect(response.status).toBe(404)
  })

  it('treats a zero-count year as absent', async () => {
    // The backend never emits one, but a year that reached the strip with no
    // rows behind it would render an empty archive at HTTP 200.
    mockHistogram([{ year: 1999, count: 0 }])

    const response = await proxy(requestFor(`${ARCHIVE}/1999`))

    expect(response.status).toBe(404)
  })

  it('404s a missing venue', async () => {
    mockStatus(404)

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    expect(response.status).toBe(404)
  })

  it.each(['20xx', '202', '20255', 'years', '2024a'])(
    '404s the malformed year %p without a round trip',
    async segment => {
      const fetchMock = mockHistogram([{ year: 2024, count: 60 }])

      const response = await proxy(requestFor(`${ARCHIVE}/${segment}`))

      expect(response.status).toBe(404)
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  it.each(['0000', '1899'])(
    '404s the out-of-range year %p without a round trip',
    async segment => {
      const fetchMock = mockHistogram([{ year: 2024, count: 60 }])

      const response = await proxy(requestFor(`${ARCHIVE}/${segment}`))

      expect(response.status).toBe(404)
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  /**
   * The proxy keeps its own copy of the window (it does not import `features/`,
   * matching the scenes and charts branches), so this is what stops the two
   * drifting into a soft-404 or a needless round trip.
   */
  it('applies the same year window the archive route accepts', () => {
    expect([ARCHIVE_YEAR_RANGE.min, ARCHIVE_YEAR_RANGE.max]).toEqual([1900, 9999])
  })

  it.each([500, 429, 403])(
    'fails OPEN on a backend %d rather than inventing a 404',
    async status => {
      mockStatus(status)

      const response = await proxy(requestFor(`${ARCHIVE}/2024`))

      expect(response.status).toBe(200)
    }
  )

  it('fails OPEN when the histogram body is an unrecognised shape', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ years: null }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    )

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    expect(response.status).toBe(200)
  })

  it('fails OPEN when the backend is unreachable', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'))

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    expect(response.status).toBe(200)
  })

  /** The venue page itself is untouched: it keeps the generic HEAD check. */
  it('leaves the bare venue detail on the generic existence check', async () => {
    const fetchMock = mockStatus(200)

    await proxy(requestFor('/venues/the-van-buren'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/venues/the-van-buren/exists',
      { method: 'HEAD', redirect: 'manual' }
    )
  })

  /** Anything else under a venue is somebody else's route; pass it through. */
  it('passes an unrelated venue sub-route through untouched', async () => {
    const fetchMock = mockStatus(200)

    const response = await proxy(requestFor('/venues/the-van-buren/edit/2024'))

    expect(response.status).toBe(200)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
