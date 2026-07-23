import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LibraryTasteSidebar } from './LibraryTasteSidebar'

const mockUseAuthContext = vi.fn()
const mockUsePersonalChartsStats = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

vi.mock('@/features/charts/hooks', () => ({
  usePersonalChartsStats: (...args: unknown[]) =>
    mockUsePersonalChartsStats(...args),
}))

const baseStats = {
  saved_shows: 12,
  artists_followed: 4,
  venues_followed: 2,
  labels_followed: 1,
  scenes_followed: 1,
  festivals_followed: 0,
  top_venue: {
    venue_id: 9,
    name: 'Valley Bar',
    slug: 'valley-bar',
    saved_show_count: 5,
  },
  first_activity_at: '2024-03-15T00:00:00Z',
  top_scenes: [
    {
      metro: 'phoenix',
      name: 'Phoenix',
      slug: 'phoenix',
      city: 'Phoenix',
      state: 'AZ',
      count: 61,
    },
  ],
  top_tags: [
    {
      tag_id: 1,
      name: 'shoegaze',
      slug: 'shoegaze',
      category: 'genre',
      count: 8,
    },
  ],
  top_artists: [
    {
      artist_id: 2,
      name: 'Duster',
      slug: 'duster',
      count: 9,
    },
  ],
}

describe('LibraryTasteSidebar', () => {
  beforeEach(() => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: '1' },
    })
    mockUsePersonalChartsStats.mockReturnValue({
      data: baseStats,
      isLoading: false,
      isError: false,
    })
  })

  it('renders snapshot and top-N blocks from /charts/me', () => {
    render(<LibraryTasteSidebar />)

    expect(screen.getByTestId('library-taste-sidebar')).toBeInTheDocument()
    expect(screen.getByText('Your taste')).toBeInTheDocument()
    expect(screen.getByText('Saved shows')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
    expect(screen.getByText('Mar 2024')).toBeInTheDocument()
    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
    expect(screen.getByText('shoegaze')).toBeInTheDocument()
    expect(screen.getByText('Duster')).toBeInTheDocument()
    expect(screen.queryByText(/needs extension/i)).not.toBeInTheDocument()
  })

  it('hides empty top-N blocks without inventing rows', () => {
    mockUsePersonalChartsStats.mockReturnValue({
      data: {
        ...baseStats,
        top_scenes: [],
        top_tags: [],
        top_artists: [],
      },
      isLoading: false,
      isError: false,
    })

    render(<LibraryTasteSidebar />)

    expect(screen.getByText('Snapshot')).toBeInTheDocument()
    expect(screen.queryByText('Top scenes')).not.toBeInTheDocument()
    expect(screen.queryByText('Top tags')).not.toBeInTheDocument()
    expect(screen.queryByText('Top artists')).not.toBeInTheDocument()
  })

  it('returns null when unauthenticated', () => {
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null,
    })

    const { container } = render(<LibraryTasteSidebar />)
    expect(container).toBeEmptyDOMElement()
  })
})
