import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VenueList } from './VenueList'
import type { VenueWithShowCount } from '../types'
import type { CityState } from '@/components/filters'

// Mock AuthContext
const mockAuthContext = vi.fn(
  (): {
    user: { id: string } | null
    authStatus: 'pending' | 'anonymous' | 'authenticated'
    logout: () => void
  } => ({ user: null, authStatus: 'anonymous', logout: vi.fn() })
)
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(mockAuthContext()) }
})

// Mock next/navigation. A REAL `URLSearchParams`, because the pager's href
// builder spreads the live params rather than reading one key at a time: a
// `{ get }` stub would answer every lookup and still serve `?page=2` with every
// other param dropped.
const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockSearchParams = vi.fn(() => new URLSearchParams())
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams(),
}))

// nuqs `useQueryState` bridged to the SAME mocked searchParams the component
// reads for tags and legacy params, so tests keep a single URL source of truth.
// The real `citiesParser` and `parseAsStringLiteral` stay in the loop; the
// setters are spies the tests assert on.
const mockSetCities = vi.fn()
const mockSetPage = vi.fn()
const mockSetSort = vi.fn()
vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return {
    ...actual,
    useQueryState: (key: string, parser: { parse: (v: string) => unknown }) => {
      const raw = mockSearchParams().get(key)
      const setter =
        key === 'page' ? mockSetPage : key === 'sort' ? mockSetSort : mockSetCities
      return [raw != null ? parser.parse(raw) : null, setter]
    },
  }
})

// Mock venue hooks
const mockUseVenues = vi.fn()
const mockUseVenueCities = vi.fn()
vi.mock('../hooks/useVenues', () => ({
  useVenues: (opts: unknown) => mockUseVenues(opts),
  useVenueCities: () => mockUseVenueCities(),
}))

// The derived-city source. Mocked so a test can put the hook in each of its
// three states (pending, derived, settled-with-nothing); the hook's own
// derivation is covered in `useGeoDefaultCity.test.tsx`.
const mockGeoDefault = vi.fn(() => ({
  appliedGeoDefault: null as CityState | null,
  notifyUserInteracted: vi.fn(),
  isResolving: false,
}))
vi.mock('@/components/filters/useGeoDefaultCity', () => ({
  useGeoDefaultCity: () => mockGeoDefault(),
}))

// Mock profile hooks (controllable so tests can set favorite_cities)
const mockUseProfile = vi.fn(() => ({ data: null as unknown }))
vi.mock('@/features/auth', () => ({
  useProfile: () => mockUseProfile(),
}))

const mockUseTags = vi.fn(() => ({
  data: { tags: [{ slug: 'punk', usage_count: 0 }] } as unknown,
}))
vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div data-testid="tag-facet-panel" />,
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
  useTags: () => mockUseTags(),
}))

vi.mock('./VenueSearch', () => ({
  VenueSearch: () => <div data-testid="venue-search" />,
}))

// The city filter's own anatomy is pinned by `CityFilters.test.tsx`. This stub
// echoes the selection it was handed and exposes the two writes the list owns.
vi.mock('@/components/filters', async importOriginal => {
  const actual = await importOriginal<typeof import('@/components/filters')>()
  return {
    ...actual,
    CityFilters: ({
      selectedCities,
      onFilterChange,
      showPopularCities,
    }: {
      selectedCities: CityState[]
      onFilterChange: (cities: CityState[]) => void
      showPopularCities?: boolean
    }) => (
      <div
        data-testid="city-filters"
        data-selected={selectedCities.map(c => `${c.city},${c.state}`).join('|')}
        data-popular={String(showPopularCities !== false)}
      >
        <button
          data-testid="mock-pick-tucson"
          onClick={() => onFilterChange([{ city: 'Tucson', state: 'AZ' }])}
        >
          pick Tucson
        </button>
        <button data-testid="mock-clear-cities" onClick={() => onFilterChange([])}>
          all cities
        </button>
      </div>
    ),
  }
})

function makeVenue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'test-venue',
    name: 'Test Venue',
    address: '101 Main St',
    city: 'Phoenix',
    state: 'AZ',
    timezone: 'America/Phoenix',
    verified: true,
    upcoming_show_count: 3,
    next_show: {
      event_date: '2026-09-15T02:30:00Z',
      slug: 'a-show',
      title: '',
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

const PHOENIX: CityState = { city: 'Phoenix', state: 'AZ' }

describe('VenueList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams.mockReturnValue(new URLSearchParams())
    mockAuthContext.mockReturnValue({
      user: null,
      authStatus: 'anonymous',
      logout: vi.fn(),
    })
    mockUseProfile.mockReturnValue({ data: null })
    mockGeoDefault.mockReturnValue({
      appliedGeoDefault: null,
      notifyUserInteracted: vi.fn(),
      isResolving: false,
    })
    mockUseTags.mockReturnValue({
      data: { tags: [{ slug: 'punk', usage_count: 0 }] },
    })
    mockUseVenueCities.mockReturnValue({
      data: {
        cities: [
          { city: 'Chicago', state: 'IL', venue_count: 42 },
          { city: 'Phoenix', state: 'AZ', venue_count: 8 },
        ],
      },
      isLoading: false,
      isFetching: false,
      isPlaceholderData: false,
    })
    setVenues([makeVenue()])
  })

  function setVenues(venues: VenueWithShowCount[], total = venues.length) {
    mockUseVenues.mockReturnValue({
      data: { venues, total, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      isPlaceholderData: false,
      error: null,
      refetch: vi.fn(),
    })
  }

  function anonWithGeo(city: CityState = PHOENIX) {
    mockGeoDefault.mockReturnValue({
      appliedGeoDefault: city,
      notifyUserInteracted: vi.fn(),
      isResolving: false,
    })
  }

  function authedWithFavourite(city: CityState = PHOENIX) {
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      authStatus: 'authenticated',
      logout: vi.fn(),
    })
    mockUseProfile.mockReturnValue({
      data: { user: { preferences: { favorite_cities: [city] } } },
    })
  }

  describe('derived city', () => {
    it('filters by the favourite city on a bare URL WITHOUT writing to the URL', () => {
      authedWithFavourite()

      render(<VenueList />)

      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ cities: [PHOENIX] })
      )
      // Regression guard: the default is derived during render, not seeded, so
      // client-side navigation cannot drop it.
      expect(mockSetCities).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('filters by the geo city for an anonymous visitor WITHOUT writing to the URL', () => {
      anonWithGeo()

      render(<VenueList />)

      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ cities: [PHOENIX] })
      )
      expect(mockSetCities).not.toHaveBeenCalled()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('names the derived city and says it came from the location', () => {
      anonWithGeo()
      render(<VenueList />)
      expect(screen.getByTestId('venues-derived-city')).toHaveTextContent(
        'Showing Phoenix, AZ from your location'
      )
    })

    it('says the derived city came from the favourites when it did', () => {
      authedWithFavourite()
      render(<VenueList />)
      expect(screen.getByTestId('venues-derived-city')).toHaveTextContent(
        'Showing Phoenix, AZ from your favourites'
      )
    })

    it('names the derived city in the heading', () => {
      anonWithGeo()
      render(<VenueList />)
      expect(
        screen.getByRole('heading', { level: 1, name: 'Venues in Phoenix, AZ' })
      ).toBeInTheDocument()
    })

    it('lets an explicit ?cities= win over the derived default', () => {
      authedWithFavourite()
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Chicago,IL' })
      )

      render(<VenueList />)

      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ cities: [{ city: 'Chicago', state: 'IL' }] })
      )
      expect(screen.queryByTestId('venues-derived-city')).not.toBeInTheDocument()
    })

    it('treats ?cities=all as an explicit whole catalogue, not a missing city', () => {
      anonWithGeo()
      mockSearchParams.mockReturnValue(new URLSearchParams({ cities: 'all' }))

      render(<VenueList />)

      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ cities: undefined, enabled: true })
      )
      expect(screen.queryByTestId('venues-city-chooser')).not.toBeInTheDocument()
    })

    it('holds the rows back while the derivation is still resolving', () => {
      mockGeoDefault.mockReturnValue({
        appliedGeoDefault: null,
        notifyUserInteracted: vi.fn(),
        isResolving: true,
      })

      render(<VenueList />)

      expect(screen.getByTestId('venues-skeleton')).toBeInTheDocument()
      expect(screen.queryByTestId('venues-city-chooser')).not.toBeInTheDocument()
      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ enabled: false })
      )
    })
  })

  describe('no derivable city', () => {
    it('offers the busiest cities instead of a table, and asks for no rows', () => {
      render(<VenueList />)

      const chooser = screen.getByTestId('venues-city-chooser')
      expect(chooser).toHaveTextContent('Choose a city to see its rooms.')
      expect(
        within(chooser).getByTestId('venues-busiest-chicago-il')
      ).toHaveAttribute('href', '/venues?cities=Chicago%2CIL')
      expect(screen.queryByRole('table')).not.toBeInTheDocument()
      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ enabled: false })
      )
    })

    it('counts the whole catalogue beside the heading', () => {
      render(<VenueList />)
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
        'Venues'
      )
      expect(screen.getByText('50 rooms in 2 cities')).toBeInTheDocument()
    })

    it('suppresses the shared Popular row, which the chips replace', () => {
      render(<VenueList />)
      expect(screen.getByTestId('city-filters')).toHaveAttribute(
        'data-popular',
        'false'
      )
    })

    it('renders no tag facet, which would have no rows to narrow', () => {
      mockUseTags.mockReturnValue({
        data: { tags: [{ slug: 'punk', usage_count: 12 }] },
      })
      render(<VenueList />)
      expect(screen.queryByTestId('tag-facet-panel')).not.toBeInTheDocument()
      expect(screen.queryByTestId('tag-facet-sheet')).not.toBeInTheDocument()
    })
  })

  describe('sort', () => {
    it('writes ?sort= and resets the page in one navigation', async () => {
      const user = userEvent.setup()
      anonWithGeo()
      render(<VenueList />)

      await user.click(screen.getByTestId('venue-sort-header-name'))

      expect(mockSetSort).toHaveBeenCalledWith('name')
      expect(mockSetPage).toHaveBeenCalledWith(null)
      // Both writes go through nuqs; a router push beside them would abort
      // nuqs's pending queue and the reset could be dropped.
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('clears the param rather than writing the default order', async () => {
      const user = userEvent.setup()
      anonWithGeo()
      mockSearchParams.mockReturnValue(new URLSearchParams({ sort: 'name' }))
      render(<VenueList />)

      await user.click(screen.getByTestId('venue-sort-header-upcoming'))

      expect(mockSetSort).toHaveBeenCalledWith(null)
    })

    it('requests the order named in the URL and marks its column', () => {
      anonWithGeo()
      mockSearchParams.mockReturnValue(new URLSearchParams({ sort: 'next' }))
      render(<VenueList />)

      expect(mockUseVenues).toHaveBeenCalledWith(
        expect.objectContaining({ sort: 'next' })
      )
      expect(
        screen.getByRole('columnheader', { name: /next show/i })
      ).toHaveAttribute('aria-sort', 'ascending')
      expect(
        screen.getByRole('columnheader', { name: /upcoming shows/i })
      ).toHaveAttribute('aria-sort', 'none')
    })
  })

  describe('city filter writes', () => {
    it('resets the page and writes the pick through nuqs', async () => {
      const user = userEvent.setup()
      anonWithGeo()
      render(<VenueList />)

      await user.click(screen.getByTestId('mock-pick-tucson'))

      expect(mockSetPage).toHaveBeenCalledWith(null)
      expect(mockSetCities).toHaveBeenCalledWith([
        { city: 'Tucson', state: 'AZ' },
      ])
    })

    it('writes the all-cities sentinel when the selection is cleared', async () => {
      const user = userEvent.setup()
      anonWithGeo()
      render(<VenueList />)

      await user.click(screen.getByTestId('mock-clear-cities'))

      // A bare URL would mean "apply my derived default", so clearing has to
      // say "all" explicitly or the cleared city comes straight back.
      expect(mockSetCities).toHaveBeenCalledWith('all')
    })
  })

  describe('pagination', () => {
    function pagedVenues() {
      anonWithGeo()
      setVenues(
        Array.from({ length: 50 }, (_, i) =>
          makeVenue({ id: i + 1, slug: `venue-${i + 1}`, name: `Venue ${i + 1}` })
        ),
        120
      )
    }

    it('links page 2 from the params already on screen', () => {
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ sort: 'name', utm_source: 'newsletter' })
      )
      pagedVenues()

      render(<VenueList />)

      const links = screen.getAllByRole('link', { name: 'Page 2' })
      expect(links[0]).toHaveAttribute(
        'href',
        '/venues?sort=name&utm_source=newsletter&page=2'
      )
    })

    it('writes no page param for page 1', () => {
      mockSearchParams.mockReturnValue(new URLSearchParams({ page: '2' }))
      pagedVenues()

      render(<VenueList />)

      expect(screen.getAllByRole('link', { name: 'Page 1' })[0]).toHaveAttribute(
        'href',
        '/venues'
      )
    })

    it('renders the pager above and below, announcing from one of them', () => {
      pagedVenues()
      render(<VenueList />)

      const pagers = screen.getAllByRole('navigation', { name: /pagination/i })
      expect(pagers).toHaveLength(2)
    })

    it('renders no pager when the whole city fits one page', () => {
      anonWithGeo()
      setVenues([makeVenue()], 1)
      render(<VenueList />)

      expect(
        screen.queryByRole('navigation', { name: /pagination/i })
      ).not.toBeInTheDocument()
    })

    it('distinguishes a page past the end from an empty city', () => {
      mockSearchParams.mockReturnValue(new URLSearchParams({ page: '9' }))
      anonWithGeo()
      setVenues([], 120)

      render(<VenueList />)

      expect(screen.getByTestId('venues-page-beyond-end')).toBeInTheDocument()
      expect(screen.queryByTestId('venues-zero-result')).not.toBeInTheDocument()
    })
  })

  describe('rows', () => {
    it('renders a room per row with its next show in the venue zone', () => {
      anonWithGeo()
      setVenues([
        makeVenue({ id: 1, name: 'The Van Buren', upcoming_show_count: 74 }),
      ])

      render(<VenueList />)

      expect(
        screen.getByRole('link', { name: 'The Van Buren' })
      ).toHaveAttribute('href', '/venues/test-venue')
      expect(screen.getByText('101 Main St')).toBeInTheDocument()
      // 2026-09-15T02:30:00Z is 7:30 PM on Monday Sep 14 in Phoenix.
      expect(screen.getByText(/Mon, Sep 14 7:30 PM/)).toBeInTheDocument()
      expect(screen.getByText('74')).toBeInTheDocument()
    })

    it('groups quiet rooms under a heading and dates them by their LAST show', () => {
      anonWithGeo()
      setVenues([
        makeVenue({ id: 1, name: 'Busy Room', upcoming_show_count: 4 }),
        makeVenue({
          id: 2,
          slug: 'quiet-room',
          name: 'Quiet Room',
          upcoming_show_count: 0,
          next_show: undefined,
          last_show: {
            event_date: '2026-08-23T02:00:00Z',
            slug: 'old-show',
            title: '',
          },
        }),
      ])

      render(<VenueList />)

      expect(screen.getByTestId('venues-quiet-group-header')).toHaveTextContent(
        /quiet rooms/i
      )
      expect(screen.getByText(/last: Sat, Aug 22/)).toBeInTheDocument()
    })

    it('renders no quiet heading when every room has something booked', () => {
      anonWithGeo()
      setVenues([makeVenue({ upcoming_show_count: 4 })])
      render(<VenueList />)
      expect(
        screen.queryByTestId('venues-quiet-group-header')
      ).not.toBeInTheDocument()
    })

    it('links the website and nothing else', () => {
      anonWithGeo()
      setVenues([
        makeVenue({
          name: 'The Van Buren',
          social: {
            website: 'https://thevanburenphx.com',
            instagram: 'https://instagram.com/x',
          },
        }),
      ])

      render(<VenueList />)

      const site = screen.getByTestId('venue-site-link-1')
      expect(site).toHaveAttribute('href', 'https://thevanburenphx.com')
      expect(site).toHaveAccessibleName('Website for The Van Buren')
      expect(
        screen.queryByRole('link', { name: /instagram/i })
      ).not.toBeInTheDocument()
    })

    it('carries the verified legend under the table', () => {
      anonWithGeo()
      render(<VenueList />)
      expect(screen.getByTestId('venues-verified-legend')).toHaveTextContent(
        'verified room'
      )
    })

    it('has no Load More button anywhere', () => {
      anonWithGeo()
      setVenues([makeVenue()], 120)
      render(<VenueList />)
      expect(
        screen.queryByRole('button', { name: /load more/i })
      ).not.toBeInTheDocument()
    })
  })

  describe('counts', () => {
    it('states the upcoming total only while the whole set is on screen', () => {
      anonWithGeo()
      setVenues(
        [
          makeVenue({ id: 1, upcoming_show_count: 74 }),
          makeVenue({ id: 2, slug: 'b', upcoming_show_count: 66 }),
        ],
        2
      )

      render(<VenueList />)

      expect(screen.getByTestId('venues-count-rule')).toHaveTextContent(
        "2 rooms · 140 upcoming shows at verified rooms, on each venue's local calendar"
      )
    })

    it('labels the upcoming total as this page when the city is paged', () => {
      anonWithGeo()
      setVenues([makeVenue({ upcoming_show_count: 74 })], 120)

      render(<VenueList />)

      expect(screen.getByTestId('venues-count-rule')).toHaveTextContent(
        '74 upcoming shows on this page'
      )
    })
  })

  describe('tag facet', () => {
    it('is hidden when every venue tag count is zero', () => {
      anonWithGeo()
      render(<VenueList />)
      expect(screen.queryByTestId('tag-facet-panel')).not.toBeInTheDocument()
      expect(screen.queryByTestId('tag-facet-sheet')).not.toBeInTheDocument()
    })

    it('is shown once a venue tag has been applied to something', () => {
      anonWithGeo()
      mockUseTags.mockReturnValue({
        data: { tags: [{ slug: 'punk', usage_count: 12 }] },
      })
      render(<VenueList />)
      expect(screen.getByTestId('tag-facet-panel')).toBeInTheDocument()
    })

    it('shows an applied tag as a removable chip', async () => {
      const user = userEvent.setup()
      anonWithGeo()
      mockSearchParams.mockReturnValue(new URLSearchParams({ tags: 'punk' }))
      render(<VenueList />)

      await user.click(screen.getByTestId('venue-tag-chip-punk-remove'))

      expect(mockPush).toHaveBeenCalledWith('/venues', { scroll: false })
    })
  })

  describe('empty city', () => {
    it('says so quietly and offers the one action', () => {
      anonWithGeo({ city: 'Flagstaff', state: 'AZ' })
      setVenues([], 0)

      render(<VenueList />)

      expect(screen.getByTestId('venues-zero-result')).toHaveTextContent(
        'No verified rooms in Flagstaff, AZ yet.'
      )
      expect(screen.getByRole('link', { name: 'Add a venue' })).toHaveAttribute(
        'href',
        '/contribute'
      )
    })

    it('offers to clear the filters when a tag is what emptied it', () => {
      anonWithGeo()
      mockSearchParams.mockReturnValue(new URLSearchParams({ tags: 'punk' }))
      mockUseTags.mockReturnValue({
        data: { tags: [{ slug: 'punk', usage_count: 12 }] },
      })
      setVenues([], 0)

      render(<VenueList />)

      expect(screen.getByTestId('venues-zero-result')).toHaveTextContent(
        'No verified rooms match the current filters.'
      )
    })
  })

  describe('errors', () => {
    it('reports a failure only when nothing on screen answers the request', () => {
      anonWithGeo()
      mockUseVenues.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
        isPlaceholderData: false,
        error: new Error('boom'),
        refetch: vi.fn(),
      })

      render(<VenueList />)

      expect(screen.getByText(/Failed to load venues/)).toBeInTheDocument()
    })

    it('keeps a rendered page through a failed background refetch', () => {
      anonWithGeo()
      mockUseVenues.mockReturnValue({
        data: { venues: [makeVenue()], total: 1, limit: 50, offset: 0 },
        isLoading: false,
        isFetching: false,
        isPlaceholderData: false,
        error: new Error('boom'),
        refetch: vi.fn(),
      })

      render(<VenueList />)

      expect(screen.queryByText(/Failed to load venues/)).not.toBeInTheDocument()
      expect(screen.getByRole('table')).toBeInTheDocument()
    })
  })
})
