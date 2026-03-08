import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ArtistShow } from '@/lib/types/artist'

// Mock the artist shows hook
const mockUseArtistShows = vi.fn()
vi.mock('@/lib/hooks/useArtists', () => ({
  useArtistShows: (opts: unknown) => mockUseArtistShows(opts),
}))

// Mock CompactShowRow to avoid pulling in its dependencies
vi.mock('@/components/shows/CompactShowRow', () => ({
  CompactShowRow: ({
    show,
  }: {
    show: { id: number; title: string }
  }) => (
    <div data-testid={`show-row-${show.id}`}>{show.title}</div>
  ),
}))

import { ArtistShowsList } from './ArtistShowsList'

function makeShow(overrides: Partial<ArtistShow> = {}): ArtistShow {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2025-06-15T20:00:00Z',
    price: 15,
    age_requirement: null,
    venue: {
      id: 1,
      slug: 'test-venue',
      name: 'Test Venue',
      city: 'Phoenix',
      state: 'AZ',
    },
    artists: [
      { id: 42, slug: 'main-artist', name: 'Main Artist' },
      { id: 99, slug: 'opener', name: 'The Opener' },
    ],
    ...overrides,
  }
}

describe('ArtistShowsList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseArtistShows.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    })
  })

  it('renders upcoming and past tabs', () => {
    renderWithProviders(<ArtistShowsList artistId={42} />)
    expect(screen.getByText('Upcoming')).toBeInTheDocument()
    expect(screen.getByText('Past Shows')).toBeInTheDocument()
  })

  it('defaults to the upcoming tab', () => {
    renderWithProviders(<ArtistShowsList artistId={42} />)
    // Upcoming tab should be active - the hook should be called with upcoming enabled
    expect(mockUseArtistShows).toHaveBeenCalledWith(
      expect.objectContaining({
        timeFilter: 'upcoming',
        enabled: true,
      })
    )
  })

  it('shows loading spinner when loading', () => {
    mockUseArtistShows.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    // The loading spinner should be present (a Loader2 with animate-spin)
    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('shows error message on error', () => {
    mockUseArtistShows.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed'),
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    expect(screen.getByText('Failed to load shows')).toBeInTheDocument()
  })

  it('shows empty state for no upcoming shows', () => {
    mockUseArtistShows.mockReturnValue({
      data: { shows: [], total: 0, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    expect(screen.getByText('No upcoming shows')).toBeInTheDocument()
  })

  it('renders shows when data available', () => {
    const shows = [
      makeShow({ id: 1, title: 'Show One' }),
      makeShow({ id: 2, title: 'Show Two' }),
    ]
    mockUseArtistShows.mockReturnValue({
      data: { shows, total: 2, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    expect(screen.getByTestId('show-row-1')).toBeInTheDocument()
    expect(screen.getByTestId('show-row-2')).toBeInTheDocument()
  })

  it('switches to past tab on click', async () => {
    const user = userEvent.setup()
    // Return empty for both upcoming and past
    mockUseArtistShows.mockReturnValue({
      data: { shows: [], total: 0, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)

    await user.click(screen.getByText('Past Shows'))

    // After click, past tab hook should be called with enabled=true and upcoming with enabled=false
    const calls = mockUseArtistShows.mock.calls
    const lastCalls = calls.slice(-2)
    const pastCall = lastCalls.find(
      (c: unknown[]) => (c[0] as { timeFilter: string }).timeFilter === 'past'
    )
    expect(pastCall).toBeTruthy()
    expect((pastCall![0] as { enabled: boolean }).enabled).toBe(true)
  })

  it('shows count indicator when there are more shows than displayed', () => {
    const shows = [makeShow({ id: 1, title: 'Show One' })]
    mockUseArtistShows.mockReturnValue({
      data: { shows, total: 25, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    expect(screen.getByText('Showing 1 of 25 shows')).toBeInTheDocument()
  })

  it('does not show count indicator when all shows displayed', () => {
    const shows = [makeShow({ id: 1, title: 'Show One' })]
    mockUseArtistShows.mockReturnValue({
      data: { shows, total: 1, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    expect(screen.queryByText(/Showing/)).not.toBeInTheDocument()
  })

  it('shows "No past shows" empty state on past tab', async () => {
    const user = userEvent.setup()
    mockUseArtistShows.mockReturnValue({
      data: { shows: [], total: 0, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ArtistShowsList artistId={42} />)
    await user.click(screen.getByText('Past Shows'))

    expect(screen.getByText('No past shows')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    mockUseArtistShows.mockReturnValue({
      data: { shows: [], total: 0, artist_id: 42 },
      isLoading: false,
      error: null,
    })

    const { container } = renderWithProviders(
      <ArtistShowsList artistId={42} className="custom-class" />
    )
    expect(container.firstChild).toHaveClass('custom-class')
  })
})
