import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShowList } from './ShowList'
import type { ShowResponse, ArtistResponse } from '../types'

// Mock AuthContext
const mockAuthContext = vi.fn(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock next/navigation
const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockSearchParams = vi.fn(() => ({
  get: vi.fn(() => null),
}))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams(),
}))

// Mock show hooks
const mockUseUpcomingShows = vi.fn()
const mockUseShowCities = vi.fn()
vi.mock('../hooks/useShows', () => ({
  useUpcomingShows: (opts: unknown) => mockUseUpcomingShows(opts),
  useShowCities: (opts: unknown) => mockUseShowCities(opts),
}))

vi.mock('../hooks/useSavedShows', () => ({
  useSavedShowBatch: () => ({ data: new Set<number>() }),
}))

vi.mock('../hooks/useAttendance', () => ({
  useBatchAttendance: () => ({ data: {} }),
}))

// Mock profile hooks
vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: null }),
  useSetFavoriteCities: () => ({ mutate: vi.fn() }),
}))

// PSY-309: mock tag facet components
vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div data-testid="tag-facet-panel" />,
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
}))

// Mock density hook
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: () => ({ density: 'comfortable', setDensity: vi.fn() }),
}))

// Mock child components
vi.mock('./ShowCard', () => ({
  ShowCard: ({ show }: { show: ShowResponse }) => (
    <article data-testid={`show-card-${show.id}`}>{show.title}</article>
  ),
}))

vi.mock('./ShowListSkeleton', () => ({
  ShowListSkeleton: () => <div data-testid="show-skeleton">Loading...</div>,
}))

vi.mock('@/components/filters', () => ({
  CityFilters: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="city-filters">{children}</div>
  ),
}))

vi.mock('@/components/filters/SaveDefaultsButton', () => ({
  SaveDefaultsButton: () => <button data-testid="save-defaults">Save defaults</button>,
}))

vi.mock('@/components/shared', () => ({
  DensityToggle: () => <div data-testid="density-toggle" />,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, onClick, disabled, ...props }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => (
    <button onClick={onClick} disabled={disabled}>{children}</button>
  ),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'approved',
    city: 'Phoenix',
    state: 'AZ',
    venues: [],
    artists: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('ShowList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockSearchParams.mockReturnValue({
      get: vi.fn(() => null),
    })
    mockUseShowCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })
  })

  describe('loading state', () => {
    it('shows skeleton when loading and no data', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: undefined,
        isLoading: true,
        isFetching: true,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('show-skeleton')).toBeInTheDocument()
    })

    it('shows skeleton when cities are loading', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [], pagination: { has_more: false, next_cursor: null, limit: 20 } },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      mockUseShowCities.mockReturnValue({
        data: undefined,
        isLoading: true,
        isFetching: true,
      })
      render(<ShowList />)
      expect(screen.getByTestId('show-skeleton')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message when fetch fails', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
        error: new Error('Network error'),
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('Failed to load shows. Please try again later.')).toBeInTheDocument()
    })

    it('shows retry button on error', () => {
      const mockRefetch = vi.fn()
      mockUseUpcomingShows.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      render(<ShowList />)
      expect(screen.getByText('Retry')).toBeInTheDocument()
    })

    it('calls refetch when retry clicked', async () => {
      const user = userEvent.setup()
      const mockRefetch = vi.fn()
      mockUseUpcomingShows.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      render(<ShowList />)
      await user.click(screen.getByText('Retry'))
      expect(mockRefetch).toHaveBeenCalled()
    })
  })

  describe('empty state', () => {
    it('shows empty message when no shows', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [], pagination: { has_more: false, next_cursor: null, limit: 20 } },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('No upcoming shows at this time.')).toBeInTheDocument()
    })

    it('shows city-specific empty message when cities are filtered', () => {
      mockSearchParams.mockReturnValue({
        get: vi.fn((key: string): string | null => {
          if (key === 'cities') return 'Phoenix,AZ'
          return null
        }),
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [], pagination: { has_more: false, next_cursor: null, limit: 20 } },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('No upcoming shows match the current filters.')).toBeInTheDocument()
    })

    it('shows "Clear filters" button when filtered to city with no results', () => {
      mockSearchParams.mockReturnValue({
        get: vi.fn((key: string): string | null => {
          if (key === 'cities') return 'Phoenix,AZ'
          return null
        }),
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [], pagination: { has_more: false, next_cursor: null, limit: 20 } },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('Clear filters')).toBeInTheDocument()
    })
  })

  describe('with show data', () => {
    it('renders show cards', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [
            makeShow({ id: 1, title: 'Show One' }),
            makeShow({ id: 2, title: 'Show Two' }),
          ],
          pagination: { has_more: false, next_cursor: null, limit: 20 },
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('show-card-1')).toBeInTheDocument()
      expect(screen.getByTestId('show-card-2')).toBeInTheDocument()
      expect(screen.getByText('Show One')).toBeInTheDocument()
      expect(screen.getByText('Show Two')).toBeInTheDocument()
    })

    it('shows density toggle', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [makeShow()],
          pagination: { has_more: false, next_cursor: null, limit: 20 },
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('density-toggle')).toBeInTheDocument()
    })
  })

  describe('pagination', () => {
    it('shows Load More button when has_more is true', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [makeShow()],
          pagination: { has_more: true, next_cursor: 'abc123', limit: 20 },
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('Load More')).toBeInTheDocument()
    })

    it('does not show Load More when no more pages', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [makeShow()],
          pagination: { has_more: false, next_cursor: null, limit: 20 },
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.queryByText('Load More')).not.toBeInTheDocument()
    })

    it('shows "Loading..." text when fetching more', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [makeShow()],
          pagination: { has_more: true, next_cursor: 'abc123', limit: 20 },
        },
        isLoading: false,
        isFetching: true,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('Loading...')).toBeInTheDocument()
    })
  })

  describe('city filters', () => {
    it('shows city filters when multiple cities available', () => {
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [
            { city: 'Phoenix', state: 'AZ', show_count: 10 },
            { city: 'Tempe', state: 'AZ', show_count: 5 },
          ],
        },
        isLoading: false,
        isFetching: false,
      })
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [makeShow()],
          pagination: { has_more: false, next_cursor: null, limit: 20 },
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('city-filters')).toBeInTheDocument()
    })

    it('hides city filters when only one city', () => {
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [{ city: 'Phoenix', state: 'AZ', show_count: 10 }],
        },
        isLoading: false,
        isFetching: false,
      })
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [makeShow()],
          pagination: { has_more: false, next_cursor: null, limit: 20 },
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.queryByTestId('city-filters')).not.toBeInTheDocument()
    })
  })
})
