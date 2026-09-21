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

vi.mock('@/features/shows/components/useHomeShowCitySelection', () => ({
  useHomeShowCitySelection: () => selection,
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

  it('hands the list the same selection and asks it to drop saved rows', () => {
    render(<NearbyShowsSection id="nearby" />)

    expect(screen.getByTestId('home-show-list')).toBeInTheDocument()
    expect(listProps).toHaveBeenCalledWith(
      expect.objectContaining({ selection, excludeSavedShows: true })
    )
  })
})
