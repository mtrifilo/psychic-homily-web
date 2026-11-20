import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { GEO_CACHE_KEY, readCachedGeoAnswer } from '@/lib/geo-client'
import { useGeoDefaultCity } from './useGeoDefaultCity'
import type { CityWithCount } from './CityFilters'

// `readCachedGeoAnswer` and `useGeoDefaultCity` each read the cached `/api/geo`
// answer. A reader that treats an entry as an answer must be one the hook
// applies without a request, and to the same default; one it treats as missing
// must be one the hook goes on to request.

const cities: CityWithCount[] = [
  { city: 'Phoenix', state: 'AZ', count: 3, latitude: 33.45, longitude: -112.07 },
  { city: 'Mesa', state: 'AZ', count: 2, latitude: 33.42, longitude: -111.82 },
]

const anonymous = {
  cities,
  authStatus: 'anonymous' as const,
  favoriteCities: [],
  hasExistingSelection: false,
}

const STORED: Array<[string, string]> = [
  ['a city with shows', JSON.stringify({ geo: { city: 'Mesa', state: 'AZ' } })],
  [
    'a city with none, and coordinates',
    JSON.stringify({ geo: { city: 'Gilbert', state: 'AZ', latitude: 33.35, longitude: -111.79 } }),
  ],
  ['a city with none, no coordinates', JSON.stringify({ geo: { city: 'Flagstaff', state: 'AZ' } })],
  ['"no city"', JSON.stringify({ geo: null })],
  ['an empty object', '{}'],
  ['a JSON null', 'null'],
  ['a malformed location', JSON.stringify({ geo: { city: 3 } })],
  ['text that does not parse', '{not json'],
]

let fetchSpy: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchSpy = vi.fn(() => new Promise(() => {}))
  vi.stubGlobal('fetch', fetchSpy)
  window.sessionStorage.clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
  window.sessionStorage.clear()
})

describe('readCachedGeoAnswer against the list geo hook', () => {
  it.each(STORED)('reads %s the way the hook restores it', async (_name, stored) => {
    window.sessionStorage.setItem(GEO_CACHE_KEY, stored)
    const answer = readCachedGeoAnswer()

    const fromCache = renderHook(() =>
      useGeoDefaultCity({ ...anonymous, enableClientFetch: true })
    )
    await act(async () => {})

    if (answer === undefined) {
      expect(fetchSpy).toHaveBeenCalled()
      expect(fromCache.result.current.isResolving).toBe(true)
      return
    }

    const fromAnswer = renderHook(() =>
      useGeoDefaultCity({ ...anonymous, geoFromServer: answer, enableClientFetch: false })
    )
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(fromCache.result.current.isResolving).toBe(false)
    expect(fromCache.result.current.appliedGeoDefault).toEqual(
      fromAnswer.result.current.appliedGeoDefault
    )
  })

  it('reads an empty cache as missing, and the hook requests it', async () => {
    expect(readCachedGeoAnswer()).toBeUndefined()

    renderHook(() => useGeoDefaultCity({ ...anonymous, enableClientFetch: true }))
    await act(async () => {})

    expect(fetchSpy).toHaveBeenCalled()
  })
})
