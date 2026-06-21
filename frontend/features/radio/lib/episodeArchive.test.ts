import { describe, it, expect, vi } from 'vitest'
import {
  isLiveNow,
  previewToHops,
  computeArtistMatchStats,
  formatPlayTime,
  formatTimeOfDay,
  formatDurationMinutes,
  formatArchiveDate,
  formatShortNavDate,
  walkEpisodeNeighbors,
} from './episodeArchive'
import type { RadioEpisodeListItem, RadioEpisodesListResponse, RadioPlay } from '../types'

function makeEpisode(
  id: number,
  airDate: string,
  overrides: Partial<RadioEpisodeListItem> = {}
): RadioEpisodeListItem {
  return {
    id,
    show_id: 1,
    title: null,
    air_date: airDate,
    air_time: null,
    duration_minutes: null,
    archive_url: null,
    starts_at: null,
    ends_at: null,
    status: 'aired',
    play_count: 10,
    created_at: '2026-01-01T00:00:00Z',
    artist_preview: [],
    ...overrides,
  }
}

function makePlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 10,
    position: 1,
    artist_name: 'CAN',
    track_title: 'Mother Sky',
    album_title: 'Soundtracks',
    label_name: 'United Artists',
    release_year: 1970,
    is_new: false,
    rotation_status: null,
    dj_comment: null,
    is_live_performance: false,
    is_request: false,
    artist_id: null,
    artist_slug: null,
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

describe('isLiveNow', () => {
  const start = '2026-06-09T21:00:00Z'
  const end = '2026-06-09T23:00:00Z'

  it('is true when now is within the air window', () => {
    expect(isLiveNow(start, end, new Date('2026-06-09T22:00:00Z'))).toBe(true)
  })

  it('is true at the inclusive window edges', () => {
    expect(isLiveNow(start, end, new Date(start))).toBe(true)
    expect(isLiveNow(start, end, new Date(end))).toBe(true)
  })

  it('is false before and after the window (the PSY-1128 fix)', () => {
    expect(isLiveNow(start, end, new Date('2026-06-09T20:59:00Z'))).toBe(false)
    expect(isLiveNow(start, end, new Date('2026-06-09T23:01:00Z'))).toBe(false)
  })

  it('is false for a null / absent / unbounded window (WFMU, NTS)', () => {
    const now = new Date('2026-06-09T22:00:00Z')
    expect(isLiveNow(null, null, now)).toBe(false)
    expect(isLiveNow(undefined, undefined, now)).toBe(false)
    expect(isLiveNow(start, null, now)).toBe(false) // start but no end
    expect(isLiveNow(null, end, now)).toBe(false)
  })

  it('is false for an unparseable window', () => {
    expect(isLiveNow('not-a-date', end, new Date('2026-06-09T22:00:00Z'))).toBe(false)
  })
})

describe('previewToHops', () => {
  it('maps preview artists onto hops, preserving matched slugs', () => {
    expect(
      previewToHops([
        { artist_name: 'CAN', artist_id: 5, artist_slug: 'can' },
        { artist_name: 'The Tweeters', artist_id: null, artist_slug: null },
      ])
    ).toEqual([
      { name: 'CAN', slug: 'can' },
      { name: 'The Tweeters', slug: null },
    ])
  })

  it('returns an empty array for null / undefined', () => {
    expect(previewToHops(null)).toEqual([])
    expect(previewToHops(undefined)).toEqual([])
  })
})

describe('computeArtistMatchStats', () => {
  it('counts distinct artists and how many are matched', () => {
    const plays = [
      makePlay({ id: 1, artist_name: 'CAN', artist_id: 5 }),
      makePlay({ id: 2, artist_name: 'Neu!', artist_id: 6 }),
      makePlay({ id: 3, artist_name: 'The Tweeters', artist_id: null }),
    ]
    expect(computeArtistMatchStats(plays)).toEqual({ matched: 2, total: 3 })
  })

  it('dedups artist names case-insensitively across plays', () => {
    const plays = [
      makePlay({ id: 1, artist_name: 'CAN', artist_id: 5 }),
      makePlay({ id: 2, artist_name: 'can', artist_id: null }),
    ]
    expect(computeArtistMatchStats(plays)).toEqual({ matched: 1, total: 1 })
  })

  it('counts a name as matched when ANY of its plays carries an artist_id', () => {
    const plays = [
      makePlay({ id: 1, artist_name: 'Faust', artist_id: null }),
      makePlay({ id: 2, artist_name: 'Faust', artist_id: 9 }),
    ]
    expect(computeArtistMatchStats(plays)).toEqual({ matched: 1, total: 1 })
  })

  it('handles empty / missing plays', () => {
    expect(computeArtistMatchStats([])).toEqual({ matched: 0, total: 0 })
    expect(computeArtistMatchStats(undefined)).toEqual({ matched: 0, total: 0 })
  })
})

describe('formatPlayTime', () => {
  it('formats an ISO timestamp as a local clock time', () => {
    expect(formatPlayTime('2026-06-02T21:02:00')).toBe('9:02 PM')
  })

  it('returns null for missing or unparseable input (blank TIME cell)', () => {
    expect(formatPlayTime(null)).toBeNull()
    expect(formatPlayTime(undefined)).toBeNull()
    expect(formatPlayTime('not-a-date')).toBeNull()
  })
})

describe('formatTimeOfDay', () => {
  it('formats HH:MM:SS strings', () => {
    expect(formatTimeOfDay('21:00:00')).toBe('9:00 PM')
    expect(formatTimeOfDay('00:30:00')).toBe('12:30 AM')
    expect(formatTimeOfDay('09:05')).toBe('9:05 AM')
  })

  it('returns null for missing or unparseable input', () => {
    expect(formatTimeOfDay(null)).toBeNull()
    expect(formatTimeOfDay(undefined)).toBeNull()
    expect(formatTimeOfDay('bogus')).toBeNull()
  })
})

describe('formatDurationMinutes', () => {
  it('formats hours and minutes', () => {
    expect(formatDurationMinutes(178)).toBe('2h 58m')
    expect(formatDurationMinutes(120)).toBe('2h')
    expect(formatDurationMinutes(45)).toBe('45m')
  })

  it('returns null for unknown or non-positive durations', () => {
    expect(formatDurationMinutes(null)).toBeNull()
    expect(formatDurationMinutes(undefined)).toBeNull()
    expect(formatDurationMinutes(0)).toBeNull()
  })
})

describe('formatArchiveDate / formatShortNavDate', () => {
  it('formats archive dates without a comma', () => {
    expect(formatArchiveDate('2026-06-09')).toBe('Jun 9 2026')
  })

  it('formats short nav dates as month + day', () => {
    expect(formatShortNavDate('2026-05-26')).toBe('May 26')
  })

  it('passes through unparseable input', () => {
    expect(formatArchiveDate('bogus')).toBe('bogus')
    expect(formatShortNavDate('bogus')).toBe('bogus')
  })
})

describe('walkEpisodeNeighbors', () => {
  const page = (episodes: RadioEpisodeListItem[], total: number): RadioEpisodesListResponse => ({
    episodes,
    total,
  })

  it('returns both neighbors when the date sits mid-page', async () => {
    const episodes = [
      makeEpisode(3, '2026-06-09'),
      makeEpisode(2, '2026-06-02'),
      makeEpisode(1, '2026-05-26'),
    ]
    const fetchPage = vi.fn().mockResolvedValue(page(episodes, 3))

    const result = await walkEpisodeNeighbors('2026-06-02', fetchPage)

    expect(result.newer?.air_date).toBe('2026-06-09')
    expect(result.older?.air_date).toBe('2026-05-26')
    expect(fetchPage).toHaveBeenCalledTimes(1)
    expect(fetchPage).toHaveBeenCalledWith(0, 100)
  })

  it('returns null newer at the newest episode', async () => {
    const episodes = [makeEpisode(2, '2026-06-09'), makeEpisode(1, '2026-06-02')]
    const fetchPage = vi.fn().mockResolvedValue(page(episodes, 2))

    const result = await walkEpisodeNeighbors('2026-06-09', fetchPage)

    expect(result.newer).toBeNull()
    expect(result.older?.air_date).toBe('2026-06-02')
  })

  it('returns null older at the oldest episode', async () => {
    const episodes = [makeEpisode(2, '2026-06-09'), makeEpisode(1, '2026-06-02')]
    const fetchPage = vi.fn().mockResolvedValue(page(episodes, 2))

    const result = await walkEpisodeNeighbors('2026-06-02', fetchPage)

    expect(result.newer?.air_date).toBe('2026-06-09')
    expect(result.older).toBeNull()
  })

  it('fetches one extra row when the date sits at the bottom of a full page', async () => {
    const pageOne = Array.from({ length: 100 }, (_, i) =>
      makeEpisode(200 - i, `2025-0${(i % 9) + 1}-01`)
    )
    pageOne[99] = makeEpisode(101, '2026-06-02')
    const fetchPage = vi
      .fn()
      .mockResolvedValueOnce(page(pageOne, 150))
      .mockResolvedValueOnce(page([makeEpisode(100, '2026-05-26')], 150))

    const result = await walkEpisodeNeighbors('2026-06-02', fetchPage)

    expect(result.older?.air_date).toBe('2026-05-26')
    expect(fetchPage).toHaveBeenNthCalledWith(2, 100, 1)
  })

  it('takes the newer neighbor from the previous page tail across a boundary', async () => {
    const pageOne = Array.from({ length: 100 }, (_, i) => makeEpisode(300 - i, '2026-01-01'))
    pageOne[99] = makeEpisode(201, '2026-06-09')
    const pageTwo = [makeEpisode(200, '2026-06-02'), makeEpisode(199, '2026-05-26')]
    const fetchPage = vi
      .fn()
      .mockResolvedValueOnce(page(pageOne, 102))
      .mockResolvedValueOnce(page(pageTwo, 102))

    const result = await walkEpisodeNeighbors('2026-06-02', fetchPage)

    expect(result.newer?.air_date).toBe('2026-06-09')
    expect(result.older?.air_date).toBe('2026-05-26')
  })

  it('returns nulls when the date is not in the archive', async () => {
    const fetchPage = vi.fn().mockResolvedValue(page([makeEpisode(1, '2026-06-09')], 1))

    const result = await walkEpisodeNeighbors('1999-01-01', fetchPage)

    expect(result).toEqual({ newer: null, older: null })
    expect(fetchPage).toHaveBeenCalledTimes(1)
  })

  it('stops after one page when the date sorts newer than the page tail (DESC early exit)', async () => {
    const fullPage = Array.from({ length: 100 }, (_, i) =>
      makeEpisode(500 - i, `2020-01-${String((i % 28) + 1).padStart(2, '0')}`)
    )
    const fetchPage = vi.fn().mockResolvedValue(page(fullPage, 5000))

    // 2026 sorts newer than everything in the 2020 page — it can't be on an
    // older page, so the walk must stop without paging the whole archive.
    const result = await walkEpisodeNeighbors('2026-06-02', fetchPage)

    expect(result).toEqual({ newer: null, older: null })
    expect(fetchPage).toHaveBeenCalledTimes(1)
  })

  it('never returns a same-date sibling as a neighbor', async () => {
    // Two episodes share an air_date (distinct external_ids upstream).
    const episodes = [
      makeEpisode(4, '2026-06-09'),
      makeEpisode(3, '2026-06-02'),
      makeEpisode(2, '2026-06-02'),
      makeEpisode(1, '2026-05-26'),
    ]
    const fetchPage = vi.fn().mockResolvedValue(page(episodes, 4))

    const result = await walkEpisodeNeighbors('2026-06-02', fetchPage)

    expect(result.newer?.air_date).toBe('2026-06-09')
    expect(result.older?.air_date).toBe('2026-05-26')
  })

  it('caps the walk instead of paging forever', async () => {
    const fullPage = Array.from({ length: 100 }, (_, i) => makeEpisode(i + 1, '2026-01-01'))
    const fetchPage = vi.fn().mockResolvedValue(page(fullPage, 1_000_000))

    const result = await walkEpisodeNeighbors('1999-01-01', fetchPage)

    expect(result).toEqual({ newer: null, older: null })
    expect(fetchPage).toHaveBeenCalledTimes(20)
  })
})
