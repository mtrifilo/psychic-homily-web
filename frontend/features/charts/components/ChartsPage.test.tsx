import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    CHARTS: {
      OVERVIEW: '/charts/overview',
      TRENDING_SHOWS: '/charts/trending-shows',
      POPULAR_ARTISTS: '/charts/popular-artists',
      ACTIVE_VENUES: '/charts/active-venues',
      HOT_RELEASES: '/charts/hot-releases',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    charts: {
      all: ['charts'],
      overview: ['charts', 'overview'],
      trendingShows: (limit?: number) => ['charts', 'trending-shows', limit],
      popularArtists: (limit?: number) => ['charts', 'popular-artists', limit],
      activeVenues: (limit?: number) => ['charts', 'active-venues', limit],
      hotReleases: (limit?: number) => ['charts', 'hot-releases', limit],
    },
  },
}))

import { ChartsPage } from './ChartsPage'

const mockOverviewData = {
  trending_shows: [
    {
      show_id: 1,
      title: 'Night of Echoes',
      slug: 'night-of-echoes',
      date: '2026-04-15T20:00:00Z',
      venue_name: 'Valley Bar',
      venue_slug: 'valley-bar',
      city: 'Phoenix',
      going_count: 42,
      interested_count: 88,
      total_attendance: 130,
    },
  ],
  popular_artists: [
    {
      artist_id: 1,
      name: 'Moonlight Parade',
      slug: 'moonlight-parade',
      image_url: '',
      follow_count: 120,
      upcoming_show_count: 5,
      score: 125,
    },
  ],
  active_venues: [
    {
      venue_id: 1,
      name: 'The Rebel Lounge',
      slug: 'the-rebel-lounge',
      city: 'Phoenix',
      state: 'AZ',
      upcoming_show_count: 18,
      follow_count: 65,
      score: 83,
    },
  ],
  hot_releases: [
    {
      release_id: 1,
      title: 'Eternal Drift',
      slug: 'eternal-drift',
      release_date: '2026-03-01',
      artist_names: ['Moonlight Parade'],
      bookmark_count: 34,
    },
  ],
}

describe('ChartsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('renders overview by default with all chart sections', async () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Night of Echoes')).toBeInTheDocument()
    })

    // Section headings appear in both tabs and card headers — use getAllByText
    expect(screen.getAllByText('Upcoming Shows').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('Popular Artists').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('Active Venues').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('Recent Releases').length).toBeGreaterThanOrEqual(2)
  })

  it('renders entity names as links', async () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Night of Echoes')).toBeInTheDocument()
    })

    const links = screen.getAllByRole('link')

    // Show title links to show detail
    const showLink = links.find(l => l.textContent?.includes('Night of Echoes'))
    expect(showLink).toHaveAttribute('href', '/shows/night-of-echoes')

    // Artist name links to artist detail
    const artistLink = links.find(l => l.textContent?.includes('Moonlight Parade'))
    expect(artistLink).toHaveAttribute('href', '/artists/moonlight-parade')

    // Venue name links to venue detail
    const venueLink = links.find(l => l.textContent?.includes('The Rebel Lounge'))
    expect(venueLink).toHaveAttribute('href', '/venues/the-rebel-lounge')

    // Release title links to release detail
    const releaseLink = links.find(l => l.textContent?.includes('Eternal Drift'))
    expect(releaseLink).toHaveAttribute('href', '/releases/eternal-drift')
  })

  it('shows error message when API fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Failed to load charts. Please try again later.')).toBeInTheDocument()
    })
  })

  it('switches to trending shows detail view', async () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    const user = userEvent.setup()
    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Night of Echoes')).toBeInTheDocument()
    })

    // Get tab buttons (first match in the tab bar, not the card header)
    const trendingButtons = screen.getAllByRole('button', { name: /Upcoming Shows/i })
    const tabButton = trendingButtons[0]

    mockApiRequest.mockResolvedValueOnce({
      shows: [
        mockOverviewData.trending_shows[0],
        {
          show_id: 2,
          title: 'Desert Bloom Fest',
          slug: 'desert-bloom-fest',
          date: '2026-05-01T19:00:00Z',
          venue_name: 'Crescent Ballroom',
          venue_slug: 'crescent-ballroom',
          city: 'Phoenix',
          going_count: 30,
          interested_count: 60,
          total_attendance: 90,
        },
      ],
    })

    await user.click(tabButton)

    await waitFor(() => {
      expect(screen.getByText('Shows coming up soon, ordered by date.')).toBeInTheDocument()
    })
  })

  it('switches to popular artists detail view', async () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    const user = userEvent.setup()
    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Moonlight Parade')).toBeInTheDocument()
    })

    const artistsTab = screen.getAllByRole('button', { name: /Popular Artists/i })[0]

    mockApiRequest.mockResolvedValueOnce({
      artists: [mockOverviewData.popular_artists[0]],
    })

    await user.click(artistsTab)

    await waitFor(() => {
      expect(screen.getByText('Artists with the most followers and upcoming shows.')).toBeInTheDocument()
    })
  })

  it('switches to active venues detail view', async () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    const user = userEvent.setup()
    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('The Rebel Lounge')).toBeInTheDocument()
    })

    const venuesTab = screen.getAllByRole('button', { name: /Active Venues/i })[0]

    mockApiRequest.mockResolvedValueOnce({
      venues: [mockOverviewData.active_venues[0]],
    })

    await user.click(venuesTab)

    await waitFor(() => {
      expect(screen.getByText('Venues with the most upcoming shows and followers.')).toBeInTheDocument()
    })
  })

  it('switches to hot releases detail view', async () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    const user = userEvent.setup()
    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Eternal Drift')).toBeInTheDocument()
    })

    const releasesTab = screen.getAllByRole('button', { name: /Recent Releases/i })[0]

    mockApiRequest.mockResolvedValueOnce({
      releases: [mockOverviewData.hot_releases[0]],
    })

    await user.click(releasesTab)

    await waitFor(() => {
      expect(screen.getByText('Recently added releases.')).toBeInTheDocument()
    })
  })

  it('returns to overview via Overview tab', async () => {
    // First call: overview data
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    const user = userEvent.setup()
    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('Night of Echoes')).toBeInTheDocument()
    })

    // Second call: trending shows detail data
    mockApiRequest.mockResolvedValueOnce({
      shows: [mockOverviewData.trending_shows[0]],
    })

    // Switch to trending shows detail
    const trendingTab = screen.getAllByRole('button', { name: /Upcoming Shows/i })[0]
    await user.click(trendingTab)

    await waitFor(() => {
      expect(screen.getByText('Shows coming up soon, ordered by date.')).toBeInTheDocument()
    })

    // Third call: overview data again when switching back
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    // Switch back to overview
    const overviewTab = screen.getByRole('button', { name: /Overview/i })
    await user.click(overviewTab)

    // Should show overview grid with "View all" buttons
    await waitFor(() => {
      expect(screen.getAllByText('View all')).toHaveLength(4)
    })
  })

  it('renders empty state messages when no data', async () => {
    mockApiRequest.mockResolvedValueOnce({
      trending_shows: [],
      popular_artists: [],
      active_venues: [],
      hot_releases: [],
    })

    renderWithProviders(<ChartsPage />)

    await waitFor(() => {
      expect(screen.getByText('No upcoming shows right now.')).toBeInTheDocument()
    })

    expect(screen.getByText('No popular artists right now.')).toBeInTheDocument()
    expect(screen.getByText('No active venues right now.')).toBeInTheDocument()
    expect(screen.getByText('No recent releases yet.')).toBeInTheDocument()
  })

  it('renders all tab buttons', () => {
    mockApiRequest.mockResolvedValueOnce(mockOverviewData)

    renderWithProviders(<ChartsPage />)

    expect(screen.getByRole('button', { name: /Overview/i })).toBeInTheDocument()
    // Tab buttons exist (may also appear as card headings)
    expect(screen.getAllByRole('button', { name: /Upcoming Shows/i }).length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByRole('button', { name: /Popular Artists/i }).length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByRole('button', { name: /Active Venues/i }).length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByRole('button', { name: /Recent Releases/i }).length).toBeGreaterThanOrEqual(1)
  })
})
