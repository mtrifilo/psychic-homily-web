import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useGeoDefaultScene } from './useGeoDefaultScene'
import { GEO_CACHE_KEY } from '@/lib/geo-client'

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

describe('useGeoDefaultScene', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
    captureException.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('settles synchronously from a warm session cache (no fetch)', () => {
    window.sessionStorage.setItem(
      GEO_CACHE_KEY,
      JSON.stringify({ geo: { city: 'Phoenix', state: 'AZ' } }),
    )
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const { result } = renderHook(() => useGeoDefaultScene())
    expect(result.current).toEqual({
      suggestion: { city: 'Phoenix', state: 'AZ' },
      resolved: true,
    })
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('treats a cached "no geo" as settled (no re-hold of the skeleton)', () => {
    window.sessionStorage.setItem(GEO_CACHE_KEY, JSON.stringify({ geo: null }))
    const { result } = renderHook(() => useGeoDefaultScene())
    expect(result.current).toEqual({ suggestion: null, resolved: true })
  })

  it('fetches /api/geo on a cold cache and caches the validated result', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ geo: { city: 'Chicago', state: 'IL' } }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    )
    const { result } = renderHook(() => useGeoDefaultScene())
    // Cold cache: unresolved until the fetch settles.
    expect(result.current).toEqual({ suggestion: null, resolved: false })
    await waitFor(() =>
      expect(result.current).toEqual({
        suggestion: { city: 'Chicago', state: 'IL' },
        resolved: true,
      }),
    )
    expect(fetchSpy).toHaveBeenCalledWith('/api/geo')
    expect(
      JSON.parse(window.sessionStorage.getItem(GEO_CACHE_KEY) as string),
    ).toEqual({ geo: { city: 'Chicago', state: 'IL' } })
  })

  it('settles to no-geo (no throw, no Sentry) when the edge route responds non-OK', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('nope', { status: 500 }),
    )
    const { result } = renderHook(() => useGeoDefaultScene())
    await waitFor(() => expect(result.current.resolved).toBe(true))
    expect(result.current.suggestion).toBeNull()
    expect(captureException).not.toHaveBeenCalled()
  })

  it('captures to Sentry and settles to no-geo on a network error', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('offline'))
    const { result } = renderHook(() => useGeoDefaultScene())
    await waitFor(() => expect(captureException).toHaveBeenCalled())
    expect(result.current).toEqual({ suggestion: null, resolved: true })
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { service: 'geo-default-scene' } }),
    )
  })

  it('settles to no-geo after the timeout when the edge route hangs', async () => {
    vi.useFakeTimers()
    // A fetch that never settles — only the timeout can resolve the hook.
    vi.spyOn(globalThis, 'fetch').mockReturnValue(
      new Promise<Response>(() => {}),
    )
    const { result } = renderHook(() => useGeoDefaultScene())
    expect(result.current).toEqual({ suggestion: null, resolved: false })
    // GEO_TIMEOUT_MS is 700ms; advancing past it fires the settle-to-null.
    await act(async () => {
      vi.advanceTimersByTime(700)
    })
    expect(result.current).toEqual({ suggestion: null, resolved: true })
  })
})
