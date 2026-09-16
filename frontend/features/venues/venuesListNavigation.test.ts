import { describe, it, expect } from 'vitest'
import {
  DEFAULT_VENUE_SORT,
  VENUES_PAGE_SIZE,
  VENUE_SORTS,
  countLabel,
  nearbyCitiesWithRooms,
  venuesCityHref,
  venuesPageHref,
} from './venuesListNavigation'
import { VENUE_LIST_PAGE_LIMIT } from './api'

describe('venuesPageHref', () => {
  it('writes no page param for page 1, so one slice has one address', () => {
    expect(venuesPageHref(new URLSearchParams(), 1)).toBe('/venues')
    expect(
      venuesPageHref(new URLSearchParams({ page: '4' }), 1)
    ).toBe('/venues')
  })

  it('carries every other param through a page click', () => {
    expect(
      venuesPageHref(
        new URLSearchParams({ cities: 'Phoenix,AZ', sort: 'name', utm: 'x' }),
        3
      )
    ).toBe('/venues?cities=Phoenix%2CAZ&sort=name&utm=x&page=3')
  })
})

describe('venuesCityHref', () => {
  it('writes the shared ?cities= wire format', () => {
    expect(venuesCityHref(new URLSearchParams(), 'Phoenix', 'AZ')).toBe(
      '/venues?cities=Phoenix%2CAZ'
    )
  })

  it('carries the order and the tag filter the reader already chose', () => {
    expect(
      venuesCityHref(
        new URLSearchParams({ sort: 'name', tags: 'diy' }),
        'Chicago',
        'IL'
      )
    ).toBe('/venues?sort=name&tags=diy&cities=Chicago%2CIL')
  })

  it('drops the page, because a different city is answered from its first', () => {
    expect(
      venuesCityHref(new URLSearchParams({ page: '5' }), 'Chicago', 'IL')
    ).toBe('/venues?cities=Chicago%2CIL')
  })

  it('evicts the legacy single-city params it supersedes', () => {
    // Left in place they would go on feeding the derivation beside a `?cities=`
    // that is meant to replace them.
    expect(
      venuesCityHref(
        new URLSearchParams({ city: 'Phoenix', state: 'AZ' }),
        'Chicago',
        'IL'
      )
    ).toBe('/venues?cities=Chicago%2CIL')
  })

  it('replaces a city already in the params rather than appending one', () => {
    expect(
      venuesCityHref(
        new URLSearchParams({ cities: 'Phoenix,AZ' }),
        'Chicago',
        'IL'
      )
    ).toBe('/venues?cities=Chicago%2CIL')
  })
})

describe('countLabel', () => {
  it('names the unit beside the number, singular at one', () => {
    expect(countLabel(1, 'room')).toBe('1 room')
    expect(countLabel(0, 'room')).toBe('0 rooms')
    expect(countLabel(2, 'upcoming show')).toBe('2 upcoming shows')
  })

  it('takes an irregular plural from the caller', () => {
    expect(countLabel(3, 'city', 'cities')).toBe('3 cities')
    expect(countLabel(1, 'city', 'cities')).toBe('1 city')
  })

  it('groups a large number for reading', () => {
    expect(countLabel(1383, 'room')).toBe('1,383 rooms')
  })
})

describe('vocabulary', () => {
  it('pages at the size the query key records', () => {
    expect(VENUES_PAGE_SIZE).toBe(VENUE_LIST_PAGE_LIMIT)
  })

  it('offers the default order among the accepted ones', () => {
    expect(VENUE_SORTS).toContain(DEFAULT_VENUE_SORT)
  })
})

describe('nearbyCitiesWithRooms', () => {
  const CITIES = [
    { city: 'Chicago', state: 'IL', count: 42 },
    { city: 'Phoenix', state: 'AZ', count: 8 },
    { city: 'Tucson', state: 'AZ', count: 5 },
    { city: 'Flagstaff', state: 'AZ', count: 0 },
    { city: 'Austin', state: 'TX', count: 9 },
    { city: 'Denver', state: 'CO', count: 7 },
    { city: 'Portland', state: 'OR', count: 6 },
  ]

  // "Nearest" is same state first, then busiest: `/venues/cities` serves no
  // coordinates, so sharing a state is the only proximity signal available.
  it('puts the subject state first, then the busiest', () => {
    expect(
      nearbyCitiesWithRooms(CITIES, { city: 'Flagstaff', state: 'AZ' }).map(
        c => c.city
      )
    ).toEqual(['Phoenix', 'Tucson', 'Chicago', 'Austin', 'Denver'])
  })

  it('leaves out the city the reader was just told is empty', () => {
    expect(
      nearbyCitiesWithRooms(CITIES, { city: 'phoenix', state: 'az' }).map(
        c => c.city
      )
    ).not.toContain('Phoenix')
  })

  it('offers at most five', () => {
    expect(nearbyCitiesWithRooms(CITIES, { city: 'Nowhere', state: 'ZZ' })).toHaveLength(5)
  })

  it('takes a shorter list from the caller', () => {
    expect(
      nearbyCitiesWithRooms(CITIES, { city: 'Flagstaff', state: 'AZ' }, 2).map(
        c => c.city
      )
    ).toEqual(['Phoenix', 'Tucson'])
  })

  // Sorting in place would reorder the facet array every other surface reads.
  it('leaves the caller list in its own order', () => {
    const input = [...CITIES]
    nearbyCitiesWithRooms(input, { city: 'Flagstaff', state: 'AZ' })
    expect(input.map(c => c.city)).toEqual(CITIES.map(c => c.city))
  })
})
