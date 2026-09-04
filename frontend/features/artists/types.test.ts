import { describe, it, expect } from 'vitest'
import { getArtistLocation } from './types'

describe('getArtistLocation (PSY-558 display rule)', () => {
  it('suppresses "USA" for a domestic city + state', () => {
    expect(
      getArtistLocation({ city: 'Phoenix', state: 'AZ', country: 'USA' })
    ).toBe('Phoenix, AZ')
  })

  it('renders city + state when no country is provided', () => {
    expect(getArtistLocation({ city: 'Phoenix', state: 'AZ' })).toBe(
      'Phoenix, AZ'
    )
  })

  it('shows the country for an international city + country', () => {
    expect(
      getArtistLocation({ city: 'Tokyo', country: 'Japan' })
    ).toBe('Tokyo, Japan')
  })

  it('shows the country when city + state + a non-US country are set', () => {
    // State alone is not enough to locate an international artist, so the
    // country is appended even though a state is present.
    expect(
      getArtistLocation({
        city: 'Melbourne',
        state: 'Victoria',
        country: 'Australia',
      })
    ).toBe('Melbourne, Victoria, Australia')
  })

  it('drops the city when it is missing', () => {
    expect(getArtistLocation({ state: 'AZ', country: 'USA' })).toBe('AZ')
  })

  it('shows country alone when city and state are both missing', () => {
    expect(getArtistLocation({ country: 'Japan' })).toBe('Japan')
  })

  // Missing state: "USA" is no longer suppressed because the suppression
  // requires BOTH a state AND a US country.
  it('shows "USA" when state is missing (suppression needs a state)', () => {
    expect(getArtistLocation({ city: 'Phoenix', country: 'USA' })).toBe(
      'Phoenix, USA'
    )
  })

  it('suppresses lowercase "usa" with a state set', () => {
    expect(
      getArtistLocation({ city: 'Phoenix', state: 'AZ', country: 'usa' })
    ).toBe('Phoenix, AZ')
  })

  it('suppresses "US" (both spellings) with a state set', () => {
    expect(
      getArtistLocation({ city: 'Austin', state: 'TX', country: 'US' })
    ).toBe('Austin, TX')
    expect(
      getArtistLocation({ city: 'Austin', state: 'TX', country: 'us' })
    ).toBe('Austin, TX')
  })

  it('trims whitespace around the country before comparing', () => {
    expect(
      getArtistLocation({ city: 'Austin', state: 'TX', country: '  USA  ' })
    ).toBe('Austin, TX')
  })

  it('returns "Location Unknown" when nothing is set', () => {
    expect(getArtistLocation({})).toBe('Location Unknown')
  })

  it('treats null fields the same as missing', () => {
    expect(
      getArtistLocation({ city: null, state: null, country: null })
    ).toBe('Location Unknown')
  })
})
