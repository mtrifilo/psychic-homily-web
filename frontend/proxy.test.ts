import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { proxy } from './proxy'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

describe('proxy entity existence checks', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses the lightweight HEAD exists probe instead of the full detail GET', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await proxy(requestFor('/shows/e2e-attendance-test'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/shows/e2e-attendance-test/exists',
      // objectContaining, not an exact init: every probe also carries an
      // AbortSignal now, and pinning the whole object would make adding one
      // more fetch option a test failure in three files.
      expect.objectContaining({
        method: 'HEAD',
        redirect: 'manual',
      })
    )
  })

  it('rewrites backend 404 probes to a real not-found response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(null, { status: 404 })
    )

    const response = await proxy(requestFor('/tags/missing-tag'))

    expect(response.status).toBe(404)
  })

  it('does not existence-check reserved static show routes', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await proxy(requestFor('/shows/submit'))
    await proxy(requestFor('/shows/saved'))

    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('proxy numeric show ids', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  function showBody(slug: unknown): Response {
    return new Response(JSON.stringify({ id: 1359, slug }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  }

  function passedThrough(response: Response): boolean {
    return response.headers.get('x-middleware-next') === '1'
  }

  it('reads the show by id instead of the HEAD probe', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(showBody('white-denim-at-moth-club'))

    await proxy(requestFor('/shows/1359'))

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/shows/1359',
      expect.objectContaining({ redirect: 'manual' })
    )
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.method ?? 'GET').toBe('GET')
  })

  it('permanently redirects to the slug URL, keeping the query', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      showBody('white-denim-at-moth-club')
    )

    const response = await proxy(requestFor('/shows/1359?utm_source=news'))

    expect(response.status).toBe(308)
    expect(response.headers.get('location')).toBe(
      'http://localhost:3000/shows/white-denim-at-moth-club?utm_source=news'
    )
  })

  it('bounds how long a browser may reuse the redirect', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      showBody('white-denim-at-moth-club')
    )

    const response = await proxy(requestFor('/shows/1359'))

    expect(response.headers.get('cache-control')).toBe('private, max-age=3600')
  })

  it('leaves a year-shaped id on the HEAD probe with no redirect', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    const response = await proxy(requestFor('/shows/2026'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/shows/2026/exists',
      expect.objectContaining({ method: 'HEAD' })
    )
    expect(passedThrough(response)).toBe(true)
  })

  it('redirects an id just outside the calendar year window', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () =>
      showBody('white-denim-at-moth-club')
    )

    expect((await proxy(requestFor('/shows/1999'))).status).toBe(308)
    expect((await proxy(requestFor('/shows/2101'))).status).toBe(308)
  })

  it('releases the error body before answering', async () => {
    const errorBody = new Response('{"title":"Not Found"}', { status: 404 })
    const cancel = vi.spyOn(errorBody.body!, 'cancel')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(errorBody)

    const response = await proxy(requestFor('/shows/999999'))

    expect(response.status).toBe(404)
    expect(cancel).toHaveBeenCalled()
  })

  it('leaves a slug request on the HEAD probe with no redirect', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    const response = await proxy(requestFor('/shows/white-denim-at-moth-club'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/shows/white-denim-at-moth-club/exists',
      expect.objectContaining({ method: 'HEAD' })
    )
    expect(passedThrough(response)).toBe(true)
  })

  it('renders the page at the numeric URL for a show with no addressable slug', async () => {
    for (const slug of [null, '', '2325', 42]) {
      vi.spyOn(globalThis, 'fetch').mockResolvedValue(showBody(slug))
      const response = await proxy(requestFor('/shows/1359'))
      expect(passedThrough(response)).toBe(true)
    }
  })

  it('answers a real 404 for an id the backend does not have', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(null, { status: 404 })
    )

    const response = await proxy(requestFor('/shows/999999'))

    expect(response.status).toBe(404)
  })

  it('fails open on a backend error, a bad body or a network failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(null, { status: 503 })
    )
    expect(passedThrough(await proxy(requestFor('/shows/1359')))).toBe(true)

    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response('not json', { status: 200 })
    )
    expect(passedThrough(await proxy(requestFor('/shows/1359')))).toBe(true)

    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('ECONNRESET'))
    expect(passedThrough(await proxy(requestFor('/shows/1359')))).toBe(true)
  })

  it('passes the share card under a numeric id through with no read', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    const response = await proxy(requestFor('/shows/1359/opengraph-image'))

    expect(fetchMock).not.toHaveBeenCalled()
    expect(passedThrough(response)).toBe(true)
  })

  it('leaves numeric segments under other entities on their own probe', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await proxy(requestFor('/venues/42'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/venues/42/exists',
      expect.objectContaining({ method: 'HEAD' })
    )
  })
})

describe('proxy radio sub-stream redirect', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  function stationBody(network: unknown): Response {
    return new Response(JSON.stringify({ name: 'Station', network }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  }

  const SUB_STREAM = { slug: 'wfmu', name: 'WFMU', is_flagship: false }
  const FLAGSHIP = { slug: 'wfmu', name: 'WFMU', is_flagship: true }

  function passedThrough(response: Response): boolean {
    return response.headers.get('x-middleware-next') === '1'
  }

  it('reads the station with the page URL and cache window', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(stationBody(null))

    await proxy(requestFor('/radio/kexp'))

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/radio-stations/kexp',
      expect.objectContaining({ next: { revalidate: 3600 } })
    )
    // Next keys a cached fetch on `redirect` and `method` among others; the
    // page sets neither, so setting one here would split the shared entry.
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.redirect).toBeUndefined()
    expect(init.method).toBeUndefined()
  })

  it('permanently redirects a sub-stream to its channel URL', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(stationBody(SUB_STREAM))

    const response = await proxy(requestFor('/radio/wfmu-drummer'))

    expect(response.status).toBe(308)
    expect(response.headers.get('location')).toBe(
      'http://localhost:3000/radio/wfmu/channel/drummer'
    )
    expect(response.headers.get('cache-control')).toBe('private, max-age=3600')
  })

  it('keeps the query string across the redirect', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(stationBody(SUB_STREAM))

    const response = await proxy(
      requestFor('/radio/wfmu-drummer?utm_source=news&a=1')
    )

    expect(response.headers.get('location')).toBe(
      'http://localhost:3000/radio/wfmu/channel/drummer?utm_source=news&a=1'
    )
  })

  it('uses a slug without the network prefix whole', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(stationBody(SUB_STREAM))

    const response = await proxy(requestFor('/radio/legacy-show'))

    expect(response.headers.get('location')).toBe(
      'http://localhost:3000/radio/wfmu/channel/legacy-show'
    )
  })

  it('passes a flagship or network-less station through to the page', async () => {
    for (const network of [FLAGSHIP, null]) {
      vi.spyOn(globalThis, 'fetch').mockResolvedValue(stationBody(network))
      expect(passedThrough(await proxy(requestFor('/radio/wfmu')))).toBe(true)
    }
  })

  it('passes an unknown station through to the page rather than a 404', async () => {
    const errorBody = new Response('{"title":"Not Found"}', { status: 404 })
    const cancel = vi.spyOn(errorBody.body!, 'cancel')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(errorBody)

    const response = await proxy(requestFor('/radio/no-such-station'))

    expect(passedThrough(response)).toBe(true)
    expect(cancel).toHaveBeenCalled()
  })

  it('passes through on a backend error, a bad body or a network failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(null, { status: 503 })
    )
    expect(passedThrough(await proxy(requestFor('/radio/wfmu-drummer')))).toBe(
      true
    )

    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response('not json', { status: 200 })
    )
    expect(passedThrough(await proxy(requestFor('/radio/wfmu-drummer')))).toBe(
      true
    )

    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('ECONNRESET'))
    expect(passedThrough(await proxy(requestFor('/radio/wfmu-drummer')))).toBe(
      true
    )
  })

  it('passes a malformed network through to the page', async () => {
    for (const network of [
      { slug: 'wfmu' },
      { slug: '', is_flagship: false },
      { slug: 7, is_flagship: false },
      'wfmu',
    ]) {
      vi.spyOn(globalThis, 'fetch').mockResolvedValue(stationBody(network))
      expect(
        passedThrough(await proxy(requestFor('/radio/wfmu-drummer')))
      ).toBe(true)
    }
  })

  it('never reads a station for the hub, its static routes or deeper routes', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(stationBody(SUB_STREAM))

    for (const path of [
      '/radio',
      '/radio/playlists',
      '/radio/new-releases',
      '/radio/wfmu/channel/drummer',
      '/radio/kexp/the-morning-show',
      '/radio/kexp/the-morning-show/2025-01-15',
    ]) {
      expect(passedThrough(await proxy(requestFor(path)))).toBe(true)
    }
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
