import { describe, it, expect } from 'vitest'
import { googleMapsSearchUrl, googleMapsEmbedUrl } from './maps'

describe('googleMapsSearchUrl', () => {
  it('builds a name-first query from every available part', () => {
    expect(
      googleMapsSearchUrl({
        name: 'Salt Shed',
        address: '1357 N Elston Ave',
        city: 'Chicago',
        state: 'IL',
        zipcode: '60642',
      })
    ).toBe(
      `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(
        'Salt Shed, 1357 N Elston Ave, Chicago, IL, 60642'
      )}`
    )
  })

  // city/state are non-nullable in the venue types but arrive over the wire
  // and can be empty; the query must not carry empty comma segments.
  it('drops empty and whitespace-only parts instead of joining them', () => {
    expect(
      googleMapsSearchUrl({
        name: 'The Venue',
        address: null,
        city: 'Berlin',
        state: '  ',
      })
    ).toBe(
      `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(
        'The Venue, Berlin'
      )}`
    )
  })
})

describe('googleMapsEmbedUrl', () => {
  it('uses the same filtered query in the embed format', () => {
    expect(
      googleMapsEmbedUrl({ name: 'The Venue', city: 'Phoenix', state: 'AZ' })
    ).toBe(
      `https://maps.google.com/maps?q=${encodeURIComponent(
        'The Venue, Phoenix, AZ'
      )}&t=&z=15&ie=UTF8&iwloc=&output=embed`
    )
  })
})
