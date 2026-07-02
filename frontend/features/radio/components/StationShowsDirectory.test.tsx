import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { StationShowsDirectory } from './StationShowsDirectory'
import type { RadioShowListItem } from '../types'
import { localIso } from '../lib/localIso.testutil'

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

const mockUseRadioShows = vi.fn()
vi.mock('../hooks/useRadioShows', () => ({
  useRadioShows: (...args: unknown[]) => mockUseRadioShows(...args),
}))

function makeShow(overrides: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 1,
    station_id: 1,
    station_name: 'WFMU',
    name: 'The Night Owl Show',
    slug: 'night-owl',
    host_name: 'Pedro Santos',
    schedule_display: 'Mon 9pm-12am',
    genre_tags: ['krautrock', 'psych'],
    image_url: null,
    is_active: true,
    episode_count: 142,
    latest_air_date: '2026-06-09',
    latest_starts_at: null,
    latest_ends_at: null,
    ...overrides,
  }
}

function setShows(shows: RadioShowListItem[]) {
  mockUseRadioShows.mockReturnValue({
    data: { shows, count: shows.length },
    isLoading: false,
  })
}

describe('StationShowsDirectory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('requests the server-side latest sort', () => {
    setShows([makeShow()])
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)
    expect(mockUseRadioShows).toHaveBeenCalledWith(1, { sort: 'latest' })
  })

  it('renders show, host, schedule, genres, LAST date, and episode count', () => {
    setShows([makeShow()])
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)

    const showLink = screen.getByRole('link', { name: 'The Night Owl Show' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/night-owl')
    expect(screen.getByText('Pedro Santos')).toBeInTheDocument()
    expect(screen.getByText('Mon 9pm-12am')).toBeInTheDocument()
    expect(screen.getByText('krautrock · psych')).toBeInTheDocument()
    expect(screen.getByText('Jun 9')).toBeInTheDocument()
    expect(screen.getByText('142')).toBeInTheDocument()
  })

  it('renders LAST viewer-local from the latest episode window (PSY-1306)', () => {
    // latest_air_date differs from the window's local day: discriminates the
    // starts_at-derived date (a station-date fallback would render Jun 8).
    setShows([
      makeShow({
        latest_air_date: '2026-06-08',
        latest_starts_at: localIso(2026, 5, 9, 15),
        latest_ends_at: localIso(2026, 5, 9, 18),
      }),
    ])
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)
    expect(screen.getByText('Jun 9')).toBeInTheDocument()
    expect(screen.queryByText('Jun 8')).not.toBeInTheDocument()
  })

  it('preserves the server order and dashes out missing LAST dates', () => {
    setShows([
      makeShow({ id: 1, name: 'Fresh Show', slug: 'fresh' }),
      makeShow({
        id: 2,
        name: 'Dormant Show',
        slug: 'dormant',
        host_name: null,
        schedule_display: null,
        genre_tags: null,
        latest_air_date: null,
        latest_starts_at: null,
        latest_ends_at: null,
        episode_count: 0,
      }),
    ])
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)

    const rows = screen.getAllByRole('row').slice(1) // skip header
    expect(within(rows[0]).getByText('Fresh Show')).toBeInTheDocument()
    expect(within(rows[1]).getByText('Dormant Show')).toBeInTheDocument()
    // Dormant row: host / schedule / genres / LAST all dash out.
    expect(within(rows[1]).getAllByText('—')).toHaveLength(4)
  })

  it('summarizes active vs archived counts', () => {
    setShows([
      makeShow({ id: 1 }),
      makeShow({ id: 2, name: 'Retired', slug: 'retired', is_active: false }),
    ])
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)
    expect(screen.getByText('1 active · 1 archived')).toBeInTheDocument()
  })

  it('collapses to 10 rows and expands in place via View all', () => {
    const shows = Array.from({ length: 14 }, (_, i) =>
      makeShow({ id: i + 1, name: `Show ${i + 1}`, slug: `show-${i + 1}` })
    )
    setShows(shows)
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)

    expect(screen.getAllByRole('row')).toHaveLength(11) // header + 10
    expect(screen.queryByText('Show 11')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'View all 14' }))
    expect(screen.getAllByRole('row')).toHaveLength(15)
    expect(screen.getByText('Show 14')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show fewer' }))
    expect(screen.getAllByRole('row')).toHaveLength(11)
  })

  it('renders the empty state when a station has no shows', () => {
    setShows([])
    render(<StationShowsDirectory stationId={1} stationSlug="wfmu" />)
    expect(screen.getByText('No shows yet.')).toBeInTheDocument()
  })
})
