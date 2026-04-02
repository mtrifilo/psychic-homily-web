import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ArtistListItem } from '../types'

// Mock next/navigation
const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockGet = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => ({
    get: mockGet,
  }),
}))

// Mock hooks
const mockUseArtists = vi.fn()
const mockUseArtistCities = vi.fn()
vi.mock('../hooks/useArtists', () => ({
  useArtists: (opts: unknown) => mockUseArtists(opts),
  useArtistCities: () => mockUseArtistCities(),
}))

const mockUseDensity = vi.fn()
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: (key: string) => mockUseDensity(key),
}))

// Mock auth hooks
const mockUseProfile = vi.fn()
const mockUseIsAuthenticated = vi.fn()
vi.mock('@/features/auth', () => ({
  useProfile: () => mockUseProfile(),
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

// Mock SaveDefaultsButton
vi.mock('@/components/filters/SaveDefaultsButton', () => ({
  SaveDefaultsButton: () => <div data-testid="save-defaults-button">SaveDefaultsButton</div>,
}))

// Mock child components that are complex
vi.mock('./ArtistSearch', () => ({
  ArtistSearch: () => <div data-testid="artist-search">ArtistSearch</div>,
}))

vi.mock('@/components/filters', () => ({
  CityFilters: ({
    onFilterChange,
    selectedCities,
    children,
  }: {
    onFilterChange: (cities: { city: string; state: string }[]) => void
    selectedCities: { city: string; state: string }[]
    cities: unknown[]
    children?: React.ReactNode
  }) => (
    <div data-testid="city-filters">
      <span data-testid="selected-count">{selectedCities.length}</span>
      <button
        data-testid="clear-filters"
        onClick={() => onFilterChange([])}
      >
        Clear
      </button>
      {children}
    </div>
  ),
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
  DensityToggle: ({ density }: { density: string; onDensityChange: (v: string) => void }) => (
    <div data-testid="density-toggle">{density}</div>
  ),
}))

import { ArtistList } from './ArtistList'

function makeArtist(overrides: Partial<ArtistListItem> = {}): ArtistListItem {
  return {
    id: 1,
    slug: 'test-artist',
    name: 'Test Artist',
    city: 'Phoenix',
    state: 'AZ',
    bandcamp_embed_url: null,
    upcoming_show_count: 3,
    social: {
      instagram: null,
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: null,
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ArtistList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockReturnValue(null)
    mockUseDensity.mockReturnValue({ density: 'comfortable', setDensity: vi.fn() })
    mockUseProfile.mockReturnValue({ data: null })
    mockUseIsAuthenticated.mockReturnValue({ isAuthenticated: false })
    mockUseArtistCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })
    mockUseArtists.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
  })

  it('shows loading spinner on initial load', () => {
    mockUseArtists.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
      error: null,
      refetch: vi.fn(),
    })
    mockUseArtistCities.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  it('renders artist search component', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('artist-search')).toBeInTheDocument()
  })

  it('renders density toggle with current density', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('density-toggle')).toHaveTextContent('comfortable')
  })

  it('renders empty state when no artists', () => {
    renderWithProviders(<ArtistList />)
    expect(screen.getByText('No artists available at this time.')).toBeInTheDocument()
  })

  it('renders filtered empty state when cities selected', () => {
    mockGet.mockReturnValue('Phoenix,AZ')
    mockUseArtists.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<ArtistList />)
    expect(
      screen.getByText('No artists found in the selected cities.')
    ).toBeInTheDocument()
  })

  it('shows "View all artists" link when filtered and empty', () => {
    mockGet.mockReturnValue('Phoenix,AZ')
    mockUseArtists.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByText('View all artists')).toBeInTheDocument()
  })

  it('renders artist cards when data available', () => {
    const artists = [
      makeArtist({ id: 1, name: 'Artist One', slug: 'artist-one' }),
      makeArtist({ id: 2, name: 'Artist Two', slug: 'artist-two' }),
    ]
    mockUseArtists.mockReturnValue({
      data: { artists, count: 2 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByText('Artist One')).toBeInTheDocument()
    expect(screen.getByText('Artist Two')).toBeInTheDocument()
  })

  it('shows error state with retry button', () => {
    const refetch = vi.fn()
    mockUseArtists.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: new Error('Network error'),
      refetch,
    })

    renderWithProviders(<ArtistList />)
    expect(
      screen.getByText('Failed to load artists. Please try again later.')
    ).toBeInTheDocument()
    expect(screen.getByText('Retry')).toBeInTheDocument()
  })

  it('calls refetch on retry button click', async () => {
    const user = userEvent.setup()
    const refetch = vi.fn()
    mockUseArtists.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: new Error('Network error'),
      refetch,
    })

    renderWithProviders(<ArtistList />)
    await user.click(screen.getByText('Retry'))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('renders city filters when cities data available', () => {
    mockUseArtistCities.mockReturnValue({
      data: {
        cities: [
          { city: 'Phoenix', state: 'AZ', artist_count: 5 },
          { city: 'Mesa', state: 'AZ', artist_count: 3 },
        ],
      },
      isLoading: false,
      isFetching: false,
    })

    renderWithProviders(<ArtistList />)
    expect(screen.getByTestId('city-filters')).toBeInTheDocument()
  })

  it('does not render city filters when no cities', () => {
    mockUseArtistCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })

    renderWithProviders(<ArtistList />)
    expect(screen.queryByTestId('city-filters')).not.toBeInTheDocument()
  })

  it('parses cities from URL search params', () => {
    mockGet.mockReturnValue('Phoenix,AZ|Mesa,AZ')
    mockUseArtistCities.mockReturnValue({
      data: {
        cities: [{ city: 'Phoenix', state: 'AZ', artist_count: 5 }],
      },
      isLoading: false,
      isFetching: false,
    })

    renderWithProviders(<ArtistList />)
    // useArtists should be called with the parsed cities
    expect(mockUseArtists).toHaveBeenCalledWith({
      cities: [
        { city: 'Phoenix', state: 'AZ' },
        { city: 'Mesa', state: 'AZ' },
      ],
    })
  })

  it('passes no cities filter when no search params', () => {
    mockGet.mockReturnValue(null)

    renderWithProviders(<ArtistList />)
    expect(mockUseArtists).toHaveBeenCalledWith({
      cities: undefined,
    })
  })
})
