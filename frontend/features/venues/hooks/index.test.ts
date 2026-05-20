import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Type-only re-export
// (TimeFilter) has no runtime presence and is not asserted.
describe('venues hooks barrel', () => {
  it('re-exports the useVenues hooks', () => {
    expect(typeof hooks.useVenues).toBe('function')
    expect(typeof hooks.useVenue).toBe('function')
    expect(typeof hooks.useVenueShows).toBe('function')
    expect(typeof hooks.useVenueCities).toBe('function')
    expect(typeof hooks.useVenueGenres).toBe('function')
    expect(typeof hooks.useVenueBillNetwork).toBe('function')
  })

  it('re-exports the venue search hook', () => {
    expect(typeof hooks.useVenueSearch).toBe('function')
  })

  it('re-exports the venue edit hooks', () => {
    expect(typeof hooks.useVenueUpdate).toBe('function')
    expect(typeof hooks.useVenueDelete).toBe('function')
  })
})
