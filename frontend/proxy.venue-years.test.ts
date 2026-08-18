import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import {
  proxy,
  VENUE_ARCHIVE_MAX_YEAR,
  VENUE_ARCHIVE_MIN_YEAR,
} from './proxy'
import { ARCHIVE_YEAR_RANGE } from '@/features/shows/showArchive'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

/**
 * A response from the API itself. 404s carry `application/problem+json`, which
 * is what tells them apart from a 404 the router produced for a path it does not
 * know — see API_ERROR_CONTENT_TYPE in proxy.ts.
 *
 * The success status is 200, not 204: huma pins DefaultStatus to 200 for HEAD
 * before the body-less rule can apply, and the generated OpenAPI document says
 * so. Measured against the running backend.
 */
function mockStatus(status: number) {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(null, {
      status,
      headers:
        status === 404 ? { 'content-type': 'application/problem+json' } : {},
    })
  )
}

/** A 404 from something that is not the API — chi's default for an unknown path. */
function mockUnroutedNotFound() {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(null, {
      status: 404,
      headers: { 'content-type': 'text/plain; charset=utf-8' },
    })
  )
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
    const fetchMock = mockStatus(200)

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    // A HEAD on the year's own existence endpoint, like every other branch in
    // the file (PSY-1770). Before it, this was the one probe that had to GET a
    // list and read `total` out of the body, because the list answers 200 for
    // any venue that exists.
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/venues/the-van-buren/shows/2024/exists',
      expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
    )
    expect(response.status).toBe(200)
  })

  /**
   * No body is read, and that is a property worth asserting rather than
   * inferring from the method: a probe that parsed a response would reintroduce
   * the cost the HEAD removes, and HEAD responses have no body to parse.
   */
  it('settles the year on the status alone, never on a body', async () => {
    const body = vi.fn()
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      status: 204,
      ok: true,
      json: body,
      text: body,
    } as unknown as Response)

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    expect(response.status).toBe(200)
    expect(body).not.toHaveBeenCalled()
  })

  it('bounds the probe with a timeout so a slow backend cannot pin the edge', async () => {
    const fetchMock = mockStatus(200)

    await proxy(requestFor(`${ARCHIVE}/2024`))

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.signal).toBeInstanceOf(AbortSignal)
  })

  /**
   * The backend answers 404 for BOTH "no such venue" and "no shows that year",
   * and the proxy deliberately cannot tell them apart — a crawler walking the
   * 8,100-year space must not be able to either. Both are the same real 404.
   */
  it('turns a year with no past shows into a real 404', async () => {
    mockStatus(404)

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
      const fetchMock = mockStatus(200)

      const response = await proxy(requestFor(`${ARCHIVE}/${segment}`))

      expect(response.status).toBe(404)
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  it.each(['0000', '1899'])(
    '404s the out-of-range year %p without a round trip',
    async segment => {
      const fetchMock = mockStatus(200)

      const response = await proxy(requestFor(`${ARCHIVE}/${segment}`))

      expect(response.status).toBe(404)
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  /**
   * The proxy keeps its own copy of the window (it does not import `features/`,
   * matching the scenes and charts branches), so this is what stops the two
   * drifting into a soft-404 or a needless round trip.
   *
   * It compares the two COPIES, not each against a literal: narrowing the
   * proxy's minimum to the neighbouring `FIRST_TRACKED_YEAR` (2015) would hard
   * 404 every pre-2015 archive the sitemap still advertises, and a
   * literal-versus-literal assertion would stay green through it.
   */
  it('applies the same year window the archive route accepts', () => {
    expect(VENUE_ARCHIVE_MIN_YEAR).toBe(ARCHIVE_YEAR_RANGE.min)
    expect(VENUE_ARCHIVE_MAX_YEAR).toBe(ARCHIVE_YEAR_RANGE.max)
  })

  /** The lower boundary itself must pass through, not just fail to 404. */
  it('probes the backend for the earliest in-range year', async () => {
    const fetchMock = mockStatus(200)

    const response = await proxy(
      requestFor(`${ARCHIVE}/${ARCHIVE_YEAR_RANGE.min}`)
    )

    expect(fetchMock).toHaveBeenCalled()
    expect(response.status).toBe(200)
  })

  it.each([500, 429, 403])(
    'fails OPEN on a backend %d rather than inventing a 404',
    async status => {
      mockStatus(status)

      const response = await proxy(requestFor(`${ARCHIVE}/2024`))

      expect(response.status).toBe(200)
    }
  )

  /**
   * DEPLOY SKEW. A frontend live ahead of its backend probes a route the running
   * API does not carry; chi answers 404 as `text/plain`. Reading that as
   * "missing" would hard-404 every venue year archive — a sitemap-announced
   * family — for the length of the skew window. Only a 404 the API AUTHORED may
   * produce a real 404; this one has to fail open.
   */
  it('fails OPEN on a 404 the API did not author', async () => {
    mockUnroutedNotFound()

    const response = await proxy(requestFor(`${ARCHIVE}/2024`))

    expect(response.status).toBe(200)
  })

  /** A 404 with no content type at all is equally undecidable. */
  it('fails OPEN on a 404 carrying no content type', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(null, { status: 404 })
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
      expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
    )
  })

  /**
   * The retired `?year=` form. It shipped days earlier as a shareable address
   * (PSY-1753), so these links exist; without the redirect they resolve to the
   * unfiltered archive while the URL claims one year.
   */
  describe('the retired ?year= form', () => {
    it('308s to the year path, with no query left behind', async () => {
      const fetchMock = mockStatus(200)

      const response = await proxy(
        requestFor('/venues/the-van-buren?year=2025')
      )

      expect(response.status).toBe(308)
      expect(response.headers.get('location')).toBe(
        'http://localhost:3000/venues/the-van-buren/shows/2025'
      )
      // Settled locally: no existence probe is spent on a redirect.
      expect(fetchMock).not.toHaveBeenCalled()
    })

    it('carries ?page= across, since the page is a different axis', async () => {
      const response = await proxy(
        requestFor('/venues/the-van-buren?year=2025&page=3')
      )

      expect(response.headers.get('location')).toBe(
        'http://localhost:3000/venues/the-van-buren/shows/2025?page=3'
      )
    })

    it.each(['garbage', '999', '1899', ''])(
      'leaves ?year=%p alone and lets the venue page render',
      async year => {
        const fetchMock = mockStatus(200)

        const response = await proxy(
          requestFor(`/venues/the-van-buren?year=${year}`)
        )

        expect(response.status).toBe(200)
        // Fell through to the generic existence check rather than redirecting.
        expect(fetchMock).toHaveBeenCalled()
      }
    )

    it('does not touch a bare venue page', async () => {
      mockStatus(200)

      const response = await proxy(requestFor('/venues/the-van-buren'))

      expect(response.status).toBe(200)
    })
  })

  /** Anything else under a venue is somebody else's route; pass it through. */
  it('passes an unrelated venue sub-route through untouched', async () => {
    const fetchMock = mockStatus(200)

    const response = await proxy(requestFor('/venues/the-van-buren/edit/2024'))

    expect(response.status).toBe(200)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
