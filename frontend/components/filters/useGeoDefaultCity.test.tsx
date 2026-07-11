import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from './useGeoDefaultCity'
import type { CityWithCount } from './CityFilters'
import type { GeoLocation } from '@/lib/geo-default'


// Centroids match the offline geocoder the backend uses (PSY-981).
const PHOENIX: CityWithCount = {
  city: 'Phoenix',
  state: 'AZ',
  count: 5,
  latitude: 33.4484,
  longitude: -112.074,
}
const OMAHA: CityWithCount = {
  city: 'Omaha',
  state: 'NE',
  count: 3,
  latitude: 41.2587,
  longitude: -95.9384,
}
const TUCSON: CityWithCount = {
  city: 'Tucson',
  state: 'AZ',
  count: 2,
  latitude: 32.2217,
  longitude: -110.9265,
}
const ALL_CITIES: CityWithCount[] = [PHOENIX, OMAHA]

// Real-ish coords for Paradise Valley, AZ — a Phoenix suburb with NO PH shows
// of its own (the PSY-981 motivating case). Nearest has-shows city = Phoenix.
const PARADISE_VALLEY = {
  city: 'Paradise Valley',
  state: 'AZ',
  latitude: 33.5312,
  longitude: -111.9426,
}

type HookParams = Parameters<typeof useGeoDefaultCity>[0]

/** Base params for an anon visitor with no favorites, no existing selection. */
function baseParams(overrides: Partial<HookParams> = {}): HookParams {
  return {
    cities: ALL_CITIES,
    isAuthenticated: false,
    authLoading: false,
    favoriteCities: [],
    hasExistingSelection: false,
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

// The hook RETURNS the derived default (appliedGeoDefault); it never seeds a
// URL or caller state. These tests assert on the returned value only.
describe('useGeoDefaultCity — server-prop path (/explore)', () => {
  it('derives the canonical geo city for an anon visitor when it has shows', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: 'Omaha', state: 'NE' } }),
      ),
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
  })

  it('matches case/whitespace-insensitively, returning the PH canonical casing', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: ' omaha ', state: 'ne' } }),
      ),
    )
    // The canonical "Omaha,NE" from the cities list, not the raw header.
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
  })

  it('derives null when the geo city has no shows', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ geoFromServer: { city: 'Tucson', state: 'AZ' } }),
      ),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('derives null when there is no geo default (null)', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(baseParams({ geoFromServer: null })),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('derives null for an authed user (favorites are the caller’s concern)', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          isAuthenticated: true,
          geoFromServer: { city: 'Omaha', state: 'NE' },
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('derives null when favorites are present (favorites win)', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          isAuthenticated: true,
          favoriteCities: [{ city: 'Phoenix', state: 'AZ' }],
          geoFromServer: { city: 'Omaha', state: 'NE' },
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('waits for auth to settle before deriving the anon geo default', () => {
    const { result, rerender } = renderHook(
      (props: HookParams) => useGeoDefaultCity(props),
      {
        initialProps: baseParams({
          authLoading: true,
          geoFromServer: { city: 'Omaha', state: 'NE' },
        }),
      },
    )
    expect(result.current.appliedGeoDefault).toBeNull()
    // Auth settles → the default derives on the same render pass.
    rerender(
      baseParams({
        authLoading: false,
        geoFromServer: { city: 'Omaha', state: 'NE' },
      }),
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
  })

  it('derives null when a selection already exists (hasExistingSelection)', () => {
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          hasExistingSelection: true,
          geoFromServer: { city: 'Omaha', state: 'NE' },
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('nulls the derived default the moment a selection appears (favorites arriving late)', () => {
    // The old seed effect raced this exact transition; the derived value
    // reconciles on the same render the favorites arrive.
    const { result, rerender } = renderHook(
      (props: HookParams) => useGeoDefaultCity(props),
      {
        initialProps: baseParams({
          geoFromServer: { city: 'Omaha', state: 'NE' },
        }),
      },
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Omaha',
      state: 'NE',
    })
    rerender(
      baseParams({
        geoFromServer: { city: 'Omaha', state: 'NE' },
        isAuthenticated: true,
        favoriteCities: [{ city: 'Phoenix', state: 'AZ' }],
      }),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('drops the derived default after notifyUserInteracted', () => {
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

  function mockGeoFetch(geo: GeoLocation | null) {
    return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ geo }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
  }

  it('fetches /api/geo and derives the canonical city', async () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    const { result } = renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true })),
    )
    await waitFor(() =>
      expect(result.current.appliedGeoDefault).toEqual({
        city: 'Omaha',
        state: 'NE',
      }),
    )
    expect(fetchSpy).toHaveBeenCalledWith('/api/geo')
  })

  it('caches the response in sessionStorage (cross-page reuse, no re-fetch)', async () => {
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    const first = renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true })),
    )
    await waitFor(() =>
      expect(first.result.current.appliedGeoDefault).not.toBeNull(),
    )
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    first.unmount()

    // A second mount (simulating navigation to another surface) reads the
    // cached value — no second fetch.
    const second = renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true })),
    )
    await waitFor(() =>
      expect(second.result.current.appliedGeoDefault).toEqual({
        city: 'Omaha',
        state: 'NE',
      }),
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
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({ isAuthenticated: true, enableClientFetch: true }),
      ),
    )
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(result.current.appliedGeoDefault).toBeNull()
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
    const { result, rerender } = renderHook(
      (props: HookParams) => useGeoDefaultCity(props),
      {
        initialProps: baseParams({
          authLoading: true,
          enableClientFetch: true,
        }),
      },
    )
    expect(fetchSpy).not.toHaveBeenCalled()
    rerender(baseParams({ authLoading: false, enableClientFetch: true }))
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledWith('/api/geo'))
    await waitFor(() =>
      expect(result.current.appliedGeoDefault).toEqual({
        city: 'Omaha',
        state: 'NE',
      }),
    )
  })

  it('derives null when the fetched city has no shows', async () => {
    mockGeoFetch({ city: 'Tucson', state: 'AZ' })
    const { result } = renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true })),
    )
    // Give the fetch a tick; the derived default stays null.
    await waitFor(() =>
      expect(
        window.sessionStorage.getItem('ph-geo-default-city'),
      ).not.toBeNull(),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('never surfaces a late-resolving fetch after the user has interacted', async () => {
    // The async race the old seed effect guarded with refs: the user acts
    // BEFORE /api/geo resolves. The derived value must stay null afterwards.
    let resolveFetch: (r: Response) => void = () => {}
    vi.spyOn(globalThis, 'fetch').mockReturnValue(
      new Promise<Response>(resolve => {
        resolveFetch = resolve
      }),
    )
    const { result } = renderHook(() =>
      useGeoDefaultCity(baseParams({ enableClientFetch: true })),
    )
    act(() => result.current.notifyUserInteracted())
    // Fetch resolves AFTER the interaction.
    act(() =>
      resolveFetch(
        new Response(
          JSON.stringify({ geo: { city: 'Omaha', state: 'NE' } }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )
    await waitFor(() => expect(result.current.appliedGeoDefault).toBeNull())
  })

  it('ignores a malformed cached value without crashing (defensive shape check)', () => {
    // A non-string city would throw on .trim() in geoCityWithShows if it
    // reached reconciliation. The shape check coerces it to null instead.
    window.sessionStorage.setItem(
      'ph-geo-default-city',
      JSON.stringify({ geo: { city: 123, state: 'AZ' } }),
    )
    const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
    let result: { current: { appliedGeoDefault: unknown } } | undefined
    expect(() => {
      result = renderHook(() =>
        useGeoDefaultCity(baseParams({ enableClientFetch: true })),
      ).result
    }).not.toThrow()
    // Cache hit short-circuits the fetch; the malformed value derives nothing.
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(result?.current.appliedGeoDefault ?? null).toBeNull()
  })
})

describe('useGeoDefaultCity — nearest has-shows city by haversine (PSY-981)', () => {
  it('derives Phoenix for a Paradise Valley visitor whose exact city has no shows', () => {
    // The motivating case: Paradise Valley, AZ is a Phoenix suburb with no PH
    // shows. With the visitor's coords + city centroids, the hook picks the
    // geographically NEAREST has-shows city (Phoenix, ~15 km away) over Tucson
    // (~160 km) and Omaha (~1,200 km). No exact "Paradise Valley" match exists.
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          cities: [PHOENIX, TUCSON, OMAHA],
          geoFromServer: PARADISE_VALLEY,
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('prefers the EXACT city match over the nearest, even when coords are present', () => {
    // A visitor IN Tucson (which HAS shows) must derive Tucson, not the
    // nearest-by-distance result — tier 1 (exact match) wins over tier 2.
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          cities: [PHOENIX, TUCSON, OMAHA],
          geoFromServer: {
            city: 'Tucson',
            state: 'AZ',
            latitude: TUCSON.latitude,
            longitude: TUCSON.longitude,
          },
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Tucson',
      state: 'AZ',
    })
  })

  it('falls back to NO default when the exact city has no shows AND coords are absent', () => {
    // Pre-PSY-981 behavior preserved: no coords → no nearest computation → the
    // exact-miss case yields no default (never worse than before).
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          cities: [PHOENIX, TUCSON, OMAHA],
          // Paradise Valley, no lat/long → exact-match only, which misses.
          geoFromServer: { city: 'Paradise Valley', state: 'AZ' },
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('falls back to NO default when no show-city carries a centroid (uncoded cities)', () => {
    // The backend geocoder missed every show city (coords undefined). The
    // nearest computation has no candidates → no default; exact-match for a
    // visitor whose own city is on the list would still work (covered above).
    const citiesNoCentroid: CityWithCount[] = [
      { city: 'Phoenix', state: 'AZ', count: 5 },
      { city: 'Tucson', state: 'AZ', count: 2 },
    ]
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          cities: citiesNoCentroid,
          geoFromServer: PARADISE_VALLEY,
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toBeNull()
  })

  it('skips uncoded cities as distance candidates but still uses coded ones', () => {
    // Cottonwood (1 show) is too small for the GeoNames slice → no centroid →
    // it must NOT be chosen as nearest; Phoenix (coded) wins for the suburb.
    const cottonwood: CityWithCount = { city: 'Cottonwood', state: 'AZ', count: 1 }
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          cities: [cottonwood, PHOENIX, TUCSON],
          geoFromServer: PARADISE_VALLEY,
        }),
      ),
    )
    expect(result.current.appliedGeoDefault).toEqual({
      city: 'Phoenix',
      state: 'AZ',
    })
  })

  it('derives the nearest via the client-fetch path too (home / /shows parity)', async () => {
    // The same nearest logic must fire on the client-fetch surfaces, not just
    // /explore's server prop — home and /shows fetch /api/geo with coords.
    mockGeoFetchPV(PARADISE_VALLEY)
    const { result } = renderHook(() =>
      useGeoDefaultCity(
        baseParams({
          cities: [PHOENIX, TUCSON, OMAHA],
          enableClientFetch: true,
        }),
      ),
    )
    await waitFor(() =>
      expect(result.current.appliedGeoDefault).toEqual({
        city: 'Phoenix',
        state: 'AZ',
      }),
    )
  })
})

/** Local client-fetch mock for the nearest-city block (its own session clear). */
function mockGeoFetchPV(geo: GeoLocation | null) {
  window.sessionStorage.clear()
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ geo }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}
