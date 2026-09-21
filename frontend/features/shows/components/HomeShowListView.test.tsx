import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { HomeShowListView } from './HomeShowListView'
import type { HomeShowCitySelection } from './useHomeShowCitySelection'

const mockUseUpcomingShows = vi.fn()
const mockUseShowSaveCountBatch = vi.fn()

vi.mock('../hooks/useShows', () => ({
  useUpcomingShows: (...args: unknown[]) => mockUseUpcomingShows(...args),
}))
vi.mock('../hooks/useSavedShows', () => ({
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
  CityFilters: () => <div data-testid="city-filters" />,
}))

const selection: HomeShowCitySelection = {
  cities: [],
  favoriteCities: [],
  effectiveCities: [],
  geoAffordanceCity: null,
  selectionDiffersFromFavorites: false,
  onFilterChange: vi.fn(),
}

const shows = [
  { id: 1, title: 'Saved one' },
  { id: 2, title: 'Not saved' },
]

beforeEach(() => {
  mockUseUpcomingShows.mockReturnValue({
    data: { shows },
    isLoading: false,
    isFetching: false,
    error: null,
  })
  mockUseShowSaveCountBatch.mockReturnValue({
    data: {
      '1': { save_count: 3, is_saved: true },
      '2': { save_count: 1, is_saved: false },
    },
    isLoading: false,
  })
})

describe('HomeShowListView', () => {
  it('renders every row by default', () => {
    render(<HomeShowListView selection={selection} />)

    expect(screen.getByText('Saved one')).toBeInTheDocument()
    expect(screen.getByText('Not saved')).toBeInTheDocument()
  })

  it('drops rows the viewer already saved when asked to', () => {
    render(<HomeShowListView selection={selection} excludeSavedShows />)

    expect(screen.queryByText('Saved one')).not.toBeInTheDocument()
    expect(screen.getByText('Not saved')).toBeInTheDocument()
  })

  it('waits for the save batch rather than painting rows it will drop', () => {
    mockUseShowSaveCountBatch.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    render(<HomeShowListView selection={selection} excludeSavedShows />)

    expect(screen.queryByTestId('show-card')).not.toBeInTheDocument()
  })

  it('does not wait on the batch when it is not filtering', () => {
    mockUseShowSaveCountBatch.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    render(<HomeShowListView selection={selection} />)

    expect(screen.getAllByTestId('show-card')).toHaveLength(2)
  })
})
