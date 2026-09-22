import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { NearbyShowsSection } from './NearbyShowsSection'
import type { CityState } from '@/components/filters'
import type { HomeShowCitySource } from '@/features/shows/hooks/useHomeShowCitySelection'

const selection = {
  cities: [],
  favoriteCities: [] as CityState[],
  effectiveCities: [] as CityState[],
  source: 'none' as HomeShowCitySource,
  isResolving: false,
  geoAffordanceCity: null,
  selectionDiffersFromFavorites: false,
  onFilterChange: vi.fn(),
}

const listProps = vi.fn()
const citySelectionOptions = vi.fn()
vi.mock('@/features/shows/hooks/useHomeShowCitySelection', () => ({
  useHomeShowCitySelection: (options?: unknown) => {
    citySelectionOptions(options)
    return selection
  },
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

const phoenix = { city: 'Phoenix', state: 'AZ' }
const tucson = { city: 'Tucson', state: 'AZ' }

beforeEach(() => {
  selection.effectiveCities = []
  selection.source = 'none'
  selection.isResolving = false
  listProps.mockClear()
})

describe('NearbyShowsSection', () => {
  it('names the city the rows were fetched for and links to it', () => {
    selection.effectiveCities = [phoenix]
    selection.source = 'geo'

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
    selection.effectiveCities = [phoenix, tucson]
    selection.source = 'favorites'

    render(<NearbyShowsSection id="nearby" />)

    expect(
      screen.getByRole('link', { name: 'All upcoming shows →' })
    ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ%7CTucson%2CAZ')
    // Prose joins with the subline's own separator, so two cities do not read
    // as four.
    expect(
      screen.getByText('Phoenix, AZ · Tucson, AZ · tap ♡ to save')
    ).toBeInTheDocument()
  })

  it('does not claim proximity for a city that was only a guess', () => {
    selection.effectiveCities = [phoenix]
    selection.source = 'liveliest'

    render(<NearbyShowsSection id="nearby" />)

    expect(
      screen.getByRole('heading', { name: 'Shows this week' })
    ).toBeInTheDocument()
    expect(
      screen.getByText('Phoenix, AZ · the liveliest scene right now · tap ♡ to save')
    ).toBeInTheDocument()
  })

  it('keeps the list painting while geo decides, and only the copy holds back', () => {
    selection.isResolving = true

    render(<NearbyShowsSection id="nearby" />)

    // The list is not gated on the city (an unfiltered page is truthful);
    // the heading just does not claim proximity yet.
    expect(screen.getByTestId('home-show-list')).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Shows this week' })
    ).toBeInTheDocument()
    expect(
      screen.getByText('finding your city · tap ♡ to save')
    ).toBeInTheDocument()
  })

  it('hands the list its rows, the week window, and the saved-row exclusion', () => {
    selection.effectiveCities = [phoenix]
    selection.source = 'favorites'

    render(<NearbyShowsSection id="nearby" />)

    expect(listProps).toHaveBeenCalledWith(
      expect.objectContaining({
        selection,
        rows: 4,
        excludeSaved: true,
        withinDays: 7,
        excludedLabel: 'Phoenix, AZ',
      })
    )
  })

  it('asks the selection to resolve a city, because its copy names one', () => {
    render(<NearbyShowsSection id="nearby" />)

    expect(citySelectionOptions).toHaveBeenCalledWith({
      resolveCityForCopy: true,
    })
  })

  it('keeps the Discover links row at its foot', () => {
    render(<NearbyShowsSection id="nearby" />)

    expect(
      screen.getByRole('navigation', { name: 'Discover' })
    ).toBeInTheDocument()
  })
})
