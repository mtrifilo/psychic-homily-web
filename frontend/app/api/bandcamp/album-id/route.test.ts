import { describe, it, expect, vi, afterEach } from 'vitest'
import { NextRequest } from 'next/server'
import { GET } from './route'

// Sentry is a no-op in tests; mock so the failure paths don't reach the SDK.
vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

let fetchSpy: ReturnType<typeof vi.spyOn> | undefined

afterEach(() => {
  fetchSpy?.mockRestore()
  fetchSpy = undefined
  vi.clearAllMocks()
})

function getRequest(url?: string) {
  const target = url
    ? `http://localhost:3000/api/bandcamp/album-id?url=${encodeURIComponent(url)}`
    : 'http://localhost:3000/api/bandcamp/album-id'
  return new NextRequest(target)
}

describe('GET /api/bandcamp/album-id', () => {
  it('returns { kind, id } for a resolvable track URL', async () => {
    const trackPage =
      '<div data-embed="{&quot;tralbum_param&quot;:{&quot;name&quot;:&quot;track&quot;,&quot;value&quot;:777}}"></div>'
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(trackPage, { status: 200 }))

    const res = await GET(getRequest('https://x.bandcamp.com/track/song'))

    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ kind: 'track', id: '777' })
  })

  it('returns 400 when the url param is missing', async () => {
    const res = await GET(getRequest())
    expect(res.status).toBe(400)
  })

  it('returns 400 for a non-Bandcamp URL', async () => {
    const res = await GET(getRequest('https://example.com/album/x'))
    expect(res.status).toBe(400)
  })

  it('blocks an SSRF substring-bypass URL with 400 and no outbound fetch', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch')
    const res = await GET(
      getRequest('http://169.254.169.254/latest/meta-data/?x=bandcamp.com')
    )
    expect(res.status).toBe(400)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('maps an unresolvable URL (both path types 404) to a 404', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response('nope', { status: 404 }))

    const res = await GET(getRequest('https://x.bandcamp.com/album/ghost'))

    expect(res.status).toBe(404)
  })

  it('maps a page with no embed id to a 404', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response('<html>nothing</html>', { status: 200 }))

    const res = await GET(getRequest('https://x.bandcamp.com/album/empty'))

    expect(res.status).toBe(404)
  })
})
