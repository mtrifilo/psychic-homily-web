import { existsSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { clearExistenceVerdictsForTests, proxy, RESERVED_SEGMENTS } from './proxy'

function requestFor(pathname: string, cookies: string[] = []): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
    cookies: { has: (name: string) => cookies.includes(name) },
  } as unknown as NextRequest
}

function passedThrough(response: Response): boolean {
  return response.headers.get('x-middleware-next') === '1'
}

/** An answer from the API itself; its 404s carry the API's error content type. */
function mockStatus(status: number) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(
    async () =>
      new Response(null, {
        status,
        headers: status === 404 ? { 'content-type': 'application/problem+json' } : {},
      })
  )
}

/** The router's 404 for a path it has no route for, as an API without the probe sends. */
function mockUnroutedNotFound() {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(
    async () =>
      new Response(null, {
        status: 404,
        headers: { 'content-type': 'text/plain; charset=utf-8' },
      })
  )
}

const API = 'http://localhost:8080'

afterEach(() => {
  vi.restoreAllMocks()
  clearExistenceVerdictsForTests()
})

describe('proxy: radio routes', () => {
  // Each page route and the one probe that answers for it.
  const probes: [page: string, probe: string][] = [
    ['/radio/kexp', `${API}/radio-stations/kexp/exists`],
    ['/radio/kexp/the-morning-show', `${API}/radio-stations/kexp/radio-shows/the-morning-show/exists`],
    [
      '/radio/kexp/the-morning-show/2025-01-15',
      `${API}/radio-stations/kexp/radio-shows/the-morning-show/episodes/2025-01-15/exists`,
    ],
    ['/radio/wfmu/channel/drummer', `${API}/radio-stations/wfmu-drummer/exists`],
    ['/radio/wfmu/channel/wfmu-drummer', `${API}/radio-stations/wfmu-drummer/exists`],
    // `channel` one level up is the show route, with `channel` as the show.
    ['/radio/kexp/channel', `${API}/radio-stations/kexp/radio-shows/channel/exists`],
    // Below a static radio page the router resolves the station and show
    // routes, so these are probed like any other station.
    ['/radio/playlists/x', `${API}/radio-stations/playlists/radio-shows/x/exists`],
    ['/radio/new-releases/x', `${API}/radio-stations/new-releases/radio-shows/x/exists`],
  ]

  it.each(probes)('probes %s with one HEAD to %s', async (page, probe) => {
    const fetchMock = mockStatus(200)

    const response = await proxy(requestFor(page))

    expect(passedThrough(response)).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(
      probe,
      expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
    )
  })

  it.each(probes)('answers a real 404 for %s when the API says it is missing', async page => {
    mockStatus(404)

    expect((await proxy(requestFor(page))).status).toBe(404)
  })

  // During a deploy the frontend can reach an API without these routes. Its
  // router 404s the probe as text/plain, which must not 404 every radio page.
  it.each(probes)('renders %s when the API has no such probe route', async page => {
    mockUnroutedNotFound()

    expect(passedThrough(await proxy(requestFor(page)))).toBe(true)
  })

  it.each(probes)('renders %s when the backend errors or is unreachable', async page => {
    mockStatus(503)
    expect(passedThrough(await proxy(requestFor(page)))).toBe(true)
    clearExistenceVerdictsForTests()

    mockStatus(429)
    expect(passedThrough(await proxy(requestFor(page)))).toBe(true)
    clearExistenceVerdictsForTests()

    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'))
    expect(passedThrough(await proxy(requestFor(page)))).toBe(true)
  })

  it.each(['/radio/new-releases', '/radio/playlists', '/radio'])(
    'leaves %s to its static page with no probe',
    async page => {
      const fetchMock = mockStatus(404)

      expect(passedThrough(await proxy(requestFor(page)))).toBe(true)
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  it.each([
    '/radio/kexp/the-morning-show/2025-02-30',
    '/radio/kexp/the-morning-show/2025-1-15',
    '/radio/kexp/the-morning-show/latest',
  ])('404s %s, which names no day, with no probe', async page => {
    const fetchMock = mockStatus(200)

    expect((await proxy(requestFor(page))).status).toBe(404)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('leaves a path deeper than any radio route to the router', async () => {
    const fetchMock = mockStatus(200)

    expect(passedThrough(await proxy(requestFor('/radio/kexp/the-morning-show/2025-01-15/x')))).toBe(
      true
    )
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('reuses a positive verdict, and asks again after a negative one', async () => {
    const fetchMock = mockStatus(200)
    await proxy(requestFor('/radio/kexp/the-morning-show'))
    await proxy(requestFor('/radio/kexp/the-morning-show'))
    expect(fetchMock).toHaveBeenCalledTimes(1)

    fetchMock.mockRestore()
    const missing = mockStatus(404)
    await proxy(requestFor('/radio/kexp/no-such-show'))
    await proxy(requestFor('/radio/kexp/no-such-show'))
    expect(missing).toHaveBeenCalledTimes(2)
  })

  // Only the exact `/radio/<segment>` address is the static page, so the
  // registry must name every static directory under app/radio and nothing else.
  it('reserves exactly the static pages under app/radio', () => {
    const appRadio = join(__dirname, 'app', 'radio')
    const staticPages = readdirSync(appRadio, { withFileTypes: true })
      .filter(entry => entry.isDirectory() && !/^[[_(]/.test(entry.name))
      .filter(entry => existsSync(join(appRadio, entry.name, 'page.tsx')))
      .map(entry => entry.name)
      .sort()

    expect([...RESERVED_SEGMENTS.radio].sort()).toEqual(staticPages)
  })
})

describe('proxy: collections', () => {
  const PAGE = '/collections/essential-shoegaze'
  const PROBE = `${API}/collections/essential-shoegaze/exists`

  it('probes a request without a session with one HEAD', async () => {
    const fetchMock = mockStatus(200)

    expect(passedThrough(await proxy(requestFor(PAGE)))).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith(PROBE, expect.objectContaining({ method: 'HEAD' }))
  })

  // The API answers a private collection exactly as an unused slug, so both
  // are this one case.
  it('answers a real 404 when the API says the collection is not served', async () => {
    mockStatus(404)

    expect((await proxy(requestFor(PAGE))).status).toBe(404)
  })

  it('sends no session to the probe', async () => {
    const fetchMock = mockStatus(200)

    await proxy(requestFor(PAGE, ['theme']))

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(JSON.stringify(init.headers ?? {})).not.toMatch(/cookie|auth_token/i)
  })

  // The creator of a private collection must reach it; the probe cannot ask
  // as them, so a request with a session is the page's to answer.
  it('does not probe a request that carries a session', async () => {
    const fetchMock = mockStatus(404)

    expect(passedThrough(await proxy(requestFor(PAGE, ['auth_token'])))).toBe(true)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('renders the page when the API has no such probe route, errors or is unreachable', async () => {
    mockUnroutedNotFound()
    expect(passedThrough(await proxy(requestFor(PAGE)))).toBe(true)

    mockStatus(503)
    expect(passedThrough(await proxy(requestFor(PAGE)))).toBe(true)

    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'))
    expect(passedThrough(await proxy(requestFor(PAGE)))).toBe(true)
  })

  it('leaves the list page and deeper paths to the router', async () => {
    const fetchMock = mockStatus(404)

    expect(passedThrough(await proxy(requestFor('/collections')))).toBe(true)
    expect(passedThrough(await proxy(requestFor(`${PAGE}/x`)))).toBe(true)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
