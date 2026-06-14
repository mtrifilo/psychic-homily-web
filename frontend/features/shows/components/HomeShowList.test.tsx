import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { HomeShowList } from './HomeShowList'
import type { ShowResponse } from '../types'


// Mock AuthContext.
// Return type widened so individual tests can override `user`/`isAuthenticated`
// without TS narrowing from the default-null literal.
type MockAuthContextValue = {
  user: { id: string } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthContextValue>(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
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
  useProfile: () => ({ data: null as unknown }),
  useSetFavoriteCities: () => ({ mutate: vi.fn() }),
}))

// Mock prefetch hook
vi.mock('@/lib/hooks/common/usePrefetchRoutes', () => ({
  usePrefetchRoutes: vi.fn(),
}))

// Mock child components
vi.mock('./ShowCard', () => ({
  ShowCard: ({ show }: { show: ShowResponse }) => (
    <article data-testid={`show-card-${show.id}`}>{show.title}</article>
  ),
}))

vi.mock('@/components/filters', () => ({
  CityFilters: ({
    selectedCities,
    children,
  }: {
    selectedCities?: Array<{ city: string; state: string }>
    children?: React.ReactNode
  }) => (
    <div data-testid="city-filters">
      <span data-testid="selected-labels">
        {(selectedCities ?? []).map(c => `${c.city},${c.state}`).join('|')}
      </span>
      {children}
    </div>
  ),
}))

vi.mock('@/components/filters/SaveDefaultsButton', () => ({
  SaveDefaultsButton: () => <button data-testid="save-defaults">Save defaults</button>,
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

describe('HomeShowList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseShowCities.mockReturnValue({
      data: { cities: [] },
    })
  })

  describe('loading state', () => {
    it('shows spinner when loading', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: undefined,
        isLoading: true,
        isFetching: true,
        error: null,
      })
      const { container } = render(<HomeShowList />)
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
        error: new Error('Network error'),
      })
      render(<HomeShowList />)
      expect(screen.getByText('Unable to load shows.')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('shows empty message when no shows', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      expect(screen.getByText('No upcoming shows at this time.')).toBeInTheDocument()
    })

    it('shows empty message when shows is null', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: null },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      expect(screen.getByText('No upcoming shows at this time.')).toBeInTheDocument()
    })
  })

  describe('with show data', () => {
    it('renders show cards', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: {
          shows: [
            makeShow({ id: 1, title: 'Show One' }),
            makeShow({ id: 2, title: 'Show Two' }),
            makeShow({ id: 3, title: 'Show Three' }),
          ],
        },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      expect(screen.getByTestId('show-card-1')).toBeInTheDocument()
      expect(screen.getByTestId('show-card-2')).toBeInTheDocument()
      expect(screen.getByTestId('show-card-3')).toBeInTheDocument()
    })

    it('applies dimming when fetching', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: true,
        error: null,
      })
      const { container } = render(<HomeShowList />)
      const dimContainer = container.querySelector('.opacity-60')
      expect(dimContainer).toBeInTheDocument()
    })

    it('does not apply dimming when not fetching', () => {
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      const { container } = render(<HomeShowList />)
      const dimContainer = container.querySelector('.opacity-60')
      expect(dimContainer).toBeNull()
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
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      expect(screen.getByTestId('city-filters')).toBeInTheDocument()
    })

    it('shows city filters when one city has shows (PSY-932)', () => {
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [{ city: 'Phoenix', state: 'AZ', show_count: 10 }],
        },
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      expect(screen.getByTestId('city-filters')).toBeInTheDocument()
    })

    it('hides city filters when no cities', () => {
      mockUseShowCities.mockReturnValue({
        data: { cities: [] },
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      expect(screen.queryByTestId('city-filters')).not.toBeInTheDocument()
    })

    it('shows save defaults button for authenticated user with different selection', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [
            { city: 'Phoenix', state: 'AZ', show_count: 10 },
            { city: 'Tempe', state: 'AZ', show_count: 5 },
          ],
        },
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
      render(<HomeShowList />)
      // With no selected cities and no favorites, selectionDiffersFromFavorites is false (both empty)
      // so SaveDefaultsButton should NOT show
      expect(screen.queryByTestId('save-defaults')).not.toBeInTheDocument()
    })
  })

  // IP-geo soft default (PSY-946). Home seeds the geo city into local
  // selection state (no URL) via the real useGeoDefaultCity hook, driven here
  // by a mocked /api/geo fetch + the useShowCities has-shows list.
  describe('IP-geo default city (PSY-946)', () => {
    function mockGeoFetch(geo: { city: string; state: string } | null) {
      return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
        new Response(JSON.stringify({ geo }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }

    beforeEach(() => {
      window.sessionStorage.clear()
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [
            { city: 'Phoenix', state: 'AZ', show_count: 5 },
            { city: 'Omaha', state: 'NE', show_count: 3 },
          ],
        },
      })
      mockUseUpcomingShows.mockReturnValue({
        data: { shows: [makeShow()] },
        isLoading: false,
        isFetching: false,
        error: null,
      })
    })

    afterEach(() => {
      vi.restoreAllMocks()
      window.sessionStorage.clear()
    })

    it('seeds the canonical geo city into local selection state for an anon visitor', async () => {
      mockGeoFetch({ city: 'Omaha', state: 'NE' })
      render(<HomeShowList />)
      await waitFor(() =>
        expect(screen.getByTestId('selected-labels')).toHaveTextContent(
          'Omaha,NE',
        ),
      )
      // The overridable affordance renders for the geo-seeded selection.
      expect(screen.getByTestId('geo-default-affordance')).toHaveTextContent(
        'Omaha, NE',
      )
    })

    it('does NOT seed when the geo city has no shows', async () => {
      mockGeoFetch({ city: 'Tucson', state: 'AZ' })
      render(<HomeShowList />)
      await waitFor(() =>
        expect(
          window.sessionStorage.getItem('ph-geo-default-city'),
        ).not.toBeNull(),
      )
      expect(screen.getByTestId('selected-labels')).toHaveTextContent('')
      expect(
        screen.queryByTestId('geo-default-affordance'),
      ).not.toBeInTheDocument()
    })

    it('does NOT fetch geo for an authed visitor (no wasted edge request)', () => {
      const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<HomeShowList />)
      expect(fetchSpy).not.toHaveBeenCalled()
    })
  })
})
