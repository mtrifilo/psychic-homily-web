import { describe, it, expect } from 'vitest'
import * as festivals from './index'

// The module barrel is the only entry point other features import from, so a
// missing/renamed re-export is a real breakage. Assert the public surface.
describe('festivals public API barrel', () => {
  it('re-exports the API config', () => {
    expect(festivals.festivalEndpoints).toBeDefined()
    expect(festivals.festivalQueryKeys).toBeDefined()
  })

  it('re-exports the type helper functions and constants', () => {
    expect(typeof festivals.getFestivalStatusVariant).toBe('function')
    expect(typeof festivals.getFestivalStatusLabel).toBe('function')
    expect(typeof festivals.getBillingTierLabel).toBe('function')
    expect(typeof festivals.formatFestivalLocation).toBe('function')
    expect(typeof festivals.formatFestivalDateRange).toBe('function')
    expect(typeof festivals.formatFestivalDates).toBe('function')
    expect(typeof festivals.getTierBarWidth).toBe('function')
    expect(typeof festivals.getMilestoneLabel).toBe('function')
    expect(Array.isArray(festivals.FESTIVAL_STATUSES)).toBe(true)
    expect(Array.isArray(festivals.BILLING_TIERS)).toBe(true)
    expect(festivals.BILLING_TIER_ORDER).toBe(festivals.BILLING_TIERS)
  })

  it('re-exports the read hooks', () => {
    expect(typeof festivals.useFestivals).toBe('function')
    expect(typeof festivals.useFestival).toBe('function')
    expect(typeof festivals.useFestivalArtists).toBe('function')
    expect(typeof festivals.useFestivalLineup).toBe('function')
    expect(typeof festivals.useFestivalVenues).toBe('function')
    expect(typeof festivals.useArtistFestivals).toBe('function')
    expect(typeof festivals.useSimilarFestivals).toBe('function')
    expect(typeof festivals.useFestivalBreakouts).toBe('function')
    expect(typeof festivals.useArtistFestivalTrajectory).toBe('function')
    expect(typeof festivals.useSeriesComparison).toBe('function')
  })

  it('re-exports the public components', () => {
    expect(typeof festivals.FestivalCard).toBe('function')
    expect(typeof festivals.FestivalLineup).toBe('function')
    expect(typeof festivals.FestivalList).toBe('function')
  })
})
