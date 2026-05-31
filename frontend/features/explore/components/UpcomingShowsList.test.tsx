import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { UpcomingShowsList } from './UpcomingShowsList'
import type { ExploreUpcomingShowsResponse } from '../types'

type MockHookResult = {
  data: ExploreUpcomingShowsResponse | undefined
  isLoading: boolean
  error: Error | null
}

const mockUseExploreUpcomingShows = vi.fn<() => MockHookResult>(() => ({
  data: undefined,
  isLoading: false,
  error: null,
}))

vi.mock('../hooks', () => ({
  useExploreUpcomingShows: () => mockUseExploreUpcomingShows(),
}))

// City-filter dependencies. Defaults below keep the filter bar hidden
// (≤1 city, no URL param, anon) so the list-focused tests are unaffected;
// individual tests override the module-level vars.
let mockCitiesParam: string | null = null
const mockReplace = vi.fn()
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
  useSearchParams: () =>
    new URLSearchParams(mockCitiesParam ? `cities=${mockCitiesParam}` : ''),
}))

let mockShowCities: Array<{ city: string; state: string; show_count: number }> = []
vi.mock('@/features/shows', () => ({
  useShowCities: () => ({ data: { cities: mockShowCities } }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: null }),
}))

vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: undefined }),
}))

// Stub the heavy filter UI (cmdk + Radix popover, dynamic-imported by the
// component) at its specific module path. The parse/build/equal helpers
// are imported from cityParams directly and kept real, so the component's
// URL→selection logic runs against the real parser.
vi.mock('@/components/filters/CityFilters', () => ({
  CityFilters: ({
    selectedCities,
    children,
  }: {
    selectedCities: unknown[]
    children?: ReactNode
  }) => (
    <div data-testid="city-filters">
      <span data-testid="selected-count">{selectedCities.length}</span>
      {children}
    </div>
  ),
}))
vi.mock('@/components/filters/SaveDefaultsButton', () => ({
  SaveDefaultsButton: () => <div data-testid="save-defaults" />,
}))

const sampleResponse: ExploreUpcomingShowsResponse = {
  shows: [
    {
      id: 1,
      slug: 'show-one',
      title: 'Show One',
      event_date: '2026-06-15T03:00:00Z',
      headliner_name: 'Headliner A',
      venue_name: 'The Trunk Space',
      venue_city: 'Phoenix',
      venue_state: 'AZ',
    },
    {
      id: 2,
      slug: 'show-two',
      title: 'Show Two',
      event_date: '2026-06-16T03:00:00Z',
      headliner_name: 'Headliner B',
      venue_name: 'Crescent Ballroom',
      venue_city: 'Phoenix',
      venue_state: 'AZ',
    },
  ],
  total: 2,
  limit: 5,
  offset: 0,
}

describe('UpcomingShowsList', () => {
  beforeEach(() => {
    mockUseExploreUpcomingShows.mockReset()
    mockCitiesParam = null
    mockShowCities = []
    mockReplace.mockReset()
    mockPush.mockReset()
  })

  it('renders a loading spinner while fetching', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    const { container } = render(<UpcomingShowsList />)
    expect(container.querySelector('.animate-spin')).toBeTruthy()
  })

  it('renders an error message when the hook fails', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })
    render(<UpcomingShowsList />)
    expect(screen.getByText(/unable to load shows/i)).toBeInTheDocument()
  })

  it('renders the empty state when no shows come back', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: { shows: [], total: 0, limit: 5, offset: 0 },
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    expect(screen.getByText(/no upcoming shows/i)).toBeInTheDocument()
  })

  it('renders one row per show with a link to the show detail page', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: sampleResponse,
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)

    const linkOne = screen.getByRole('link', { name: 'Show One' })
    expect(linkOne).toHaveAttribute('href', '/shows/show-one')
    expect(linkOne).toHaveTextContent('Headliner A')

    const linkTwo = screen.getByRole('link', { name: 'Show Two' })
    expect(linkTwo).toHaveAttribute('href', '/shows/show-two')
    expect(linkTwo).toHaveTextContent('Headliner B')
  })

  it('renders the city filter when more than one city has upcoming shows', async () => {
    mockShowCities = [
      { city: 'Phoenix', state: 'AZ', show_count: 5 },
      { city: 'Omaha', state: 'NE', show_count: 3 },
    ]
    mockUseExploreUpcomingShows.mockReturnValue({
      data: sampleResponse,
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    // CityFilters is dynamic-imported (ssr:false) — await its async mount.
    expect(await screen.findByTestId('city-filters')).toBeInTheDocument()
  })

  it('hides the city filter when only one city has shows', () => {
    mockShowCities = [{ city: 'Phoenix', state: 'AZ', show_count: 5 }]
    mockUseExploreUpcomingShows.mockReturnValue({
      data: sampleResponse,
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    expect(screen.queryByTestId('city-filters')).not.toBeInTheDocument()
  })

  it('parses the ?cities= URL param into the selected-city state', async () => {
    mockCitiesParam = 'Omaha,NE'
    mockShowCities = [
      { city: 'Phoenix', state: 'AZ', show_count: 5 },
      { city: 'Omaha', state: 'NE', show_count: 3 },
    ]
    mockUseExploreUpcomingShows.mockReturnValue({
      data: sampleResponse,
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    expect(await screen.findByTestId('selected-count')).toHaveTextContent('1')
  })

  it('shows a clear-filter affordance when a city filter yields no shows', () => {
    mockCitiesParam = 'Omaha,NE'
    mockShowCities = [
      { city: 'Phoenix', state: 'AZ', show_count: 5 },
      { city: 'Omaha', state: 'NE', show_count: 3 },
    ]
    mockUseExploreUpcomingShows.mockReturnValue({
      data: { shows: [], total: 0, limit: 5, offset: 0 },
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    expect(screen.getByText(/no upcoming shows in the selected city/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /show all cities/i })).toBeInTheDocument()
  })
})
