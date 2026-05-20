import { describe, it, expect } from 'vitest'
import {
  FESTIVAL_STATUSES,
  FESTIVAL_STATUS_LABELS,
  BILLING_TIERS,
  BILLING_TIER_ORDER,
  BILLING_TIER_LABELS,
  getFestivalStatusVariant,
  getFestivalStatusLabel,
  getBillingTierLabel,
  formatFestivalLocation,
  formatFestivalDateRange,
  formatFestivalDates,
  getTierBarWidth,
  getMilestoneLabel,
} from './types'

describe('status constants', () => {
  it('keeps a label for every status in the filter list', () => {
    for (const status of FESTIVAL_STATUSES) {
      expect(FESTIVAL_STATUS_LABELS[status]).toBeTruthy()
    }
  })
})

describe('getFestivalStatusVariant', () => {
  it('maps known statuses to badge variants', () => {
    expect(getFestivalStatusVariant('confirmed')).toBe('default')
    expect(getFestivalStatusVariant('announced')).toBe('secondary')
    expect(getFestivalStatusVariant('cancelled')).toBe('destructive')
    expect(getFestivalStatusVariant('completed')).toBe('outline')
  })

  it('falls back to secondary for unknown statuses', () => {
    expect(getFestivalStatusVariant('something-else')).toBe('secondary')
  })
})

describe('getFestivalStatusLabel', () => {
  it('returns the curated label for known statuses', () => {
    expect(getFestivalStatusLabel('announced')).toBe('Announced')
    expect(getFestivalStatusLabel('completed')).toBe('Completed')
  })

  it('capitalizes the first letter for unknown statuses', () => {
    expect(getFestivalStatusLabel('postponed')).toBe('Postponed')
  })
})

describe('billing tier constants', () => {
  it('keeps a label for every billing tier', () => {
    for (const tier of BILLING_TIERS) {
      expect(BILLING_TIER_LABELS[tier]).toBeTruthy()
    }
  })

  it('aliases BILLING_TIER_ORDER to BILLING_TIERS', () => {
    expect(BILLING_TIER_ORDER).toBe(BILLING_TIERS)
  })
})

describe('getBillingTierLabel', () => {
  it('returns the curated label for known tiers', () => {
    expect(getBillingTierLabel('headliner')).toBe('Headliner')
    expect(getBillingTierLabel('sub_headliner')).toBe('Sub-Headliner')
    expect(getBillingTierLabel('dj')).toBe('DJ')
  })

  it('title-cases snake_case for unknown tiers', () => {
    expect(getBillingTierLabel('special_guest')).toBe('Special Guest')
  })
})

describe('formatFestivalLocation', () => {
  it('returns null when nothing is set', () => {
    expect(
      formatFestivalLocation({ location_name: null, city: null, state: null })
    ).toBeNull()
  })

  it('combines city and state', () => {
    expect(
      formatFestivalLocation({ city: 'Phoenix', state: 'AZ' })
    ).toBe('Phoenix, AZ')
  })

  it('uses city alone when state is missing', () => {
    expect(formatFestivalLocation({ city: 'Phoenix', state: null })).toBe(
      'Phoenix'
    )
  })

  it('uses state alone when city is missing', () => {
    expect(formatFestivalLocation({ city: null, state: 'AZ' })).toBe('AZ')
  })

  it('prepends a venue/location name with an em-dash separator', () => {
    expect(
      formatFestivalLocation({
        location_name: 'Hance Park',
        city: 'Phoenix',
        state: 'AZ',
      })
    ).toBe('Hance Park — Phoenix, AZ')
  })
})

describe('formatFestivalDateRange', () => {
  it('renders a single date when start equals end', () => {
    expect(formatFestivalDateRange('2025-05-09', '2025-05-09')).toBe(
      'May 9, 2025'
    )
  })

  it('collapses the month when start and end share one', () => {
    expect(formatFestivalDateRange('2025-05-09', '2025-05-11')).toBe(
      'May 9–11, 2025'
    )
  })

  it('spells out both months when they differ', () => {
    expect(formatFestivalDateRange('2025-05-30', '2025-06-01')).toBe(
      'May 30 – Jun 1, 2025'
    )
  })

  it('is aliased by formatFestivalDates', () => {
    expect(formatFestivalDates).toBe(formatFestivalDateRange)
  })
})

describe('getTierBarWidth', () => {
  it('returns a descending width per known tier', () => {
    expect(getTierBarWidth('headliner')).toBe(100)
    expect(getTierBarWidth('sub_headliner')).toBe(80)
    expect(getTierBarWidth('mid_card')).toBe(60)
    expect(getTierBarWidth('undercard')).toBe(40)
    expect(getTierBarWidth('local')).toBe(25)
    expect(getTierBarWidth('dj')).toBe(25)
    expect(getTierBarWidth('host')).toBe(15)
  })

  it('falls back to 30 for unknown tiers', () => {
    expect(getTierBarWidth('mystery')).toBe(30)
  })
})

describe('getMilestoneLabel', () => {
  it('returns curated labels for known milestones', () => {
    expect(getMilestoneLabel('first_festival_appearance')).toBe(
      'Festival Debut'
    )
    expect(getMilestoneLabel('first_headliner')).toBe('First Headliner')
    expect(getMilestoneLabel('local_graduation')).toBe('Graduated from Local')
  })

  it('title-cases snake_case for unknown milestones', () => {
    expect(getMilestoneLabel('first_sub_headliner')).toBe(
      'First Sub Headliner'
    )
  })
})
