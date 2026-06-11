import { describe, it, expect } from 'vitest'
import * as components from './index'

// components/index.ts is the barrel that also exposes the FestivalDetail
// component (kept out of the root module barrel to avoid colliding with the
// FestivalDetail type). Assert every component re-export resolves.
describe('festivals components barrel', () => {
  it('re-exports every festival component', () => {
    expect(typeof components.FestivalCard).toBe('function')
    expect(typeof components.FestivalDetail).toBe('function')
    expect(typeof components.FestivalLineup).toBe('function')
    expect(typeof components.FestivalList).toBe('function')
    expect(typeof components.SimilarFestivals).toBe('function')
    expect(typeof components.ArtistTrajectoryChart).toBe('function')
    expect(typeof components.SeriesHistory).toBe('function')
  })
})
