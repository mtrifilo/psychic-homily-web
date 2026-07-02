import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { RadioEpisodeDetail } from '@/features/radio'
import { localIso } from '@/features/radio/lib/localIso.testutil'

const mockUseRadioEpisode = vi.fn()
const mockUseEpisodeNeighbors = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useRadioEpisode: (...args: unknown[]) => mockUseRadioEpisode(...args),
    useEpisodeNeighbors: (...args: unknown[]) => mockUseEpisodeNeighbors(...args),
  }
})

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

import EpisodeDateDetail from './EpisodeDateDetail'

function makeEpisode(overrides: Partial<RadioEpisodeDetail> = {}): RadioEpisodeDetail {
  return {
    id: 1,
    show_id: 3,
    show_name: 'The Night Owl Show',
    show_slug: 'night-owl',
    station_name: 'WFMU',
    station_slug: 'wfmu',
    title: null,
    air_date: '2026-06-08',
    air_time: '21:00',
    starts_at: null,
    ends_at: null,
    station_timezone: null,
    is_upcoming: false,
    duration_minutes: null,
    description: null,
    archive_url: null,
    mixcloud_url: null,
    genre_tags: null,
    mood_tags: null,
    play_count: 3,
    plays: [],
    created_at: '2026-06-08T00:00:00Z',
    ...overrides,
  }
}

function setEpisode(episode: RadioEpisodeDetail) {
  mockUseRadioEpisode.mockReturnValue({ data: episode, isLoading: false, error: null })
  mockUseEpisodeNeighbors.mockReturnValue({ data: undefined })
}

const props = { stationSlug: 'wfmu', showSlug: 'night-owl', date: '2026-06-08' }

describe('EpisodeDateDetail (PSY-1306)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the H1 station-dated while the aired line is viewer-local (design decision 1)', () => {
    // Window's local day (Jun 9) deliberately differs from the station
    // air_date (Jun 8): the H1 must stay on the URL-keyed station date while
    // the aired line derives from the window.
    setEpisode(
      makeEpisode({
        starts_at: localIso(2026, 5, 9, 15),
        ends_at: localIso(2026, 5, 9, 18),
        station_timezone: 'Pacific/Kiritimati', // never the test machine's zone
      })
    )
    render(<EpisodeDateDetail {...props} />)

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('June 8, 2026')
    expect(screen.getByText(/aired Tue 3–6 PM your time \(/)).toBeInTheDocument()
  })

  it('falls back to the station-dated air_time line for windowless episodes', () => {
    setEpisode(makeEpisode())
    render(<EpisodeDateDetail {...props} />)
    // Jun 8 2026 is a Monday; 21:00 renders via formatTimeOfDay
    expect(screen.getByText(/aired Mon 9:00 PM/)).toBeInTheDocument()
  })

  it('says "airing" mid-window instead of claiming an in-progress show aired', () => {
    const starts = new Date(Date.now() - 30 * 60 * 1000).toISOString()
    const ends = new Date(Date.now() + 60 * 60 * 1000).toISOString()
    setEpisode(makeEpisode({ starts_at: starts, ends_at: ends }))
    render(<EpisodeDateDetail {...props} />)
    expect(screen.getByText(/· airing .* your time/)).toBeInTheDocument()
  })
})
