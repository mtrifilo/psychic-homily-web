import { describe, expect, it } from 'vitest'
import { suggestAlternativeCities } from './suggestCities'
import type { CityWithCount } from '@/components/filters'

const cities: CityWithCount[] = [
  {
    city: 'Phoenix',
    state: 'AZ',
    count: 40,
    latitude: 33.4484,
    longitude: -112.074,
  },
  {
    city: 'Tempe',
    state: 'AZ',
    count: 8,
    latitude: 33.4255,
    longitude: -111.94,
  },
  {
    city: 'Tucson',
    state: 'AZ',
    count: 20,
    latitude: 32.2226,
    longitude: -110.9747,
  },
  {
    city: 'Los Angeles',
    state: 'CA',
    count: 50,
    latitude: 34.0522,
    longitude: -118.2437,
  },
]

describe('suggestAlternativeCities', () => {
  it('ranks by proximity when centroids are available', () => {
    expect(
      suggestAlternativeCities(cities, [{ city: 'Phoenix', state: 'AZ' }], 2).map(
        c => c.city
      )
    ).toEqual(['Tempe', 'Tucson'])
  })

  it('excludes the selected city', () => {
    const result = suggestAlternativeCities(
      cities,
      [{ city: 'Phoenix', state: 'AZ' }],
      5
    )
    expect(result.every(c => c.city !== 'Phoenix')).toBe(true)
  })

  it('falls back to most-active when selected city has no centroid', () => {
    const withoutCoords: CityWithCount[] = cities.map(
      ({ latitude: _lat, longitude: _lng, ...rest }) => rest
    )
    expect(
      suggestAlternativeCities(
        withoutCoords,
        [{ city: 'Phoenix', state: 'AZ' }],
        2
      ).map(c => c.city)
    ).toEqual(['Los Angeles', 'Tucson'])
  })

  it('returns empty when nothing is selected', () => {
    expect(suggestAlternativeCities(cities, [])).toEqual([])
  })
})
