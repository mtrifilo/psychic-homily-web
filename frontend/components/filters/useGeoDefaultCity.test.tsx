import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from './useGeoDefaultCity'
import type { CityState, CityWithCount } from './CityFilters'

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

const PHOENIX: CityWithCount = { city: 'Phoenix', state: 'AZ', count: 5 }
const OMAHA: CityWithCount = { city: 'Omaha', state: 'NE', count: 3 }
const ALL_CITIES: CityWithCount[] = [PHOENIX, OMAHA]

type HookParams = Parameters<typeof useGeoDefaultCity>[0]

/** Base params for an anon visitor with no favorites, no existing selection. */
function baseParams(overrides: Partial<HookParams> = {}): HookParams {
  return {
    cities: ALL_CITIES,
    isAuthenticated: false,
    authLoading: false,
    favoriteCities: [],
    hasExistingSelection: false,
    onSeed: vi.fn(),
    ...overrides,
  }
}

describe('shouldShowGeoAffordance', () => {
  it('is true when the single selected city equals the applied geo default', () => {
    expect(
      shouldShowGeoAffordance({ city: 'Omaha', state: 'NE' }, [
        { city: 'Omaha', state: 'NE' },
      ]),
    ).toBe(true)
  })

  it('is false when nothing is applied', () => {
    expect(shouldShowGeoAffordance(null, [{ city: 'Omaha', state: 'NE' }])).toBe(
      false,
    )
  })

  it('is false when the selection differs from the applied default', () => {
    expect(
      shouldShowGeoAffordance({ city: 'Omaha', state: 'NE' }, [
        { city: 'Phoenix', state: 'AZ' },
      ]),
    ).toBe(false)
  })

  it('is false when more than one city is selected', () => {
    expect(
      shouldShowGeoAffordance({ city: 'Omaha', state: 'NE' }, [
        { city: 'Omaha', state: 'NE' },
        { city: 'Phoenix', state: 'AZ' },
      ]),
    ).toBe(false)
  })
})

describe('useGeoDefaultCity — server-prop path (/explore)', () => {
  it('seeds the canonical geo city for an anon visitor when it has shows', () => {
    const onSeed = vi.fn()
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: 'Omaha', state: 'NE' }, onSeed }),
      ),
    )
    expect(onSeed).toHaveBeenCalledWith({ city: 'Omaha', state: 'NE' })
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
  })

  it('matches case/whitespace-insensitively and seeds the PH canonical casing', () => {
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: ' omaha ', state: 'ne' }, onSeed }),
      ),
    )
    // Seeds the canonical "Omaha,NE" from the cities list, not the raw header.
    expect(onSeed).toHaveBeenCalledWith({ city: 'Omaha', state: 'NE' })
  })

  it('does NOT seed when the geo city has no shows', () => {
    const onSeed = vi.fn()
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: 'Tucson', state: 'AZ' }, onSeed }),
      ),
    )
    expect(onSeed).not.toHaveBeenCalled()
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('does NOT seed when there is no geo default (null)', () => {
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(baseParams({ geoFromServer: null, onSeed })),
    )
    expect(onSeed).not.toHaveBeenCalled()
  })

  it('does NOT seed for an authed user (favorites are the caller’s concern)', () => {
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          isAuthenticated: true,
          geoFromServer: { city: 'Omaha', state: 'NE' },
          onSeed,
        }),
      ),
    )
    expect(onSeed).not.toHaveBeenCalled()
  })

  it('does NOT seed when favorites are present (favorites win)', () => {
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          isAuthenticated: true,
          favoriteCities: [{ city: 'Phoenix', state: 'AZ' }],
          geoFromServer: { city: 'Omaha', state: 'NE' },
          onSeed,
        }),
      ),
    )
    expect(onSeed).not.toHaveBeenCalled()
  })

  it('waits for auth to settle before seeding the anon geo default', () => {
    const onSeed = vi.fn()
    const { rerender } = renderHook(
      (props: HookParams) => useGeoDefaultCity(props),
      {
        initialProps: baseParams({
          authLoading: true,
          geoFromServer: { city: 'Omaha', state: 'NE' },
          onSeed,
        }),
      },
    )
    expect(onSeed).not.toHaveBeenCalled()
    // Auth settles → now it seeds.
    rerender(
      baseParams({
        authLoading: false,
        geoFromServer: { city: 'Omaha', state: 'NE' },
        onSeed,
      }),
    )
    expect(onSeed).toHaveBeenCalledWith({ city: 'Omaha', state: 'NE' })
  })

  it('does NOT seed when a selection already exists (hasExistingSelection)', () => {
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          hasExistingSelection: true,
          geoFromServer: { city: 'Omaha', state: 'NE' },
          onSeed,
        }),
      ),
    )
    expect(onSeed).not.toHaveBeenCalled()
  })

  it('seeds only once even as inputs change', () => {
    const onSeed = vi.fn()
    const { rerender } = renderHook(
      (props: HookParams) => useGeoDefaultCity(props),
      {
        initialProps: baseParams({
          geoFromServer: { city: 'Omaha', state: 'NE' },
          onSeed,
        }),
      },
    )
    rerender(
      baseParams({ geoFromServer: { city: 'Omaha', state: 'NE' }, onSeed }),
    )
    expect(onSeed).toHaveBeenCalledTimes(1)
  })

  it('drops the affordance after notifyUserInteracted', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: 'Omaha', state: 'NE' } }),
      ),
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
    act(() => result.current.notifyUserInteracted())
    expect(result.current.appliedGeoDefault).toBeNull()
  })
})

describe('useGeoDefaultCity — client-fetch path (/shows + home)', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
    vi.restoreAllMocks()
  })

  afterEach(() => {
    window.sessionStorage.clear()
  })

  function mockGeoFetch(geo: CityState | null) {
    return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ geo }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
  }

  it('fetches /api/geo and seeds the canonical city on mount', async () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true, onSeed })),
    )
    await waitFor(() =>
      expect(onSeed).toHaveBeenCalledWith({ city: 'Omaha', state: 'NE' }),
    )
    expect(fetchSpy).toHaveBeenCalledWith('/api/geo')
  })

  it('caches the response in sessionStorage (cross-page reuse, no re-fetch)', async () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    const onSeed1 = vi.fn()
    const first = renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true, onSeed: onSeed1 })),
    )
    await waitFor(() => expect(onSeed1).toHaveBeenCalled())
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    first.unmount()

    // A second mount (simulating navigation to another surface) reads the
    // cached value synchronously — no second fetch.
    const onSeed2 = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true, onSeed: onSeed2 })),
    )
    await waitFor(() =>
      expect(onSeed2).toHaveBeenCalledWith({ city: 'Omaha', state: 'NE' }),
    )
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })

  it('does NOT fetch when enableClientFetch is false (e.g. authed up front)', () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    renderHook(() => useGeoDefaultCity(baseParams({ enableClientFetch: false })))
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('does NOT fetch for an authed visitor (efficiency: no wasted edge request)', () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(
        baseParams({ isAuthenticated: true, enableClientFetch: true, onSeed }),
      ),
    )
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(onSeed).not.toHaveBeenCalled()
  })

  it('does NOT fetch for an anon visitor with favorites (efficiency gate)', () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          favoriteCities: [{ city: 'Phoenix', state: 'AZ' }],
          enableClientFetch: true,
        }),
      ),
    )
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('does NOT fetch while auth is still loading, then fetches once it settles to anon', async () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    const onSeed = vi.fn()
    const { rerender } = renderHook(
      (props: HookParams) => useGeoDefaultCity(props),
      {
        initialProps: baseParams({
          authLoading: true,
          enableClientFetch: true,
          onSeed,
        }),
      },
    )
    expect(fetchSpy).not.toHaveBeenCalled()
    rerender(
      baseParams({ authLoading: false, enableClientFetch: true, onSeed }),
    )
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledWith('/api/geo'))
    await waitFor(() =>
      expect(onSeed).toHaveBeenCalledWith({ city: 'Omaha', state: 'NE' }),
    )
  })

  it('does not seed when the fetched city has no shows', async () => {
    mockGeoFetch({ city: 'Tucson', state: 'AZ' })
    const onSeed = vi.fn()
    renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true, onSeed })),
    )
    // Give the effect a tick; assert no seed.
    await waitFor(() => expect(window.sessionStorage.getItem('ph-geo-default-city')).not.toBeNull())
    expect(onSeed).not.toHaveBeenCalled()
  })
})
