import { describe, it, expect } from 'vitest'
import {
  pickNowPlayingShow,
  recentArtistsFromEpisode,
  deriveNowPlaying,
  formatShortAirDate,
  formatStationLocation,
} from './stationOverview'
import type { RadioShowListItem, RadioPlay, RadioEpisodeDetail } from '../types'

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

function makePlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 100,
    position: 1,
    artist_name: 'Sleater-Kinney',
    track_title: 'Dig Me Out',
    album_title: 'Dig Me Out',
    label_name: 'Kill Rock Stars',
    release_year: 1997,
    is_new: false,
    rotation_status: null,
    dj_comment: null,
    is_live_performance: false,
    is_request: false,
    artist_id: 1,
    artist_slug: 'sleater-kinney',
    release_id: null,
    release_slug: null,
    label_id: null,
    label_slug: null,
    musicbrainz_artist_id: null,
    musicbrainz_recording_id: null,
    musicbrainz_release_id: null,
    air_timestamp: null,
    ...overrides,
  }
}

function makeEpisode(plays: RadioPlay[], airDate = '2026-06-04'): RadioEpisodeDetail {
  return {
    id: 100,
    show_id: 1,
    show_name: 'Variety Mix',
    show_slug: 'variety-mix',
    station_name: 'KEXP',
    station_slug: 'kexp',
    title: null,
    air_date: airDate,
    air_time: null,
    duration_minutes: null,
    description: null,
    archive_url: null,
    mixcloud_url: null,
    genre_tags: null,
    mood_tags: null,
    play_count: plays.length,
    plays,
    created_at: '2026-06-04T00:00:00Z',
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

describe('recentArtistsFromEpisode', () => {
  it('returns artists most-recent first (plays stored position-ASC)', () => {
    const plays = [
      makePlay({ id: 1, position: 1, artist_name: 'Wipers' }),
      makePlay({ id: 2, position: 2, artist_name: 'Unwound' }),
      makePlay({ id: 3, position: 3, artist_name: 'Bikini Kill' }),
    ]
    const hops = recentArtistsFromEpisode(plays)
    expect(hops.map(h => h.name)).toEqual(['Bikini Kill', 'Unwound', 'Wipers'])
  })

  it('de-duplicates artists case-insensitively, keeping the most-recent casing', () => {
    // Walking newest-first, the most-recent occurrence's casing wins; the
    // earlier duplicate is dropped.
    const plays = [
      makePlay({ id: 1, position: 1, artist_name: 'Wipers' }),
      makePlay({ id: 2, position: 2, artist_name: 'WIPERS' }),
      makePlay({ id: 3, position: 3, artist_name: 'Unwound' }),
    ]
    const hops = recentArtistsFromEpisode(plays)
    expect(hops.map(h => h.name)).toEqual(['Unwound', 'WIPERS'])
  })

  it('skips the currently-playing track so it is not echoed', () => {
    const plays = [
      makePlay({ id: 1, position: 1, artist_name: 'Wipers' }),
      makePlay({ id: 2, position: 2, artist_name: 'Unwound' }),
    ]
    const hops = recentArtistsFromEpisode(plays, { skipPlayId: 2 })
    expect(hops.map(h => h.name)).toEqual(['Wipers'])
  })

  it('honors the limit', () => {
    const plays = Array.from({ length: 10 }, (_, i) =>
      makePlay({ id: i + 1, position: i + 1, artist_name: `Artist ${i}` })
    )
    expect(recentArtistsFromEpisode(plays, { limit: 3 })).toHaveLength(3)
  })

  it('preserves the artist graph slug (or null)', () => {
    const plays = [
      makePlay({ id: 1, artist_name: 'Linked', artist_slug: 'linked' }),
      makePlay({ id: 2, position: 2, artist_name: 'Unlinked', artist_slug: null }),
    ]
    const hops = recentArtistsFromEpisode(plays)
    expect(hops).toEqual([
      { name: 'Unlinked', slug: null },
      { name: 'Linked', slug: 'linked' },
    ])
  })

  it('returns an empty array for no plays', () => {
    expect(recentArtistsFromEpisode(undefined)).toEqual([])
    expect(recentArtistsFromEpisode([])).toEqual([])
  })
})

describe('deriveNowPlaying', () => {
  it('treats the last (highest-position) play as the current track', () => {
    const episode = makeEpisode([
      makePlay({ id: 1, position: 1, artist_name: 'Wipers' }),
      makePlay({ id: 2, position: 2, artist_name: 'Sleater-Kinney' }),
    ])
    const np = deriveNowPlaying(episode)
    expect(np.current?.artist_name).toBe('Sleater-Kinney')
    // the current track is not echoed in the recent-artists row
    expect(np.recentArtists.map(h => h.name)).toEqual(['Wipers'])
  })

  it('handles an episode with no plays gracefully', () => {
    expect(deriveNowPlaying(makeEpisode([]))).toEqual({ current: null, recentArtists: [] })
    expect(deriveNowPlaying(undefined)).toEqual({ current: null, recentArtists: [] })
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
