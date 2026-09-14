import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShowList } from './ShowList'
import type { ShowResponse, ArtistResponse } from '../types'


// Mock AuthContext
const mockAuthContext = vi.fn(
  (): {
    user: { id: string } | null
    authStatus: 'pending' | 'anonymous' | 'authenticated'
    isLoading?: boolean
    logout: () => void
  } => ({ user: null, authStatus: 'anonymous', logout: vi.fn() })
)
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(mockAuthContext()) }
})

// Mock next/navigation
const mockPush = vi.fn()
const mockReplace = vi.fn()
// A REAL `URLSearchParams`, because the pager's href builder spreads the live
// params rather than reading one key at a time: a `{ get }` stub would answer
// every lookup and still serve `?page=2` with every other param dropped.
const mockSearchParams = vi.fn(() => new URLSearchParams())
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams(),
}))

// nuqs `useQueryState` is bridged to the SAME mocked searchParams the component
// uses for legacy/tags, so tests keep a single URL source of truth. `createParser`
// (used by cityParams) stays real. The cities setter is a mock tests assert on —
// the component writes `?cities=` through it, not through the router.
const mockSetCities = vi.fn()
const mockSetPage = vi.fn()
vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return {
    ...actual,
    useQueryState: (key: string, parser: { parse: (v: string) => unknown }) => {
      const raw = mockSearchParams().get(key)
      const setter = key === 'page' ? mockSetPage : mockSetCities
      return [raw != null ? parser.parse(raw) : null, setter]
    },
  }
})

// Mock show hooks
const mockUseShowsCalendar = vi.fn()
const mockUseShowCities = vi.fn()
const mockUseShowMonths = vi.fn<
  (options?: unknown) => {
    data?: {
      months: Array<{ year: number; month: number; count: number }>
      total: number
    }
    isPlaceholderData?: boolean
  }
>(() => ({ data: undefined }))
vi.mock('../hooks/useShows', () => ({
  useShowsCalendar: (opts: unknown) => mockUseShowsCalendar(opts),
  useShowCities: (opts: unknown) => mockUseShowCities(opts),
  useShowMonths: (opts: unknown) => mockUseShowMonths(opts),
}))

// Controllable so a test can hold the batch in flight (data undefined), which
// is the state that used to make every card self-fetch.
const mockUseShowSaveCountBatch = vi.fn<() => { data: unknown }>(() => ({
  data: {},
}))
vi.mock('../hooks/useSavedShows', () => ({
  useShowSaveCountBatch: () => mockUseShowSaveCountBatch(),
}))

// Mock profile hooks (controllable so tests can set favorite_cities)
const mockUseProfile = vi.fn(() => ({ data: null as unknown }))
vi.mock('@/features/auth', () => ({
  useProfile: () => mockUseProfile(),
  useSetFavoriteCities: () => ({ mutate: vi.fn() }),
}))

// PSY-309: mock tag facet components
vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div data-testid="tag-facet-panel" />,
  TagFacetSheet: ({ onToggle }: { onToggle?: (slugs: string[]) => void }) => (
    <div data-testid="tag-facet-sheet">
      <button data-testid="mock-toggle-tag" onClick={() => onToggle?.(['noise'])}>
        toggle noise
      </button>
    </div>
  ),
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
}))

// Mock density hook
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: () => ({ density: 'comfortable', setDensity: vi.fn() }),
}))

// Mock child components. The ROW is mocked for the same reason `ShowCard` was
// before it: this file tests paging, filters and the scope line, and the row's
// own anatomy is pinned by `DayGroupedShowRow.test.tsx`.
const showCardSaveData: unknown[] = []
vi.mock('./DayGroupedShowRow', () => ({
  DayGroupedShowRow: ({
    show,
    saveData,
  }: {
    show: ShowResponse
    saveData?: unknown
  }) => {
    showCardSaveData.push(saveData)
    return <article data-testid={`show-card-${show.id}`}>{show.title}</article>
  },
  DayGroupedShowListHeader: () => <div data-testid="show-list-header" />,
}))

vi.mock('./ShowListSkeleton', () => ({
  ShowListSkeleton: () => <div data-testid="show-skeleton">Loading...</div>,
}))

vi.mock('@/components/filters', () => ({
  CityFilters: ({
    children,
    onFilterChange,
  }: {
    children?: React.ReactNode
    onFilterChange?: (cities: Array<{ city: string; state: string }>) => void
  }) => (
    <div data-testid="city-filters">
      <button
        data-testid="mock-select-city"
        onClick={() => onFilterChange?.([{ city: 'Tucson', state: 'AZ' }])}
      >
        select Tucson
      </button>
      {children}
    </div>
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
      authStatus: 'anonymous',
      logout: vi.fn(),
    })
    mockSearchParams.mockReturnValue(new URLSearchParams())
    mockUseProfile.mockReturnValue({ data: null as unknown })
    mockUseShowCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })
  })

  describe('loading state', () => {
    it('shows skeleton when loading and no data', () => {
      mockUseShowsCalendar.mockReturnValue({
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
      mockUseShowsCalendar.mockReturnValue({
        data: { shows: [], total: 0 },
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
      mockUseShowsCalendar.mockReturnValue({
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
      mockUseShowsCalendar.mockReturnValue({
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
      mockUseShowsCalendar.mockReturnValue({
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
      mockUseShowsCalendar.mockReturnValue({
        data: { shows: [], total: 0 },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('No upcoming shows at this time.')).toBeInTheDocument()
    })

    it('shows city-specific empty message when cities are filtered', () => {
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ' })
      )
      mockUseShowsCalendar.mockReturnValue({
        data: { shows: [], total: 0 },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('No upcoming shows match the current filters.')).toBeInTheDocument()
    })

    it('shows "Clear filters" button when filtered to city with no results', () => {
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ' })
      )
      mockUseShowsCalendar.mockReturnValue({
        data: { shows: [], total: 0 },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByText('Clear filters')).toBeInTheDocument()
    })

    it('suggests nearby cities and applies one on click', async () => {
      const user = userEvent.setup()
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ' })
      )
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [
            {
              city: 'Phoenix',
              state: 'AZ',
              show_count: 40,
              latitude: 33.4484,
              longitude: -112.074,
            },
            {
              city: 'Tempe',
              state: 'AZ',
              show_count: 8,
              latitude: 33.4255,
              longitude: -111.94,
            },
            {
              city: 'Tucson',
              state: 'AZ',
              show_count: 20,
              latitude: 32.2226,
              longitude: -110.9747,
            },
          ],
        },
        isLoading: false,
        isFetching: false,
      })
      mockUseShowsCalendar.mockReturnValue({
        data: { shows: [], total: 0 },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)

      expect(screen.getByTestId('shows-city-suggestions')).toHaveTextContent(
        'Try Tempe, or Tucson.'
      )
      await user.click(screen.getByTestId('shows-suggest-city-tempe-az'))
      expect(mockSetCities).toHaveBeenCalledWith([{ city: 'Tempe', state: 'AZ' }])
    })

    it('offers same-tags-all-cities when both city and tags yield nothing', async () => {
      const user = userEvent.setup()
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ', tags: 'shoegaze' })
      )
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [
            {
              city: 'Phoenix',
              state: 'AZ',
              show_count: 40,
              latitude: 33.4484,
              longitude: -112.074,
            },
            {
              city: 'Tucson',
              state: 'AZ',
              show_count: 20,
              latitude: 32.2226,
              longitude: -110.9747,
            },
          ],
        },
        isLoading: false,
        isFetching: false,
      })
      mockUseShowsCalendar.mockReturnValue({
        data: { shows: [], total: 0 },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)

      await user.click(screen.getByTestId('shows-suggest-same-tags-all-cities'))
      expect(mockPush).toHaveBeenCalledWith(
        '/shows?cities=all&tags=shoegaze',
        { scroll: false }
      )
    })
  })

  describe('with show data', () => {
    it('renders show cards', () => {
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [
            makeShow({ id: 1, title: 'Show One' }),
            makeShow({ id: 2, title: 'Show Two' }),
          ],
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
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow()],
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
    /** A page of rows plus the unwindowed total the pager sizes itself from. */
    const setPage = (
      rowCount: number,
      total: number,
      extra: Record<string, unknown> = {}
    ) =>
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: Array.from({ length: rowCount }, (_, index) =>
            makeShow({ id: index + 1, title: `Show ${index + 1}` })
          ),
          total,
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
        ...extra,
      })

    it('serves numbered page links instead of a Load More control', () => {
      setPage(50, 268)
      render(<ShowList />)

      expect(screen.queryByRole('button', { name: /load more/i })).toBeNull()
      // 268 rows at 50 a page is six pages.
      expect(
        screen.getAllByRole('link', { name: /^page 6$/i }).length
      ).toBeGreaterThan(0)
    })

    it('links page 2 at ?page=2 and page 1 at the bare URL', () => {
      setPage(50, 268)
      render(<ShowList />)

      expect(screen.getAllByRole('link', { name: /^page 2$/i })[0]).toHaveAttribute(
        'href',
        '/shows?page=2'
      )
      // Page 1 writes NO `page`, so the first page has exactly one address.
      expect(screen.getAllByRole('link', { name: /^page 1$/i })[0]).toHaveAttribute(
        'href',
        '/shows'
      )
    })

    it('labels the boundary controls Sooner and Later', () => {
      setPage(50, 268)
      render(<ShowList />)

      // The list runs soonest first, so paging back moves toward tonight.
      expect(screen.getAllByRole('link', { name: /^later$/i })[0]).toHaveAttribute(
        'href',
        '/shows?page=2'
      )
      // On page 1 the Sooner control keeps its space rather than unmounting.
      expect(
        screen.getAllByTestId('pagination-previous-disabled').length
      ).toBeGreaterThan(0)
    })

    it('captions the page with the row range it covers', () => {
      setPage(50, 268)
      render(<ShowList />)

      // Once per pager, top and bottom.
      expect(
        screen.getAllByText('Showing 1–50 of 268 · Page 1 of 6')
      ).toHaveLength(2)
    })

    it('renders no pager at all when one page holds everything', () => {
      setPage(3, 3)
      render(<ShowList />)

      expect(screen.queryByTestId('pagination')).toBeNull()
    })

    it('withholds the caption while the rows belong to the previous page', () => {
      // `keepPreviousData` holds the outgoing page across a page change, and
      // "Showing 51-100" over rows 1-50 is a wrong number, not a stale one.
      setPage(50, 268, { isFetching: true, isPlaceholderData: true })
      render(<ShowList />)

      expect(screen.queryByText(/Showing/)).toBeNull()
      expect(screen.getAllByText('Page 1 of 6').length).toBeGreaterThan(0)
    })

    // PSY-1768: two pagers on one surface ship two live regions, and a screen
    // reader speaks each one.
    it('mounts exactly one page-change live region for two pagers', () => {
      setPage(50, 268)
      const { container } = render(<ShowList />)

      expect(container.querySelectorAll('[data-testid="pagination"]')).toHaveLength(2)
      expect(container.querySelectorAll('[role="status"][aria-live]')).toHaveLength(1)
    })

    it('requests the offset the page in view names', () => {
      mockSearchParams.mockReturnValue(new URLSearchParams({ page: '3' }))
      setPage(50, 268)
      render(<ShowList />)

      expect(mockUseShowsCalendar).toHaveBeenCalledWith(
        expect.objectContaining({ offset: 100, limit: 50 })
      )
    })

    it('carries foreign params through a page link', () => {
      // The list shares its query string with the filters and with whatever a
      // campaign link brought along; a pager that rebuilt the URL from its own
      // keys would drop all of it on every click.
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ', utm_source: 'newsletter' })
      )
      setPage(50, 268)
      render(<ShowList />)

      const href = screen
        .getAllByRole('link', { name: /^page 2$/i })[0]
        .getAttribute('href')
      expect(href).toContain('cities=Phoenix%2CAZ')
      expect(href).toContain('utm_source=newsletter')
      expect(href).toContain('page=2')
    })

    it('labels page links with the months each page covers', () => {
      mockUseShowMonths.mockReturnValue({
        data: {
          months: [
            { year: 2026, month: 9, count: 60 },
            { year: 2026, month: 10, count: 120 },
            { year: 2026, month: 11, count: 88 },
          ],
          total: 268,
        },
      })
      setPage(50, 268)
      render(<ShowList />)

      expect(
        screen.getAllByRole('link', { name: 'Page 1, Sep 2026' }).length
      ).toBeGreaterThan(0)
    })

    // The histogram holds its own previous data across a filter change. A stale
    // one whose bucket sum happens to equal the current total would pass the
    // premise check, so the rows answering the request is not enough on its
    // own. EVERY page in the window loses its label, not just the current one:
    // withholding the total alone skips the premise check altogether and drops
    // only the current page.
    it('drops every label while the histogram itself is stale', () => {
      mockUseShowMonths.mockReturnValue({
        data: {
          months: [
            { year: 2026, month: 9, count: 60 },
            { year: 2026, month: 10, count: 120 },
            { year: 2026, month: 11, count: 88 },
          ],
          total: 268,
        },
        isPlaceholderData: true,
      })
      setPage(50, 268)
      render(<ShowList />)

      expect(screen.queryByRole('link', { name: /Page \d+, / })).toBeNull()
      // The numbered links are all still there, just unlabelled.
      for (const page of [1, 2, 6]) {
        expect(
          screen.getAllByRole('link', { name: new RegExp(`^page ${page}$`, 'i') })
            .length
        ).toBeGreaterThan(0)
      }
    })

    // The counterpart: a FRESH histogram labels every page in the window, so
    // the test above is pinning the gate rather than a list that never labels.
    it('labels every page in the window while the histogram is fresh', () => {
      mockUseShowMonths.mockReturnValue({
        data: {
          months: [
            { year: 2026, month: 9, count: 60 },
            { year: 2026, month: 10, count: 120 },
            { year: 2026, month: 11, count: 88 },
          ],
          total: 268,
        },
        isPlaceholderData: false,
      })
      setPage(50, 268)
      render(<ShowList />)

      expect(
        screen.getAllByRole('link', { name: 'Page 1, Sep 2026' }).length
      ).toBeGreaterThan(0)
      expect(
        screen.getAllByRole('link', { name: /^Page 6, / }).length
      ).toBeGreaterThan(0)
    })

    // The histogram and the rows are separate reads. A disagreement proves the
    // histogram's ordinals are no longer the list's ordinals, and a wrong label
    // costs more than a missing one: the pager announces the current page's
    // label into a live region and never corrects it.
    it('drops every label when the histogram disagrees with the row total', () => {
      mockUseShowMonths.mockReturnValue({
        data: {
          months: [
            { year: 2026, month: 9, count: 60 },
            { year: 2026, month: 10, count: 120 },
          ],
          total: 180,
        },
      })
      setPage(50, 268)
      render(<ShowList />)

      expect(screen.queryByRole('link', { name: /Page 1, / })).toBeNull()
      expect(
        screen.getAllByRole('link', { name: /^page 1$/i }).length
      ).toBeGreaterThan(0)
    })

    // A filter change answers a different question, so it starts at page 1
    // again. Otherwise a reader on page 4 of Phoenix lands on page 4 of Tucson,
    // which may not exist.
    it('resets the page when the city filter changes', async () => {
      const user = userEvent.setup()
      mockSearchParams.mockReturnValue(new URLSearchParams({ page: '3' }))
      mockUseShowCities.mockReturnValue({
        data: { cities: [{ city: 'Tucson', state: 'AZ', show_count: 3 }] },
        isLoading: false,
        isFetching: false,
      })
      setPage(50, 268)
      render(<ShowList />)

      await user.click(screen.getByTestId('mock-select-city'))

      // Both writes go through nuqs, which batches them into ONE history entry.
      // A `router.push` in the same tick would abort nuqs's pending queue.
      expect(mockSetPage).toHaveBeenCalledWith(null)
      expect(mockSetCities).toHaveBeenCalledWith([
        { city: 'Tucson', state: 'AZ' },
      ])
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('resets the page in the same navigation as a tag change', async () => {
      const user = userEvent.setup()
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ page: '3', cities: 'Phoenix,AZ' })
      )
      setPage(50, 268)
      render(<ShowList />)

      await user.click(screen.getByTestId('mock-toggle-tag'))

      // ONE router write carrying both changes: the tag params and the dropped
      // page. A separate nuqs reset beside it could be silently dropped.
      expect(mockPush).toHaveBeenCalledTimes(1)
      const pushed = mockPush.mock.calls[0][0] as string
      expect(pushed).toContain('tags=noise')
      expect(pushed).toContain('cities=Phoenix%2CAZ')
      expect(pushed).not.toContain('page=')
    })

    // A stale bookmark, a hand-typed number, or a page that existed until shows
    // graduated out of the upcoming set. "No upcoming shows at this time." over
    // a catalogue of 268 is a flatly false claim.
    it('says the page is past the end rather than that the list is empty', () => {
      mockSearchParams.mockReturnValue(new URLSearchParams({ page: '99' }))
      setPage(0, 268)
      render(<ShowList />)

      expect(screen.getByTestId('shows-page-beyond-end')).toHaveTextContent(
        'That page is past the end of this list.'
      )
      expect(screen.queryByTestId('shows-zero-result')).toBeNull()
      expect(screen.queryByText('No upcoming shows at this time.')).toBeNull()
    })

    it('offers the way back to page 1 from past the end', () => {
      mockSearchParams.mockReturnValue(new URLSearchParams({ page: '99' }))
      setPage(0, 268)
      render(<ShowList />)

      expect(
        screen.getByRole('link', { name: 'Back to the first page' })
      ).toHaveAttribute('href', '/shows')
    })

    // The other zero-rows case is a genuinely empty result, and it keeps the
    // copy and the filter affordances it always had.
    it('still reports a genuinely empty list as empty', () => {
      setPage(0, 0)
      render(<ShowList />)

      expect(screen.getByTestId('shows-zero-result')).toBeInTheDocument()
      expect(screen.queryByTestId('shows-page-beyond-end')).toBeNull()
    })

    // The scope line states the WHOLE matching set, not the rows on screen.
    // The pager's caption already says exactly which rows those are ("Showing
    // 51-100 of 1,088"), so repeating a second, page-scoped count above it was
    // two readings of one fact.
    it('states the size of the whole matching set, not the page', () => {
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow(), makeShow({ id: 2 })],
          total: 1088,
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('show-count')).toHaveTextContent(
        '· 1,088 upcoming'
      )
    })

    it('names the metro when exactly one city is selected', () => {
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ' })
      )
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow()],
          total: 166,
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('show-count')).toHaveTextContent(
        '· 166 in Phoenix, AZ'
      )
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
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow()],
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('city-filters')).toBeInTheDocument()
    })

    it('shows city filters when one city has shows (PSY-932)', () => {
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [{ city: 'Phoenix', state: 'AZ', show_count: 10 }],
        },
        isLoading: false,
        isFetching: false,
      })
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow()],
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<ShowList />)
      expect(screen.getByTestId('city-filters')).toBeInTheDocument()
    })

    it('hides city filters when no cities have shows', () => {
      mockUseShowCities.mockReturnValue({
        data: { cities: [] },
        isLoading: false,
        isFetching: false,
      })
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow()],
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

  // IP-geo soft default (PSY-946, derived PSY-1391). ShowList consumes the
  // real useGeoDefaultCity hook, driven here via a mocked /api/geo fetch + the
  // useShowCities has-shows list; the derived value folds into the effective
  // filter — nothing is written to the URL.
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
        isLoading: false,
        isFetching: false,
      })
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows: [makeShow()],
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })
    })

    afterEach(() => {
      vi.restoreAllMocks()
      window.sessionStorage.clear()
    })

    it('derives the canonical geo city for an anon visitor with shows (no URL write)', async () => {
      mockGeoFetch({ city: 'Omaha', state: 'NE' })
      render(<ShowList />)
      // The geo default is DERIVED into the effective filter...
      await waitFor(() =>
        expect(mockUseShowsCalendar).toHaveBeenCalledWith(
          expect.objectContaining({
            cities: [{ city: 'Omaha', state: 'NE' }],
          }),
        ),
      )
      // ...and never seeded onto the URL.
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockSetCities).not.toHaveBeenCalled()
    })

    it('does NOT seed when the geo city has no shows', async () => {
      mockGeoFetch({ city: 'Tucson', state: 'AZ' })
      render(<ShowList />)
      // Let the fetch resolve, then assert no replace.
      await waitFor(() =>
        expect(
          window.sessionStorage.getItem('ph-geo-default-city'),
        ).not.toBeNull(),
      )
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('does NOT fetch geo or seed when ?cities= is already present', () => {
      const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ' })
      )
      render(<ShowList />)
      expect(fetchSpy).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
    })

    it('does NOT fetch geo for an authed user (no wasted edge request)', () => {
      const fetchSpy = mockGeoFetch({ city: 'Omaha', state: 'NE' })
      mockAuthContext.mockReturnValue({
        user: { id: 1 } as never,
        authStatus: 'authenticated',
        logout: vi.fn(),
      })
      render(<ShowList />)
      expect(fetchSpy).not.toHaveBeenCalled()
    })
  })

  // PSY-1388: the favorite-city default is DERIVED during render, not seeded
  // into the URL by a mount effect. This is the fix for the default being
  // dropped after client-side navigation.
  describe('favorites default (derived, no URL write)', () => {
    const setShows = (shows: ShowResponse[] = [makeShow()]) =>
      mockUseShowsCalendar.mockReturnValue({
        data: { shows, total: shows.length },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })

    const authedWithFavorite = () => {
      mockAuthContext.mockReturnValue({
        user: { id: 1 } as never,
        authStatus: 'authenticated',
        logout: vi.fn(),
      })
      mockUseProfile.mockReturnValue({
        data: {
          user: { preferences: { favorite_cities: [{ city: 'Phoenix', state: 'AZ' }] } },
        },
      })
      mockUseShowCities.mockReturnValue({
        data: { cities: [{ city: 'Phoenix', state: 'AZ', show_count: 5 }] },
        isLoading: false,
        isFetching: false,
      })
    }

    it('filters by the favorite city on a bare URL WITHOUT writing to the URL', () => {
      authedWithFavorite()
      setShows()

      render(<ShowList />)

      // Filtered by the favorite — derived during render...
      expect(mockUseShowsCalendar).toHaveBeenCalledWith(
        expect.objectContaining({ cities: [{ city: 'Phoenix', state: 'AZ' }] }),
      )
      // ...and nothing was written to the URL. Regression guard: the default is
      // derived, not seeded, so client-side navigation can't drop it.
      expect(mockSetCities).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockPush).not.toHaveBeenCalled()
    })

    it('treats ?cities=all as explicit all-cities, overriding the favorite default', () => {
      authedWithFavorite()
      mockSearchParams.mockReturnValue(new URLSearchParams({ cities: 'all' }))
      setShows()

      render(<ShowList />)

      expect(mockUseShowsCalendar).toHaveBeenCalledWith(
        expect.objectContaining({ cities: undefined }),
      )
    })

    it('"Clear filters" resets to all-cities in a single navigation (no nuqs/router race)', async () => {
      const user = userEvent.setup()
      mockSearchParams.mockReturnValue(
        new URLSearchParams({ cities: 'Phoenix,AZ' })
      )
      mockUseShowCities.mockReturnValue({
        data: { cities: [{ city: 'Phoenix', state: 'AZ', show_count: 5 }] },
        isLoading: false,
        isFetching: false,
      })
      setShows([]) // empty result → the in-list "Clear filters" affordance renders

      render(<ShowList />)
      await user.click(screen.getByText('Clear filters'))

      // A single router push (not a competing nuqs setCities) — avoids nuqs
      // aborting its throttled queue on a foreign history update.
      expect(mockPush).toHaveBeenCalledWith('/shows?cities=all', { scroll: false })
      expect(mockSetCities).not.toHaveBeenCalled()
    })

    it('selecting a city writes that selection via nuqs', async () => {
      const user = userEvent.setup()
      mockUseShowCities.mockReturnValue({
        data: {
          cities: [
            { city: 'Tucson', state: 'AZ', show_count: 3 },
            { city: 'Phoenix', state: 'AZ', show_count: 5 },
          ],
        },
        isLoading: false,
        isFetching: false,
      })
      setShows()

      render(<ShowList />)
      await user.click(screen.getByTestId('mock-select-city'))

      expect(mockSetCities).toHaveBeenCalledWith([{ city: 'Tucson', state: 'AZ' }])
    })
  })
  // ── Batched save counts (rate-limit regression)
  //
  // While the batch request is in flight its `data` is undefined. Passing that
  // straight through as `saveData` meant every SaveButton saw "nobody is
  // fetching this for me" and fired its own /shows/{id}/saves request, racing
  // the batch that exists to replace them. A 50-show list became ~50 requests
  // per load and tripped the public-read rate limit for a user on cellular,
  // where the batch is slowest and the race window widest.
  describe('save-count batching', () => {
    const setListShows = (shows: ShowResponse[] = [makeShow()]) =>
      mockUseShowsCalendar.mockReturnValue({
        data: {
          shows,
        },
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
      })

    it("passes 'pending' to every card while the batch is in flight", () => {
      showCardSaveData.length = 0
      mockUseShowSaveCountBatch.mockReturnValue({ data: undefined })
      setListShows()

      render(<ShowList />)

      expect(showCardSaveData.length).toBeGreaterThan(0)
      // Not undefined: undefined is the signal that licenses a per-card fetch.
      expect(showCardSaveData.every(v => v === 'pending')).toBe(true)
    })

    it('passes the resolved entry once the batch lands', () => {
      showCardSaveData.length = 0
      mockUseShowSaveCountBatch.mockReturnValue({
        data: { '1': { save_count: 3, is_saved: true } },
      })
      setListShows()

      render(<ShowList />)

      expect(showCardSaveData).toContainEqual({ save_count: 3, is_saved: true })
    })
  })
})
