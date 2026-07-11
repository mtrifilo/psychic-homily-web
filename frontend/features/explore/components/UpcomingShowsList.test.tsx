import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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
/** Pull the `cities=` value out of a `/explore?cities=...` href so the
 * mocked router updates the param a subsequent `rerender` will read. */
function citiesFromHref(href: string): string | null {
  const qIndex = href.indexOf('?')
  if (qIndex === -1) return null
  return new URLSearchParams(href.slice(qIndex + 1)).get('cities')
}
const mockReplace = vi.fn((href: string) => {
  mockCitiesParam = citiesFromHref(href)
})
const mockPush = vi.fn((href: string) => {
  mockCitiesParam = citiesFromHref(href)
})
vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
  useSearchParams: () =>
    new URLSearchParams(mockCitiesParam ? `cities=${mockCitiesParam}` : ''),
}))

// nuqs `useQueryState` is bridged to the SAME mockCitiesParam the router mock
// tracks, so tests keep one URL source of truth. The real citiesParser
// parse/serialize run (via the passed parser), so the ALL_CITIES sentinel and
// wire format are genuinely exercised. The setter updates mockCitiesParam the
// way a real URL write would, so rerender-based tests observe the new state.
const mockSetCities = vi.fn()
vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return {
    ...actual,
    useQueryState: (
      _key: string,
      parser: {
        parse: (v: string) => unknown
        serialize?: (v: never) => string
      },
    ) => {
      const state = mockCitiesParam != null ? parser.parse(mockCitiesParam) : null
      const set = (value: unknown) => {
        mockSetCities(value)
        mockCitiesParam =
          value == null ? null : (parser.serialize?.(value as never) ?? String(value))
        return Promise.resolve(new URLSearchParams())
      }
      return [state, set]
    },
  }
})

let mockShowCities: Array<{ city: string; state: string; show_count: number }> = []
vi.mock('@/features/shows', () => ({
  useShowCities: () => ({ data: { cities: mockShowCities } }),
}))

let mockIsAuthenticated = false
let mockAuthLoading = false
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockIsAuthenticated,
    isLoading: mockAuthLoading,
    user: null,
  }),
}))

let mockProfileData: unknown = undefined
vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: mockProfileData }),
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
    selectedCities: Array<{ city: string; state: string }>
    children?: ReactNode
  }) => (
    <div data-testid="city-filters">
      <span data-testid="selected-count">{selectedCities.length}</span>
      <span data-testid="selected-labels">
        {selectedCities.map(c => `${c.city},${c.state}`).join('|')}
      </span>
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
    // mockClear (not mockReset) preserves the param-tracking implementation.
    mockReplace.mockClear()
    mockPush.mockClear()
    mockSetCities.mockClear()
    mockIsAuthenticated = false
    mockAuthLoading = false
    mockProfileData = undefined
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

  it('renders the city filter when one city has shows (PSY-932)', async () => {
    mockShowCities = [{ city: 'Phoenix', state: 'AZ', show_count: 5 }]
    mockUseExploreUpcomingShows.mockReturnValue({
      data: sampleResponse,
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    // CityFilters is dynamic-imported (ssr:false) — await its async mount.
    expect(await screen.findByTestId('city-filters')).toBeInTheDocument()
  })

  it('hides the city filter when no cities have shows', () => {
    mockShowCities = []
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

  describe('IP-geo default city (PSY-926)', () => {
    const omahaShow: ExploreUpcomingShowsResponse = {
      shows: [
        {
          id: 1,
          slug: 'omaha-show',
          title: 'Omaha Show',
          event_date: '2026-06-15T03:00:00Z',
          headliner_name: 'Omaha Headliner',
          venue_name: 'Reverb',
          venue_city: 'Omaha',
          venue_state: 'NE',
        },
      ],
      total: 1,
      limit: 5,
      offset: 0,
    }

    it('seeds the geo city for an anon visitor when it has upcoming shows', async () => {
      mockShowCities = [
        { city: 'Phoenix', state: 'AZ', show_count: 5 },
        { city: 'Omaha', state: 'NE', show_count: 3 },
      ]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: omahaShow,
        isLoading: false,
        error: null,
      })
      render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      // The effect seeds via router.replace with the canonical city,state.
      expect(mockReplace).toHaveBeenCalledWith('/explore?cities=Omaha%2CNE', {
        scroll: false,
      })
    })

    it('does NOT seed when the geo city has no upcoming shows', async () => {
      // Geo says Tucson, but only Phoenix/Omaha have shows → no seed.
      mockShowCities = [
        { city: 'Phoenix', state: 'AZ', show_count: 5 },
        { city: 'Omaha', state: 'NE', show_count: 3 },
      ]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: sampleResponse,
        isLoading: false,
        error: null,
      })
      render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Tucson', state: 'AZ' }} />,
      )
      expect(mockReplace).not.toHaveBeenCalled()
      expect(await screen.findByTestId('selected-count')).toHaveTextContent('0')
    })

    it('does NOT seed when there is no geo default (null → All cities)', async () => {
      mockShowCities = [
        { city: 'Phoenix', state: 'AZ', show_count: 5 },
        { city: 'Omaha', state: 'NE', show_count: 3 },
      ]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: sampleResponse,
        isLoading: false,
        error: null,
      })
      render(<UpcomingShowsList geoDefaultCity={null} />)
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('matches the geo city case/whitespace-insensitively, seeding PH canonical casing', () => {
      mockShowCities = [{ city: 'Omaha', state: 'NE', show_count: 3 }]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: omahaShow,
        isLoading: false,
        error: null,
      })
      render(
        <UpcomingShowsList geoDefaultCity={{ city: ' omaha ', state: 'ne' }} />,
      )
      // Seeds the canonical "Omaha,NE" from the cities list, not the raw header.
      expect(mockReplace).toHaveBeenCalledWith('/explore?cities=Omaha%2CNE', {
        scroll: false,
      })
    })

    it('lets authed favorites win over geo (resolution order)', () => {
      mockIsAuthenticated = true
      mockProfileData = {
        user: { preferences: { favorite_cities: [{ city: 'Phoenix', state: 'AZ' }] } },
      }
      mockShowCities = [
        { city: 'Phoenix', state: 'AZ', show_count: 5 },
        { city: 'Omaha', state: 'NE', show_count: 3 },
      ]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: sampleResponse,
        isLoading: false,
        error: null,
      })
      // Geo says Omaha, but the authed user favorites Phoenix → favorites win.
      // PSY-1389: favorites are DERIVED during render, never written to the
      // URL — so the selection is Phoenix with zero router/nuqs writes, and
      // the geo hook stands down.
      render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      expect(screen.getByTestId('selected-labels')).toHaveTextContent(
        'Phoenix,AZ',
      )
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockSetCities).not.toHaveBeenCalled()
    })

    it('does NOT apply geo for an authed user with no favorites', () => {
      mockIsAuthenticated = true
      mockProfileData = { user: { preferences: { favorite_cities: [] } } }
      mockShowCities = [{ city: 'Omaha', state: 'NE', show_count: 3 }]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: omahaShow,
        isLoading: false,
        error: null,
      })
      render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('waits for auth to settle before applying the anon geo default', () => {
      mockAuthLoading = true
      mockShowCities = [{ city: 'Omaha', state: 'NE', show_count: 3 }]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: omahaShow,
        isLoading: false,
        error: null,
      })
      render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('does NOT override an existing ?cities= URL selection', () => {
      mockCitiesParam = 'Phoenix,AZ'
      mockShowCities = [
        { city: 'Phoenix', state: 'AZ', show_count: 5 },
        { city: 'Omaha', state: 'NE', show_count: 3 },
      ]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: sampleResponse,
        isLoading: false,
        error: null,
      })
      render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('renders the "from your location — change" affordance for a geo-seeded city', () => {
      mockShowCities = [{ city: 'Omaha', state: 'NE', show_count: 3 }]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: omahaShow,
        isLoading: false,
        error: null,
      })
      const { rerender } = render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      // The seed effect updated mockCitiesParam via the router mock; re-render
      // so useSearchParams reflects the seeded selection and the affordance
      // (gated on selection === geo default) shows.
      rerender(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      const affordance = screen.getByTestId('geo-default-affordance')
      expect(affordance).toHaveTextContent('Omaha, NE')
      expect(affordance).toHaveTextContent(/from your location/i)
    })

    it('clears the geo affordance when the user clicks "change"', () => {
      mockShowCities = [{ city: 'Omaha', state: 'NE', show_count: 3 }]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: omahaShow,
        isLoading: false,
        error: null,
      })
      const { rerender } = render(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      rerender(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      fireEvent.click(screen.getByTestId('geo-default-change'))
      // "change" writes the explicit ALL_CITIES sentinel (a bare URL would
      // mean "apply my default" and could re-derive the same geo city).
      expect(mockSetCities).toHaveBeenCalledWith('all')
      rerender(
        <UpcomingShowsList geoDefaultCity={{ city: 'Omaha', state: 'NE' }} />,
      )
      expect(
        screen.queryByTestId('geo-default-affordance'),
      ).not.toBeInTheDocument()
    })
  })

  // PSY-1389: the favorites default is DERIVED during render, not seeded into
  // the URL by a mount effect — the fix for the default being dropped after
  // client-side navigation.
  describe('favorites default (derived, no URL write)', () => {
    beforeEach(() => {
      mockIsAuthenticated = true
      mockProfileData = {
        user: {
          preferences: { favorite_cities: [{ city: 'Phoenix', state: 'AZ' }] },
        },
      }
      mockShowCities = [
        { city: 'Phoenix', state: 'AZ', show_count: 5 },
        { city: 'Omaha', state: 'NE', show_count: 3 },
      ]
      mockUseExploreUpcomingShows.mockReturnValue({
        data: sampleResponse,
        isLoading: false,
        error: null,
      })
    })

    it('selects the favorite on a bare URL WITHOUT writing to the URL', () => {
      render(<UpcomingShowsList />)

      expect(screen.getByTestId('selected-labels')).toHaveTextContent(
        'Phoenix,AZ',
      )
      // Regression guard: derived, not seeded — navigation can't drop it.
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockPush).not.toHaveBeenCalled()
      expect(mockSetCities).not.toHaveBeenCalled()
    })

    it('treats ?cities=all as explicit all-cities, overriding the favorite', () => {
      mockCitiesParam = 'all'
      render(<UpcomingShowsList />)

      expect(screen.getByTestId('selected-count')).toHaveTextContent('0')
    })
  })
})
