import { describe, it, expect } from 'vitest'
import {
  pickNowPlayingShow,
  formatShortAirDate,
  formatLocalAirDate,
  formatLocalTimeRange,
  formatStationLocation,
} from './stationOverview'
import type { RadioShowListItem } from '../types'

function makeShow(overrides: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 1,
    station_id: 10,
    station_name: 'KEXP',
    name: 'Variety Mix',
    slug: 'variety-mix',
    host_name: 'Cheryl Waters',
    genre_tags: ['eclectic'],
    image_url: null,
    is_active: true,
    episode_count: 5,
    
    schedule_display: null,
    latest_air_date: null,
    ...overrides,
  }
}

describe('pickNowPlayingShow', () => {
  it('returns null for undefined or empty show lists', () => {
    expect(pickNowPlayingShow(undefined)).toBeNull()
    expect(pickNowPlayingShow([])).toBeNull()
  })

  it('picks the show with the most episodes (the v1 "current show" proxy)', () => {
    const shows = [
      makeShow({ id: 1, episode_count: 3 }),
      makeShow({ id: 2, episode_count: 12 }),
      makeShow({ id: 3, episode_count: 7 }),
    ]
    expect(pickNowPlayingShow(shows)?.id).toBe(2)
  })

  it('breaks ties on the lower id for deterministic selection', () => {
    const shows = [
      makeShow({ id: 5, episode_count: 9 }),
      makeShow({ id: 2, episode_count: 9 }),
    ]
    expect(pickNowPlayingShow(shows)?.id).toBe(2)
  })
})

describe('formatShortAirDate', () => {
  it('formats YYYY-MM-DD as "Mon D" with no year', () => {
    expect(formatShortAirDate('2026-06-04')).toBe('Jun 4')
  })

  it('returns "" for missing or invalid dates', () => {
    expect(formatShortAirDate(null)).toBe('')
    expect(formatShortAirDate(undefined)).toBe('')
    expect(formatShortAirDate('not-a-date')).toBe('')
  })
})

describe('formatStationLocation', () => {
  it('joins city and state', () => {
    expect(formatStationLocation('Seattle', 'WA')).toBe('Seattle, WA')
  })

  it('drops empty parts', () => {
    expect(formatStationLocation('London', null)).toBe('London')
    expect(formatStationLocation(null, 'WA')).toBe('WA')
    expect(formatStationLocation(null, null)).toBe('')
  })
})

// PSY-1298 viewer-local helpers. Inputs are built FROM local-time Date
// constructors so the expectations hold in any machine timezone — the
// helpers render in the viewer's local zone by design.
const localIso = (
  y: number,
  m: number,
  d: number,
  h: number,
  min = 0
): string => new Date(y, m, d, h, min).toISOString()

describe('formatLocalTimeRange', () => {
  it('drops :00 minutes and shares a single meridiem', () => {
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 15), localIso(2026, 6, 1, 18))).toBe(
      '3–6 PM'
    )
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 6), localIso(2026, 6, 1, 9))).toBe(
      '6–9 AM'
    )
  })

  it('keeps minutes when non-zero', () => {
    expect(
      formatLocalTimeRange(localIso(2026, 6, 1, 18, 30), localIso(2026, 6, 1, 21))
    ).toBe('6:30–9 PM')
  })

  it('carries both meridiems across noon/midnight boundaries', () => {
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 21), localIso(2026, 6, 2, 0))).toBe(
      '9 PM–12 AM'
    )
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 9), localIso(2026, 6, 1, 12))).toBe(
      '9 AM–12 PM'
    )
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 23), localIso(2026, 6, 2, 2))).toBe(
      '11 PM–2 AM'
    )
  })

  it('renders noon as 12 PM within a shared-meridiem range', () => {
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 12), localIso(2026, 6, 1, 15))).toBe(
      '12–3 PM'
    )
  })

  it('returns "" for windowless or unparsable inputs', () => {
    expect(formatLocalTimeRange(null, null)).toBe('')
    expect(formatLocalTimeRange(localIso(2026, 6, 1, 9), null)).toBe('')
    expect(formatLocalTimeRange('not-a-date', 'also-not')).toBe('')
  })
})

describe('formatLocalAirDate', () => {
  it('derives the date from starts_at when the window exists (viewer-local)', () => {
    expect(formatLocalAirDate(localIso(2026, 6, 1, 21), '2026-06-30')).toBe('Jul 1')
  })

  it('falls back to air_date for windowless rows', () => {
    expect(formatLocalAirDate(null, '2026-07-01')).toBe('Jul 1')
    expect(formatLocalAirDate(undefined, '2026-06-30')).toBe('Jun 30')
  })

  it('falls back to air_date when starts_at is unparsable', () => {
    expect(formatLocalAirDate('garbage', '2026-07-01')).toBe('Jul 1')
  })
})
