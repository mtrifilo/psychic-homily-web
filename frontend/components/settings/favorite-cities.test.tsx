import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { FavoriteCitiesSettings } from './favorite-cities'

// --- Mocks ---

const mockMutate = vi.fn()
let mockMutationState = {
  isPending: false,
  isError: false,
  isSuccess: false,
}

let mockProfileData: {
  user?: {
    preferences?: {
      favorite_cities?: { city: string; state: string }[]
    }
  }
} = {}

let mockCitiesData: {
  cities?: { city: string; state: string; show_count: number }[]
} | undefined = undefined

let mockCitiesLoading = false

vi.mock('@/lib/hooks/useAuth', () => ({
  useProfile: () => ({
    data: mockProfileData,
  }),
}))

vi.mock('@/lib/hooks/useShows', () => ({
  useShowCities: () => ({
    data: mockCitiesData,
    isLoading: mockCitiesLoading,
  }),
}))

vi.mock('@/lib/hooks/useFavoriteCities', () => ({
  useSetFavoriteCities: () => ({
    mutate: mockMutate,
    ...mockMutationState,
  }),
}))

// --- Tests ---

describe('FavoriteCitiesSettings', () => {
  beforeEach(() => {
    mockMutate.mockReset()
    mockMutationState = { isPending: false, isError: false, isSuccess: false }
    mockProfileData = {}
    mockCitiesData = undefined
    mockCitiesLoading = false
  })

  it('renders the card title and description', () => {
    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('Favorite Cities')).toBeInTheDocument()
    expect(
      screen.getByText(/Choose your default cities for the show calendar/)
    ).toBeInTheDocument()
  })

  it('shows loading state when cities are loading', () => {
    mockCitiesLoading = true
    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('Loading cities...')).toBeInTheDocument()
  })

  it('shows empty state when no cities available', () => {
    mockCitiesData = { cities: [] }
    renderWithProviders(<FavoriteCitiesSettings />)

    expect(
      screen.getByText('No cities with upcoming shows found.')
    ).toBeInTheDocument()
  })

  it('renders city chips when cities are available', () => {
    mockCitiesData = {
      cities: [
        { city: 'Phoenix', state: 'AZ', show_count: 8 },
        { city: 'Tempe', state: 'AZ', show_count: 3 },
      ],
    }
    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
    expect(screen.getByText('Tempe, AZ')).toBeInTheDocument()
  })

  it('toggles a city when chip is clicked', async () => {
    mockCitiesData = {
      cities: [
        { city: 'Phoenix', state: 'AZ', show_count: 8 },
        { city: 'Tempe', state: 'AZ', show_count: 3 },
      ],
    }
    mockProfileData = { user: { preferences: { favorite_cities: [] } } }

    const user = userEvent.setup()
    renderWithProviders(<FavoriteCitiesSettings />)

    await user.click(screen.getByText('Phoenix, AZ'))

    expect(mockMutate).toHaveBeenCalledWith([{ city: 'Phoenix', state: 'AZ' }])
  })

  it('removes a city when already-selected chip is clicked', async () => {
    mockCitiesData = {
      cities: [
        { city: 'Phoenix', state: 'AZ', show_count: 8 },
        { city: 'Tempe', state: 'AZ', show_count: 3 },
      ],
    }
    mockProfileData = {
      user: {
        preferences: {
          favorite_cities: [{ city: 'Phoenix', state: 'AZ' }],
        },
      },
    }

    const user = userEvent.setup()
    renderWithProviders(<FavoriteCitiesSettings />)

    await user.click(screen.getByText('Phoenix, AZ'))

    expect(mockMutate).toHaveBeenCalledWith([])
  })

  it('shows selected count and Clear all button when cities are selected', () => {
    mockCitiesData = {
      cities: [
        { city: 'Phoenix', state: 'AZ', show_count: 8 },
        { city: 'Tempe', state: 'AZ', show_count: 3 },
      ],
    }
    mockProfileData = {
      user: {
        preferences: {
          favorite_cities: [
            { city: 'Phoenix', state: 'AZ' },
            { city: 'Tempe', state: 'AZ' },
          ],
        },
      },
    }

    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('2 cities selected')).toBeInTheDocument()
    expect(screen.getByText('Clear all')).toBeInTheDocument()
  })

  it('shows singular "city" when 1 city is selected', () => {
    mockCitiesData = {
      cities: [{ city: 'Phoenix', state: 'AZ', show_count: 8 }],
    }
    mockProfileData = {
      user: {
        preferences: {
          favorite_cities: [{ city: 'Phoenix', state: 'AZ' }],
        },
      },
    }

    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('1 city selected')).toBeInTheDocument()
  })

  it('does not show selected count or Clear all when no cities are selected', () => {
    mockCitiesData = {
      cities: [{ city: 'Phoenix', state: 'AZ', show_count: 8 }],
    }
    mockProfileData = { user: { preferences: { favorite_cities: [] } } }

    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.queryByText(/selected/)).not.toBeInTheDocument()
    expect(screen.queryByText('Clear all')).not.toBeInTheDocument()
  })

  it('calls mutate with empty array when Clear all is clicked', async () => {
    mockCitiesData = {
      cities: [{ city: 'Phoenix', state: 'AZ', show_count: 8 }],
    }
    mockProfileData = {
      user: {
        preferences: {
          favorite_cities: [{ city: 'Phoenix', state: 'AZ' }],
        },
      },
    }

    const user = userEvent.setup()
    renderWithProviders(<FavoriteCitiesSettings />)

    await user.click(screen.getByText('Clear all'))

    expect(mockMutate).toHaveBeenCalledWith([])
  })

  it('shows Saving indicator when mutation is pending', () => {
    mockCitiesData = {
      cities: [{ city: 'Phoenix', state: 'AZ', show_count: 8 }],
    }
    mockMutationState = { isPending: true, isError: false, isSuccess: false }

    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('Saving...')).toBeInTheDocument()
  })

  it('shows Saved indicator when mutation succeeds', () => {
    mockCitiesData = {
      cities: [{ city: 'Phoenix', state: 'AZ', show_count: 8 }],
    }
    mockMutationState = { isPending: false, isError: false, isSuccess: true }

    renderWithProviders(<FavoriteCitiesSettings />)

    expect(screen.getByText('Saved')).toBeInTheDocument()
  })

  it('shows error message when mutation fails', () => {
    mockCitiesData = {
      cities: [{ city: 'Phoenix', state: 'AZ', show_count: 8 }],
    }
    mockMutationState = { isPending: false, isError: true, isSuccess: false }

    renderWithProviders(<FavoriteCitiesSettings />)

    expect(
      screen.getByText('Failed to save. Please try again.')
    ).toBeInTheDocument()
  })
})
