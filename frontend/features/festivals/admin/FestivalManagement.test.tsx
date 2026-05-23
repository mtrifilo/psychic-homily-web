import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { FestivalListItem } from '../types'

// Read hooks
const mockUseFestivals = vi.fn()
const mockUseFestival = vi.fn()
const mockUseFestivalLineup = vi.fn()
const mockUseFestivalVenues = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useFestivals: (opts: unknown) => mockUseFestivals(opts),
  useFestival: (opts: unknown) => mockUseFestival(opts),
  useFestivalLineup: (opts: unknown) => mockUseFestivalLineup(opts),
  useFestivalVenues: (opts: unknown) => mockUseFestivalVenues(opts),
}))

// Admin mutation hooks. The factory is hoisted above module scope by vitest,
// so the no-op return value is inlined rather than referencing an outer const.
vi.mock('../hooks/useAdminFestivals', () => {
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    useCreateFestival: noopMutation,
    useUpdateFestival: noopMutation,
    useDeleteFestival: noopMutation,
    useAddFestivalArtist: noopMutation,
    useUpdateFestivalArtist: noopMutation,
    useRemoveFestivalArtist: noopMutation,
    useAddFestivalVenue: noopMutation,
    useRemoveFestivalVenue: noopMutation,
  }
})

// Search hooks pulled in by the lineup/venue management panels
vi.mock('@/features/artists', () => ({
  useArtistSearch: () => ({ data: { artists: [] as unknown[] }, isLoading: false }),
}))
vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({ data: { venues: [] as unknown[] }, isLoading: false }),
}))

import { FestivalManagement } from './FestivalManagement'

function makeFestival(overrides: Partial<FestivalListItem> = {}): FestivalListItem {
  return {
    id: 1,
    name: 'FORM Arcosanti',
    slug: 'form-arcosanti-2025',
    series_slug: 'form-arcosanti',
    edition_year: 2025,
    city: 'Mayer',
    state: 'AZ',
    start_date: '2025-05-09',
    end_date: '2025-05-11',
    status: 'confirmed',
    artist_count: 12,
    venue_count: 1,
    ...overrides,
  }
}

describe('FestivalManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseFestival.mockReturnValue({ data: undefined, isLoading: false })
    mockUseFestivalLineup.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
    })
    mockUseFestivalVenues.mockReturnValue({
      data: { venues: [], count: 0 },
      isLoading: false,
    })
  })

  it('shows the loading spinner while festivals are fetching', () => {
    mockUseFestivals.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders an error banner when the query fails', () => {
    mockUseFestivals.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('load failed'),
    })
    renderWithProviders(<FestivalManagement />)
    expect(screen.getByText('load failed')).toBeInTheDocument()
  })

  it('renders the empty state when there are no festivals', () => {
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)
    expect(screen.getByText('No Festivals Found')).toBeInTheDocument()
    expect(
      screen.getByText(/No festivals yet\. Create your first festival/)
    ).toBeInTheDocument()
  })

  it('lists festivals with their status and a count', () => {
    mockUseFestivals.mockReturnValue({
      data: {
        festivals: [
          makeFestival({ id: 1, name: 'FORM Arcosanti' }),
          makeFestival({ id: 2, name: 'Desert Daze', status: 'announced' }),
        ],
        count: 2,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    expect(screen.getByText('FORM Arcosanti')).toBeInTheDocument()
    expect(screen.getByText('Desert Daze')).toBeInTheDocument()
    expect(screen.getByText('2 festivals')).toBeInTheDocument()
    // "Confirmed"/"Announced" also appear as <option> text in the status
    // filter; the row status badges are <div>s, so filter to those.
    const confirmedBadges = screen
      .getAllByText('Confirmed')
      .filter((el) => el.tagName === 'DIV')
    expect(confirmedBadges).toHaveLength(1)
    const announcedBadges = screen
      .getAllByText('Announced')
      .filter((el) => el.tagName === 'DIV')
    expect(announcedBadges).toHaveLength(1)
  })

  it('filters the list client-side by the debounced search input', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: {
        festivals: [
          makeFestival({ id: 1, name: 'FORM Arcosanti' }),
          makeFestival({ id: 2, name: 'Desert Daze' }),
        ],
        count: 2,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.type(screen.getByPlaceholderText('Search festivals...'), 'desert')

    await waitFor(() =>
      expect(screen.queryByText('FORM Arcosanti')).not.toBeInTheDocument()
    )
    expect(screen.getByText('Desert Daze')).toBeInTheDocument()
    expect(screen.getByText(/matching "desert"/)).toBeInTheDocument()
  })

  it('opens the create dialog from the New Festival button', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('button', { name: /New Festival/ }))
    expect(
      screen.getByRole('heading', { name: 'Create Festival' })
    ).toBeInTheDocument()
  })

  it('opens the delete confirmation with the festival name', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    const { container } = renderWithProviders(<FestivalManagement />)

    // Edit/Delete are icon-only buttons with no accessible name; the delete
    // button is the only one carrying the destructive hover affordance.
    const deleteButton = container.querySelector<HTMLButtonElement>(
      'button.hover\\:text-destructive'
    )
    expect(deleteButton).not.toBeNull()
    await user.click(deleteButton!)

    expect(
      screen.getByRole('heading', { name: 'Delete Festival' })
    ).toBeInTheDocument()
    // The confirmation copy echoes the name (quoted) in a bold span. The row
    // behind the dialog also shows the name, so match the quoted variant.
    expect(screen.getByText('"FORM Arcosanti"')).toBeInTheDocument()
  })

  it('navigates into the lineup management panel', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ id: 7, name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('button', { name: /Lineup/ }))

    expect(
      screen.getByRole('heading', { name: /FORM Arcosanti - Lineup/ })
    ).toBeInTheDocument()
    expect(mockUseFestivalLineup).toHaveBeenCalledWith({
      festivalId: 7,
      enabled: true,
    })
    // Back button returns to the list
    await user.click(screen.getByRole('button', { name: /Back/ }))
    expect(
      screen.getByRole('heading', { name: 'Festivals' })
    ).toBeInTheDocument()
  })

  it('navigates into the venue management panel', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ id: 7, name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('button', { name: /Venues/ }))

    expect(
      screen.getByRole('heading', { name: /FORM Arcosanti - Venues/ })
    ).toBeInTheDocument()
    expect(mockUseFestivalVenues).toHaveBeenCalledWith({
      festivalIdOrSlug: 7,
      enabled: true,
    })
  })
})
