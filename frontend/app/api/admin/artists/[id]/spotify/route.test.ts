import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NextRequest } from 'next/server'
import { cookies } from 'next/headers'
import { revalidatePath } from 'next/cache'
import { POST, DELETE } from './route'

// Mock Sentry so capture calls are observable and never hit the network.
vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// Mock next/headers cookies(); each test sets the resolved cookie store.
vi.mock('next/headers', () => ({
  cookies: vi.fn(),
}))

// Mock next/cache so ISR revalidation calls are observable.
vi.mock('next/cache', () => ({
  revalidatePath: vi.fn(),
}))

const mockCookies = vi.mocked(cookies)
const mockRevalidatePath = vi.mocked(revalidatePath)

// BACKEND_URL is unset in the vitest env, so the route falls back to this.
const BACKEND = 'http://localhost:8080'

const ARTIST_ID = '47'
const VALID_SPOTIFY_URL = 'https://open.spotify.com/artist/0WThQFCFaU1YR5s0bNLvtP'

/**
 * Build a cookie store stub matching the subset of the next/headers
 * ReadonlyRequestCookies API the route uses: cookies().get('auth_token').
 */
function cookieStore(token?: string) {
  return {
    get: (name: string) =>
      name === 'auth_token' && token !== undefined
        ? { name: 'auth_token', value: token }
        : undefined,
  }
}

/** Resolve cookies() to a store with the given auth_token (or none). */
function setAuthToken(token?: string) {
  mockCookies.mockResolvedValue(
    cookieStore(token) as unknown as Awaited<ReturnType<typeof cookies>>
  )
}

/**
 * Route fetch traffic by URL: auth profile check and the backend PATCH.
 * (Spotify URL validation is regex-only — no network call.)
 */
function mockFetchRouting({
  isAdmin = true,
  backendResponse = new Response(
    JSON.stringify({ id: 47, slug: 'bright-eyes', name: 'Bright Eyes' }),
    { status: 200 }
  ),
}: {
  isAdmin?: boolean
  backendResponse?: Response
} = {}) {
  return vi
    .spyOn(globalThis, 'fetch')
    .mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === `${BACKEND}/auth/profile`) {
        return new Response(
          JSON.stringify({ success: true, user: { id: '1', is_admin: isAdmin } }),
          { status: 200 }
        )
      }
      if (url === `${BACKEND}/admin/artists/${ARTIST_ID}/spotify`) {
        return backendResponse
      }
      throw new Error(`Unexpected fetch in test: ${url}`)
    })
}

function postRequest(body: unknown) {
  return new NextRequest(
    `http://localhost:3000/api/admin/artists/${ARTIST_ID}/spotify`,
    { method: 'POST', body: JSON.stringify(body) }
  )
}

function deleteRequest() {
  return new NextRequest(
    `http://localhost:3000/api/admin/artists/${ARTIST_ID}/spotify`,
    { method: 'DELETE' }
  )
}

const params = { params: Promise.resolve({ id: ARTIST_ID }) }

let fetchSpy: ReturnType<typeof vi.spyOn> | undefined

beforeEach(() => {
  vi.clearAllMocks()
  setAuthToken('admin-token')
})

afterEach(() => {
  fetchSpy?.mockRestore()
  fetchSpy = undefined
})

describe('POST /api/admin/artists/[id]/spotify', () => {
  it('saves the URL and revalidates the artist ISR page', async () => {
    fetchSpy = mockFetchRouting()

    const res = await POST(postRequest({ spotify_url: VALID_SPOTIFY_URL }), params)

    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.success).toBe(true)
    expect(mockRevalidatePath).toHaveBeenCalledTimes(1)
    expect(mockRevalidatePath).toHaveBeenCalledWith('/artists/bright-eyes')
  })

  it('does NOT revalidate when the backend save fails', async () => {
    fetchSpy = mockFetchRouting({
      backendResponse: new Response(JSON.stringify({ detail: 'boom' }), {
        status: 500,
      }),
    })

    const res = await POST(postRequest({ spotify_url: VALID_SPOTIFY_URL }), params)

    expect(res.status).toBe(500)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('does NOT revalidate or hit the backend when the caller is not admin', async () => {
    fetchSpy = mockFetchRouting({ isAdmin: false })

    const res = await POST(postRequest({ spotify_url: VALID_SPOTIFY_URL }), params)

    expect(res.status).toBe(403)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
    const patchCalls = (fetchSpy.mock.calls as unknown[][]).filter((call) =>
      String(call[0]).startsWith(`${BACKEND}/admin/`)
    )
    expect(patchCalls).toHaveLength(0)
  })

  it('returns 401 without auth and never revalidates', async () => {
    setAuthToken(undefined)
    fetchSpy = mockFetchRouting()

    const res = await POST(postRequest({ spotify_url: VALID_SPOTIFY_URL }), params)

    expect(res.status).toBe(401)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('rejects a non-artist Spotify URL without revalidating', async () => {
    fetchSpy = mockFetchRouting()

    const res = await POST(
      postRequest({ spotify_url: 'https://open.spotify.com/track/abc123' }),
      params
    )

    expect(res.status).toBe(400)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })
})

describe('DELETE /api/admin/artists/[id]/spotify', () => {
  it('clears the URL and revalidates the artist ISR page', async () => {
    fetchSpy = mockFetchRouting()

    const res = await DELETE(deleteRequest(), params)

    expect(res.status).toBe(200)
    expect(mockRevalidatePath).toHaveBeenCalledTimes(1)
    expect(mockRevalidatePath).toHaveBeenCalledWith('/artists/bright-eyes')
  })

  it('does NOT revalidate when the backend clear fails', async () => {
    fetchSpy = mockFetchRouting({
      backendResponse: new Response(JSON.stringify({ detail: 'boom' }), {
        status: 500,
      }),
    })

    const res = await DELETE(deleteRequest(), params)

    expect(res.status).toBe(500)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })
})
