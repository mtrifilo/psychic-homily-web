import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// The hooks barrel re-exports both the read hooks (useFestivals.ts) and the
// admin mutation hooks (useAdminFestivals.ts). Assert both groups resolve.
describe('festivals hooks barrel', () => {
  it('re-exports the read hooks', () => {
    expect(typeof hooks.useFestivals).toBe('function')
    expect(typeof hooks.useFestival).toBe('function')
    expect(typeof hooks.useFestivalArtists).toBe('function')
    expect(typeof hooks.useFestivalLineup).toBe('function')
    expect(typeof hooks.useFestivalVenues).toBe('function')
    expect(typeof hooks.useArtistFestivals).toBe('function')
    expect(typeof hooks.useSimilarFestivals).toBe('function')
    expect(typeof hooks.useFestivalBreakouts).toBe('function')
    expect(typeof hooks.useArtistFestivalTrajectory).toBe('function')
    expect(typeof hooks.useSeriesComparison).toBe('function')
  })

  it('re-exports the admin mutation hooks', () => {
    expect(typeof hooks.useCreateFestival).toBe('function')
    expect(typeof hooks.useUpdateFestival).toBe('function')
    expect(typeof hooks.useDeleteFestival).toBe('function')
    expect(typeof hooks.useAddFestivalArtist).toBe('function')
    expect(typeof hooks.useUpdateFestivalArtist).toBe('function')
    expect(typeof hooks.useRemoveFestivalArtist).toBe('function')
    expect(typeof hooks.useAddFestivalVenue).toBe('function')
    expect(typeof hooks.useRemoveFestivalVenue).toBe('function')
  })
})
