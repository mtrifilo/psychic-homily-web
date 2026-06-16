import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { cookies } from 'next/headers'
import { revalidateArtistDetail } from '@/lib/revalidate-entity'
import { requireAdmin, forwardArtistMusicUpdate } from './admin-artist-route'

vi.mock('next/headers', () => ({ cookies: vi.fn() }))
vi.mock('@/lib/revalidate-entity', () => ({ revalidateArtistDetail: vi.fn() }))
vi.mock('@sentry/nextjs', () => ({ captureException: vi.fn() }))

const mockCookies = vi.mocked(cookies)
const mockRevalidate = vi.mocked(revalidateArtistDetail)
const BACKEND = 'http://localhost:8080'

function setCookie(token?: string) {
  mockCookies.mockResolvedValue({
    get: (name: string) =>
      name === 'auth_token' && token ? { name, value: token } : undefined,
  } as unknown as Awaited<ReturnType<typeof cookies>>)
}

function profileResponse(isAdmin: boolean) {
  return new Response(
    JSON.stringify({ success: true, user: { id: '1', is_admin: isAdmin } }),
    { status: 200 }
  )
}

let fetchSpy: ReturnType<typeof vi.spyOn> | undefined

beforeEach(() => vi.clearAllMocks())
afterEach(() => {
  fetchSpy?.mockRestore()
  fetchSpy = undefined
})

describe('requireAdmin', () => {
  it('401 Authentication required when no auth_token cookie', async () => {
    setCookie(undefined)
    const result = await requireAdmin()
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(401)
      expect(await result.response.json()).toEqual({
        error: 'Authentication required',
      })
    }
  })

  it('403 Admin access required when the profile is not admin', async () => {
    setCookie('token')
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(profileResponse(false))
    const result = await requireAdmin()
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(403)
      expect(await result.response.json()).toEqual({
        error: 'Admin access required',
      })
    }
  })

  it('403 when the profile fetch fails (treated as unauthenticated)', async () => {
    setCookie('token')
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response('nope', { status: 500 }))
    const result = await requireAdmin()
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.response.status).toBe(403)
  })

  it('returns the validated session token for an admin', async () => {
    setCookie('admin-token')
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(profileResponse(true))
    const result = await requireAdmin()
    expect(result).toEqual({ ok: true, authToken: 'admin-token' })
  })
})

describe('forwardArtistMusicUpdate', () => {
  const base = {
    artistId: '47',
    authToken: 'tok',
    field: 'bandcamp' as const,
    body: { bandcamp_embed_url: 'https://x.bandcamp.com/album/y' },
    sentryService: 'admin-bandcamp',
    sentryOperation: 'update',
    failureMessage: 'Failed to update artist',
  }

  it('PATCHes the field path with the auth cookie, revalidates, returns success', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        new Response(JSON.stringify({ id: 47, slug: 'bright-eyes' }), {
          status: 200,
        })
      )

    const res = await forwardArtistMusicUpdate(base)

    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({
      success: true,
      artist: { id: 47, slug: 'bright-eyes' },
    })
    expect(mockRevalidate).toHaveBeenCalledWith('bright-eyes')
    const [url, init] = fetchSpy.mock.calls[0]
    expect(String(url)).toBe(`${BACKEND}/admin/artists/47/bandcamp`)
    expect(init).toMatchObject({
      method: 'PATCH',
      headers: { Cookie: 'auth_token=tok' },
    })
  })

  it('passes through the backend detail + status on a non-2xx and does not revalidate', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        new Response(JSON.stringify({ detail: 'boom' }), { status: 422 })
      )

    const res = await forwardArtistMusicUpdate(base)

    expect(res.status).toBe(422)
    expect(await res.json()).toEqual({ error: 'boom' })
    expect(mockRevalidate).not.toHaveBeenCalled()
  })

  it('maps a thrown fetch to a 500 with the failure message', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network'))

    const res = await forwardArtistMusicUpdate(base)

    expect(res.status).toBe(500)
    expect(await res.json()).toEqual({ error: 'Failed to update artist' })
  })
})
