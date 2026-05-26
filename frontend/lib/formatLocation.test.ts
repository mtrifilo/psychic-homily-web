import { describe, it, expect } from 'vitest'
import { formatLocation } from './formatLocation'

describe('formatLocation (canonical PSY-780, PSY-558 rule)', () => {
  // Trailing-comma guards — the original `getVenueLocation` bug that motivated
  // PSY-780. Each variant of "one or more parts missing" should drop the
  // empty piece rather than rendering a stray separator.
  describe('trailing-separator guards', () => {
    it('returns "city" when state and country are absent', () => {
      expect(formatLocation({ city: 'Phoenix' })).toBe('Phoenix')
    })

    it('returns "state" when city and country are absent', () => {
      expect(formatLocation({ state: 'AZ' })).toBe('AZ')
    })

    it('drops empty-string state', () => {
      expect(formatLocation({ city: 'Phoenix', state: '' })).toBe('Phoenix')
    })

    it('drops whitespace-only city', () => {
      expect(formatLocation({ city: '  ', state: 'AZ' })).toBe('AZ')
    })

    it('returns "Location Unknown" when every part is missing', () => {
      expect(formatLocation({})).toBe('Location Unknown')
    })

    it('returns "Location Unknown" when every part is null', () => {
      expect(
        formatLocation({ city: null, state: null, country: null })
      ).toBe('Location Unknown')
    })

    it('returns "Location Unknown" when every part is empty/whitespace', () => {
      expect(formatLocation({ city: '', state: '   ', country: '' })).toBe(
        'Location Unknown'
      )
    })
  })

  // PSY-558 country-suppression rule. "Phoenix, AZ" reads as US-implicit to
  // local users; "Phoenix, AZ, USA" is noise. International artists always
  // render the country since the state cue alone isn't a sufficient locator.
  describe('PSY-558 country-suppression rule', () => {
    it('suppresses "USA" for a domestic city + state', () => {
      expect(
        formatLocation({ city: 'Phoenix', state: 'AZ', country: 'USA' })
      ).toBe('Phoenix, AZ')
    })

    it('suppresses "US" for a domestic city + state', () => {
      expect(
        formatLocation({ city: 'Austin', state: 'TX', country: 'US' })
      ).toBe('Austin, TX')
    })

    it('treats US suppression as case-insensitive', () => {
      expect(
        formatLocation({ city: 'Phoenix', state: 'AZ', country: 'usa' })
      ).toBe('Phoenix, AZ')
      expect(
        formatLocation({ city: 'Austin', state: 'TX', country: 'us' })
      ).toBe('Austin, TX')
    })

    it('trims whitespace around the country before comparing', () => {
      expect(
        formatLocation({ city: 'Austin', state: 'TX', country: '  USA  ' })
      ).toBe('Austin, TX')
    })

    it('renders non-US country with city + state', () => {
      expect(
        formatLocation({
          city: 'Melbourne',
          state: 'Victoria',
          country: 'Australia',
        })
      ).toBe('Melbourne, Victoria, Australia')
    })

    it('renders non-US country with city only', () => {
      expect(
        formatLocation({ city: 'Melbourne', country: 'Australia' })
      ).toBe('Melbourne, Australia')
    })

    it('renders "USA" when state is missing (suppression needs both)', () => {
      // No state ⇒ country carries the locator load and must render.
      expect(formatLocation({ city: 'Phoenix', country: 'USA' })).toBe(
        'Phoenix, USA'
      )
    })

    it('renders country alone when city + state are absent', () => {
      expect(formatLocation({ country: 'Japan' })).toBe('Japan')
    })
  })
})
