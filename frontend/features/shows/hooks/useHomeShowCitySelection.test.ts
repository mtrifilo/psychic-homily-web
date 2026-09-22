import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useHomeShowCitySelection } from './useHomeShowCitySelection'

type AuthStatus = 'pending' | 'authenticated' | 'anonymous'
let mockAuthStatus: AuthStatus = 'anonymous'
let mockProfileData: unknown = undefined
let mockGeo: { city: string; state: string } | null = null
let mockIsResolving = false
const geoParams = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ authStatus: mockAuthStatus }),
}))
vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: mockProfileData }),
}))
vi.mock('./useShows', () => ({
  useShowCities: () => ({
    data: {
      cities: [
        { city: 'Phoenix', state: 'AZ', show_count: 9 },
        { city: 'Tucson', state: 'AZ', show_count: 2 },
      ],
    },
  }),
}))
vi.mock('@/components/filters/useGeoDefaultCity', () => ({
  useGeoDefaultCity: (params: unknown) => {
    geoParams(params)
    return {
      appliedGeoDefault: mockGeo,
      isResolving: mockIsResolving,
      notifyUserInteracted: vi.fn(),
    }
  },
  shouldShowGeoAffordance: (
    applied: { city: string } | null,
    cities: { city: string }[]
  ) => applied !== null && cities.length === 1 && cities[0].city === applied.city,
}))

const phoenix = { city: 'Phoenix', state: 'AZ' }
const tucson = { city: 'Tucson', state: 'AZ' }

beforeEach(() => {
  mockAuthStatus = 'anonymous'
  mockProfileData = undefined
  mockGeo = null
  mockIsResolving = false
  geoParams.mockClear()
})

describe('useHomeShowCitySelection resolution order', () => {
  it('starts from nothing for an anonymous viewer with no geo', () => {
    const { result } = renderHook(() => useHomeShowCitySelection())

    expect(result.current.effectiveCities).toEqual([])
    expect(result.current.source).toBe('none')
  })

  it('takes the geo city when it applies and names it as such', () => {
    mockGeo = tucson

    const { result } = renderHook(() => useHomeShowCitySelection())

    expect(result.current.effectiveCities).toEqual([tucson])
    expect(result.current.source).toBe('geo')
    expect(result.current.geoAffordanceCity).toEqual(tucson)
  })

  it('lets favorites outrank geo', () => {
    mockAuthStatus = 'authenticated'
    mockProfileData = { user: { preferences: { favorite_cities: [phoenix] } } }
    mockGeo = tucson

    const { result } = renderHook(() => useHomeShowCitySelection())

    expect(result.current.effectiveCities).toEqual([phoenix])
    expect(result.current.source).toBe('favorites')
  })

  it("lets the viewer's own pick outrank everything, and says it is theirs", () => {
    mockAuthStatus = 'authenticated'
    mockProfileData = { user: { preferences: { favorite_cities: [phoenix] } } }

    const { result } = renderHook(() => useHomeShowCitySelection())
    act(() => result.current.onFilterChange([tucson]))

    expect(result.current.effectiveCities).toEqual([tucson])
    expect(result.current.source).toBe('user')
    expect(result.current.selectionDiffersFromFavorites).toBe(true)
  })

  it('falls back to the liveliest city only for a caller that names the city', () => {
    const plain = renderHook(() => useHomeShowCitySelection())
    expect(plain.result.current.source).toBe('none')

    const naming = renderHook(() =>
      useHomeShowCitySelection({ resolveCityForCopy: true })
    )
    expect(naming.result.current.effectiveCities).toEqual([phoenix])
    expect(naming.result.current.source).toBe('liveliest')
  })

  it('does not guess while geo is still deciding', () => {
    mockIsResolving = true

    const { result } = renderHook(() =>
      useHomeShowCitySelection({ resolveCityForCopy: true })
    )

    expect(result.current.effectiveCities).toEqual([])
    expect(result.current.source).toBe('none')
    expect(result.current.isResolving).toBe(true)
  })
})

describe('useHomeShowCitySelection geo gating', () => {
  it('opens geo to an authenticated viewer only when the caller names the city', () => {
    mockAuthStatus = 'authenticated'
    mockProfileData = { user: { preferences: {} } }

    renderHook(() => useHomeShowCitySelection())
    expect(geoParams).toHaveBeenLastCalledWith(
      expect.objectContaining({ allowAuthenticated: false })
    )

    renderHook(() => useHomeShowCitySelection({ resolveCityForCopy: true }))
    expect(geoParams).toHaveBeenLastCalledWith(
      expect.objectContaining({ allowAuthenticated: true, favoritesSettled: true })
    )
  })

  it('tells geo the favorites are not yet known while the profile is in flight', () => {
    mockAuthStatus = 'authenticated'
    mockProfileData = undefined

    renderHook(() => useHomeShowCitySelection({ resolveCityForCopy: true }))

    expect(geoParams).toHaveBeenLastCalledWith(
      expect.objectContaining({ favoritesSettled: false })
    )
  })
})
