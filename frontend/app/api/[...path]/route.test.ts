import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NextRequest } from 'next/server'
import * as Sentry from '@sentry/nextjs'
import { GET, POST, PUT, DELETE, PATCH, OPTIONS } from './route'

// Mock Sentry so capture calls are observable and never hit the network.
vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// Mock next/headers cookies(); each test sets the resolved cookie store.
vi.mock('next/headers', () => ({
  cookies: vi.fn(),
}))

import { cookies } from 'next/headers'

const mockCookies = vi.mocked(cookies)

// BACKEND_URL is unset in the vitest env, so the route falls back to this.
const BACKEND = 'http://localhost:8080'

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
  // The route only reads `.get`; cast through unknown to the mock signature.
  mockCookies.mockResolvedValue(
    cookieStore(token) as unknown as Awaited<ReturnType<typeof cookies>>
  )
}

let fetchSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  vi.clearAllMocks()
  setAuthToken() // default: no auth cookie
  fetchSpy = vi.spyOn(globalThis, 'fetch')
})

afterEach(() => {
  fetchSpy.mockRestore()
})

describe('api/[...path] proxy route', () => {
  describe('GET forwarding', () => {
    it('forwards the path and querystring to the backend', async () => {
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200 }))

      const req = new NextRequest(
        'http://localhost:3000/api/shows?city=phoenix&page=2'
      )
      const res = await GET(req)

      expect(res.status).toBe(200)
      expect(fetchSpy).toHaveBeenCalledTimes(1)
      const [calledUrl, init] = fetchSpy.mock.calls[0]
      expect(calledUrl).toBe(`${BACKEND}/shows?city=phoenix&page=2`)
      expect(init?.method).toBe('GET')
      // GET must not forward a body.
      expect(init?.body).toBeUndefined()
    })

    it('injects Cookie: auth_token from cookies() when present', async () => {
      setAuthToken('abc123')
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200 }))

      const req = new NextRequest('http://localhost:3000/api/me')
      await GET(req)

      const init = fetchSpy.mock.calls[0][1]
      const headers = init?.headers as Record<string, string>
      expect(headers['Cookie']).toBe('auth_token=abc123')
    })

    it('omits the Cookie header when no auth_token is present', async () => {
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200 }))

      const req = new NextRequest('http://localhost:3000/api/me')
      await GET(req)

      const init = fetchSpy.mock.calls[0][1]
      const headers = init?.headers as Record<string, string>
      expect(headers['Cookie']).toBeUndefined()
    })

    it('forwards the request Content-Type header', async () => {
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200 }))

      const req = new NextRequest('http://localhost:3000/api/me', {
        headers: { 'content-type': 'application/json' },
      })
      await GET(req)

      const init = fetchSpy.mock.calls[0][1]
      const headers = init?.headers as Record<string, string>
      expect(headers['Content-Type']).toBe('application/json')
    })

    it('returns the backend body and Content-Type to the client', async () => {
      fetchSpy.mockResolvedValue(
        new Response('{"hello":"world"}', {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      )

      const req = new NextRequest('http://localhost:3000/api/me')
      const res = await GET(req)

      expect(res.headers.get('Content-Type')).toBe('application/json')
      expect(await res.text()).toBe('{"hello":"world"}')
    })
  })

  describe('non-GET forwarding', () => {
    it('forwards the body for POST', async () => {
      fetchSpy.mockResolvedValue(new Response('created', { status: 201 }))

      const body = JSON.stringify({ name: 'New Show' })
      const req = new NextRequest('http://localhost:3000/api/shows', {
        method: 'POST',
        body,
        headers: { 'content-type': 'application/json' },
      })
      const res = await POST(req)

      expect(res.status).toBe(201)
      const init = fetchSpy.mock.calls[0][1]
      expect(init?.method).toBe('POST')
      expect(init?.body).toBe(body)
    })

    it('forwards the body for PUT', async () => {
      fetchSpy.mockResolvedValue(new Response('updated', { status: 200 }))

      const body = JSON.stringify({ name: 'Edited' })
      const req = new NextRequest('http://localhost:3000/api/shows/1', {
        method: 'PUT',
        body,
        headers: { 'content-type': 'application/json' },
      })
      await PUT(req)

      const init = fetchSpy.mock.calls[0][1]
      expect(init?.method).toBe('PUT')
      expect(init?.body).toBe(body)
    })

    it('forwards the body for PATCH', async () => {
      fetchSpy.mockResolvedValue(new Response('patched', { status: 200 }))

      const body = JSON.stringify({ name: 'Patched' })
      const req = new NextRequest('http://localhost:3000/api/shows/1', {
        method: 'PATCH',
        body,
        headers: { 'content-type': 'application/json' },
      })
      await PATCH(req)

      const init = fetchSpy.mock.calls[0][1]
      expect(init?.method).toBe('PATCH')
      expect(init?.body).toBe(body)
    })

    it('forwards the (empty) body for DELETE without a request body', async () => {
      fetchSpy.mockResolvedValue(new Response(null, { status: 204 }))

      const req = new NextRequest('http://localhost:3000/api/shows/1', {
        method: 'DELETE',
      })
      await DELETE(req)

      const init = fetchSpy.mock.calls[0][1]
      expect(init?.method).toBe('DELETE')
      // A bodyless DELETE forwards an empty string (await request.text()).
      expect(init?.body).toBe('')
    })
  })

  describe('204 No Content handling', () => {
    it('does not read or forward a body for a 204 backend response', async () => {
      // A Response built with a body would throw on .text() at 204 in some
      // runtimes; the route must skip the read entirely.
      const backendResponse = new Response(null, { status: 204 })
      const textSpy = vi.spyOn(backendResponse, 'text')
      fetchSpy.mockResolvedValue(backendResponse)

      const req = new NextRequest('http://localhost:3000/api/shows/1', {
        method: 'DELETE',
      })
      const res = await DELETE(req)

      expect(res.status).toBe(204)
      expect(textSpy).not.toHaveBeenCalled()
      expect(await res.text()).toBe('')
    })
  })

  describe('5xx capture', () => {
    it('captures a Sentry message tagged service: api-proxy on 5xx', async () => {
      fetchSpy.mockResolvedValue(
        new Response('boom', { status: 503, statusText: 'Service Unavailable' })
      )

      const req = new NextRequest('http://localhost:3000/api/shows')
      const res = await GET(req)

      // The 5xx is passed through to the client, not swallowed.
      expect(res.status).toBe(503)
      expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
      expect(Sentry.captureMessage).toHaveBeenCalledWith(
        'Backend returned 503',
        expect.objectContaining({
          level: 'error',
          tags: expect.objectContaining({
            service: 'api-proxy',
            backend_status: 503,
          }),
          extra: expect.objectContaining({ path: '/shows', method: 'GET' }),
        })
      )
    })

    it('does not capture a Sentry message for a 4xx backend response', async () => {
      fetchSpy.mockResolvedValue(new Response('nope', { status: 404 }))

      const req = new NextRequest('http://localhost:3000/api/shows')
      const res = await GET(req)

      expect(res.status).toBe(404)
      expect(Sentry.captureMessage).not.toHaveBeenCalled()
    })
  })

  describe('Set-Cookie rewrite', () => {
    it('rewrites SameSite=None to Lax and strips Domain, preserving other attrs', async () => {
      const headers = new Headers()
      headers.append(
        'set-cookie',
        'auth_token=xyz; Path=/; Domain=.psychichomily.com; HttpOnly; Secure; SameSite=None'
      )
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200, headers }))

      const req = new NextRequest('http://localhost:3000/api/auth/login', {
        method: 'POST',
      })
      const res = await POST(req)

      const cookies = res.headers.getSetCookie()
      expect(cookies).toHaveLength(1)
      const cookie = cookies[0]
      expect(cookie).toContain('auth_token=xyz')
      expect(cookie).toContain('Path=/')
      expect(cookie).toContain('HttpOnly')
      expect(cookie).toContain('Secure')
      expect(cookie).toContain('SameSite=Lax')
      expect(cookie).not.toContain('SameSite=None')
      expect(cookie).not.toContain('Domain=')
    })

    it('forwards multiple Set-Cookie headers, rewriting each', async () => {
      const headers = new Headers()
      headers.append('set-cookie', 'a=1; SameSite=None; Domain=.example.com')
      headers.append('set-cookie', 'b=2; Path=/')
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200, headers }))

      const req = new NextRequest('http://localhost:3000/api/auth/login', {
        method: 'POST',
      })
      const res = await POST(req)

      const cookies = res.headers.getSetCookie()
      expect(cookies).toHaveLength(2)
      expect(cookies[0]).toContain('SameSite=Lax')
      expect(cookies[0]).not.toContain('Domain=')
      expect(cookies[1]).toContain('b=2')
      expect(cookies[1]).toContain('Path=/')
    })

    it('leaves a cookie without SameSite=None or Domain untouched', async () => {
      const headers = new Headers()
      headers.append('set-cookie', 'plain=1; Path=/; HttpOnly')
      fetchSpy.mockResolvedValue(new Response('ok', { status: 200, headers }))

      const req = new NextRequest('http://localhost:3000/api/auth/login', {
        method: 'POST',
      })
      const res = await POST(req)

      const cookies = res.headers.getSetCookie()
      expect(cookies).toEqual(['plain=1; Path=/; HttpOnly'])
    })
  })

  describe('Content-Disposition forwarding', () => {
    it('forwards Content-Disposition for file downloads', async () => {
      fetchSpy.mockResolvedValue(
        new Response('id,name\n1,a', {
          status: 200,
          headers: {
            'content-type': 'text/csv',
            'content-disposition': 'attachment; filename="export.csv"',
          },
        })
      )

      const req = new NextRequest('http://localhost:3000/api/shows/export')
      const res = await GET(req)

      expect(res.headers.get('Content-Disposition')).toBe(
        'attachment; filename="export.csv"'
      )
    })

    it('does not set Content-Disposition when the backend omits it', async () => {
      fetchSpy.mockResolvedValue(
        new Response('ok', {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      )

      const req = new NextRequest('http://localhost:3000/api/shows')
      const res = await GET(req)

      expect(res.headers.get('Content-Disposition')).toBeNull()
    })
  })

  describe('fetch failure handling', () => {
    it('captures the exception and returns a 502 when fetch throws', async () => {
      const networkError = new Error('ECONNREFUSED')
      fetchSpy.mockRejectedValue(networkError)

      const req = new NextRequest('http://localhost:3000/api/shows', {
        method: 'POST',
      })
      const res = await POST(req)

      expect(res.status).toBe(502)
      expect(await res.json()).toEqual({ error: 'Backend service unavailable' })
      expect(Sentry.captureException).toHaveBeenCalledTimes(1)
      expect(Sentry.captureException).toHaveBeenCalledWith(
        networkError,
        expect.objectContaining({
          level: 'error',
          tags: expect.objectContaining({ service: 'api-proxy' }),
          extra: expect.objectContaining({ path: '/shows', method: 'POST' }),
        })
      )
    })
  })

  describe('OPTIONS preflight', () => {
    it('short-circuits with 204 and does not hit the backend', async () => {
      const req = new NextRequest('http://localhost:3000/api/shows', {
        method: 'OPTIONS',
      })
      const res = await OPTIONS(req)

      expect(res.status).toBe(204)
      expect(fetchSpy).not.toHaveBeenCalled()
      expect(await res.text()).toBe('')
    })
  })
})
