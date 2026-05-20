import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function.
describe('charts hooks barrel', () => {
  it('re-exports the charts hooks', () => {
    expect(typeof hooks.useChartsOverview).toBe('function')
    expect(typeof hooks.useTrendingShows).toBe('function')
    expect(typeof hooks.usePopularArtists).toBe('function')
    expect(typeof hooks.useActiveVenues).toBe('function')
    expect(typeof hooks.useHotReleases).toBe('function')
  })
})
