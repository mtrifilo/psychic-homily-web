import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type {
  RadioStationListItem,
  RadioStationDetail,
  RadioShowDetail,
  RadioEpisodeDetail,
  RadioPlay,
} from '@/features/radio'

const mockUseStationOverview = vi.fn()
const mockUseRadioShows = vi.fn()
const mockUseRadioStation = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useStationOverview: (...args: unknown[]) => mockUseStationOverview(...args),
    useRadioShows: (...args: unknown[]) => mockUseRadioShows(...args),
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

const showDetail = {
  id: 3,
  slug: 'night-owl',
  name: 'The Night Owl Show',
  host_name: 'Pedro Santos',
} as RadioShowDetail

const currentPlay = {
  id: 77,
  artist_name: 'CAN',
  artist_slug: 'can',
  track_title: 'Vitamin C',
  rotation_status: 'heavy',
} as RadioPlay

const latestEpisode = { air_date: '2026-06-09' } as RadioEpisodeDetail

function overviewLoaded() {
  return {
    station: stationDetail,
    nowPlayingShow: { id: 3, slug: 'night-owl' },
    nowPlayingShowDetail: showDetail,
    nowPlaying: {
      current: currentPlay,
      recentArtists: [
        { name: 'Brentford All Stars', slug: null },
        { name: 'Mdou Moctar', slug: 'mdou-moctar' },
      ],
    },
    latestEpisode,
    recentShows: [],
    isLoading: false,
    isEmpty: false,
    error: null,
  }
}

describe('DialStationStrip', () => {
  beforeEach(() => {
    mockUseStationOverview.mockReset().mockReturnValue(overviewLoaded())
    mockUseRadioShows.mockReset().mockReturnValue({
      data: {
        shows: [
          {
            id: 9,
            slug: 'honky-tonk',
            name: 'Honky Tonk Radio Girl',
            host_name: 'Becky',
            episode_count: 40,
          },
        ],
      },
      isLoading: false,
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

  it('renders the on-air show, current track, rotation tag, and earlier hops', () => {
    render(<DialStationStrip station={makeStation()} />)

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

  it('renders channel sub-rows with underlined channel links and current show', () => {
    render(<DialStationStrip station={makeStation()} />)

    const channelLink = screen.getByRole('link', {
      name: 'Give the Drummer Radio',
    })
    expect(channelLink).toHaveAttribute('href', '/radio/wfmu/channel/drummer')
    expect(channelLink.className).toContain('underline')

    expect(
      screen.getByRole('link', { name: 'Honky Tonk Radio Girl' })
    ).toHaveAttribute('href', '/radio/wfmu-drummer/honky-tonk')
    expect(screen.getByText(/w\/ Becky/)).toBeInTheDocument()

    expect(screen.getByRole('link', { name: '[ listen ]' })).toHaveAttribute(
      'href',
      'https://wfmu.org/drummer'
    )
  })

  it('renders the empty on-air state for a station with no shows', () => {
    mockUseStationOverview.mockReturnValue({
      ...overviewLoaded(),
      nowPlayingShow: null,
      nowPlayingShowDetail: undefined,
      nowPlaying: { current: null, recentArtists: [] },
      latestEpisode: undefined,
      isEmpty: true,
    })
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)

    expect(screen.getByText('No playlists tracked yet.')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'live playlist' })
    ).not.toBeInTheDocument()
  })

  it('renders a loading indicator while on-air info is in flight', () => {
    mockUseStationOverview.mockReturnValue({
      ...overviewLoaded(),
      station: undefined,
      nowPlayingShowDetail: undefined,
      nowPlaying: { current: null, recentArtists: [] },
      latestEpisode: undefined,
      isLoading: true,
      isEmpty: false,
    })
    render(<DialStationStrip station={makeStation({ sibling_stations: [] })} />)
    expect(screen.getByText('Loading on-air info')).toBeInTheDocument()
  })
})
