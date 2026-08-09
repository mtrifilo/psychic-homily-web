import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
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
  })

  it('returns a warm session cache synchronously (no fetch)', () => {
    window.sessionStorage.setItem(
      GEO_CACHE_KEY,
      JSON.stringify({ geo: { city: 'Phoenix', state: 'AZ' } }),
    )
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const { result } = renderHook(() => useGeoDefaultScene())
    expect(result.current).toEqual({ city: 'Phoenix', state: 'AZ' })
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('does not re-fetch when the cache holds an explicit "no geo"', () => {
    window.sessionStorage.setItem(GEO_CACHE_KEY, JSON.stringify({ geo: null }))
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const { result } = renderHook(() => useGeoDefaultScene())
    expect(result.current).toBeNull()
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('fetches /api/geo on a cold cache and caches the validated result', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ geo: { city: 'Chicago', state: 'IL' } }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    )
    const { result } = renderHook(() => useGeoDefaultScene())
    // Cold cache: null until the fetch resolves (non-blocking — the section
    // shows its liveliest default meanwhile).
    expect(result.current).toBeNull()
    await waitFor(() =>
      expect(result.current).toEqual({ city: 'Chicago', state: 'IL' }),
    )
    expect(fetchSpy).toHaveBeenCalledWith('/api/geo')
    expect(
      JSON.parse(window.sessionStorage.getItem(GEO_CACHE_KEY) as string),
    ).toEqual({ geo: { city: 'Chicago', state: 'IL' } })
  })

  it('stays null (no throw, no Sentry) when the edge route responds non-OK', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('nope', { status: 500 }),
    )
    const { result } = renderHook(() => useGeoDefaultScene())
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    expect(result.current).toBeNull()
    expect(captureException).not.toHaveBeenCalled()
  })

  it('captures to Sentry and stays null on a network error', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('offline'))
    const { result } = renderHook(() => useGeoDefaultScene())
    await waitFor(() => expect(captureException).toHaveBeenCalled())
    expect(result.current).toBeNull()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { service: 'geo-default-scene' } }),
    )
  })
})
