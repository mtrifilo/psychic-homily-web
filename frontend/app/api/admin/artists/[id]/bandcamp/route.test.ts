import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NextRequest } from 'next/server'
import { cookies } from 'next/headers'
import { revalidatePath } from 'next/cache'
import { POST, DELETE } from './route'


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
const VALID_BANDCAMP_URL = 'https://brighteyes.bandcamp.com/album/kids-table'

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
 * Route fetch traffic by URL: auth profile check, Bandcamp URL validation,
 * and the backend PATCH each get their own response.
 */
function mockFetchRouting({
  isAdmin = true,
  bandcampPageOk = true,
  backendResponse = new Response(
    JSON.stringify({ id: 47, slug: 'bright-eyes', name: 'Bright Eyes' }),
    { status: 200 }
  ),
}: {
  isAdmin?: boolean
  bandcampPageOk?: boolean
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
      if (url.includes('bandcamp.com') && !url.startsWith(BACKEND)) {
        return bandcampPageOk
          ? new Response('<html>album=12345</html>', { status: 200 })
          : new Response('not found', { status: 404 })
      }
      if (url === `${BACKEND}/admin/artists/${ARTIST_ID}/bandcamp`) {
        return backendResponse
      }
      throw new Error(`Unexpected fetch in test: ${url}`)
    })
}

function postRequest(body: unknown) {
  return new NextRequest(
    `http://localhost:3000/api/admin/artists/${ARTIST_ID}/bandcamp`,
    { method: 'POST', body: JSON.stringify(body) }
  )
}

function deleteRequest() {
  return new NextRequest(
    `http://localhost:3000/api/admin/artists/${ARTIST_ID}/bandcamp`,
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

describe('POST /api/admin/artists/[id]/bandcamp', () => {
  it('saves the URL and revalidates the artist ISR page', async () => {
    fetchSpy = mockFetchRouting()

    const res = await POST(postRequest({ bandcamp_url: VALID_BANDCAMP_URL }), params)

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

    const res = await POST(postRequest({ bandcamp_url: VALID_BANDCAMP_URL }), params)

    expect(res.status).toBe(500)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('does NOT revalidate or hit the backend when the caller is not admin', async () => {
    fetchSpy = mockFetchRouting({ isAdmin: false })

    const res = await POST(postRequest({ bandcamp_url: VALID_BANDCAMP_URL }), params)

    expect(res.status).toBe(403)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
    // Only the auth profile check should have fired — never the backend PATCH.
    const patchCalls = (fetchSpy.mock.calls as unknown[][]).filter((call) =>
      String(call[0]).startsWith(`${BACKEND}/admin/`)
    )
    expect(patchCalls).toHaveLength(0)
  })

  it('returns 401 without auth and never revalidates', async () => {
    setAuthToken(undefined)
    fetchSpy = mockFetchRouting()

    const res = await POST(postRequest({ bandcamp_url: VALID_BANDCAMP_URL }), params)

    expect(res.status).toBe(401)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('skips revalidation when the backend response has no slug', async () => {
    fetchSpy = mockFetchRouting({
      backendResponse: new Response(
        JSON.stringify({ id: 47, name: 'Bright Eyes' }),
        { status: 200 }
      ),
    })

    const res = await POST(postRequest({ bandcamp_url: VALID_BANDCAMP_URL }), params)

    expect(res.status).toBe(200)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })
})

describe('DELETE /api/admin/artists/[id]/bandcamp', () => {
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
