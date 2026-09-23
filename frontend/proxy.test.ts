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
