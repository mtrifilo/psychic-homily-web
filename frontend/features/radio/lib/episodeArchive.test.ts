import { describe, it, expect, vi } from 'vitest'
import {
  isLiveNow,
  previewToHops,
  computeArtistMatchStats,
  formatPlayTime,
  formatTimeOfDay,
  formatDurationMinutes,
  formatRelativeMinutes,
  formatUpdatedAgo,
  liveEpisodePollMs,
  LIVE_EPISODE_POLL_MS,
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
    is_upcoming: false,
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

describe('liveEpisodePollMs (PSY-1511 polling gate)', () => {
  const start = '2026-06-09T21:00:00Z'
  const end = '2026-06-09T23:00:00Z'
  const during = new Date('2026-06-09T22:00:00Z')
  const after = new Date('2026-06-09T23:01:00Z')

  it('polls while the episode is live', () => {
    expect(liveEpisodePollMs(null, start, end, during)).toBe(LIVE_EPISODE_POLL_MS)
  })

  it('stops on a failing query even mid-window (PSY-1136 class)', () => {
    expect(liveEpisodePollMs(new Error('boom'), start, end, during)).toBe(false)
  })

  it('stops past ends_at', () => {
    expect(liveEpisodePollMs(null, start, end, after)).toBe(false)
  })

  it('never polls a windowless episode or before data arrives', () => {
    expect(liveEpisodePollMs(null, null, null, during)).toBe(false)
    expect(liveEpisodePollMs(null, undefined, undefined, during)).toBe(false)
  })

  it('does not poll before the window opens (upcoming)', () => {
    expect(liveEpisodePollMs(null, start, end, new Date('2026-06-09T20:00:00Z'))).toBe(
      false
    )
  })
})

describe('formatRelativeMinutes (PSY-1511 live ledger rows)', () => {
  const now = new Date('2026-06-09T22:00:00Z')

  it('renders minute granularity, then hours + minutes', () => {
    expect(formatRelativeMinutes('2026-06-09T21:58:00Z', now)).toBe('2m')
    expect(formatRelativeMinutes('2026-06-09T21:51:30Z', now)).toBe('8m')
    expect(formatRelativeMinutes('2026-06-09T20:58:00Z', now)).toBe('1h 2m')
    expect(formatRelativeMinutes('2026-06-09T21:00:00Z', now)).toBe('1h')
  })

  it('renders "now" under a minute and clamps future timestamps (clock skew)', () => {
    expect(formatRelativeMinutes('2026-06-09T21:59:30Z', now)).toBe('now')
    expect(formatRelativeMinutes('2026-06-09T22:00:30Z', now)).toBe('now')
  })

  it('is null for missing or unparseable timestamps (blank cell, never fabricated)', () => {
    expect(formatRelativeMinutes(null, now)).toBeNull()
    expect(formatRelativeMinutes(undefined, now)).toBeNull()
    expect(formatRelativeMinutes('not-a-date', now)).toBeNull()
  })
})

describe('formatUpdatedAgo (PSY-1511 live band)', () => {
  const now = new Date('2026-06-09T22:00:00Z')

  it('renders seconds under a minute, then minutes, then hours', () => {
    expect(formatUpdatedAgo(now.getTime() - 40_000, now)).toBe('updated 40s ago')
    expect(formatUpdatedAgo(now.getTime() - 2 * 60_000, now)).toBe('updated 2m ago')
    // A long-backgrounded tab must not read "updated 187m ago"
    expect(formatUpdatedAgo(now.getTime() - 187 * 60_000, now)).toBe('updated 3h ago')
  })

  it('is null before the first fetch resolves (dataUpdatedAt 0)', () => {
    expect(formatUpdatedAgo(0, now)).toBeNull()
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

  it('skips upcoming episodes — the latest aired episode has no newer neighbor (PSY-1205)', async () => {
    const episodes = [
      makeEpisode(3, '2026-06-29', { is_upcoming: true }), // future placeholder
      makeEpisode(2, '2026-06-22'), // latest aired
      makeEpisode(1, '2026-06-15'),
    ]
    const fetchPage = vi.fn().mockResolvedValue(page(episodes, 3))

    const result = await walkEpisodeNeighbors('2026-06-22', fetchPage)

    // the upcoming row is never surfaced as the "newer ▶" neighbor (no link to an
    // empty, not-yet-aired page)
    expect(result.newer).toBeNull()
    expect(result.older?.air_date).toBe('2026-06-15')
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

import { formatViewerAiredLine } from './episodeArchive'
import { localIso } from './localIso.testutil'

describe('formatViewerAiredLine (PSY-1306)', () => {
  it('renders viewer weekday + range with a station-local aside', () => {
    // Window: local Tue Jun 9, 3–6 PM (built from local constructors, so the
    // viewer part is timezone-agnostic). Station zone chosen to DIFFER from
    // the machine's so the aside isn't self-suppressed as redundant.
    const viewerZone = Intl.DateTimeFormat().resolvedOptions().timeZone
    const stationZone =
      viewerZone === 'Pacific/Kiritimati' ? 'America/Phoenix' : 'Pacific/Kiritimati'
    const line = formatViewerAiredLine(
      localIso(2026, 5, 9, 15),
      localIso(2026, 5, 9, 18),
      stationZone
    )
    expect(line).toMatch(/^Tue 3–6 PM your time \(/)
    expect(line).toMatch(/\)$/)
  })

  it('skips the aside when the viewer is in the station zone', () => {
    const viewerZone = Intl.DateTimeFormat().resolvedOptions().timeZone
    const line = formatViewerAiredLine(
      localIso(2026, 5, 9, 15),
      localIso(2026, 5, 9, 18),
      viewerZone
    )
    expect(line).toBe('Tue 3–6 PM your time')
  })

  it('returns null for windowless or degenerate windows (caller falls back)', () => {
    expect(formatViewerAiredLine(null, null, 'America/Phoenix')).toBeNull()
    expect(
      formatViewerAiredLine(localIso(2026, 5, 9, 18), localIso(2026, 5, 9, 15), 'America/Phoenix')
    ).toBeNull()
  })
})

import { airedVerbForWindow } from './episodeArchive'

describe('airedVerbForWindow (PSY-1306)', () => {
  const now = new Date(2026, 5, 9, 16, 0) // Jun 9, 4 PM local

  it('says airs before the window, airing inside it, aired after it', () => {
    expect(
      airedVerbForWindow(localIso(2026, 5, 9, 18), localIso(2026, 5, 9, 21), false, now)
    ).toBe('airs')
    expect(
      airedVerbForWindow(localIso(2026, 5, 9, 15), localIso(2026, 5, 9, 18), false, now)
    ).toBe('airing')
    expect(
      airedVerbForWindow(localIso(2026, 5, 9, 9), localIso(2026, 5, 9, 12), false, now)
    ).toBe('aired')
  })

  it('falls back to is_upcoming for windowless episodes (PSY-1205 unchanged)', () => {
    expect(airedVerbForWindow(null, null, true, now)).toBe('airs')
    expect(airedVerbForWindow(null, null, false, now)).toBe('aired')
  })

  it('ignores degenerate windows (same validity bar as the rendered line)', () => {
    // wrong-day ends_at (≥12h span): raw end > now would read "airing" for
    // weeks while the line renderer rejected the same window
    expect(
      airedVerbForWindow(localIso(2026, 5, 9, 9), localIso(2026, 5, 20, 12), false, now)
    ).toBe('aired')
    // inverted window with future start must not force "airs" on an aired row
    expect(
      airedVerbForWindow(localIso(2026, 5, 10, 9), localIso(2026, 5, 9, 12), false, now)
    ).toBe('aired')
  })
})
