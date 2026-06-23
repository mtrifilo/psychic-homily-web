import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enforceThrottle } from './ai-extraction-throttle'

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

let fetchSpy: ReturnType<typeof vi.spyOn> | undefined

function backendResponse(
  body: Record<string, unknown>,
  status = 200
): Response {
  return new Response(JSON.stringify(body), { status })
}

beforeEach(() => vi.clearAllMocks())
afterEach(() => {
  fetchSpy?.mockRestore()
  fetchSpy = undefined
})

describe('enforceThrottle', () => {
  it('forwards the auth_token cookie to the backend throttle endpoint', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        backendResponse({ allowed: true, retry_after_seconds: 0, limit: 10, window_seconds: 3600 })
      )

    await enforceThrottle('tok-123', 'extract-show')

    expect(fetchSpy).toHaveBeenCalledWith(
      'http://localhost:8080/ai-extraction/throttle',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ Cookie: 'auth_token=tok-123' }),
      })
    )
  })

  it('returns ok when the backend allows the attempt', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        backendResponse({ allowed: true, retry_after_seconds: 0, limit: 10, window_seconds: 3600 })
      )

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(true)
  })

  it('returns a 429 with Retry-After header + decided JSON body when throttled', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      backendResponse({
        allowed: false,
        retry_after_seconds: 2520, // 42 minutes
        limit: 10,
        window_seconds: 3600,
      })
    )

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(429)
      expect(result.response.headers.get('Retry-After')).toBe('2520')
      expect(await result.response.json()).toEqual({
        success: false,
        error: 'Rate limit exceeded. Try again in 42 minutes.',
        retry_after: 2520,
      })
    }
  })

  it('rounds the retry hint up to whole minutes', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      backendResponse({
        allowed: false,
        retry_after_seconds: 61, // just over a minute -> "2 minutes"
        limit: 10,
        window_seconds: 3600,
      })
    )

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      const body = await result.response.json()
      expect(body.error).toBe('Rate limit exceeded. Try again in 2 minutes.')
    }
  })

  it('uses seconds copy for a sub-minute wait', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      backendResponse({
        allowed: false,
        retry_after_seconds: 30,
        limit: 10,
        window_seconds: 3600,
      })
    )

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      const body = await result.response.json()
      expect(body.error).toBe('Rate limit exceeded. Try again in 30 seconds.')
    }
  })

  // Fail-closed: the gate being down must NOT become an open door to the paid
  // Anthropic API.
  it('fails closed with 503 when the backend is unreachable', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockRejectedValue(new Error('ECONNREFUSED'))

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(503)
    }
  })

  it('fails closed with 503 when the backend handler returns 5xx', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(backendResponse({ detail: 'down' }, 503))

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(503)
    }
  })

  it('passes a 401 through when the backend rejects the cookie', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(backendResponse({ detail: 'unauthorized' }, 401))

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(401)
    }
  })

  it('fails closed with 503 when the backend returns a malformed decision', async () => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(backendResponse({ unexpected: true }, 200))

    const result = await enforceThrottle('tok', 'extract-show')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.response.status).toBe(503)
    }
  })
})
