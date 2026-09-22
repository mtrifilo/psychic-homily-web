import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { NearbyShowsSection } from './NearbyShowsSection'
import type { CityState } from '@/components/filters'

const selection = {
  cities: [],
  favoriteCities: [] as CityState[],
  effectiveCities: [] as CityState[],
  geoAffordanceCity: null,
  selectionDiffersFromFavorites: false,
  onFilterChange: vi.fn(),
}

const listProps = vi.fn()
type SavedShowsResult = {
  data?: { shows: { id: number }[] }
  isPending: boolean
  error?: Error | null
}
const mockUseSavedShows = vi.fn<() => SavedShowsResult>(() => ({
  data: { shows: [{ id: 7 }, { id: 9 }] },
  isPending: false,
}))

const citySelectionOptions = vi.fn()
vi.mock('@/features/shows/hooks/useHomeShowCitySelection', () => ({
  useHomeShowCitySelection: (options?: unknown) => {
    citySelectionOptions(options)
    return selection
  },
}))
vi.mock('@/features/shows/hooks/useSavedShows', () => ({
  SAVED_SHOWS_COLLAPSED_COUNT: 4,
  SAVED_SHOWS_HOME_READ_LIMIT: 100,
  useSavedShows: () => mockUseSavedShows(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: { id: '42' }, authStatus: 'authenticated' }),
}))
vi.mock('@/features/shows/components/HomeShowListView', () => ({
  HomeShowListView: (props: unknown) => {
    listProps(props)
    return <div data-testid="home-show-list" />
  },
}))

beforeEach(() => {
  selection.effectiveCities = []
  listProps.mockClear()
  mockUseSavedShows.mockReturnValue({
    data: { shows: [{ id: 7 }, { id: 9 }] },
    isPending: false,
  })
})

describe('NearbyShowsSection', () => {
  it('names the city the rows were fetched for and links to it', () => {
    selection.effectiveCities = [{ city: 'Phoenix', state: 'AZ' }]

    render(<NearbyShowsSection id="nearby" />)

    expect(
      screen.getByRole('heading', { name: 'Shows near you this week' })
    ).toBeInTheDocument()
    expect(screen.getByText('Phoenix, AZ · tap ♡ to save')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'All upcoming shows in Phoenix, AZ →' })
    ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ')
  })

  it('claims no city when none resolved', () => {
    render(<NearbyShowsSection id="nearby" />)

    expect(screen.getByText('tap ♡ to save')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'All upcoming shows →' })
    ).toHaveAttribute('href', '/shows')
  })

  it('carries every selected city in the link rather than speaking for one', () => {
    selection.effectiveCities = [
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Tucson', state: 'AZ' },
    ]

    render(<NearbyShowsSection id="nearby" />)

    expect(
      screen.getByRole('link', { name: 'All upcoming shows →' })
    ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ%7CTucson%2CAZ')
  })

  it('hands the list the same selection and the saved ids to drop', () => {
    render(<NearbyShowsSection id="nearby" />)

    expect(screen.getByTestId('home-show-list')).toBeInTheDocument()
    expect(listProps).toHaveBeenCalledWith(
      expect.objectContaining({ selection, excludeShowIds: [7, 9] })
    )
  })

  it("reports the exclusions as pending rather than empty while they load", () => {
    mockUseSavedShows.mockReturnValueOnce({ data: undefined, isPending: true })

    render(<NearbyShowsSection id="nearby" />)

    expect(listProps).toHaveBeenCalledWith(
      expect.objectContaining({ excludeShowIds: 'pending' })
    )
  })
})

describe('NearbyShowsSection exclusion sourcing', () => {
  it('asks the selection to resolve a city, because its copy names one', () => {
    render(<NearbyShowsSection id="nearby" />)

    expect(citySelectionOptions).toHaveBeenCalledWith({
      resolveCityForCopy: true,
    })
  })

  it('excludes nothing rather than spinning forever when the saved read fails', () => {
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: false,
      error: new Error('boom'),
    })

    render(<NearbyShowsSection id="nearby" />)

    // A repeated row is recoverable; a permanent spinner is not.
    expect(listProps).toHaveBeenCalledWith(
      expect.objectContaining({ excludeShowIds: [] })
    )
  })

  it('passes the resolved city through for the all-excluded sentence', () => {
    selection.effectiveCities = [{ city: 'Phoenix', state: 'AZ' }]

    render(<NearbyShowsSection id="nearby" />)

    expect(listProps).toHaveBeenCalledWith(
      expect.objectContaining({ excludedLabel: 'Phoenix, AZ' })
    )
  })
})
