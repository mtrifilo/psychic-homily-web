import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Grouped by source
// module so a missing module surfaces an obvious cluster of failures.
describe('artists hooks barrel', () => {
  it('re-exports the useArtists hooks', () => {
    expect(typeof hooks.useArtists).toBe('function')
    expect(typeof hooks.useArtistCities).toBe('function')
    expect(typeof hooks.useArtist).toBe('function')
    expect(typeof hooks.useArtistShows).toBe('function')
  })

  it('re-exports the artist search hook', () => {
    expect(typeof hooks.useArtistSearch).toBe('function')
  })

  it('re-exports the artist report hooks', () => {
    expect(typeof hooks.useMyArtistReport).toBe('function')
    expect(typeof hooks.useReportArtist).toBe('function')
  })

  it('re-exports the artist graph hooks', () => {
    expect(typeof hooks.useArtistGraph).toBe('function')
    expect(typeof hooks.useArtistRelationshipVote).toBe('function')
    expect(typeof hooks.useCreateArtistRelationship).toBe('function')
  })

  it('re-exports the reduced-motion hook', () => {
    expect(typeof hooks.useReducedMotion).toBe('function')
  })
})
