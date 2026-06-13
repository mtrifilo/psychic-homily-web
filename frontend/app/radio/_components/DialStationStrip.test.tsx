import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type {
  RadioStationListItem,
  RadioStationDetail,
  RadioEpisodeDetail,
  RadioNowPlaying,
} from '@/features/radio'

const mockUseStationOverview = vi.fn()
const mockUseStationNowPlaying = vi.fn()
const mockUseRadioStation = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useStationOverview: (...args: unknown[]) => mockUseStationOverview(...args),
    useStationNowPlaying: (...args: unknown[]) =>
      mockUseStationNowPlaying(...args),
    useRadioStation: (...args: unknown[]) => mockUseRadioStation(...args),
  }
})

import { DialStationStrip } from './DialStationStrip'

function makeStation(
  overrides: Partial<RadioStationListItem> = {}
): RadioStationListItem {
  return {
    id: 2,
    name: 'WFMU',
    slug: 'wfmu',
    city: 'Jersey City',
    state: 'NJ',
    country: 'USA',
    broadcast_type: 'both',
    frequency_mhz: 91.1,
    logo_url: null,
    is_active: true,
    network: { slug: 'wfmu', name: 'WFMU', is_flagship: true },
    sibling_stations: [
      {
        id: 5,
        slug: 'wfmu-drummer',
        name: 'Give the Drummer Radio',
        broadcast_type: 'internet',
        frequency_mhz: null,
        is_flagship: false,
      },
    ],
    show_count: 12,
    ...overrides,
  }
}

const stationDetail = {
  slug: 'wfmu',
  name: 'WFMU',
  website: 'https://wfmu.org',
} as RadioStationDetail

const latestEpisode = { air_date: '2026-06-09' } as RadioEpisodeDetail

/** Live payload for the flagship strip (slug "wfmu"). */
function liveNowPlaying(overrides: Partial<RadioNowPlaying> = {}): RadioNowPlaying {
  return {
    source: 'live',
    on_air: true,
    show: {
      id: 3,
      name: 'The Night Owl Show',
      slug: 'night-owl',
      host_name: 'Pedro Santos',
    },
    show_name: 'The Night Owl Show',
    host_name: null,
    current_track: {
      artist_name: 'CAN',
      track_title: 'Vitamin C',
      album_title: null,
      label_name: null,
      release_year: null,
      rotation_status: 'heavy',
      dj_comment: null,
      artist_id: 7,
      artist_slug: 'can',
      release_id: null,
      release_slug: null,
      label_id: null,
      label_slug: null,
    },
    recent_artists: [
      { artist_name: 'Brentford All Stars', artist_id: null, artist_slug: null },
      { artist_name: 'Mdou Moctar', artist_id: 4, artist_slug: 'mdou-moctar' },
    ],
    episode_air_date: null,
    ...overrides,
  }
}

/** Live payload for the channel sub-row (slug "wfmu-drummer"). */
function channelNowPlaying(
  overrides: Partial<RadioNowPlaying> = {}
): RadioNowPlaying {
  return liveNowPlaying({
    show: {
      id: 9,
      name: 'Honky Tonk Radio Girl',
      slug: 'honky-tonk',
      host_name: 'Becky',
    },
    show_name: 'Honky Tonk Radio Girl',
    current_track: null,
    recent_artists: [],
    ...overrides,
  })
}

// Mirrors the slimmed StationOverview shape (PSY-1075): station detail for
// [▶ Listen], plus the signature show + latest episode for [ live playlist ].
function overviewLoaded() {
  return {
    station: stationDetail,
    nowPlayingShow: { id: 3, slug: 'night-owl' },
    latestEpisode,
  }
}

function setNowPlayingBySlug(bySlug: Record<string, RadioNowPlaying | undefined>) {
  mockUseStationNowPlaying.mockImplementation((slug: string) => ({
    data: bySlug[slug],
    isLoading: false,
    error: null,
  }))
}

describe('DialStationStrip', () => {
  beforeEach(() => {
    mockUseStationOverview.mockReset().mockReturnValue(overviewLoaded())
    mockUseStationNowPlaying.mockReset()
    setNowPlayingBySlug({
      wfmu: liveNowPlaying(),
      'wfmu-drummer': channelNowPlaying(),
    })
    mockUseRadioStation.mockReset().mockReturnValue({
      data: { website: 'https://wfmu.org/drummer' },
      isLoading: false,
    })
  })

  it('renders the station name as an underlined link to the station page', () => {
    render(<DialStationStrip station={makeStation()} />)
    const link = screen.getByRole('link', { name: 'WFMU' })
    expect(link).toHaveAttribute('href', '/radio/wfmu')
    expect(link.className).toContain('underline')
  })

  it('renders frequency and identity line', () => {
    render(<DialStationStrip station={makeStation()} />)
    expect(screen.getByText('91.1 FM')).toBeInTheDocument()
    expect(
      screen.getByText('Jersey City, NJ · FM/AM + Internet · 1 channel')
    ).toBeInTheDocument()
  })

  it('renders the live show, current track, rotation tag, and earlier hops', () => {
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)

    expect(screen.getByText('On air')).toBeInTheDocument()
    const showLink = screen.getByRole('link', { name: 'The Night Owl Show' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/night-owl')
    expect(screen.getByText('w/ Pedro Santos')).toBeInTheDocument()

    expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
      'href',
      '/artists/can'
    )
    expect(screen.getByText('— Vitamin C')).toBeInTheDocument()
    expect(screen.getByText('Heavy Rotation')).toBeInTheDocument()

    // earlier: matched artist links, unmatched stays plain text.
    expect(screen.getByText('earlier:')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Mdou Moctar' })).toHaveAttribute(
      'href',
      '/artists/mdou-moctar'
    )
    expect(
      screen.queryByRole('link', { name: 'Brentford All Stars' })
    ).not.toBeInTheDocument()
  })

  it('labels the latest-archive fallback honestly instead of claiming ON AIR', () => {
    setNowPlayingBySlug({
      wfmu: liveNowPlaying({ source: 'latest_archive', on_air: false }),
    })
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)

    expect(screen.getByText('Latest playlist')).toBeInTheDocument()
    expect(screen.queryByText('On air')).not.toBeInTheDocument()
    // The show identity still renders.
    expect(
      screen.getByRole('link', { name: 'The Night Owl Show' })
    ).toBeInTheDocument()
  })

  it('renders an unmatched live show name as plain text (no dead link)', () => {
    setNowPlayingBySlug({
      wfmu: liveNowPlaying({
        show: null,
        show_name: 'Secret Canine Agents',
        host_name: 'DJ Perro Caliente',
      }),
    })
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)

    expect(screen.getByText('Secret Canine Agents')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Secret Canine Agents' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('w/ DJ Perro Caliente')).toBeInTheDocument()
  })

  it('renders Listen + live playlist actions', () => {
    render(<DialStationStrip station={makeStation()} />)

    expect(screen.getByRole('link', { name: /Listen/ })).toHaveAttribute(
      'href',
      'https://wfmu.org'
    )
    expect(
      screen.getByRole('link', { name: 'live playlist' })
    ).toHaveAttribute('href', '/radio/wfmu/night-owl/2026-06-09')
  })

  it('renders channel sub-rows with the CHANNEL OWN broadcast', () => {
    render(<DialStationStrip station={makeStation()} />)

    const channelLink = screen.getByRole('link', {
      name: 'Give the Drummer Radio',
    })
    expect(channelLink).toHaveAttribute('href', '/radio/wfmu/channel/drummer')
    expect(channelLink.className).toContain('underline')

    // The channel row consumed ITS OWN now-playing (PSY-1022 — not the
    // flagship's heuristic show).
    expect(mockUseStationNowPlaying).toHaveBeenCalledWith('wfmu-drummer')
    expect(
      screen.getByRole('link', { name: 'Honky Tonk Radio Girl' })
    ).toHaveAttribute('href', '/radio/wfmu-drummer/honky-tonk')
    expect(screen.getByText(/w\/ Becky/)).toBeInTheDocument()

    expect(screen.getByRole('link', { name: '[listen]' })).toHaveAttribute(
      'href',
      'https://wfmu.org/drummer'
    )
  })

  it('renders an unmatched channel show name as plain text with its track', () => {
    setNowPlayingBySlug({
      wfmu: liveNowPlaying(),
      'wfmu-drummer': channelNowPlaying({
        show: null,
        show_name: 'Secret Canine Agents',
        current_track: {
          artist_name: 'Nirvana',
          track_title: 'In The Courtyard',
          album_title: null,
          label_name: null,
          release_year: null,
          rotation_status: null,
          dj_comment: null,
          artist_id: null,
          artist_slug: null,
          release_id: null,
          release_slug: null,
          label_id: null,
          label_slug: null,
        },
      }),
    })
    render(<DialStationStrip station={makeStation()} />)

    expect(screen.getByText(/Secret Canine Agents/)).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Secret Canine Agents' })
    ).not.toBeInTheDocument()
    expect(screen.getByText(/Nirvana — In The Courtyard/)).toBeInTheDocument()
  })

  it('renders the empty on-air state for a station with no shows', () => {
    setNowPlayingBySlug({
      wfmu: liveNowPlaying({
        source: 'latest_archive',
        on_air: false,
        show: null,
        show_name: null,
        current_track: null,
        recent_artists: [],
      }),
    })
    mockUseStationOverview.mockReturnValue({
      ...overviewLoaded(),
      nowPlayingShow: null,
      latestEpisode: undefined,
    })
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)

    expect(screen.getByText('No playlists tracked yet.')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'live playlist' })
    ).not.toBeInTheDocument()
  })

  it('renders a loading indicator while on-air info is in flight', () => {
    mockUseStationNowPlaying.mockImplementation(() => ({
      data: undefined,
      isLoading: true,
      error: null,
    }))
    mockUseStationOverview.mockReturnValue({
      ...overviewLoaded(),
      station: undefined,
      nowPlayingShow: null,
      latestEpisode: undefined,
    })
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)
    expect(screen.getByText('Loading on-air info')).toBeInTheDocument()
  })

  it('renders an error state when the now-playing endpoint fails', () => {
    mockUseStationNowPlaying.mockImplementation(() => ({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    }))
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)
    expect(screen.getByText("Couldn't load on-air info.")).toBeInTheDocument()
  })
})
