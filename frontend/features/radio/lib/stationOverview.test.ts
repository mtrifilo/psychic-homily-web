import { describe, it, expect } from 'vitest'
import {
  pickNowPlayingShow,
  formatShortAirDate,
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
