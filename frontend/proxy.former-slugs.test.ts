import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { clearExistenceVerdictsForTests, proxy } from './proxy'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

/**
 * A probe answer as the API sends it: a 200 that names `current` in
 * `Canonical-Slug` when the request named a former slug (no header for a live
 * slug), or a 404 carrying the API's error content type.
 */
function mockProbe(answer: { status: 200; current?: string } | { status: 404 }) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async () =>
    answer.status === 404
      ? new Response(null, {
          status: 404,
          headers: { 'content-type': 'application/problem+json' },
        })
      : new Response(null, {
          status: 200,
          headers:
            answer.current === undefined
              ? {}
              : { 'canonical-slug': answer.current },
        })
  )
}

function locationOf(response: Response): string {
  const url = new URL(response.headers.get('location') ?? '')
  return `${url.pathname}${url.search}`
}

const FAMILIES = ['shows', 'venues', 'artists'] as const

/**
 * A show, venue or artist URL under a former slug (one the entity no longer
 * answers at, after a slug change or a merge) answers one permanent redirect
 * to the entity's current URL. A live slug renders as it always has, and a
 * slug that never existed is a real 404.
 */
describe('proxy: former entity slugs', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    clearExistenceVerdictsForTests()
  })

  describe.each(FAMILIES)('/%s', family => {
    it('redirects a former slug to the current URL in one hop', async () => {
      const fetchMock = mockProbe({ status: 200, current: 'new-slug' })

      const response = await proxy(requestFor(`/${family}/old-slug`))

      expect(fetchMock).toHaveBeenCalledWith(
        `http://localhost:8080/entities/${family}/old-slug/exists`,
        expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
      )
      expect(response.status).toBe(308)
      expect(locationOf(response)).toBe(`/${family}/new-slug`)
      expect(response.headers.get('link')).toBe(
        `<https://psychichomily.com/${family}/new-slug>; rel="canonical"`
      )
      expect(response.headers.get('cache-control')).toBe('private, max-age=3600')
    })

    it('keeps the query string on the redirect', async () => {
      mockProbe({ status: 200, current: 'new-slug' })

      const response = await proxy(requestFor(`/${family}/old-slug?utm_source=x&a=1`))

      expect(response.status).toBe(308)
      expect(locationOf(response)).toBe(`/${family}/new-slug?utm_source=x&a=1`)
    })

    it('asks again on every request for a former slug', async () => {
      const fetchMock = mockProbe({ status: 200, current: 'new-slug' })

      expect((await proxy(requestFor(`/${family}/old-slug`))).status).toBe(308)
      expect((await proxy(requestFor(`/${family}/old-slug`))).status).toBe(308)

      expect(fetchMock).toHaveBeenCalledTimes(2)
    })

    it('renders a live slug and remembers it', async () => {
      const fetchMock = mockProbe({ status: 200 })

      const first = await proxy(requestFor(`/${family}/live-slug`))
      const second = await proxy(requestFor(`/${family}/live-slug`))

      expect(first.status).toBe(200)
      expect(first.headers.get('location')).toBeNull()
      expect(second.status).toBe(200)
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })

    it('404s a slug that never existed', async () => {
      mockProbe({ status: 404 })

      const response = await proxy(requestFor(`/${family}/never-existed`))

      expect(response.status).toBe(404)
      expect(response.headers.get('location')).toBeNull()
    })

    // The header is read only when it is a slug the target URL can carry as a
    // slug: digits alone would be read back as an id, and anything else could
    // leave the path.
    it.each([
      ['digits alone', '12345'],
      ['an uppercase slug', 'New-Slug'],
      ['a path', 'a/b'],
      ['a query', 'a?b=1'],
      ['an empty value', ''],
      ['an oversized slug', `a${'b'.repeat(255)}`],
    ])('does not redirect to %s', async (_label, current) => {
      const fetchMock = mockProbe({ status: 200, current })

      const response = await proxy(requestFor(`/${family}/old-slug`))

      expect(response.status).toBe(200)
      expect(response.headers.get('location')).toBeNull()
      // Such an answer names no path: the request renders where it is, and the
      // verdict is reused only for the short unnamed-verdict window.
      await proxy(requestFor(`/${family}/old-slug`))
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })

    it('fails open when the backend is unreachable', async () => {
      vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'))

      const response = await proxy(requestFor(`/${family}/old-slug`))

      expect(response.status).toBe(200)
      expect(response.headers.get('location')).toBeNull()
    })
  })

  // The other generic families' probes send no header, so they render and are
  // remembered exactly as before.
  it.each(['releases', 'labels', 'festivals', 'tags'])(
    'renders and remembers a /%s slug the probe names no slug for',
    async family => {
      const fetchMock = mockProbe({ status: 200 })

      const first = await proxy(requestFor(`/${family}/live-slug`))
      await proxy(requestFor(`/${family}/live-slug`))

      expect(first.status).toBe(200)
      expect(first.headers.get('location')).toBeNull()
      expect(fetchMock).toHaveBeenCalledTimes(1)
    }
  )

  describe('sub-pages', () => {
    it('redirects a former venue slug year archive to the same year', async () => {
      const fetchMock = mockProbe({ status: 200, current: 'new-room' })

      const response = await proxy(requestFor('/venues/old-room/shows/2024?page=2'))

      expect(fetchMock).toHaveBeenCalledWith(
        'http://localhost:8080/venues/old-room/shows/2024/exists',
        expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
      )
      expect(response.status).toBe(308)
      expect(locationOf(response)).toBe('/venues/new-room/shows/2024?page=2')
      expect(response.headers.get('link')).toBe(
        '<https://psychichomily.com/venues/new-room/shows/2024>; rel="canonical"'
      )
    })

    it.each([
      ['digits alone', '12345'],
      ['a path', 'a/b'],
    ])('does not redirect a year archive to %s', async (_label, current) => {
      mockProbe({ status: 200, current })

      const response = await proxy(requestFor('/venues/old-room/shows/2024'))

      expect(response.status).toBe(200)
      expect(response.headers.get('location')).toBeNull()
    })

    it('renders a live venue slug year archive', async () => {
      mockProbe({ status: 200 })

      const response = await proxy(requestFor('/venues/live-room/shows/2024'))

      expect(response.status).toBe(200)
      expect(response.headers.get('location')).toBeNull()
    })

    it('404s a year archive under a slug that never existed', async () => {
      mockProbe({ status: 404 })

      const response = await proxy(requestFor('/venues/never-existed/shows/2024'))

      expect(response.status).toBe(404)
    })

    // A show's OG card is not probed: the page advertises only its current
    // card URL, and the card renders its fallback for an unknown show.
    it('passes a show OG card through without a probe', async () => {
      const fetchMock = mockProbe({ status: 200, current: 'new-show' })

      const response = await proxy(requestFor('/shows/old-show/opengraph-image'))

      expect(response.status).toBe(200)
      expect(fetchMock).not.toHaveBeenCalled()
    })
  })
})
