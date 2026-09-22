import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { HomeShowListView } from './HomeShowListView'
import type { HomeShowCitySelection } from '../hooks/useHomeShowCitySelection'

const mockUseUpcomingShows = vi.fn()
const mockUseShowSaveCountBatch = vi.fn()

vi.mock('../hooks/useShows', () => ({
  useUpcomingShows: (...args: unknown[]) => mockUseUpcomingShows(...args),
}))
vi.mock('../hooks/useSavedShows', async importOriginal => ({
  ...(await importOriginal<typeof import('../hooks/useSavedShows')>()),
  useShowSaveCountBatch: (...args: unknown[]) =>
    mockUseShowSaveCountBatch(...args),
}))
vi.mock('@/lib/hooks/common/usePrefetchRoutes', () => ({
  usePrefetchRoutes: () => undefined,
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: { id: '42', is_admin: false },
    isAuthenticated: true,
    authStatus: 'authenticated',
  }),
}))
vi.mock('./ShowCard', () => ({
  ShowCard: ({ show }: { show: { id: number; title: string } }) => (
    <div data-testid="show-card">{show.title}</div>
  ),
}))
vi.mock('@/components/filters', () => ({
  CityFilters: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="city-filters">{children}</div>
  ),
}))
vi.mock('@/components/filters/SaveDefaultsButton', () => ({
  SaveDefaultsButton: () => <button>Save as default</button>,
}))

const phoenix = { city: 'Phoenix', state: 'AZ' }
const baseSelection: HomeShowCitySelection = {
  cities: [],
  favoriteCities: [],
  effectiveCities: [],
  source: 'none',
  isResolving: false,
  geoAffordanceCity: null,
  selectionDiffersFromFavorites: false,
  onFilterChange: vi.fn(),
}
const selection = baseSelection

const soon = (days: number) =>
  new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString()

const shows = [
  { id: 1, title: 'Saved one', event_date: soon(1) },
  { id: 2, title: 'Not saved', event_date: soon(2) },
]

function batch(entries: Record<string, { save_count: number; is_saved: boolean }>) {
  mockUseShowSaveCountBatch.mockReturnValue({
    data: entries,
    fetchStatus: 'idle',
  })
}

beforeEach(() => {
  mockUseUpcomingShows.mockReturnValue({
    data: { shows },
    isLoading: false,
    isFetching: false,
    error: null,
  })
  batch({
    '1': { save_count: 3, is_saved: true },
    '2': { save_count: 1, is_saved: false },
  })
})

describe('HomeShowListView', () => {
  it('renders every row by default', () => {
    render(<HomeShowListView selection={selection} />)

    expect(screen.getAllByTestId('show-card')).toHaveLength(2)
  })

  it('asks for only its rows when it is not excluding', () => {
    render(<HomeShowListView selection={selection} />)

    expect(mockUseUpcomingShows).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 5 })
    )
  })

  it('drops the rows the viewer has saved, read from the batch it already makes', () => {
    render(<HomeShowListView selection={selection} excludeSaved />)

    expect(screen.queryByText('Saved one')).not.toBeInTheDocument()
    expect(screen.getByText('Not saved')).toBeInTheDocument()
    // The batch is keyed on every FETCHED row, so the exclusion is exact for
    // the page without a second read.
    expect(mockUseShowSaveCountBatch).toHaveBeenCalledWith([1, 2], true, '42')
  })

  it('over-fetches by a constant headroom so the query key cannot churn', () => {
    render(<HomeShowListView selection={selection} excludeSaved rows={4} />)

    expect(mockUseUpcomingShows).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 8 })
    )
  })

  it('waits for the batch rather than painting rows it is about to drop', () => {
    mockUseShowSaveCountBatch.mockReturnValue({
      data: undefined,
      fetchStatus: 'fetching',
    })

    render(<HomeShowListView selection={selection} excludeSaved />)

    expect(screen.queryByTestId('show-card')).not.toBeInTheDocument()
  })

  it('paints the rows unexcluded when the batch errored, rather than spin forever', () => {
    mockUseShowSaveCountBatch.mockReturnValue({
      data: undefined,
      fetchStatus: 'idle',
      isError: true,
    })

    render(<HomeShowListView selection={selection} excludeSaved />)

    expect(screen.getAllByTestId('show-card')).toHaveLength(2)
  })

  it('reports an exhausted COMPLETE page as saved, not as empty', () => {
    batch({
      '1': { save_count: 3, is_saved: true },
      '2': { save_count: 1, is_saved: true },
    })

    render(
      <HomeShowListView
        selection={selection}
        excludeSaved
        excludedLabel="Phoenix, AZ"
      />
    )

    expect(screen.queryByTestId('show-card')).not.toBeInTheDocument()
    expect(screen.queryByText(/No upcoming shows/i)).not.toBeInTheDocument()
    expect(
      screen.getByText(
        'Every show in Phoenix, AZ is already in your saved shows.'
      )
    ).toBeInTheDocument()
  })

  it('scopes the exhausted claim to the window when one is set', () => {
    // Complete page of two; both inside the week and saved; a third would be
    // outside the week, so the sentence must not speak for the whole city.
    batch({
      '1': { save_count: 3, is_saved: true },
      '2': { save_count: 1, is_saved: true },
    })

    render(
      <HomeShowListView
        selection={selection}
        excludeSaved
        withinDays={7}
        excludedLabel="Phoenix, AZ"
      />
    )

    expect(
      screen.getByText(
        'Every show in Phoenix, AZ in the next 7 days is already in your saved shows.'
      )
    ).toBeInTheDocument()
  })

  it('does not claim the city is exhausted when the page was cut off at the limit', () => {
    // rows 1 + headroom 4 = limit 5; a full page of five says nothing about
    // the shows beyond it.
    const page = [1, 2, 3, 4, 5].map(id => ({
      id,
      title: `Show ${id}`,
      event_date: soon(1),
    }))
    mockUseUpcomingShows.mockReturnValue({
      data: { shows: page },
      isLoading: false,
      isFetching: false,
      error: null,
    })
    batch(
      Object.fromEntries(
        page.map(show => [String(show.id), { save_count: 1, is_saved: true }])
      )
    )

    render(
      <HomeShowListView
        selection={selection}
        excludeSaved
        rows={1}
        excludedLabel="Phoenix, AZ"
      />
    )

    expect(
      screen.getByText(
        'The next 5 shows in Phoenix, AZ are all in your saved shows.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/^Every show/)).not.toBeInTheDocument()
  })

  it('drops the city from those sentences when none resolved', () => {
    batch({
      '1': { save_count: 3, is_saved: true },
      '2': { save_count: 1, is_saved: true },
    })

    render(<HomeShowListView selection={selection} excludeSaved />)

    expect(
      screen.getByText('Every show is already in your saved shows.')
    ).toBeInTheDocument()
  })

  it('holds rows to the window it was asked for', () => {
    mockUseUpcomingShows.mockReturnValue({
      data: {
        shows: [
          { id: 1, title: 'This week', event_date: soon(3) },
          { id: 2, title: 'Next month', event_date: soon(30) },
        ],
      },
      isLoading: false,
      isFetching: false,
      error: null,
    })

    render(<HomeShowListView selection={selection} withinDays={7} />)

    expect(screen.getByText('This week')).toBeInTheDocument()
    expect(screen.queryByText('Next month')).not.toBeInTheDocument()
  })

  it('says the window is empty rather than the city, when only the window is', () => {
    mockUseUpcomingShows.mockReturnValue({
      data: { shows: [{ id: 2, title: 'Next month', event_date: soon(30) }] },
      isLoading: false,
      isFetching: false,
      error: null,
    })

    render(
      <HomeShowListView
        selection={selection}
        withinDays={7}
        excludedLabel="Phoenix, AZ"
      />
    )

    expect(
      screen.getByText('No shows in Phoenix, AZ in the next 7 days.')
    ).toBeInTheDocument()
  })

  it('still reports a genuinely empty page', () => {
    mockUseUpcomingShows.mockReturnValue({
      data: { shows: [] },
      isLoading: false,
      isFetching: false,
      error: null,
    })

    render(<HomeShowListView selection={selection} excludeSaved />)

    expect(
      screen.getByText('No upcoming shows at this time.')
    ).toBeInTheDocument()
  })
})

describe('HomeShowListView save-as-default', () => {
  const withCities = (
    source: HomeShowCitySelection['source']
  ): HomeShowCitySelection => ({
    ...baseSelection,
    cities: [{ ...phoenix, count: 3 }],
    effectiveCities: [phoenix],
    source,
    selectionDiffersFromFavorites: true,
  })

  it('offers to save only a city the viewer chose themselves', () => {
    render(<HomeShowListView selection={withCities('user')} />)

    expect(
      screen.getByRole('button', { name: 'Save as default' })
    ).toBeInTheDocument()
  })

  it.each(['geo', 'liveliest'] as const)(
    'never offers to persist a %s default the viewer did not pick',
    source => {
      render(<HomeShowListView selection={withCities(source)} />)

      expect(
        screen.queryByRole('button', { name: 'Save as default' })
      ).not.toBeInTheDocument()
    }
  )
})
