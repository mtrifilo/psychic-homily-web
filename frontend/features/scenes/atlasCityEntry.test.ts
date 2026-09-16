import { describe, it, expect } from 'vitest'
import {
  ATLAS_CITY_PARAM,
  atlasCityHref,
  parseAtlasCityParam,
} from './atlasCityEntry'

describe('atlasCityHref', () => {
  it('addresses the Atlas opened on one city', () => {
    expect(atlasCityHref('Phoenix', 'AZ')).toBe('/atlas?city=Phoenix%2CAZ')
  })

  it('escapes a city name rather than letting it reshape the query', () => {
    expect(atlasCityHref('Salt Lake City', 'UT')).toBe(
      '/atlas?city=Salt%20Lake%20City%2CUT'
    )
    expect(atlasCityHref('a&b=c', 'ZZ')).toBe('/atlas?city=a%26b%3Dc%2CZZ')
  })

  it('writes the key the Atlas reads', () => {
    expect(atlasCityHref('Mesa', 'AZ')).toContain(`${ATLAS_CITY_PARAM}=`)
  })

  it('round-trips through the parser', () => {
    const href = atlasCityHref('Phoenix', 'AZ')
    const raw = new URL(href, 'https://psychichomily.com').searchParams.get(
      ATLAS_CITY_PARAM
    )
    expect(parseAtlasCityParam(raw)).toEqual({ city: 'Phoenix', state: 'AZ' })
  })
})

describe('parseAtlasCityParam', () => {
  it('is null for an absent or empty value', () => {
    expect(parseAtlasCityParam(null)).toBeNull()
    expect(parseAtlasCityParam('')).toBeNull()
  })

  it('is null for a value that is not exactly one city', () => {
    expect(parseAtlasCityParam('Phoenix')).toBeNull()
    expect(parseAtlasCityParam('Phoenix,AZ|Austin,TX')).toBeNull()
  })
})
